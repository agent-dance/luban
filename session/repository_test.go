package session

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestProjectKeyForCWD_IsStableAndUnique(t *testing.T) {
	keyA1 := ProjectKeyForCWD("/repo/a")
	keyA2 := ProjectKeyForCWD("/repo/a")
	keyB := ProjectKeyForCWD("/repo/b")

	if keyA1 == "" {
		t.Fatal("expected non-empty key")
	}
	if keyA1 != keyA2 {
		t.Fatalf("expected stable key, got %q vs %q", keyA1, keyA2)
	}
	if keyA1 == keyB {
		t.Fatalf("expected different keys for different paths, got %q", keyA1)
	}
}

func TestDefaultRepositoryReadsLegacyDeepSeekSessionsAndWritesLUBAN(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cwd := filepath.Join(home, "project")
	legacyProject := filepath.Join(home, ".deepseek-code", "projects", ProjectKeyForCWD(cwd))
	if err := NewFileStore(legacyProject).Save("legacy-session", []types.Message{types.UserMessage("legacy")}); err != nil {
		t.Fatal(err)
	}

	repo := DefaultRepository()
	ref, err := repo.Resolve("legacy-session", "")
	if err != nil {
		t.Fatalf("resolve legacy DeepSeek session: %v", err)
	}
	if ref.ProjectDir != legacyProject {
		t.Fatalf("resolved project = %q, want %q", ref.ProjectDir, legacyProject)
	}

	currentProject := repo.ProjectDirForCWD(cwd)
	if err := repo.Save("luban-session", currentProject, []types.Message{types.UserMessage("current")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(currentProject, "luban-session.jsonl")); err != nil {
		t.Fatalf("LUBAN session was not written under current config: %v", err)
	}
}

func TestDefaultRepositoryDoesNotIndexClaudeCodeProjectTranscripts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	nativeProject := filepath.Join(home, ".claude", "projects", "native")
	if err := os.MkdirAll(nativeProject, 0o755); err != nil {
		t.Fatal(err)
	}
	nativeTranscript := []byte(
		`{"type":"queue-operation","content":"hello","operation":"enqueue"}` + "\n" +
			`{"type":"user","message":{"role":"user","content":"hello"}}` + "\n",
	)
	nativePath := filepath.Join(nativeProject, "foreign.jsonl")
	if err := os.WriteFile(nativePath, nativeTranscript, 0o644); err != nil {
		t.Fatal(err)
	}

	repo := DefaultRepository()
	got, err := repo.Search(SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("foreign Claude sessions were indexed: %+v", got)
	}
	if _, err := repo.Resolve("foreign", ""); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Resolve foreign error = %v, want fs.ErrNotExist", err)
	}
	unchanged, err := os.ReadFile(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unchanged, nativeTranscript) {
		t.Fatalf("foreign transcript changed: got %q want %q", unchanged, nativeTranscript)
	}
	if _, err := os.Stat(filepath.Join(nativeProject, "foreign.meta.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("foreign transcript gained a metadata sidecar: %v", err)
	}
}

func TestDefaultRepositorySearchesAndLoadsOwnedLegacyLayouts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	want := map[string]string{
		"luban-project":      "luban project",
		"luban-flat":         "luban flat",
		"deepseek-project": "deepseek project",
		"deepseek-flat":    "deepseek flat",
		"go-project":       "go project",
		"go-flat":          "go flat",
		"claude-flat":      "claude flat",
	}
	stores := map[string]string{
		"luban-project":      filepath.Join(home, ".luban-code", "projects", "current-project"),
		"luban-flat":         filepath.Join(home, ".luban-code", "sessions"),
		"deepseek-project": filepath.Join(home, ".deepseek-code", "projects", "legacy-project"),
		"deepseek-flat":    filepath.Join(home, ".deepseek-code", "sessions"),
		"go-project":       filepath.Join(home, ".claude-go", "projects", "legacy-project"),
		"go-flat":          filepath.Join(home, ".claude-go", "sessions"),
		"claude-flat":      filepath.Join(home, ".claude", "sessions"),
	}
	for id, dir := range stores {
		if err := NewFileStore(dir).Save(id, []types.Message{types.UserMessage(want[id])}); err != nil {
			t.Fatal(err)
		}
	}

	repo := DefaultRepository()
	found, err := repo.Search(SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != len(want) {
		t.Fatalf("Search returned %d sessions, want %d: %+v", len(found), len(want), found)
	}
	for id, wantText := range want {
		ref, err := repo.Resolve(id, "")
		if err != nil {
			t.Fatalf("resolve %s: %v", id, err)
		}
		messages, err := repo.Load(ref)
		if err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		if len(messages) != 1 || messages[0].GetText() != wantText {
			t.Fatalf("load %s = %#v, want %q", id, messages, wantText)
		}
	}
}

func TestRepositorySearchAcrossProjects(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(root)

	storeA := repo.StoreForProjectDir(filepath.Join(root, "projects", "a"))
	storeB := repo.StoreForProjectDir(filepath.Join(root, "projects", "b"))

	if err := storeA.Save("sess-a", []types.Message{types.UserMessage("hello from a")}); err != nil {
		t.Fatal(err)
	}
	if err := storeA.SaveMeta("sess-a", SessionMeta{Title: "alpha", CWD: "/repo/a"}); err != nil {
		t.Fatal(err)
	}
	if err := storeB.Save("sess-b", []types.Message{types.UserMessage("hello from b")}); err != nil {
		t.Fatal(err)
	}
	if err := storeB.SaveMeta("sess-b", SessionMeta{Title: "beta", CWD: "/repo/b"}); err != nil {
		t.Fatal(err)
	}

	results, err := repo.Search(SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(results))
	}
	if results[0].ProjectDir == "" || results[1].ProjectDir == "" {
		t.Fatalf("expected project dirs in results: %+v", results)
	}
}

func TestRepositoryResolvePrefersCurrentProject(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(root)

	projectA := filepath.Join(root, "projects", "a")
	projectB := filepath.Join(root, "projects", "b")
	id := "shared-id"

	if err := repo.StoreForProjectDir(projectA).Save(id, []types.Message{types.UserMessage("from a")}); err != nil {
		t.Fatal(err)
	}
	if err := repo.StoreForProjectDir(projectB).Save(id, []types.Message{types.UserMessage("from b")}); err != nil {
		t.Fatal(err)
	}

	ref, err := repo.Resolve(id, projectB)
	if err != nil {
		t.Fatal(err)
	}
	if ref.ProjectDir != projectB {
		t.Fatalf("expected projectB resolution, got %+v", ref)
	}
}

func TestRepositoryLoadByIDIncludesResolvedRef(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(root)
	projectDir := filepath.Join(root, "projects", "a")

	if err := repo.StoreForProjectDir(projectDir).Save("sess-1", []types.Message{
		types.UserMessage("hello"),
		types.AssistantMessage("world"),
	}); err != nil {
		t.Fatal(err)
	}

	msgs, ref, err := repo.LoadByID("sess-1", projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if ref.ProjectDir != projectDir {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestRepositoryResolveMissingWrapsNotExist(t *testing.T) {
	repo := NewRepository(t.TempDir())
	_, err := repo.Resolve("missing-session", filepath.Join(t.TempDir(), "project"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("resolve error = %v, want fs.ErrNotExist", err)
	}
}

func TestRepositoryToolUseIdentityLedgersAreProjectScoped(t *testing.T) {
	repo := NewRepository(t.TempDir())
	projectA := repo.ProjectDirForCWD("/workspace/a")
	projectB := repo.ProjectDirForCWD("/workspace/b")
	const sessionID = "duplicate-session-id"
	for _, project := range []string{projectA, projectB} {
		if err := repo.Save(sessionID, project, []types.Message{types.UserMessage("compacted")}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SaveMeta(sessionID, projectA, SessionMeta{SeenToolUseIDs: []string{"tool-a"}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMeta(sessionID, projectB, SessionMeta{SeenToolUseIDs: []string{"tool-b"}}); err != nil {
		t.Fatal(err)
	}
	metaA, _, err := repo.GetMeta(sessionID, projectA)
	if err != nil {
		t.Fatal(err)
	}
	metaB, _, err := repo.GetMeta(sessionID, projectB)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(metaA.SeenToolUseIDs, []string{"tool-a"}) || !reflect.DeepEqual(metaB.SeenToolUseIDs, []string{"tool-b"}) {
		t.Fatalf("project ledgers crossed: A=%v B=%v", metaA.SeenToolUseIDs, metaB.SeenToolUseIDs)
	}
}

func TestRepositoryDeleteHidesDurablyDeletedTranscriptFromResolveAndList(t *testing.T) {
	repo := NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	const sessionID = "durably-deleted"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("old")}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(sessionID, projectDir); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Resolve(sessionID, projectDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Resolve deleted session = %v, want not exist", err)
	}
	items, err := repo.Search(SearchOptions{CurrentProjectDir: projectDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("deleted session remained in list: %#v", items)
	}
	deleted, err := repo.IsDeleted(sessionID, projectDir)
	if err != nil || !deleted {
		t.Fatalf("Repository.IsDeleted = %v, %v", deleted, err)
	}
}
