// Package tools — glob_path_scope.go restricts Glob/Grep search roots and
// emitted matches to the configured allowed_dirs (mirroring the TS
// AllowedDirsManager behaviour). When no allow-list is configured the helpers
// degrade to no-ops so single-tenant CLI usage still works.
//
// Package-level state is retained for zero-value tools and direct legacy
// callers. Production registry tools use RuntimeScope snapshots instead so
// separate sessions cannot overwrite one another's search boundary.
package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

var (
	allowedSearchDirsMu sync.RWMutex
	allowedSearchDirs   []string
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
			allowedDirs: AllowedSearchDirs(),
			ignores:     legacyFileReadIgnoreConfig(),
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

func (t *GlobTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, Search: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *GrepTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, Search: true, ConcurrencySafe: true, MaxResultSizeChars: 20_000}
}

func (t *GlobTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return checkSearchPermission("Glob", input, request.Runtime, t.runtime != nil), nil
}

func (t *GrepTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return checkSearchPermission("Grep", input, request.Runtime, t.runtime != nil), nil
}

func (t *GlobTool) ToAutoClassifierInput(input map[string]any) string {
	pattern, _ := input["pattern"].(string)
	return strings.TrimSpace(pattern)
}

func (t *GrepTool) ToAutoClassifierInput(input map[string]any) string {
	pattern, _ := input["pattern"].(string)
	searchPath, _ := input["path"].(string)
	if searchPath != "" {
		return pattern + " in " + searchPath
	}
	return pattern
}

func (t *GlobTool) SearchReadClassification(map[string]any) types.ToolSearchReadClassification {
	return types.ToolSearchReadClassification{IsSearch: true, IsRead: false}
}

func (t *GrepTool) SearchReadClassification(map[string]any) types.ToolSearchReadClassification {
	return types.ToolSearchReadClassification{IsSearch: true, IsRead: false}
}

func checkSearchPermission(toolName string, input map[string]any, runtime types.ToolRuntimeContext, scoped bool) types.ToolPermissionResult {
	pattern, _ := input["pattern"].(string)
	if rule, ok := matchingSearchToolRule(toolName, pattern, runtime.DeniedRules); ok {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorDeny,
			Message:     toolPermissionFormat(i18n.KeyToolPermissionSearchDenied, toolName, pattern),
			BlockedPath: rule.RuleContent,
			Required:    true,
		}
	}
	if rule, ok := matchingSearchToolRule(toolName, pattern, runtime.AskRules); ok {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorAsk,
			Message:     toolPermissionFormat(i18n.KeyToolPermissionSearchRequired, toolName, pattern),
			BlockedPath: rule.RuleContent,
			Required:    true,
		}
	}

	path := permissionFilePath(input)
	if path == "" {
		path = runtime.ProjectRoot
	}
	if path == "" {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}
	}
	if isUNCPath(path) {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorAsk,
			Message:     toolPermissionFormat(i18n.KeyToolPermissionReadUNC, path),
			BlockedPath: path,
			Required:    true,
		}
	}
	if !filepath.IsAbs(path) && runtime.ProjectRoot != "" {
		path = filepath.Join(runtime.ProjectRoot, path)
	}
	allowed := runtime.AllowedDirs
	if !scoped && allowed == nil {
		allowed = AllowedSearchDirs()
	}
	if len(allowed) > 0 && checkAllowedPath(path, allowed) != nil {
		return outsideDirectoryPermission(path, toolName)
	}
	if filepath.IsAbs(pattern) {
		baseDir, _ := extractGlobBaseDirectory(pattern)
		if baseDir != "" && len(allowed) > 0 && checkAllowedPath(baseDir, allowed) != nil {
			return outsideDirectoryPermission(baseDir, toolName)
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
		if matched, err := MatchGlob(rulePattern, pattern); err == nil && matched {
			return rule, true
		}
	}
	return types.PermissionRuleValue{}, false
}

// SetAllowedSearchDirs replaces the active allow-list of absolute directories.
// Pass nil/empty to disable scoping (default for tests/CLI). Each entry is
// canonicalised via filepath.Abs + filepath.Clean. Duplicates and unresolvable
// entries are dropped.
func SetAllowedSearchDirs(dirs []string) {
	cleaned := make([]string, 0, len(dirs))
	seen := make(map[string]struct{}, len(dirs))
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		abs, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		abs = canonicalPathForComparison(abs)
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		cleaned = append(cleaned, abs)
	}
	allowedSearchDirsMu.Lock()
	allowedSearchDirs = cleaned
	allowedSearchDirsMu.Unlock()
}

// AllowedSearchDirs returns a snapshot of the configured allow-list. An empty
// slice means scoping is disabled.
func AllowedSearchDirs() []string {
	allowedSearchDirsMu.RLock()
	defer allowedSearchDirsMu.RUnlock()
	out := make([]string, len(allowedSearchDirs))
	copy(out, allowedSearchDirs)
	return out
}

// IsPathWithinAllowed reports whether absPath falls inside any allowed root.
// When no roots are configured the function returns true (no scoping).
func IsPathWithinAllowed(absPath string) bool {
	allowedSearchDirsMu.RLock()
	dirs := allowedSearchDirs
	allowedSearchDirsMu.RUnlock()
	if len(dirs) == 0 {
		return true
	}
	abs, err := filepath.Abs(absPath)
	if err != nil {
		return false
	}
	abs = canonicalPathForComparison(abs)
	for _, root := range dirs {
		if pathContains(root, abs) {
			return true
		}
	}
	return false
}

// EnsureSearchRootAllowed validates the supplied root against allowed_dirs and
// resolves any symlinks before deciding. Returns the resolved absolute root.
// When the root is outside the allow-list (or its symlink-resolved target is
// outside), an error is returned that callers can surface verbatim.
func EnsureSearchRootAllowed(rawRoot string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return ensureSearchRootAllowed(rawRoot, cwd, AllowedSearchDirs())
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
	if !isPathWithinAllowedDirs(abs, allowedDirs) || !isPathWithinAllowedDirs(resolved, allowedDirs) {
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

func isPathWithinAllowedDirs(candidate string, allowedDirs []string) bool {
	if len(allowedDirs) == 0 {
		return true
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	abs = canonicalPathForComparison(abs)
	for _, root := range allowedDirs {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if !filepath.IsAbs(root) {
			resolved, err := filepath.Abs(root)
			if err != nil {
				continue
			}
			root = resolved
		}
		if pathContains(root, abs) {
			return true
		}
	}
	return false
}

// FilterAllowedPaths returns the subset of paths that fall within the
// allow-list (after symlink resolution). When no roots are configured the
// input is returned unchanged. Errors during EvalSymlinks degrade gracefully:
// the unresolved absolute path is still consulted, so a vanished file isn't
// silently dropped just because its symlink target is unreadable.
func FilterAllowedPaths(paths []string) []string {
	allowedSearchDirsMu.RLock()
	dirs := allowedSearchDirs
	allowedSearchDirsMu.RUnlock()
	if len(dirs) == 0 {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		resolved := abs
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			resolved = filepath.Clean(real)
		}
		if !insideAny(dirs, abs) {
			continue
		}
		if !insideAny(dirs, resolved) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func insideAny(roots []string, abs string) bool {
	for _, root := range roots {
		if pathContains(root, abs) {
			return true
		}
	}
	return false
}

// pathContains reports whether the candidate path lies inside root, using
// case-insensitive comparison on Windows where filesystems are typically
// case-insensitive. Trailing separators are normalised away on both sides.
func pathContains(root string, candidate string) bool {
	root = canonicalPathForComparison(root)
	candidate = canonicalPathForComparison(candidate)
	if equalPath(root, candidate) {
		return true
	}
	withSep := root
	if !strings.HasSuffix(withSep, string(filepath.Separator)) {
		withSep += string(filepath.Separator)
	}
	if hasPrefixPath(candidate, withSep) {
		return true
	}
	return false
}

func equalPath(a, b string) bool {
	if isCaseInsensitiveFS() {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func hasPrefixPath(s, prefix string) bool {
	if isCaseInsensitiveFS() {
		if len(s) < len(prefix) {
			return false
		}
		return strings.EqualFold(s[:len(prefix)], prefix)
	}
	return strings.HasPrefix(s, prefix)
}

func isCaseInsensitiveFS() bool {
	return os.PathSeparator == '\\'
}
