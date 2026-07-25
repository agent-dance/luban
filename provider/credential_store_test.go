package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func newTestCredentialStore(t *testing.T) (*CredentialStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	s, err := NewCredentialStoreAt(path)
	if err != nil {
		t.Fatalf("NewCredentialStoreAt: %v", err)
	}
	return s, path
}

func TestCredentialStore_SetAndGet(t *testing.T) {
	s, _ := newTestCredentialStore(t)

	entry := CredentialEntry{
		Provider:   "anthropic",
		AuthMethod: "api_key",
		APIKey:     "sk-ant-test-key",
		LastUsed:   time.Now(),
	}

	if err := s.Set(entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, ok := s.Get("anthropic")
	if !ok {
		t.Fatal("expected to find anthropic entry")
	}
	if got.APIKey != "sk-ant-test-key" {
		t.Errorf("expected API key %q, got %q", "sk-ant-test-key", got.APIKey)
	}
	if got.AuthMethod != "api_key" {
		t.Errorf("expected auth method %q, got %q", "api_key", got.AuthMethod)
	}
}

func TestCredentialStore_GetMissing(t *testing.T) {
	s, _ := newTestCredentialStore(t)
	_, ok := s.Get("nonexistent")
	if ok {
		t.Fatal("expected not found for nonexistent provider")
	}
}

func TestCredentialStore_Delete(t *testing.T) {
	s, _ := newTestCredentialStore(t)

	_ = s.Set(CredentialEntry{Provider: "openai", AuthMethod: "api_key", APIKey: "test"})
	if err := s.Delete("openai"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.Get("openai"); ok {
		t.Fatal("expected openai to be deleted")
	}
}

func TestCredentialStore_DeleteMissing(t *testing.T) {
	s, _ := newTestCredentialStore(t)
	// Should not error on deleting a non-existent entry.
	if err := s.Delete("nonexistent"); err != nil {
		t.Fatalf("Delete non-existent: %v", err)
	}
}

func TestCredentialStore_All(t *testing.T) {
	s, _ := newTestCredentialStore(t)

	_ = s.Set(CredentialEntry{Provider: "openai", AuthMethod: "api_key", APIKey: "k1"})
	_ = s.Set(CredentialEntry{Provider: "anthropic", AuthMethod: "api_key", APIKey: "k2"})
	_ = s.Set(CredentialEntry{Provider: "gemini", AuthMethod: "api_key", APIKey: "k3"})

	all := s.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(all))
	}
	// Should be sorted by provider name.
	if all[0].Provider != "anthropic" || all[1].Provider != "gemini" || all[2].Provider != "openai" {
		t.Errorf("expected alphabetical order, got %v %v %v", all[0].Provider, all[1].Provider, all[2].Provider)
	}
}

func TestCredentialStore_HasCredentials_APIKey(t *testing.T) {
	s, _ := newTestCredentialStore(t)

	_ = s.Set(CredentialEntry{Provider: "openai", AuthMethod: "api_key", APIKey: "test-key"})
	if !s.HasCredentials("openai") {
		t.Error("expected HasCredentials=true for openai with API key")
	}

	_ = s.Set(CredentialEntry{Provider: "empty", AuthMethod: "api_key", APIKey: ""})
	if s.HasCredentials("empty") {
		t.Error("expected HasCredentials=false for empty API key")
	}
}

func TestCredentialStore_HasCredentials_OAuth(t *testing.T) {
	s, _ := newTestCredentialStore(t)

	// Valid OAuth token.
	_ = s.Set(CredentialEntry{
		Provider:    "oauth-valid",
		AuthMethod:  "oauth",
		AccessToken: "at-123",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	})
	if !s.HasCredentials("oauth-valid") {
		t.Error("expected HasCredentials=true for valid OAuth token")
	}

	// Expired OAuth token.
	_ = s.Set(CredentialEntry{
		Provider:    "oauth-expired",
		AuthMethod:  "oauth",
		AccessToken: "at-expired",
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
	})
	if s.HasCredentials("oauth-expired") {
		t.Error("expected HasCredentials=false for expired OAuth token")
	}

	// OAuth with no expiry (treated as valid).
	_ = s.Set(CredentialEntry{
		Provider:    "oauth-no-expiry",
		AuthMethod:  "oauth",
		AccessToken: "at-no-exp",
	})
	if !s.HasCredentials("oauth-no-expiry") {
		t.Error("expected HasCredentials=true for OAuth token with no expiry")
	}
}

func TestCredentialStore_HasCredentials_Missing(t *testing.T) {
	s, _ := newTestCredentialStore(t)
	if s.HasCredentials("nonexistent") {
		t.Error("expected HasCredentials=false for nonexistent provider")
	}
}

func TestCredentialStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	// Create store and add entries.
	s1, err := NewCredentialStoreAt(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	_ = s1.Set(CredentialEntry{Provider: "anthropic", AuthMethod: "api_key", APIKey: "persisted-key"})

	// Create a new store from the same file — should load persisted data.
	s2, err := NewCredentialStoreAt(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	got, ok := s2.Get("anthropic")
	if !ok {
		t.Fatal("expected to find anthropic in reloaded store")
	}
	if got.APIKey != "persisted-key" {
		t.Errorf("expected persisted key, got %q", got.APIKey)
	}
}

func TestNewCredentialStoreUsesLUBANCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path := filepath.Join(home, ".luban-code", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(credentialFile{Entries: map[string]CredentialEntry{
		"openai": {Provider: "openai", AuthMethod: "api_key", APIKey: "luban-key"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := NewCredentialStore()
	if err != nil {
		t.Fatalf("NewCredentialStore: %v", err)
	}
	entry, ok := store.Get("openai")
	if !ok || entry.APIKey != "luban-key" {
		t.Fatalf("entry = %#v, ok=%v; want LUBAN credentials", entry, ok)
	}
}

func TestCredentialStore_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX 0600 permissions through Mode().Perm()")
	}

	s, path := newTestCredentialStore(t)

	_ = s.Set(CredentialEntry{Provider: "test", AuthMethod: "api_key", APIKey: "secret"})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected 0600 permissions, got %o", perm)
	}
}

func TestCredentialStore_FileFormat(t *testing.T) {
	s, path := newTestCredentialStore(t)

	_ = s.Set(CredentialEntry{Provider: "test", AuthMethod: "api_key", APIKey: "k1"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var f credentialFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(f.Entries))
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw file: %v", err)
	}
	if _, exists := raw["version"]; exists {
		t.Fatal("credential file retained unused version field")
	}
}

func TestCredentialStore_UpdateLastUsed(t *testing.T) {
	s, _ := newTestCredentialStore(t)

	before := time.Now().Add(-1 * time.Hour)
	_ = s.Set(CredentialEntry{Provider: "test", AuthMethod: "api_key", APIKey: "k", LastUsed: before})

	s.UpdateLastUsed("test")

	got, _ := s.Get("test")
	if !got.LastUsed.After(before) {
		t.Error("expected LastUsed to be updated")
	}
}

func TestCredentialStore_ConcurrentAccess(t *testing.T) {
	s, _ := newTestCredentialStore(t)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			provider := "provider-" + string(rune('a'+i%10))
			_ = s.Set(CredentialEntry{
				Provider:   provider,
				AuthMethod: "api_key",
				APIKey:     "key-" + provider,
			})
			s.Get(provider)
			s.HasCredentials(provider)
			s.All()
		}(i)
	}
	wg.Wait()

	// Should not panic or corrupt data.
	all := s.All()
	if len(all) == 0 {
		t.Error("expected some entries after concurrent writes")
	}
}

func TestCredentialStore_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	// No file on disk — store should start empty.
	s, err := NewCredentialStoreAt(path)
	if err != nil {
		t.Fatalf("create store with no file: %v", err)
	}
	if len(s.All()) != 0 {
		t.Errorf("expected empty store, got %d entries", len(s.All()))
	}
}

func TestCredentialStore_CorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	// Write invalid JSON.
	_ = os.WriteFile(path, []byte("{invalid json"), 0o600)

	_, err := NewCredentialStoreAt(path)
	if err == nil {
		t.Fatal("expected error for corrupted file")
	}
}

func TestCredentialStore_Overwrite(t *testing.T) {
	s, _ := newTestCredentialStore(t)

	// Set initial entry.
	_ = s.Set(CredentialEntry{Provider: "test", AuthMethod: "api_key", APIKey: "key-1"})

	// Overwrite with new key.
	_ = s.Set(CredentialEntry{Provider: "test", AuthMethod: "api_key", APIKey: "key-2"})

	got, _ := s.Get("test")
	if got.APIKey != "key-2" {
		t.Errorf("expected overwritten key, got %q", got.APIKey)
	}
}
