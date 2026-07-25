package prompt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agent-dance/luban/brand"
)

const memoryInstructionPrompt = "Codebase and user instructions are shown below. Be sure to adhere to these instructions. IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written."

// MemoryType identifies the source and priority class of a memory file.
type MemoryType string

const (
	MemoryTypeUser    MemoryType = "User"
	MemoryTypeProject MemoryType = "Project"
	MemoryTypeLocal   MemoryType = "Local"
	MemoryTypeManaged MemoryType = "Managed"
)

// MemoryFileInfo describes a discovered memory file and its model-visible content.
type MemoryFileInfo struct {
	Path    string
	Type    MemoryType
	Content string
	Parent  string
}

// DiscoverMemoryFiles returns memory files in priority order:
// managed, user, project, then local. Later entries have higher priority.
func DiscoverMemoryFiles(cwd string) []MemoryFileInfo {
	return DiscoverMemoryFilesWithSettings(cwd, defaultPromptSettings())
}

// DiscoverMemoryFilesWithSettings returns memory files using explicit
// prompt-affecting settings supplied by a runtime settings layer.
func DiscoverMemoryFilesWithSettings(cwd string, settings PromptSettings) []MemoryFileInfo {
	if !shouldDiscoverMemory(settings) {
		return nil
	}
	return discoverMemoryFiles(cwd, defaultMemoryPaths(), settings)
}

type memoryPaths struct {
	managedDir string
	userDir    string
	userDirs   []string
}

func defaultMemoryPaths() memoryPaths {
	home, _ := os.UserHomeDir()
	userDirs := []string{
		firstNonEmptyPath(os.Getenv("LUBAN_CODE_CONFIG_DIR"), filepath.Join(home, brand.ConfigDirName)),
	}
	return memoryPaths{
		managedDir: managedMemoryDir(),
		userDirs:   nonEmptyUniquePaths(userDirs),
	}
}

func firstNonEmptyPath(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func nonEmptyUniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func managedMemoryDir() string {
	if os.Getenv("USER_TYPE") == "ant" {
		if override := strings.TrimSpace(os.Getenv("LUBAN_CODE_MANAGED_SETTINGS_PATH")); override != "" {
			return override
		}
	}
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/LUBAN Code"
	case "windows":
		return `C:\Program Files\LUBAN Code`
	default:
		return "/etc/luban-code"
	}
}

func discoverMemoryFiles(cwd string, paths memoryPaths, settingsArg ...PromptSettings) []MemoryFileInfo {
	settings := defaultPromptSettings()
	if len(settingsArg) > 0 {
		settings = settingsArg[0]
	}
	var result []MemoryFileInfo
	seen := make(map[string]struct{})

	if strings.TrimSpace(paths.managedDir) != "" {
		appendMemoryFile(&result, seen, filepath.Join(paths.managedDir, brand.InstructionsFile), MemoryTypeManaged, false, cwd, settings)
		result = append(result, processMdRules(processRulesOptions{
			rulesDir:        filepath.Join(paths.managedDir, brand.ConfigDirName, "rules"),
			typ:             MemoryTypeManaged,
			processed:       seen,
			includeExternal: false,
			cwd:             cwd,
			settings:        settings,
		})...)
	}
	userDirs := paths.userDirs
	if strings.TrimSpace(paths.userDir) != "" {
		userDirs = append(userDirs, paths.userDir)
	}
	for _, userDir := range nonEmptyUniquePaths(userDirs) {
		appendInstructionFiles(&result, seen, userDir, MemoryTypeUser, true, cwd, settings, false)
		result = append(result, processMdRules(processRulesOptions{
			rulesDir:        filepath.Join(userDir, "rules"),
			typ:             MemoryTypeUser,
			processed:       seen,
			includeExternal: true,
			cwd:             cwd,
			settings:        settings,
		})...)
	}

	if shouldDiscoverAutoMemory(settings) {
		for _, dir := range rootToCWD(cwd) {
			appendInstructionFiles(&result, seen, dir, MemoryTypeProject, false, cwd, settings, true)
			result = append(result, processMdRules(processRulesOptions{
				rulesDir:        filepath.Join(dir, brand.ConfigDirName, "rules"),
				typ:             MemoryTypeProject,
				processed:       seen,
				includeExternal: false,
				cwd:             cwd,
				settings:        settings,
			})...)
			appendMemoryFile(&result, seen, filepath.Join(dir, brand.LocalInstructionsFile), MemoryTypeLocal, false, cwd, settings)
		}
	}

	if shouldDiscoverAdditionalDirectoryMemory(settings) {
		for _, dir := range nonEmptyStrings(settings.AdditionalDirectories) {
			appendInstructionFiles(&result, seen, dir, MemoryTypeProject, false, cwd, settings, true)
			result = append(result, processMdRules(processRulesOptions{
				rulesDir:        filepath.Join(dir, brand.ConfigDirName, "rules"),
				typ:             MemoryTypeProject,
				processed:       seen,
				includeExternal: false,
				cwd:             cwd,
				settings:        settings,
			})...)
		}
	}

	return result
}

func appendInstructionFiles(result *[]MemoryFileInfo, seen map[string]struct{}, dir string, typ MemoryType, includeExternal bool, cwd string, settings PromptSettings, includeConfigDirs bool) {
	appendMemoryFile(result, seen, filepath.Join(dir, brand.AgentsFile), typ, includeExternal, cwd, settings)
	appendMemoryFile(result, seen, filepath.Join(dir, brand.InstructionsFile), typ, includeExternal, cwd, settings)
	if includeConfigDirs {
		appendMemoryFile(result, seen, filepath.Join(dir, brand.ConfigDirName, brand.InstructionsFile), typ, includeExternal, cwd, settings)
	}
}

func appendMemoryFile(result *[]MemoryFileInfo, seen map[string]struct{}, path string, typ MemoryType, includeExternal bool, cwd string, settings PromptSettings) {
	*result = append(*result, processMemoryFileWithSettings(path, typ, seen, includeExternal, 0, "", cwd, settings)...)
}

func processMemoryFileWithSettings(path string, typ MemoryType, processed map[string]struct{}, includeExternal bool, depth int, parent string, cwd string, settings PromptSettings) []MemoryFileInfo {
	if processed == nil {
		processed = make(map[string]struct{})
	}
	if depth >= maxIncludeDepth {
		return nil
	}
	if shouldExcludeInstruction(path, typ, settings) {
		return nil
	}
	normalized := normalizeMemoryPath(path)
	if _, ok := processed[normalized]; ok {
		return nil
	}
	processed[normalized] = struct{}{}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if !isSupportedIncludePath(path) {
		return nil
	}
	rawContent := string(data)
	content, _ := parseMemoryFrontmatterPaths(rawContent)
	includePaths := extractIncludePaths(content, path)
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	info := MemoryFileInfo{Path: path, Type: typ, Content: content, Parent: parent}
	result := []MemoryFileInfo{info}

	for _, includePath := range includePaths {
		if !includeExternal && !pathInMemoryCwd(includePath, cwd) {
			continue
		}
		result = append(result, processMemoryFileWithSettings(includePath, typ, processed, includeExternal, depth+1, path, cwd, settings)...)
	}
	return result
}

func normalizeMemoryPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func rootToCWD(cwd string) []string {
	if strings.TrimSpace(cwd) == "" {
		if current, err := os.Getwd(); err == nil {
			cwd = current
		} else {
			cwd = "."
		}
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	cwd = filepath.Clean(cwd)

	var dirs []string
	for {
		dirs = append(dirs, cwd)
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}

// FormatMemoryFiles renders memory files with source descriptions.
func FormatMemoryFiles(files []MemoryFileInfo) string {
	var memories []string
	for _, file := range files {
		if strings.TrimSpace(file.Content) == "" {
			continue
		}
		content := strings.TrimSpace(file.Content)
		memories = append(memories, "Contents of "+file.Path+memoryTypeDescription(file.Type)+":\n\n"+content)
	}
	if len(memories) == 0 {
		return ""
	}
	return memoryInstructionPrompt + "\n\n" + strings.Join(memories, "\n\n")
}

func pathInMemoryCwd(path, cwd string) bool {
	if strings.TrimSpace(cwd) == "" {
		if current, err := os.Getwd(); err == nil {
			cwd = current
		}
	}
	pathAbs, err := filepath.Abs(path)
	if err == nil {
		path = pathAbs
	}
	cwdAbs, err := filepath.Abs(cwd)
	if err == nil {
		cwd = cwdAbs
	}
	rel, err := filepath.Rel(filepath.Clean(cwd), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}

func memoryTypeDescription(typ MemoryType) string {
	switch typ {
	case MemoryTypeProject:
		return " (project instructions, checked into the codebase)"
	case MemoryTypeLocal:
		return " (user's private project instructions, not checked in)"
	default:
		return " (user's private global instructions for all projects)"
	}
}

// DiscoverInstructions discovers and formats project instruction files.
func DiscoverInstructions(cwd string) string {
	return FormatMemoryFiles(DiscoverMemoryFiles(cwd))
}
