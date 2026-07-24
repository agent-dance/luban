package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestInitCreatesLUBANProjectFilesWithDeepSeekProviderDefault(t *testing.T) {
	cwd := t.TempDir()
	var output strings.Builder
	ctx := &Context{CWD: cwd, OnEvent: func(s string) { output.WriteString(s) }}

	cmd := &initCmd{}
	if err := cmd.Execute(ctx, ""); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got := cmd.Description(); got != i18n.Text(i18n.LangEN, i18n.KeyCommandInitDescription) {
		t.Fatalf("Description() = %q", got)
	}
	instructionsPath := filepath.Join(cwd, "LUBAN.md")
	instructions, err := os.ReadFile(instructionsPath)
	if err != nil {
		t.Fatalf("read LUBAN.md: %v", err)
	}
	if !strings.Contains(string(instructions), "LUBAN Code") || strings.Contains(string(instructions), "DeepSeek Code") {
		t.Fatalf("unexpected LUBAN.md contents:\n%s", instructions)
	}

	settingsPath := filepath.Join(cwd, ".luban-code", "settings.json")
	settingsData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read .luban-code/settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	if settings["provider"] != "deepseek" {
		t.Fatalf("provider = %#v, want deepseek", settings["provider"])
	}
	if _, err := os.Stat(filepath.Join(cwd, ".deepseek-code")); !os.IsNotExist(err) {
		t.Fatalf("/init must not create legacy .deepseek-code, stat err = %v", err)
	}
	for _, want := range []string{".luban-code/", "LUBAN.md", filepath.Join(".luban-code", "settings.json")} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}

func TestResolveInstructionsFilePrefersLUBANAndSupportsLegacyFiles(t *testing.T) {
	tests := []struct {
		name       string
		files      []string
		want       string
		wantCreate bool
		migrated   bool
	}{
		{name: "LUBAN wins", files: []string{"CLAUDE.md", "AGENTS.md", "DEEPSEEK.md", "LUBAN.md"}, want: "LUBAN.md"},
		{name: "DeepSeek legacy migrates", files: []string{"CLAUDE.md", "AGENTS.md", "DEEPSEEK.md"}, want: "LUBAN.md", wantCreate: true, migrated: true},
		{name: "agents", files: []string{"CLAUDE.md", "AGENTS.md"}, want: "AGENTS.md"},
		{name: "Claude legacy", files: []string{"CLAUDE.md"}, want: "CLAUDE.md"},
		{name: "create LUBAN", want: "LUBAN.md", wantCreate: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cwd := t.TempDir()
			for _, name := range tt.files {
				if err := os.WriteFile(filepath.Join(cwd, name), []byte(name), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got, created, err := resolveInstructionsFile(cwd)
			if err != nil {
				t.Fatalf("resolveInstructionsFile: %v", err)
			}
			if filepath.Base(got) != tt.want || created != tt.wantCreate {
				t.Fatalf("got (%q, %t), want (%q, %t)", got, created, tt.want, tt.wantCreate)
			}
			if tt.wantCreate {
				data, readErr := os.ReadFile(got)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if tt.migrated {
					if string(data) != "DEEPSEEK.md" {
						t.Fatalf("migrated instructions = %q, want legacy contents", data)
					}
				} else if !strings.Contains(string(data), "LUBAN Code") || strings.Contains(string(data), "DeepSeek Code") {
					t.Fatalf("unexpected created instructions:\n%s", data)
				}
			}
		})
	}
}

func TestCommandDescriptionsAndMCPHintUseLUBANBrand(t *testing.T) {
	if got := (&memoryCmd{}).Description(); got != "Edit LUBAN Code instruction files" {
		t.Fatalf("memory description = %q", got)
	}
	if got := (&configCmd{}).Description(); got != "Show or edit LUBAN Code settings" {
		t.Fatalf("config description = %q", got)
	}

	output := runMCPCommand(t, &mcpCmd{backend: newFakeMCPBackend()}, "")
	if !strings.Contains(output, "`luban-code mcp add-json`") || strings.Contains(output, "deepseek-code") {
		t.Fatalf("unexpected MCP empty-state hint:\n%s", output)
	}
}
