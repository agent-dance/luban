package provider

import "strings"

// defaultMaxContext is used when a model is not found in the catalog.
// 128k is a safe middle ground — most modern models support at least this.
const defaultMaxContext = 128000

const (
	defaultMaxOutputTokens  = 16 * 1024
	deepSeekMaxOutputTokens = 256_000
)

// DefaultMaxOutputTokens returns the request-side output budget used when the
// caller did not provide an explicit limit. Model MaxOutput metadata remains a
// capability ceiling and must not be used as the request default.
func DefaultMaxOutputTokens(providerName, model string) int {
	if CanonicalProviderName(providerName) == "deepseek" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "deepseek-") {
		return deepSeekMaxOutputTokens
	}
	return defaultMaxOutputTokens
}

// LookupMaxContext returns the context window size for the given model name.
// It resolves through the default ModelCatalog so context metadata comes from
// the same source as /model and the model picker.
func LookupMaxContext(model string) int {
	if model == "" {
		return defaultMaxContext
	}
	if info, ok := DefaultCatalog().Resolve(model); ok && info.ContextWindow > 0 {
		return info.ContextWindow
	}
	if normalized, ok := normalizeBedrockAnthropicModelID(model); ok {
		if info, ok := DefaultCatalog().Resolve(normalized); ok && info.ContextWindow > 0 {
			return info.ContextWindow
		}
	}
	return defaultMaxContext
}

// LookupMaxOutput returns the maximum output token count for the given model.
// Zero means the catalog does not know a provider/model-specific maximum.
func LookupMaxOutput(model string) int {
	if model == "" {
		return 0
	}
	if info, ok := DefaultCatalog().Resolve(model); ok && info.MaxOutput > 0 {
		return info.MaxOutput
	}
	if normalized, ok := normalizeBedrockAnthropicModelID(model); ok {
		if info, ok := DefaultCatalog().Resolve(normalized); ok && info.MaxOutput > 0 {
			return info.MaxOutput
		}
	}
	return 0
}

func normalizeBedrockAnthropicModelID(model string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(model))
	idx := strings.Index(lower, "anthropic.claude-")
	if idx <= 0 {
		return "", false
	}
	return strings.TrimSpace(model)[idx:], true
}
