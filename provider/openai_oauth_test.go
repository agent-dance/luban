package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testOpenAIIDToken(t *testing.T, authClaims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadBytes, err := json.Marshal(map[string]any{
		"https://api.openai.com/auth": authClaims,
	})
	if err != nil {
		t.Fatalf("Marshal claims: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + ".sig"
}

func TestRefreshOpenAIOAuthCredential(t *testing.T) {
	origTokenURL := openAIOAuthTokenURL
	origClient := openAIOAuthHTTPClient

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content-type = %q", ct)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if payload["grant_type"] != "refresh_token" {
				t.Fatalf("grant_type = %q", payload["grant_type"])
			}
			if got := payload["refresh_token"]; got != "refresh-123" {
				t.Fatalf("refresh_token = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-456",
				"id_token":      "idtok-456",
				"refresh_token": "refresh-789",
				"expires_in":    3600,
			})
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch r.Form.Get("grant_type") {
		case "urn:ietf:params:oauth:grant-type:token-exchange":
			if got := r.Form.Get("subject_token"); got != "idtok-456" {
				t.Fatalf("subject_token = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "sk-oauth-exchanged",
			})
		default:
			t.Fatalf("unexpected grant_type %q", r.Form.Get("grant_type"))
		}
	}))
	defer server.Close()

	openAIOAuthTokenURL = server.URL
	openAIOAuthHTTPClient = server.Client()
	defer func() {
		openAIOAuthTokenURL = origTokenURL
		openAIOAuthHTTPClient = origClient
	}()

	entry := CredentialEntry{
		Provider:     "openai",
		AuthMethod:   "oauth",
		RefreshToken: "refresh-123",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}
	refreshed, err := RefreshOpenAIOAuthCredential(context.Background(), entry)
	if err != nil {
		t.Fatalf("RefreshOpenAIOAuthCredential: %v", err)
	}
	if refreshed.APIKey != "sk-oauth-exchanged" {
		t.Fatalf("APIKey = %q", refreshed.APIKey)
	}
	if refreshed.AccessToken != "access-456" {
		t.Fatalf("AccessToken = %q", refreshed.AccessToken)
	}
	if refreshed.RefreshToken != "refresh-789" {
		t.Fatalf("RefreshToken = %q", refreshed.RefreshToken)
	}
	if refreshed.ExpiresAt.IsZero() {
		t.Fatal("expected ExpiresAt to be set")
	}
}

func TestResolveCredentialConfigRefreshesOpenAIOAuth(t *testing.T) {
	origTokenURL := openAIOAuthTokenURL
	origClient := openAIOAuthHTTPClient

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if payload["grant_type"] != "refresh_token" {
				t.Fatalf("grant_type = %q", payload["grant_type"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-new",
				"id_token":      "idtok-new",
				"refresh_token": "refresh-new",
				"expires_in":    3600,
			})
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		switch r.Form.Get("grant_type") {
		case "urn:ietf:params:oauth:grant-type:token-exchange":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "sk-new",
			})
		default:
			t.Fatalf("unexpected grant_type %q", r.Form.Get("grant_type"))
		}
	}))
	defer server.Close()

	openAIOAuthTokenURL = server.URL
	openAIOAuthHTTPClient = server.Client()
	defer func() {
		openAIOAuthTokenURL = origTokenURL
		openAIOAuthHTTPClient = origClient
	}()

	cs, err := NewCredentialStoreAt(t.TempDir() + "/auth.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Set(CredentialEntry{
		Provider:     "openai",
		AuthMethod:   "oauth",
		RefreshToken: "refresh-old",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	reg := NewProviderRegistry()
	reg.SetCredentialStore(cs)

	cfg, err := ResolveCredentialConfig(reg, "openai")
	if err != nil {
		t.Fatalf("ResolveCredentialConfig: %v", err)
	}
	if cfg.AuthToken != "access-new" {
		t.Fatalf("cfg.AuthToken = %q", cfg.AuthToken)
	}
	if cfg.APIKey != "" {
		t.Fatalf("cfg.APIKey = %q, want empty", cfg.APIKey)
	}
	if cfg.BaseURL != openAIChatGPTCodexBaseURL {
		t.Fatalf("cfg.BaseURL = %q", cfg.BaseURL)
	}

	entry, ok := cs.Get("openai")
	if !ok {
		t.Fatal("expected stored openai entry")
	}
	if entry.APIKey != "sk-new" || entry.AccessToken != "access-new" || entry.RefreshToken != "refresh-new" {
		t.Fatalf("unexpected stored entry: %+v", entry)
	}
}

func TestOpenAIOAuthRefreshHandlerUpdatesResponsesProvider(t *testing.T) {
	origTokenURL := openAIOAuthTokenURL
	origClient := openAIOAuthHTTPClient

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-new",
				"id_token": testOpenAIIDToken(t, map[string]any{
					"chatgpt_account_id":         "acct_new",
					"chatgpt_plan_type":          "team",
					"chatgpt_account_is_fedramp": true,
				}),
				"refresh_token": "refresh-new",
				"expires_in":    3600,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "sk-new"})
	}))
	defer server.Close()

	openAIOAuthTokenURL = server.URL
	openAIOAuthHTTPClient = server.Client()
	defer func() {
		openAIOAuthTokenURL = origTokenURL
		openAIOAuthHTTPClient = origClient
	}()

	cs, err := NewCredentialStoreAt(t.TempDir() + "/auth.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Set(CredentialEntry{
		Provider:     "openai",
		AuthMethod:   "oauth",
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
	}); err != nil {
		t.Fatal(err)
	}
	reg := NewProviderRegistry()
	reg.SetCredentialStore(cs)

	raw := NewResponses(Config{AuthToken: "access-old", BaseURL: "https://chatgpt.com/backend-api/codex"})
	handler := openAIOAuthRefreshHandler(reg, raw)
	if handler == nil || !handler() {
		t.Fatal("expected refresh handler to refresh credentials")
	}

	raw.mu.RLock()
	apiKey := raw.apiKey
	headers := cloneHeaders(raw.headers)
	isCodex := raw.chatGPTCodexBackend
	raw.mu.RUnlock()
	if apiKey != "access-new" {
		t.Fatalf("apiKey = %q", apiKey)
	}
	if !isCodex {
		t.Fatal("expected ChatGPT Codex backend after refresh")
	}
	if headers["ChatGPT-Account-ID"] != "acct_new" {
		t.Fatalf("ChatGPT-Account-ID = %q", headers["ChatGPT-Account-ID"])
	}
	if headers["X-OpenAI-Fedramp"] != "true" {
		t.Fatalf("X-OpenAI-Fedramp = %q", headers["X-OpenAI-Fedramp"])
	}
}

func TestOpenAIResultFromTokenResponseAllowsMissingAPIKeyExchange(t *testing.T) {
	origTokenURL := openAIOAuthTokenURL
	origClient := openAIOAuthHTTPClient

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	openAIOAuthTokenURL = server.URL
	openAIOAuthHTTPClient = server.Client()
	defer func() {
		openAIOAuthTokenURL = origTokenURL
		openAIOAuthHTTPClient = origClient
	}()

	result, err := openAIResultFromTokenResponse(context.Background(), &openAITokenResponse{
		AccessToken:  "access-token",
		IDToken:      "id-token",
		RefreshToken: "refresh-token",
		ExpiresIn:    3600,
	})
	if err != nil {
		t.Fatalf("openAIResultFromTokenResponse: %v", err)
	}
	if result.AccessToken != "access-token" {
		t.Fatalf("AccessToken = %q", result.AccessToken)
	}
	if result.APIKey != "" {
		t.Fatalf("APIKey = %q, want empty", result.APIKey)
	}
	if result.APIKeyExchangeError == "" {
		t.Fatal("expected API-key exchange error to be captured")
	}
}

func TestOpenAIResultFromTokenResponseParsesChatGPTClaims(t *testing.T) {
	origTokenURL := openAIOAuthTokenURL
	origClient := openAIOAuthHTTPClient

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"exchange disabled"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	openAIOAuthTokenURL = server.URL
	openAIOAuthHTTPClient = server.Client()
	defer func() {
		openAIOAuthTokenURL = origTokenURL
		openAIOAuthHTTPClient = origClient
	}()

	result, err := openAIResultFromTokenResponse(context.Background(), &openAITokenResponse{
		AccessToken: "access-token",
		IDToken: testOpenAIIDToken(t, map[string]any{
			"chatgpt_account_id":         "acct_123",
			"chatgpt_plan_type":          "pro",
			"chatgpt_account_is_fedramp": true,
		}),
		RefreshToken: "refresh-token",
		ExpiresIn:    3600,
	})
	if err != nil {
		t.Fatalf("openAIResultFromTokenResponse: %v", err)
	}
	if result.AccountID != "acct_123" {
		t.Fatalf("AccountID = %q", result.AccountID)
	}
	if result.PlanType != "pro" {
		t.Fatalf("PlanType = %q", result.PlanType)
	}
	if !result.AccountIsFedRAMP {
		t.Fatal("expected AccountIsFedRAMP=true")
	}
}

func TestBuildOpenAIAuthorizeURLIncludesCodexParams(t *testing.T) {
	rawURL, err := buildOpenAIAuthorizeURL("challenge", "state", "http://127.0.0.1/callback")
	if err != nil {
		t.Fatalf("buildOpenAIAuthorizeURL: %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	q := parsed.Query()
	if q.Get("originator") != openAIOAuthOriginator {
		t.Fatalf("originator = %q", q.Get("originator"))
	}
	if q.Get("codex_cli_simplified_flow") != "true" {
		t.Fatalf("codex_cli_simplified_flow = %q", q.Get("codex_cli_simplified_flow"))
	}
	if q.Get("id_token_add_organizations") != "true" {
		t.Fatalf("id_token_add_organizations = %q", q.Get("id_token_add_organizations"))
	}
	if !strings.Contains(q.Get("scope"), "offline_access") {
		t.Fatalf("scope = %q", q.Get("scope"))
	}
}
