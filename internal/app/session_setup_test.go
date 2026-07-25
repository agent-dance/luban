package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/types"
)

func TestResolveSession_NewSessionUsesUUIDAndProjectDir(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	buf := &bytes.Buffer{}

	resolved, err := ResolveSession("", false, repo, "/repo/current", buf)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Resumed {
		t.Fatal("expected fresh session")
	}
	if _, err := uuid.Parse(resolved.Ref.ID); err != nil {
		t.Fatalf("expected UUID session id, got %q", resolved.Ref.ID)
	}
	if resolved.Ref.ProjectDir != repo.ProjectDirForCWD("/repo/current") {
		t.Fatalf("unexpected project dir: %+v", resolved.Ref)
	}
}

func TestResolveSession_ExplicitNewSessionIsNotAWarning(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	buf := &bytes.Buffer{}

	resolved, err := ResolveSession("sdk-new-session", false, repo, "/repo/current", buf)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Resumed || resolved.Ref.ID != "sdk-new-session" {
		t.Fatalf("unexpected explicit session result: %+v", resolved)
	}
	if buf.Len() != 0 {
		t.Fatalf("fresh explicit session emitted a warning: %q", buf.String())
	}
}

func TestResolveSession_ResumeReturnsStoredProjectAndCWD(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := filepath.Join(repo.ProjectsRoot(), "project-a")
	store := repo.StoreForProjectDir(projectDir)
	if err := store.Save("sess-a", []types.Message{types.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta("sess-a", session.SessionMeta{CWD: "/repo/a/worktree"}); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveSession("sess-a", false, repo, "/repo/current", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Resumed {
		t.Fatal("expected resumed session")
	}
	if resolved.Ref.ProjectDir != projectDir {
		t.Fatalf("unexpected ref: %+v", resolved.Ref)
	}
	if resolved.SessionCWD != "/repo/a/worktree" {
		t.Fatalf("unexpected session cwd: %q", resolved.SessionCWD)
	}
}

func TestResolveSession_CorruptMetadataFailsClosedWithoutOverwrite(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD("/repo/current")
	store := repo.StoreForProjectDir(projectDir)
	const sessionID = "corrupt-startup-meta"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("preserve me")}); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(projectDir, sessionID+".meta.json")
	want := []byte("{not valid json\n")
	if err := os.WriteFile(metaPath, want, 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveSession(sessionID, false, repo, "/repo/current", &bytes.Buffer{})
	if err == nil {
		t.Fatalf("ResolveSession unexpectedly resumed corrupt metadata: %+v", resolved)
	}
	got, readErr := os.ReadFile(metaPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("corrupt metadata was overwritten during resolution: got %q want %q", got, want)
	}
}

func TestResolveSession_LatestCorruptMetadataFailsClosed(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD("/repo/current")
	store := repo.StoreForProjectDir(projectDir)
	const sessionID = "corrupt-latest-meta"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("preserve me")}); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(projectDir, sessionID+".meta.json")
	want := []byte("{not valid json\n")
	if err := os.WriteFile(metaPath, want, 0o644); err != nil {
		t.Fatal(err)
	}

	if resolved, err := ResolveSession("", true, repo, "/repo/current", &bytes.Buffer{}); err == nil {
		t.Fatalf("latest resume unexpectedly bypassed corrupt metadata: %+v", resolved)
	}
	got, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("latest resume overwrote metadata: got %q want %q", got, want)
	}
}
