package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// OAuthTokens is the token endpoint shape used by MCP OAuth flows.
type OAuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuthClientInformation is the stored client credentials for a server.
type OAuthClientInformation struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// OAuthDiscoveryState persists the compact discovery pointers that are safe to
// keep in local storage. Metadata is intentionally re-fetched to avoid storing
// large provider documents.
type OAuthDiscoveryState struct {
	AuthorizationServerURL string `json:"authorizationServerUrl,omitempty"`
	ResourceMetadataURL    string `json:"resourceMetadataUrl,omitempty"`
}

// StoredOAuthCredentials is the per-MCP-server OAuth record. It mirrors the
// TypeScript secure-storage record while remaining usable with a file fallback.
type StoredOAuthCredentials struct {
	ServerName     string              `json:"serverName"`
	ServerURL      string              `json:"serverUrl"`
	AccessToken    string              `json:"accessToken,omitempty"`
	RefreshToken   string              `json:"refreshToken,omitempty"`
	TokenType      string              `json:"tokenType,omitempty"`
	ExpiresAt      time.Time           `json:"expiresAt,omitempty"`
	Scope          string              `json:"scope,omitempty"`
	ClientID       string              `json:"clientId,omitempty"`
	ClientSecret   string              `json:"clientSecret,omitempty"`
	DiscoveryState OAuthDiscoveryState `json:"discoveryState,omitempty"`
	StepUpScope    string              `json:"stepUpScope,omitempty"`
	Extra          map[string]any      `json:"extra,omitempty"`
	UpdatedAt      time.Time           `json:"updatedAt,omitempty"`
}

// TokenStore persists OAuth credentials and client information.
type TokenStore interface {
	Load(ctx context.Context, serverKey string) (StoredOAuthCredentials, bool, error)
	LoadByServerURL(ctx context.Context, serverURL string) (string, StoredOAuthCredentials, bool, error)
	Save(ctx context.Context, serverKey string, creds StoredOAuthCredentials) error
	Clear(ctx context.Context, serverKey string) error
}

// ServerKey returns the TS-compatible server credential key: name plus a
// stable hash of the security-relevant remote config.
func ServerKey(serverName string, cfg MCPServerConfig) string {
	return ServerKeyFromFields(serverName, string(cfg.Type), cfg.URL, cfg.Headers)
}

// ServerKeyFromFields is a small adapter for callers that do not use
// services/mcp.MCPServerConfig directly.
func ServerKeyFromFields(serverName, transport, serverURL string, headers map[string]string) string {
	payload := struct {
		Type    string            `json:"type"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}{
		Type:    transport,
		URL:     serverURL,
		Headers: sortedHeaderMap(headers),
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%s|%s", serverName, hex.EncodeToString(sum[:])[:16])
}

func sortedHeaderMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(in))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}

// NewStoredOAuthCredentials builds a stored record from a token endpoint
// response.
func NewStoredOAuthCredentials(serverName string, cfg MCPServerConfig, tokens OAuthTokens, now time.Time) StoredOAuthCredentials {
	expiresIn := tokens.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	tokenType := tokens.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	return StoredOAuthCredentials{
		ServerName:   serverName,
		ServerURL:    cfg.URL,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    tokenType,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second),
		Scope:        tokens.Scope,
		UpdatedAt:    now,
	}
}

// ExpiresIn returns the remaining token lifetime in seconds.
func (c StoredOAuthCredentials) ExpiresIn(now time.Time) int {
	if c.ExpiresAt.IsZero() {
		return 0
	}
	return int(c.ExpiresAt.Sub(now).Seconds())
}

func (c StoredOAuthCredentials) toOAuthTokens(now time.Time) OAuthTokens {
	tokenType := c.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	return OAuthTokens{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		TokenType:    tokenType,
		ExpiresIn:    int(c.ExpiresAt.Sub(now).Seconds()),
		Scope:        c.Scope,
	}
}

// MemoryTokenStore is a concurrency-safe in-memory store for tests and
// embedded callers.
type MemoryTokenStore struct {
	mu      sync.RWMutex
	entries map[string]StoredOAuthCredentials
}

// NewMemoryTokenStore constructs an empty in-memory token store.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{entries: make(map[string]StoredOAuthCredentials)}
}

// Load returns credentials by server key.
func (s *MemoryTokenStore) Load(_ context.Context, serverKey string) (StoredOAuthCredentials, bool, error) {
	if s == nil {
		return StoredOAuthCredentials{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	creds, ok := s.entries[serverKey]
	return creds, ok, nil
}

// LoadByServerURL returns the first credential record matching serverURL.
func (s *MemoryTokenStore) LoadByServerURL(_ context.Context, serverURL string) (string, StoredOAuthCredentials, bool, error) {
	if s == nil {
		return "", StoredOAuthCredentials{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for key, creds := range s.entries {
		if creds.ServerURL == serverURL {
			return key, creds, true, nil
		}
	}
	return "", StoredOAuthCredentials{}, false, nil
}

// Save stores credentials by server key.
func (s *MemoryTokenStore) Save(_ context.Context, serverKey string, creds StoredOAuthCredentials) error {
	if s == nil {
		return i18n.NewError(i18n.KeyServicesMCPOAuthMemoryTokenStoreNil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]StoredOAuthCredentials)
	}
	s.entries[serverKey] = creds
	return nil
}

// Clear deletes credentials by server key.
func (s *MemoryTokenStore) Clear(_ context.Context, serverKey string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, serverKey)
	return nil
}

// FileTokenStore is the local fallback when no platform credential store is
// available. The file is written 0600 and contains the same compact record as
// MemoryTokenStore; callers with a real secure store can provide TokenStore
// instead.
type FileTokenStore struct {
	path string
	mu   sync.Mutex
}

// NewFileTokenStore creates a JSON-backed token store at path.
func NewFileTokenStore(path string) *FileTokenStore {
	return &FileTokenStore{path: path}
}

// DefaultTokenStore returns the documented local fallback store.
func DefaultOAuthTokenStore() TokenStore {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return NewFileTokenStore(filepath.Join(dir, "claude-code", "mcp_oauth_tokens.json"))
}

func (s *FileTokenStore) Load(ctx context.Context, serverKey string) (StoredOAuthCredentials, bool, error) {
	data, err := s.readAll(ctx)
	if err != nil {
		return StoredOAuthCredentials{}, false, err
	}
	creds, ok := data[serverKey]
	return creds, ok, nil
}

func (s *FileTokenStore) LoadByServerURL(ctx context.Context, serverURL string) (string, StoredOAuthCredentials, bool, error) {
	data, err := s.readAll(ctx)
	if err != nil {
		return "", StoredOAuthCredentials{}, false, err
	}
	for key, creds := range data {
		if creds.ServerURL == serverURL {
			return key, creds, true, nil
		}
	}
	return "", StoredOAuthCredentials{}, false, nil
}

func (s *FileTokenStore) Save(ctx context.Context, serverKey string, creds StoredOAuthCredentials) error {
	if s == nil || s.path == "" {
		return i18n.NewError(i18n.KeyServicesMCPOAuthTokenStorePathMissing)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.readAllLocked(ctx)
	if err != nil {
		return err
	}
	data[serverKey] = creds
	return s.writeAllLocked(ctx, data)
}

func (s *FileTokenStore) Clear(ctx context.Context, serverKey string) error {
	if s == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.readAllLocked(ctx)
	if err != nil {
		return err
	}
	delete(data, serverKey)
	return s.writeAllLocked(ctx, data)
}

func (s *FileTokenStore) readAll(ctx context.Context) (map[string]StoredOAuthCredentials, error) {
	if s == nil || s.path == "" {
		return map[string]StoredOAuthCredentials{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readAllLocked(ctx)
}

func (s *FileTokenStore) readAllLocked(ctx context.Context) (map[string]StoredOAuthCredentials, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]StoredOAuthCredentials{}, nil
		}
		return nil, i18n.WrapError(i18n.KeyServicesMCPOAuthTokenStoreOperation, err, "read")
	}
	if len(raw) == 0 {
		return map[string]StoredOAuthCredentials{}, nil
	}
	var data map[string]StoredOAuthCredentials
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPOAuthTokenStoreOperation, err, "decode")
	}
	if data == nil {
		data = map[string]StoredOAuthCredentials{}
	}
	return data, nil
}

func (s *FileTokenStore) writeAllLocked(ctx context.Context, data map[string]StoredOAuthCredentials) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPOAuthTokenStoreCreateDir, err)
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPOAuthTokenStoreOperation, err, "encode")
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPOAuthTokenStoreOperation, err, "write")
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPOAuthTokenStoreOperation, err, "replace")
	}
	_ = os.Chmod(s.path, 0o600)
	return nil
}
