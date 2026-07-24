package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitNoPromptEnvVars verifies WT-06: every git invocation should
// receive credential / SSH prompt suppression env vars so a private
// remote can't hang the agent.
func TestGitNoPromptEnvVars(t *testing.T) {
	env := gitNoPromptEnv()
	expected := []string{
		"GIT_TERMINAL_PROMPT=0",
		"SSH_ASKPASS_REQUIRE=never",
	}
	for _, want := range expected {
		found := false
		for _, line := range env {
			if line == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("gitNoPromptEnv missing %q", want)
		}
	}
	// GIT_SSH_COMMAND must contain BatchMode=yes.
	hasBatch := false
	for _, line := range env {
		if strings.HasPrefix(line, "GIT_SSH_COMMAND=") && strings.Contains(line, "BatchMode=yes") {
			hasBatch = true
			break
		}
	}
	if !hasBatch {
		t.Errorf("GIT_SSH_COMMAND must contain BatchMode=yes")
	}
}

// TestApplyWorktreeSettingsAndHuskyCopiesSettings verifies WT-04: when
// .claude/settings.local.json exists in the source repo we copy it
// into the new worktree.
func TestApplyWorktreeSettingsAndHuskyCopiesSettings(t *testing.T) {
	repo := t.TempDir()
	wt := t.TempDir()
	srcDir := filepath.Join(repo, ".claude")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	srcFile := filepath.Join(srcDir, "settings.local.json")
	want := []byte(`{"foo":"bar"}`)
	if err := os.WriteFile(srcFile, want, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	applyWorktreeSettingsAndHusky(repo, wt)

	got, err := os.ReadFile(filepath.Join(wt, ".claude", "settings.local.json"))
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

// TestStdioRestartTrackerLifecycle exercises MCP-02 tracker bookkeeping
// without standing up a real subprocess.
func TestStdioRestartTrackerLifecycle(t *testing.T) {
	mgr := NewMCPManager()
	tr := mgr.acquireRestartTracker("srv")
	if tr.attempts != 0 || tr.failed {
		t.Fatalf("fresh tracker should be zero/false")
	}
	tr.attempts = 2
	mgr.releaseRestartTracker("srv", tr)

	again := mgr.acquireRestartTracker("srv")
	if again.attempts != 2 {
		t.Fatalf("tracker state should persist between acquires; got %d", again.attempts)
	}

	mgr.markServerFailed("srv")
	if !mgr.IsServerFailed("srv") {
		t.Fatalf("expected IsServerFailed=true after markServerFailed")
	}
	mgr.ResetRestartState("srv")
	if mgr.IsServerFailed("srv") {
		t.Fatalf("ResetRestartState should clear the failed flag")
	}
}
