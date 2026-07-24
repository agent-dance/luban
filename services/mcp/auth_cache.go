package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultNeedsAuthTTL = 15 * time.Minute

// NeedsAuthState records that a remote MCP server should surface an
// authenticate pseudo-tool instead of repeatedly probing a known-401 endpoint.
type NeedsAuthState struct {
	ServerName          string         `json:"serverName"`
	ServerURL           string         `json:"serverUrl"`
	Transport           TransportType  `json:"transport,omitempty"`
	StatusCode          int            `json:"statusCode"`
	Reason              string         `json:"reason"`
	Challenge           *AuthChallenge `json:"challenge,omitempty"`
	Scope               string         `json:"scope,omitempty"`
	ResourceMetadataURL string         `json:"resourceMetadataUrl,omitempty"`
	RecordedAt          time.Time      `json:"recordedAt"`
	ExpiresAt           time.Time      `json:"expiresAt"`
}

// Expired reports whether the cache entry is no longer valid.
func (s NeedsAuthState) Expired(now time.Time) bool {
	return !s.ExpiresAt.IsZero() && !now.Before(s.ExpiresAt)
}

// NeedsAuthCache is a small TTL cache keyed by server credential key.
type NeedsAuthCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	now     func() time.Time
	entries map[string]NeedsAuthState
}

// NewNeedsAuthCache constructs a needs-auth cache. A zero ttl uses the TS
// parity default of 15 minutes.
func NewNeedsAuthCache(ttl time.Duration) *NeedsAuthCache {
	if ttl <= 0 {
		ttl = defaultNeedsAuthTTL
	}
	return &NeedsAuthCache{
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[string]NeedsAuthState),
	}
}

// SetNow overrides the clock for tests.
func (c *NeedsAuthCache) SetNow(now func() time.Time) {
	if c == nil || now == nil {
		return
	}
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

// Put stores a needs-auth state.
func (c *NeedsAuthCache) Put(serverKey string, state NeedsAuthState) NeedsAuthState {
	if c == nil {
		return state
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	state.RecordedAt = now
	state.ExpiresAt = now.Add(c.ttl)
	if c.entries == nil {
		c.entries = make(map[string]NeedsAuthState)
	}
	c.entries[serverKey] = state
	return state
}

// Get returns a non-expired needs-auth state.
func (c *NeedsAuthCache) Get(serverKey string) (NeedsAuthState, bool) {
	if c == nil {
		return NeedsAuthState{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.entries[serverKey]
	if !ok {
		return NeedsAuthState{}, false
	}
	if state.Expired(c.now()) {
		delete(c.entries, serverKey)
		return NeedsAuthState{}, false
	}
	return state, true
}

// Clear removes a needs-auth state.
func (c *NeedsAuthCache) Clear(serverKey string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, serverKey)
	c.mu.Unlock()
}

// ClearServerURL removes all entries for serverURL.
func (c *NeedsAuthCache) ClearServerURL(serverURL string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, state := range c.entries {
		if state.ServerURL == serverURL {
			delete(c.entries, key)
		}
	}
}

// DefaultNeedsAuthCache is the process-wide cache used by helper functions.
var DefaultNeedsAuthCache = NewNeedsAuthCache(defaultNeedsAuthTTL)

// NeedsAuthFromHTTPResponse converts 401 and 403 insufficient_scope responses
// into a cacheable needs-auth state.
func NeedsAuthFromHTTPResponse(serverName string, cfg MCPServerConfig, resp *http.Response) (NeedsAuthState, bool) {
	if resp == nil {
		return NeedsAuthState{}, false
	}
	challenge := ParseWWWAuthenticate(resp)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return needsAuthState(serverName, cfg, resp.StatusCode, "unauthorized", challenge), true
	case http.StatusForbidden:
		if challenge != nil && strings.EqualFold(challenge.ErrorCode, "insufficient_scope") {
			state := needsAuthState(serverName, cfg, resp.StatusCode, "insufficient_scope", challenge)
			state.Scope = challenge.Scope
			state.ResourceMetadataURL = challenge.ResourceMetadataURL
			return state, true
		}
	}
	return NeedsAuthState{}, false
}

// RecordNeedsAuthFromHTTPResponse records a cache entry for a qualifying HTTP
// response.
func RecordNeedsAuthFromHTTPResponse(cache *NeedsAuthCache, serverName string, cfg MCPServerConfig, resp *http.Response) (NeedsAuthState, bool) {
	state, ok := NeedsAuthFromHTTPResponse(serverName, cfg, resp)
	if !ok {
		return NeedsAuthState{}, false
	}
	if cache == nil {
		cache = DefaultNeedsAuthCache
	}
	return cache.Put(ServerKey(serverName, cfg), state), true
}

// RecordNeedsAuthFromError records needs-auth state from typed remote errors.
func RecordNeedsAuthFromError(cache *NeedsAuthCache, serverName string, cfg MCPServerConfig, err error) (NeedsAuthState, bool) {
	if err == nil {
		return NeedsAuthState{}, false
	}
	var unauthorized *UnauthorizedError
	if errors.As(err, &unauthorized) {
		state := needsAuthState(serverName, cfg, unauthorized.StatusCode, "unauthorized", unauthorized.Challenge)
		if cache == nil {
			cache = DefaultNeedsAuthCache
		}
		return cache.Put(ServerKey(serverName, cfg), state), true
	}
	var remote *RemoteHTTPError
	if errors.As(err, &remote) && remote.StatusCode == http.StatusForbidden && remote.Challenge != nil && strings.EqualFold(remote.Challenge.ErrorCode, "insufficient_scope") {
		state := needsAuthState(serverName, cfg, remote.StatusCode, "insufficient_scope", remote.Challenge)
		state.Scope = remote.Challenge.Scope
		state.ResourceMetadataURL = remote.Challenge.ResourceMetadataURL
		if cache == nil {
			cache = DefaultNeedsAuthCache
		}
		return cache.Put(ServerKey(serverName, cfg), state), true
	}
	return NeedsAuthState{}, false
}

// HasMCPNeedsAuth reports whether a server currently has a non-expired
// needs-auth entry.
func HasMCPNeedsAuth(cache *NeedsAuthCache, serverName string, cfg MCPServerConfig) (NeedsAuthState, bool) {
	if cache == nil {
		cache = DefaultNeedsAuthCache
	}
	return cache.Get(ServerKey(serverName, cfg))
}

func needsAuthState(serverName string, cfg MCPServerConfig, status int, reason string, challenge *AuthChallenge) NeedsAuthState {
	state := NeedsAuthState{
		ServerName: serverName,
		ServerURL:  cfg.URL,
		Transport:  cfg.Type,
		StatusCode: status,
		Reason:     reason,
		Challenge:  challenge,
	}
	if challenge != nil {
		state.Scope = challenge.Scope
		state.ResourceMetadataURL = challenge.ResourceMetadataURL
	}
	return state
}

// NeedsAuthTokenSource wraps another TokenSource and clears needs-auth state
// once a token is successfully resolved.
type NeedsAuthTokenSource struct {
	Base  TokenSource
	Cache *NeedsAuthCache
}

// TokenFor implements TokenSource.
func (s NeedsAuthTokenSource) TokenFor(ctx context.Context, serverURL string) (string, error) {
	if s.Base == nil {
		return "", nil
	}
	token, err := s.Base.TokenFor(ctx, serverURL)
	if err == nil && strings.TrimSpace(token) != "" {
		cache := s.Cache
		if cache == nil {
			cache = DefaultNeedsAuthCache
		}
		cache.ClearServerURL(serverURL)
	}
	return token, err
}
