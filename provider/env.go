package provider

import (
	"fmt"
	"os"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// RuntimeOverrides contains invocation-scoped wire choices that must cross the
// credential-resolution boundary without being inferred from environment or
// endpoint location. Zero values preserve the ordinary provider defaults.
type RuntimeOverrides struct {
	APIFormat          string
	ResponsesWebSocket CapabilitySupport
}

// NewFromEnvWithOverrides creates a provider from environment variables, but allows
// explicit overrides for provider name and model. This avoids mutating global env
// with os.Setenv, which is not concurrency-safe.
//
// When a CredentialStore is attached to the registry, provider factories can
// also look up credentials from the store (see registry_builtins.go).
func NewFromEnvWithOverrides(providerOverride, modelOverride string) (Provider, error) {
	return NewFromEnvWithRuntimeOverrides(providerOverride, modelOverride, RuntimeOverrides{})
}

// NewFromEnvWithRuntimeOverrides resolves stored credentials, then applies the
// caller's typed invocation settings to the exact Config consumed by the
// provider factory. Responses WebSocket remains fail-closed unless Supported
// is explicitly supplied.
func NewFromEnvWithRuntimeOverrides(providerOverride, modelOverride string, overrides RuntimeOverrides) (Provider, error) {
	return newFromRegistryWithRuntimeOverrides(DefaultRegistry(), providerOverride, modelOverride, overrides)
}

func newFromRegistryWithRuntimeOverrides(registry *ProviderRegistry, providerOverride, modelOverride string, overrides RuntimeOverrides) (Provider, error) {
	providerType := resolveProviderType(providerOverride)
	normalized := strings.ToLower(strings.TrimSpace(providerType))

	// Check registry for the provider.
	if _, ok := registry.Get(normalized); ok {
		// Build a Config with credentials from the CredentialStore if available.
		cfg, err := ResolveCredentialConfig(registry, normalized)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(overrides.APIFormat) != "" {
			cfg.APIFormat = overrides.APIFormat
		}
		cfg.ResponsesWebSocket = normalizeCapabilitySupport(overrides.ResponsesWebSocket)
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
	if providerType == "" && os.Getenv("LUBAN_CODE_USE_BEDROCK") == "1" {
		providerType = "bedrock"
	}
	if providerType == "" && os.Getenv("LUBAN_CODE_USE_VERTEX") == "1" {
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
