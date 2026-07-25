package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// authHTTPClient is a shared HTTP client with a 30-second timeout used for all
// OAuth token endpoint requests. Using a dedicated client (instead of
// http.DefaultClient) ensures timeouts are always enforced and the transport
// can be swapped in tests.
var authHTTPClient = &http.Client{Timeout: 30 * time.Second}

// OAuthConfig holds the configuration for an OAuth2 PKCE flow.
type OAuthConfig struct {
	ClientID    string
	AuthURL     string
	TokenURL    string
	RedirectURI string // if empty, a localhost redirect is auto-assigned
	Scopes      []string
}

// TokenResponse is the decoded response from the token endpoint.
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"` // seconds
	ExpiresAt    time.Time `json:"-"`          // derived from ExpiresIn at parse time
	Scope        string    `json:"scope"`
}

// GenerateCodeVerifier generates a PKCE code verifier (43-128 chars, URL-safe
// base64 with no padding) and its corresponding SHA-256 code challenge.
// Returns (verifier, challenge, err).
func GenerateCodeVerifier() (string, string, error) {
	// 96 bytes → 128-char base64url string (within the 43-128 RFC 7636 limit).
	b := make([]byte, 96)
	if _, err := rand.Read(b); err != nil {
		return "", "", i18n.WrapError(i18n.KeyAuthOAuthGenerateVerifier, err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)

	// S256 challenge: BASE64URL(SHA256(ASCII(verifier)))
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	return verifier, challenge, nil
}

// buildAuthURL constructs the authorization URL with PKCE and state parameters.
func buildAuthURL(cfg OAuthConfig, challenge, state, redirectURI string) (string, error) {
	u, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return "", i18n.WrapError(i18n.KeyAuthOAuthInvalidAuthorizationURL, err, cfg.AuthURL)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	if len(cfg.Scopes) > 0 {
		q.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// StartOAuthFlowWithURL is like StartOAuthFlow but sends the authorization URL
// to authURLOut so the caller can display it before blocking.
func StartOAuthFlowWithURL(ctx context.Context, cfg OAuthConfig, authURLOut chan<- string) (*TokenResponse, error) {
	return startOAuthFlowInternal(ctx, cfg, authURLOut)
}

func startOAuthFlowInternal(ctx context.Context, cfg OAuthConfig, authURLOut chan<- string) (*TokenResponse, error) {
	// Generate PKCE credentials.
	verifier, challenge, err := GenerateCodeVerifier()
	if err != nil {
		return nil, err
	}

	// Generate random state for CSRF protection.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthGenerateState, err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	// Start local callback server on a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthStartCallbackServer, err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := cfg.RedirectURI
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	}

	// Channels to receive the authorization code or an error from the handler.
	codeCh := make(chan string, 1)
	handlerErrCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		lang := i18n.DetectOrLoadLanguage()

		if gotState := q.Get("state"); gotState != state {
			http.Error(w, i18n.Text(lang, i18n.KeyOAuthInvalidState), http.StatusBadRequest)
			handlerErrCh <- i18n.NewError(i18n.KeyAuthOAuthCallbackStateMismatch, gotState)
			return
		}

		if errParam := q.Get("error"); errParam != "" {
			desc := q.Get("error_description")
			http.Error(w, i18n.Format(lang, i18n.KeyOAuthAuthorizationDenied, errParam), http.StatusBadRequest)
			handlerErrCh <- i18n.NewError(i18n.KeyAuthOAuthAuthorizationError, errParam, desc)
			return
		}

		code := q.Get("code")
		if code == "" {
			http.Error(w, i18n.Text(lang, i18n.KeyOAuthMissingCode), http.StatusBadRequest)
			handlerErrCh <- i18n.NewError(i18n.KeyAuthOAuthCallbackMissingCode)
			return
		}

		fmt.Fprintln(w, i18n.Text(lang, i18n.KeyOAuthAuthorizationSuccess))
		codeCh <- code
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		// Let an in-flight callback finish writing its browser response before
		// closing the connection. Close can otherwise race a fast token exchange
		// and leave the browser (or test client) with an EOF.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
		}
	}()

	// Build and optionally emit the authorization URL.
	authURL, err := buildAuthURL(cfg, challenge, state, redirectURI)
	if err != nil {
		return nil, err
	}
	if authURLOut != nil {
		select {
		case authURLOut <- authURL:
		default:
		}
	}

	// Wait for code, error, or context cancellation.
	var code string
	select {
	case <-ctx.Done():
		return nil, i18n.WrapError(i18n.KeyAuthOAuthFlowCancelled, ctx.Err())
	case err := <-handlerErrCh:
		return nil, err
	case code = <-codeCh:
	}

	return exchangeCode(ctx, cfg, code, verifier, redirectURI)
}

// exchangeCode exchanges an authorization code for tokens using PKCE.
func exchangeCode(ctx context.Context, cfg OAuthConfig, code, verifier, redirectURI string) (*TokenResponse, error) {
	vals := url.Values{}
	vals.Set("grant_type", "authorization_code")
	vals.Set("code", code)
	vals.Set("redirect_uri", redirectURI)
	vals.Set("client_id", cfg.ClientID)
	vals.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthBuildTokenRequest, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := authHTTPClient.Do(req)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthTokenRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, i18n.NewError(i18n.KeyAuthOAuthTokenEndpointRejected, resp.StatusCode, string(errBody))
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthDecodeTokenResponse, err)
	}
	if tr.ExpiresIn > 0 {
		tr.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return &tr, nil
}
