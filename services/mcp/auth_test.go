package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOAuthDiscoveryUsesProtectedResourceAndPathAwareMetadata(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			writeJSON(t, w, map[string]any{
				"authorization_servers": []string{server.URL + "/issuer/team"},
			})
		case "/.well-known/oauth-authorization-server/issuer/team":
			writeJSON(t, w, map[string]any{
				"issuer":                 server.URL + "/issuer/team",
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
				"scopes_supported":       []string{"read", "write"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	mgr := NewOAuthManager(NewMemoryTokenStore(), NewNeedsAuthCache(time.Minute))
	meta, err := mgr.DiscoverAuthServerMetadata(context.Background(), "srv", server.URL+"/mcp", "", "")
	if err != nil {
		t.Fatalf("DiscoverAuthServerMetadata: %v", err)
	}
	if meta.Issuer != server.URL+"/issuer/team" {
		t.Fatalf("issuer = %q", meta.Issuer)
	}
	if meta.AuthorizationEndpoint != server.URL+"/authorize" || meta.TokenEndpoint != server.URL+"/token" {
		t.Fatalf("metadata endpoints not preserved: %#v", meta)
	}
}

func TestOAuthFlowPKCECallbackStoresTokensAndClearsNeedsAuth(t *testing.T) {
	var tokenForm url.Values
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			writeJSON(t, w, map[string]any{"authorization_servers": []string{server.URL + "/issuer"}})
		case "/.well-known/oauth-authorization-server/issuer":
			writeJSON(t, w, map[string]any{
				"issuer":                 server.URL + "/issuer",
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
				"scopes_supported":       []string{"files:read"},
			})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			tokenForm = cloneURLValues(r.Form)
			writeJSON(t, w, map[string]any{
				"access_token":  "access-1",
				"refresh_token": "refresh-1",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         "files:read",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := NewMemoryTokenStore()
	cache := NewNeedsAuthCache(15 * time.Minute)
	mgr := NewOAuthManager(store, cache)
	cfg := MCPServerConfig{
		Type: TransportHTTP,
		URL:  server.URL + "/mcp",
		OAuth: &OAuthConfig{
			ClientID: "client-1",
		},
	}
	cache.Put(ServerKey("srv", cfg), NeedsAuthState{ServerName: "srv", ServerURL: cfg.URL, ResourceMetadataURL: server.URL + "/.well-known/oauth-protected-resource/mcp"})

	flow, err := mgr.StartOAuthFlow(context.Background(), "srv", cfg, OAuthFlowOptions{SkipBrowserOpen: true, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("StartOAuthFlow: %v", err)
	}
	authURL, err := url.Parse(flow.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	if authURL.Query().Get("code_challenge_method") != PKCEChallengeMethodS256 {
		t.Fatalf("code_challenge_method = %q", authURL.Query().Get("code_challenge_method"))
	}
	if authURL.Query().Get("code_challenge") == "" {
		t.Fatalf("authorization URL missing code_challenge")
	}

	resp, err := http.Get(flow.RedirectURI + "?code=auth-code&state=" + url.QueryEscape(flow.State))
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %d", resp.StatusCode)
	}
	if err := <-flow.Done(); err != nil {
		t.Fatalf("flow done: %v", err)
	}

	if tokenForm.Get("grant_type") != "authorization_code" || tokenForm.Get("code") != "auth-code" {
		t.Fatalf("bad token form: %v", tokenForm)
	}
	if tokenForm.Get("code_verifier") == "" || tokenForm.Get("client_id") != "client-1" {
		t.Fatalf("token form missing verifier/client: %v", tokenForm)
	}
	creds, ok, err := store.Load(context.Background(), ServerKey("srv", cfg))
	if err != nil || !ok {
		t.Fatalf("stored creds ok=%v err=%v", ok, err)
	}
	if creds.AccessToken != "access-1" || creds.RefreshToken != "refresh-1" || creds.ClientID != "client-1" {
		t.Fatalf("tokens not stored: %#v", creds)
	}
	if _, ok := cache.Get(ServerKey("srv", cfg)); ok {
		t.Fatalf("needs-auth cache was not cleared after successful OAuth")
	}
}

func TestOAuthTokenSourceRefreshesAndClearsInvalidGrantAliases(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-authorization-server/issuer":
			writeJSON(t, w, map[string]any{
				"issuer":                 server.URL + "/issuer",
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
			})
		case "/token":
			writeJSON(t, w, map[string]any{
				"error":             "invalid_refresh_token",
				"error_description": "rotated away",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := NewMemoryTokenStore()
	mgr := NewOAuthManager(store, NewNeedsAuthCache(time.Minute))
	key := ServerKeyFromFields("srv", string(TransportHTTP), server.URL+"/mcp", nil)
	err := store.Save(context.Background(), key, StoredOAuthCredentials{
		ServerName:     "srv",
		ServerURL:      server.URL + "/mcp",
		ClientID:       "client-1",
		AccessToken:    "old-access",
		RefreshToken:   "old-refresh",
		ExpiresAt:      time.Now().Add(-time.Minute),
		DiscoveryState: OAuthDiscoveryState{AuthorizationServerURL: server.URL + "/issuer"},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	token, err := mgr.TokenFor(context.Background(), server.URL+"/mcp")
	if err != nil {
		t.Fatalf("TokenFor: %v", err)
	}
	if token != "" {
		t.Fatalf("TokenFor token = %q, want empty after invalid_grant", token)
	}
	if _, ok, err := store.Load(context.Background(), key); err != nil || ok {
		t.Fatalf("stale credentials not cleared ok=%v err=%v", ok, err)
	}
}

func TestNeedsAuthCacheRecords401And403InsufficientScopeWithTTL(t *testing.T) {
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	cache := NewNeedsAuthCache(15 * time.Minute)
	cache.SetNow(func() time.Time { return now })
	cfg := MCPServerConfig{Type: TransportHTTP, URL: "https://mcp.example.test/api"}

	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header: http.Header{
			"WWW-Authenticate": {`Bearer error="insufficient_scope", scope="repo read", resource_metadata="https://mcp.example.test/.well-known/oauth-protected-resource/api"`},
		},
	}
	state, ok := RecordNeedsAuthFromHTTPResponse(cache, "github", cfg, resp)
	if !ok {
		t.Fatalf("403 insufficient_scope was not recorded")
	}
	if state.Scope != "repo read" || state.ResourceMetadataURL == "" {
		t.Fatalf("step-up challenge not preserved: %#v", state)
	}
	if _, ok := cache.Get(ServerKey("github", cfg)); !ok {
		t.Fatalf("needs-auth entry missing before TTL")
	}
	now = now.Add(16 * time.Minute)
	if _, ok := cache.Get(ServerKey("github", cfg)); ok {
		t.Fatalf("needs-auth entry survived past TTL")
	}

	resp401 := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header: http.Header{
			"WWW-Authenticate": {`Bearer realm="mcp", as_uri="https://auth.example.test/issuer"`},
		},
	}
	if _, ok := RecordNeedsAuthFromHTTPResponse(cache, "github", cfg, resp401); !ok {
		t.Fatalf("401 was not recorded")
	}
}

func TestFileTokenStorePersistsLocalFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := NewFileTokenStore(path)
	key := "srv|abc"
	want := StoredOAuthCredentials{ServerName: "srv", ServerURL: "https://mcp.example.test", AccessToken: "token"}
	if err := store.Save(context.Background(), key, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok, err := store.Load(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("Load ok=%v err=%v", ok, err)
	}
	if got.AccessToken != "token" {
		t.Fatalf("AccessToken = %q", got.AccessToken)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("token store permissions too open: %s", info.Mode().Perm())
	}
}

func TestParseWWWAuthenticatePreservesOAuthFields(t *testing.T) {
	resp := &http.Response{Header: http.Header{
		"WWW-Authenticate": {`Bearer realm="mcp", as_uri="https://auth.example.test", error="insufficient_scope", error_description="Need repo", scope="repo", resource_metadata="https://mcp.example.test/prm"`},
	}}
	ch := ParseWWWAuthenticate(resp)
	if ch == nil {
		t.Fatal("challenge nil")
	}
	if ch.ASURI != "https://auth.example.test" || ch.ErrorCode != "insufficient_scope" || ch.Scope != "repo" || !strings.Contains(ch.ErrorDescription, "Need") {
		t.Fatalf("challenge fields not parsed: %#v", ch)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("Encode: %v", err)
	}
}

func cloneURLValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}
