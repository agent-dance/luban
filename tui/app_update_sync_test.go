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

func TestAppLifecycleGateSerializesPreRunUpdateAndStop(t *testing.T) {
	gate := newAppLifecycleGate()
	stopCh := make(chan struct{})
	started := make(chan struct{})
	release := make(chan struct{})
	updated := make(chan bool, 1)
	go func() {
		updated <- gate.updateBeforeRun(stopCh, func() {
			close(started)
			<-release
		})
	}()
	<-started

	stopped := make(chan struct{})
	go func() {
		gate.stop(func() { close(stopCh) })
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("stop returned before the pre-run update completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	if ok := <-updated; !ok {
		t.Fatal("update lost after it acquired the lifecycle gate")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop did not acquire the lifecycle gate after update completion")
	}

	var calls atomic.Int32
	if gate.updateBeforeRun(stopCh, func() { calls.Add(1) }) {
		t.Fatal("update succeeded after stop acquired the lifecycle gate")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("post-stop update callback ran: calls=%d", got)
	}
}
