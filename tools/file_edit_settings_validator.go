// Package tools — file_edit_settings_validator.go is the Go counterpart of
// src/utils/settings/validateEditTool.ts. It refuses edits to a recognised
// .luban-code or legacy settings.json that would corrupt the JSON
// structure (parse failure), strip required keys, or introduce keys not
// recognised by the runtime.
//
// We don't recreate TS's full schema validation — that lives in the runtime's
// own settings store. We enforce just the contract that the alignment tests
// codify: the top-level value must be an object, the `permissions` block must
// be present, and only a fixed set of top-level keys is allowed.
package tools

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
)

// claudeSettingsAllowedTopLevelKeys lists the top-level keys recognised by
// the LUBAN Code or legacy product settings file. Anything else is treated as
// `additionalProperties:false` and rejected.
var claudeSettingsAllowedTopLevelKeys = map[string]struct{}{
	"$schema":                       {},
	"apiKeyHelper":                  {},
	"autoUpdaterStatus":             {},
	"cleanupPeriodDays":             {},
	"env":                           {},
	"hooks":                         {},
	"includeCoAuthoredBy":           {},
	"model":                         {},
	"permissions":                   {},
	"theme":                         {},
	"tipsHistory":                   {},
	"projects":                      {},
	"skillOverrides":                {},
	"verbose":                       {},
	"forceLoginMethod":              {},
	"enableAllProjectMcpServers":    {},
	"enabledMcpjsonServers":         {},
	"disabledMcpjsonServers":        {},
	"allowedTools":                  {},
	"deniedTools":                   {},
	"oauthAccount":                  {},
	"shiftEnterKeyBindingInstalled": {},
}

// claudeSettingsRequiredTopLevelKeys lists the keys the runtime expects to
// be present on every settings file. Mirrors the schema's `required` set.
var claudeSettingsRequiredTopLevelKeys = []string{"permissions"}

// validateSettingsEdit validates a proposed edit to a settings file. It
// returns:
//
//   - ("", nil) when the path is not a settings file or the validation
//     passes
//   - ("warning text", nil) when the edit is allowed but the validator
//     wants to surface advisory text in the result payload
//   - ("", error) when the edit must be rejected
//
// originalContent (optional, variadic) — the file's content before the
// edit. Used so we don't reject edits that simply preserve a
// pre-existing schema deviation (e.g. older settings files without a
// permissions block). When omitted, only structural checks are
// performed (parse + top-level-object); schema-rule enforcement
// (required keys, additionalProperties) is skipped because there is no
// baseline to diff against.
func validateSettingsEdit(absPath, updatedContent string, originalContent ...string) (string, error) {
	if !isClaudeSettingsPath(absPath) {
		return "", nil
	}
	// Empty file is fine — settings.json is optional.
	trimmed := strings.TrimSpace(updatedContent)
	if trimmed == "" {
		return "", nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(updatedContent), &parsed); err != nil {
		return "", i18n.WrapError(i18n.KeyToolFileHelperSettingsInvalidJSON, err)
	}
	obj, ok := parsed.(map[string]any)
	if !ok {
		return "", i18n.NewError(i18n.KeyToolFileHelperSettingsTopLevelObject, parsed)
	}
	if skillOverrides, exists := obj["skillOverrides"]; exists {
		if err := validateSkillOverridesSettings(skillOverrides); err != nil {
			return "", i18n.WrapError(i18n.KeyToolFileHelperSettingsSkillOverrides, err)
		}
	}

	// When called without an original baseline (the 2-arg form used by
	// direct conformance tests), structural and skillOverrides checks are all
	// we can do. Diff-driven rules below require the original.
	if len(originalContent) == 0 {
		return "", nil
	}
	origRaw := originalContent[0]

	// Decode the original (best-effort) so we can compare key sets. If the
	// original was missing/invalid, treat it as "anything goes" — we don't
	// punish edits to files that already deviated from the schema.
	original := map[string]any{}
	if origRaw != "" {
		if err := json.Unmarshal([]byte(origRaw), &original); err != nil {
			return "", nil
		}
	}

	// Required-keys: only reject if the original HAD the key and the edit
	// removed it.
	for _, key := range claudeSettingsRequiredTopLevelKeys {
		if _, hadOriginal := original[key]; hadOriginal {
			if _, hasNew := obj[key]; !hasNew {
				return "", i18n.NewError(i18n.KeyToolFileHelperSettingsMissingKey, key)
			}
		}
	}

	// AdditionalProperties:false at the top level — only flag NEWLY-introduced
	// unknown keys. Preserving an existing unknown key (legacy files) does
	// not need to fail.
	for k := range obj {
		if _, allowed := claudeSettingsAllowedTopLevelKeys[k]; allowed {
			continue
		}
		if _, hadOriginal := original[k]; hadOriginal {
			continue
		}
		return "", i18n.NewError(i18n.KeyToolFileHelperSettingsUnknownKey, k)
	}
	return "", nil
}

// validateSkillOverridesSettings validates the nested settings wire shape
// before FileEdit writes it. Persistent settings are keyed only by stable
// SkillID; command and UI selectors must be resolved before this boundary.
func validateSkillOverridesSettings(value any) error {
	overrides, ok := value.(map[string]any)
	if !ok || overrides == nil {
		return i18n.NewError(i18n.KeyToolFileHelperSkillOverridesObject, value)
	}

	ids := make([]string, 0, len(overrides))
	for rawID := range overrides {
		ids = append(ids, rawID)
	}
	sort.Strings(ids)
	for _, rawID := range ids {
		id := skills.SkillID(rawID)
		if err := id.Validate(); err != nil {
			return i18n.WrapError(i18n.KeyToolFileHelperSkillOverrideKey, err, rawID)
		}
		if err := validateSkillOverrideSettingsRecord(overrides[rawID]); err != nil {
			return i18n.WrapError(i18n.KeyToolFileHelperSkillOverrideRecord, err, id)
		}
	}
	return nil
}

func validateSkillOverrideSettingsRecord(value any) error {
	if scalar, ok := value.(string); ok {
		return skills.Visibility(scalar).Validate()
	}

	record, ok := value.(map[string]any)
	if !ok || record == nil {
		return i18n.NewError(i18n.KeyToolFileHelperSkillOverrideShape, value)
	}
	fields := make([]string, 0, len(record))
	for field := range record {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		switch field {
		case "visibility", "last_non_off":
		default:
			return i18n.NewError(i18n.KeyToolFileHelperSkillOverrideField, field)
		}
	}

	rawVisibility, exists := record["visibility"]
	if !exists {
		return i18n.NewError(i18n.KeyToolFileHelperSkillOverrideMissingField, "visibility")
	}
	visibilityText, ok := rawVisibility.(string)
	if !ok {
		return i18n.NewError(i18n.KeyToolFileHelperSkillOverrideStringField, "visibility", rawVisibility)
	}
	visibility := skills.Visibility(visibilityText)
	if err := visibility.Validate(); err != nil {
		return err
	}

	rawLastNonOff, exists := record["last_non_off"]
	if !exists {
		return nil
	}
	lastNonOffText, ok := rawLastNonOff.(string)
	if !ok {
		return i18n.NewError(i18n.KeyToolFileHelperSkillOverrideStringField, "last_non_off", rawLastNonOff)
	}
	lastNonOff := skills.Visibility(lastNonOffText)
	if visibility != skills.VisibilityOff || !lastNonOff.IsNonOff() {
		return i18n.WrapError(i18n.KeyToolFileHelperSkillOverrideLastNonOff, skills.ErrInvalidVisibility)
	}
	return nil
}

// isClaudeSettingsPath reports whether absPath looks like a settings.json
// produced by LUBAN Code or either legacy product config layout
// config dir layout. Mirrors src/utils/permissions/filesystem.ts
// isClaudeSettingsPath.
func isClaudeSettingsPath(absPath string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(absPath))
	base := filepath.Base(cleaned)
	if base != "settings.json" && base != "settings.local.json" {
		return false
	}
	dir := filepath.Base(filepath.Dir(cleaned))
	switch dir {
	case ".claude", ".deepseek-code", ".luban-code":
		return true
	}
	return false
}
