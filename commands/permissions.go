package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// ---------------------------------------------------------------------------
// /permissions  (/perms)
// ---------------------------------------------------------------------------

type permissionsCmd struct{}

func (c *permissionsCmd) Name() string      { return "permissions" }
func (c *permissionsCmd) Aliases() []string { return []string{"perms"} }
func (c *permissionsCmd) Description() string {
	return builtinCommandDescription("permissions")
}

// permissionsEntry mirrors the permissions section of settings.json.
type permissionsEntry struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

func (c *permissionsCmd) Execute(ctx *Context, args string) error {
	parts := strings.Fields(args)

	if len(parts) == 0 || parts[0] == "list" {
		return c.list(ctx)
	}

	switch parts[0] {
	case "allow":
		if len(parts) < 2 {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyPermissionsUsageAllow))
			return nil
		}
		return c.addPermission(ctx, "allow", parts[1])
	case "deny":
		if len(parts) < 2 {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyPermissionsUsageDeny))
			return nil
		}
		return c.addPermission(ctx, "deny", parts[1])
	default:
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyPermissionsUsage))
		return nil
	}
}

func (c *permissionsCmd) settingsPath(ctx *Context) string {
	cwd := ctx.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return filepath.Join(cwd, brand.ConfigDirName, "settings.json")
}

func (c *permissionsCmd) readAllSettings(path string, languages ...i18n.Language) (map[string]interface{}, error) {
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

func (c *permissionsCmd) writeAllSettings(path string, m map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (c *permissionsCmd) parseEntry(m map[string]interface{}) permissionsEntry {
	var pe permissionsEntry
	if raw, ok := m["permissions"]; ok {
		if data, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(data, &pe)
		}
	}
	return pe
}

func (c *permissionsCmd) list(ctx *Context) error {
	path := c.settingsPath(ctx)
	m, err := c.readAllSettings(path, ctx.Language)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyAdminReadSettingsError, err))
		return nil
	}
	pe := c.parseEntry(m)

	var sb strings.Builder
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyPermissionsTitle))
	sb.WriteString(strings.Repeat("─", 42) + "\n")

	if len(pe.Allow) == 0 && len(pe.Deny) == 0 {
		sb.WriteString(i18n.Text(ctx.Language, i18n.KeyPermissionsNone))
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyPermissionsEdit, path))
	} else {
		if len(pe.Allow) > 0 {
			sb.WriteString(i18n.Text(ctx.Language, i18n.KeyPermissionsAllowed))
			for _, t := range pe.Allow {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyPermissionsAllowItem, t))
			}
		}
		if len(pe.Deny) > 0 {
			sb.WriteString(i18n.Text(ctx.Language, i18n.KeyPermissionsDenied))
			for _, t := range pe.Deny {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyPermissionsDenyItem, t))
			}
		}
	}
	ctx.OnEvent(sb.String())
	return nil
}

func (c *permissionsCmd) addPermission(ctx *Context, kind, tool string) error {
	path := c.settingsPath(ctx)
	m, err := c.readAllSettings(path, ctx.Language)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyAdminReadSettingsError, err))
		return nil
	}

	pe := c.parseEntry(m)

	// Remove from both lists first — a tool should only appear in one.
	pe.Allow = removePermEntry(pe.Allow, tool)
	pe.Deny = removePermEntry(pe.Deny, tool)

	if kind == "allow" {
		pe.Allow = append(pe.Allow, tool)
	} else {
		pe.Deny = append(pe.Deny, tool)
	}

	m["permissions"] = pe
	if err := c.writeAllSettings(path, m); err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyAdminWriteSettingsError, err))
		return nil
	}
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyPermissionsUpdated, tool, kind, path))
	return nil
}

// removePermEntry returns a copy of slice with all occurrences of val removed.
func removePermEntry(slice []string, val string) []string {
	out := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != val {
			out = append(out, s)
		}
	}
	return out
}
