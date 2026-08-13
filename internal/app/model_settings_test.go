package app

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
		"LUBAN_CODE_USE_BEDROCK",
		"LUBAN_CODE_USE_VERTEX",
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

func TestLoadStartupModelSettingsReadsExplicitMaxTokens(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(cwd, brand.ConfigDirName, "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"provider":"deepseek","model":"deepseek-v4-flash","maxTokens":32000}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStartupModelSettings(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxTokens != 32000 {
		t.Fatalf("MaxTokens = %d, want explicit 32000", got.MaxTokens)
	}
}

func TestLoadStartupModelSettingsProgressiveControlPlane(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	userPath := filepath.Join(home, brand.ConfigDirName, "settings.json")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte(`{"progressiveContext":{"enabled":true,"shadow":true,"rolloutPercent":25,"providerAllowlist":["openai-responses"],"modelAllowlist":["gpt-5.6-sol"],"minTokenSavings":3000,"cacheRecoveryRequests":3,"maxProjectedTools":8}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStartupModelSettings(cwd)
	if err != nil {
		t.Fatal(err)
	}
	config := got.ProgressiveContext
	if !got.ProgressiveContextSet || !config.Enabled || !config.Shadow || config.RolloutPercent != 25 || config.MinTokenSavings != 3000 || config.CacheRecoveryRequests != 3 || config.MaxProjectedTools != 8 || config.MaxProjectedTokens == 0 {
		t.Fatalf("progressive settings = %+v", got)
	}
}

func TestLoadStartupModelSettingsDefaultsToReviewedProgressiveScope(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := loadStartupModelSettings(cwd)
	if err != nil {
		t.Fatal(err)
	}
	config := got.ProgressiveContext
	if got.ProgressiveContextSet || !config.Enabled || config.RolloutPercent != 100 ||
		len(config.ProviderAllowlist) != 2 || config.ProviderAllowlist[0] != "openai" || config.ProviderAllowlist[1] != "deepseek" ||
		len(config.ModelAllowlist) != 2 || config.ModelAllowlist[0] != "gpt-5.6-sol" || config.ModelAllowlist[1] != "deepseek-v4-flash" ||
		len(config.ProviderModelAllowlist) != 2 || config.ProviderModelAllowlist[0] != "openai/gpt-5.6-sol" || config.ProviderModelAllowlist[1] != "deepseek/deepseek-v4-flash" ||
		len(config.ToolAllowlist) != 1 || config.ToolAllowlist[0] != "Inspect" {
		t.Fatalf("production progressive default = %+v", got)
	}
}

func TestLoadStartupModelSettingsPreservesExplicitProgressiveDisable(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	userPath := filepath.Join(home, brand.ConfigDirName, "settings.json")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte(`{"progressiveContext":{"enabled":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStartupModelSettings(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ProgressiveContextSet || got.ProgressiveContext.Enabled {
		t.Fatalf("explicit progressive disable was not preserved: %+v", got)
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

	userPath := filepath.Join(home, ".luban-code", "settings.json")
	if err := os.MkdirAll(filepath.Dir(userPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte(`{"provider":"openai","model":"gpt-user"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	projectPath := filepath.Join(cwd, ".luban-code", "settings.json")
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

	localPath := filepath.Join(cwd, ".luban-code", "settings.local.json")
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

func TestSaveUserModelSettingsWritesCanonicalConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := saveUserModelSettings("openai", "gpt-luban"); err != nil {
		t.Fatalf("saveUserModelSettings: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, brand.ConfigDirName, "settings.json")); err != nil {
		t.Fatalf("expected LUBAN settings file: %v", err)
	}
}

func TestSaveModelSettingsAtPreservesExistingSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".luban-code", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := saveModelSettingsAt(path, "openai", "gpt-5.4"); err != nil {
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
	catalog.Register(provider.ModelInfo{CostCurrency: "USD",
		Provider:         "openai",
		ID:               "gpt-5.5",
		ReasoningEfforts: []string{"low", "medium", "high", "xhigh"},
	})
	catalog.Register(provider.ModelInfo{CostCurrency: "USD",
		Provider:               "deepseek",
		ID:                     "deepseek-v4-flash",
		ReasoningEfforts:       []string{"low", "high", "max"},
		DefaultReasoningEffort: "high",
	})

	tests := []struct {
		name         string
		explicit     string
		settings     startupModelSettings
		providerName string
		modelID      string
		want         string
	}{
		{name: "environment wins", explicit: "xhigh", settings: startupModelSettings{Provider: "openai", Model: "gpt-5.5", ReasoningEffort: "high"}, providerName: "openai", modelID: "gpt-5.5", want: "xhigh"},
		{name: "saved selection", settings: startupModelSettings{Provider: "openai", Model: "gpt-5.5", ReasoningEffort: "high"}, providerName: "openai", modelID: "gpt-5.5", want: "high"},
		{name: "catalog default", providerName: "openai", modelID: "gpt-5.5", want: "medium"},
		{name: "mismatched saved model uses catalog default", settings: startupModelSettings{Provider: "openai", Model: "other", ReasoningEffort: "high"}, providerName: "openai", modelID: "gpt-5.5", want: "medium"},
		{name: "provider catalog default", providerName: "deepseek", modelID: "deepseek-v4-flash", want: "high"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveStartupReasoningEffort(test.explicit, test.settings, test.providerName, test.modelID, catalog)
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
