package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// ---------------------------------------------------------------------------
// /init  (/setup)
// ---------------------------------------------------------------------------

type initCmd struct{}

func (c *initCmd) Name() string      { return "init" }
func (c *initCmd) Aliases() []string { return []string{"setup"} }
func (c *initCmd) Description() string {
	return builtinCommandDescription("init")
}

func (c *initCmd) Execute(ctx *Context, _ string) error {
	cwd := ctx.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	var created, existing []string
	hadFailure := false

	// --- .luban-code/ directory ---
	configDir := filepath.Join(cwd, brand.ConfigDirName)
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		if mkErr := os.MkdirAll(configDir, 0o755); mkErr != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyInitCreateDirError, brand.ConfigDirName, mkErr))
			reportCommandFailed(ctx)
			return nil
		}
		created = append(created, brand.ConfigDirName+"/")
	} else {
		existing = append(existing, brand.ConfigDirName+"/")
	}

	// --- LUBAN.md ---
	instructionsPath := filepath.Join(cwd, brand.InstructionsFile)
	if _, err := os.Stat(instructionsPath); os.IsNotExist(err) {
		instructionsTemplate := i18n.Text(ctx.Language, i18n.KeyRuntimeProjectInstructionsTemplate)
		if writeErr := os.WriteFile(instructionsPath, []byte(instructionsTemplate), 0o644); writeErr != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyInitCreateFileError, brand.InstructionsFile, writeErr))
			hadFailure = true
		} else {
			created = append(created, brand.InstructionsFile)
		}
	} else {
		existing = append(existing, brand.InstructionsFile)
	}

	// --- .luban-code/settings.json ---
	settingsPath := filepath.Join(configDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		defaults := map[string]interface{}{
			"provider":   brand.DeepSeekProvider,
			"model":      brand.DeepSeekDefaultModel,
			"mcpServers": map[string]interface{}{},
		}
		data, _ := json.MarshalIndent(defaults, "", "  ")
		if writeErr := os.WriteFile(settingsPath, append(data, '\n'), 0o600); writeErr != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyInitCreateSettingsError, writeErr))
			hadFailure = true
		} else {
			created = append(created, filepath.Join(brand.ConfigDirName, "settings.json"))
		}
	} else {
		existing = append(existing, filepath.Join(brand.ConfigDirName, "settings.json"))
	}

	var sb strings.Builder
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyInitReport))
	for _, f := range created {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyInitCreated, f))
	}
	for _, f := range existing {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyInitExists, f))
	}
	ctx.OnEvent(sb.String())
	if hadFailure {
		reportCommandFailed(ctx)
	} else {
		reportCommandSucceeded(ctx)
	}
	return nil
}
