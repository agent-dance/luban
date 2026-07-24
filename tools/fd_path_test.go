package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFdRealPathResolvesOpenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("sample"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	actualPath, err := fdRealPath(f)
	if err != nil {
		t.Fatalf("fdRealPath: %v", err)
	}
	if actualPath != filepath.Clean(path) {
		t.Fatalf("fdRealPath() = %q, want %q", actualPath, filepath.Clean(path))
	}
	if err := validatePathInDirs(actualPath, []string{dir}); err != nil {
		t.Fatalf("validatePathInDirs: %v", err)
	}
}
