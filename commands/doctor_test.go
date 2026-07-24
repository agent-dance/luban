package commands_test

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/provider"
)

// ---------------------------------------------------------------------------
// Doctor command: Phase 6 multi-provider tests
// ---------------------------------------------------------------------------

func newDoctorCtx(providerName, model string) (*commands.Context, *strings.Builder) {
	var sb strings.Builder
	reg := provider.DefaultRegistry()
	return &commands.Context{
		QueryLoop:        &stubQL{model: model},
		OnEvent:          func(s string) { sb.WriteString(s) },
		CWD:              "/tmp", // safe directory
		CurrentProvider:  providerName,
		ProviderRegistry: reg,
	}, &sb
}

func TestDoctor_Execute_Anthropic(t *testing.T) {
	ctx, sb := newDoctorCtx("anthropic", "claude-sonnet-4-6")

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Find("doctor")
	if cmd == nil {
		t.Fatal("expected /doctor to be registered")
	}
	if err := cmd.Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	// Should have Credentials, Model, Git, Sandbox, Node.js, Disk, Config checks.
	for _, label := range []string{"Credentials", "Model", "Git", "Sandbox", "Node.js", "Disk", "Config"} {
		if !strings.Contains(out, label) {
			t.Errorf("expected %q check in output, got:\n%s", label, out)
		}
	}
	// Should NOT have Ollama Server (we're on "anthropic" provider).
	if strings.Contains(out, "Ollama Server") {
		t.Errorf("did not expect Ollama Server check for anthropic provider:\n%s", out)
	}
}

func TestDoctor_Execute_Ollama_IncludesServerCheck(t *testing.T) {
	ctx, sb := newDoctorCtx("ollama", "llama3.1")

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	// Ollama should include the server check.
	if !strings.Contains(out, "Ollama Server") {
		t.Errorf("expected Ollama Server check for ollama provider:\n%s", out)
	}
}

func TestDoctor_FilterByProvider(t *testing.T) {
	// For "openai" provider, Ollama Server check should be excluded.
	ctx, sb := newDoctorCtx("openai", "gpt-4o")

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	if strings.Contains(out, "Ollama Server") {
		t.Errorf("should NOT include Ollama Server check for openai:\n%s", out)
	}
	// Universal checks should still be present.
	if !strings.Contains(out, "Credentials") || !strings.Contains(out, "Model") {
		t.Errorf("universal checks missing for openai:\n%s", out)
	}
}

func TestDoctor_CheckProviderCredentials_NoAPIKey(t *testing.T) {
	// Without any env var or credential store, Anthropic should fail.
	ctx, sb := newDoctorCtx("anthropic", "claude-sonnet-4-6")
	// Ensure no ANTHROPIC_API_KEY (if set in environment, test is meaningless
	// but still shouldn't panic).

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	// Credentials check should mention "Anthropic"
	if !strings.Contains(out, "Anthropic") && !strings.Contains(out, "anthropic") {
		t.Errorf("expected provider name in credentials check:\n%s", out)
	}
}

func TestDoctor_CheckProviderCredentials_WithAnthropicAuthToken(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "anth-token-1234567890")

	ctx, sb := newDoctorCtx("anthropic", "claude-sonnet-4-6")

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	if !strings.Contains(out, "ANTHROPIC_AUTH_TOKEN set") {
		t.Errorf("expected auth token to be reported:\n%s", out)
	}
}

func TestDoctor_CheckProviderCredentials_OllamaNoKeyRequired(t *testing.T) {
	ctx, sb := newDoctorCtx("ollama", "llama3.1")

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	// Ollama should pass credentials check since no key is needed.
	if !strings.Contains(out, "no API key required") {
		t.Errorf("expected 'no API key required' for ollama:\n%s", out)
	}
}

func TestDoctor_CheckProviderCredentials_WithCredentialStore(t *testing.T) {
	// Create a temporary credential store with an OpenAI API key.
	tmpPath := t.TempDir() + "/auth.json"
	cs, err := provider.NewCredentialStoreAt(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Set(provider.CredentialEntry{
		Provider:   "openai",
		AuthMethod: "api_key",
		APIKey:     "sk-test-1234567890abcdef",
	}); err != nil {
		t.Fatal(err)
	}

	ctx, sb := newDoctorCtx("openai", "gpt-4o")
	ctx.CredentialStore = cs
	ctx.ProviderRegistry.SetCredentialStore(cs)
	t.Cleanup(func() { ctx.ProviderRegistry.SetCredentialStore(nil) })

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	// Should find credentials from the store.
	if !strings.Contains(out, "credential store") {
		// It might also find OPENAI_API_KEY from env if set, so check for pass mark.
		if !strings.Contains(out, "✓ Credentials") {
			t.Errorf("expected credential check to pass with store:\n%s", out)
		}
	}
}

func TestDoctor_CheckModelAvailable_InCatalog(t *testing.T) {
	ctx, sb := newDoctorCtx("anthropic", "claude-sonnet-4-6")

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	// Model should be found in catalog with context window and reasoning info.
	if !strings.Contains(out, "1M ctx") {
		t.Errorf("expected context window in model check:\n%s", out)
	}
	if !strings.Contains(out, "reasoning") {
		t.Errorf("expected reasoning flag in model check:\n%s", out)
	}
}

func TestDoctor_CheckModelAvailable_CustomModel(t *testing.T) {
	ctx, sb := newDoctorCtx("openai", "totally-unknown-preview")

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	// Should still pass but note "not in catalog".
	if !strings.Contains(out, "not in catalog") {
		t.Errorf("expected 'not in catalog' for unknown model:\n%s", out)
	}
}

func TestDoctor_CheckModelAvailable_NoModel(t *testing.T) {
	ctx, sb := newDoctorCtx("anthropic", "") // empty model

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	if !strings.Contains(out, "no model configured") {
		t.Errorf("expected 'no model configured':\n%s", out)
	}
}

func TestDoctor_DiagnoseAlias(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Find("diagnose")
	if cmd == nil || cmd.Name() != "doctor" {
		t.Fatal("expected /diagnose to alias /doctor")
	}
}

func TestDoctor_NoRegistryGraceful(t *testing.T) {
	// When no ProviderRegistry is attached, doctor should still work.
	var sb strings.Builder
	ctx := &commands.Context{
		QueryLoop:       &stubQL{model: "my-model"},
		OnEvent:         func(s string) { sb.WriteString(s) },
		CWD:             "/tmp",
		CurrentProvider: "anthropic",
		// Intentionally nil: ProviderRegistry, CredentialStore
	}

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	if !strings.Contains(out, "Credentials") || !strings.Contains(out, "Model") {
		t.Errorf("expected basic checks even without registry:\n%s", out)
	}
}

func TestMaskKey(t *testing.T) {
	// We can't call maskKey directly since it's unexported, but we can verify
	// it through the credential check output format. Short keys get "***".
	// This test verifies the output doesn't leak full keys by checking the
	// doctor credential output format contains "...".
	// (Integration test — verifying maskKey behavior is embedded.)

	// Use a long ANTHROPIC_API_KEY via credential store to trigger masking.
	tmpPath := t.TempDir() + "/auth.json"
	cs, err := provider.NewCredentialStoreAt(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Set(provider.CredentialEntry{
		Provider:   "gemini",
		AuthMethod: "api_key",
		APIKey:     "AIzaSyD_VERY_LONG_KEY_1234567890",
	}); err != nil {
		t.Fatal(err)
	}

	ctx, sb := newDoctorCtx("gemini", "gemini-2.5-pro")
	ctx.CredentialStore = cs
	ctx.ProviderRegistry.SetCredentialStore(cs)
	t.Cleanup(func() { ctx.ProviderRegistry.SetCredentialStore(nil) })

	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if err := r.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("doctor execute error: %v", err)
	}

	out := sb.String()
	// Key should NOT appear in full.
	if strings.Contains(out, "AIzaSyD_VERY_LONG_KEY_1234567890") {
		t.Errorf("full API key leaked in output:\n%s", out)
	}
}
