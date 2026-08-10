package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/types"
)

func TestSessionStoreAdapterCurrentDoesNotEnumerateOtherProjects(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	currentProject := repo.ProjectDirForCWD("/workspace/current")
	otherProject := repo.ProjectDirForCWD("/workspace/other")
	if err := repo.Save("current-session", currentProject, []types.Message{types.UserMessage("current")}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMeta("current-session", currentProject, session.SessionMeta{Title: "current title"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save("other-session", otherProject, []types.Message{types.UserMessage("other")}); err != nil {
		t.Fatal(err)
	}
	unsafeMeta := filepath.Join(otherProject, "other-session.meta.json")
	if err := os.Remove(unsafeMeta); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(unsafeMeta, 0o700); err != nil {
		t.Fatal(err)
	}

	adapter := &sessionStoreAdapter{repo: repo, currentProjectDir: func() string { return currentProject }}
	entry, err := adapter.Current("current-session")
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID != "current-session" || entry.Title != "current title" || entry.ProjectDir != currentProject {
		t.Fatalf("precise current entry = %+v", entry)
	}
	if _, err := adapter.List(); err == nil {
		t.Fatal("global list unexpectedly hid unrelated metadata path safety failure")
	}
}

func TestSessionStoreAdapterCurrentDoesNotResolveSameIDFromAnotherProject(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	currentProject := repo.ProjectDirForCWD("/workspace/current")
	otherProject := repo.ProjectDirForCWD("/workspace/other")
	if err := repo.Save("shared-session", otherProject, []types.Message{types.UserMessage("other")}); err != nil {
		t.Fatal(err)
	}
	adapter := &sessionStoreAdapter{repo: repo, currentProjectDir: func() string { return currentProject }}
	if _, err := adapter.Current("shared-session"); err == nil {
		t.Fatal("current lookup crossed into another project")
	}
}
