package toolbase

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPathContainsUsesRelativeDirectoryBoundary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if !PathContains(root, root) {
		t.Fatal("root did not contain itself")
	}
	if !PathContains(root+string(filepath.Separator), root) {
		t.Fatal("trailing separator changed root containment")
	}
	if !PathContains(root, filepath.Join(root, "src", "main.go")) {
		t.Fatal("descendant was not contained")
	}
	if PathContains(root, root+"-other") {
		t.Fatal("shared string prefix crossed a directory boundary")
	}
}

func TestPathWithinAllowedDirsResolvesRelativeRoots(t *testing.T) {
	root := t.TempDir()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativeRoot, err := filepath.Rel(workingDirectory, root)
	if err != nil {
		t.Fatal(err)
	}
	if !PathWithinAllowedDirs(filepath.Join(root, "child"), []string{relativeRoot}) {
		t.Fatal("relative allowed root did not contain child")
	}
}

func TestPathWithinAllowedDirsAcceptsRootItselfAndTrailingSeparator(t *testing.T) {
	root := t.TempDir()
	for _, allowedRoot := range []string{root, root + string(filepath.Separator)} {
		if !PathWithinAllowedDirs(root, []string{allowedRoot}) {
			t.Fatalf("candidate root %q was not contained by %q", root, allowedRoot)
		}
	}
}

func TestPathWithinAllowedDirsCanonicalizesDarwinPrivateAlias(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin exposes /private aliases for public system directories")
	}
	if !PathWithinAllowedDirs("/private/tmp/project/child", []string{"/tmp/project"}) {
		t.Fatal("Darwin /private alias changed containment")
	}
}

func TestPathWithinAllowedDirsResolvesSymlinkRootAndCandidate(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real-project")
	if err := os.Mkdir(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(base, "linked-project")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	tests := []struct {
		name      string
		candidate string
		root      string
	}{
		{
			name:      "symlink root contains real candidate",
			candidate: filepath.Join(realRoot, "not-created", "file.txt"),
			root:      linkedRoot,
		},
		{
			name:      "real root contains symlink candidate",
			candidate: filepath.Join(linkedRoot, "not-created", "file.txt"),
			root:      realRoot,
		},
		{
			name:      "symlink root contains itself",
			candidate: linkedRoot,
			root:      realRoot,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !PathWithinAllowedDirs(test.candidate, []string{test.root}) {
				t.Fatalf("candidate %q was not contained by %q", test.candidate, test.root)
			}
		})
	}
}

func TestPathWithinAllowedDirsRejectsSymlinkEscapeWithMissingLeaf(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{root, outside} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	escape := filepath.Join(root, "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	candidate := filepath.Join(escape, "not-created", "file.txt")
	if PathWithinAllowedDirs(candidate, []string{root}) {
		t.Fatalf("symlink escape %q was contained by %q", candidate, root)
	}
}

func TestSuggestNearbyPath(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "config.json")
	if err := os.WriteFile(want, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := SuggestNearbyPath(filepath.Join(root, "confg.json"), root); got != want {
		t.Fatalf("suggestion = %q, want %q", got, want)
	}
}
