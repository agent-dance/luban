package tools

import (
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

func checkFileWriteToolPermission(t *FileWriteTool, input map[string]any, request types.ToolPermissionRequest) types.ToolPermissionResult {
	rawPath, _ := input["file_path"].(string)
	baseDir := strings.TrimSpace(request.Runtime.ProjectRoot)
	if baseDir == "" {
		baseDir = t.writeBaseDir()
	}
	path, err := expandReadPath(rawPath, baseDir)
	if err != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolPermissionText(i18n.KeyToolPermissionInvalidPath), Required: true}
	}
	updated := cloneToolInput(input)
	updated["file_path"] = path
	paths := fileWritePermissionPaths(path)

	if rule, ok := matchingFileWriteRule(paths, request.Runtime.DeniedRules); ok {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorDeny,
			Message:     toolPermissionFormat(i18n.KeyToolPermissionWriteDenied, path),
			BlockedPath: rule.RuleContent, Required: true,
		}
	}

	// A session-scoped rule for one .claude/skills/<name> subtree is the one
	// deliberate exception to protected-path prompting in TS. Source-aware
	// RuntimeScope wiring supplies these rules via SessionAllowedRules.
	if rule, ok := matchingSessionClaudeSkillRule(path, request.Runtime); ok {
		_ = rule
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: updated}
	}

	if permissions.IsProtectedPath(path) {
		decision := fileWriteAskDecision(path, updated, request.Runtime)
		decision.Message = toolPermissionFormat(i18n.KeyToolPermissionWriteProtected, path)
		decision.Required = true
		if pattern := claudeSkillPermissionPattern(path); pattern != "" {
			decision.Suggestions = []types.PermissionUpdate{{
				Type: types.PermissionUpdateAddRules, Destination: types.PermissionDestinationSession,
				Behavior: types.PermissionBehaviorAllow,
				Rules:    []types.PermissionRuleValue{{ToolName: "Edit", RuleContent: pattern}},
			}}
		}
		return decision
	}

	if rule, ok := matchingFileWriteRule(paths, request.Runtime.AskRules); ok {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorAsk,
			Message:     toolPermissionFormat(i18n.KeyToolPermissionWritePending, path),
			BlockedPath: rule.RuleContent, Required: true, UpdatedInput: updated,
		}
	}

	inWorkingDir := fileWritePathInWorkingDir(path, request.Runtime, t.allowedDirs())
	if strings.EqualFold(strings.TrimSpace(request.Runtime.PermissionMode), "acceptEdits") && inWorkingDir {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: updated}
	}
	if _, ok := matchingFileWriteRule([]string{path}, request.Runtime.AllowedRules); ok {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: updated}
	}
	if inWorkingDir {
		// i18n:allow display-literal identifier -- Write is the canonical tool ID and must remain unchanged.
		return types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorPassthrough,
			Message:  toolPermissionFormat(i18n.KeyToolPermissionModifyRequired, "Write", path),
			Suggestions: []types.PermissionUpdate{{
				Type: types.PermissionUpdateAddRules, Destination: types.PermissionDestinationLocalSettings,
				Behavior: types.PermissionBehaviorAllow,
				Rules:    []types.PermissionRuleValue{{ToolName: "Edit", RuleContent: path}},
			}},
			UpdatedInput: updated,
		}
	}
	return fileWriteAskDecision(path, updated, request.Runtime)
}

func fileWritePermissionPaths(path string) []string {
	paths := []string{path}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && canonicalPathForComparison(resolved) != canonicalPathForComparison(path) {
		paths = append(paths, resolved)
	}
	return paths
}

func isFileEditPermissionTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "edit", "fileedit", "write", "filewrite":
		return true
	default:
		return false
	}
}

func matchingFileWriteRule(paths []string, rules []types.PermissionRuleValue) (types.PermissionRuleValue, bool) {
	for _, rule := range rules {
		if !isFileEditPermissionTool(rule.ToolName) || strings.TrimSpace(rule.RuleContent) == "" {
			continue
		}
		pattern := filepath.Clean(expandPermissionRuleTilde(strings.TrimSpace(rule.RuleContent)))
		for _, path := range paths {
			if fileWriteRulePathMatches(pattern, path) {
				return rule, true
			}
		}
	}
	return types.PermissionRuleValue{}, false
}

func fileWriteRulePathMatches(pattern, path string) bool {
	if canonicalPathForComparison(pattern) == canonicalPathForComparison(path) {
		return true
	}
	patternSlash := filepath.ToSlash(pattern)
	pathSlash := filepath.ToSlash(path)
	if matched, err := MatchGlob(patternSlash, pathSlash); err == nil && matched {
		return true
	}
	if strings.HasSuffix(patternSlash, "/**") {
		root := strings.TrimSuffix(patternSlash, "/**")
		return pathSlash == root || strings.HasPrefix(pathSlash, root+"/")
	}
	return false
}

func expandPermissionRuleTilde(pattern string) string {
	if pattern == "~" || strings.HasPrefix(pattern, "~/") {
		if home, err := filepath.Abs(pattern); err == nil {
			return home
		}
	}
	return pattern
}

func fileWritePathInWorkingDir(path string, runtime types.ToolRuntimeContext, fallback []string) bool {
	dirs := runtime.AllowedDirs
	if dirs == nil {
		dirs = fallback
	}
	if len(dirs) == 0 && strings.TrimSpace(runtime.ProjectRoot) != "" {
		dirs = []string{runtime.ProjectRoot}
	}
	return len(dirs) == 0 || checkAllowedPath(path, dirs) == nil
}

func fileWriteAskDecision(path string, updated map[string]any, runtime types.ToolRuntimeContext) types.ToolPermissionResult {
	directory := filepath.Dir(path)
	suggestions := []types.PermissionUpdate{{
		Type: types.PermissionUpdateAddRules, Destination: types.PermissionDestinationLocalSettings,
		Behavior: types.PermissionBehaviorAllow,
		Rules:    []types.PermissionRuleValue{{ToolName: "Edit", RuleContent: path}},
	}}
	if !fileWritePathInWorkingDir(path, runtime, nil) {
		suggestions = append([]types.PermissionUpdate{{
			Type: types.PermissionUpdateAddDirectories, Destination: types.PermissionDestinationLocalSettings,
			Directories: []string{directory},
		}}, suggestions...)
	}
	return types.ToolPermissionResult{
		Behavior:    types.PermissionBehaviorAsk,
		Message:     toolPermissionFormat(i18n.KeyToolPermissionWritePending, path),
		Suggestions: suggestions, UpdatedInput: updated,
	}
}

func claudeSkillPermissionPattern(path string) string {
	slash := filepath.ToSlash(path)
	marker := "/.claude/skills/"
	idx := strings.Index(slash, marker)
	if idx < 0 {
		return ""
	}
	rest := slash[idx+len(marker):]
	name := strings.SplitN(rest, "/", 2)[0]
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return slash[:idx] + marker + name + "/**"
}

// matchingSessionClaudeSkillRule is completed by the source-aware permission
// rule surface. Until then, no persistent rule is accidentally treated as a
// session safety bypass.
func matchingSessionClaudeSkillRule(_ string, _ types.ToolRuntimeContext) (types.PermissionRuleValue, bool) {
	return types.PermissionRuleValue{}, false
}
