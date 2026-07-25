package transport

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

const (
	// streamableHTTPAccept is required by the Streamable HTTP transport on
	// every POST. Servers may reject requests that only advertise JSON.
	streamableHTTPAccept = "application/json, text/event-stream"

	defaultMCPUserAgent = brand.CommandName + "/1.0"
)

type remoteHeaderConfig struct {
	ServerURL string
	Headers   map[string]string
	Auth      mcpauth.TokenSource
}

func resolveRemoteHeaders(ctx context.Context, cfg remoteHeaderConfig) (map[string]string, error) {
	out := make(map[string]string, len(cfg.Headers)+4)
	out["User-Agent"] = defaultMCPUserAgent

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

// RemoteHTTPError is returned when a remote transport receives a non-2xx
// status that is not a typed 401. RPCError is populated when the response body
// contains a JSON-RPC error envelope.
type RemoteHTTPError struct {
	StatusCode int
	Status     string
	ServerURL  string
	Body       string
	RPCError   *protocol.RPCError
	Challenge  *mcpauth.AuthChallenge
}

// AuthStatusCode exposes the HTTP status to the auth domain without coupling
// auth to this transport-specific error type.
func (e *RemoteHTTPError) AuthStatusCode() int {
	if e == nil {
		return 0
	}
	return e.StatusCode
}

// AuthChallenge exposes the parsed challenge to the auth domain.
func (e *RemoteHTTPError) AuthChallenge() *mcpauth.AuthChallenge {
	if e == nil {
		return nil
	}
	return e.Challenge
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
