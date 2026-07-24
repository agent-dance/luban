package prompt

import (
	"os"
	"path/filepath"
	"strings"

	doublestar "github.com/bmatcuk/doublestar/v4"
)

type processRulesOptions struct {
	rulesDir        string
	typ             MemoryType
	processed       map[string]struct{}
	includeExternal bool
	conditional     bool
	cwd             string
	settings        PromptSettings
	visitedDirs     map[string]struct{}
}

func processMdRules(opts processRulesOptions) []MemoryFileInfo {
	if opts.processed == nil {
		opts.processed = make(map[string]struct{})
	}
	if opts.visitedDirs == nil {
		opts.visitedDirs = make(map[string]struct{})
	}
	rulesDir := filepath.Clean(opts.rulesDir)
	visitKey := normalizeMemoryPath(rulesDir)
	if _, ok := opts.visitedDirs[visitKey]; ok {
		return nil
	}
	opts.visitedDirs[visitKey] = struct{}{}

	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return nil
	}

	var result []MemoryFileInfo
	for _, entry := range entries {
		entryPath := filepath.Join(rulesDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.IsDir() {
			next := opts
			next.rulesDir = entryPath
			result = append(result, processMdRules(next)...)
			continue
		}
		if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		isConditional, ok := ruleFileIsConditional(entryPath)
		if !ok || opts.conditional != isConditional {
			continue
		}
		files := processMemoryFileWithSettings(entryPath, opts.typ, opts.processed, opts.includeExternal, 0, "", opts.cwd, opts.settings)
		if len(files) == 0 {
			continue
		}
		result = append(result, files...)
	}
	return result
}

func ruleFileIsConditional(path string) (bool, bool) {
	if !isSupportedIncludePath(path) {
		return false, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	_, paths := parseMemoryFrontmatterPaths(string(data))
	return len(paths) > 0, true
}

func processConditionedMdRules(targetPath, rulesDir string, typ MemoryType, processed map[string]struct{}, includeExternal bool, cwd string) []MemoryFileInfo {
	return processConditionedMdRulesWithSettings(targetPath, rulesDir, typ, processed, includeExternal, cwd, defaultPromptSettings())
}

func processConditionedMdRulesWithSettings(targetPath, rulesDir string, typ MemoryType, processed map[string]struct{}, includeExternal bool, cwd string, settings PromptSettings) []MemoryFileInfo {
	files := processMdRules(processRulesOptions{
		rulesDir:        rulesDir,
		typ:             typ,
		processed:       processed,
		includeExternal: includeExternal,
		conditional:     true,
		cwd:             cwd,
		settings:        settings,
	})

	baseDir := cwd
	if typ == MemoryTypeProject {
		baseDir = filepath.Dir(filepath.Dir(rulesDir))
	}
	var result []MemoryFileInfo
	includeGroup := false
	for _, file := range files {
		if file.Parent == "" {
			includeGroup = memoryRuleMatchesTarget(file.Globs, targetPath, baseDir)
		}
		if includeGroup {
			result = append(result, file)
		}
	}
	return result
}

func memoryRuleMatchesTarget(globs []string, targetPath, baseDir string) bool {
	if len(globs) == 0 {
		return false
	}
	relativePath := targetPath
	if filepath.IsAbs(targetPath) {
		rel, err := filepath.Rel(baseDir, targetPath)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
			return false
		}
		relativePath = rel
	}
	relativePath = filepath.ToSlash(relativePath)
	for _, pattern := range globs {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if ok, _ := doublestar.Match(pattern, relativePath); ok {
			return true
		}
		if !strings.Contains(pattern, "/") {
			if ok, _ := doublestar.Match(pattern, filepath.Base(relativePath)); ok {
				return true
			}
		}
	}
	return false
}
