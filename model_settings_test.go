package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/provider"
)

func clearModelSelectionEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PROVIDER",
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
		"DEEPSEEK_API_KEY",
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"GEMINI_API_KEY",
		"GROQ_API_KEY",
		"XAI_API_KEY",
		"MISTRAL_API_KEY",
		"ZHIPU_API_KEY",
		"MINIMAX_API_KEY",
		"MOONSHOT_API_KEY",
		"OAUTH_ACCESS_TOKEN",
		"CLAUDE_MODEL",
		"ANTHROPIC_MODEL",
		"OPENAI_MODEL",
		"BEDROCK_MODEL",
		"VERTEX_MODEL",
		"OLLAMA_MODEL",
		"DEEPSEEK_MODEL",
		"GEMINI_MODEL",
		"GROQ_MODEL",
		"XAI_MODEL",
		"MISTRAL_MODEL",
		"ZHIPU_MODEL",
		"MINIMAX_MODEL",
		"KIMI_MODEL",
		"LUBAN_CODE_CACHE_ROUTING_MODE",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadStartupModelSettingsOverlaysCacheRoutingModeWithoutClearingModel(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	userPath := filepath.Join(home, brand.ConfigDirName, "settings.json")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte(`{"provider":"openai","model":"gpt-user","reasoningEffort":"high"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(cwd, brand.ConfigDirName, "settings.json")
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte(`{"cacheRoutingMode":"off"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStartupModelSettings(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "openai" || got.Model != "gpt-user" || got.ReasoningEffort != "high" || got.CacheRoutingMode != "off" || got.Source != projectPath {
		t.Fatalf("merged settings = %+v", got)
	}

	clearModelSelectionEnv(t)
	applyStartupModelSettings(&cli.Options{}, got)
	if mode := os.Getenv("LUBAN_CODE_CACHE_ROUTING_MODE"); mode != "off" {
		t.Fatalf("LUBAN_CODE_CACHE_ROUTING_MODE = %q, want off", mode)
	}
}

func TestApplyStartupModelSettingsKeepsExplicitCacheRoutingEnvironment(t *testing.T) {
	clearModelSelectionEnv(t)
	t.Setenv("LUBAN_CODE_CACHE_ROUTING_MODE", "on")
	applyStartupModelSettings(&cli.Options{}, startupModelSettings{CacheRoutingMode: "off"})
	if mode := os.Getenv("LUBAN_CODE_CACHE_ROUTING_MODE"); mode != "on" {
		t.Fatalf("LUBAN_CODE_CACHE_ROUTING_MODE = %q, want explicit on", mode)
	}
}

func TestLoadStartupModelSettingsDoesNotCarryAPIKeyAcrossProviderOverride(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	userPath := filepath.Join(home, brand.ConfigDirName, "settings.json")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte(`{"provider":"openai","apiKey":"openai-secret","reasoningEffort":"high"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(cwd, brand.ConfigDirName, "settings.json")
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte(`{"provider":"anthropic","model":"claude-project","cacheRoutingMode":"off"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStartupModelSettings(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "anthropic" || got.Model != "claude-project" || got.CacheRoutingMode != "off" || got.APIKey != "" || got.ReasoningEffort != "" {
		t.Fatalf("merged settings leaked provider-scoped state: %+v", got)
	}
}

func TestApplyStartupModelSettingsAppliesAPIKeyForEffectiveProvider(t *testing.T) {
	clearModelSelectionEnv(t)
	opts := cli.Options{}

	applyStartupModelSettings(&opts, startupModelSettings{
		Provider: brand.DeepSeekProvider,
		Model:    brand.DeepSeekDefaultModel,
		APIKey:   "sk-settings",
	})

	if opts.Provider != brand.DeepSeekProvider {
		t.Fatalf("Provider = %q, want %s", opts.Provider, brand.DeepSeekProvider)
	}
	if opts.Model != brand.DeepSeekDefaultModel {
		t.Fatalf("Model = %q, want %s", opts.Model, brand.DeepSeekDefaultModel)
	}
	if got := os.Getenv("DEEPSEEK_API_KEY"); got != "sk-settings" {
		t.Fatalf("DEEPSEEK_API_KEY = %q, want sk-settings", got)
	}
}

func TestApplyStartupModelSettingsMapsXAIEnvironment(t *testing.T) {
	clearModelSelectionEnv(t)
	opts := cli.Options{}

	applyStartupModelSettings(&opts, startupModelSettings{
		Provider: "xai",
		Model:    "grok-4.5",
		APIKey:   "xai-settings-key",
	})

	if opts.Provider != "xai" || opts.Model != "grok-4.5" {
		t.Fatalf("options = %+v, want xai/grok-4.5", opts)
	}
	if got := os.Getenv("XAI_API_KEY"); got != "xai-settings-key" {
		t.Fatalf("XAI_API_KEY = %q, want xai-settings-key", got)
	}
}

func TestApplyStartupModelSettingsKeepsXAIModelEnvironment(t *testing.T) {
	clearModelSelectionEnv(t)
	t.Setenv("XAI_MODEL", "grok-env")
	opts := cli.Options{}

	applyStartupModelSettings(&opts, startupModelSettings{
		Provider: "xai",
		Model:    "grok-settings",
	})

	if opts.Provider != "xai" {
		t.Fatalf("Provider = %q, want xai", opts.Provider)
	}
	if opts.Model != "" {
		t.Fatalf("Model override = %q, want empty so XAI_MODEL remains authoritative", opts.Model)
	}
}

func TestApplyStartupModelSettingsUsesSettingsWithoutExplicitOverride(t *testing.T) {
	clearModelSelectionEnv(t)
	opts := cli.Options{}

	applyStartupModelSettings(&opts, startupModelSettings{
		Provider: "openai",
		Model:    "gpt-5.4",
	})

	if opts.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", opts.Provider)
	}
	if opts.Model != "gpt-5.4" {
		t.Fatalf("Model = %q, want gpt-5.4", opts.Model)
	}
}

func TestNormalizeSettingsProviderMapsLegacyAliases(t *testing.T) {
	if got := normalizeSettingsProvider("openai-responses"); got != "openai" {
		t.Fatalf("openai-responses normalized to %q, want openai", got)
	}
	if got := normalizeSettingsProvider("oauth"); got != "anthropic" {
		t.Fatalf("oauth normalized to %q, want anthropic", got)
	}
}

func TestApplyStartupModelSettingsKeepsExplicitEnvProvider(t *testing.T) {
	clearModelSelectionEnv(t)
	t.Setenv("PROVIDER", "anthropic")
	opts := cli.Options{}

	applyStartupModelSettings(&opts, startupModelSettings{
		Provider: "openai",
		Model:    "gpt-5.4",
	})

	if opts.Provider != "" {
		t.Fatalf("Provider override = %q, want empty so PROVIDER env remains authoritative", opts.Provider)
	}
	if opts.Model != "" {
		t.Fatalf("Model override = %q, want empty for mismatched settings provider", opts.Model)
	}
}

func TestApplyStartupModelSettingsKeepsProviderModelEnv(t *testing.T) {
	clearModelSelectionEnv(t)
	t.Setenv("OPENAI_MODEL", "gpt-env")
	opts := cli.Options{}

	applyStartupModelSettings(&opts, startupModelSettings{
		Provider: "openai",
		Model:    "gpt-settings",
	})

	if opts.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", opts.Provider)
	}
	if opts.Model != "" {
		t.Fatalf("Model override = %q, want empty so OPENAI_MODEL remains authoritative", opts.Model)
	}
}

func TestLoadStartupModelSettingsUsesSettingsSourcePriority(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	userPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte(`{"provider":"openai","model":"gpt-user"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	projectPath := filepath.Join(cwd, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStartupModelSettings(cwd)
	if err != nil {
		t.Fatalf("loadStartupModelSettings: %v", err)
	}
	if got.Provider != "openai" || got.Model != "gpt-user" {
		t.Fatalf("settings = %+v, want user provider/model when project has no model settings", got)
	}

	localPath := filepath.Join(cwd, ".claude", "settings.local.json")
	if err := os.WriteFile(localPath, []byte(`{"provider":"anthropic","model":"claude-local"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err = loadStartupModelSettings(cwd)
	if err != nil {
		t.Fatalf("loadStartupModelSettings: %v", err)
	}
	if got.Provider != "anthropic" || got.Model != "claude-local" || got.Source != localPath {
		t.Fatalf("settings = %+v, want local settings to override lower-priority sources", got)
	}
}

func TestLoadStartupModelSettingsPrefersLUBANOverDeepSeekAndClaude(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	files := map[string]string{
		filepath.Join(home, ".claude", "settings.json"):             `{"provider":"openai","model":"claude-user"}`,
		filepath.Join(home, ".deepseek-code", "settings.json"):      `{"provider":"openai","model":"deepseek-user"}`,
		filepath.Join(home, ".luban-code", "settings.json"):         `{"provider":"openai","model":"luban-user"}`,
		filepath.Join(cwd, ".claude", "settings.json"):              `{"provider":"openai","model":"claude-project"}`,
		filepath.Join(cwd, ".deepseek-code", "settings.json"):       `{"provider":"openai","model":"deepseek-project"}`,
		filepath.Join(cwd, ".luban-code", "settings.json"):          `{"provider":"openai","model":"luban-project"}`,
		filepath.Join(cwd, ".deepseek-code", "settings.local.json"): `{"provider":"openai","model":"deepseek-local"}`,
		filepath.Join(cwd, ".luban-code", "settings.local.json"):    `{"provider":"openai","model":"luban-local"}`,
	}
	for path, data := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got, err := loadStartupModelSettings(cwd)
	if err != nil {
		t.Fatalf("loadStartupModelSettings: %v", err)
	}
	wantSource := filepath.Join(cwd, ".luban-code", "settings.local.json")
	if got.Model != "luban-local" || got.Source != wantSource {
		t.Fatalf("settings = %+v, want LUBAN local settings from %s", got, wantSource)
	}
}

func TestSaveUserModelSettingsWritesOnlyLUBANConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := saveUserModelSettings("openai", "gpt-luban"); err != nil {
		t.Fatalf("saveUserModelSettings: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".luban-code", "settings.json")); err != nil {
		t.Fatalf("expected LUBAN settings file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".deepseek-code", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy DeepSeek settings should not be written, err=%v", err)
	}
}

func TestSaveModelSettingsAtPreservesExistingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := saveModelSettingsAt(path, "openai-responses", "gpt-5.4"); err != nil {
		t.Fatalf("saveModelSettingsAt: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "dark" {
		t.Fatalf("theme = %v, want dark", got["theme"])
	}
	if got["provider"] != "openai" {
		t.Fatalf("provider = %v, want openai", got["provider"])
	}
	if got["model"] != "gpt-5.4" {
		t.Fatalf("model = %v, want gpt-5.4", got["model"])
	}
}

func TestSaveModelSettingsAtPersistsAndClearsReasoningEffort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := saveModelSettingsAt(path, "openai", "gpt-5.5", "high"); err != nil {
		t.Fatalf("save reasoning effort: %v", err)
	}

	settings, ok, err := readModelSettingsFile(path)
	if err != nil || !ok {
		t.Fatalf("read saved settings: ok=%v err=%v", ok, err)
	}
	if settings.ReasoningEffort != "high" {
		t.Fatalf("ReasoningEffort = %q, want high", settings.ReasoningEffort)
	}

	if err := saveModelSettingsAt(path, "anthropic", "claude-sonnet-5", ""); err != nil {
		t.Fatalf("clear reasoning effort: %v", err)
	}
	settings, ok, err = readModelSettingsFile(path)
	if err != nil || !ok {
		t.Fatalf("read cleared settings: ok=%v err=%v", ok, err)
	}
	if settings.ReasoningEffort != "" {
		t.Fatalf("ReasoningEffort = %q, want empty", settings.ReasoningEffort)
	}
}

func TestResolveStartupReasoningEffort(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{
		Provider:         "openai",
		ID:               "gpt-5.5",
		ReasoningEfforts: []string{"low", "medium", "high", "xhigh"},
	})

	tests := []struct {
		name     string
		explicit string
		settings startupModelSettings
		want     string
	}{
		{name: "environment wins", explicit: "xhigh", settings: startupModelSettings{Provider: "openai", Model: "gpt-5.5", ReasoningEffort: "high"}, want: "xhigh"},
		{name: "saved selection", settings: startupModelSettings{Provider: "openai", Model: "gpt-5.5", ReasoningEffort: "high"}, want: "high"},
		{name: "catalog default", want: "medium"},
		{name: "mismatched saved model uses catalog default", settings: startupModelSettings{Provider: "openai", Model: "other", ReasoningEffort: "high"}, want: "medium"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveStartupReasoningEffort(test.explicit, test.settings, "openai", "gpt-5.5", catalog)
			if got != test.want {
				t.Fatalf("resolveStartupReasoningEffort() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReadModelSettingsFileIgnoresUnrelatedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, ok, err := readModelSettingsFile(path)
	if err != nil {
		t.Fatalf("readModelSettingsFile: %v", err)
	}
	if ok {
		t.Fatal("expected unrelated settings file to be ignored")
	}
}
