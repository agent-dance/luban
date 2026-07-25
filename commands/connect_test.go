package commands_test

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
)

func TestRegisterBuiltins_RemovesConnect(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if r.Find("connect") != nil {
		t.Fatal("expected /connect to be unregistered")
	}
}

func TestRunProviderOAuthConnect_UsesConnectValidation(t *testing.T) {
	tmpDir := t.TempDir()
	cs, err := provider.NewCredentialStoreAt(tmpDir + "/auth.json")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &commands.Context{
		QueryLoop:        &stubQL{},
		OnEvent:          func(string) {},
		ProviderRegistry: provider.DefaultRegistry(),
		CredentialStore:  cs,
	}

	err = commands.RunProviderOAuthConnect(ctx, "bedrock")
	if err == nil {
		t.Fatal("expected unsupported OAuth provider error")
	}
	if !strings.Contains(err.Error(), "does not support OAuth") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunProviderOAuthConnect_LocalizesValidationError(t *testing.T) {
	tmpDir := t.TempDir()
	cs, err := provider.NewCredentialStoreAt(tmpDir + "/auth.json")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &commands.Context{
		Language:         i18n.LangZH,
		QueryLoop:        &stubQL{},
		OnEvent:          func(string) {},
		ProviderRegistry: provider.DefaultRegistry(),
		CredentialStore:  cs,
	}
	err = commands.RunProviderOAuthConnect(ctx, "bedrock")
	if err == nil {
		t.Fatal("expected unsupported OAuth provider error")
	}
	if got := err.Error(); !strings.Contains(got, "不支持 OAuth PKCE 认证") || !strings.Contains(got, "Bedrock") {
		t.Fatalf("localized error did not preserve the Provider name: %q", got)
	}
}

// TestModelCmd_ShowModelsWithRegistry verifies /model with no args shows provider/model info.
func TestModelCmd_ShowModelsWithRegistry(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	var output strings.Builder
	ctx := &commands.Context{
		QueryLoop:             &stubQL{model: "claude-sonnet-4-20250514"},
		OnCommandPresentation: captureCompletedCommand(&output),
		CurrentProvider:       "anthropic",
		ProviderRegistry:      provider.DefaultRegistry(),
	}

	if err := r.Find("model").Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Current: anthropic/claude-sonnet-4-20250514") {
		t.Fatalf("expected current model info, got: %s", output.String())
	}
	if !strings.Contains(output.String(), "Available models") {
		t.Fatalf("expected available models section, got: %s", output.String())
	}
}

// TestModelCmd_SwitchWithinProvider verifies simple model switch (no provider prefix).
func TestModelCmd_SwitchWithinProvider(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	ql := &stubQL{model: "claude-sonnet-4-20250514"}
	var output strings.Builder
	ctx := &commands.Context{
		QueryLoop:             ql,
		OnCommandPresentation: captureCompletedCommand(&output),
	}

	if err := r.Find("model").Execute(ctx, "claude-opus-4-20250514"); err != nil {
		t.Fatal(err)
	}
	if ql.model != "claude-opus-4-20250514" {
		t.Fatalf("expected model switch, got %q", ql.model)
	}
	if !strings.Contains(output.String(), "Model switched to: claude-opus-4-20250514") {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

// TestModelCmd_ProviderSlashModel_UnknownProvider verifies error for unknown provider.
func TestModelCmd_ProviderSlashModel_UnknownProvider(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	ql := &stubQL{model: "old-model"}
	var output string
	ctx := &commands.Context{
		QueryLoop:        ql,
		OnEvent:          func(s string) { output += s },
		ProviderRegistry: provider.DefaultRegistry(),
	}

	err := r.Find("model").Execute(ctx, "fakeprovider/some-model")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("unexpected error: %v", err)
	}
	if ql.provider != nil || ql.model != "old-model" {
		t.Fatalf("unknown provider mutated runtime: provider=%v model=%q", ql.provider, ql.model)
	}
}

func TestModelCmd_ProviderSlashModel_BedrockWithoutCredentials(t *testing.T) {
	clearCommandAWSEnv(t)

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	ql := &stubQL{model: "old-model"}
	ctx := &commands.Context{
		QueryLoop:        ql,
		OnEvent:          func(s string) {},
		ProviderRegistry: provider.DefaultRegistry(),
	}

	err := r.Find("model").Execute(ctx, "bedrock/anthropic.claude-sonnet-4-6")
	if err == nil {
		t.Fatal("expected bedrock switch to fail without AWS credentials")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("unexpected error: %v", err)
	}
	if ql.provider != nil || ql.model != "old-model" {
		t.Fatalf("unready provider mutated runtime: provider=%v model=%q", ql.provider, ql.model)
	}
}

func TestModelCmd_ProviderSlashModel_UsesCanonicalHotSwap(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	ql := &stubQL{model: "old-model"}
	ctx := &commands.Context{
		QueryLoop:        ql,
		OnEvent:          func(string) {},
		ProviderRegistry: provider.DefaultRegistry(),
	}

	if err := r.Find("model").Execute(ctx, "ollama/llama3.1"); err != nil {
		t.Fatal(err)
	}
	if ql.provider == nil || ql.provider.Name() != "ollama" {
		t.Fatalf("canonical provider was not installed: %v", ql.provider)
	}
	if ql.model != "llama3.1" || ql.provider.ModelID() != "llama3.1" {
		t.Fatalf("canonical model mismatch: loop=%q provider=%q", ql.model, ql.provider.ModelID())
	}
}

// TestParseProviderModel tests the provider/model format parsing.
func TestParseProviderModel(t *testing.T) {
	tests := []struct {
		input     string
		wantProv  string
		wantModel string
	}{
		{"openai/o3", "openai", "o3"},
		{"anthropic/claude-sonnet-4-20250514", "anthropic", "claude-sonnet-4-20250514"},
		{"claude-sonnet-4", "", "claude-sonnet-4"},
		{"", "", ""},
		{"/model-only", "", "/model-only"}, // leading slash = no provider
	}

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	for _, tt := range tests {
		// We use the model command's Execute to test parsing indirectly.
		// For format parsing specifically, we test via the switch behavior.
		ql := &stubQL{model: "old"}
		var output string
		ctx := &commands.Context{
			QueryLoop: ql,
			OnEvent:   func(s string) { output += s },
		}

		// Simple model (no slash) should just switch model.
		if tt.wantProv == "" && tt.input != "" {
			_ = r.Find("model").Execute(ctx, tt.input)
			if ql.model != tt.input {
				t.Errorf("input=%q: expected model=%q, got %q", tt.input, tt.input, ql.model)
			}
		}
	}
}

func clearCommandAWSEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_BEARER_TOKEN_BEDROCK",
		"AWS_PROFILE",
	} {
		t.Setenv(key, "")
	}
}
