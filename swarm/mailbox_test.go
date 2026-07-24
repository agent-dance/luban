package swarm

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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

// ---- Send / Read round-trip ----

func TestSendRead_SingleMessage(t *testing.T) {
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

	got, err := m.Read(context.Background(), "agent1")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
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

func TestSendRead_MultipleMessages(t *testing.T) {
	m := mailboxWithTempDir(t)

	for i := 0; i < 5; i++ {
		msg := Message{From: "leader", Text: "task", Timestamp: "2026-01-01T00:00:00Z"}
		if err := m.Send(context.Background(), "agent2", msg); err != nil {
			t.Fatalf("Send #%d: %v", i, err)
		}
	}

	got, err := m.Read(context.Background(), "agent2")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("expected 5 messages, got %d", len(got))
	}
}

func TestRead_PreservesInboxAndReadFlags(t *testing.T) {
	m := mailboxWithTempDir(t)

	_ = m.Send(context.Background(), "agent3", Message{From: "leader", Text: "msg", Timestamp: "2026-01-01T00:00:00Z"})

	first, err := m.Read(context.Background(), "agent3")
	if err != nil || len(first) != 1 {
		t.Fatalf("first read: %v msgs, err %v", len(first), err)
	}

	second, err := m.Read(context.Background(), "agent3")
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if len(second) != 1 || second[0].Read {
		t.Errorf("expected non-destructive unread inbox after read, got %#v", second)
	}
	if err := m.MarkAllRead(context.Background(), "agent3"); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	third, err := m.Read(context.Background(), "agent3")
	if err != nil || len(third) != 1 || !third[0].Read {
		t.Fatalf("read audit history after MarkAllRead: %#v err=%v", third, err)
	}
}

func TestRead_EmptyInbox(t *testing.T) {
	m := mailboxWithTempDir(t)

	msgs, err := m.Read(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Read on nonexistent: %v", err)
	}
	if msgs != nil && len(msgs) != 0 {
		t.Errorf("expected nil/empty slice, got %v", msgs)
	}
}

// ---- Timestamp auto-fill ----

func TestSend_AutoFillsTimestamp(t *testing.T) {
	m := mailboxWithTempDir(t)

	_ = m.Send(context.Background(), "agentT", Message{From: "leader", Text: "hi"})

	msgs, err := m.Read(context.Background(), "agentT")
	if err != nil || len(msgs) == 0 {
		t.Fatalf("read: %v msgs err=%v", len(msgs), err)
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

// ---- Poll ----

func TestPoll_ReceivesMessage(t *testing.T) {
	m := mailboxWithTempDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Send a message slightly after Poll starts.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)
		_ = m.Send(context.Background(), "pollAgent", Message{
			From:      "leader",
			Text:      "work on this",
			Timestamp: "2026-01-01T00:00:00Z",
		})
	}()

	msg, err := m.Poll(ctx, "pollAgent")
	wg.Wait()

	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if msg.Text != "work on this" {
		t.Errorf("Poll text: got %q, want %q", msg.Text, "work on this")
	}
}

func TestPoll_CancelledContext(t *testing.T) {
	m := mailboxWithTempDir(t)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately.
	cancel()

	_, err := m.Poll(ctx, "noAgent")
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestPoll_LeavesRemainingMessages(t *testing.T) {
	m := mailboxWithTempDir(t)

	// Pre-populate inbox with 3 messages.
	for i := 0; i < 3; i++ {
		_ = m.Send(context.Background(), "multiAgent", Message{From: "leader", Text: "msg", Timestamp: "2026-01-01T00:00:00Z"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Poll uses PopFirst — returns first message; remaining two stay in inbox.
	_, err := m.Poll(ctx, "multiAgent")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}

	// PopFirst marks the first message read and preserves all audit entries.
	remaining, err := m.Read(context.Background(), "multiAgent")
	if err != nil {
		t.Fatalf("Read after poll: %v", err)
	}
	if len(remaining) != 3 || !remaining[0].Read || remaining[1].Read || remaining[2].Read {
		t.Errorf("expected one read and two unread audit messages, got %#v", remaining)
	}
}
