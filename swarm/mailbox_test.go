package swarm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mailboxWithTempDir creates a Mailbox backed by a temp directory via NewMailbox.
func mailboxWithTempDir(t *testing.T) *Mailbox {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	m, err := NewMailbox("test-team")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func persistedMailboxMessages(t *testing.T, mailbox *Mailbox, agentName string) []Message {
	t.Helper()
	path, err := mailbox.inboxPath(agentName)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := readMailboxMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	return messages
}

// ---- Send persistence ----

func TestSend_PersistsSingleMessage(t *testing.T) {
	m := mailboxWithTempDir(t)
	msg := Message{
		From:      "leader",
		Text:      "hello teammate",
		Timestamp: "2026-01-01T00:00:00Z",
		Color:     "blue",
	}

	if err := m.Send(context.Background(), "agent1", msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := persistedMailboxMessages(t, m, "agent1")
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if got[0].Text != msg.Text {
		t.Errorf("text: got %q, want %q", got[0].Text, msg.Text)
	}
	if got[0].From != msg.From {
		t.Errorf("from: got %q, want %q", got[0].From, msg.From)
	}
	if got[0].Color != msg.Color {
		t.Errorf("color: got %q, want %q", got[0].Color, msg.Color)
	}
}

func TestSend_PersistsMultipleMessages(t *testing.T) {
	m := mailboxWithTempDir(t)

	for i := 0; i < 5; i++ {
		msg := Message{From: "leader", Text: "task", Timestamp: "2026-01-01T00:00:00Z"}
		if err := m.Send(context.Background(), "agent2", msg); err != nil {
			t.Fatalf("Send #%d: %v", i, err)
		}
	}

	got := persistedMailboxMessages(t, m, "agent2")
	if len(got) != 5 {
		t.Errorf("expected 5 messages, got %d", len(got))
	}
}

// ---- Timestamp auto-fill ----

func TestSend_AutoFillsTimestamp(t *testing.T) {
	m := mailboxWithTempDir(t)

	_ = m.Send(context.Background(), "agentT", Message{From: "leader", Text: "hi"})

	msgs := persistedMailboxMessages(t, m, "agentT")
	if len(msgs) == 0 {
		t.Fatal("expected a persisted message")
	}
	if msgs[0].Timestamp == "" {
		t.Error("expected Timestamp to be auto-filled")
	}
}

// ---- Inbox path ----

func TestInboxPath(t *testing.T) {
	m := &Mailbox{baseDir: "/tmp/base"}
	got, err := m.inboxPath("alice")
	if err != nil {
		t.Fatalf("inboxPath: %v", err)
	}
	want := filepath.Join("/tmp/base", "inboxes", "alice.json")
	if got != want {
		t.Errorf("inboxPath: got %q, want %q", got, want)
	}
}

func TestInboxPath_RejectsTraversal(t *testing.T) {
	m := &Mailbox{baseDir: "/tmp/base"}

	for _, bad := range []string{"../etc", "foo/bar", "..", ".", ""} {
		_, err := m.inboxPath(bad)
		if err == nil {
			t.Errorf("inboxPath(%q): expected error, got nil", bad)
		}
	}
}

func TestValidateName(t *testing.T) {
	for _, good := range []string{"agent1", "my-agent", "agent_2", "A"} {
		if err := validateName(good, "test"); err != nil {
			t.Errorf("validateName(%q): unexpected error: %v", good, err)
		}
	}
	for _, bad := range []string{"", "../x", "a/b", ".hidden", "-start", "_start"} {
		if err := validateName(bad, "test"); err == nil {
			t.Errorf("validateName(%q): expected error, got nil", bad)
		}
	}
}

// ---- Atomic write ----

func TestAtomicWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	if err := atomicWrite(path, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("content mismatch: %s", data)
	}
}

func TestAtomicWrite_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	_ = atomicWrite(path, []byte(`old`))
	_ = atomicWrite(path, []byte(`new`))

	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("expected overwrite, got %q", data)
	}
}
