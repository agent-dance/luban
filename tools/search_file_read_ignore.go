// Package tools — file-read deny patterns applied to Glob/Grep output.
//
// Mirrors src/utils/glob.ts:86-89,110-112: when the harness has configured
// "file_read_ignore" deny patterns (typically ".env", "secrets/**", etc.),
// Glob and Grep both filter their results so the agent cannot enumerate
// paths it isn't allowed to read. Patterns use the same minimatch-style
// syntax as the rest of the search tooling and are matched against both
// the absolute and the relative-to-cwd path forms.
//
// The package-level state is intentionally global so SetupRegistry can
// configure it once at startup and every search-tool invocation enforces
// the deny rules without plumbing a dependency through every call site.
package tools

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/types"
	doublestar "github.com/bmatcuk/doublestar/v4"
)

const orphanedPluginMarker = ".orphaned_at"

type fileReadIgnoreRule struct {
	Root    string
	Pattern string
}

// fileReadIgnoreConfig is an invocation-local snapshot. Production tools
// derive it from RuntimeScope so concurrent registries cannot overwrite one
// another's deny rules. The package globals below remain only as a compatibility
// source for zero-value tools and older direct callers.
type fileReadIgnoreConfig struct {
	rules           []fileReadIgnoreRule
	pluginCacheRoot string
	pluginDirs      []string
}

func legacyFileReadIgnoreConfig() fileReadIgnoreConfig {
	fileReadIgnoreMu.RLock()
	patterns := append([]string(nil), fileReadIgnorePatterns...)
	fileReadIgnoreMu.RUnlock()
	orphanedPluginCacheExclusionsMu.RLock()
	pluginDirs := append([]string(nil), orphanedPluginCacheExclusions...)
	orphanedPluginCacheExclusionsMu.RUnlock()
	rules := make([]fileReadIgnoreRule, 0, len(patterns))
	for _, pattern := range patterns {
		rules = append(rules, fileReadIgnoreRule{Pattern: pattern})
	}
	return fileReadIgnoreConfig{rules: rules, pluginDirs: pluginDirs}
}

func runtimeFileReadIgnoreConfig(runtime types.ToolRuntimeContext, cacheRoot string, pluginDirs []string) fileReadIgnoreConfig {
	rules := make([]fileReadIgnoreRule, 0, len(runtime.DeniedRules))
	for _, rule := range runtime.DeniedRules {
		if !strings.EqualFold(strings.TrimSpace(rule.ToolName), "Read") {
			continue
		}
		pattern := strings.TrimSpace(rule.RuleContent)
		if pattern == "" {
			pattern = "**"
		}
		rules = append(rules, fileReadIgnoreRule{
			Root:    strings.TrimSpace(runtime.ProjectRoot),
			Pattern: filepath.ToSlash(pattern),
		})
	}
	cleanCacheRoot := ""
	if strings.TrimSpace(cacheRoot) != "" {
		cleanCacheRoot = filepath.Clean(cacheRoot)
	}
	return fileReadIgnoreConfig{
		rules:           rules,
		pluginCacheRoot: cleanCacheRoot,
		pluginDirs:      append([]string(nil), pluginDirs...),
	}
}

func (c fileReadIgnoreConfig) ripgrepGlobs(searchDir string) []string {
	out := make([]string, 0, len(c.rules)+len(c.pluginDirs))
	seen := make(map[string]struct{}, len(c.rules)+len(c.pluginDirs))
	add := func(pattern string) {
		pattern = strings.TrimSpace(filepath.ToSlash(pattern))
		if pattern == "" {
			return
		}
		pattern = strings.TrimPrefix(pattern, "./")
		if !strings.HasPrefix(pattern, "!") {
			pattern = "!" + pattern
		}
		if _, ok := seen[pattern]; ok {
			return
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	for _, rule := range c.rules {
		if pattern, ok := normalizeIgnoreRuleToSearchDir(rule, searchDir); ok {
			add(pattern)
		}
	}
	if c.pluginCacheRoot == "" || pathsOverlap(searchDir, c.pluginCacheRoot) {
		for _, dir := range c.pluginDirs {
			if pattern, ok := normalizePluginDirToSearchDir(dir, searchDir); ok {
				add(pattern)
			}
		}
	}
	return out
}

func normalizeIgnoreRuleToSearchDir(rule fileReadIgnoreRule, searchDir string) (string, bool) {
	pattern := strings.TrimSpace(filepath.FromSlash(rule.Pattern))
	if pattern == "" {
		return "", false
	}
	if rule.Root != "" && !filepath.IsAbs(pattern) {
		pattern = filepath.Join(rule.Root, pattern)
	}
	if filepath.IsAbs(pattern) {
		rel, err := filepath.Rel(searchDir, pattern)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", false
		}
		pattern = rel
	}
	return filepath.ToSlash(pattern), true
}

func normalizePluginDirToSearchDir(dir string, searchDir string) (string, bool) {
	dir = filepath.Clean(filepath.FromSlash(strings.TrimSpace(dir)))
	if dir == "" || dir == "." {
		return "", false
	}
	if !filepath.IsAbs(dir) {
		return path.Join(filepath.ToSlash(dir), "**"), true
	}
	rel, err := filepath.Rel(searchDir, dir)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return "**", true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		// The search may itself be nested inside an orphaned version directory.
		if pathContains(dir, searchDir) {
			return "**", true
		}
		return "", false
	}
	return path.Join(filepath.ToSlash(rel), "**"), true
}

func (c fileReadIgnoreConfig) ignored(filePath string) bool {
	if c.pluginCacheRoot == "" || pathsOverlap(filePath, c.pluginCacheRoot) {
		for _, dir := range c.pluginDirs {
			if pluginDirectoryContains(dir, filePath) {
				return true
			}
		}
	}
	for _, rule := range c.rules {
		if ignoreRuleMatchesPath(rule, filePath) {
			return true
		}
	}
	return false
}

func ignoreRuleMatchesPath(rule fileReadIgnoreRule, filePath string) bool {
	pattern := filepath.ToSlash(strings.TrimSpace(rule.Pattern))
	if pattern == "" {
		return false
	}
	candidates := []string{filepath.ToSlash(filePath), filepath.Base(filePath)}
	if rule.Root != "" {
		if rel, err := filepath.Rel(rule.Root, filePath); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			candidates = append(candidates, filepath.ToSlash(rel))
		}
	}
	for _, candidate := range candidates {
		if ok, _ := doublestar.Match(pattern, candidate); ok {
			return true
		}
		if ok, _ := doublestar.PathMatch(pattern, candidate); ok {
			return true
		}
		if !filepath.IsAbs(pattern) && strings.Contains(pattern, "/") && !strings.HasPrefix(pattern, "**/") {
			if ok, _ := doublestar.Match("**/"+pattern, candidate); ok {
				return true
			}
		}
	}
	return false
}

func pluginDirectoryContains(dir string, candidate string) bool {
	dir = filepath.Clean(filepath.FromSlash(dir))
	if filepath.IsAbs(dir) {
		return pathContains(dir, candidate)
	}
	candidate = filepath.ToSlash(candidate)
	dir = filepath.ToSlash(dir)
	return candidate == dir || strings.Contains(candidate, "/"+strings.Trim(dir, "/")+"/")
}

func (c fileReadIgnoreConfig) filter(paths []string) []string {
	if len(paths) == 0 || (len(c.rules) == 0 && len(c.pluginDirs) == 0) {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, filePath := range paths {
		if !c.ignored(filePath) {
			out = append(out, filePath)
		}
	}
	return out
}

type orphanedPluginCache struct {
	once sync.Once
	root string
	dirs []string
}

func (c *orphanedPluginCache) snapshot() (string, []string) {
	if c == nil {
		return "", nil
	}
	c.once.Do(func() {
		c.root, c.dirs = discoverOrphanedPluginDirectories()
	})
	return c.root, append([]string(nil), c.dirs...)
}

func discoverOrphanedPluginDirectories() (string, []string) {
	pluginsDir, err := claudePluginsDir()
	if err != nil || strings.TrimSpace(pluginsDir) == "" {
		return "", nil
	}
	cacheRoot := filepath.Join(pluginsDir, "cache")
	dirs := make([]string, 0)
	seen := make(map[string]struct{})
	_ = filepath.Walk(cacheRoot, func(filePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) || os.IsPermission(walkErr) {
				return nil
			}
			return walkErr
		}
		if info == nil {
			return nil
		}
		rel, err := filepath.Rel(cacheRoot, filePath)
		if err != nil {
			return nil
		}
		depth := 0
		if rel != "." {
			depth = len(strings.Split(filepath.Clean(rel), string(filepath.Separator)))
		}
		if info.IsDir() && depth > 3 {
			return filepath.SkipDir
		}
		if info.IsDir() || info.Name() != orphanedPluginMarker || depth != 4 {
			return nil
		}
		dir := filepath.Clean(filepath.Dir(filePath))
		if _, ok := seen[dir]; !ok {
			seen[dir] = struct{}{}
			dirs = append(dirs, dir)
		}
		return nil
	})
	return filepath.Clean(cacheRoot), dirs
}

func pathsOverlap(a string, b string) bool {
	a = canonicalPathForComparison(a)
	b = canonicalPathForComparison(b)
	return pathContains(a, b) || pathContains(b, a)
}

var (
	fileReadIgnoreMu       sync.RWMutex
	fileReadIgnorePatterns []string
)

// SetFileReadIgnorePatterns replaces the active deny pattern list. Pass
// nil/empty to disable filtering. Each pattern is normalised via filepath
// separators so Windows-style entries still match POSIX-built paths.
func SetFileReadIgnorePatterns(patterns []string) {
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Normalise to forward slashes — doublestar matches against
		// forward-slash paths, and ToSlash gives us a stable key.
		out = append(out, filepath.ToSlash(p))
	}
	fileReadIgnoreMu.Lock()
	fileReadIgnorePatterns = out
	fileReadIgnoreMu.Unlock()
}

// FileReadIgnorePatterns returns a snapshot of the configured deny list.
func FileReadIgnorePatterns() []string {
	fileReadIgnoreMu.RLock()
	defer fileReadIgnoreMu.RUnlock()
	out := make([]string, len(fileReadIgnorePatterns))
	copy(out, fileReadIgnorePatterns)
	return out
}

// RipgrepFileReadIgnoreGlobs returns negative --glob patterns for ripgrep so
// denied files are excluded before collection and cannot consume the first
// Glob result page. Post-filtering remains as defense in depth.
func RipgrepFileReadIgnoreGlobs(searchDir string) []string {
	fileReadIgnoreMu.RLock()
	ignorePatterns := append([]string(nil), fileReadIgnorePatterns...)
	fileReadIgnoreMu.RUnlock()
	orphanedPluginCacheExclusionsMu.RLock()
	pluginPatterns := append([]string(nil), orphanedPluginCacheExclusions...)
	orphanedPluginCacheExclusionsMu.RUnlock()

	out := make([]string, 0, len(ignorePatterns)+len(pluginPatterns))
	seen := make(map[string]struct{}, len(ignorePatterns)+len(pluginPatterns))
	add := func(pattern string) {
		pattern = strings.TrimSpace(filepath.ToSlash(pattern))
		if pattern == "" {
			return
		}
		if filepath.IsAbs(pattern) {
			if rel, err := filepath.Rel(searchDir, filepath.FromSlash(pattern)); err == nil && !strings.HasPrefix(rel, "..") {
				pattern = filepath.ToSlash(rel)
			}
		}
		pattern = strings.TrimPrefix(pattern, "./")
		negative := pattern
		if !strings.HasPrefix(negative, "!") {
			negative = "!" + negative
		}
		if _, ok := seen[negative]; !ok {
			seen[negative] = struct{}{}
			out = append(out, negative)
		}
		bare := strings.TrimPrefix(pattern, "!")
		if !strings.Contains(bare, "/") {
			nested := "!**/" + bare
			if _, ok := seen[nested]; !ok {
				seen[nested] = struct{}{}
				out = append(out, nested)
			}
		} else if !filepath.IsAbs(bare) && !strings.HasPrefix(bare, "**/") {
			nested := "!**/" + bare
			if _, ok := seen[nested]; !ok {
				seen[nested] = struct{}{}
				out = append(out, nested)
			}
		}
	}
	for _, pattern := range ignorePatterns {
		add(pattern)
	}
	for _, pattern := range pluginPatterns {
		p := strings.TrimSpace(filepath.ToSlash(pattern))
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "!") {
			p = path.Join(p, "**")
		}
		add(p)
	}
	return out
}

// IsFileReadIgnored reports whether the given path matches one of the
// configured deny patterns. The path is checked in three forms — the
// raw string, ToSlash-normalised, and basename — so patterns like
// ".env" match both bare and qualified paths. Matches use doublestar's
// globstar-aware matcher (the same engine the Go glob fallback uses).
// Orphaned plugin-cache exclusions are also honoured.
func IsFileReadIgnored(path string) bool {
	if IsOrphanedPluginCachePath(path) {
		return true
	}
	fileReadIgnoreMu.RLock()
	patterns := fileReadIgnorePatterns
	fileReadIgnoreMu.RUnlock()
	if len(patterns) == 0 {
		return false
	}
	candidates := []string{path, filepath.ToSlash(path), filepath.Base(path)}
	for _, p := range patterns {
		for _, c := range candidates {
			if ok, _ := doublestar.Match(p, c); ok {
				return true
			}
			if ok, _ := doublestar.PathMatch(p, c); ok {
				return true
			}
			if !filepath.IsAbs(p) && strings.Contains(p, "/") && !strings.HasPrefix(p, "**/") {
				nested := "**/" + p
				if ok, _ := doublestar.Match(nested, c); ok {
					return true
				}
				if ok, _ := doublestar.PathMatch(nested, c); ok {
					return true
				}
			}
		}
	}
	return false
}

// FilterFileReadIgnored returns the subset of paths that are NOT denied
// by file_read_ignore patterns. Order is preserved.
func FilterFileReadIgnored(paths []string) []string {
	if len(paths) == 0 {
		return paths
	}
	fileReadIgnoreMu.RLock()
	hasPatterns := len(fileReadIgnorePatterns) > 0
	fileReadIgnoreMu.RUnlock()
	orphanedPluginCacheExclusionsMu.RLock()
	hasOrphan := len(orphanedPluginCacheExclusions) > 0
	orphanedPluginCacheExclusionsMu.RUnlock()
	if !hasPatterns && !hasOrphan {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if IsFileReadIgnored(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// orphanedPluginCacheExclusionsMu / orphanedPluginCacheExclusions hold the
// active list of plugin-cache directories that should be skipped from
// glob results. Mirrors src/utils/glob.ts:114-117 — old plugin install
// directories pollute results with duplicate copies of files at different
// versions.
var (
	orphanedPluginCacheExclusionsMu sync.RWMutex
	orphanedPluginCacheExclusions   []string
)

// SetOrphanedPluginCacheExclusions configures the directories to skip.
// Each entry is matched as a glob pattern via doublestar.
func SetOrphanedPluginCacheExclusions(dirs []string) {
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		out = append(out, filepath.ToSlash(d))
	}
	orphanedPluginCacheExclusionsMu.Lock()
	orphanedPluginCacheExclusions = out
	orphanedPluginCacheExclusionsMu.Unlock()
}

// IsOrphanedPluginCachePath reports whether a path falls inside a
// configured orphaned plugin cache directory.
func IsOrphanedPluginCachePath(path string) bool {
	orphanedPluginCacheExclusionsMu.RLock()
	patterns := orphanedPluginCacheExclusions
	orphanedPluginCacheExclusionsMu.RUnlock()
	if len(patterns) == 0 {
		return false
	}
	candidate := filepath.ToSlash(path)
	for _, p := range patterns {
		if ok, _ := doublestar.Match(p, candidate); ok {
			return true
		}
		if ok, _ := doublestar.PathMatch(p, candidate); ok {
			return true
		}
		// Allow bare directory entries to act as prefix matches.
		if strings.HasPrefix(candidate, p+"/") || candidate == p {
			return true
		}
	}
	return false
}
