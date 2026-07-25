package session

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"

	storepaths "github.com/agent-dance/luban/internal/store/paths"
	"github.com/agent-dance/luban/types"
)

func TestProjectKeyForCWD_IsStableAndUnique(t *testing.T) {
	keyA1 := storepaths.ProjectKeyForCWD("/repo/a")
	keyA2 := storepaths.ProjectKeyForCWD("/repo/a")
	keyB := storepaths.ProjectKeyForCWD("/repo/b")

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
