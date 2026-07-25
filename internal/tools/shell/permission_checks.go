package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/observability"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
	"mvdan.cc/sh/v3/syntax"
)

var processSubstitutionApprovalPattern = regexp.MustCompile(`(?s)(?:>\s*\(|<\s*\()`)

var gitInternalPathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^HEAD$`),
	regexp.MustCompile(`^objects(?:/|$)`),
	regexp.MustCompile(`^refs(?:/|$)`),
	regexp.MustCompile(`^hooks(?:/|$)`),
}

type bashPermissionFacts struct {
	cdCount               int
	hasCd                 bool
	hasGit                bool
	hasOutputRedirect     bool
	writesGitInternalPath bool
}

func (t *BashTool) ToolMetadata(input map[string]any) types.ToolMetadata {
	command, _ := input["command"].(string)
	semantics := ClassifyCommand(command)
	readOnly := IsReadOnlyCommand(command, semantics)
	metadata := types.ToolMetadata{
		ReadOnly:           readOnly,
		Write:              !readOnly,
		Destructive:        semantics == SemanticDestructive,
		ConcurrencySafe:    readOnly,
		MaxResultSizeChars: 30_000,
	}
	name, _ := firstCallToken(command)
	switch name {
	case "find", "grep", "egrep", "fgrep", "rg", "ack":
		metadata.Search = readOnly
	}
	return metadata
}

// CheckPermissions moves Bash rule matching, read-only auto-allow, and
// mandatory approval checks ahead of Execute so every dispatcher observes the
// same decision and approved asks do not get blocked a second time.
func (t *BashTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	localScope := t.executionScopeSnapshot()
	command, _ := input["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		allowPolicy := allowShellPolicyDecision()
		return types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorPassthrough, ExecutionPolicyCode: localScope.executionPolicyCode(allowPolicy.ExecutionBindingCode()),
			Sandboxed:         localScope.sandboxAvailable && localScope.sandboxName != "none",
			SandboxCapability: localScope.sandboxCapability,
		}, nil
	}
	policyContext := localScope.shellPolicyContext(request.Runtime, !request.AvoidPrompts)
	policy, _ := analyzeBashCommandWithSedEvidencePolicy(command, policyContext)
	observability.RecordShellPolicy(string(policy.Disposition), policy.Code)
	executionPolicyCode := localScope.executionPolicyCode(policy.ExecutionBindingCode())
	withExecutionPolicy := func(result types.ToolPermissionResult) types.ToolPermissionResult {
		result.ExecutionPolicyCode = executionPolicyCode
		result.Sandboxed = localScope.sandboxAvailable && localScope.sandboxName != "none"
		result.SandboxCapability = localScope.sandboxCapability
		return result
	}
	switch policy.Disposition {
	case types.PolicyBlock:
		policyCopy := policy.Clone()
		return withExecutionPolicy(types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny, Message: toolPermissionFormat(policy.PublicKey, policy.PublicArgs...),
			Required: true, PolicyDecision: &policyCopy,
		}), nil
	}

	permissionRules := append([]permissions.Rule(nil), localScope.permissionRules...)
	permissionRules = append(permissionRules, bashRuntimePermissionRules(request.Runtime)...)
	var ruleDecision permissions.Decision
	var matchedRule *permissions.Rule
	rulePartial := false
	if len(permissionRules) > 0 {
		ruleDecision, matchedRule, rulePartial = matchBashRuleDetailed(command, permissionRules)
	}

	// Merge all content-specific authorities before returning. The precedence is
	// intentionally monotonic: hard Block > explicit/plan Deny > RequiredAsk or
	// rule Ask > Allow. In particular, a dynamic-command RequiredAsk must never
	// hide an operator's explicit Deny rule.
	if localScope.planState != nil && localScope.planState.IsActive() {
		return withExecutionPolicy(types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny,
			Message:  toolPermissionText(i18n.KeyToolPermissionBashPlanMode),
			Required: true,
		}), nil
	}
	if matchedRule != nil && ruleDecision == permissions.DecisionDeny {
		return withExecutionPolicy(types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny,
			Message:  toolPermissionText(i18n.KeyToolPermissionBashRuleDenied),
			Required: true,
		}), nil
	}
	if policy.Disposition == types.PolicyRequiredAsk {
		result := bashAskDecision(command, toolPermissionFormat(policy.PublicKey, policy.PublicArgs...), true)
		policyCopy := policy.Clone()
		result.PolicyDecision = &policyCopy
		return withExecutionPolicy(result), nil
	}
	if rulePartial || matchedRule != nil && ruleDecision == permissions.DecisionAsk {
		return withExecutionPolicy(bashAskDecision(command, toolPermissionText(i18n.KeyToolPermissionBashRuleApproval), true)), nil
	}
	if matchedRule != nil {
		switch ruleDecision {
		case permissions.DecisionAllow, permissions.DecisionAllowOnce:
			return withExecutionPolicy(types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}), nil
		}
	}

	semantics := ClassifyCommand(command)
	// TS ExitPlanMode approval converts allowedPrompts into session-scoped Bash
	// rules. Keep mandatory/destructive checks above this branch so a semantic
	// approval cannot bypass the safety floor.
	if localScope.planState != nil && localScope.planState.AllowedPromptMatches("Bash", command) {
		return withExecutionPolicy(types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}), nil
	}
	if IsReadOnlyCommand(command, semantics) {
		return withExecutionPolicy(types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}), nil
	}
	decision := bashAskDecision(command, toolPermissionText(i18n.KeyToolPermissionBashGenericApproval), false)
	decision.Behavior = types.PermissionBehaviorPassthrough
	return withExecutionPolicy(decision), nil
}

func (scope bashExecutionScope) executionPolicyCode(policyCode string) string {
	localAuthority := struct {
		Policy            types.PolicyContext `json:"policy"`
		PermissionRules   []permissions.Rule  `json:"permissionRules,omitempty"`
		SandboxCapability string              `json:"sandboxCapability,omitempty"`
	}{
		Policy:            scope.shellPolicyContext(types.ToolRuntimeContext{}, false),
		PermissionRules:   append([]permissions.Rule(nil), scope.permissionRules...),
		SandboxCapability: scope.sandboxCapability,
	}
	encoded, err := json.Marshal(localAuthority)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return policyCode + "\x1f" + hex.EncodeToString(digest[:])
}

func bashRuntimePermissionRules(runtime types.ToolRuntimeContext) []permissions.Rule {
	var rules []permissions.Rule
	appendRules := func(values []types.PermissionRuleValue, decision permissions.Decision) {
		for _, value := range values {
			if !strings.EqualFold(strings.TrimSpace(value.ToolName), "Bash") || strings.TrimSpace(value.RuleContent) == "" {
				continue
			}
			rules = append(rules, permissions.Rule{
				Tool: "Bash", Pattern: strings.TrimSpace(value.RuleContent), Decision: decision,
			})
		}
	}
	appendRules(runtime.AllowedRules, permissions.DecisionAllow)
	appendRules(runtime.AskRules, permissions.DecisionAsk)
	appendRules(runtime.DeniedRules, permissions.DecisionDeny)
	return rules
}

func bashAskDecision(command, message string, required bool) types.ToolPermissionResult {
	name, _ := firstCallToken(command)
	if name == "" {
		name = command
	}
	result := types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorAsk,
		Message:  message,
		Required: required,
	}
	if !required {
		result.Suggestions = []types.PermissionUpdate{{
			Type:        types.PermissionUpdateAddRules,
			Destination: types.PermissionDestinationLocalSettings,
			Behavior:    types.PermissionBehaviorAllow,
			Rules: []types.PermissionRuleValue{{
				ToolName:    "Bash",
				RuleContent: name + " *",
			}},
		}}
	}
	return result
}

// MapToolPermissionRejection preserves the typed policy code and remediation
// when a RequiredAsk cannot be displayed or is rejected. The public message is
// localized at this final boundary; the structured data contains no command,
// environment snapshot, parser error, or other private diagnostic state.
func (t *BashTool) MapToolPermissionRejection(input map[string]any, toolUseID, message string) types.ToolResultBlock {
	command, _ := input["command"].(string)
	decision, _ := analyzeBashCommandWithSedEvidencePolicy(command, t.shellPolicyContext(types.ToolRuntimeContext{}, false))
	if decision.IsRequiredAsk() {
		observability.RecordShellPolicy("deny", decision.Code)
	}
	if strings.TrimSpace(message) == "" && decision.PublicKey != "" {
		message = toolPermissionFormat(decision.PublicKey, decision.PublicArgs...)
	}
	if decision.Remediation != nil && decision.Remediation.PublicKey != "" {
		message = strings.TrimSpace(message) + "\n" + toolPermissionFormat(decision.Remediation.PublicKey, decision.Remediation.PublicArgs...)
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   strings.TrimSpace(message),
		Data:      decision,
		IsError:   true,
		Outcome:   types.ToolOutcomeDenied,
	}
}

func (t *BashTool) shellPolicyContext(runtime types.ToolRuntimeContext, interactive bool) types.PolicyContext {
	return t.executionScopeSnapshot().shellPolicyContext(runtime, interactive)
}

func (scope bashExecutionScope) shellPolicyContext(runtime types.ToolRuntimeContext, interactive bool) types.PolicyContext {
	policy := DefaultShellPolicyContext()
	if strings.TrimSpace(scope.cwd) != "" {
		policy.CWD = scope.cwd
	}
	if len(scope.allowedDirs) > 0 {
		policy.AllowedDirs = append([]string(nil), scope.allowedDirs...)
	}
	policy.Sandboxed = scope.sandboxAvailable && scope.sandboxName != "none"
	if len(runtime.AllowedDirs) > 0 {
		policy.AllowedDirs = append([]string(nil), runtime.AllowedDirs...)
	}
	policy.Interactive = interactive || runtime.Interactive
	return policy
}

func bashPermissionApprovalReason(command string) (bool, i18n.Key) {
	if strings.TrimSpace(command) == "" {
		return false, ""
	}
	if processSubstitutionApprovalPattern.MatchString(command) {
		return true, i18n.KeyToolPermissionBashProcessSubstitution
	}

	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(command), "")
	if err != nil {
		return false, ""
	}

	facts := collectBashPermissionFacts(prog)
	switch {
	case facts.cdCount > 1:
		return true, i18n.KeyToolPermissionBashMultipleDirectories
	case facts.hasCd && facts.hasGit:
		return true, i18n.KeyToolPermissionBashCDAndGit
	case facts.hasCd && facts.hasOutputRedirect:
		return true, i18n.KeyToolPermissionBashCDAndRedirect
	case facts.hasGit && isCurrentDirectoryBareGitRepoLike():
		return true, i18n.KeyToolPermissionBashBareGit
	case facts.hasGit && facts.writesGitInternalPath:
		return true, i18n.KeyToolPermissionBashGitInternal
	default:
		return false, ""
	}
}

func collectBashPermissionFacts(prog *syntax.File) bashPermissionFacts {
	var facts bashPermissionFacts

	syntax.Walk(prog, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			name := cmdName(n)
			switch name {
			case "cd":
				facts.cdCount++
				facts.hasCd = true
			case "git":
				facts.hasGit = true
			}
			for _, target := range gitInternalWriteTargets(name, argLiterals(n)) {
				if isGitInternalPath(target) {
					facts.writesGitInternalPath = true
					break
				}
			}
		case *syntax.Stmt:
			for _, redir := range n.Redirs {
				if redir.Op != syntax.RdrOut && redir.Op != syntax.AppOut && redir.Op != syntax.RdrAll {
					continue
				}
				facts.hasOutputRedirect = true
				if isGitInternalPath(wordToString(redir.Word)) {
					facts.writesGitInternalPath = true
				}
			}
		}
		return true
	})

	return facts
}

func gitInternalWriteTargets(name string, args []string) []string {
	switch name {
	case "mkdir", "touch":
		return allNonFlagArgs(args)
	case "cp", "mv", "install", "ln":
		if dest := lastNonFlagArg(args); dest != "" {
			return []string{dest}
		}
	case "tee":
		return allNonFlagArgs(args)
	case "dd":
		var targets []string
		for _, arg := range args {
			if strings.HasPrefix(arg, "of=") {
				targets = append(targets, strings.TrimPrefix(arg, "of="))
			}
		}
		return targets
	case "truncate":
		return truncateTargets(args)
	}
	return nil
}

func truncateTargets(args []string) []string {
	var targets []string
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "-s" || arg == "--size" {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "-s") || strings.HasPrefix(arg, "--size=") || strings.HasPrefix(arg, "-") {
			continue
		}
		targets = append(targets, arg)
	}
	return targets
}

func isGitInternalPath(path string) bool {
	if path == "" {
		return false
	}
	normalized := filepath.ToSlash(filepath.Clean(path))
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	for _, pattern := range gitInternalPathPatterns {
		if pattern.MatchString(normalized) {
			return true
		}
	}
	return false
}

func isCurrentDirectoryBareGitRepoLike() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	gitDir := filepath.Join(cwd, ".git")
	if info, err := os.Stat(gitDir); err == nil {
		if info.Mode().IsRegular() {
			return false
		}
		if info.IsDir() {
			gitHead := filepath.Join(gitDir, "HEAD")
			if headInfo, err := os.Stat(gitHead); err == nil && headInfo.Mode().IsRegular() {
				return false
			}
		}
	}

	if info, err := os.Stat(filepath.Join(cwd, "HEAD")); err == nil && info.Mode().IsRegular() {
		return true
	}
	if info, err := os.Stat(filepath.Join(cwd, "objects")); err == nil && info.IsDir() {
		return true
	}
	if info, err := os.Stat(filepath.Join(cwd, "refs")); err == nil && info.IsDir() {
		return true
	}
	return false
}
