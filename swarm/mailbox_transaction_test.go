package swarm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/google/uuid"
)

func TestMailboxSameUUIDIsIdempotentUnderConcurrentDelivery(t *testing.T) {
	mailbox := mailboxWithTempDir(t)
	message := Message{
		ID: NewMessageID(), From: "leader", Text: "one logical task",
		Timestamp: "2026-07-18T00:00:00Z",
	}
	const senders = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, senders)
	for range senders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- mailbox.Send(context.Background(), "worker", message)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("idempotent send: %v", err)
		}
	}
	messages, err := mailbox.Read(context.Background(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != message.ID || messages[0].Sequence != 1 {
		t.Fatalf("mailbox = %#v", messages)
	}
}

func TestMailboxCASRejectsStaleRevisionAndUUIDPayloadConflict(t *testing.T) {
	mailbox := mailboxWithTempDir(t)
	first := Message{ID: NewMessageID(), From: "leader", Text: "first", Timestamp: "2026-07-18T00:00:00Z"}
	stored, err := mailbox.SendCAS(context.Background(), "worker", first, 0)
	if err != nil || stored.Sequence != 1 {
		t.Fatalf("first CAS = %#v, %v", stored, err)
	}
	_, err = mailbox.SendCAS(context.Background(), "worker", Message{
		ID: NewMessageID(), From: "leader", Text: "stale", Timestamp: "2026-07-18T00:00:01Z",
	}, 0)
	if !errors.Is(err, ErrMailboxSequenceConflict) {
		t.Fatalf("stale CAS error = %v", err)
	}
	_, err = mailbox.SendCAS(context.Background(), "worker", Message{
		ID: first.ID, From: "leader", Text: "different", Timestamp: first.Timestamp,
	}, AnyMailboxSequence)
	if !errors.Is(err, ErrMailboxSequenceConflict) {
		t.Fatalf("UUID payload conflict = %v", err)
	}
	// The exact same logical write remains idempotent despite a stale CAS.
	again, err := mailbox.SendCAS(context.Background(), "worker", first, 0)
	if err != nil || again.Sequence != 1 {
		t.Fatalf("idempotent stale retry = %#v, %v", again, err)
	}
}

func TestMailboxAcknowledgeIsIdempotentAndDoesNotConsumeLaterMessage(t *testing.T) {
	mailbox := mailboxWithTempDir(t)
	first, err := mailbox.SendCAS(context.Background(), "worker", Message{
		ID: NewMessageID(), From: "leader", Text: "first", Timestamp: "2026-07-18T00:00:00Z",
	}, AnyMailboxSequence)
	if err != nil {
		t.Fatal(err)
	}
	if err := mailbox.Send(context.Background(), "worker", Message{
		ID: NewMessageID(), From: "leader", Text: "later", Timestamp: "2026-07-18T00:00:01Z",
	}); err != nil {
		t.Fatal(err)
	}
	receipt := MessageReceipt{ID: first.ID, Sequence: first.Sequence}
	if err := mailbox.Acknowledge(context.Background(), "worker", []MessageReceipt{receipt}); err != nil {
		t.Fatal(err)
	}
	if err := mailbox.Acknowledge(context.Background(), "worker", []MessageReceipt{receipt}); err != nil {
		t.Fatalf("duplicate acknowledge: %v", err)
	}
	messages, err := mailbox.Read(context.Background(), "worker")
	if err != nil || len(messages) != 2 || !messages[0].Read || messages[1].Read {
		t.Fatalf("messages=%#v err=%v", messages, err)
	}
	if err := mailbox.Acknowledge(context.Background(), "worker", []MessageReceipt{{ID: first.ID, Sequence: first.Sequence + 1}}); !errors.Is(err, ErrMailboxSequenceConflict) {
		t.Fatalf("stale receipt error=%v", err)
	}
}

func TestMailboxConcurrentMessagesReceiveUniqueMonotonicSequences(t *testing.T) {
	mailbox := mailboxWithTempDir(t)
	const count = 96
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for index := range count {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- mailbox.Send(context.Background(), "worker", Message{
				ID: NewMessageID(), From: "leader", Text: string(rune('a' + index%26)),
				Timestamp: "2026-07-18T00:00:00Z",
			})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	messages, err := mailbox.Read(context.Background(), "worker")
	if err != nil || len(messages) != count {
		t.Fatalf("messages=%d err=%v", len(messages), err)
	}
	seenIDs := make(map[string]struct{}, count)
	for index, message := range messages {
		if _, err := uuid.Parse(message.ID); err != nil {
			t.Fatalf("message %d id %q: %v", index, message.ID, err)
		}
		if _, exists := seenIDs[message.ID]; exists {
			t.Fatalf("duplicate UUID %s", message.ID)
		}
		seenIDs[message.ID] = struct{}{}
		if want := uint64(index + 1); message.Sequence != want {
			t.Fatalf("message %d sequence=%d want=%d", index, message.Sequence, want)
		}
	}
}

func TestMailboxLegacyMigrationDeduplicatesCrashReplayAndPreservesRepeats(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacyPath := filepath.Join(home, brand.ConfigDirName, "teams", "migration", "worker", "inbox.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []Message{
		{From: "old", Text: "same", Timestamp: "2026-07-18T00:00:00Z"},
		{From: "old", Text: "same", Timestamp: "2026-07-18T00:00:00Z"},
	}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(legacyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	mailbox, err := NewMailbox("migration")
	if err != nil {
		t.Fatal(err)
	}
	first, err := mailbox.Read(context.Background(), "worker")
	if err != nil || len(first) != 2 || first[0].ID == first[1].ID {
		t.Fatalf("first migration = %#v err=%v", first, err)
	}
	// Simulate a crash after canonical publish but before the legacy writer is
	// retired. Re-reading must merge by deterministic UUID without duplication.
	second, err := mailbox.Read(context.Background(), "worker")
	if err != nil || len(second) != 2 || second[0].ID != first[0].ID || second[1].ID != first[1].ID {
		t.Fatalf("crash replay = %#v err=%v", second, err)
	}
	// Simulate an old writer appending another intentionally identical record.
	legacy = append(legacy, legacy[0])
	data, _ = json.Marshal(legacy)
	if err := os.WriteFile(legacyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := mailbox.Read(context.Background(), "worker")
	if err != nil || len(third) != 3 {
		t.Fatalf("intentional repeat = %#v err=%v", third, err)
	}
	for index, message := range third {
		if message.Sequence != uint64(index+1) {
			t.Fatalf("sequence[%d]=%d", index, message.Sequence)
		}
	}
}
