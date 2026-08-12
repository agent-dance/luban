package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDirectoryAcceptsExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := syncDirectory(dir); err != nil {
		t.Fatalf("syncDirectory(%q) error = %v", dir, err)
	}
}

func TestSyncDirectoryRejectsMissingPathAndRegularFile(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	if err := syncDirectory(missing); !os.IsNotExist(err) {
		t.Fatalf("syncDirectory(%q) error = %v, want os.ErrNotExist", missing, err)
	}

	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syncDirectory(file); err == nil {
		t.Fatalf("syncDirectory(%q) accepted a regular file", file)
	}
}

func TestPrivateFilePermissionsAcceptPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !privateFilePermissionsValid(info) {
		t.Fatalf("privateFilePermissionsValid(%04o) = false", info.Mode().Perm())
	}
}
