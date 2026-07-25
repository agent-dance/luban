package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

// TestApplyWorktreeSettingsAndHuskyCopiesSettings verifies WT-04: when
// .luban-code/settings.local.json exists in the source repo we copy it
// into the new worktree.
func TestApplyWorktreeSettingsAndHuskyCopiesSettings(t *testing.T) {
	repo := t.TempDir()
	wt := t.TempDir()
	srcDir := filepath.Join(repo, ".luban-code")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	srcFile := filepath.Join(srcDir, "settings.local.json")
	want := []byte(`{"foo":"bar"}`)
	if err := os.WriteFile(srcFile, want, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	applyWorktreeSettingsAndHusky(repo, wt)

	got, err := os.ReadFile(filepath.Join(wt, ".luban-code", "settings.local.json"))
	if err != nil {
		t.Fatalf("settings.local.json should be copied: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("contents mismatch: got %q, want %q", got, want)
	}
}

// TestWorktreeSparseCheckoutPatternsParse covers WT-02 env-var parsing.
func TestWorktreeSparseCheckoutPatternsParse(t *testing.T) {
	t.Setenv("WORKTREE_SPARSE_CHECKOUT", "src/, , docs/")
	got := worktreeSparseCheckoutPatterns()
	if len(got) != 2 || got[0] != "src/" || got[1] != "docs/" {
		t.Fatalf("expected [src/ docs/], got %v", got)
	}
}

// TestWorktreeSparseCheckoutPatternsEmpty returns nil when env var is
// missing or blank.
func TestWorktreeSparseCheckoutPatternsEmpty(t *testing.T) {
	t.Setenv("WORKTREE_SPARSE_CHECKOUT", "")
	if got := worktreeSparseCheckoutPatterns(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// TestApplyWorktreeIncludesCopiesFile verifies the WT-03 file branch.
func TestApplyWorktreeIncludesCopiesFile(t *testing.T) {
	repo := t.TempDir()
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("X=1"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	applyWorktreeIncludes(repo, wt, []string{".env"})
	got, err := os.ReadFile(filepath.Join(wt, ".env"))
	if err != nil {
		t.Fatalf("expected .env to be copied: %v", err)
	}
	if string(got) != "X=1" {
		t.Fatalf("got %q", got)
	}
}
