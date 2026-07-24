//go:build windows

package session

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryStoreWindowsRejectsMultiplyLinkedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	want := marshalMemoryFixture(t, "linked")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "memory-alias.json")
	if err := os.Link(path, alias); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
	store := NewMemoryStore(path)
	if err := store.Add("invalid", "test", "security"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("Add multiply-linked error = %v, want fs.ErrInvalid", err)
	}
	for _, candidate := range []string{path, alias} {
		got, err := os.ReadFile(candidate)
		if err != nil || string(got) != string(want) {
			t.Fatalf("linked memory %s changed: %q, %v", candidate, got, err)
		}
	}
}
