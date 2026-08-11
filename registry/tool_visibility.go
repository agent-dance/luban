package registry

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/types"
)

type toolDiscoveryMetadataProvider interface {
	ToolDiscoveryMetadata() toolmeta.Metadata
}

type toolPermissionIdentityProvider interface {
	ToolPermissionIdentity() string
}

type permissionGrantRecord struct {
	toolName       string
	canonicalTool  string
	toolGeneration uint64
	digest         [32]byte
	binding        types.ToolPermissionBinding
	policyCode     string
	expiresAt      time.Time
	executable     bool
}

func shellPermissionDigest(toolName string, input map[string]any) ([32]byte, bool) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return [32]byte{}, false
	}
	return sha256.Sum256(append(append([]byte(toolName), 0), encoded...)), true
}

func permissionRuntimeDigest(runtime types.ToolRuntimeContext) string {
	encoded, err := json.Marshal(runtime)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func permissionBindingForRequest(request types.ToolPermissionRequest) types.ToolPermissionBinding {
	binding := request.Binding()
	binding.PolicyOwnerSessionID = strings.TrimSpace(request.Runtime.SessionID)
	binding.PolicySnapshotDigest = permissionRuntimeDigest(request.Runtime)
	return binding
}

const maxPermissionGrants = 4096

func (r *Registry) issuePermissionGrantAtGeneration(toolName, canonical string, generation uint64, input map[string]any, binding types.ToolPermissionBinding, policyCode string, executable bool) string {
	if r == nil || toolName == "" {
		return ""
	}
	if !r.generationMatches(canonical, generation) {
		return ""
	}
	digest, ok := shellPermissionDigest(toolName, input)
	if !ok {
		return ""
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return ""
	}
	token := hex.EncodeToString(random)
	r.permissionGrantMu.Lock()
	if r.permissionGrants == nil {
		r.permissionGrants = make(map[string]permissionGrantRecord)
	}
	now := time.Now()
	for grant, record := range r.permissionGrants {
		if !record.expiresAt.After(now) {
			delete(r.permissionGrants, grant)
		}
	}
	if len(r.permissionGrants) >= maxPermissionGrants {
		r.permissionGrantMu.Unlock()
		return ""
	}
	r.permissionGrants[token] = permissionGrantRecord{
		toolName: toolName, canonicalTool: canonical, toolGeneration: generation,
		digest: digest, binding: binding,
		policyCode: policyCode, expiresAt: now.Add(5 * time.Minute), executable: executable,
	}
	r.permissionGrantMu.Unlock()
	return token
}

func (r *Registry) consumePermissionGrant(token, toolName string, input map[string]any, binding types.ToolPermissionBinding, policyCode, dispatchCanonical string, dispatchGeneration uint64) (permissionGrantRecord, bool) {
	if r == nil || token == "" {
		return permissionGrantRecord{}, false
	}
	digest, digestOK := shellPermissionDigest(toolName, input)
	r.permissionGrantMu.Lock()
	record, exists := r.permissionGrants[token]
	if exists {
		delete(r.permissionGrants, token)
	}
	r.permissionGrantMu.Unlock()
	ok := exists && digestOK && record.executable && record.expiresAt.After(time.Now()) && record.toolName == toolName &&
		record.digest == digest && record.binding == binding && record.policyCode == policyCode &&
		record.canonicalTool == dispatchCanonical && record.toolGeneration == dispatchGeneration &&
		r.generationMatches(dispatchCanonical, dispatchGeneration)
	runtime := r.RuntimeContext()
	ok = ok && record.binding.PolicySnapshotDigest == permissionRuntimeDigest(runtime) && permissionOwnerBindingValid(record.binding, runtime)
	return record, ok
}

// RevokePermissionGrant invalidates an unused handoff after a rejected or
// cancelled approval. Revocation is idempotent and reveals no grant details.
func (r *Registry) RevokePermissionGrant(token string) {
	if r == nil || token == "" {
		return
	}
	r.permissionGrantMu.Lock()
	delete(r.permissionGrants, token)
	r.permissionGrantMu.Unlock()
}

// AuthorizePermissionGrant promotes a preflight nonce into a one-time execution
// grant after the trusted loop has received an approval decision. The external
// API requires a complete loop identity; direct Registry auto-allow uses the
// private helper without pretending an interactive approval occurred.
func (r *Registry) AuthorizePermissionGrant(token, toolName string, input map[string]any, binding types.ToolPermissionBinding, policyCode string) string {
	return r.promotePermissionGrant(token, toolName, input, binding, policyCode, true)
}

func (r *Registry) authorizePermissionGrant(token, toolName string, input map[string]any, binding types.ToolPermissionBinding, policyCode string) string {
	return r.promotePermissionGrant(token, toolName, input, binding, policyCode, false)
}

func (r *Registry) promotePermissionGrant(token, toolName string, input map[string]any, binding types.ToolPermissionBinding, policyCode string, requireCompleteOwner bool) string {
	if r == nil || token == "" {
		return ""
	}
	// Take-and-burn comes first. Every failure below permanently invalidates the
	// nonce, including malformed bindings, stale runtime policy, and unhashable
	// input.
	r.permissionGrantMu.Lock()
	record, exists := r.permissionGrants[token]
	if exists {
		delete(r.permissionGrants, token)
	}
	r.permissionGrantMu.Unlock()
	runtime := r.RuntimeContext()
	digest, digestOK := shellPermissionDigest(toolName, input)
	complete := binding.SessionID != "" && binding.TurnID != "" && binding.ToolUseID != "" && binding.ApprovalEpoch != "" && binding.PolicySnapshotDigest != ""
	if requireCompleteOwner && !complete {
		return ""
	}
	if !exists || record.executable || !record.expiresAt.After(time.Now()) || record.toolName != toolName ||
		record.binding != binding || record.policyCode != policyCode || !digestOK || record.digest != digest ||
		binding.PolicySnapshotDigest == "" || binding.PolicySnapshotDigest != permissionRuntimeDigest(runtime) ||
		!permissionOwnerBindingValid(binding, runtime) || !r.generationMatches(record.canonicalTool, record.toolGeneration) {
		return ""
	}
	return r.issuePermissionGrantAtGeneration(
		toolName, record.canonicalTool, record.toolGeneration, input, binding, policyCode, true,
	)
}

func permissionOwnerBindingValid(binding types.ToolPermissionBinding, runtime types.ToolRuntimeContext) bool {
	owner := strings.TrimSpace(binding.PolicyOwnerSessionID)
	runtimeOwner := strings.TrimSpace(runtime.SessionID)
	return runtimeOwner == owner
}

var builtinToolDiscoveryMetadata = map[string]toolmeta.Metadata{
	"AskUserQuestion":      {ShouldDefer: true, SearchHint: "ask user question decision choices confirmation interview prompt"},
	"ApplyPatch":           {AlwaysLoad: true, SearchHint: "multi file unified diff patch create update delete atomic"},
	"Bash":                 {AlwaysLoad: true},
	"Config":               {ShouldDefer: true, SearchHint: "config configuration settings get set runtime preferences"},
	"CreateGoal":           {AlwaysLoad: true, SearchHint: "create a persisted session goal when explicitly requested"},
	"CronCreate":           {ShouldDefer: true, SearchHint: "cron schedule recurring deferred task automation create"},
	"CronDelete":           {ShouldDefer: true, SearchHint: "cron schedule delete remove automation task"},
	"CronList":             {ShouldDefer: true, SearchHint: "cron schedule list tasks automation recurring"},
	"EnterPlanMode":        {ShouldDefer: true, SearchHint: "switch to plan mode to design an approach before coding"},
	"EnterWorktree":        {ShouldDefer: true, SearchHint: "create an isolated git worktree and switch into it"},
	"ExitPlanMode":         {ShouldDefer: true, SearchHint: "plan planning finish exit approval implementation"},
	"ExitWorktree":         {ShouldDefer: true, SearchHint: "git worktree cleanup branch remove exit"},
	"GetGoal":              {AlwaysLoad: true, SearchHint: "inspect the current persisted session goal"},
	"Edit":                 {AlwaysLoad: true},
	"Glob":                 {AlwaysLoad: true, SearchHint: "find files by name pattern or wildcard"},
	"Grep":                 {AlwaysLoad: true, SearchHint: "search file contents with regex (ripgrep)"},
	"Inspect":              {AlwaysLoad: true, SearchHint: "batch repository inspect read search glob source evidence"},
	"LSP":                  {ShouldDefer: true, SearchHint: "language server symbols definitions references diagnostics rename"},
	"ListMcpResourcesTool": {ShouldDefer: true, SearchHint: "list resources from connected MCP servers"},
	"NotebookEdit":         {ShouldDefer: true, SearchHint: "notebook jupyter ipynb cell output edit"},
	"PowerShell":           {AlwaysLoad: true},
	"Read":                 {AlwaysLoad: true, SearchHint: "read files, images, PDFs, notebooks"},
	"ReadMcpResourceTool":  {ShouldDefer: true, SearchHint: "read a specific MCP resource by URI"},
	"RemoteTrigger":        {ShouldDefer: true, SearchHint: "remote trigger scheduled agent automation run create update"},
	"Run":                  {AlwaysLoad: true, SearchHint: "run command dependency graph tests build lint verification"},
	"SendMessage":          {ShouldDefer: true, SearchHint: "send message teammate team agent coordination mailbox"},
	"TaskCreate":           {ShouldDefer: true, SearchHint: "task create work item plan tracker todo"},
	"TaskGet":              {ShouldDefer: true, SearchHint: "task get inspect details status work item"},
	"TaskList":             {ShouldDefer: true, SearchHint: "task list work items tracker status"},
	"TaskOutput":           {ShouldDefer: true, SearchHint: "task output logs progress background result"},
	"TaskStop":             {ShouldDefer: true, SearchHint: "task stop cancel background work"},
	"TaskUpdate":           {ShouldDefer: true, SearchHint: "task update status metadata progress work item"},
	"TeamCreate":           {ShouldDefer: true, SearchHint: "team create swarm teammates collaboration"},
	"TeamDelete":           {ShouldDefer: true, SearchHint: "team delete cleanup swarm teammates collaboration"},
	"UpdateGoal":           {AlwaysLoad: true, SearchHint: "revise goal acceptance criteria or mark the goal complete or blocked"},
	"WebFetch":             {ShouldDefer: true, SearchHint: "web fetch page url article website read"},
	"WebSearch":            {ShouldDefer: true, SearchHint: "web search internet query results search engine"},
	"Write":                {AlwaysLoad: true},
}

// eagerCoreToolOrder is the stable model-facing prefix for the coding tools
// used on nearly every repository task. Keeping this prefix eager avoids a
// ToolSearch round trip for basic file access, while canonical ordering keeps
// their serialized definitions stable when a rarer deferred tool is loaded.
var eagerCoreToolOrder = [...]string{
	"Bash",
	"PowerShell",
	"Read",
	"Write",
	"Edit",
	"Glob",
	"Grep",
	"Inspect",
	"ApplyPatch",
	"Run",
}

var eagerCoreToolNames = func() map[string]struct{} {
	names := make(map[string]struct{}, len(eagerCoreToolOrder))
	for _, name := range eagerCoreToolOrder {
		names[name] = struct{}{}
	}
	return names
}()

// DiscoveryMetadata returns the merged discovery metadata for a tool.
func DiscoveryMetadata(tool types.Tool) toolmeta.Metadata {
	meta := builtinToolDiscoveryMetadata[tool.Name()]
	if provider, ok := tool.(toolDiscoveryMetadataProvider); ok {
		override := provider.ToolDiscoveryMetadata()
		if override.ShouldDefer {
			meta.ShouldDefer = true
		}
		if override.AlwaysLoad {
			meta.AlwaysLoad = true
		}
		if override.SearchHint != "" {
			meta.SearchHint = override.SearchHint
		}
	}
	return meta
}

// PermissionIdentity returns the tool name used by permission rules. MCP
// dynamic tools expose their fully-qualified mcp__server__tool identity so a
// rule for a builtin like "Write" cannot accidentally match an MCP replacement
// that displays the same unqualified action.
func PermissionIdentity(tool types.Tool) string {
	if tool == nil {
		return ""
	}
	if provider, ok := tool.(toolPermissionIdentityProvider); ok {
		if identity := provider.ToolPermissionIdentity(); identity != "" {
			return identity
		}
	}
	return tool.Name()
}

// IsDeferredTool reports whether the tool should be hidden until discovered by
// ToolSearch. ToolSearch itself is always visible.
func IsDeferredTool(tool types.Tool) bool {
	if tool == nil || tool.Name() == "ToolSearch" {
		return false
	}
	meta := DiscoveryMetadata(tool)
	if meta.AlwaysLoad {
		return false
	}
	return meta.ShouldDefer
}

func featureEnabled(runtime types.ToolRuntimeContext, name string, fallback bool) bool {
	if runtime.Features == nil {
		return fallback
	}
	enabled, ok := runtime.Features[name]
	if !ok {
		return fallback
	}
	return enabled
}

func toolPermissionNames(tool types.Tool) []string {
	if tool == nil {
		return nil
	}
	names := []string{tool.Name()}
	if identity := PermissionIdentity(tool); identity != "" && identity != tool.Name() {
		names = append(names, identity)
	}
	return names
}

func toolNameAllowedByMap(tool types.Tool, allowed map[string]bool) bool {
	if allowed == nil {
		return true
	}
	for _, name := range toolPermissionNames(tool) {
		if permissionToolNameMatchesAnyRule(allowed, name) {
			return true
		}
	}
	return false
}

func toolNameDeniedByMap(tool types.Tool, denied map[string]bool) bool {
	for _, name := range toolPermissionNames(tool) {
		if permissionToolNameMatchesAnyRule(denied, name) {
			return true
		}
	}
	return false
}

func permissionToolNameMatchesAnyRule(rules map[string]bool, toolName string) bool {
	for ruleName, enabled := range rules {
		if enabled && permissionToolNameMatches(ruleName, toolName) {
			return true
		}
	}
	return false
}

func permissionToolNameMatches(ruleName, toolName string) bool {
	ruleName = strings.TrimSpace(ruleName)
	toolName = strings.TrimSpace(toolName)
	if ruleName == "" || toolName == "" {
		return false
	}
	if ruleName == toolName {
		return true
	}
	if !strings.HasPrefix(ruleName, "mcp__") || !strings.HasPrefix(toolName, "mcp__") {
		return false
	}
	ruleServer, ruleTool, ok := splitMCPPermissionName(ruleName)
	if !ok || ruleServer == "" {
		return false
	}
	toolServer, _, ok := splitMCPPermissionName(toolName)
	if !ok {
		return false
	}
	return ruleServer == toolServer && (ruleTool == "" || ruleTool == "*")
}

func splitMCPPermissionName(name string) (server, tool string, ok bool) {
	rest := strings.TrimPrefix(name, "mcp__")
	if rest == name || rest == "" {
		return "", "", false
	}
	parts := strings.SplitN(rest, "__", 2)
	server = parts[0]
	if len(parts) == 2 {
		tool = parts[1]
	}
	return server, tool, true
}

func matchingBlanketRule(tool types.Tool, rules []types.PermissionRuleValue) (types.PermissionRuleValue, bool) {
	for _, rule := range rules {
		if rule.RuleContent != "" {
			continue
		}
		for _, name := range toolPermissionNames(tool) {
			if permissionToolNameMatches(rule.ToolName, name) {
				return rule, true
			}
		}
	}
	return types.PermissionRuleValue{}, false
}

// IsToolEnabled applies blanket deny rules and TS-equivalent runtime feature
// gates before a tool is exposed or dispatched.
func (r *Registry) IsToolEnabled(tool types.Tool) bool {
	if tool == nil {
		return false
	}
	runtime := r.RuntimeContext()
	if !toolNameAllowedByMap(tool, runtime.AllowedTools) {
		return false
	}
	if toolNameDeniedByMap(tool, runtime.DeniedTools) {
		return false
	}
	if _, denied := matchingBlanketRule(tool, runtime.DeniedRules); denied {
		return false
	}
	if provider, ok := tool.(types.ToolEnabledProvider); ok && !provider.IsEnabled(runtime) {
		return false
	}

	switch tool.Name() {
	case "TeamCreate", "TeamDelete", "SendMessage":
		return featureEnabled(runtime, types.ToolFeatureTeams, true)
	case "RemoteTrigger":
		return featureEnabled(runtime, types.ToolFeatureRemoteTrigger, true)
	case "CronCreate", "CronDelete", "CronList":
		return featureEnabled(runtime, types.ToolFeatureCron, true)
	case "WebSearch":
		return featureEnabled(runtime, types.ToolFeatureWebSearch, true)
	case "ToolSearch":
		return featureEnabled(runtime, types.ToolFeatureToolSearch, true)
	case "EnterPlanMode", "ExitPlanMode", "AskUserQuestion":
		return featureEnabled(runtime, types.ToolFeaturePlanMode, true)
	case "EnterWorktree", "ExitWorktree":
		return featureEnabled(runtime, types.ToolFeatureWorktree, true)
	default:
		return true
	}
}

// EnabledTools returns the runtime-enabled tool pool without applying deferred
// loading. SDK/system metadata must advertise deferred tools even though the
// model-facing request omits them until ToolSearch loads them.
func (r *Registry) EnabledTools() []types.Tool {
	all := r.All()
	enabled := make([]types.Tool, 0, len(all))
	for _, tool := range all {
		if r.IsToolEnabled(tool) {
			enabled = append(enabled, tool)
		}
	}
	return enabled
}

// EnabledNames returns canonical names for the runtime-enabled tool pool.
func (r *Registry) EnabledNames() []string {
	tools := r.EnabledTools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

// EnabledDefinitions returns schemas for all runtime-enabled tools, including
// tools whose schemas are deferred from the current model request.
func (r *Registry) EnabledDefinitions() []types.ToolDefinition {
	return types.ToDefinitions(r.EnabledTools())
}

// VisibleTools returns the tools that should be visible to the model for the
// current loaded-tool set. When ToolSearch is not enabled, deferred discovery
// is unavailable, so every runtime-enabled tool is visible.
func (r *Registry) VisibleTools(loaded map[string]struct{}) []types.Tool {
	all := r.All()
	if r.ModelToolProfile() == ModelToolProfileAgenticV2 {
		// Agentic V2 is a fixed provider protocol, not a projection of the
		// current execution allow-list. Keep its three schemas stable while
		// IsToolEnabled and the permission checker independently deny dispatch
		// for tools excluded by --allowed-tools or --disallowed-tools.
		byName := make(map[string]types.Tool, len(all))
		for _, tool := range all {
			if tool != nil {
				byName[tool.Name()] = tool
			}
		}
		visible := make([]types.Tool, 0, len(agenticV2VisibleToolOrder))
		for _, name := range agenticV2VisibleToolOrder {
			if tool := byName[name]; tool != nil {
				visible = append(visible, tool)
			}
		}
		if tool := byName[agenticV2OptionalContextUpdateTool]; tool != nil {
			visible = append(visible, tool)
		}
		return visible
	}
	toolSearchEnabled := r.IsToolEnabled(r.Get("ToolSearch"))

	visible := make([]types.Tool, 0, len(all))
	byName := make(map[string]types.Tool, len(all))
	for _, tool := range all {
		byName[tool.Name()] = tool
	}
	appendVisible := func(tool types.Tool) {
		if !r.IsToolEnabled(tool) {
			return
		}
		if !toolSearchEnabled || !IsDeferredTool(tool) {
			visible = append(visible, tool)
			return
		}
		if _, ok := loaded[tool.Name()]; ok {
			visible = append(visible, tool)
		}
	}

	for _, name := range eagerCoreToolOrder {
		if tool := byName[name]; tool != nil {
			appendVisible(tool)
		}
	}
	for _, tool := range all {
		if _, core := eagerCoreToolNames[tool.Name()]; core {
			continue
		}
		appendVisible(tool)
	}
	return visible
}

// VisibleDefinitions returns the model-visible tool definitions for the
// current loaded-tool set.
func (r *Registry) VisibleDefinitions(loaded map[string]struct{}) []types.ToolDefinition {
	return types.ToDefinitions(r.VisibleTools(loaded))
}

// DeferredTools returns every registry tool that participates in ToolSearch.
// Already-loaded tools remain discoverable so repeated selections are no-ops.
func (r *Registry) DeferredTools() []types.Tool {
	all := r.All()
	deferred := make([]types.Tool, 0, len(all))
	for _, tool := range all {
		if r.IsToolEnabled(tool) && IsDeferredTool(tool) {
			deferred = append(deferred, tool)
		}
	}
	return deferred
}

// CheckToolPermissions is the single tool-specific pre-execution path used by
// the loop and direct registry dispatch.
func (r *Registry) CheckToolPermissions(ctx context.Context, name string, input map[string]any, request types.ToolPermissionRequest) (permissionResult types.ToolPermissionResult, permissionErr error) {
	tool, canonicalTool, toolGeneration := r.getWithGeneration(name)
	if tool == nil {
		// ExecuteToolWithError owns the canonical unknown-tool result and
		// runtime-enabled names list.
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorPassthrough}, nil
	}
	request.Runtime = r.RuntimeContext()
	metadata := r.ToolMetadata(tool.Name(), input)
	defer func() {
		snapshot := clonePermissionReviewRuntime(request.Runtime)
		permissionResult.RuntimeSnapshot = &snapshot
		permissionResult.ToolMetadata = metadata
	}()
	if !r.IsToolEnabled(tool) {
		return types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny,
			Message:  i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolDisabled, tool.Name()),
		}, nil
	}
	if runtimeOwner := strings.TrimSpace(request.Runtime.SessionID); runtimeOwner != "" && strings.TrimSpace(request.SessionID) == "" {
		request.SessionID = runtimeOwner
	}
	binding := permissionBindingForRequest(request)
	planDenied := false
	if strings.EqualFold(strings.TrimSpace(request.Runtime.PermissionMode), "plan") && tool.Name() != "ExitPlanMode" {
		planDenied = metadata.Write || metadata.Destructive
	}
	_, blanketDenied := matchingBlanketRule(tool, request.Runtime.DeniedRules)
	_, blanketAsk := matchingBlanketRule(tool, request.Runtime.AskRules)
	_, blanketAllowed := matchingBlanketRule(tool, request.Runtime.AllowedRules)
	checker, ok := tool.(types.ToolPermissionChecker)
	if !ok {
		if planDenied || blanketDenied {
			message := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolRuleDenied, tool.Name())
			if planDenied {
				message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPlanDenied, tool.Name())
			}
			return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: message, Required: true}, nil
		}
		if blanketAsk {
			return r.withPermissionGrantAtGeneration(types.ToolPermissionResult{
				Behavior: types.PermissionBehaviorAsk,
				Message:  i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tool.Name()),
				Required: true,
			}, tool.Name(), canonicalTool, toolGeneration, input, binding), nil
		}
		if blanketAllowed {
			return r.withPermissionGrantAtGeneration(types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, tool.Name(), canonicalTool, toolGeneration, input, binding), nil
		}
		return r.withPermissionGrantAtGeneration(types.ToolPermissionResult{Behavior: types.PermissionBehaviorPassthrough}, tool.Name(), canonicalTool, toolGeneration, input, binding), nil
	}
	result, err := checker.CheckPermissions(ctx, input, request)
	if err != nil {
		return types.ToolPermissionResult{}, err
	}
	// The checker must run before blanket Ask/Allow so its hard Block and
	// content Deny cannot be hidden by a broader rule. Merge the resulting
	// authorities with Block > Deny > Ask > Allow precedence.
	if result.PolicyDecision != nil && result.PolicyDecision.Disposition == types.PolicyBlock {
		return result, nil
	}
	if planDenied || blanketDenied {
		message := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolRuleDenied, tool.Name())
		if planDenied {
			message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPlanDenied, tool.Name())
		}
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: message, Required: true}, nil
	}
	if result.Behavior == types.PermissionBehaviorDeny {
		return result, nil
	}
	if blanketAsk && result.Behavior != types.PermissionBehaviorAsk {
		result.Behavior = types.PermissionBehaviorAsk
		result.Message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tool.Name())
		result.Required = true
	}
	if result.Behavior == types.PermissionBehaviorPassthrough && blanketAllowed {
		result.Behavior = types.PermissionBehaviorAllow
		if result.UpdatedInput == nil {
			result.UpdatedInput = input
		}
	}
	switch result.Behavior {
	case types.PermissionBehaviorAllow, types.PermissionBehaviorDeny, types.PermissionBehaviorAsk, types.PermissionBehaviorPassthrough:
	default:
		result.Behavior = types.PermissionBehaviorPassthrough
	}
	return r.withPermissionGrantAtGeneration(result, tool.Name(), canonicalTool, toolGeneration, input, binding), nil
}

func clonePermissionReviewRuntime(source types.ToolRuntimeContext) types.ToolRuntimeContext {
	cloned := source
	cloned.AllowedDirs = append([]string(nil), source.AllowedDirs...)
	cloned.Features = clonePermissionReviewMap(source.Features)
	cloned.AllowedTools = clonePermissionReviewMap(source.AllowedTools)
	cloned.DeniedTools = clonePermissionReviewMap(source.DeniedTools)
	cloned.AllowedRules = append([]types.PermissionRuleValue(nil), source.AllowedRules...)
	cloned.DeniedRules = append([]types.PermissionRuleValue(nil), source.DeniedRules...)
	cloned.AskRules = append([]types.PermissionRuleValue(nil), source.AskRules...)
	return cloned
}

func clonePermissionReviewMap(source map[string]bool) map[string]bool {
	if source == nil {
		return nil
	}
	cloned := make(map[string]bool, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (r *Registry) withPermissionGrantAtGeneration(result types.ToolPermissionResult, toolName, canonical string, generation uint64, input map[string]any, binding types.ToolPermissionBinding) types.ToolPermissionResult {
	if result.Behavior == types.PermissionBehaviorDeny || toolName == "" {
		return result
	}
	// The executable authority is part of the one-time permission handoff, not
	// an advisory field on the UI request. This prevents a handler backed by one
	// sandbox from authorizing execution through another backend instance.
	binding.SandboxCapability = result.SandboxCapability
	if result.PolicyDecision != nil {
		binding.PolicyRisk = result.PolicyDecision.Risk
	} else {
		binding.PolicyRisk = types.PolicyRiskNone
	}
	effectiveInput := input
	if result.UpdatedInput != nil {
		effectiveInput = result.UpdatedInput
	}
	policyCode := result.ExecutionPolicyCode
	if result.PolicyDecision != nil {
		if policyCode == "" {
			policyCode = result.PolicyDecision.Code
		}
	}
	result.PermissionGrant = r.issuePermissionGrantAtGeneration(toolName, canonical, generation, effectiveInput, binding, policyCode, false)
	result.PermissionBinding = binding
	return result
}

// ToolMetadata returns the classification declared by the registered tool.
// Built-in and dynamic tools own this metadata so scheduling and permission
// behavior cannot silently depend on a registry-side name table.
func (r *Registry) ToolMetadata(name string, input map[string]any) types.ToolMetadata {
	tool := r.Get(name)
	if tool == nil {
		return types.ToolMetadata{}
	}
	if provider, ok := tool.(types.ToolMetadataProvider); ok {
		return provider.ToolMetadata(input)
	}
	return types.ToolMetadata{}
}
