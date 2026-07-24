package main

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestAsyncGateRejectsWorkAfterCloseAndWaitsForAcceptedWork(t *testing.T) {
	gate := newAsyncGate()
	started := make(chan struct{})
	release := make(chan struct{})
	if !gate.Go(func() {
		close(started)
		<-release
	}) {
		t.Fatal("gate rejected work before close")
	}
	<-started

	gate.Close()
	var lateCalls atomic.Int32
	if gate.Go(func() { lateCalls.Add(1) }) {
		t.Fatal("gate accepted work after close")
	}

	waited := make(chan struct{})
	go func() {
		gate.Wait()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("Wait returned before accepted work completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after accepted work completed")
	}
	if got := lateCalls.Load(); got != 0 {
		t.Fatalf("late work ran after close: calls=%d", got)
	}
}
