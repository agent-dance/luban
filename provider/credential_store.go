package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// CredentialEntry stores authentication credentials for a single provider.
type CredentialEntry struct {
	// Provider is the canonical provider name (e.g. "anthropic", "openai").
	Provider string `json:"provider"`

	// AuthMethod describes how the credential was obtained.
	// Values: "api_key", "oauth"
	AuthMethod string `json:"auth_method"`

	// APIKey is the provider API key, or an optional exchanged API key for OAuth entries.
	APIKey string `json:"api_key,omitempty"`

	// AccessToken is the OAuth access token (for "oauth" auth method).
	AccessToken string `json:"access_token,omitempty"`

	// RefreshToken is the OAuth refresh token (for "oauth" auth method).
	RefreshToken string `json:"refresh_token,omitempty"`

	// ExpiresAt is the expiry time for OAuth tokens.
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// AccountID is the ChatGPT workspace/account identifier used by the Codex backend.
	AccountID string `json:"account_id,omitempty"`

	// ChatGPTPlanType is informational metadata parsed from the OpenAI ID token.
	ChatGPTPlanType string `json:"chatgpt_plan_type,omitempty"`

	// ChatGPTAccountIsFedRAMP routes workspace traffic through the FedRAMP edge.
	ChatGPTAccountIsFedRAMP bool `json:"chatgpt_account_is_fedramp,omitempty"`

	// BaseURL is an optional custom API base URL.
	BaseURL string `json:"base_url,omitempty"`

	// APIStyle selects the compatibility protocol used for model discovery and
	// inference. Empty entries predate compatible aggregate providers.
	APIStyle APIStyle `json:"api_style,omitempty"`
	// APIFormat is an explicit OpenAI-family wire override. It is deliberately
	// separate from Models[].APIFormat, which is discovered catalog metadata.
	APIFormat string `json:"api_format,omitempty"`

	// DisplayName and UserDefined persist providers created through the generic
	// gateway flow without requiring a second configuration file.
	DisplayName string `json:"display_name,omitempty"`
	UserDefined bool   `json:"user_defined,omitempty"`

	// Models caches the most recently discovered catalog so a configured
	// provider remains selectable before its next online refresh.
	Models []ModelInfo `json:"models,omitempty"`

	// LastUsed records when this credential was last used.
	LastUsed time.Time `json:"last_used"`
}

// credentialFile is the persisted JSON structure.
type credentialFile struct {
	Entries map[string]CredentialEntry `json:"entries"`
}

// CredentialStore manages provider credentials with file-based persistence.
// Credentials are stored in ~/.luban-code/auth.json with 0600 permissions.
//
// Thread-safe for concurrent access.
type CredentialStore struct {
	path    string
	mu      sync.RWMutex
	entries map[string]CredentialEntry
}

// defaultCredentialDir returns the LUBAN Code credential directory.
func defaultCredentialDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialHomeFailed), err)
	}
	return filepath.Join(home, brand.ConfigDirName), nil
}

// NewCredentialStore creates a CredentialStore backed by ~/.luban-code/auth.json.
// It loads existing credentials from disk if the file exists.
func NewCredentialStore() (*CredentialStore, error) {
	dir, err := defaultCredentialDir()
	if err != nil {
		return nil, err
	}
	store, err := NewCredentialStoreAt(filepath.Join(dir, "auth.json"))
	if err != nil {
		return nil, err
	}
	return store, nil
}

// NewCredentialStoreAt creates a CredentialStore at a specific file path.
// Used for testing and custom configurations.
func NewCredentialStoreAt(path string) (*CredentialStore, error) {
	s := &CredentialStore{
		path:    path,
		entries: make(map[string]CredentialEntry),
	}

	// Load existing credentials if the file exists.
	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

// Get returns the credential entry for a provider.
func (s *CredentialStore) Get(provider string) (CredentialEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[provider]
	return entry, ok
}

// Set saves or updates a credential entry and persists to disk.
func (s *CredentialStore) Set(entry CredentialEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[entry.Provider] = entry
	return s.save()
}

// Delete removes a credential entry and persists the change.
func (s *CredentialStore) Delete(provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entries[provider]; !ok {
		return nil // no-op if not present
	}
	delete(s.entries, provider)
	return s.save()
}

// All returns all credential entries, sorted by provider name.
func (s *CredentialStore) All() []CredentialEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]CredentialEntry, 0, len(s.entries))
	for _, e := range s.entries {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Provider < result[j].Provider
	})
	return result
}

// HasCredentials checks if valid credentials exist for a provider.
// For API keys, checks non-empty. For OAuth, checks non-expired access token.
func (s *CredentialStore) HasCredentials(provider string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[provider]
	if !ok {
		return false
	}

	switch entry.AuthMethod {
	case "api_key":
		return entry.APIKey != ""
	case "oauth":
		if entry.AccessToken != "" && (entry.ExpiresAt.IsZero() || time.Now().Before(entry.ExpiresAt)) {
			return true
		}
		return entry.RefreshToken != ""
	default:
		return false
	}
}

// UpdateLastUsed updates the LastUsed timestamp for a provider.
func (s *CredentialStore) UpdateLastUsed(provider string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.entries[provider]; ok {
		entry.LastUsed = time.Now()
		s.entries[provider] = entry
		_ = s.save() // best effort
	}
}

// ── Persistence ──────────────────────────────────────────────────────────────

// load reads credentials from disk. Returns nil if the file doesn't exist.
func (s *CredentialStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no file yet, start empty
		}
		return fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialReadFailed, s.path), err)
	}

	var f credentialFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialDecodeFailed, s.path), err)
	}

	if f.Entries != nil {
		s.entries = f.Entries
	}

	return nil
}

// save writes credentials to disk atomically with 0600 permissions.
func (s *CredentialStore) save() error {
	// Ensure the directory exists.
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialDirectoryFailed, dir), err)
	}

	f := credentialFile{
		Entries: s.entries,
	}

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialEncodeFailed), err)
	}

	// Atomic write: temp file + rename.
	tmp, err := os.CreateTemp(dir, ".auth-*.tmp")
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialTempCreateFailed), err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialTempWriteFailed), err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialPermissionsFailed), err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialTempCloseFailed), err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCredentialReplaceFailed), err)
	}

	return nil
}

// ── Singleton ────────────────────────────────────────────────────────────────

var (
	defaultCredStoreOnce sync.Once
	defaultCredStoreInst *CredentialStore
	defaultCredStoreErr  error
)

// DefaultCredentialStore returns the singleton CredentialStore.
func DefaultCredentialStore() (*CredentialStore, error) {
	defaultCredStoreOnce.Do(func() {
		defaultCredStoreInst, defaultCredStoreErr = NewCredentialStore()
	})
	return defaultCredStoreInst, defaultCredStoreErr
}
