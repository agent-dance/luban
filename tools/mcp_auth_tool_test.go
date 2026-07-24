package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	svcmcp "github.com/agent-dance/luban/services/mcp"
)

func TestMcpAuthToolReturnsAuthURLAndRunsCallbackContinuation(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			writeMCPAuthJSON(t, w, map[string]any{"authorization_servers": []string{server.URL + "/issuer"}})
		case "/.well-known/oauth-authorization-server/issuer":
			writeMCPAuthJSON(t, w, map[string]any{
				"issuer":                 server.URL + "/issuer",
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
			})
		case "/token":
			writeMCPAuthJSON(t, w, map[string]any{
				"access_token":  "tool-access",
				"refresh_token": "tool-refresh",
				"expires_in":    3600,
				"token_type":    "Bearer",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := svcmcp.NewMemoryTokenStore()
	manager := svcmcp.NewOAuthManager(store, svcmcp.NewNeedsAuthCache(time.Minute))
	cfg := svcmcp.MCPServerConfig{
		Type:  svcmcp.TransportHTTP,
		URL:   server.URL + "/mcp",
		OAuth: &svcmcp.OAuthConfig{ClientID: "client-1"},
	}
	reconnected := make(chan string, 1)
	tool := NewMcpAuthTool("my server", cfg, manager)
	tool.OnAuthenticated = func(_ context.Context, serverName string, _ svcmcp.MCPServerConfig) error {
		reconnected <- serverName
		return nil
	}

	if tool.Name() != "mcp__my_server__authenticate" {
		t.Fatalf("Name = %q", tool.Name())
	}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error result: %s", result.Content)
	}
	var out McpAuthOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.Status != "auth_url" || out.AuthURL == "" {
		t.Fatalf("unexpected output: %#v", out)
	}
	flow, ok := manager.ActiveOAuthFlow("my server", cfg)
	if !ok {
		t.Fatalf("active flow not recorded")
	}
	resp, err := http.Get(flow.RedirectURI + "?code=tool-code&state=" + url.QueryEscape(flow.State))
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	_ = resp.Body.Close()
	if err := <-flow.Done(); err != nil {
		t.Fatalf("flow done: %v", err)
	}
	select {
	case got := <-reconnected:
		if got != "my server" {
			t.Fatalf("OnAuthenticated server = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("OnAuthenticated was not called")
	}
	if token, err := manager.TokenFor(context.Background(), cfg.URL); err != nil || token != "tool-access" {
		t.Fatalf("TokenFor token=%q err=%v", token, err)
	}
}

func TestMcpAuthToolUnsupportedTransports(t *testing.T) {
	claudeAI := NewMcpAuthTool("connector", svcmcp.MCPServerConfig{Type: svcmcp.TransportClaudeAIProxy, URL: "https://example.test", ID: "abc"}, svcmcp.NewOAuthManager(svcmcp.NewMemoryTokenStore(), svcmcp.NewNeedsAuthCache(time.Minute)))
	result, err := claudeAI.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute claudeai: %v", err)
	}
	var out McpAuthOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("decode claudeai output: %v", err)
	}
	if out.Status != "unsupported" {
		t.Fatalf("claudeai status = %q", out.Status)
	}

	stdio := NewMcpAuthTool("stdio", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "echo"}, svcmcp.NewOAuthManager(svcmcp.NewMemoryTokenStore(), svcmcp.NewNeedsAuthCache(time.Minute)))
	result, err = stdio.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute stdio: %v", err)
	}
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("decode stdio output: %v", err)
	}
	if out.Status != "unsupported" {
		t.Fatalf("stdio status = %q", out.Status)
	}
}

func writeMCPAuthJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("Encode: %v", err)
	}
}
