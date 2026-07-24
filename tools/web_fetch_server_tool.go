// Package tools — Anthropic web_fetch_20250910 server-tool integration.
//
// Mirrors the role of the web_fetch server tool described in
// src/tools/WebFetchTool/WebFetchTool.ts: when an Anthropic API client is
// configured the tool routes the (url, prompt) pair through Claude's own
// hosted fetch + summariser instead of doing the HTTP round-trip locally.
// Falls back to the legacy fetch when the server tool is unavailable so
// the public Execute path always produces a result.
//
// The integration is interface-driven so the rest of the package never
// imports the SDK directly — that lets test harnesses inject a fake
// provider without bringing the network into the test loop.
package tools

import (
	"context"
	"errors"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// WebFetchServerToolName matches the value used in the TS reference.
const WebFetchServerToolName = "web_fetch_20250910"

// webFetchServerToolBetaHeader is the SDK beta flag required to enable
// the server tool on the Messages API. Surfaced as a constant so tests
// and adapters can assert it without hard-coding strings.
const webFetchServerToolBetaHeader = "web-fetch-2025-09-10"

// WebFetchServerToolProvider executes a single web_fetch_20250910 call
// against the Anthropic API and returns the assistant text plus the
// resolved URL. Implementations should set the betas header to
// webFetchServerToolBetaHeader and respect any auth tokens captured in
// the application config.
type WebFetchServerToolProvider interface {
	FetchViaServerTool(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error)
}

// WebFetchServerToolRequest captures the inputs needed to invoke the
// server tool. AllowedDomains/BlockedDomains are optional and pass
// through unchanged.
type WebFetchServerToolRequest struct {
	URL            string
	Prompt         string
	AllowedDomains []string
	BlockedDomains []string
	MaxTokens      int
}

// WebFetchServerToolResponse is the parsed structured payload the server
// returns. Summary is the rendered text; ResolvedURL is the URL the
// server actually fetched (after any internal redirects); IsRedirect
// indicates the response is a REDIRECT marker rather than content.
type WebFetchServerToolResponse struct {
	Summary      string
	ResolvedURL  string
	StatusCode   int
	StatusText   string
	ContentType  string
	IsRedirect   bool
	RedirectURL  string
	RedirectCode int
	RawBlocks    []string // serialized content blocks for downstream propagation
}

// WebFetchServerToolFunc adapts a closure to the provider interface.
type WebFetchServerToolFunc func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error)

// FetchViaServerTool implements WebFetchServerToolProvider.
func (f WebFetchServerToolFunc) FetchViaServerTool(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
	return f(ctx, req)
}

// ErrWebFetchServerToolUnavailable signals that the server tool can't
// service the request right now (no provider configured, beta disabled,
// etc.). Execute callers map this to the legacy local fetch.
var ErrWebFetchServerToolUnavailable = errors.New("web_fetch server tool is unavailable")

// runWebFetchServerTool dispatches to the provider and converts the
// response into the structured payload the rest of the tool already
// understands. Returns ErrWebFetchServerToolUnavailable when no provider
// is registered so callers can fall back to local fetch transparently.
func runWebFetchServerTool(
	ctx context.Context,
	provider WebFetchServerToolProvider,
	in WebFetchInput,
	allowed, blocked []string,
) (webFetchStructuredPayload, error) {
	if provider == nil {
		return webFetchStructuredPayload{}, ErrWebFetchServerToolUnavailable
	}
	resp, err := provider.FetchViaServerTool(ctx, WebFetchServerToolRequest{
		URL:            in.URL,
		Prompt:         in.Prompt,
		AllowedDomains: allowed,
		BlockedDomains: blocked,
		MaxTokens:      WebFetchSummariserMaxTokens,
	})
	if err != nil {
		return webFetchStructuredPayload{}, i18n.WrapError(i18n.KeyToolWebServerToolFailed, err)
	}
	if resp.IsRedirect {
		return webFetchStructuredPayload{
			URL:     in.URL,
			Prompt:  in.Prompt,
			Method:  string(webExecutionModeProviderNative),
			Summary: formatRedirectMarker(in.URL, resp.RedirectURL, resp.RedirectCode, in.Prompt),
		}, nil
	}
	url := resp.ResolvedURL
	if url == "" {
		url = in.URL
	}
	summary := strings.TrimSpace(resp.Summary)
	return webFetchStructuredPayload{
		URL:     url,
		Prompt:  in.Prompt,
		Method:  string(webExecutionModeProviderNative),
		Summary: summary,
	}, nil
}

// SetWebFetchServerToolProvider wires a provider into the WebFetchTool
// instance and (optionally) flips it to provider-native mode.
func (w *WebFetchTool) SetWebFetchServerToolProvider(p WebFetchServerToolProvider, preferred bool) {
	w.serverTool = p
	if preferred && p != nil {
		w.mode = webExecutionModeProviderNative
	}
}

// HasWebFetchServerTool reports whether a provider has been configured.
func (w *WebFetchTool) HasWebFetchServerTool() bool {
	return w.serverTool != nil
}

// formatRedirectMarker formats a REDIRECT marker for the web_fetch server-tool
// response. Mirrors the TS reference which surfaces the redirect target back
// to the model so it can choose to follow.
func formatRedirectMarker(originalURL, redirectURL string, code int, prompts ...string) string {
	originalURL = strings.TrimSpace(originalURL)
	redirectURL = strings.TrimSpace(redirectURL)
	prompt := ""
	if len(prompts) > 0 {
		prompt = prompts[0]
	}
	statusText := redirectStatusText(code)
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebRedirectMarker,
		originalURL, redirectURL, code, statusText, redirectURL, prompt)
}

func redirectStatusText(code int) string {
	switch code {
	case 301:
		return "Moved Permanently"
	case 308:
		return "Permanent Redirect"
	case 307:
		return "Temporary Redirect"
	default:
		return "Found"
	}
}
