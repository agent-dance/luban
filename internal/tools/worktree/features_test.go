package worktree

import (
	"os"
	"testing"
)

func TestWorktreeIncludeFileEmpty(t *testing.T) {
	dir := t.TempDir()
	if got := worktreeIncludeFile(dir); got != nil {
		t.Fatalf("expected nil for missing file, got %v", got)
	}
}

func TestWorktreeIncludeFileParse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.worktreeinclude", []byte("# comment\nnode_modules\n\n.env\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := worktreeIncludeFile(dir)
	if len(got) != 2 || got[0] != "node_modules" || got[1] != ".env" {
		t.Fatalf("expected [node_modules .env], got %v", got)
	}
}

func TestParsePRRef(t *testing.T) {
	short, err := parsePRRef("pr:42")
	if err != nil || short == nil || short.Number != 42 {
		t.Fatalf("short ref: ref=%+v err=%v", short, err)
	}
	full, err := parsePRRef("pr:owner/repo#7")
	if err != nil || full == nil || full.Owner != "owner" || full.Repo != "repo" || full.Number != 7 {
		t.Fatalf("full ref: ref=%+v err=%v", full, err)
	}
	none, err := parsePRRef("main")
	if err != nil || none != nil {
		t.Fatalf("non-PR ref: ref=%+v err=%v", none, err)
	}
}
