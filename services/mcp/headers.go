package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

const (
	// MCPStreamableHTTPAccept is required by the Streamable HTTP transport on
	// every POST. Servers may reject requests that only advertise JSON.
	MCPStreamableHTTPAccept = "application/json, text/event-stream"

	defaultMCPUserAgent = "ClaudeCode/1.0"
)

// HeaderProvider resolves dynamic headers for remote MCP transports. The
// TypeScript implementation gets these from headersHelper; this interface is
// the Go transport hook without owning the helper execution policy itself.
type HeaderProvider interface {
	Headers(ctx context.Context, serverName, serverURL string) (map[string]string, error)
}

// HeaderProviderFunc adapts a function to HeaderProvider.
type HeaderProviderFunc func(ctx context.Context, serverName, serverURL string) (map[string]string, error)

// Headers implements HeaderProvider.
func (f HeaderProviderFunc) Headers(ctx context.Context, serverName, serverURL string) (map[string]string, error) {
	if f == nil {
		return nil, nil
	}
	return f(ctx, serverName, serverURL)
}

type remoteHeaderConfig struct {
	ServerName     string
	ServerURL      string
	UserAgent      string
	Headers        map[string]string
	HeaderProvider HeaderProvider
	Auth           TokenSource
}

func resolveRemoteHeaders(ctx context.Context, cfg remoteHeaderConfig) (map[string]string, error) {
	out := make(map[string]string, len(cfg.Headers)+4)
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = defaultMCPUserAgent
	}
	out["User-Agent"] = userAgent

	if cfg.Auth != nil {
		token, err := cfg.Auth.TokenFor(ctx, cfg.ServerURL)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyServicesMCPResolveAuthToken, err)
		}
		if strings.TrimSpace(token) != "" {
			out["Authorization"] = "Bearer " + strings.TrimSpace(token)
		}
	}

	copyHeaderMap(out, cfg.Headers)
	if cfg.HeaderProvider != nil {
		dynamic, err := cfg.HeaderProvider.Headers(ctx, cfg.ServerName, cfg.ServerURL)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyServicesMCPResolveDynamicHeaders, err)
		}
		copyHeaderMap(out, dynamic)
	}
	return out, nil
}

func copyHeaderMap(dst map[string]string, src map[string]string) {
	for key, value := range src {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		dst[key] = value
	}
}

func applyRemoteHeaders(req *http.Request, headers map[string]string) {
	for key, value := range headers {
		req.Header.Set(key, value)
	}
}

// UnauthorizedError carries a parsed WWW-Authenticate challenge so the tool
// layer can produce a structured "needs OAuth" hint instead of bubbling up a
// raw 401 string.
type UnauthorizedError struct {
	Challenge  *AuthChallenge
	ServerURL  string
	StatusCode int
}

// Error implements error.
func (e *UnauthorizedError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPUnauthorized)
	}
	if e.Challenge != nil && e.Challenge.ASURI != "" {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPUnauthorizedASURI, e.Challenge.ASURI)
	}
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPUnauthorized)
}

// RemoteHTTPError is returned when a remote transport receives a non-2xx
// status that is not a typed 401. RPCError is populated when the response body
// contains a JSON-RPC error envelope.
type RemoteHTTPError struct {
	StatusCode int
	Status     string
	ServerURL  string
	Body       string
	RPCError   *RPCError
	Challenge  *AuthChallenge
}

// Error implements error.
func (e *RemoteHTTPError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPRemoteHTTPError)
	}
	status := e.Status
	if status == "" && e.StatusCode != 0 {
		status = fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	if e.RPCError != nil {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPRemoteStatusDetail, status, e.RPCError.Error())
	}
	body := strings.TrimSpace(e.Body)
	if body != "" {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPRemoteStatusDetail, status, body)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPRemoteStatus, status)
}
