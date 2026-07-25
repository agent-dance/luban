package prompt

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryDiscoverySettingsGates(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	extra := filepath.Join(tmp, "extra")
	writeFile(t, filepath.Join(project, "LUBAN.md"), "project")
	writeFile(t, filepath.Join(extra, "LUBAN.md"), "extra")
	t.Setenv("LUBAN_CODE_CONFIG_DIR", filepath.Join(tmp, "luban-user"))
	t.Setenv("USER_TYPE", "ant")
	t.Setenv("LUBAN_CODE_MANAGED_SETTINGS_PATH", filepath.Join(tmp, "managed"))

	tests := []struct {
		name     string
		settings PromptSettings
		want     []string
	}{
		{name: "default auto discovery", settings: PromptSettings{}, want: []string{"project"}},
		{
			name: "disable instructions hard off",
			settings: PromptSettings{
				DisableInstructions: true, AdditionalDirectories: []string{extra},
				AdditionalDirectoryInstructions: true,
			},
			want: nil,
		},
		{name: "bare skips auto discovery", settings: PromptSettings{BareMode: true}, want: nil},
		{
			name: "bare preserves explicit additional directories",
			settings: PromptSettings{
				BareMode: true, AdditionalDirectories: []string{extra},
				AdditionalDirectoryInstructions: true,
			},
			want: []string{"extra"},
		},
		{
			name:     "additional directories default closed",
			settings: PromptSettings{AdditionalDirectories: []string{extra}},
			want:     []string{"project"},
		},
		{
			name: "additional directories enabled",
			settings: PromptSettings{
				AdditionalDirectories: []string{extra}, AdditionalDirectoryInstructions: true,
			},
			want: []string{"project", "extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := memoryContents(DiscoverMemoryFilesWithSettings(project, tt.settings))
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("memory contents mismatch\nwant %q\ngot  %q", tt.want, got)
			}
		})
	}
}

func TestDefaultPromptSettingsCanonicalEnv(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_INSTRUCTIONS", "true")
	t.Setenv("LUBAN_CODE_BARE", "yes")
	t.Setenv("LUBAN_CODE_ADDITIONAL_DIRECTORY_INSTRUCTIONS", "on")
	t.Setenv("LUBAN_CODE_LANGUAGE", "spanish")
	t.Setenv("LUBAN_CODE_OUTPUT_STYLE", "concise")

	got := defaultPromptSettings()
	if !got.DisableInstructions || !got.BareMode || !got.AdditionalDirectoryInstructions {
		t.Fatalf("prompt gates not loaded: %#v", got)
	}
	if got.Language != "spanish" || got.OutputStyle != "concise" {
		t.Fatalf("runtime settings env mismatch: %#v", got)
	}
}

func TestInstructionExcludesPatternHandling(t *testing.T) {
	tmp := t.TempDir()
	managed := filepath.Join(tmp, "managed")
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(managed, "LUBAN.md"), "managed")
	writeFile(t, filepath.Join(user, "LUBAN.md"), "user")
	writeFile(t, filepath.Join(project, "LUBAN.md"), "project")
	writeFile(t, filepath.Join(project, ".luban-code", "rules", "skip.md"), "rule")
	writeFile(t, filepath.Join(project, "LUBAN.local.md"), "local")

	settings := PromptSettings{InstructionExcludes: []string{
		filepath.ToSlash(filepath.Join(user, "LUBAN.md")),
		"**/.luban-code/rules/**",
		"**/LUBAN.local.md",
	}}
	got := strings.Join(memoryContents(discoverMemoryFiles(project, memoryPaths{managedDir: managed, userDir: user}, settings)), "|")
	for _, notWant := range []string{"user", "rule", "local"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("excluded memory %q was loaded in %q", notWant, got)
		}
	}
	for _, want := range []string{"managed", "project"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q to remain loadable in %q", want, got)
		}
	}
}

func TestRuntimeSettingsSectionFromConfig(t *testing.T) {
	got := BuildSystemPrompt(nil, Config{Language: "japanese", OutputStyle: "concise"})
	assertInOrder(t, got, []string{
		"# Runtime settings",
		"Respond in japanese unless the user explicitly asks for another language.",
		"Use the concise output style for assistant responses.",
	})
}
