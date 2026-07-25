package engine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/types"
)

func TestEngineQueryProjectRootOverridesNestedExecutionCWDForEvents(t *testing.T) {
	const projectRoot = "/workspace/project"
	eng, err := New(Config{
		Provider: &mockProvider{name: "project-identity", modelID: "project-identity-model"},
		Sessions: newMemorySessionManager(), ProjectRoot: "/workspace/default", CWD: "/workspace/default/nested",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Shutdown(context.Background())

	events, err := eng.Query(context.Background(), QueryRequest{
		SessionID: "session-project", Message: "hello", ProjectRoot: projectRoot, CWD: "/workspace/project/nested",
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for event := range events {
		if event.Final {
			if event.Error != nil {
				t.Fatal(event.Error)
			}
			continue
		}
		seen++
		if event.Inner.ProjectRoot != projectRoot {
			t.Fatalf("event %s project root = %q, want %q", event.Inner.Type, event.Inner.ProjectRoot, projectRoot)
		}
	}
	if seen == 0 {
		t.Fatal("engine query emitted no events")
	}
}

func TestRepositorySessionManagerRejectsResumeWithoutProjectIdentity(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	const sessionID = "unscoped-resume"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("stored")}); err != nil {
		t.Fatal(err)
	}

	manager := newRepositorySessionManager(repo, func() string { return "" })
	if _, err := manager.Load(sessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Load without project identity error = %v, want ErrSessionNotFound", err)
	}
}

func TestQueryCWDDoesNotSelectSessionProject(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	rootA := t.TempDir()
	rootB := t.TempDir()
	projectA := repo.ProjectDirForCWD(rootA)
	projectB := repo.ProjectDirForCWD(rootB)
	const sessionID = "cwd-is-not-project-identity"

	eng, err := New(Config{
		Provider:    &mockProvider{name: "project-identity", modelID: "project-identity-model"},
		Sessions:    newRepositorySessionManager(repo, func() string { return projectA }),
		ProjectRoot: rootA,
		CWD:         rootA,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Shutdown(context.Background())

	events, err := eng.Query(context.Background(), QueryRequest{
		SessionID: sessionID,
		Message:   "run from a nested or alternate execution directory",
		CWD:       rootB,
	})
	if err != nil {
		t.Fatal(err)
	}
	for event := range events {
		if event.Final && event.Error != nil {
			t.Fatal(event.Error)
		}
	}

	if _, err := repo.StoreForProjectDir(projectA).Load(sessionID); err != nil {
		t.Fatalf("load active project transcript: %v", err)
	}
	if _, err := repo.StoreForProjectDir(projectB).Load(sessionID); err == nil {
		t.Fatalf("CWD created transcript in inferred project %q", filepath.Clean(projectB))
	}
}
