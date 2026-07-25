package sdk

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/types"
)

func currentSessionsRepository(t *testing.T) *session.Repository {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return session.DefaultRepository()
}

func writeCurrentSession(t *testing.T, repo *session.Repository, projectDir, id, title, model string, updatedAt time.Time, messages []types.Message) {
	t.Helper()
	if err := repo.Save(id, projectDir, messages); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := repo.SaveMeta(id, projectDir, session.SessionMeta{
		Title:     title,
		CreatedAt: updatedAt.Add(-time.Minute),
		UpdatedAt: updatedAt,
		Model:     model,
	}); err != nil {
		t.Fatalf("save session metadata: %v", err)
	}
}

func TestListSessionsEmpty(t *testing.T) {
	currentSessionsRepository(t)

	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessionsUsesCurrentProjectStoresAndOrdersNewestFirst(t *testing.T) {
	repo := currentSessionsRepository(t)
	projectDir := repo.ProjectDirForCWD(filepath.Join(t.TempDir(), "project"))
	base := time.Now().Add(-time.Hour).UTC()
	ids := []string{"aaa111", "bbb222", "ccc333"}
	for index, id := range ids {
		writeCurrentSession(t, repo, projectDir, id, "title "+id, "model-"+id, base.Add(time.Duration(index)*time.Minute), []types.Message{
			types.UserMessage("hello from " + id),
		})
	}

	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != "ccc333" {
		t.Fatalf("expected ccc333 first, got %q", sessions[0].ID)
	}
	if sessions[0].ProjectDir != projectDir || sessions[0].Title != "title ccc333" || sessions[0].Model != "model-ccc333" {
		t.Fatalf("current repository metadata was not projected: %#v", sessions[0])
	}
}

func TestGetSessionProjectsCurrentRepositoryMetadata(t *testing.T) {
	repo := currentSessionsRepository(t)
	projectDir := repo.ProjectDirForCWD(filepath.Join(t.TempDir(), "project"))
	updatedAt := time.Now().UTC().Truncate(time.Second)
	writeCurrentSession(t, repo, projectDir, "mysession01", "Please summarise the budget", "model-current", updatedAt, []types.Message{
		types.UserMessage("What is 2+2?"),
		types.AssistantMessage("4"),
	})

	info, err := GetSession("mysession01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != "mysession01" || info.ProjectDir != projectDir {
		t.Fatalf("unexpected session identity: %#v", info)
	}
	if info.MessageCount != 2 || info.Title != "Please summarise the budget" || info.Model != "model-current" {
		t.Fatalf("unexpected current session metadata: %#v", info)
	}
	if info.CreatedAt.IsZero() || info.UpdatedAt.IsZero() {
		t.Fatalf("current repository timestamps were not projected: %#v", info)
	}
}

func TestGetSessionNotFound(t *testing.T) {
	currentSessionsRepository(t)

	if _, err := GetSession("doesnotexist"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestDeleteSessionDeletesCurrentRepositorySession(t *testing.T) {
	repo := currentSessionsRepository(t)
	projectDir := repo.ProjectDirForCWD(filepath.Join(t.TempDir(), "project"))
	writeCurrentSession(t, repo, projectDir, "todelete", "", "", time.Now().UTC(), []types.Message{
		types.UserMessage("delete me"),
	})

	if err := DeleteSession("todelete"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := GetSession("todelete"); err == nil {
		t.Fatal("deleted session remained discoverable")
	}
}

func TestDeleteSessionNotFound(t *testing.T) {
	currentSessionsRepository(t)

	if err := DeleteSession("ghost-session"); err == nil {
		t.Fatal("expected error for non-existent session")
	}
}

func TestValidateSessionIDRejects(t *testing.T) {
	cases := []string{
		"",
		"../etc/passwd",
		"../sibling",
		"foo/bar",
		"foo\\bar",
		"foo bar",
		"foo.bar",
		".hidden",
		"a!b",
	}
	for _, id := range cases {
		if err := validateSessionID(id); err == nil {
			t.Errorf("validateSessionID(%q) should have returned an error", id)
		}
	}
}

func TestValidateSessionIDAccepts(t *testing.T) {
	cases := []string{
		"abc123",
		"A1b2C3",
		"session-id-1",
		"session_id_2",
		"a",
	}
	for _, id := range cases {
		if err := validateSessionID(id); err != nil {
			t.Errorf("validateSessionID(%q) unexpected error: %v", id, err)
		}
	}
}

func TestGetSessionRejectsPathTraversal(t *testing.T) {
	currentSessionsRepository(t)

	if _, err := GetSession("../../../etc/passwd"); err == nil {
		t.Fatal("expected error for path traversal attempt")
	}
}

func TestDeleteSessionRejectsPathTraversal(t *testing.T) {
	currentSessionsRepository(t)

	if err := DeleteSession("../../../etc/passwd"); err == nil {
		t.Fatal("expected error for path traversal attempt")
	}
}
