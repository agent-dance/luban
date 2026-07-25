package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/brand"
)

func newTestConfigStore(t *testing.T) *ConfigStore {
	t.Helper()
	return &ConfigStore{
		path:     filepath.Join(t.TempDir(), "config.json"),
		settings: make(map[string]string),
	}
}

func TestConfigStoreGetSet(t *testing.T) {
	store := newTestConfigStore(t)
	if _, ok := store.Get("model"); ok {
		t.Fatal("missing setting reported as present")
	}
	if err := store.Set("model", "deepseek-v4-pro"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, ok := store.Get("model"); !ok || got != "deepseek-v4-pro" {
		t.Fatalf("Get(model) = %q, %v", got, ok)
	}
}

func TestConfigStorePersistsToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	first := &ConfigStore{path: path, settings: make(map[string]string)}
	if err := first.Set("verbose", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	second := &ConfigStore{path: path, settings: make(map[string]string)}
	if err := second.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, ok := second.Get("verbose"); !ok || got != "true" {
		t.Fatalf("Get(verbose) = %q, %v", got, ok)
	}
}

func TestConfigStoreMissingFileIsEmpty(t *testing.T) {
	store := &ConfigStore{
		path:     filepath.Join(t.TempDir(), "missing", "config.json"),
		settings: make(map[string]string),
	}
	if err := store.load(); err != nil {
		t.Fatalf("load missing file: %v", err)
	}
}

func TestNewConfigStoreUsesUserConfigDirectory(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	store := NewConfigStore()
	if err := store.Set("max_turns", "10"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := filepath.Join(tempHome, brand.ConfigDirName, "config.json")
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config permissions = %o, want 600", got)
	}
}
