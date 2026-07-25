package provider

import (
	"net/url"
	"strings"
)

const (
	openAICodexOriginator     = "codex_cli_rs"
	openAIChatGPTCodexBaseURL = "https://chatgpt.com/backend-api/codex"
)

func isOpenAIChatGPTCodexBaseURL(baseURL string) bool {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") == openAIChatGPTCodexBaseURL
}

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

func catalogOpenAIAPIFormat(model string) string {
	info, ok := DefaultCatalog().ResolveForProvider("openai", strings.TrimSpace(model))
	if !ok {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(info.APIFormat)) {
	case "responses", "chat-completions":
		return strings.ToLower(strings.TrimSpace(info.APIFormat))
	default:
		return ""
	}
}

func resolveOpenAIResponsesMode(authToken, requestedFormat, baseURL, model string) bool {
	if strings.TrimSpace(authToken) != "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(requestedFormat)) {
	case "responses":
		return true
	case "chat-completions":
		return false
	}
	// A model ID describes model capabilities, not the protocol implemented by
	// an arbitrary OpenAI-compatible gateway. Keep custom endpoints on the broad
	// Chat Completions default unless the user explicitly selected Responses.
	if isCustomOpenAIBaseURL(baseURL) {
		return false
	}
	// This model-aware routing is deliberately confined to the OpenAI factory.
	// It only applies to the first-party API; other providers may share the
	// OpenAI-compatible client, but own their endpoint selection themselves.
	switch catalogOpenAIAPIFormat(model) {
	case "responses":
		return true
	case "chat-completions":
		return false
	}
	return shouldUseOpenAIResponsesAPI(model)
}

// shouldNegotiateOpenAIResponses lets cataloged Responses models use their
// native protocol on compatible gateways without breaking chat-only servers.
// Explicit protocol choices remain authoritative; the protocol provider
// remembers a 404/405/501 Responses endpoint failure and falls back to Chat.
func shouldNegotiateOpenAIResponses(authToken, requestedFormat, baseURL, model string) bool {
	if strings.TrimSpace(authToken) != "" || !isCustomOpenAIBaseURL(baseURL) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(requestedFormat)) {
	case "responses", "chat-completions":
		return false
	}
	return catalogOpenAIAPIFormat(model) == "responses"
}

func isOpenAIPublicAPIBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	return err == nil && strings.EqualFold(parsed.Hostname(), "api.openai.com")
}

func isFirstPartyOpenAIResponsesBaseURL(raw string) bool {
	return isOpenAIPublicAPIBaseURL(raw) || isOpenAIChatGPTCodexBaseURL(raw)
}

func isOpenAIResponsesLiteModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	return lower == "gpt-5.6" || strings.HasPrefix(lower, "gpt-5.6-")
}

func isCustomOpenAIBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return true
	}
	return !strings.EqualFold(parsed.Hostname(), "api.openai.com")
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
