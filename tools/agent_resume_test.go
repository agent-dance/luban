package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTouchAgentWorktreePath_BumpsMtime(t *testing.T) {
	dir := t.TempDir()
	wt := filepath.Join(dir, "worktree")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Set an old mtime first.
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(wt, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := touchAgentWorktreePath(wt); err != nil {
		t.Fatalf("touch: %v", err)
	}
	info, err := os.Stat(wt)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.ModTime().Before(time.Now().Add(-1 * time.Minute)) {
		t.Fatalf("expected mtime bumped to ~now, got %v", info.ModTime())
	}
}

func TestTouchAgentWorktreePath_MissingIsOK(t *testing.T) {
	if err := touchAgentWorktreePath(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Fatalf("missing path should return nil error, got %v", err)
	}
	if err := touchAgentWorktreePath(""); err != nil {
		t.Fatalf("empty path should return nil error, got %v", err)
	}
}
