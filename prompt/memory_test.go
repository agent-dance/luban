package prompt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoverMemoryFilesPriorityAndNestedOrder(t *testing.T) {
	tmp := t.TempDir()
	managed := filepath.Join(tmp, "managed")
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")
	child := filepath.Join(project, "child")
	leaf := filepath.Join(child, "leaf")

	writeFile(t, filepath.Join(managed, "CLAUDE.md"), "managed")
	writeFile(t, filepath.Join(user, "CLAUDE.md"), "user")
	writeFile(t, filepath.Join(project, "CLAUDE.md"), "project root")
	writeFile(t, filepath.Join(project, ".claude", "CLAUDE.md"), "project dot root")
	writeFile(t, filepath.Join(project, "CLAUDE.local.md"), "local root")
	writeFile(t, filepath.Join(child, "CLAUDE.md"), "project child")
	writeFile(t, filepath.Join(leaf, ".claude", "CLAUDE.md"), "project dot leaf")
	writeFile(t, filepath.Join(leaf, "CLAUDE.local.md"), "local leaf")

	files := discoverMemoryFiles(leaf, memoryPaths{managedDir: managed, userDir: user})
	got := memoryContents(files)
	want := []string{
		"managed",
		"user",
		"project root",
		"project dot root",
		"local root",
		"project child",
		"project dot leaf",
		"local leaf",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected memory order:\nwant %q\ngot  %q", want, got)
	}

	if files[0].Type != MemoryTypeManaged || files[1].Type != MemoryTypeUser {
		t.Fatalf("expected managed then user, got %s then %s", files[0].Type, files[1].Type)
	}
	if files[len(files)-1].Type != MemoryTypeLocal {
		t.Fatalf("expected nearest local memory last, got %s", files[len(files)-1].Type)
	}
}

func TestDiscoverMemoryFilesLoadsLUBANAndLegacyInstructionsInPriorityOrder(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	userClaude := filepath.Join(tmp, ".claude")
	userDeepSeek := filepath.Join(tmp, ".deepseek-code")
	userLuban := filepath.Join(tmp, ".luban-code")

	writeFile(t, filepath.Join(userClaude, "CLAUDE.md"), "user claude")
	writeFile(t, filepath.Join(userDeepSeek, "DEEPSEEK.md"), "user deepseek")
	writeFile(t, filepath.Join(userLuban, "LUBAN.md"), "user luban")
	writeFile(t, filepath.Join(project, "CLAUDE.md"), "project claude")
	writeFile(t, filepath.Join(project, "AGENTS.md"), "project agents")
	writeFile(t, filepath.Join(project, "DEEPSEEK.md"), "project deepseek")
	writeFile(t, filepath.Join(project, "LUBAN.md"), "project luban")

	files := discoverMemoryFiles(project, memoryPaths{
		userDirs: []string{userClaude, userDeepSeek, userLuban},
	})
	got := memoryContents(files)
	want := []string{
		"user claude", "user deepseek", "user luban",
		"project claude", "project agents", "project deepseek", "project luban",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected branded memory order:\nwant %q\ngot  %q", want, got)
	}
}

func TestFormatMemoryFilesUsesOriginalStyleBlocksAndDescriptions(t *testing.T) {
	files := []MemoryFileInfo{
		{Path: "/repo/CLAUDE.md", Type: MemoryTypeProject, Content: " project instructions \n"},
		{Path: "/repo/CLAUDE.local.md", Type: MemoryTypeLocal, Content: "local instructions"},
		{Path: "/home/user/.claude/CLAUDE.md", Type: MemoryTypeUser, Content: "user instructions"},
	}

	got := FormatMemoryFiles(files)
	assertInOrder(t, got, []string{
		memoryInstructionPrompt,
		"Contents of /repo/CLAUDE.md (project instructions, checked into the codebase):\n\nproject instructions",
		"Contents of /repo/CLAUDE.local.md (user's private project instructions, not checked in):\n\nlocal instructions",
		"Contents of /home/user/.claude/CLAUDE.md (user's private global instructions for all projects):\n\nuser instructions",
	})
}

func TestDiscoverMemoryFilesToleratesMissingDirectorySymlinkAndPermissionErrors(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	cwd := filepath.Join(project, "nested")
	if err := os.MkdirAll(filepath.Join(project, ".claude", "CLAUDE.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(project, "CLAUDE.md"), "project")

	broken := filepath.Join(cwd, "CLAUDE.md")
	if runtime.GOOS != "windows" {
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(tmp, "missing.md"), broken); err != nil {
			t.Fatal(err)
		}
	}

	local := filepath.Join(project, "CLAUDE.local.md")
	writeFile(t, local, "unreadable local")
	if runtime.GOOS != "windows" {
		if err := os.Chmod(local, 0); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(local, 0o644)
	}

	files := discoverMemoryFiles(cwd, memoryPaths{
		managedDir: filepath.Join(tmp, "missing-managed"),
		userDir:    filepath.Join(tmp, "missing-user"),
	})
	if len(files) == 0 || files[0].Content != "project" {
		t.Fatalf("expected readable project memory despite unreadable/missing candidates, got %#v", files)
	}
}

func TestDiscoverMemoryFilesDeduplicatesSymlinkedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows hosts")
	}

	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	target := filepath.Join(project, "CLAUDE.md")
	link := filepath.Join(project, ".claude", "CLAUDE.md")
	writeFile(t, target, "project")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	files := discoverMemoryFiles(project, memoryPaths{})
	if len(files) != 1 {
		t.Fatalf("expected symlinked duplicate to be skipped, got %#v", files)
	}
}

func TestDiscoverClaudeMDCompatibilityWrapper(t *testing.T) {
	tmp := t.TempDir()
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(user, "CLAUDE.md"), "user")
	writeFile(t, filepath.Join(project, "CLAUDE.md"), "project")

	t.Setenv("CLAUDE_CONFIG_DIR", user)
	t.Setenv("USER_TYPE", "ant")
	t.Setenv("CLAUDE_CODE_MANAGED_SETTINGS_PATH", filepath.Join(tmp, "managed"))

	got := DiscoverClaudeMD(project)
	assertInOrder(t, got, []string{
		"Contents of " + filepath.Join(user, "CLAUDE.md"),
		"Contents of " + filepath.Join(project, "CLAUDE.md"),
	})
}

func memoryContents(files []MemoryFileInfo) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, file.Content)
	}
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
