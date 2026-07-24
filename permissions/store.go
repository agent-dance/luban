package permissions

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/agent-dance/luban/brand"
)

const permissionsFile = "permissions.json"

// permissionsDir returns the directory where permissions.json is stored.
// Defaults to ~/.luban-code/.
func permissionsDir() (string, error) {
	if brand.HomeDir() == "" {
		return "", fmt.Errorf("cannot determine home directory")
	}
	return brand.UserConfigDir(), nil
}

// PermissionStore persists "always allow" decisions across sessions.
// The backing file is ~/.luban-code/permissions.json (0600).
type PermissionStore struct {
	mu      sync.RWMutex
	allowed map[string]bool // key → true if permanently allowed
	path    string          // absolute path to the JSON file
}

// NewPermissionStore creates a store backed by the default path (~/.luban-code/permissions.json).
// If the file doesn't exist yet it is created on the first Save.
func NewPermissionStore() (*PermissionStore, error) {
	dir, err := permissionsDir()
	if err != nil {
		return nil, err
	}
	store, err := NewPermissionStoreAt(filepath.Join(dir, permissionsFile))
	if err != nil || len(store.allowed) != 0 {
		return store, err
	}
	for _, legacyDir := range []string{brand.LegacyDeepSeekUserConfigDir(), brand.LegacyUserConfigDir()} {
		legacy, legacyErr := NewPermissionStoreAt(filepath.Join(legacyDir, permissionsFile))
		if legacyErr == nil && len(legacy.allowed) > 0 {
			store.allowed = legacy.allowed
			break
		}
	}
	return store, nil
}

// NewPermissionStoreAt creates a store backed by an explicit file path.
// This is primarily useful for testing.
func NewPermissionStoreAt(path string) (*PermissionStore, error) {
	ps := &PermissionStore{
		allowed: make(map[string]bool),
		path:    path,
	}
	if err := ps.load(); err != nil {
		return nil, err
	}
	return ps, nil
}

// load reads the JSON file into memory.  A missing file is not an error.
func (ps *PermissionStore) load() error {
	data, err := os.ReadFile(ps.path)
	if os.IsNotExist(err) {
		return nil // first run — start empty
	}
	if err != nil {
		return fmt.Errorf("permissions: read %s: %w", ps.path, err)
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if err := json.Unmarshal(data, &ps.allowed); err != nil {
		return fmt.Errorf("permissions: parse %s: %w", ps.path, err)
	}
	return nil
}

// Load reloads the store from disk, replacing any in-memory state.
// The reset and reload are performed under a single lock to prevent a
// concurrent reader from observing a transiently empty map.
func (ps *PermissionStore) Load() error {
	data, err := os.ReadFile(ps.path)
	if os.IsNotExist(err) {
		ps.mu.Lock()
		ps.allowed = make(map[string]bool)
		ps.mu.Unlock()
		return nil
	}
	if err != nil {
		return fmt.Errorf("permissions: read %s: %w", ps.path, err)
	}
	fresh := make(map[string]bool)
	if err := json.Unmarshal(data, &fresh); err != nil {
		return fmt.Errorf("permissions: parse %s: %w", ps.path, err)
	}
	ps.mu.Lock()
	ps.allowed = fresh
	ps.mu.Unlock()
	return nil
}

// Save atomically writes the in-memory state to disk (temp file + rename).
// The file is written with mode 0600 so only the owner can read it.
func (ps *PermissionStore) Save() error {
	ps.mu.RLock()
	data, err := json.MarshalIndent(ps.allowed, "", "  ")
	ps.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("permissions: marshal: %w", err)
	}

	dir := filepath.Dir(ps.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("permissions: mkdir %s: %w", dir, err)
	}

	// Write to a temp file in the same directory so rename is atomic.
	tmp, err := os.CreateTemp(dir, ".permissions-*.json.tmp")
	if err != nil {
		return fmt.Errorf("permissions: create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("permissions: write temp: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("permissions: chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("permissions: close temp: %w", err)
	}

	if err := os.Rename(tmpName, ps.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("permissions: rename to %s: %w", ps.path, err)
	}
	return nil
}

// IsAllowed reports whether a permanent allow decision exists for the given
// tool call.  It does NOT consult the in-session cache inside Checker.
func (ps *PermissionStore) IsAllowed(toolName string, input map[string]any) bool {
	key := storeKey(toolName, input)
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.allowed[key]
}

// AddAllow records a permanent allow for the given tool call and persists it.
func (ps *PermissionStore) AddAllow(toolName string, input map[string]any) error {
	key := storeKey(toolName, input)
	ps.mu.Lock()
	ps.allowed[key] = true
	ps.mu.Unlock()
	return ps.Save()
}

// storeKey builds the map key: "toolName:sha256(sortedInputJSON)".
// Sorting the JSON keys guarantees the same hash regardless of map iteration
// order.
func storeKey(toolName string, input map[string]any) string {
	hash := inputHash(input)
	return fmt.Sprintf("%s:%s", toolName, hash)
}

// inputHash returns a hex-encoded SHA-256 of the canonically sorted JSON
// encoding of input.  An empty map produces the hash of "{}".
func inputHash(input map[string]any) string {
	// Sort keys for determinism.
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ordered := make([]byte, 0, 128)
	ordered = append(ordered, '{')
	for i, k := range keys {
		kBytes, _ := json.Marshal(k)
		vBytes, _ := json.Marshal(input[k])
		ordered = append(ordered, kBytes...)
		ordered = append(ordered, ':')
		ordered = append(ordered, vBytes...)
		if i < len(keys)-1 {
			ordered = append(ordered, ',')
		}
	}
	ordered = append(ordered, '}')

	sum := sha256.Sum256(ordered)
	return fmt.Sprintf("%x", sum[:])
}
