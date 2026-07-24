package tui

import (
	"testing"
	"time"
)

func TestFinalizeStreamDoesNotDeadlockThroughScrollPersistence(t *testing.T) {
	state := NewAppState()
	state.AppendOrStreamTextForTurn("complete response", 1)
	root := NewRootComponent(state, nil, nil)
	eventQueue := make(chan func(), 32)
	stop := make(chan struct{})
	for _, watcher := range root.Watchers() {
		watcher.Start(eventQueue, stop)
	}
	defer close(stop)

	done := make(chan struct{})
	go func() {
		state.FinalizeStream()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("FinalizeStream deadlocked while Messages watcher persisted scroll state")
	}

	// Execute both sides of the original deadlock chain: Messages triggers
	// scrollY, then scrollY persists the offset back into AppState.
	for i := 0; i < 2; i++ {
		select {
		case fn := <-eventQueue:
			fn()
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for main-loop watcher event %d", i+1)
		}
	}
}
