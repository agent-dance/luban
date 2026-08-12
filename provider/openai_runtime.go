package provider

import (
	"net/url"
	"strings"
)

const (
	openAICodexOriginator     = "codex_cli_rs"
	openAIChatGPTCodexBaseURL = "https://chatgpt.com/backend-api/codex"
)

func openAICodexHeaders() map[string]string {
	return map[string]string{
		"originator": openAICodexOriginator,
		"User-Agent": openAICodexOriginator,
	}
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(headers))
	for k, v := range headers {
		cloned[k] = v
	}
	return cloned
}

func mergeHeaders(dst, src map[string]string) map[string]string {
	if len(src) == 0 {
		return cloneHeaders(dst)
	}
	out := cloneHeaders(dst)
	if out == nil {
		out = make(map[string]string, len(src))
	}
	for k, v := range src {
		out[k] = v
	}
	return out
}

func shouldUseOpenAIResponsesAPI(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	if lower == "" {
		return false
	}
	return strings.HasPrefix(lower, "gpt-5") || strings.Contains(lower, "codex")
}

func normalizeOpenAIAPIFormat(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "responses", "chat-completions":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func modelCatalogOpenAIAPIFormat(catalog *ModelCatalog, providerName, model string) string {
	if catalog == nil {
		return ""
	}
	info, ok := catalog.ResolveForProvider(CanonicalProviderName(providerName), strings.TrimSpace(model))
	if !ok {
		return ""
	}
	return normalizeOpenAIAPIFormat(info.APIFormat)
}

func catalogOpenAIAPIFormat(model string) string {
	return modelCatalogOpenAIAPIFormat(DefaultCatalog(), "openai", model)
}

// compatibleOpenAIAPIFormat combines discovered gateway metadata with the
// built-in OpenAI catalog. Historical discovery labeled every OpenAI-style
// model chat-completions, so that inferred value must not suppress a known
// built-in Responses model. A true operator override is carried separately in
// Config.APIFormat and is resolved by the compatible provider factory.
func compatibleOpenAIAPIFormat(catalog *ModelCatalog, providerName, model string) string {
	builtinFormat := modelCatalogOpenAIAPIFormat(catalog, "openai", model)
	if builtinFormat == "" {
		builtinFormat = catalogOpenAIAPIFormat(model)
	}
	if builtinFormat == "responses" {
		return builtinFormat
	}
	if gatewayFormat := modelCatalogOpenAIAPIFormat(catalog, providerName, model); gatewayFormat != "" {
		return gatewayFormat
	}
	return builtinFormat
}

func resolveOpenAIResponsesMode(authToken, requestedFormat, model string) bool {
	if strings.TrimSpace(authToken) != "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(requestedFormat)) {
	case "responses":
		return true
	case "chat-completions":
		return false
	}
	// This model-aware routing is deliberately confined to the OpenAI factory.
	// A BaseURL changes only the transport location; it cannot change the native
	// OpenAI wire contract selected by the model catalog.
	switch catalogOpenAIAPIFormat(model) {
	case "responses":
		return true
	case "chat-completions":
		return false
	}
	return shouldUseOpenAIResponsesAPI(model)
}

// newNegotiatingOpenAIProvider is restricted to explicitly compatible
// providers. Native providers select one authoritative wire contract before
// construction and never infer protocol support from endpoint behavior.
func newNegotiatingOpenAIProvider(cfg Config) *openAIProtocolProvider {
	responses := NewResponses(cfg)
	chatCfg := cfg
	chatCfg.BaseURL = normalizeOpenAIChatBaseURL(chatCfg.BaseURL)
	// Reaching Chat means the custom gateway already rejected the native
	// Responses envelope. Such gateways commonly implement function schemas
	// without OpenAI's recursive strict-schema contract. Keep the projected
	// definition's Strict bit derived from its local schema, but omit the wire
	// extension on this compatibility fallback (the same policy used by the
	// explicit compatible-provider factory).
	chatCfg.DisableStrictTools = true
	return newOpenAIProtocolProvider(responses, NewOpenAI(chatCfg))
}

func isOpenAIResponsesLiteModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	return lower == "gpt-5.6" || strings.HasPrefix(lower, "gpt-5.6-")
}

// supportsOpenAIResponsesCustomTools is intentionally narrower than generic
// tool support. Only the explicit public Responses contract and the verified
// GPT-5.6 family may carry freeform grammar tools. Codex/Lite and compatible
// endpoints remain fail-closed until they gain their own wire contract tests.
func supportsOpenAIResponsesCustomTools(semantics ResponsesSemantics, model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	if semantics == ResponsesSemanticsDeepSeek {
		return lower == "deepseek-v4-flash"
	}
	if semantics != ResponsesSemanticsOpenAIPublic {
		return false
	}
	return lower == "gpt-5.6" || strings.HasPrefix(lower, "gpt-5.6-")
}

func normalizeOpenAIChatBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Path != "" && parsed.Path != "/") {
		return raw
	}
	parsed.Path = "/v1"
	return strings.TrimRight(parsed.String(), "/")
}
