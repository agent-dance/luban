package prompt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	disableInstructionsEnv             = "LUBAN_CODE_DISABLE_INSTRUCTIONS"
	lubanBareModeEnv                   = "LUBAN_CODE_BARE"
	additionalDirectoryInstructionsEnv = "LUBAN_CODE_ADDITIONAL_DIRECTORY_INSTRUCTIONS"
)

// PromptSettings contains the prompt-affecting runtime settings.
type PromptSettings struct {
	DisableInstructions             bool
	BareMode                        bool
	InstructionExcludes             []string
	AdditionalDirectories           []string
	AdditionalDirectoryInstructions bool
	Language                        string
	OutputStyle                     string
}

func defaultPromptSettings() PromptSettings {
	return PromptSettings{
		DisableInstructions:             promptEnvTruthy(disableInstructionsEnv),
		BareMode:                        promptEnvTruthy(lubanBareModeEnv),
		AdditionalDirectoryInstructions: promptEnvTruthy(additionalDirectoryInstructionsEnv),
		Language:                        firstPromptEnv("LUBAN_CODE_LANGUAGE"),
		OutputStyle:                     firstPromptEnv("LUBAN_CODE_OUTPUT_STYLE"),
	}
}

func promptEnvTruthy(key string) bool {
	return isTruthyPromptEnv(os.Getenv(key))
}

func firstPromptEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func shouldDiscoverMemory(settings PromptSettings) bool {
	if settings.DisableInstructions {
		return false
	}
	if settings.BareMode && len(nonEmptyStrings(settings.AdditionalDirectories)) == 0 {
		return false
	}
	return true
}

func shouldDiscoverAutoMemory(settings PromptSettings) bool {
	return !settings.BareMode
}

func shouldDiscoverAdditionalDirectoryMemory(settings PromptSettings) bool {
	return settings.AdditionalDirectoryInstructions && len(nonEmptyStrings(settings.AdditionalDirectories)) > 0
}

func shouldExcludeInstruction(path string, typ MemoryType, settings PromptSettings) bool {
	if typ != MemoryTypeUser && typ != MemoryTypeProject && typ != MemoryTypeLocal {
		return false
	}
	patterns := nonEmptyStrings(settings.InstructionExcludes)
	if len(patterns) == 0 {
		return false
	}
	candidates := []string{slashPath(path)}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		candidates = append(candidates, slashPath(real))
	}
	for _, pattern := range expandExcludePatterns(patterns) {
		for _, candidate := range candidates {
			if matchInstructionExclude(pattern, candidate) {
				return true
			}
		}
	}
	return false
}

func expandExcludePatterns(patterns []string) []string {
	expanded := make([]string, 0, len(patterns)*2)
	for _, pattern := range patterns {
		normalized := slashPath(pattern)
		expanded = append(expanded, normalized)
		if !filepath.IsAbs(pattern) {
			continue
		}
		prefix := staticGlobPrefix(pattern)
		dir := prefix
		if info, err := os.Stat(dir); err == nil && !info.IsDir() {
			dir = filepath.Dir(dir)
		}
		if real, err := filepath.EvalSymlinks(dir); err == nil && real != dir {
			expanded = append(expanded, slashPath(real+pattern[len(dir):]))
		}
	}
	return expanded
}

func staticGlobPrefix(pattern string) string {
	cut := len(pattern)
	for _, marker := range []string{"*", "?", "[", "{"} {
		if idx := strings.Index(pattern, marker); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	if cut == len(pattern) {
		return pattern
	}
	prefix := pattern[:cut]
	if prefix == "" {
		return string(filepath.Separator)
	}
	return filepath.Clean(prefix)
}

func matchInstructionExclude(pattern, candidate string) bool {
	pattern = slashPath(pattern)
	candidate = slashPath(candidate)
	if ok, err := doublestar.PathMatch(pattern, candidate); err == nil && ok {
		return true
	}
	if !strings.HasPrefix(pattern, "/") {
		if ok, err := doublestar.PathMatch(pattern, strings.TrimPrefix(candidate, "/")); err == nil && ok {
			return true
		}
	}
	return false
}

func slashPath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return filepath.ToSlash(path)
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
