// Package search restricts Glob/Grep search roots and
// emitted matches to the allowed directories in the invocation's runtime
// scope. Zero-value tools remain unscoped for direct use, while registry tools
// carry an immutable per-session RuntimeScope snapshot.
package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

type searchRuntimeSnapshot struct {
	cwd         string
	allowedDirs []string
	ignores     fileReadIgnoreConfig
}

func searchRuntimeSnapshotFor(provider types.ToolRuntimeContextProvider, pluginCache *orphanedPluginCache) (searchRuntimeSnapshot, error) {
	if provider == nil {
		cwd, err := os.Getwd()
		if err != nil {
			return searchRuntimeSnapshot{}, err
		}
		return searchRuntimeSnapshot{
			cwd:         filepath.Clean(cwd),
			allowedDirs: nil,
		}, nil
	}

	runtime := provider.ToolRuntimeContext()
	cwd := strings.TrimSpace(runtime.ProjectRoot)
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return searchRuntimeSnapshot{}, err
		}
	}
	if !filepath.IsAbs(cwd) {
		abs, err := filepath.Abs(cwd)
		if err != nil {
			return searchRuntimeSnapshot{}, err
		}
		cwd = abs
	}
	cacheRoot, pluginDirs := pluginCache.snapshot()
	return searchRuntimeSnapshot{
		cwd:         filepath.Clean(cwd),
		allowedDirs: append([]string(nil), runtime.AllowedDirs...),
		ignores:     runtimeFileReadIgnoreConfig(runtime, cacheRoot, pluginDirs),
	}, nil
}

func (t *globTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, Search: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *grepTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, Search: true, ConcurrencySafe: true, MaxResultSizeChars: 20_000}
}

func (t *globTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return checkSearchPermission("Glob", input, request.Runtime), nil
}

func (t *grepTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return checkSearchPermission("Grep", input, request.Runtime), nil
}

func checkSearchPermission(toolName string, input map[string]any, runtime types.ToolRuntimeContext) types.ToolPermissionResult {
	pattern, _ := input["pattern"].(string)
	if rule, ok := matchingSearchToolRule(toolName, pattern, runtime.DeniedRules); ok {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorDeny,
			Message:     i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPermissionSearchDenied, toolName, pattern),
			BlockedPath: rule.RuleContent,
			Required:    true,
		}
	}
	if rule, ok := matchingSearchToolRule(toolName, pattern, runtime.AskRules); ok {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorAsk,
			Message:     i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPermissionSearchRequired, toolName, pattern),
			BlockedPath: rule.RuleContent,
			Required:    true,
		}
	}

	path := searchPathFromInput(input)
	if path == "" {
		path = runtime.ProjectRoot
	}
	if path == "" {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}
	}
	if isUNCPath(path) {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorAsk,
			Message:     i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPermissionReadUNC, path),
			BlockedPath: path,
			Required:    true,
		}
	}
	if !filepath.IsAbs(path) && runtime.ProjectRoot != "" {
		path = filepath.Join(runtime.ProjectRoot, path)
	}
	allowed := runtime.AllowedDirs
	if !searchPathAllowed(path, allowed) {
		return outsideSearchDirectoryPermission(path, toolName)
	}
	if filepath.IsAbs(pattern) {
		baseDir, _ := extractGlobBaseDirectory(pattern)
		if baseDir != "" && !searchPathAllowed(baseDir, allowed) {
			return outsideSearchDirectoryPermission(baseDir, toolName)
		}
	}
	return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}
}

func matchingSearchToolRule(toolName string, pattern string, rules []types.PermissionRuleValue) (types.PermissionRuleValue, bool) {
	for _, rule := range rules {
		if !strings.EqualFold(strings.TrimSpace(rule.ToolName), toolName) {
			continue
		}
		rulePattern := strings.TrimSpace(rule.RuleContent)
		if rulePattern == "" {
			continue
		}
		if rulePattern == pattern {
			return rule, true
		}
		if matched, err := toolbase.MatchGlob(rulePattern, pattern); err == nil && matched {
			return rule, true
		}
	}
	return types.PermissionRuleValue{}, false
}

func ensureSearchRootAllowed(rawRoot string, baseCWD string, allowedDirs []string) (string, error) {
	abs, err := absolutePathFromBase(rawRoot, baseCWD)
	if err != nil {
		return "", err
	}
	resolved := abs
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		resolved = filepath.Clean(real)
	}
	if !toolbase.PathWithinAllowedDirs(abs, allowedDirs) || !toolbase.PathWithinAllowedDirs(resolved, allowedDirs) {
		return "", i18n.NewError(i18n.KeyToolSourceSinkSearchOutsideAllowed, rawRoot)
	}
	return abs, nil
}

func absolutePathFromBase(rawPath string, baseCWD string) (string, error) {
	if filepath.IsAbs(rawPath) {
		return filepath.Clean(rawPath), nil
	}
	baseCWD = strings.TrimSpace(baseCWD)
	if baseCWD == "" {
		return filepath.Abs(rawPath)
	}
	if !filepath.IsAbs(baseCWD) {
		abs, err := filepath.Abs(baseCWD)
		if err != nil {
			return "", err
		}
		baseCWD = abs
	}
	return filepath.Clean(filepath.Join(baseCWD, rawPath)), nil
}

func searchPathFromInput(input map[string]any) string {
	if input == nil {
		return ""
	}
	path, _ := input["path"].(string)
	return strings.TrimSpace(path)
}

func searchPathAllowed(path string, allowedDirs []string) bool {
	if len(allowedDirs) == 0 {
		return true
	}
	if !toolbase.PathWithinAllowedDirs(path, allowedDirs) {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return toolbase.PathWithinAllowedDirs(resolved, allowedDirs)
	}
	return true
}

func outsideSearchDirectoryPermission(path, toolName string) types.ToolPermissionResult {
	absPath := path
	if resolved, err := filepath.Abs(path); err == nil {
		absPath = filepath.Clean(resolved)
	}
	directory := absPath
	if filepath.Ext(absPath) != "" {
		directory = filepath.Dir(absPath)
	}
	return types.ToolPermissionResult{
		Behavior:    types.PermissionBehaviorAsk,
		Message:     i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPermissionOutsideDirectories, absPath),
		BlockedPath: absPath,
		Required:    true,
		Suggestions: []types.PermissionUpdate{{
			Type:        types.PermissionUpdateAddDirectories,
			Destination: types.PermissionDestinationLocalSettings,
			Directories: []string{directory},
		}, {
			Type:        types.PermissionUpdateAddRules,
			Destination: types.PermissionDestinationLocalSettings,
			Behavior:    types.PermissionBehaviorAllow,
			Rules:       []types.PermissionRuleValue{{ToolName: toolName, RuleContent: absPath}},
		}},
	}
}
