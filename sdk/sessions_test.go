package sdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// overrideSessionsDir redirects the sessions dir to a temp directory for the
// duration of a test, then restores the original override on cleanup.
func overrideSessionsDir(t *testing.T, dir string) {
	t.Helper()
	prev := sessionsDirOverride
	sessionsDirOverride = func() (string, error) { return dir, nil }
	t.Cleanup(func() { sessionsDirOverride = prev })
}

// writeSession writes a minimal JSONL session file to dir/id.jsonl.
func writeSession(t *testing.T, dir, id string, messages []map[string]any) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(filepath.Join(dir, id+".jsonl"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, m := range messages {
		if err := enc.Encode(m); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
}

// ─── ListSessions ─────────────────────────────────────────────────────────────

func TestListSessions_Empty(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessions_NonExistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-such-dir")
	overrideSessionsDir(t, dir)

	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("should return nil,nil for missing dir: %v", err)
	}
	if sessions != nil {
		t.Fatalf("expected nil slice, got %v", sessions)
	}
}

func TestListSessions_Multiple(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	ids := []string{"aaa111", "bbb222", "ccc333"}
	for _, id := range ids {
		writeSession(t, dir, id, []map[string]any{
			{"role": "user", "content": "hello from " + id},
		})
		// Stagger mtime so ordering is deterministic.
		time.Sleep(2 * time.Millisecond)
	}

	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	// Newest-first: last written should be first.
	if sessions[0].ID != "ccc333" {
		t.Errorf("expected ccc333 first, got %q", sessions[0].ID)
	}
}

func TestListSessions_IgnoresNonJSONL(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	writeSession(t, dir, "valid-session", nil)
	// Create files that should be ignored.
	for _, name := range []string{"README.md", ".hidden", "data.json", "dir"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte("noise"), 0o600)
	}
	_ = os.Mkdir(filepath.Join(dir, "subdir"), 0o700)

	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected exactly 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "valid-session" {
		t.Errorf("unexpected session ID %q", sessions[0].ID)
	}
}

// ─── GetSession ───────────────────────────────────────────────────────────────

func TestGetSession_Success(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	msgs := []map[string]any{
		{"role": "user", "content": "What is 2+2?"},
		{"role": "assistant", "content": "4"},
	}
	writeSession(t, dir, "mysession01", msgs)

	info, err := GetSession("mysession01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ID != "mysession01" {
		t.Errorf("ID mismatch: got %q", info.ID)
	}
	if info.MessageCount != 2 {
		t.Errorf("expected MessageCount 2, got %d", info.MessageCount)
	}
	if info.Title == "" {
		t.Error("expected non-empty title")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	_, err := GetSession("doesnotexist")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestGetSession_TitleFromFirstUserMessage(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	msgs := []map[string]any{
		{"role": "assistant", "content": "hi"},
		{"role": "user", "content": "Please summarise the budget"},
	}
	writeSession(t, dir, "titled-session", msgs)

	info, err := GetSession("titled-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Title != "Please summarise the budget" {
		t.Errorf("unexpected title: %q", info.Title)
	}
}

// ─── DeleteSession ────────────────────────────────────────────────────────────

func TestDeleteSession_Success(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	writeSession(t, dir, "todelete", nil)

	if err := DeleteSession("todelete"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "todelete.jsonl")); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestDeleteSession_NotFound(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	err := DeleteSession("ghost-session")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
}

// ─── Path traversal prevention ────────────────────────────────────────────────

func TestValidateSessionID_Rejects(t *testing.T) {
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

func TestValidateSessionID_Accepts(t *testing.T) {
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

func TestGetSession_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	_, err := GetSession("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal attempt")
	}
}

func TestDeleteSession_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	overrideSessionsDir(t, dir)

	err := DeleteSession("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal attempt")
	}
}
