package permissions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// Mode controls the permission behavior
type Mode int

const (
	ModeAllowAll  Mode = iota // bypass all permission checks
	ModeAskAlways             // ask for every tool use
	ModeRuleBased             // use pattern-based rules
)

// Decision represents a permission decision
type Decision int

const (
	DecisionAllow Decision = iota
	DecisionDeny
	DecisionAsk
	DecisionAllowOnce // allow this single invocation; do NOT cache in sessionCache
)

// Rule defines a permission rule
type Rule struct {
	Tool     string   `json:"tool"`             // tool name pattern (supports *)
	Pattern  string   `json:"pattern"`          // input pattern to match (optional)
	Decision Decision `json:"decision"`         // allow, deny, or ask
	Source   string   `json:"source,omitempty"` // optional settings/policy provenance
}

// CheckOptions applies per-request permission behavior without mutating the
// session-level checker. Subagents use this to mirror Claude Code's per-agent
// permissionMode semantics.
type CheckOptions struct {
	ModeOverride      *Mode
	AvoidPrompts      bool
	Required          bool
	policyRequiredAsk bool
	Prompt            *PromptRequest
	ctx               context.Context
	response          *PromptResponse
	noCache           bool
	runtimeSnapshot   *types.ToolRuntimeContext
}

// Checker evaluates permission decisions for tool usage
type Checker struct {
	mode             Mode
	rules            []Rule
	mu               sync.RWMutex
	sessionCache     map[string]Decision                                  // cached "always allow" decisions for this session
	promptFunc       func(toolName string, input map[string]any) Decision // interactive prompt
	structuredPrompt StructuredPromptFunc

	// Phase 4: Feature gates for conditional tool availability
	featureGates map[string]bool

	// frozen is set to true after the first call to Check(). Once frozen,
	// SetMode will refuse to switch to ModeAllowAll, preventing prompt-injection
	// attacks from escalating permissions mid-session.
	frozen bool

	// Tool whitelist/blacklist — bypass-immune (enforced regardless of Mode).
	allowedTools    map[string]bool // nil = all allowed; non-nil = only listed tools permitted
	disallowedTools map[string]bool // non-nil = listed tools are always denied
}

// NewChecker creates a new permission checker
func NewChecker(mode Mode, rules []Rule) *Checker {
	return &Checker{
		mode:         mode,
		rules:        rules,
		sessionCache: make(map[string]Decision),
		featureGates: make(map[string]bool),
	}
}

// SetPromptFunc sets the interactive prompt function for AskAlways mode
func (c *Checker) SetPromptFunc(fn func(toolName string, input map[string]any) Decision) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.promptFunc = fn
}

// SetStructuredPromptFunc installs the lossless, cancellable permission and
// plan decision surface. The legacy prompt remains as a fallback.
func (c *Checker) SetStructuredPromptFunc(fn StructuredPromptFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.structuredPrompt = fn
}

// CheckPrompt evaluates a fully attributed permission request and returns the
// presentation outcome separately from the execution decision.
func (c *Checker) CheckPrompt(ctx context.Context, request PromptRequest, opts CheckOptions) PromptResponse {
	if ctx == nil {
		ctx = context.Background()
	}
	var response PromptResponse
	opts.Prompt = &request
	opts.ctx = ctx
	opts.response = &response
	decision := c.CheckWithOptions(request.ToolName, request.Input, opts)
	if response.Outcome == "" {
		response = responseForDecision(decision)
	} else if response.Choice == "" {
		response.Decision = decision
	}
	return response
}

// ResetSession clears decisions whose scope is the active conversation while
// preserving configured rules, tool lists, feature gates, and the user's mode.
func (c *Checker) ResetSession() {
	c.mu.Lock()
	c.sessionCache = make(map[string]Decision)
	c.frozen = false
	c.mu.Unlock()
}

// IsFeatureEnabled checks if a feature gate is enabled
// Phase 4: Used for conditional tool availability
func (c *Checker) IsFeatureEnabled(feature string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.featureGates[feature]
}

// SetFeatureGate enables or disables a feature gate
// Phase 4: Allows runtime control of tool availability
func (c *Checker) SetFeatureGate(feature string, enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.featureGates[feature] = enabled
}

// SetFeatureGates sets multiple feature gates at once
// Phase 4: Bulk update of feature gates
func (c *Checker) SetFeatureGates(gates map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for feature, enabled := range gates {
		c.featureGates[feature] = enabled
	}
}

// SetMode changes the permission mode. Once Check() has been called at least
// once the session is considered "frozen": switching to ModeAllowAll is
// rejected to prevent prompt-injection from escalating privileges mid-session.
func (c *Checker) SetMode(m Mode) error {
	return c.setMode(m, false)
}

// SetModeFromUser changes the permission mode for an explicit local UI action.
// Unlike SetMode, it can enter ModeAllowAll after the first permission check:
// the interactive Shift+Tab mode switch is user-controlled, while SetMode stays
// conservative for programmatic callers.
func (c *Checker) SetModeFromUser(m Mode) error {
	return c.setMode(m, true)
}

func (c *Checker) setMode(m Mode, userRequested bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frozen && m == ModeAllowAll && !userRequested {
		return fmt.Errorf("%s", permissionText(i18n.KeyPermissionModeAllowAllFrozen))
	}
	c.mode = m
	return nil
}

// Mode returns the current permission mode. Thread-safe.
func (c *Checker) Mode() Mode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mode
}

// Check evaluates whether a tool can be used with the given input
func (c *Checker) Check(toolName string, input map[string]any) Decision {
	return c.CheckWithOptions(toolName, input, CheckOptions{})
}

// CheckWithOptions evaluates whether a tool can be used with request-scoped
// behavior such as a temporary mode override or non-interactive prompt denial.
func (c *Checker) CheckWithOptions(toolName string, input map[string]any, opts CheckOptions) Decision {
	// Freeze the session on first call — prevents later escalation to ModeAllowAll.
	// Snapshot mode and tool lists under the same lock to avoid data races.
	c.mu.Lock()
	c.frozen = true
	mode := c.mode
	disallowed := c.disallowedTools
	allowed := c.allowedTools
	rules := append([]Rule(nil), c.rules...)
	c.mu.Unlock()
	var snapshotDeniedRules, snapshotAskRules []Rule
	if opts.runtimeSnapshot != nil {
		runtime := opts.runtimeSnapshot
		rules = permissionRulesFromRuntimeSnapshot(*runtime)
		snapshotDeniedRules = permissionRulesForDecision(runtime.DeniedRules, DecisionDeny)
		snapshotAskRules = permissionRulesForDecision(runtime.AskRules, DecisionAsk)
		allowed = clonePermissionToolMap(runtime.AllowedTools)
		disallowed = clonePermissionToolMap(runtime.DeniedTools)
		// A child must not consume or populate the mutable foreground
		// session cache. Its snapshot rules remain authoritative for life.
		opts.noCache = true
	}
	if opts.ModeOverride != nil {
		mode = *opts.ModeOverride
	}

	// ── Bypass-immune safety checks — always run regardless of mode ──

	// 1. Path/command safety (Task 1+2+5): blocks writes to .git/, .env, .ssh/, etc.
	//    and dangerous bash commands (rm -rf /, curl|bash, etc.)
	if d, _ := SafetyCheck(toolName, input); d == DecisionDeny {
		return DecisionDeny
	}

	// 2. Tool blacklist — bypass-immune, checked before any mode logic.
	if disallowed != nil && disallowed[toolName] {
		return DecisionDeny
	}
	// 3. Tool whitelist — bypass-immune, checked before any mode logic.
	if allowed != nil && !allowed[toolName] {
		return DecisionDeny
	}
	if _, matched := matchingRule(snapshotDeniedRules, toolName, input); matched {
		return DecisionDeny
	}
	// Auto mode is an explicit user choice to execute PolicyRequiredAsk calls
	// without an interactive round trip. Hard safety checks, tool lists, and
	// snapshot deny rules above remain authoritative. Keep other Required
	// requests (for example an explicit tool Ask contract) interactive.
	if opts.Required && !(mode == ModeAllowAll && opts.policyRequiredAsk) {
		opts.noCache = true
		opts = withPromptRuleSource(opts, permissionText(i18n.KeyPermissionMandatoryPolicy))
		return c.askOrCache(toolName, input, opts)
	}
	// 4. Mandatory approval checks remain invocation-scoped in interactive
	// modes. Auto mode deliberately consumes PolicyRequiredAsk without a prompt.
	if mode != ModeAllowAll {
		if d, handled := c.handleMandatoryAsk(toolName, input, opts); handled {
			return d
		}
	}

	// TestingPermission mirrors the TS testing helper: it always goes through
	// an interactive permission prompt and never caches allow decisions.
	if toolName == "TestingPermission" {
		if opts.AvoidPrompts || !c.hasPrompt() {
			return DecisionDeny
		}
		opts.noCache = true
		opts = withPromptRuleSource(opts, permissionText(i18n.KeyPermissionTestingPolicy))
		d := c.askOrCache(toolName, input, opts)
		if d == DecisionAllowOnce {
			return DecisionAllow
		}
		return d
	}
	if rule, matched := matchingRule(snapshotAskRules, toolName, input); matched {
		opts = withPromptRuleSource(opts, configuredAskRuleSource(rule))
		return c.askOrCache(toolName, input, opts)
	}

	switch mode {
	case ModeAllowAll:
		return DecisionAllow
	case ModeAskAlways:
		if d, handled := c.handleAdvisoryAsk(toolName, input, opts); handled {
			return d
		}
		if isLowRiskShellInvocation(toolName, input) {
			return DecisionAllow
		}
		opts = withPromptRuleSource(opts, permissionText(i18n.KeyPermissionAskAlwaysPolicy))
		return c.askOrCache(toolName, input, opts)
	case ModeRuleBased:
		if rule, matched := matchingRule(rules, toolName, input); matched {
			switch rule.Decision {
			case DecisionDeny:
				return DecisionDeny
			case DecisionAsk:
				opts = withPromptRuleSource(opts, configuredAskRuleSource(rule))
				return c.askOrCache(toolName, input, opts)
			}
		}
		if d, handled := c.handleAdvisoryAsk(toolName, input, opts); handled {
			return d
		}
		if decision, matched := matchingRuleDecision(rules, toolName, input); matched && decision == DecisionAllow {
			return DecisionAllow
		}
		if isLowRiskShellInvocation(toolName, input) {
			return DecisionAllow
		}
		opts = withPromptRuleSource(opts, permissionText(i18n.KeyPermissionRuleFallback))
		return c.askOrCache(toolName, input, opts)
	}
	return DecisionAsk
}

func (c *Checker) handleAdvisoryAsk(toolName string, input map[string]any, opts CheckOptions) (Decision, bool) {
	if d, reason := AdvisoryCheck(toolName, input); d == DecisionAsk {
		opts = withPromptReason(opts, reason, permissionText(i18n.KeyPermissionAdvisoryPolicy))
		return c.askOrCache(toolName, input, opts), true
	}
	return DecisionDeny, false
}

func (c *Checker) handleMandatoryAsk(toolName string, input map[string]any, opts CheckOptions) (Decision, bool) {
	if d, reason := MandatoryApprovalCheck(toolName, input); d == DecisionAsk {
		// Mandatory policy decisions are invocation-scoped. A prior "always
		// allow" answer must never satisfy a later dynamic command.
		opts.noCache = true
		opts = withPromptReason(opts, reason, permissionText(i18n.KeyPermissionMandatoryPolicy))
		return c.askOrCache(toolName, input, opts), true
	}
	return DecisionDeny, false
}

// SetAllowedTools sets the tool whitelist. Pass nil to allow all tools.
// When non-nil, only the listed tools are permitted; all others are denied.
// This check is bypass-immune — it applies regardless of the permission mode.
func (c *Checker) SetAllowedTools(tools []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if tools == nil {
		c.allowedTools = nil
		return
	}
	m := make(map[string]bool, len(tools))
	for _, t := range tools {
		m[t] = true
	}
	c.allowedTools = m
}

// SetDisallowedTools sets the tool blacklist. Pass nil to clear.
// Listed tools are always denied regardless of mode or whitelist.
// This check is bypass-immune — it applies regardless of the permission mode.
func (c *Checker) SetDisallowedTools(tools []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if tools == nil {
		c.disallowedTools = nil
		return
	}
	m := make(map[string]bool, len(tools))
	for _, t := range tools {
		m[t] = true
	}
	c.disallowedTools = m
}

func (c *Checker) askOrCache(toolName string, input map[string]any, opts CheckOptions) Decision {
	if opts.AvoidPrompts {
		return DecisionDeny
	}
	cacheKey := ""
	if !opts.noCache {
		cacheKey = c.cacheKey(toolName, input)
		c.mu.RLock()
		if d, ok := c.sessionCache[cacheKey]; ok {
			c.mu.RUnlock()
			return d
		}
		c.mu.RUnlock()
	}

	c.mu.RLock()
	structuredPrompt := c.structuredPrompt
	legacyPrompt := c.promptFunc
	c.mu.RUnlock()

	var response PromptResponse
	if structuredPrompt != nil && opts.Prompt != nil {
		request := *opts.Prompt
		request.ToolName = toolName
		request.Input = input
		ctx := opts.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		response = structuredPrompt(ctx, request)
	} else if legacyPrompt != nil {
		response = responseForDecision(legacyPrompt(toolName, input))
	} else {
		response = responseForDecision(DecisionDeny)
	}
	if response.Outcome == "" {
		response = responseForDecision(response.Decision)
	}
	if opts.response != nil {
		*opts.response = response
	}
	d := response.Decision
	if response.Outcome != PromptOutcomeApproved {
		d = DecisionDeny
	}
	if d == DecisionAllow {
		if !opts.noCache {
			// "always" — cache so we don't ask again for the same operation
			c.mu.Lock()
			c.sessionCache[cacheKey] = DecisionAllow
			c.mu.Unlock()
		}
	}
	// DecisionAllowOnce: allow this call but do not cache.
	if d == DecisionAllowOnce {
		return DecisionAllow
	}
	return d
}

func permissionRulesFromRuntimeSnapshot(snapshot types.ToolRuntimeContext) []Rule {
	rules := make([]Rule, 0, len(snapshot.DeniedRules)+len(snapshot.AskRules)+len(snapshot.AllowedRules))
	appendRules := func(values []types.PermissionRuleValue, decision Decision) {
		for _, value := range values {
			toolName := strings.TrimSpace(value.ToolName)
			if toolName == "" {
				continue
			}
			rules = append(rules, Rule{
				Tool: toolName, Pattern: strings.TrimSpace(value.RuleContent), Decision: decision,
				Source: permissionText(i18n.KeyPermissionSnapshotSource),
			})
		}
	}
	// Snapshot rules use restrictive precedence when overlapping.
	appendRules(snapshot.DeniedRules, DecisionDeny)
	appendRules(snapshot.AskRules, DecisionAsk)
	appendRules(snapshot.AllowedRules, DecisionAllow)
	return rules
}

func permissionRulesForDecision(values []types.PermissionRuleValue, decision Decision) []Rule {
	rules := make([]Rule, 0, len(values))
	for _, value := range values {
		toolName := strings.TrimSpace(value.ToolName)
		if toolName == "" {
			continue
		}
		rules = append(rules, Rule{
			Tool: toolName, Pattern: strings.TrimSpace(value.RuleContent), Decision: decision,
			Source: permissionText(i18n.KeyPermissionSnapshotSource),
		})
	}
	return rules
}

func clonePermissionToolMap(source map[string]bool) map[string]bool {
	if source == nil {
		return nil
	}
	cloned := make(map[string]bool, len(source))
	for toolName, denied := range source {
		cloned[toolName] = denied
	}
	return cloned
}

func clonePermissionRuntimeContext(source types.ToolRuntimeContext) types.ToolRuntimeContext {
	cloned := source
	cloned.AllowedDirs = append([]string(nil), source.AllowedDirs...)
	cloned.Features = clonePermissionToolMap(source.Features)
	cloned.AllowedTools = clonePermissionToolMap(source.AllowedTools)
	cloned.DeniedTools = clonePermissionToolMap(source.DeniedTools)
	cloned.AllowedRules = append([]types.PermissionRuleValue(nil), source.AllowedRules...)
	cloned.DeniedRules = append([]types.PermissionRuleValue(nil), source.DeniedRules...)
	cloned.AskRules = append([]types.PermissionRuleValue(nil), source.AskRules...)
	return cloned
}

func (c *Checker) evaluateRules(toolName string, input map[string]any) Decision {
	if decision, matched := matchingRuleDecision(c.rules, toolName, input); matched {
		if decision == DecisionAsk {
			return c.askOrCache(toolName, input, CheckOptions{})
		}
		return decision
	}
	if isLowRiskShellInvocation(toolName, input) {
		return DecisionAllow
	}
	// No matching rule — ask
	return c.askOrCache(toolName, input, CheckOptions{})
}

func (c *Checker) hasPrompt() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.structuredPrompt != nil || c.promptFunc != nil
}

func withPromptReason(opts CheckOptions, reason, source string) CheckOptions {
	if opts.Prompt == nil {
		return opts
	}
	request := *opts.Prompt
	if strings.TrimSpace(request.RiskReason) == "" {
		request.RiskReason = reason
	}
	opts.Prompt = &request
	return withPromptRuleSource(opts, source)
}

func withPromptRuleSource(opts CheckOptions, source string) CheckOptions {
	if opts.Prompt == nil || !isBaselinePromptRuleSource(opts.Prompt.RuleSource) {
		return opts
	}
	request := *opts.Prompt
	request.RuleSource = source
	opts.Prompt = &request
	return opts
}

func isBaselinePromptRuleSource(source string) bool {
	source = strings.TrimSpace(source)
	return source == "" || source == "tool permission policy" || isLocalizedToolContractSource(source)
}

func configuredAskRuleSource(rule Rule) string {
	if source := strings.TrimSpace(rule.Source); source != "" {
		return source
	}
	if pattern := strings.TrimSpace(rule.Pattern); pattern != "" {
		return permissionFormat(i18n.KeyPermissionConfiguredPatternRule, rule.Tool, pattern)
	}
	return permissionFormat(i18n.KeyPermissionConfiguredRule, rule.Tool)
}

func matchingRule(rules []Rule, toolName string, input map[string]any) (Rule, bool) {
	var best Rule
	bestStrength := -1
	for _, rule := range rules {
		if !matchPattern(rule.Tool, toolName) {
			continue
		}
		if rule.Pattern != "" && !matchInputPattern(rule.Pattern, input) {
			continue
		}
		strength := 0
		switch rule.Decision {
		case DecisionDeny:
			strength = 3
		case DecisionAsk:
			strength = 2
		case DecisionAllow, DecisionAllowOnce:
			strength = 1
		}
		if strength > bestStrength {
			best = rule
			bestStrength = strength
		}
	}
	return best, bestStrength >= 0
}

func matchingRuleDecision(rules []Rule, toolName string, input map[string]any) (Decision, bool) {
	rule, matched := matchingRule(rules, toolName, input)
	return rule.Decision, matched
}

func (c *Checker) sandboxAutoApproveDecision(toolName string, input map[string]any) (Decision, bool) {
	c.mu.RLock()
	mode := c.mode
	allowed := c.allowedTools
	disallowed := c.disallowedTools
	rules := append([]Rule(nil), c.rules...)
	c.mu.RUnlock()

	if disallowed != nil && disallowed[toolName] {
		return DecisionDeny, true
	}
	if allowed != nil && !allowed[toolName] {
		return DecisionDeny, true
	}
	if mode != ModeRuleBased {
		return DecisionDeny, false
	}
	return matchingRuleDecision(rules, toolName, input)
}

func (c *Checker) cacheKey(toolName string, input map[string]any) string {
	// For Bash, hash the full command to prevent cache collision between
	// commands that share the same first token (e.g. "git status" vs "git push --force")
	switch toolName {
	case "Bash", "PowerShell":
		if cmd, ok := input["command"].(string); ok {
			h := sha256.Sum256([]byte(cmd))
			return fmt.Sprintf("%s:%x", toolName, h[:8])
		}
	case "Write", "FileWrite", "Edit", "FileEdit", "Read", "FileRead",
		"FileDelete", "FileAppend":
		// W4 fix: include file path in cache key for all file-based tools,
		// preventing "FileDelete foo.go → Allow" from caching for "FileDelete .env".
		if fp, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("%s:%s", toolName, filepath.Clean(fp))
		}
	case "NotebookEdit":
		if fp, ok := input["notebook_path"].(string); ok {
			return fmt.Sprintf("%s:%s", toolName, filepath.Clean(fp))
		}
	case "FileMove", "FileLink":
		// Cache key includes both path fields.
		src, _ := input["source"].(string)
		dst, _ := input["destination"].(string)
		if src == "" {
			src, _ = input["target"].(string)
		}
		if dst == "" {
			dst, _ = input["link_path"].(string)
		}
		return fmt.Sprintf("%s:%s→%s", toolName, filepath.Clean(src), filepath.Clean(dst))
	case "SendMessage":
		target := sendMessageTarget(input)
		if msg, ok := input["message"]; ok {
			if data, err := json.Marshal(msg); err == nil {
				h := sha256.Sum256(data)
				return fmt.Sprintf("SendMessage:%s:%x", target, h[:8])
			}
		}
		if content, ok := input["content"].(string); ok && content != "" {
			h := sha256.Sum256([]byte(content))
			return fmt.Sprintf("SendMessage:%s:%x", target, h[:8])
		}
		if target != "" {
			return "SendMessage:" + target
		}
	}
	if data, err := json.Marshal(input); err == nil {
		h := sha256.Sum256(data)
		return fmt.Sprintf("%s:%x", toolName, h[:8])
	}
	return toolName
}

func isLowRiskShellInvocation(toolName string, input map[string]any) bool {
	switch toolName {
	case "Bash", "PowerShell":
		return ClassifyRisk(toolName, input) == RiskLow
	default:
		return false
	}
}

// matchPattern checks if name matches a glob-style pattern.
// Returns true on match error to fail closed (deny by default).
func matchPattern(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	matched, err := filepath.Match(pattern, name)
	if err != nil {
		// Fail closed: a broken pattern is treated as a match so that
		// deny rules with bad patterns still block access.
		return true
	}
	return matched
}

// matchInputPattern checks if any input value matches the pattern.
// For file_path fields, it uses filepath.Match (glob). For other fields,
// it uses exact prefix matching instead of loose substring matching.
func matchInputPattern(pattern string, input map[string]any) bool {
	for key, v := range input {
		s, ok := v.(string)
		if !ok {
			continue
		}
		switch key {
		case "file_path":
			// Use filepath.Match for file paths; fail closed on error
			matched, err := filepath.Match(pattern, s)
			if err != nil {
				return true // fail closed
			}
			if matched {
				return true
			}
		case "command":
			// Exact prefix match for commands
			if strings.HasPrefix(s, pattern) {
				return true
			}
		default:
			// Exact match for other fields
			if s == pattern {
				return true
			}
		}
	}
	return false
}
