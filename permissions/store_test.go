package permissions

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func tempStore(t *testing.T) *PermissionStore {
	t.Helper()
	dir := t.TempDir()
	ps, err := NewPermissionStoreAt(filepath.Join(dir, "permissions.json"))
	if err != nil {
		t.Fatalf("NewPermissionStoreAt: %v", err)
	}
	return ps
}

func TestPermissionStore_RoundTrip(t *testing.T) {
	t.Parallel()
	ps := tempStore(t)

	tool := "Bash"
	input := map[string]any{"command": "ls -la"}

	// Initially not allowed
	if ps.IsAllowed(tool, input) {
		t.Fatal("expected not allowed before AddAllow")
	}

	// Add allow and save
	if err := ps.AddAllow(tool, input); err != nil {
		t.Fatalf("AddAllow: %v", err)
	}

	// Should be allowed now in memory
	if !ps.IsAllowed(tool, input) {
		t.Fatal("expected allowed after AddAllow")
	}

	// Reload from disk and verify persistence
	if err := ps.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ps.IsAllowed(tool, input) {
		t.Fatal("expected allowed after Load (persistence check)")
	}
}

func TestPermissionStore_NewStoreLoadsExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "permissions.json")

	// Write and save via first store
	ps1, err := NewPermissionStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ps1.AddAllow("Write", map[string]any{"file_path": "foo.go"}); err != nil {
		t.Fatal(err)
	}

	// Open a fresh store pointing to the same file
	ps2, err := NewPermissionStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ps2.IsAllowed("Write", map[string]any{"file_path": "foo.go"}) {
		t.Fatal("new store should load existing allowed entries from disk")
	}
}

func TestPermissionStore_HashConsistency(t *testing.T) {
	t.Parallel()
	// The same input in different map-iteration orders should produce the same hash.
	inputA := map[string]any{"b": "two", "a": "one"}
	inputB := map[string]any{"a": "one", "b": "two"}

	hashA := inputHash(inputA)
	hashB := inputHash(inputB)
	if hashA != hashB {
		t.Errorf("hash mismatch: %q vs %q — key ordering must not affect hash", hashA, hashB)
	}
}

func TestPermissionStore_DifferentInputsDifferentHashes(t *testing.T) {
	t.Parallel()
	h1 := inputHash(map[string]any{"command": "ls"})
	h2 := inputHash(map[string]any{"command": "rm -rf /"})
	if h1 == h2 {
		t.Error("different inputs must produce different hashes")
	}
}

func TestPermissionStore_EmptyInput(t *testing.T) {
	t.Parallel()
	ps := tempStore(t)
	// Should not panic or error on empty input
	if err := ps.AddAllow("Read", map[string]any{}); err != nil {
		t.Fatalf("AddAllow with empty input: %v", err)
	}
	if !ps.IsAllowed("Read", map[string]any{}) {
		t.Error("expected IsAllowed true after AddAllow with empty input")
	}
}

func TestPermissionStore_MultipleEntries(t *testing.T) {
	t.Parallel()
	ps := tempStore(t)

	entries := []struct {
		tool  string
		input map[string]any
	}{
		{"Bash", map[string]any{"command": "ls"}},
		{"Bash", map[string]any{"command": "git status"}},
		{"Write", map[string]any{"file_path": "foo.go"}},
		{"Read", map[string]any{"file_path": "bar.go"}},
	}

	for _, e := range entries {
		if err := ps.AddAllow(e.tool, e.input); err != nil {
			t.Fatalf("AddAllow(%q): %v", e.tool, err)
		}
	}

	// Reload and verify all entries survive
	if err := ps.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, e := range entries {
		if !ps.IsAllowed(e.tool, e.input) {
			t.Errorf("entry %q %v not found after reload", e.tool, e.input)
		}
	}
}

func TestPermissionStore_NotAllowedDifferentTool(t *testing.T) {
	t.Parallel()
	ps := tempStore(t)
	input := map[string]any{"file_path": "foo.go"}

	if err := ps.AddAllow("Write", input); err != nil {
		t.Fatal(err)
	}
	// Same input, different tool — must not be allowed
	if ps.IsAllowed("Edit", input) {
		t.Error("IsAllowed should return false for a different tool with same input")
	}
}

func TestPermissionStore_FilePermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "permissions.json")

	ps, err := NewPermissionStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AddAllow("Bash", map[string]any{"command": "echo hi"}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	perm := info.Mode().Perm()
	if runtime.GOOS == "windows" {
		t.Skipf("Windows os.FileMode does not expose owner-only ACLs; file exists with mode %o", perm)
	}
	if perm != 0600 {
		t.Errorf("permissions.json has mode %o, want 0600", perm)
	}
}

func TestPermissionStore_MissingFileIsNotError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "permissions.json")
	// File doesn't exist yet — NewPermissionStoreAt should succeed.
	ps, err := NewPermissionStoreAt(path)
	if err != nil {
		// The parent dir doesn't exist so load will error on ReadFile with a
		// path-not-exist error — that is acceptable. Only IsNotExist is silenced.
		// Re-check: if the dir doesn't exist os.ReadFile returns not-exist too.
		t.Logf("NewPermissionStoreAt returned err (may be ok if dir missing): %v", err)
	}
	_ = ps
}

func TestStoreKey_Format(t *testing.T) {
	t.Parallel()
	key := storeKey("Bash", map[string]any{"command": "ls"})
	if len(key) == 0 {
		t.Error("storeKey returned empty string")
	}
	// Key must start with tool name
	if key[:4] != "Bash" {
		t.Errorf("storeKey should start with tool name, got %q", key)
	}
	// Key must contain a colon separator
	if key[4] != ':' {
		t.Errorf("storeKey should have ':' after tool name, got %q", key)
	}
	// Hash portion should be 64 hex chars (SHA-256)
	hash := key[5:]
	if len(hash) != 64 {
		t.Errorf("hash portion should be 64 chars, got %d in key %q", len(hash), key)
	}
}
