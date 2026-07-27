package harness

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureGitWorkspaceIncludesTrackedUntrackedDeletedAndBinary(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repository, "init")
	runTestGit(t, repository, "config", "user.email", "benchmark@example.invalid")
	runTestGit(t, repository, "config", "user.name", "Benchmark")
	writeTestFile(t, filepath.Join(repository, "tracked.txt"), []byte("base\n"))
	writeTestFile(t, filepath.Join(repository, "deleted.txt"), []byte("delete me\n"))
	runTestGit(t, repository, "add", ".")
	runTestGit(t, repository, "commit", "-m", "base")
	base := strings.TrimSpace(runTestGit(t, repository, "rev-parse", "HEAD"))

	writeTestFile(t, filepath.Join(repository, "tracked.txt"), []byte("changed\n"))
	if err := os.Remove(filepath.Join(repository, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repository, "new.txt"), []byte("untracked\n"))
	binary := []byte{0, 1, 2, 3, 0xff, 0, 4}
	writeTestFile(t, filepath.Join(repository, "new.bin"), binary)
	patchPath := filepath.Join(t.TempDir(), "submission.patch")
	capture, err := CaptureGitWorkspaceEvidence(context.Background(), repository, base, patchPath)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Method != "temporary-git-index-v1" || capture.BaseCommit != base || !capture.IncludesTracked || !capture.IncludesUntracked || !capture.IncludesBinary {
		t.Fatalf("capture evidence is incomplete: %#v", capture)
	}
	patch, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"tracked.txt", "deleted.txt", "new.txt", "new.bin", "GIT binary patch"} {
		if !bytes.Contains(patch, []byte(marker)) {
			t.Errorf("patch does not contain %q", marker)
		}
	}

	checkout := filepath.Join(t.TempDir(), "checkout")
	runTestGit(t, t.TempDir(), "clone", repository, checkout)
	runTestGit(t, checkout, "apply", "--binary", patchPath)
	assertTestFile(t, filepath.Join(checkout, "tracked.txt"), []byte("changed\n"))
	assertTestFile(t, filepath.Join(checkout, "new.txt"), []byte("untracked\n"))
	assertTestFile(t, filepath.Join(checkout, "new.bin"), binary)
	if _, err := os.Stat(filepath.Join(checkout, "deleted.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists or stat failed unexpectedly: %v", err)
	}
}

func runTestGit(t testing.TB, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func writeTestFile(t testing.TB, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertTestFile(t testing.TB, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", path, got, want)
	}
}
