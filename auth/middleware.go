package auth

import (
	"context"
	"fmt"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// contextKey is an unexported type for context values in this package.
type contextKey string

const authTokenKey contextKey = "auth_token"

// AuthMiddleware wraps a provider.Provider and automatically injects OAuth
// access tokens, refreshing expired credentials transparently.
type AuthMiddleware struct {
	inner  provider.Provider
	store  *Store
	config OAuthConfig
}

// NewAuthMiddleware creates an AuthMiddleware wrapping inner.
func NewAuthMiddleware(inner provider.Provider, store *Store, config OAuthConfig) *AuthMiddleware {
	return &AuthMiddleware{inner: inner, store: store, config: config}
}

// Name returns "oauth:<inner provider name>".
func (m *AuthMiddleware) Name() string { return "oauth:" + m.inner.Name() }

// ModelID delegates to the inner provider.
func (m *AuthMiddleware) ModelID() string { return m.inner.ModelID() }

// CreateStream ensures credentials are valid (refreshing if necessary),
// injects the Authorization token into the request context, and delegates
// to the inner provider.
func (m *AuthMiddleware) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	creds, err := m.store.EnsureValid(ctx, m.config)
	if err != nil {
		return nil, fmt.Errorf("auth middleware: %w", err)
	}

	tokenType := creds.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	token := tokenType + " " + creds.AccessToken

	// Inject the token into the context and delegate.
	enriched := context.WithValue(ctx, authTokenKey, token)
	return m.inner.CreateStream(enriched, params)
}

// Capabilities delegates to the inner provider if it implements
// provider.CapabilityProvider, preserving the optional interface.
func (m *AuthMiddleware) Capabilities() provider.ProviderCapabilities {
	if cp, ok := m.inner.(provider.CapabilityProvider); ok {
		return cp.Capabilities()
	}
	return provider.ProviderCapabilities{}
}

// TokenFromContext retrieves the injected Authorization token from a context.
// Returns an empty string if no token has been injected.
func TokenFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(authTokenKey).(string); ok {
		return v
	}
	return ""
}
