package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCaptureRunSourceSnapshotUsesGitSourceSet(t *testing.T) {
	root := t.TempDir()
	runGitSnapshotTestCommand(t, root, "init", "-q")
	writeRunSnapshotTestFile(t, root, ".gitignore", "node_modules/\ntarget/\n")
	writeRunSnapshotTestFile(t, root, "source.go", "package example\n")
	runGitSnapshotTestCommand(t, root, "add", ".gitignore", "source.go")

	before, err := captureRunSourceSnapshot(root)
	if err != nil {
		t.Fatalf("capture before: %v", err)
	}
	writeRunSnapshotTestFile(t, root, "node_modules/pkg/generated.js", "generated\n")
	writeRunSnapshotTestFile(t, root, "target/classes/generated.class", "generated\n")
	afterIgnored, err := captureRunSourceSnapshot(root)
	if err != nil {
		t.Fatalf("capture ignored: %v", err)
	}
	if changed, err := changedRunSourcePaths(before, afterIgnored); err != nil || len(changed) != 0 {
		t.Fatalf("ignored outputs changed snapshot: paths=%v err=%v", changed, err)
	}

	writeRunSnapshotTestFile(t, root, "source.go", "package changed\n")
	afterTracked, err := captureRunSourceSnapshot(root)
	if err != nil {
		t.Fatalf("capture tracked change: %v", err)
	}
	changed, err := changedRunSourcePaths(afterIgnored, afterTracked)
	if err != nil || len(changed) != 1 || changed[0] != filepath.Join(before.root, "source.go") {
		t.Fatalf("tracked change = %v, err=%v", changed, err)
	}
}

func TestCaptureRunSourceSnapshotIncludesUntrackedSource(t *testing.T) {
	root := t.TempDir()
	runGitSnapshotTestCommand(t, root, "init", "-q")
	before, err := captureRunSourceSnapshot(root)
	if err != nil {
		t.Fatalf("capture before: %v", err)
	}
	writeRunSnapshotTestFile(t, root, "new_test.go", "package example\n")
	after, err := captureRunSourceSnapshot(root)
	if err != nil {
		t.Fatalf("capture after: %v", err)
	}
	changed, err := changedRunSourcePaths(before, after)
	if err != nil || len(changed) != 1 || changed[0] != filepath.Join(before.root, "new_test.go") {
		t.Fatalf("untracked change = %v, err=%v", changed, err)
	}
}

func TestCaptureRunSourceSnapshotExcludesUntrackedGeneratedDirectories(t *testing.T) {
	root := t.TempDir()
	runGitSnapshotTestCommand(t, root, "init", "-q")
	before, err := captureRunSourceSnapshot(root)
	if err != nil {
		t.Fatalf("capture before: %v", err)
	}
	writeRunSnapshotTestFile(t, root, ".luban-build/CMakeCache.txt", "generated\n")
	writeRunSnapshotTestFile(t, root, "build/output.o", "generated\n")
	writeRunSnapshotTestFile(t, root, "build-make/output.o", "generated\n")
	after, err := captureRunSourceSnapshot(root)
	if err != nil {
		t.Fatalf("capture after: %v", err)
	}
	if changed, err := changedRunSourcePaths(before, after); err != nil || len(changed) != 0 {
		t.Fatalf("generated outputs changed snapshot: paths=%v err=%v", changed, err)
	}
}

func TestCaptureRunSourceSnapshotKeepsTrackedBuildDirectory(t *testing.T) {
	root := t.TempDir()
	runGitSnapshotTestCommand(t, root, "init", "-q")
	writeRunSnapshotTestFile(t, root, "build/source.cmake", "before\n")
	runGitSnapshotTestCommand(t, root, "add", "build/source.cmake")
	before, err := captureRunSourceSnapshot(root)
	if err != nil {
		t.Fatalf("capture before: %v", err)
	}
	writeRunSnapshotTestFile(t, root, "build/source.cmake", "after\n")
	after, err := captureRunSourceSnapshot(root)
	if err != nil {
		t.Fatalf("capture after: %v", err)
	}
	changed, err := changedRunSourcePaths(before, after)
	if err != nil || len(changed) != 1 || changed[0] != filepath.Join(before.root, "build", "source.cmake") {
		t.Fatalf("tracked build source change = %v, err=%v", changed, err)
	}
}

func runGitSnapshotTestCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, output)
	}
}

func writeRunSnapshotTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relative, err)
	}
}
