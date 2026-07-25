package swarm

import (
	"context"
	"errors"
	"sync"
	"testing"

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
	messages := persistedMailboxMessages(t, mailbox, "worker")
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
	messages := persistedMailboxMessages(t, mailbox, "worker")
	if len(messages) != count {
		t.Fatalf("messages=%d", len(messages))
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
