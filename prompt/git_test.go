package prompt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLoadGitContextCleanRepo(t *testing.T) {
	repo := initPromptTestRepo(t)

	got := LoadGitContext(repo)
	for _, want := range []string{
		"This is the git status at the start of the conversation.",
		"snapshot in time, and will not update during the conversation",
		"Current branch: main",
		"Main branch (you will usually use this for PRs): main",
		"Git user: Prompt Tester",
		"Status:\n(clean)",
		"Recent commits:",
		"initial commit",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected git context to contain %q in:\n%s", want, got)
		}
	}
}

func TestLoadGitContextDirtyRepo(t *testing.T) {
	repo := initPromptTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := LoadGitContext(repo)
	for _, want := range []string{
		"Status:",
		" M tracked.txt",
		"?? untracked.txt",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected dirty git context to contain %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Status:\n(clean)") {
		t.Fatalf("dirty repo should not render clean placeholder:\n%s", got)
	}
}

func TestLoadGitContextNonGitDirectory(t *testing.T) {
	got := LoadGitContext(t.TempDir())
	if got != "" {
		t.Fatalf("non-git directory returned git context:\n%s", got)
	}
}

func TestLoadGitContextTimeoutReturnsEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell sleep script is POSIX-only")
	}
	repo := initPromptTestRepo(t)
	fakeGit := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nsleep 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := LoadGitContextWithOptions(GitContextOptions{
		CWD:     repo,
		GitPath: fakeGit,
		Timeout: 10 * time.Millisecond,
	})
	if got != "" {
		t.Fatalf("timeout/error should return empty git context, got:\n%s", got)
	}
}

func TestLoadGitContextLongStatusTruncates(t *testing.T) {
	repo := initPromptTestRepo(t)
	for i := 0; i < 260; i++ {
		name := filepath.Join(repo, fmt.Sprintf("untracked-file-with-a-long-name-%03d.txt", i))
		if err := os.WriteFile(name, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := LoadGitContext(repo)
	if !strings.Contains(got, "... (truncated because it exceeds 2k characters.") {
		t.Fatalf("expected truncation suffix in:\n%s", got)
	}
	statusStart := strings.Index(got, "Status:\n")
	if statusStart < 0 {
		t.Fatalf("expected Status section in:\n%s", got)
	}
	recentStart := strings.Index(got[statusStart:], "\n\nRecent commits:")
	if recentStart < 0 {
		t.Fatalf("expected Recent commits after Status in:\n%s", got)
	}
	statusBlock := got[statusStart+len("Status:\n") : statusStart+recentStart]
	if len(statusBlock) < maxGitStatusChars || len(statusBlock) > maxGitStatusChars+160 {
		t.Fatalf("status block length = %d, want around %d with suffix", len(statusBlock), maxGitStatusChars)
	}
}

func TestLoadGitContextDisableGate(t *testing.T) {
	repo := initPromptTestRepo(t)
	t.Setenv("CLAUDE_CODE_DISABLE_GIT_INSTRUCTIONS", "true")
	if got := LoadGitContext(repo); got != "" {
		t.Fatalf("disabled git instructions should return empty context, got:\n%s", got)
	}
}

func TestLoadGitContextDisableOption(t *testing.T) {
	repo := initPromptTestRepo(t)
	got := LoadGitContextWithOptions(GitContextOptions{
		CWD:                    repo,
		DisableGitInstructions: true,
	})
	if got != "" {
		t.Fatalf("disabled git instructions option should return empty context, got:\n%s", got)
	}
}

func initPromptTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runPromptTestGit(t, repo, "init", "-b", "main")
	runPromptTestGit(t, repo, "config", "user.name", "Prompt Tester")
	runPromptTestGit(t, repo, "config", "user.email", "prompt@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runPromptTestGit(t, repo, "add", "tracked.txt")
	runPromptTestGit(t, repo, "commit", "-m", "initial commit")
	return repo
}

func runPromptTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if args[0] == "init" {
			t.Skipf("git unavailable or too old for test setup: git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}
