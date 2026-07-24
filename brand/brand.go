// Package brand centralizes LUBAN Code product identity and migration names.
package brand

import (
	"os"
	"path/filepath"
)

const (
	DisplayName = "LUBAN Code"
	RuntimeName = DisplayName
	CommandName = "luban-code"

	LegacyDeepSeekCommandName = "deepseek-code"
	LegacyCommandName         = "claude-code-go"

	ConfigDirName               = ".luban-code"
	LegacyDeepSeekConfigDirName = ".deepseek-code"
	LegacyConfigDirName         = ".claude"
	LegacyGoConfigDirName       = ".claude-go"

	InstructionsFile               = "LUBAN.md"
	LegacyDeepSeekInstructionsFile = "DEEPSEEK.md"
	AgentsFile                     = "AGENTS.md"
	LegacyInstructionsFile         = "CLAUDE.md"

	DefaultProvider      = "deepseek"
	DeepSeekProvider     = "deepseek"
	DeepSeekDefaultModel = "deepseek-v4-flash"
	DeepSeekProModel     = "deepseek-v4-pro"
	DeepSeekBaseURL      = "https://api.deepseek.com/v1"

	RateLimitEnv               = "LUBAN_CODE_RATE_LIMIT"
	LegacyDeepSeekRateLimitEnv = "DEEPSEEK_CODE_RATE_LIMIT"
	LegacyRateLimitEnv         = "CLAUDE_RATE_LIMIT"
)

var (
	terminalWideLogoLines = []string{
		"█      █    █ █████   ████  █    █",
		"█      █    █ █    █ █    █ ██   █",
		"█      █    █ █████  ██████ █ █  █",
		"█      █    █ █    █ █    █ █  ███",
		"██████  ████  █████  █    █ █    █",
	}

	terminalCompactLogoLines = []string{
		"█▀▀ █ █ █▀█ █▀█ █ █",
		"█▄▄ █▄█ █▄█ █ █ █▄█",
	}
)

// TerminalLogoLines returns a terminal-safe LUBAN Code wordmark sized for
// the available cell width. Callers get a copy so renderers cannot mutate the
// shared brand data.
func TerminalLogoLines(width int) []string {
	switch {
	case width >= 78:
		return cloneStrings(terminalWideLogoLines)
	case width >= 32:
		return cloneStrings(terminalCompactLogoLines)
	default:
		return []string{"LUBAN"}
	}
}

func cloneStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func UserConfigDir() string {
	home := HomeDir()
	if home == "" {
		return ConfigDirName
	}
	return filepath.Join(home, ConfigDirName)
}

func LegacyUserConfigDir() string {
	home := HomeDir()
	if home == "" {
		return LegacyConfigDirName
	}
	return filepath.Join(home, LegacyConfigDirName)
}

func LegacyDeepSeekUserConfigDir() string {
	home := HomeDir()
	if home == "" {
		return LegacyDeepSeekConfigDirName
	}
	return filepath.Join(home, LegacyDeepSeekConfigDirName)
}

func LegacyUserGoDir() string {
	home := HomeDir()
	if home == "" {
		return LegacyGoConfigDirName
	}
	return filepath.Join(home, LegacyGoConfigDirName)
}

func SessionsDir() string {
	return filepath.Join(UserConfigDir(), "sessions")
}

func MemoryPath() string {
	return filepath.Join(UserConfigDir(), "memory.json")
}

func HistoryPath() string {
	return filepath.Join(UserConfigDir(), "history")
}
