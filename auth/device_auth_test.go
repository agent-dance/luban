package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// ---------------------------------------------------------------------------
// TestDeviceCodeRequest — verify initial device code request format
// ---------------------------------------------------------------------------

func TestDeviceCodeRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if cid := r.FormValue("client_id"); cid != "test-client" {
			t.Errorf("client_id: got %q, want test-client", cid)
		}
		if scope := r.FormValue("scope"); scope != "user:inference" {
			t.Errorf("scope: got %q, want user:inference", scope)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceCodeResponse{
			DeviceCode:              "device-code-abc",
			UserCode:                "ABCD-1234",
			VerificationURI:         "https://example.com/device",
			VerificationURIComplete: "https://example.com/device?user_code=ABCD-1234",
			ExpiresIn:               900,
			Interval:                5,
		})
	}))
	defer srv.Close()

	cfg := DeviceAuthConfig{
		ClientID:      "test-client",
		DeviceAuthURL: srv.URL,
		TokenURL:      "http://unused", // not called in this test
		Scopes:        []string{"user:inference"},
	}

	dcr, err := requestDeviceCode(context.Background(), cfg)
	if err != nil {
		t.Fatalf("requestDeviceCode: %v", err)
	}
	if dcr.DeviceCode != "device-code-abc" {
		t.Errorf("DeviceCode: got %q, want device-code-abc", dcr.DeviceCode)
	}
	if dcr.UserCode != "ABCD-1234" {
		t.Errorf("UserCode: got %q, want ABCD-1234", dcr.UserCode)
	}
	if dcr.ExpiresIn != 900 {
		t.Errorf("ExpiresIn: got %d, want 900", dcr.ExpiresIn)
	}
}

// ---------------------------------------------------------------------------
// TestDeviceAuth_FullFlow — mock auth+token endpoints, simulate user approval
// ---------------------------------------------------------------------------

func TestDeviceAuth_FullFlow(t *testing.T) {
	var pollCount atomic.Int32

	// Combined server for both device auth and token endpoints.
	mux := http.NewServeMux()

	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceCodeResponse{
			DeviceCode:      "dev-123",
			UserCode:        "USER-CODE",
			VerificationURI: "https://example.com/verify",
			ExpiresIn:       300,
			Interval:        1,
		})
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if gt := r.FormValue("grant_type"); gt != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("grant_type: got %q", gt)
		}
		if dc := r.FormValue("device_code"); dc != "dev-123" {
			t.Errorf("device_code: got %q", dc)
		}

		count := pollCount.Add(1)
		if count < 3 {
			// First 2 polls: authorization_pending
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":             "authorization_pending",
				"error_description": "The user has not yet authorized",
			})
			return
		}

		// Third poll: success
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-device-tok",
			"refresh_token": "refresh-device-tok",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := DeviceAuthConfig{
		ClientID:      "test-client",
		DeviceAuthURL: srv.URL + "/device/code",
		TokenURL:      srv.URL + "/token",
		Scopes:        []string{"user:inference"},
		PollInterval:  10 * time.Millisecond, // fast for testing
	}

	var gotCode DeviceCodeResponse
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tr, err := StartDeviceAuthFlow(ctx, cfg, func(dcr DeviceCodeResponse) {
		gotCode = dcr
	})
	if err != nil {
		t.Fatalf("StartDeviceAuthFlow: %v", err)
	}

	// Verify device code callback.
	if gotCode.UserCode != "USER-CODE" {
		t.Errorf("callback UserCode: got %q, want USER-CODE", gotCode.UserCode)
	}

	// Verify token result.
	if tr.AccessToken != "access-device-tok" {
		t.Errorf("AccessToken: got %q, want access-device-tok", tr.AccessToken)
	}
	if tr.RefreshToken != "refresh-device-tok" {
		t.Errorf("RefreshToken: got %q, want refresh-device-tok", tr.RefreshToken)
	}
	if tr.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set")
	}

	// Verify we polled at least 3 times (2 pending + 1 success).
	if c := pollCount.Load(); c != 3 {
		t.Errorf("poll count: got %d, want 3", c)
	}
}

// ---------------------------------------------------------------------------
// TestDeviceAuth_AccessDenied
// ---------------------------------------------------------------------------

func TestDeviceAuth_AccessDenied(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceCodeResponse{
			DeviceCode: "dev-denied",
			UserCode:   "DENY-CODE",
			ExpiresIn:  300,
			Interval:   1,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "access_denied",
			"error_description": "The user denied the request",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := DeviceAuthConfig{
		ClientID:      "test-client",
		DeviceAuthURL: srv.URL + "/device/code",
		TokenURL:      srv.URL + "/token",
		PollInterval:  10 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := StartDeviceAuthFlow(ctx, cfg, nil)
	if err == nil {
		t.Fatal("expected error for access_denied")
	}
	want := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAuthDeviceAuthorizationDenied)
	if got := err.Error(); got != want {
		t.Errorf("denial error = %q, want localized semantic copy %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// TestDeviceAuth_SlowDown — verify interval increases on slow_down
// ---------------------------------------------------------------------------

func TestDeviceAuth_SlowDown(t *testing.T) {
	var pollCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceCodeResponse{
			DeviceCode: "dev-slow",
			UserCode:   "SLOW-CODE",
			ExpiresIn:  300,
			Interval:   1,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		count := pollCount.Add(1)
		if count == 1 {
			// First poll: slow_down
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
			return
		}
		// Second poll: success
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "access-slow-tok",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := DeviceAuthConfig{
		ClientID:      "test-client",
		DeviceAuthURL: srv.URL + "/device/code",
		TokenURL:      srv.URL + "/token",
		PollInterval:  10 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tr, err := StartDeviceAuthFlow(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("StartDeviceAuthFlow: %v", err)
	}
	if tr.AccessToken != "access-slow-tok" {
		t.Errorf("AccessToken: got %q", tr.AccessToken)
	}
	if c := pollCount.Load(); c != 2 {
		t.Errorf("poll count: got %d, want 2", c)
	}
}

// ---------------------------------------------------------------------------
// TestDeviceAuth_Cancelled
// ---------------------------------------------------------------------------

func TestDeviceAuth_Cancelled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceCodeResponse{
			DeviceCode: "dev-cancel",
			UserCode:   "CANCEL-CODE",
			ExpiresIn:  300,
			Interval:   1,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := DeviceAuthConfig{
		ClientID:      "test-client",
		DeviceAuthURL: srv.URL + "/device/code",
		TokenURL:      srv.URL + "/token",
		PollInterval:  10 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := StartDeviceAuthFlow(ctx, cfg, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// TestProviderConfigs — verify pre-configured OAuth configs are non-empty
// ---------------------------------------------------------------------------

func TestProviderConfigs(t *testing.T) {
	oc := AnthropicOAuthConfig()
	if oc.ClientID == "" || oc.AuthURL == "" || oc.TokenURL == "" {
		t.Errorf("AnthropicOAuthConfig has empty fields: %+v", oc)
	}
	if oc.ClientID != anthropicOAuthClientID {
		t.Errorf("AnthropicOAuthConfig client_id = %q", oc.ClientID)
	}
	if oc.AuthURL != anthropicOAuthAuthorizeURL {
		t.Errorf("AnthropicOAuthConfig auth_url = %q", oc.AuthURL)
	}
	if oc.TokenURL != anthropicOAuthTokenURL {
		t.Errorf("AnthropicOAuthConfig token_url = %q", oc.TokenURL)
	}
	if len(oc.Scopes) != len(anthropicOAuthScopes) {
		t.Fatalf("AnthropicOAuthConfig scopes = %v", oc.Scopes)
	}
	for i, scope := range anthropicOAuthScopes {
		if oc.Scopes[i] != scope {
			t.Fatalf("AnthropicOAuthConfig scope[%d] = %q, want %q", i, oc.Scopes[i], scope)
		}
	}

	dc := AnthropicDeviceAuthConfig()
	if dc.ClientID == "" || dc.DeviceAuthURL == "" || dc.TokenURL == "" {
		t.Errorf("AnthropicDeviceAuthConfig has empty fields: %+v", dc)
	}
	if dc.ClientID != anthropicOAuthClientID {
		t.Errorf("AnthropicDeviceAuthConfig client_id = %q", dc.ClientID)
	}
	if dc.DeviceAuthURL != anthropicOAuthDeviceURL {
		t.Errorf("AnthropicDeviceAuthConfig device_url = %q", dc.DeviceAuthURL)
	}
	if dc.TokenURL != anthropicOAuthTokenURL {
		t.Errorf("AnthropicDeviceAuthConfig token_url = %q", dc.TokenURL)
	}
}

// ---------------------------------------------------------------------------
// TestOAuthHookAdapter
// ---------------------------------------------------------------------------

func TestOAuthHookAdapter_LoadAccessToken(t *testing.T) {
	dir := t.TempDir()
	store := newStoreAt(dir)

	// Save a valid credential.
	creds := &Credentials{
		AccessToken:  "valid-token",
		RefreshToken: "refresh-tok",
		ExpiresAt:    time.Now().Add(time.Hour),
		TokenType:    "Bearer",
	}
	if err := store.SaveCredentials(creds); err != nil {
		t.Fatal(err)
	}

	hook := NewOAuthHookAdapter(store, AnthropicOAuthConfig())
	token, err := hook.LoadAccessToken(context.Background())
	if err != nil {
		t.Fatalf("LoadAccessToken: %v", err)
	}
	if token != "valid-token" {
		t.Errorf("token: got %q, want valid-token", token)
	}
}

func TestOAuthHookAdapter_NoCredentials(t *testing.T) {
	store := newStoreAt(t.TempDir())

	hook := NewOAuthHookAdapter(store, AnthropicOAuthConfig())
	token, err := hook.LoadAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
