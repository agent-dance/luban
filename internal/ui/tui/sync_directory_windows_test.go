package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsCheckpointPublicationDoesNotRequireDirectoryHandleSync(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tui-view", "checkpoints")
	if err := ensurePrivateDirectory(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "checkpoint.json")
	if err := writePrivateAtomic(path, []byte("checkpoint\n")); err != nil {
		t.Fatal(err)
	}
	if err := syncDirectory(dir); err != nil {
		t.Fatalf("syncDirectory(%q) error = %v", dir, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "checkpoint\n" {
		t.Fatalf("published checkpoint = %q, %v", data, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 == 0 {
		t.Skipf("runtime reports POSIX-style private mode %04o", info.Mode().Perm())
	}
	if !privateFilePermissionsValid(info) {
		t.Fatalf("Windows synthetic mode %04o was treated as an ACL", info.Mode().Perm())
	}
}
