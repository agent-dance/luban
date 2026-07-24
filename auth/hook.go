package auth

import (
	"context"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

// OAuthHookAdapter bridges the auth.Store with the provider.OAuthHook interface.
// It loads access tokens (refreshing if expired) and handles 401 auto-refresh.
//
// This adapter avoids import cycles: the provider package defines OAuthHook as
// an interface, and auth implements it without importing provider.
type OAuthHookAdapter struct {
	store  *Store
	config OAuthConfig
	mu     sync.Mutex // serializes refresh attempts
}

// NewOAuthHookAdapter creates an OAuthHookAdapter.
func NewOAuthHookAdapter(store *Store, config OAuthConfig) *OAuthHookAdapter {
	return &OAuthHookAdapter{store: store, config: config}
}

// LoadAccessToken returns a valid access token, refreshing if needed.
// Returns ("", nil) if no credentials are stored.
func (h *OAuthHookAdapter) LoadAccessToken(ctx context.Context) (string, error) {
	creds, err := h.store.LoadCredentials()
	if err != nil {
		return "", err
	}
	if creds == nil {
		return "", nil // no credentials stored
	}

	if !IsExpired(creds) {
		return creds.AccessToken, nil
	}

	// Credentials expired — try to refresh.
	if creds.RefreshToken == "" {
		return "", i18n.NewError(i18n.KeyAuthOAuthCredentialsExpired)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Double-check after acquiring lock (another goroutine may have refreshed).
	creds, err = h.store.LoadCredentials()
	if err != nil {
		return "", err
	}
	if creds != nil && !IsExpired(creds) {
		return creds.AccessToken, nil
	}

	refreshed, err := h.store.EnsureValid(ctx, h.config)
	if err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

// OnAuthError is called when a 401 Unauthorized error is received.
// It attempts to refresh the token. Returns true if the token was
// successfully refreshed and a retry is warranted.
func (h *OAuthHookAdapter) OnAuthError() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	creds, err := h.store.LoadCredentials()
	if err != nil || creds == nil || creds.RefreshToken == "" {
		return false
	}

	refreshed, err := RefreshToken(context.Background(), h.config, creds.RefreshToken)
	if err != nil {
		return false
	}

	// Preserve existing refresh token if the server didn't return a new one.
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = creds.RefreshToken
	}

	newCreds := credentialsFromTokenResponse(refreshed)
	newCreds.Provider = creds.Provider
	newCreds.OrganizationUUID = creds.OrganizationUUID
	if len(newCreds.Scopes) == 0 {
		newCreds.Scopes = append([]string(nil), creds.Scopes...)
	}
	if err := h.store.SaveCredentials(newCreds); err != nil {
		return false
	}

	return true
}
