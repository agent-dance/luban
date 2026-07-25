package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/brand"
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

	mgr := NewOAuthManager(newTestMemoryTokenStore(), NewNeedsAuthCache(time.Minute))
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

type testMemoryTokenStore struct {
	mu      sync.RWMutex
	entries map[string]StoredOAuthCredentials
}

func newTestMemoryTokenStore() *testMemoryTokenStore {
	return &testMemoryTokenStore{entries: make(map[string]StoredOAuthCredentials)}
}

func (store *testMemoryTokenStore) Load(_ context.Context, serverKey string) (StoredOAuthCredentials, bool, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	credentials, ok := store.entries[serverKey]
	return credentials, ok, nil
}

func (store *testMemoryTokenStore) LoadByServerURL(_ context.Context, serverURL string) (string, StoredOAuthCredentials, bool, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	for key, credentials := range store.entries {
		if credentials.ServerURL == serverURL {
			return key, credentials, true, nil
		}
	}
	return "", StoredOAuthCredentials{}, false, nil
}

func (store *testMemoryTokenStore) Save(_ context.Context, serverKey string, credentials StoredOAuthCredentials) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.entries[serverKey] = credentials
	return nil
}

func (store *testMemoryTokenStore) Clear(_ context.Context, serverKey string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.entries, serverKey)
	return nil
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

	store := newTestMemoryTokenStore()
	cache := NewNeedsAuthCache(15 * time.Minute)
	mgr := NewOAuthManager(store, cache)
	cfg := ServerDescriptor{
		Transport:    "http",
		OAuthCapable: true,
		URL:          server.URL + "/mcp",
		OAuth: &Config{
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

	store := newTestMemoryTokenStore()
	mgr := NewOAuthManager(store, NewNeedsAuthCache(time.Minute))
	key := ServerKey("srv", ServerDescriptor{Transport: "http", URL: server.URL + "/mcp"})
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

func TestOAuthDynamicRegistrationUsesCurrentProductIdentity(t *testing.T) {
	var clientName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode registration: %v", err)
		}
		clientName, _ = request["client_name"].(string)
		writeJSON(t, w, map[string]any{"client_id": "registered-client"})
	}))
	defer server.Close()

	manager := NewOAuthManager(newTestMemoryTokenStore(), NewNeedsAuthCache(time.Minute))
	_, err := manager.registerOAuthClient(
		context.Background(),
		ServerKey("repo", ServerDescriptor{Transport: "http", URL: "https://mcp.example.test"}),
		"repo",
		ServerDescriptor{Transport: "http", URL: "https://mcp.example.test"},
		server.URL,
		"http://127.0.0.1/callback",
	)
	if err != nil {
		t.Fatalf("registerOAuthClient: %v", err)
	}
	want := brand.DisplayName + " (repo)"
	if clientName != want {
		t.Fatalf("client_name = %q, want %q", clientName, want)
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
