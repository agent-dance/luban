// Package search applies file-read deny patterns to Glob/Grep output.
//
// When the runtime has configured file-read deny patterns (typically ".env",
// "secrets/**", etc.),
// Glob and Grep both filter their results so the agent cannot enumerate
// paths it isn't allowed to read. Patterns use the toolbase glob contract
// and are matched against both
// the absolute and the relative-to-cwd path forms.
//
// Each Glob/Grep instance derives an immutable snapshot from its runtime
// scope, so concurrent registries cannot overwrite one another's deny rules.
package search

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
	doublestar "github.com/bmatcuk/doublestar/v4"
)

const orphanedPluginMarker = ".orphaned_at"

type fileReadIgnoreRule struct {
	Root    string
	Pattern string
}

// fileReadIgnoreConfig is an invocation-local snapshot derived from
// RuntimeScope.
type fileReadIgnoreConfig struct {
	rules           []fileReadIgnoreRule
	pluginCacheRoot string
	pluginDirs      []string
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
		if toolbase.PathContains(dir, searchDir) {
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
		return toolbase.PathContains(dir, candidate)
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
	root := searchPluginsDir()
	if strings.TrimSpace(root) == "" {
		return "", nil
	}
	cacheRoot := filepath.Join(root, "cache")
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

func searchPluginsDir() string {
	if dir := strings.TrimSpace(os.Getenv("LUBAN_CODE_PLUGIN_CACHE_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	configDir := strings.TrimSpace(os.Getenv("LUBAN_CODE_CONFIG_DIR"))
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		configDir = filepath.Join(home, brand.ConfigDirName)
	}
	dirName := "plugins"
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LUBAN_CODE_USE_COWORK_PLUGINS"))) {
	case "1", "true", "yes", "on":
		dirName = "cowork_plugins"
	}
	return filepath.Join(configDir, dirName)
}

func pathsOverlap(a string, b string) bool {
	a = toolbase.CanonicalPath(a)
	b = toolbase.CanonicalPath(b)
	return toolbase.PathContains(a, b) || toolbase.PathContains(b, a)
}
