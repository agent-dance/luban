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
	writeFile(t, filepath.Join(project, "CLAUDE.md"), "project")
	writeFile(t, filepath.Join(extra, "CLAUDE.md"), "extra")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(tmp, "claude-user"))
	t.Setenv("DEEPSEEK_CODE_CONFIG_DIR", filepath.Join(tmp, "deepseek-user"))
	t.Setenv("LUBAN_CODE_CONFIG_DIR", filepath.Join(tmp, "luban-user"))
	t.Setenv("USER_TYPE", "ant")
	t.Setenv("CLAUDE_CODE_MANAGED_SETTINGS_PATH", filepath.Join(tmp, "managed"))

	tests := []struct {
		name     string
		settings PromptSettings
		want     []string
	}{
		{
			name:     "default auto discovery",
			settings: PromptSettings{},
			want:     []string{"project"},
		},
		{
			name:     "disable claude mds hard off",
			settings: PromptSettings{DisableClaudeMds: true, AdditionalDirectories: []string{extra}, AdditionalDirectoriesClaudeMd: true},
			want:     nil,
		},
		{
			name:     "bare skips auto discovery without explicit additional directories",
			settings: PromptSettings{BareMode: true},
			want:     nil,
		},
		{
			name:     "bare preserves explicit additional directories when enabled",
			settings: PromptSettings{BareMode: true, AdditionalDirectories: []string{extra}, AdditionalDirectoriesClaudeMd: true},
			want:     []string{"extra"},
		},
		{
			name:     "additional directories gate defaults closed",
			settings: PromptSettings{AdditionalDirectories: []string{extra}},
			want:     []string{"project"},
		},
		{
			name:     "additional directories gate appends explicit memory",
			settings: PromptSettings{AdditionalDirectories: []string{extra}, AdditionalDirectoriesClaudeMd: true},
			want:     []string{"project", "extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := DiscoverMemoryFilesWithSettings(project, tt.settings)
			got := memoryContents(files)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("memory contents mismatch\nwant %q\ngot  %q", tt.want, got)
			}
		})
	}
}

func TestDefaultPromptSettingsEnvGates(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		assert func(t *testing.T, got PromptSettings)
	}{
		{
			name: "original disable env",
			env:  map[string]string{"CLAUDE_CODE_DISABLE_CLAUDE_MDS": "true"},
			assert: func(t *testing.T, got PromptSettings) {
				if !got.DisableClaudeMds {
					t.Fatal("DisableClaudeMds = false, want true")
				}
			},
		},
		{
			name: "legacy DeepSeek disable env",
			env:  map[string]string{"DEEPSEEK_CODE_DISABLE_CLAUDE_MDS": "1"},
			assert: func(t *testing.T, got PromptSettings) {
				if !got.DisableClaudeMds {
					t.Fatal("DisableClaudeMds = false, want true")
				}
			},
		},
		{
			name: "LUBAN disable env",
			env:  map[string]string{"LUBAN_CODE_DISABLE_CLAUDE_MDS": "1"},
			assert: func(t *testing.T, got PromptSettings) {
				if !got.DisableClaudeMds {
					t.Fatal("DisableClaudeMds = false, want true")
				}
			},
		},
		{
			name: "bare and additional directories env",
			env: map[string]string{
				"CLAUDE_CODE_BARE": "yes",
				"CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD": "on",
			},
			assert: func(t *testing.T, got PromptSettings) {
				if !got.BareMode || !got.AdditionalDirectoriesClaudeMd {
					t.Fatalf("bare/additional env not honored: %#v", got)
				}
			},
		},
		{
			name: "language and output style env",
			env: map[string]string{
				"CLAUDE_CODE_LANGUAGE":     "spanish",
				"CLAUDE_CODE_OUTPUT_STYLE": "concise",
			},
			assert: func(t *testing.T, got PromptSettings) {
				if got.Language != "spanish" || got.OutputStyle != "concise" {
					t.Fatalf("runtime settings env mismatch: %#v", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			tt.assert(t, defaultPromptSettings())
		})
	}
}

func TestDefaultPromptSettingsPrefersLUBANEnv(t *testing.T) {
	for _, key := range []string{
		"LUBAN_CODE_LANGUAGE", "DEEPSEEK_CODE_LANGUAGE", "CLAUDE_CODE_LANGUAGE",
		"LUBAN_CODE_OUTPUT_STYLE", "DEEPSEEK_CODE_OUTPUT_STYLE", "CLAUDE_CODE_OUTPUT_STYLE",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("CLAUDE_CODE_LANGUAGE", "claude-language")
	t.Setenv("DEEPSEEK_CODE_LANGUAGE", "deepseek-language")
	t.Setenv("LUBAN_CODE_LANGUAGE", "luban-language")
	t.Setenv("CLAUDE_CODE_OUTPUT_STYLE", "claude-style")
	t.Setenv("DEEPSEEK_CODE_OUTPUT_STYLE", "deepseek-style")
	t.Setenv("LUBAN_CODE_OUTPUT_STYLE", "luban-style")

	got := defaultPromptSettings()
	if got.Language != "luban-language" || got.OutputStyle != "luban-style" {
		t.Fatalf("LUBAN env should win, got %#v", got)
	}
}

func TestClaudeMdExcludesPatternHandling(t *testing.T) {
	tmp := t.TempDir()
	managed := filepath.Join(tmp, "managed")
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(managed, "CLAUDE.md"), "managed")
	writeFile(t, filepath.Join(user, "CLAUDE.md"), "user")
	writeFile(t, filepath.Join(project, "CLAUDE.md"), "project")
	writeFile(t, filepath.Join(project, ".claude", "rules", "skip.md"), "rule")
	writeFile(t, filepath.Join(project, "CLAUDE.local.md"), "local")

	settings := PromptSettings{
		ClaudeMdExcludes: []string{
			filepath.ToSlash(filepath.Join(user, "CLAUDE.md")),
			"**/.claude/rules/**",
			"**/CLAUDE.local.md",
		},
	}
	files := discoverMemoryFiles(project, memoryPaths{managedDir: managed, userDir: user}, settings)
	got := strings.Join(memoryContents(files), "|")
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

func TestConditionalMemoryDiscoveryRespectsSettingsGates(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, ".claude", "rules", "go.md"), "---\npaths: *.go\n---\nrule")

	disabled := DiscoverMemoryFilesForTargetWithSettings(project, filepath.Join(project, "main.go"), PromptSettings{DisableClaudeMds: true})
	if len(disabled) != 0 {
		t.Fatalf("disabled discovery should return no conditional rules, got %#v", disabled)
	}

	bare := DiscoverMemoryFilesForTargetWithSettings(project, filepath.Join(project, "main.go"), PromptSettings{BareMode: true})
	if len(bare) != 0 {
		t.Fatalf("bare discovery should skip auto conditional rules, got %#v", bare)
	}
}

func TestRuntimeSettingsSectionFromConfig(t *testing.T) {
	got := BuildSystemPrompt(nil, Config{
		Language:    "japanese",
		OutputStyle: "concise",
	})
	assertInOrder(t, got, []string{
		"# Runtime settings",
		"Respond in japanese unless the user explicitly asks for another language.",
		"Use the concise output style for assistant responses.",
	})
}
