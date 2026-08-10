package provider

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/agent-dance/luban/types"
)

// annotateProviderRequestError attaches only bounded, display-safe request
// identity. Raw URLs, response headers, credentials, and provider bodies stay
// in the private cause chain.
func annotateProviderRequestError(apiErr *types.APIError, providerName, apiFormat, endpoint string, headers http.Header) *types.APIError {
	if apiErr == nil {
		return nil
	}
	apiErr.Provider = CanonicalProviderName(providerName)
	apiErr.APIFormat = normalizeOpenAIAPIFormat(apiFormat)
	apiErr.Endpoint = redactProviderEndpoint(endpoint, apiErr.APIFormat)
	apiErr.RequestID = providerRequestID(headers)
	return apiErr
}

func providerRequestID(headers http.Header) string {
	for _, name := range []string{"X-Request-ID", "OpenAI-Request-ID", "Request-ID", "X-Amzn-RequestId", "CF-Ray"} {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

// redactProviderEndpoint preserves the network origin and the standardized
// protocol suffix. Configured path segments, userinfo, query parameters, and
// fragments may contain gateway credentials and are never disclosed.
func redactProviderEndpoint(raw, apiFormat string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.RawPath = ""
	origin := parsed.Scheme + "://" + parsed.Host
	switch normalizeOpenAIAPIFormat(apiFormat) {
	case "responses":
		return origin + "/…/responses"
	case "chat-completions":
		return origin + "/…/chat/completions"
	default:
		return origin + "/…"
	}
}

func markAttemptedAPIFormats(err error, formats ...string) {
	apiErr, ok := AsAPIError(err)
	if !ok {
		return
	}
	apiErr.AttemptedAPIFormats = apiErr.AttemptedAPIFormats[:0]
	for _, format := range formats {
		if normalized := normalizeOpenAIAPIFormat(format); normalized != "" {
			apiErr.AttemptedAPIFormats = append(apiErr.AttemptedAPIFormats, normalized)
		}
	}
}
