package mcp

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

const (
	defaultClaudeAIProxyURL  = "https://mcp-proxy.anthropic.com"
	defaultClaudeAIProxyPath = "/v1/mcp/{server_id}"
	claudeAIProxySessionID   = "X-Mcp-Client-Session-Id"
)

// ClaudeAIProxyTransportConfig captures the specialized claude.ai connector
// proxy settings.
type ClaudeAIProxyTransportConfig struct {
	ServerName     string
	ServerID       string
	URL            string
	Headers        map[string]string
	HeaderProvider HeaderProvider
	Auth           TokenSource
	HTTPClient     *http.Client
	UserAgent      string
	SessionID      string
}

// NewClaudeAIProxyTransport constructs the streamable HTTP transport used for
// org-managed claude.ai MCP connectors. A bearer token is required before the
// first request so connectors never receive anonymous probes.
func NewClaudeAIProxyTransport(ctx context.Context, cfg ClaudeAIProxyTransportConfig) (Transport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	proxyURL, err := BuildClaudeAIProxyURL(cfg.ServerID)
	if err != nil {
		return nil, err
	}
	auth := cfg.Auth
	if auth == nil {
		auth = DefaultTokenSource()
	}
	token, err := auth.TokenFor(ctx, proxyURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, &UnauthorizedError{
			StatusCode: http.StatusUnauthorized,
			ServerURL:  proxyURL,
			Challenge: &AuthChallenge{
				Scheme:           "Bearer",
				ErrorCode:        "invalid_token",
				ErrorDescription: "claude.ai OAuth token is required for claudeai-proxy MCP servers",
			},
		}
	}
	headers := cloneStringMap(cfg.Headers)
	if cfg.SessionID != "" {
		if headers == nil {
			headers = map[string]string{}
		}
		headers[claudeAIProxySessionID] = cfg.SessionID
	}
	return NewHTTPTransport(HTTPTransportConfig{
		BaseURL:        proxyURL,
		HTTPClient:     cfg.HTTPClient,
		Headers:        headers,
		HeaderProvider: cfg.HeaderProvider,
		Auth: TokenSourceFunc(func(context.Context, string) (string, error) {
			return token, nil
		}),
		ServerName: cfg.ServerName,
		UserAgent:  cfg.UserAgent,
	})
}

// BuildClaudeAIProxyURL mirrors getOauthConfig().MCP_PROXY_URL +
// MCP_PROXY_PATH.replace('{server_id}', id). Env overrides keep tests and local
// deployments off the production endpoint.
func BuildClaudeAIProxyURL(serverID string) (string, error) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return "", i18n.NewError(i18n.KeyServicesMCPClaudeAIProxyServerID)
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("CLAUDE_CODE_MCP_PROXY_URL")), "/")
	if base == "" {
		base = defaultClaudeAIProxyURL
	}
	path := strings.TrimSpace(os.Getenv("CLAUDE_CODE_MCP_PROXY_PATH"))
	if path == "" {
		path = defaultClaudeAIProxyPath
	}
	escapedID := url.PathEscape(serverID)
	if strings.Contains(path, "{server_id}") {
		path = strings.ReplaceAll(path, "{server_id}", escapedID)
	} else {
		path = strings.TrimRight(path, "/") + "/" + escapedID
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	proxyURL := base + path
	parsed, err := url.Parse(proxyURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", i18n.NewError(i18n.KeyServicesMCPClaudeAIProxyURL, proxyURL)
	}
	return proxyURL, nil
}

func claudeAIProxySessionIDFromEnv() string {
	for _, key := range []string{"CLAUDE_CODE_SESSION_ID", "CLAUDE_SESSION_ID", "SESSION_ID"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
