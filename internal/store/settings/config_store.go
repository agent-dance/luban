package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// ConfigStore is a concurrency-safe string setting store backed by the user
// configuration file.
type ConfigStore struct {
	mu       sync.Mutex
	path     string
	settings map[string]string
}

// NewConfigStore loads the user's persisted configuration. A missing or
// unreadable file leaves the store empty; the first Set reports any write
// failure to its caller.
func NewConfigStore() *ConfigStore {
	store := &ConfigStore{
		path:     filepath.Join(brand.UserConfigDir(), "config.json"),
		settings: make(map[string]string),
	}
	_ = store.load()
	return store
}

func (s *ConfigStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.settings)
}

// Get returns a setting and whether it is present.
func (s *ConfigStore) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.settings[key]
	return value, ok
}

// Set persists a setting immediately.
func (s *ConfigStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[key] = value
	return s.save()
}

// save persists settings to disk. The caller must hold s.mu.
func (s *ConfigStore) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkConfigCreateDirectory, err)
	}
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkConfigMarshal, err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkConfigWrite, err)
	}
	return nil
}
