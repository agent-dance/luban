package tui

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestSyncUpdateOutcomeStopWinsBeforeQueuedCallback(t *testing.T) {
	outcome := newSyncUpdateOutcome()
	if outcome.stopOrWait() {
		t.Fatal("stopOrWait reported a committed update after stop won")
	}

	var calls atomic.Int32
	outcome.run(func() {
		calls.Add(1)
	})
	if got := calls.Load(); got != 0 {
		t.Fatalf("queued callback ran after stop won: calls=%d", got)
	}
}

func TestSyncUpdateOutcomeCallbackWinMakesStopWaitForCompletion(t *testing.T) {
	outcome := newSyncUpdateOutcome()
	started := make(chan struct{})
	release := make(chan struct{})
	go outcome.run(func() {
		close(started)
		<-release
	})
	<-started

	result := make(chan bool, 1)
	go func() {
		result <- outcome.stopOrWait()
	}()
	select {
	case got := <-result:
		t.Fatalf("stopOrWait returned before the callback completed: %v", got)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	select {
	case got := <-result:
		if !got {
			t.Fatal("stopOrWait did not report the completed update")
		}
	case <-time.After(time.Second):
		t.Fatal("stopOrWait did not unblock after callback completion")
	}
}
