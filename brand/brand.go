// Package brand centralizes LUBAN Code product identity.
package brand

import (
	"os"
	"path/filepath"
)

const (
	DisplayName = "LUBAN Code"
	RuntimeName = DisplayName
	CommandName = "luban-code"

	ConfigDirName         = ".luban-code"
	InstructionsFile      = "LUBAN.md"
	LocalInstructionsFile = "LUBAN.local.md"
	AgentsFile            = "AGENTS.md"

	DefaultProvider      = "deepseek"
	DeepSeekProvider     = "deepseek"
	DeepSeekDefaultModel = "deepseek-v4-flash"
	DeepSeekProModel     = "deepseek-v4-pro"
	DeepSeekBaseURL      = "https://api.deepseek.com"
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
