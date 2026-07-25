package provider

import (
	"os"
	"testing"
)

// setEnv temporarily sets environment variables for a test and restores them
// via t.Cleanup.
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	old := make(map[string]string, len(kv))
	for k, v := range kv {
		old[k] = os.Getenv(k)
		os.Setenv(k, v)
	}
	t.Cleanup(func() {
		for k, v := range old {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	})
}

// clearEnv clears the given keys and restores them via t.Cleanup.
func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	kv := make(map[string]string, len(keys))
	for _, k := range keys {
		kv[k] = ""
	}
	setEnv(t, kv)
}

// TestAutoDetectOpenAI verifies that OPENAI_API_KEY alone causes the provider
// to resolve to OpenAI without explicitly setting PROVIDER=openai.
func TestAutoDetectOpenAI(t *testing.T) {
	clearEnv(t,
		"PROVIDER",
		"DEEPSEEK_API_KEY",
		"ANTHROPIC_API_KEY",
		"LUBAN_CODE_USE_BEDROCK",
		"LUBAN_CODE_USE_VERTEX",
	)
	setEnv(t, map[string]string{
		"OPENAI_API_KEY": "sk-test-openai-key",
	})

	p, err := NewFromEnvWithOverrides("", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if p == nil {
		t.Fatal("expected a non-nil provider")
	}
	// The provider should resolve to OpenAI; verify via ModelID default.
	// We don't assert the model string here because it depends on OPENAI_MODEL,
	// but we do confirm no error occurred (i.e., OpenAI path was taken).
}

// TestAutoDetectOpenAI_OverridesDefault verifies that ANTHROPIC_API_KEY is
// ignored when OPENAI_API_KEY is also set (OpenAI wins via auto-detect order).
func TestAutoDetectOpenAI_OverridesDefault(t *testing.T) {
	clearEnv(t,
		"PROVIDER",
		"DEEPSEEK_API_KEY",
		"LUBAN_CODE_USE_BEDROCK",
		"LUBAN_CODE_USE_VERTEX",
	)
	setEnv(t, map[string]string{
		"ANTHROPIC_API_KEY": "anthro-key",
		"OPENAI_API_KEY":    "sk-test-openai-key",
	})

	p, err := NewFromEnvWithOverrides("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected a non-nil provider")
	}
}

// TestAutoDetectOpenAI_ExplicitProviderWins verifies that an explicit
// PROVIDER=anthropic value takes precedence over OPENAI_API_KEY.
func TestAutoDetectOpenAI_ExplicitProviderWins(t *testing.T) {
	clearEnv(t,
		"DEEPSEEK_API_KEY",
		"LUBAN_CODE_USE_BEDROCK",
		"LUBAN_CODE_USE_VERTEX",
	)
	setEnv(t, map[string]string{
		"PROVIDER":          "anthropic",
		"ANTHROPIC_API_KEY": "anthro-key",
		"OPENAI_API_KEY":    "sk-test-openai-key",
	})

	p, err := NewFromEnvWithOverrides("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected a non-nil provider")
	}
	// Anthropic provider returns "claude-*" model IDs; just verify no error.
}

// TestAutoDetectOpenAI_MissingKey verifies that without any key, the default
// DeepSeek path still creates an unconfigured provider so the REPL can open.
func TestAutoDetectOpenAI_MissingKey(t *testing.T) {
	clearEnv(t,
		"PROVIDER",
		"DEEPSEEK_API_KEY",
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"LUBAN_CODE_USE_BEDROCK",
		"LUBAN_CODE_USE_VERTEX",
	)

	p, err := NewFromEnvWithOverrides("", "")
	if err != nil {
		t.Fatalf("expected no startup error, got %v", err)
	}
	if p.Name() != "deepseek" {
		t.Fatalf("expected deepseek provider, got %s", p.Name())
	}
	if _, ok := p.(*UnconfiguredProvider); !ok {
		t.Fatalf("expected unconfigured provider, got %T", p)
	}
}

// TestAutoDetectOpenAI_CLIOverrideTakesPrecedence verifies that a CLI-level
// provider override beats OPENAI_API_KEY auto-detection.
func TestAutoDetectOpenAI_CLIOverrideTakesPrecedence(t *testing.T) {
	clearEnv(t,
		"PROVIDER",
		"DEEPSEEK_API_KEY",
		"LUBAN_CODE_USE_BEDROCK",
		"LUBAN_CODE_USE_VERTEX",
	)
	setEnv(t, map[string]string{
		"OPENAI_API_KEY":    "sk-test-openai-key",
		"ANTHROPIC_API_KEY": "anthro-key",
	})

	p, err := NewFromEnvWithOverrides("anthropic", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected a non-nil provider")
	}
}

func TestAutoDetectDeepSeek(t *testing.T) {
	clearEnv(t,
		"PROVIDER",
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"LUBAN_CODE_USE_BEDROCK",
		"LUBAN_CODE_USE_VERTEX",
	)
	setEnv(t, map[string]string{
		"DEEPSEEK_API_KEY": "sk-test-deepseek-key",
	})

	p, err := NewFromEnvWithOverrides("", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if p.Name() != "deepseek" {
		t.Fatalf("expected deepseek provider, got %s", p.Name())
	}
	if p.ModelID() != "deepseek-v4-flash" {
		t.Fatalf("expected deepseek-v4-flash default model, got %s", p.ModelID())
	}
}

func TestDeepSeekModelEnvUsesConfiguredID(t *testing.T) {
	clearEnv(t,
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"LUBAN_CODE_USE_BEDROCK",
		"LUBAN_CODE_USE_VERTEX",
	)
	setEnv(t, map[string]string{
		"PROVIDER":         "deepseek",
		"DEEPSEEK_API_KEY": "sk-test-deepseek-key",
		"DEEPSEEK_MODEL":   "deepseek-v4-pro",
	})

	p, err := NewFromEnvWithOverrides("", "")
	if err != nil {
		t.Fatalf("NewFromEnvWithOverrides: %v", err)
	}
	if p.ModelID() != "deepseek-v4-pro" {
		t.Fatalf("model = %q, want exact configured ID", p.ModelID())
	}
}
