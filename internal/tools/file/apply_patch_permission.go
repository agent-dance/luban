package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

func (t *ApplyPatchTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	if t.PlanState != nil && t.PlanState.IsActive() {
		return types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny,
			Message:  toolPermissionText(i18n.KeyToolApplyPatchPlanMode),
			Required: true,
		}, nil
	}
	in, err := types.DecodeStrictToolInput[ApplyPatchInput](input)
	if err != nil {
		return applyPatchUnparseablePermission(input), nil
	}
	parsed, parseErr := parseApplyPatch(in.Patch)
	if parseErr != nil {
		return applyPatchUnparseablePermission(input), nil
	}
	paths, resolveErr := resolveApplyPatchPermissionPaths(parsed, request.Runtime, t)
	if resolveErr != nil {
		return applyPatchInvalidPermission(resolveErr.Code), nil
	}
	for _, path := range paths {
		if rule, found := matchingApplyPatchRule(path, request.Runtime.DeniedRules); found {
			// i18n:allow display-literal protocol -- denied_rule is a stable policy reason code interpolated into localized copy.
			return types.ToolPermissionResult{
				Behavior:    types.PermissionBehaviorDeny,
				Message:     toolPermissionFormat(i18n.KeyToolApplyPatchPermissionDenied, path, "denied_rule"),
				BlockedPath: rule.RuleContent, Required: true,
			}, nil
		}
	}
	for _, path := range paths {
		if permissions.IsProtectedPath(path) {
			return applyPatchAskPermission(paths, path, input, true), nil
		}
		if rule, found := matchingApplyPatchRule(path, request.Runtime.AskRules); found {
			decision := applyPatchAskPermission(paths, path, input, true)
			decision.BlockedPath = rule.RuleContent
			return decision, nil
		}
	}
	allowedDirs := request.Runtime.AllowedDirs
	if allowedDirs == nil {
		allowedDirs = t.allowedDirs()
	}
	for _, path := range paths {
		if len(allowedDirs) > 0 && checkAllowedPath(path, allowedDirs) != nil {
			return outsideDirectoryPermission(path, "ApplyPatch"), nil
		}
	}
	if strings.EqualFold(strings.TrimSpace(request.Runtime.PermissionMode), "acceptEdits") {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
	}
	allExplicitlyAllowed := len(paths) > 0
	for _, path := range paths {
		if _, found := matchingApplyPatchRule(path, request.Runtime.AllowedRules); !found {
			allExplicitlyAllowed = false
			break
		}
	}
	if allExplicitlyAllowed {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
	}
	return applyPatchAskPermission(paths, paths[0], input, false), nil
}

// applyPatchUnparseablePermission deliberately allows execution to continue
// only after the same deterministic parser used by Execute has rejected the
// input. Execute will therefore return the localized, structured
// file.apply_patch.parse result before resolving or writing any target. This
// keeps input diagnostics out of the permission-denial channel without
// weakening path-policy checks for any syntactically valid patch.
func applyPatchUnparseablePermission(input map[string]any) types.ToolPermissionResult {
	return types.ToolPermissionResult{
		Behavior:     types.PermissionBehaviorAllow,
		UpdatedInput: input,
	}
}

func resolveApplyPatchPermissionPaths(parsed parsedApplyPatch, runtime types.ToolRuntimeContext, tool *ApplyPatchTool) ([]string, *applyPatchTargetFailure) {
	base := strings.TrimSpace(runtime.ProjectRoot)
	if base == "" {
		base = tool.baseDir()
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return nil, &applyPatchTargetFailure{Code: "invalid_root"}
	}
	resolvedBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return nil, &applyPatchTargetFailure{Code: "invalid_root"}
	}
	info, err := os.Stat(resolvedBase)
	if err != nil || !info.IsDir() {
		return nil, &applyPatchTargetFailure{Code: "invalid_root"}
	}
	paths := make([]string, 0, len(parsed.Files))
	for _, file := range parsed.Files {
		path := filepath.Clean(filepath.Join(resolvedBase, filepath.FromSlash(file.Path)))
		if symlinkErr := rejectApplyPatchSymlinkPath(resolvedBase, path); symlinkErr != nil {
			return nil, symlinkErr
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func applyPatchInvalidPermission(reason string) types.ToolPermissionResult {
	return types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorDeny,
		Message:  toolPermissionFormat(i18n.KeyToolApplyPatchPermissionInvalid, reason),
		Required: true,
	}
}

func applyPatchAskPermission(paths []string, blockedPath string, input map[string]any, required bool) types.ToolPermissionResult {
	behavior := types.PermissionBehaviorPassthrough
	if required {
		behavior = types.PermissionBehaviorAsk
	}
	rules := make([]types.PermissionRuleValue, 0, len(paths))
	for _, path := range paths {
		rules = append(rules, types.PermissionRuleValue{ToolName: "ApplyPatch", RuleContent: path})
	}
	return types.ToolPermissionResult{
		Behavior:    behavior,
		Message:     toolPermissionFormat(i18n.KeyToolApplyPatchPermissionPrompt, len(paths), blockedPath),
		BlockedPath: blockedPath,
		Required:    required,
		Suggestions: []types.PermissionUpdate{{
			Type: types.PermissionUpdateAddRules, Destination: types.PermissionDestinationLocalSettings,
			Behavior: types.PermissionBehaviorAllow, Rules: rules,
		}},
		UpdatedInput: input,
	}
}

func matchingApplyPatchRule(path string, rules []types.PermissionRuleValue) (types.PermissionRuleValue, bool) {
	for _, rule := range rules {
		if strings.TrimSpace(rule.ToolName) != "ApplyPatch" || strings.TrimSpace(rule.RuleContent) == "" {
			continue
		}
		pattern := filepath.Clean(expandPermissionRuleTilde(strings.TrimSpace(rule.RuleContent)))
		if fileWriteRulePathMatches(pattern, path) {
			return rule, true
		}
	}
	return types.PermissionRuleValue{}, false
}
