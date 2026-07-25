package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TestGenerateCodeVerifier
// ---------------------------------------------------------------------------

func TestGenerateCodeVerifier(t *testing.T) {
	verifier, challenge, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier returned error: %v", err)
	}

	// RFC 7636: verifier must be 43–128 URL-safe characters.
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("verifier length %d is outside 43-128 range", len(verifier))
	}

	// Verify the challenge is BASE64URL(SHA256(verifier)).
	h := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(h[:])
	if challenge != wantChallenge {
		t.Errorf("challenge mismatch:\n  got  %s\n  want %s", challenge, wantChallenge)
	}

	// Two calls should produce distinct verifiers.
	v2, c2, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("second GenerateCodeVerifier returned error: %v", err)
	}
	if verifier == v2 || challenge == c2 {
		t.Error("two consecutive GenerateCodeVerifier calls returned identical output")
	}
}

// ---------------------------------------------------------------------------
// TestSaveLoadCredentials — round-trip with temp dir
// ---------------------------------------------------------------------------

func TestSaveLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(dir)

	now := time.Now().Truncate(time.Second).UTC()
	creds := &Credentials{
		AccessToken:  "tok-access",
		RefreshToken: "tok-refresh",
		ExpiresAt:    now.Add(time.Hour),
		TokenType:    "Bearer",
	}

	if err := s.SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	loaded, err := s.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadCredentials returned nil after save")
	}
	if loaded.AccessToken != creds.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", loaded.AccessToken, creds.AccessToken)
	}
	if loaded.RefreshToken != creds.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", loaded.RefreshToken, creds.RefreshToken)
	}
	if !loaded.ExpiresAt.Equal(creds.ExpiresAt) {
		t.Errorf("ExpiresAt: got %v, want %v", loaded.ExpiresAt, creds.ExpiresAt)
	}
}

func TestLoadCredentialsNotExist(t *testing.T) {
	s := newStoreAt(t.TempDir())
	creds, err := s.LoadCredentials()
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if creds != nil {
		t.Errorf("expected nil credentials for missing file, got %+v", creds)
	}
}

func TestNewStoreWritesLUBANCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.SaveCredentials(&Credentials{AccessToken: "luban-token", TokenType: "Bearer"}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".luban-code", credentialsFile)); err != nil {
		t.Fatalf("expected LUBAN credentials: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestIsExpired
// ---------------------------------------------------------------------------

func TestIsExpired(t *testing.T) {
	t.Run("nil credentials", func(t *testing.T) {
		if !IsExpired(nil) {
			t.Error("expected IsExpired(nil) == true")
		}
	})

	t.Run("expired", func(t *testing.T) {
		c := &Credentials{ExpiresAt: time.Now().Add(-time.Hour)}
		if !IsExpired(c) {
			t.Error("expected IsExpired to be true for past expiry")
		}
	})

	t.Run("within 5-minute buffer", func(t *testing.T) {
		c := &Credentials{ExpiresAt: time.Now().Add(4 * time.Minute)}
		if !IsExpired(c) {
			t.Error("expected IsExpired to be true when within 5-minute buffer")
		}
	})

	t.Run("not expired", func(t *testing.T) {
		c := &Credentials{ExpiresAt: time.Now().Add(10 * time.Minute)}
		if IsExpired(c) {
			t.Error("expected IsExpired to be false for future expiry beyond buffer")
		}
	})

	t.Run("zero expiry", func(t *testing.T) {
		c := &Credentials{} // zero ExpiresAt
		if IsExpired(c) {
			t.Error("expected IsExpired to be false when ExpiresAt is zero (unknown expiry)")
		}
	})
}

// ---------------------------------------------------------------------------
// TestRefreshToken — mock HTTP server returning new tokens
// ---------------------------------------------------------------------------

func TestRefreshToken(t *testing.T) {
	wantAccess := "new-access-token"
	wantRefresh := "new-refresh-token"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if gt := r.FormValue("grant_type"); gt != "refresh_token" {
			t.Errorf("grant_type: got %q, want refresh_token", gt)
		}
		if rt := r.FormValue("refresh_token"); rt != "old-refresh" {
			t.Errorf("refresh_token: got %q, want old-refresh", rt)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  wantAccess,
			"refresh_token": wantRefresh,
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	cfg := OAuthConfig{ClientID: "test-client", TokenURL: srv.URL}
	tr, err := RefreshToken(context.Background(), cfg, "old-refresh")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if tr.AccessToken != wantAccess {
		t.Errorf("AccessToken: got %q, want %q", tr.AccessToken, wantAccess)
	}
	if tr.RefreshToken != wantRefresh {
		t.Errorf("RefreshToken: got %q, want %q", tr.RefreshToken, wantRefresh)
	}
	if tr.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set when expires_in > 0")
	}
}

func TestRefreshTokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	cfg := OAuthConfig{ClientID: "test-client", TokenURL: srv.URL}
	_, err := RefreshToken(context.Background(), cfg, "bad-token")
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
}

// ---------------------------------------------------------------------------
// TestOAuthCallbackServer — start server, simulate callback, verify code exchange
// ---------------------------------------------------------------------------

func TestOAuthCallbackServer(t *testing.T) {
	// Stand up a mock token endpoint.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if gt := r.FormValue("grant_type"); gt != "authorization_code" {
			t.Errorf("grant_type: got %q, want authorization_code", gt)
		}
		if code := r.FormValue("code"); code != "test-code-123" {
			t.Errorf("code: got %q, want test-code-123", code)
		}
		if cv := r.FormValue("code_verifier"); cv == "" {
			t.Error("code_verifier must not be empty")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access-abc",
			"refresh_token": "refresh-xyz",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	cfg := OAuthConfig{
		ClientID: "test-client",
		AuthURL:  "https://example.com/oauth/authorize", // never called in this test
		TokenURL: tokenServer.URL,
	}

	authURLCh := make(chan string, 1)
	resultCh := make(chan *TokenResponse, 1)
	flowErrCh := make(chan error, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		tr, err := StartOAuthFlowWithURL(ctx, cfg, authURLCh)
		if err != nil {
			flowErrCh <- err
			return
		}
		resultCh <- tr
	}()

	// Retrieve the auth URL to extract state and redirect_uri.
	var authURL string
	select {
	case authURL = <-authURLCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for auth URL")
	}

	// Parse query parameters from the auth URL using net/http helper.
	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	params := req.URL.Query()
	redirectURI := params.Get("redirect_uri")
	state := params.Get("state")
	if redirectURI == "" || state == "" {
		t.Fatalf("missing redirect_uri or state in auth URL params: %v", params)
	}

	// Simulate browser redirect: hit the callback with the code + state.
	callbackURL := redirectURI + "?code=test-code-123&state=" + state
	resp, err := http.Get(callbackURL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	resp.Body.Close()

	// Wait for the flow to complete.
	select {
	case tr := <-resultCh:
		if tr.AccessToken != "access-abc" {
			t.Errorf("AccessToken: got %q, want access-abc", tr.AccessToken)
		}
	case err := <-flowErrCh:
		t.Fatalf("OAuth flow error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for token response")
	}
}

// ---------------------------------------------------------------------------
// TestAtomicWrite — verify file permissions are 0600
// ---------------------------------------------------------------------------

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(dir)

	creds := &Credentials{AccessToken: "tok", TokenType: "Bearer"}
	if err := s.SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, credentialsFile))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX owner-only file mode semantics")
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credentials file permissions: got %04o, want 0600", perm)
	}
}
