package engine

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestCoreEngineCancelledCompactDoesNotCommitSummaryWhenProviderClosesEmpty(t *testing.T) {
	const sessionID = "compact-cancel-empty-race"
	sessions := newMemorySessionManager()
	seedCompactableSession(t, sessions, sessionID)
	before, err := sessions.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessions.mu.Lock()
	savesBefore := sessions.saveCalls
	sessions.mu.Unlock()

	started, release := make(chan struct{}), make(chan struct{})
	p := &mockProvider{name: "race-provider", modelID: "race-model"}
	p.defaultFn = func(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
		ch := make(chan types.StreamEvent)
		close(started)
		go func() {
			<-release
			close(ch)
		}()
		return ch, nil
	}
	eng, err := New(Config{Provider: p, Sessions: sessions, MaxContextTokens: 100_000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var eventsMu sync.Mutex
	var events []stream.Event
	done := make(chan error, 1)
	go func() {
		_, compactErr := eng.CompactWithEvents(ctx, sessionID, "", func(event stream.Event) {
			eventsMu.Lock()
			events = append(events, event)
			eventsMu.Unlock()
		})
		done <- compactErr
	}()
	<-started
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("CompactWithEvents error = %v, want context.Canceled", err)
	}

	after, err := sessions.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessions.mu.Lock()
	savesAfter := sessions.saveCalls
	sessions.mu.Unlock()
	if !reflect.DeepEqual(after, before) || savesAfter != savesBefore {
		t.Fatalf("cancelled compact committed history: saves %d→%d before=%+v after=%+v", savesBefore, savesAfter, before, after)
	}

	eventsMu.Lock()
	observed := append([]stream.Event(nil), events...)
	eventsMu.Unlock()
	var cancelled, failed, boundaries int
	for _, event := range observed {
		if event.Type == stream.EventCompactBoundary {
			boundaries++
		}
		if event.Progress == nil {
			continue
		}
		switch event.Progress.Stage {
		case "compact_cancelled":
			cancelled++
		case "compact_failed":
			failed++
		}
	}
	if cancelled != 1 || failed != 0 || boundaries != 0 {
		t.Fatalf("cancelled compact lifecycle cancelled=%d failed=%d boundaries=%d events=%+v", cancelled, failed, boundaries, observed)
	}
}
