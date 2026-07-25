package prompt

import (
	"os"
	"path/filepath"
	"strings"
)

type processRulesOptions struct {
	rulesDir        string
	typ             MemoryType
	processed       map[string]struct{}
	includeExternal bool
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
		if !ok || isConditional {
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
