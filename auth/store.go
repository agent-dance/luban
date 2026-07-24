package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

const credentialsLockFile = ".credentials.lock"
const lockRetries = 20
const lockRetryDelay = 50 * time.Millisecond

// acquireCredLock creates a lock file using O_CREATE|O_EXCL (atomic on POSIX)
// and retries up to lockRetries times with lockRetryDelay between attempts.
// Returns the open lock file on success; caller must call releaseCredLock.
func (s *Store) acquireCredLock() (*os.File, error) {
	lockPath := filepath.Join(s.dir, credentialsLockFile)
	for i := 0; i < lockRetries; i++ {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return f, nil
		}
		if !os.IsExist(err) {
			return nil, i18n.WrapError(i18n.KeyAuthStoreAcquireLock, err)
		}
		time.Sleep(lockRetryDelay)
	}
	return nil, i18n.NewError(i18n.KeyAuthStoreLockHeld, lockRetries)
}

// releaseCredLock closes and removes the lock file.
func (s *Store) releaseCredLock(f *os.File) {
	_ = f.Close()
	_ = os.Remove(f.Name())
}

const credentialsFile = ".credentials.json"
const expiryBuffer = 5 * time.Minute

// Credentials stores the persisted OAuth token data.
type Credentials struct {
	Provider         string    `json:"provider,omitempty"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	ExpiresAt        time.Time `json:"expires_at"`
	TokenType        string    `json:"token_type"`
	Scopes           []string  `json:"scopes,omitempty"`
	OrganizationUUID string    `json:"organization_uuid,omitempty"`
}

var ensureValidMu sync.Mutex

// Store manages loading, saving, and refreshing OAuth credentials.
type Store struct {
	dir        string   // directory containing the credentials file
	legacyDirs []string // fallback directories, highest priority first
}

// NewStore creates a Store that persists credentials in ~/.luban-code/.credentials.json.
func NewStore() (*Store, error) {
	if brand.HomeDir() == "" {
		return nil, i18n.NewError(i18n.KeyAuthStoreHomeUnavailable)
	}
	return &Store{
		dir: brand.UserConfigDir(),
		legacyDirs: []string{
			brand.LegacyDeepSeekUserConfigDir(),
			brand.LegacyUserConfigDir(),
		},
	}, nil
}

// newStoreAt creates a Store with an explicit directory (used in tests).
func newStoreAt(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) credPath() string {
	return filepath.Join(s.dir, credentialsFile)
}

func (s *Store) legacyCredPaths() []string {
	paths := make([]string, 0, len(s.legacyDirs))
	for _, dir := range s.legacyDirs {
		if dir == "" || dir == s.dir {
			continue
		}
		paths = append(paths, filepath.Join(dir, credentialsFile))
	}
	return paths
}

// LoadCredentials reads credentials from disk with file locking.
// Returns (nil, nil) if the file does not exist.
func (s *Store) LoadCredentials() (*Credentials, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthStoreCreateDirectory, err)
	}
	lf, err := s.acquireCredLock()
	if err != nil {
		return nil, err
	}
	defer s.releaseCredLock(lf)

	data, err := os.ReadFile(s.credPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			for _, legacyPath := range s.legacyCredPaths() {
				legacyData, legacyErr := os.ReadFile(legacyPath)
				if legacyErr != nil {
					if errors.Is(legacyErr, os.ErrNotExist) {
						continue
					}
					return nil, i18n.WrapError(i18n.KeyAuthStoreReadLegacy, legacyErr)
				}
				data = legacyData
				break
			}
			if data == nil {
				return nil, nil
			}
		} else {
			return nil, i18n.WrapError(i18n.KeyAuthStoreRead, err)
		}
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthStoreDecode, err)
	}
	return &creds, nil
}

// SaveCredentials writes credentials to disk atomically with 0600 permissions,
// protected by a lock file to prevent concurrent writes.
func (s *Store) SaveCredentials(creds *Credentials) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return i18n.WrapError(i18n.KeyAuthStoreCreateDirectory, err)
	}

	lf, err := s.acquireCredLock()
	if err != nil {
		return err
	}
	defer s.releaseCredLock(lf)

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return i18n.WrapError(i18n.KeyAuthStoreEncode, err)
	}

	// Atomic write: write to a temp file in the same directory, then rename.
	tmp, err := os.CreateTemp(s.dir, ".credentials-*.tmp")
	if err != nil {
		return i18n.WrapError(i18n.KeyAuthStoreCreateTemporary, err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return i18n.WrapError(i18n.KeyAuthStoreWriteTemporary, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return i18n.WrapError(i18n.KeyAuthStoreSetPermissions, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return i18n.WrapError(i18n.KeyAuthStoreCloseTemporary, err)
	}
	if err := os.Rename(tmpName, s.credPath()); err != nil {
		_ = os.Remove(tmpName)
		return i18n.WrapError(i18n.KeyAuthStoreReplaceCredentials, err)
	}
	return nil
}

// IsExpired returns true if the credentials are expired or within the 5-minute
// refresh buffer. A nil argument or zero ExpiresAt is treated as not expired.
func IsExpired(creds *Credentials) bool {
	if creds == nil {
		return true
	}
	if creds.ExpiresAt.IsZero() {
		return false // no expiry info — assume valid
	}
	return time.Now().Add(expiryBuffer).After(creds.ExpiresAt)
}

// RefreshToken exchanges a refresh token for a new access + refresh token pair.
func RefreshToken(ctx context.Context, cfg OAuthConfig, refreshToken string) (*TokenResponse, error) {
	vals := url.Values{}
	vals.Set("grant_type", "refresh_token")
	vals.Set("refresh_token", refreshToken)
	vals.Set("client_id", cfg.ClientID)
	if len(cfg.Scopes) > 0 {
		vals.Set("scope", strings.Join(cfg.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthBuildRefreshRequest, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := authHTTPClient.Do(req)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthRefreshRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, i18n.NewError(i18n.KeyAuthOAuthRefreshEndpointRejected, resp.StatusCode, string(errBody))
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthDecodeRefreshResponse, err)
	}
	if tr.ExpiresIn > 0 {
		tr.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return &tr, nil
}

// credentialsFromTokenResponse converts a TokenResponse to a Credentials struct.
func credentialsFromTokenResponse(tr *TokenResponse) *Credentials {
	creds := &Credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		ExpiresAt:    tr.ExpiresAt,
		Scopes:       strings.Fields(tr.Scope),
	}
	if creds.TokenType == "" {
		creds.TokenType = "Bearer"
	}
	return creds
}

// EnsureValid checks if credentials are valid, refreshing them if needed.
// Returns updated credentials and persists them to disk if refreshed.
func (s *Store) EnsureValid(ctx context.Context, cfg OAuthConfig) (*Credentials, error) {
	// Load-refresh-save is one logical operation. Serializing it prevents
	// concurrent tool calls from exchanging the same refresh token twice; each
	// waiter re-reads the credentials after the active refresh completes.
	ensureValidMu.Lock()
	defer ensureValidMu.Unlock()

	creds, err := s.LoadCredentials()
	if err != nil {
		return nil, err
	}
	if creds == nil {
		return nil, i18n.NewError(i18n.KeyAuthOAuthNoCredentials)
	}
	if !IsExpired(creds) {
		return creds, nil
	}
	if creds.RefreshToken == "" {
		return nil, i18n.NewError(i18n.KeyAuthOAuthCredentialsExpired)
	}

	tr, err := RefreshToken(ctx, cfg, creds.RefreshToken)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyAuthOAuthRefreshFailed, err)
	}
	// Preserve existing refresh token if the server didn't return a new one.
	if tr.RefreshToken == "" {
		tr.RefreshToken = creds.RefreshToken
	}

	newCreds := credentialsFromTokenResponse(tr)
	newCreds.Provider = creds.Provider
	newCreds.OrganizationUUID = creds.OrganizationUUID
	if len(newCreds.Scopes) == 0 {
		newCreds.Scopes = append([]string(nil), creds.Scopes...)
	}
	if saveErr := s.SaveCredentials(newCreds); saveErr != nil {
		// Non-fatal: return the new credentials even if persistence fails.
		return newCreds, i18n.WrapError(i18n.KeyAuthOAuthSaveRefreshedCredentials, saveErr)
	}
	return newCreds, nil
}
