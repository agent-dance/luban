package provider

import (
	"fmt"
	"os"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// NewFromEnv creates the appropriate provider based on environment variables.
// Missing credentials return an unconfigured provider from the registry so the
// terminal can still open and persist settings before the first model call.
func NewFromEnv() (Provider, error) {
	return NewFromEnvWithOverrides("", "")
}

// NewFromEnvWithOverrides creates a provider from environment variables, but allows
// explicit overrides for provider name and model. This avoids mutating global env
// with os.Setenv, which is not concurrency-safe.
//
// Internally this uses the DefaultRegistry() to look up the provider factory,
// but the external behaviour (env-var precedence, auto-detection, error messages)
// is 100% backward-compatible.
//
// When a CredentialStore is attached to the registry, provider factories can
// also look up credentials from the store (see registry_builtins.go).
func NewFromEnvWithOverrides(providerOverride, modelOverride string) (Provider, error) {
	providerType := resolveProviderType(providerOverride)

	registry := DefaultRegistry()
	normalized := strings.ToLower(strings.TrimSpace(providerType))
	if normalized == "oauth" {
		normalized = CanonicalProviderName(normalized)
	}

	// Check registry for the provider.
	if _, ok := registry.Get(normalized); ok {
		// Build a Config with credentials from the CredentialStore if available.
		cfg, err := ResolveCredentialConfig(registry, normalized)
		if err != nil {
			return nil, err
		}
		return registry.Create(normalized, cfg, modelOverride)
	}

	// Reject unknown provider names.
	return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderUnknown,
		providerType, strings.Join(registry.VisibleNames(), ", ")))
}

// resolveProviderType determines the provider type from overrides and environment
// variables, applying the auto-detection logic.
func resolveProviderType(providerOverride string) string {
	providerType := providerOverride
	if providerType == "" {
		providerType = os.Getenv("PROVIDER")
	}
	// Support the TypeScript-compatible env flags for enterprise cloud providers.
	if providerType == "" && os.Getenv("CLAUDE_CODE_USE_BEDROCK") == "1" {
		providerType = "bedrock"
	}
	if providerType == "" && os.Getenv("CLAUDE_CODE_USE_VERTEX") == "1" {
		providerType = "vertex"
	}
	if providerType == "" && os.Getenv("DEEPSEEK_API_KEY") != "" {
		providerType = brand.DeepSeekProvider
	}
	// Auto-detect OpenAI when OPENAI_API_KEY is set and no DeepSeek/Anthropic key
	// is present. DeepSeek is the product default, but explicit usable keys win.
	if providerType == "" && os.Getenv("OPENAI_API_KEY") != "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		providerType = "openai"
	}
	if providerType == "" && os.Getenv("ANTHROPIC_API_KEY") != "" {
		providerType = "anthropic"
	}
	if providerType == "" && os.Getenv("OAUTH_ACCESS_TOKEN") != "" {
		providerType = "anthropic"
	}
	if providerType == "" {
		providerType = brand.DefaultProvider
	}
	return providerType
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
