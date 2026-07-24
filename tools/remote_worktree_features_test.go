package tools

import (
	"os"
	"testing"
)

// TestRemoteTriggerLoadCustomCABundleEmpty verifies RT-06: when no env
// vars are set, no custom pool is constructed.
func TestRemoteTriggerLoadCustomCABundleEmpty(t *testing.T) {
	t.Setenv("SSL_CERT_FILE", "")
	t.Setenv("NODE_EXTRA_CA_CERTS", "")
	if pool := loadCustomCABundle(); pool != nil {
		t.Fatalf("expected nil pool when no env vars set")
	}
}

// TestWorktreeIncludeFileEmpty covers WT-03: missing .worktreeinclude
// returns nil (no entries).
func TestWorktreeIncludeFileEmpty(t *testing.T) {
	dir := t.TempDir()
	if got := WorktreeIncludeFile(dir); got != nil {
		t.Fatalf("expected nil for missing file, got %v", got)
	}
}

// TestWorktreeIncludeFileParse covers comments and blank-line handling.
func TestWorktreeIncludeFileParse(t *testing.T) {
	dir := t.TempDir()
	contents := []byte("# comment\nnode_modules\n\n.env\n")
	if err := os.WriteFile(dir+"/.worktreeinclude", contents, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := WorktreeIncludeFile(dir)
	if len(got) != 2 || got[0] != "node_modules" || got[1] != ".env" {
		t.Fatalf("expected [node_modules .env], got %v", got)
	}
}

// TestParsePRRefShort covers WT-01 short form `pr:42`.
func TestParsePRRefShort(t *testing.T) {
	ref, err := ParsePRRef("pr:42")
	if err != nil || ref == nil {
		t.Fatalf("unexpected error: %v %v", ref, err)
	}
	if ref.Number != 42 {
		t.Fatalf("expected 42, got %d", ref.Number)
	}
}

// TestParsePRRefFull covers WT-01 full form.
func TestParsePRRefFull(t *testing.T) {
	ref, err := ParsePRRef("pr:owner/repo#7")
	if err != nil || ref == nil {
		t.Fatalf("unexpected: %v %v", ref, err)
	}
	if ref.Owner != "owner" || ref.Repo != "repo" || ref.Number != 7 {
		t.Fatalf("got %+v", ref)
	}
}

// TestParsePRRefNotPR returns nil ref + nil error for non-PR strings.
func TestParsePRRefNotPR(t *testing.T) {
	ref, err := ParsePRRef("main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != nil {
		t.Fatalf("expected nil ref, got %+v", ref)
	}
}

// TestWorktreeCacheInvalidator covers WT-07 registration / invocation.
func TestWorktreeCacheInvalidator(t *testing.T) {
	called := 0
	RegisterWorktreeCacheInvalidator(func() { called++ })
	InvalidateWorktreeCaches()
	if called == 0 {
		t.Fatalf("expected invalidator to run")
	}
}

// TestIsEphemeralWorktreeName covers WT-05 pattern matcher.
func TestIsEphemeralWorktreeName(t *testing.T) {
	cases := map[string]bool{
		"/repo/.luban-code/worktrees/luban-agent-task":       true,
		"/repo/.deepseek-code/worktrees/deepseek-agent-task": true,
		"/repo/.deepseek-code/worktrees/deepseek-wt-legacy":  true,
		"/repo/.claude/worktrees/agent-abc123def456":         true,
		"/repo/.claude/worktrees/wt-1700000000000":           true,
		"/repo/branches/feature":                             false,
		"agent-abc123def456":                                 true,
	}
	for path, want := range cases {
		if got := isEphemeralWorktreeName(path); got != want {
			t.Errorf("isEphemeralWorktreeName(%q) = %v, want %v", path, got, want)
		}
	}
}
