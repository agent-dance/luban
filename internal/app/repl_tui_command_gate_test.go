package app

import (
	"sync"
	"testing"
	"time"
)

func TestTUICommandLockSerializesSessionSnapshotAndPickerTransition(t *testing.T) {
	gate := &sync.Mutex{}
	sessionID := "origin"
	cfg := TUIREPLConfig{SessionID: &sessionID, CommandMu: gate}
	commandStarted := make(chan struct{})
	releaseCommand := make(chan struct{})
	commandDone := make(chan string, 1)
	go withTUICommandLock(cfg, func() {
		snapshot := *cfg.SessionID
		close(commandStarted)
		<-releaseCommand
		commandDone <- snapshot + ":" + *cfg.SessionID
	})
	<-commandStarted

	transitionDone := make(chan struct{})
	go withTUICommandLock(cfg, func() {
		*cfg.SessionID = "target"
		close(transitionDone)
	})
	select {
	case <-transitionDone:
		t.Fatal("picker transition crossed a running command snapshot")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseCommand)
	if got := <-commandDone; got != "origin:origin" {
		t.Fatalf("command observed a mixed session snapshot: %q", got)
	}
	select {
	case <-transitionDone:
	case <-time.After(time.Second):
		t.Fatal("picker transition did not proceed after command completion")
	}
	if got := sessionID; got != "target" {
		t.Fatalf("session ID = %q, want target", got)
	}
}
