package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const task12SkillID = "skill:project:task12-skill"

func TestSkillOverridesSettingsAcceptsLocalScalarAndStructuredRecords(t *testing.T) {
	valid := []struct {
		name      string
		overrides string
	}{
		{name: "empty", overrides: `{}`},
		{name: "auto scalar", overrides: fmt.Sprintf(`{%q:"auto"}`, task12SkillID)},
		{name: "name-only scalar", overrides: fmt.Sprintf(`{%q:"name-only"}`, task12SkillID)},
		{name: "manual-only scalar", overrides: fmt.Sprintf(`{%q:"manual-only"}`, task12SkillID)},
		{name: "off scalar", overrides: fmt.Sprintf(`{%q:"off"}`, task12SkillID)},
		{name: "structured auto", overrides: fmt.Sprintf(`{%q:{"visibility":"auto"}}`, task12SkillID)},
		{name: "structured off without history", overrides: fmt.Sprintf(`{%q:{"visibility":"off"}}`, task12SkillID)},
		{name: "structured off remembers auto", overrides: fmt.Sprintf(`{%q:{"visibility":"off","last_non_off":"auto"}}`, task12SkillID)},
		{name: "structured off remembers name-only", overrides: fmt.Sprintf(`{%q:{"visibility":"off","last_non_off":"name-only"}}`, task12SkillID)},
		{name: "structured off remembers manual-only", overrides: fmt.Sprintf(`{%q:{"visibility":"off","last_non_off":"manual-only"}}`, task12SkillID)},
		{name: "multiple stable sources", overrides: `{"skill:user:task12-user":"auto","skill:mcp:server/task12":{"visibility":"off"}}`},
	}

	path := filepath.Join(t.TempDir(), ".luban-code", "settings.json")
	original := `{"permissions":{}}`
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			updated := fmt.Sprintf(`{"permissions":{},"skillOverrides":%s}`, test.overrides)
			if _, err := validateSettingsEdit(path, updated); err != nil {
				t.Fatalf("2-arg validation rejected valid settings: %v", err)
			}
			if _, err := validateSettingsEdit(path, updated, original); err != nil {
				t.Fatalf("3-arg validation rejected valid settings: %v", err)
			}
		})
	}
}

func TestSkillOverridesSettingsRejectsMalformedKeysValuesAndRecords(t *testing.T) {
	invalid := []struct {
		name      string
		overrides string
	}{
		{name: "null map", overrides: `null`},
		{name: "array map", overrides: `[]`},
		{name: "scalar map", overrides: `"off"`},
		{name: "name selector", overrides: `{"deploy":"off"}`},
		{name: "qualified selector", overrides: `{"github:review":"off"}`},
		{name: "malformed stable ID", overrides: `{"skill:unknown:review":"off"}`},
		{name: "empty stable identity", overrides: `{"skill:project:":"off"}`},
		{name: "padded stable ID", overrides: `{" skill:project:review":"off"}`},
		{name: "control in stable ID", overrides: `{"skill:project:review\nunsafe":"off"}`},
		{name: "null record", overrides: fmt.Sprintf(`{%q:null}`, task12SkillID)},
		{name: "boolean record", overrides: fmt.Sprintf(`{%q:true}`, task12SkillID)},
		{name: "array record", overrides: fmt.Sprintf(`{%q:[]}`, task12SkillID)},
		{name: "unknown scalar", overrides: fmt.Sprintf(`{%q:"sometimes"}`, task12SkillID)},
		{name: "upstream on alias", overrides: fmt.Sprintf(`{%q:"on"}`, task12SkillID)},
		{name: "upstream user-only alias", overrides: fmt.Sprintf(`{%q:"user-invocable-only"}`, task12SkillID)},
		{name: "missing visibility", overrides: fmt.Sprintf(`{%q:{}}`, task12SkillID)},
		{name: "null visibility", overrides: fmt.Sprintf(`{%q:{"visibility":null}}`, task12SkillID)},
		{name: "non-string visibility", overrides: fmt.Sprintf(`{%q:{"visibility":false}}`, task12SkillID)},
		{name: "unknown visibility", overrides: fmt.Sprintf(`{%q:{"visibility":"sometimes"}}`, task12SkillID)},
		{name: "null last non-off", overrides: fmt.Sprintf(`{%q:{"visibility":"off","last_non_off":null}}`, task12SkillID)},
		{name: "non-string last non-off", overrides: fmt.Sprintf(`{%q:{"visibility":"off","last_non_off":false}}`, task12SkillID)},
		{name: "off remembers off", overrides: fmt.Sprintf(`{%q:{"visibility":"off","last_non_off":"off"}}`, task12SkillID)},
		{name: "off remembers unknown", overrides: fmt.Sprintf(`{%q:{"visibility":"off","last_non_off":"sometimes"}}`, task12SkillID)},
		{name: "non-off has history", overrides: fmt.Sprintf(`{%q:{"visibility":"name-only","last_non_off":"auto"}}`, task12SkillID)},
	}

	path := filepath.Join(t.TempDir(), ".claude", "settings.local.json")
	original := `{"permissions":{}}`
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			updated := fmt.Sprintf(`{"permissions":{},"skillOverrides":%s}`, test.overrides)
			if _, err := validateSettingsEdit(path, updated); err == nil {
				t.Fatal("2-arg validation accepted invalid skillOverrides")
			}
			if _, err := validateSettingsEdit(path, updated, original); err == nil {
				t.Fatal("3-arg validation accepted invalid skillOverrides")
			}
		})
	}
}

func TestSkillOverridesSettingsRejectsUnknownAndManagedFields(t *testing.T) {
	fields := []string{
		"skill_id",
		"scope",
		"managed",
		"managed_deny",
		"mutable",
		"read_only",
		"read_only_reason",
		"source",
		"nested",
	}
	path := filepath.Join(t.TempDir(), ".deepseek-code", "settings.json")
	original := `{"permissions":{}}`
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			updated := fmt.Sprintf(`{"permissions":{},"skillOverrides":{%q:{"visibility":"off",%q:true}}}`, task12SkillID, field)
			if _, err := validateSettingsEdit(path, updated); err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("2-arg field %q rejection = %v", field, err)
			}
			if _, err := validateSettingsEdit(path, updated, original); err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("3-arg field %q rejection = %v", field, err)
			}
		})
	}
}

func TestSkillOverridesSettingsPreservesExistingTopLevelValidationBehavior(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".luban-code", "settings.json")
	original := `{"permissions":{},"legacyUnknown":true}`
	valid := fmt.Sprintf(`{"permissions":{},"legacyUnknown":true,"skillOverrides":{%q:"off"}}`, task12SkillID)
	if _, err := validateSettingsEdit(path, valid, original); err != nil {
		t.Fatalf("valid skillOverrides weakened legacy-key preservation: %v", err)
	}

	newUnknown := fmt.Sprintf(`{"permissions":{},"skillOverrides":{%q:"off"},"newUnknown":true}`, task12SkillID)
	if _, err := validateSettingsEdit(path, newUnknown, `{"permissions":{}}`); err == nil {
		t.Fatal("skillOverrides weakened top-level additionalProperties validation")
	}
}

func TestSkillOverridesSettingsFileEditFailsBeforeWrite(t *testing.T) {
	directory := t.TempDir()
	settingsDirectory := filepath.Join(directory, ".luban-code")
	if err := os.MkdirAll(settingsDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(settingsDirectory, "settings.json")
	original := `{"permissions":{}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	state := NewReadFileState()
	recordStrongReadEvidenceForTest(t, state, path)
	tool := &FileEditTool{ReadState: state}
	invalid := `{"permissions":{},"skillOverrides":{"deploy":"off"}}`
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path":   path,
		"old_string":  original,
		"new_string":  invalid,
		"replace_all": false,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "skillOverrides") {
		t.Fatalf("invalid edit result = %#v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("settings changed before validation rejection: %q", got)
	}
}

func TestSkillOverridesSettingsFileEditAllowsValidRecord(t *testing.T) {
	directory := t.TempDir()
	settingsDirectory := filepath.Join(directory, ".luban-code")
	if err := os.MkdirAll(settingsDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(settingsDirectory, "settings.json")
	original := `{"permissions":{}}`
	updated := fmt.Sprintf(`{"permissions":{},"skillOverrides":{%q:{"visibility":"off","last_non_off":"manual-only"}}}`, task12SkillID)
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	state := NewReadFileState()
	recordStrongReadEvidenceForTest(t, state, path)
	tool := &FileEditTool{ReadState: state}
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path":   path,
		"old_string":  original,
		"new_string":  updated,
		"replace_all": false,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("valid edit rejected: %#v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != updated {
		t.Fatalf("valid settings edit = %q, want %q", got, updated)
	}
}
