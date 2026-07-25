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

	writeFile(t, filepath.Join(managed, "LUBAN.md"), "managed")
	writeFile(t, filepath.Join(user, "LUBAN.md"), "user")
	writeFile(t, filepath.Join(project, "LUBAN.md"), "project root")
	writeFile(t, filepath.Join(project, ".luban-code", "LUBAN.md"), "project dot root")
	writeFile(t, filepath.Join(project, "LUBAN.local.md"), "local root")
	writeFile(t, filepath.Join(child, "AGENTS.md"), "project child")
	writeFile(t, filepath.Join(leaf, ".luban-code", "LUBAN.md"), "project dot leaf")
	writeFile(t, filepath.Join(leaf, "LUBAN.local.md"), "local leaf")

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

func TestDiscoverMemoryFilesIgnoresHistoricalInstructionNames(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	historicalUserDir := filepath.Join(tmp, "historical-user-config")
	currentUserDir := filepath.Join(tmp, ".luban-code")

	writeFile(t, filepath.Join(historicalUserDir, "CLAUDE.md"), "historical user instructions")
	writeFile(t, filepath.Join(currentUserDir, "LUBAN.md"), "current user instructions")
	writeFile(t, filepath.Join(project, "CLAUDE.md"), "historical project instructions")
	writeFile(t, filepath.Join(project, "AGENTS.md"), "project agent instructions")
	writeFile(t, filepath.Join(project, "LUBAN.md"), "current project instructions")

	files := discoverMemoryFiles(project, memoryPaths{
		userDirs: []string{historicalUserDir, currentUserDir},
	})
	got := memoryContents(files)
	want := []string{
		"current user instructions",
		"project agent instructions",
		"current project instructions",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected instruction discovery result:\nwant %q\ngot  %q", want, got)
	}
}

func TestFormatMemoryFilesUsesSourceDescriptions(t *testing.T) {
	files := []MemoryFileInfo{
		{Path: "/repo/LUBAN.md", Type: MemoryTypeProject, Content: " project instructions \n"},
		{Path: "/repo/LUBAN.local.md", Type: MemoryTypeLocal, Content: "local instructions"},
		{Path: "/home/user/.luban-code/LUBAN.md", Type: MemoryTypeUser, Content: "user instructions"},
	}

	got := FormatMemoryFiles(files)
	assertInOrder(t, got, []string{
		memoryInstructionPrompt,
		"Contents of /repo/LUBAN.md (project instructions, checked into the codebase):\n\nproject instructions",
		"Contents of /repo/LUBAN.local.md (user's private project instructions, not checked in):\n\nlocal instructions",
		"Contents of /home/user/.luban-code/LUBAN.md (user's private global instructions for all projects):\n\nuser instructions",
	})
}

func TestDiscoverMemoryFilesToleratesMissingDirectorySymlinkAndPermissionErrors(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	cwd := filepath.Join(project, "nested")
	if err := os.MkdirAll(filepath.Join(project, ".luban-code", "LUBAN.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(project, "LUBAN.md"), "project")

	broken := filepath.Join(cwd, "LUBAN.md")
	if runtime.GOOS != "windows" {
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(tmp, "missing.md"), broken); err != nil {
			t.Fatal(err)
		}
	}

	local := filepath.Join(project, "LUBAN.local.md")
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
	target := filepath.Join(project, "LUBAN.md")
	link := filepath.Join(project, ".luban-code", "LUBAN.md")
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

func TestDiscoverInstructionsUsesCanonicalPaths(t *testing.T) {
	tmp := t.TempDir()
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")
	writeFile(t, filepath.Join(user, "LUBAN.md"), "user")
	writeFile(t, filepath.Join(project, "LUBAN.md"), "project")

	t.Setenv("LUBAN_CODE_CONFIG_DIR", user)
	t.Setenv("USER_TYPE", "ant")
	t.Setenv("LUBAN_CODE_MANAGED_SETTINGS_PATH", filepath.Join(tmp, "managed"))

	got := DiscoverInstructions(project)
	assertInOrder(t, got, []string{
		"Contents of " + filepath.Join(user, "LUBAN.md"),
		"Contents of " + filepath.Join(project, "LUBAN.md"),
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
