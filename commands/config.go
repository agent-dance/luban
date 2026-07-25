package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// ---------------------------------------------------------------------------
// /config  (/cfg)
// ---------------------------------------------------------------------------

type configCmd struct{}

func (c *configCmd) Name() string      { return "config" }
func (c *configCmd) Aliases() []string { return []string{"cfg"} }
func (c *configCmd) Description() string {
	return builtinCommandDescription("config")
}

func (c *configCmd) Execute(ctx *Context, args string) error {
	parts := strings.Fields(args)

	if len(parts) == 0 || parts[0] == "list" {
		return c.list(ctx)
	}

	switch parts[0] {
	case "get":
		if len(parts) < 2 {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConfigUsageGet))
			reportCommandFailed(ctx)
			return nil
		}
		return c.get(ctx, parts[1])
	case "set":
		if len(parts) < 3 {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConfigUsageSet))
			reportCommandFailed(ctx)
			return nil
		}
		return c.set(ctx, parts[1], strings.Join(parts[2:], " "))
	default:
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyConfigUsage))
		reportCommandFailed(ctx)
		return nil
	}
}

func (c *configCmd) settingsPath(ctx *Context) string {
	cwd := ctx.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	local := filepath.Join(cwd, brand.ConfigDirName, "settings.json")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return filepath.Join(brand.UserConfigDir(), "settings.json")
}

func projectSettingsPath(ctx *Context) string {
	cwd := ctx.CWD
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, brand.ConfigDirName, "settings.json")
}

func persistProjectSetting(ctx *Context, key string, value interface{}) (string, error) {
	return persistProjectSettings(ctx, map[string]interface{}{key: value})
}

func persistProjectSettings(ctx *Context, values map[string]interface{}) (string, error) {
	path := projectSettingsPath(ctx)
	if path == "" {
		return "", nil
	}
	if len(values) == 0 {
		return path, nil
	}
	c := configCmd{}
	m, err := c.readSettings(path, ctx.Language)
	if err != nil {
		return path, err
	}
	for key, value := range values {
		m[key] = value
	}
	if err := c.writeSettings(path, m); err != nil {
		return path, err
	}
	return path, nil
}

func (c *configCmd) readSettings(path string, languages ...i18n.Language) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		lang := i18n.DetectOrLoadLanguage()
		if len(languages) > 0 {
			lang = languages[0]
		}
		return nil, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyRuntimeSettingsParseError, err))
	}
	return m, nil
}

func (c *configCmd) writeSettings(path string, m map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (c *configCmd) list(ctx *Context) error {
	path := c.settingsPath(ctx)
	m, err := c.readSettings(path, ctx.Language)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyAdminReadSettingsError, err))
		reportCommandFailed(ctx)
		return nil
	}
	if len(m) == 0 {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConfigNoSettings, path))
		reportCommandSucceeded(ctx)
		return nil
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConfigSettings, path, string(data)))
	reportCommandSucceeded(ctx)
	return nil
}

func (c *configCmd) get(ctx *Context, key string) error {
	path := c.settingsPath(ctx)
	m, err := c.readSettings(path, ctx.Language)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyAdminReadSettingsError, err))
		reportCommandFailed(ctx)
		return nil
	}
	val, ok := m[key]
	if !ok {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConfigKeyMissing, key))
		reportCommandFailed(ctx)
		return nil
	}
	data, _ := json.MarshalIndent(val, "", "  ")
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConfigValue, key, string(data)))
	reportCommandSucceeded(ctx)
	return nil
}

// validConfigKeys is the allowlist of keys accepted by /config set.
// Rejecting unknown keys prevents typos from silently creating junk entries
// and limits the attack surface for settings injection.
var validConfigKeys = map[string]bool{
	"model":            true,
	"apiKey":           true,
	"provider":         true,
	"allowedTools":     true,
	"deniedTools":      true,
	"mcpServers":       true,
	"maxTokens":        true,
	"temperature":      true,
	"systemPrompt":     true,
	"theme":            true,
	"logLevel":         true,
	"cacheRoutingMode": true,
}

func (c *configCmd) set(ctx *Context, key, value string) error {
	if !validConfigKeys[key] {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConfigUnknownKey, key, "model, apiKey, provider, allowedTools, deniedTools, mcpServers, maxTokens, temperature, systemPrompt, theme, logLevel, cacheRoutingMode"))
		reportCommandFailed(ctx)
		return nil
	}
	cwd := ctx.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	path := filepath.Join(cwd, brand.ConfigDirName, "settings.json")

	m, err := c.readSettings(path, ctx.Language)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyAdminReadSettingsError, err))
		reportCommandFailed(ctx)
		return nil
	}
	if key == "cacheRoutingMode" {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "auto" && value != "on" && value != "off" {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConfigInvalidCacheRoutingMode, value))
			reportCommandFailed(ctx)
			return nil
		}
	}

	// Coerce value: bool > number > string.
	var parsed interface{}
	switch value {
	case "true":
		parsed = true
	case "false":
		parsed = false
	default:
		if n, err := strconv.ParseFloat(value, 64); err == nil {
			parsed = n
		} else {
			parsed = value
		}
	}

	m[key] = parsed
	if err := c.writeSettings(path, m); err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyAdminWriteSettingsError, err))
		reportCommandFailed(ctx)
		return nil
	}
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyConfigSet, key, parsed, path))
	reportCommandSucceeded(ctx)
	return nil
}
