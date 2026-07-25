package auth

import (
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
	Transport           string         `json:"transport,omitempty"`
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

// NewNeedsAuthCache constructs a needs-auth cache. A zero ttl uses 15 minutes.
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

var defaultNeedsAuthCache = NewNeedsAuthCache(defaultNeedsAuthTTL)

// RecordNeedsAuthFromError classifies an auth failure and caches its state.
func RecordNeedsAuthFromError(cache *NeedsAuthCache, serverName string, cfg ServerDescriptor, err error) (NeedsAuthState, bool) {
	if err == nil {
		return NeedsAuthState{}, false
	}
	var unauthorized *UnauthorizedError
	if errors.As(err, &unauthorized) {
		state := needsAuthState(serverName, cfg, unauthorized.StatusCode, "unauthorized", unauthorized.Challenge)
		if cache == nil {
			cache = defaultNeedsAuthCache
		}
		return cache.Put(ServerKey(serverName, cfg), state), true
	}
	var remote HTTPAuthError
	if !errors.As(err, &remote) {
		return NeedsAuthState{}, false
	}
	challenge := remote.AuthChallenge()
	if remote.AuthStatusCode() == http.StatusUnauthorized {
		state := needsAuthState(serverName, cfg, remote.AuthStatusCode(), "unauthorized", challenge)
		if cache == nil {
			cache = defaultNeedsAuthCache
		}
		return cache.Put(ServerKey(serverName, cfg), state), true
	}
	if remote.AuthStatusCode() == http.StatusForbidden && challenge != nil && strings.EqualFold(challenge.ErrorCode, "insufficient_scope") {
		state := needsAuthState(serverName, cfg, remote.AuthStatusCode(), "insufficient_scope", challenge)
		state.Scope = challenge.Scope
		state.ResourceMetadataURL = challenge.ResourceMetadataURL
		if cache == nil {
			cache = defaultNeedsAuthCache
		}
		return cache.Put(ServerKey(serverName, cfg), state), true
	}
	return NeedsAuthState{}, false
}

// LookupNeedsAuth returns the current cached auth state for a server.
func LookupNeedsAuth(cache *NeedsAuthCache, serverName string, cfg ServerDescriptor) (NeedsAuthState, bool) {
	if cache == nil {
		cache = defaultNeedsAuthCache
	}
	return cache.Get(ServerKey(serverName, cfg))
}

func needsAuthState(serverName string, cfg ServerDescriptor, status int, reason string, challenge *AuthChallenge) NeedsAuthState {
	state := NeedsAuthState{
		ServerName: serverName,
		ServerURL:  cfg.URL,
		Transport:  cfg.Transport,
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
