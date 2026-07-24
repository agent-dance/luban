package tui

import (
	"errors"
	"sync"
	"testing"
)

type failingTerminalControlSink struct{}

func (failingTerminalControlSink) WriteTerminalControl([]byte) error {
	return errors.New("writer failed")
}

func TestTerminalControlObserverClassifiesRejectedWrites(t *testing.T) {
	var mu sync.Mutex
	var got []TerminalControlRejection
	releaseObserver := InstallTerminalControlObserver(func(reason TerminalControlRejection) {
		mu.Lock()
		got = append(got, reason)
		mu.Unlock()
	})
	defer releaseObserver()

	// Empty writes are intentional no-ops, not rogue attempts.
	if err := WriteTerminalControl(nil); err != nil {
		t.Fatalf("empty write: %v", err)
	}
	if err := WriteTerminalControl([]byte("no-owner")); !errors.Is(err, ErrNoTerminalControlOwner) {
		t.Fatalf("no-owner error = %v", err)
	}

	releaseFailing := InstallTerminalControlSink(failingTerminalControlSink{})
	if err := WriteTerminalControl([]byte("failing-owner")); err == nil {
		t.Fatal("failing owner unexpectedly succeeded")
	}
	releaseFailing()

	app := &App{}
	if err := app.WriteTerminalControl([]byte("unavailable-owner")); !errors.Is(err, ErrTerminalControlUnavailable) {
		t.Fatalf("unavailable owner error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []TerminalControlRejection{
		TerminalControlNoOwner,
		TerminalControlWriteError,
		TerminalControlUnavailable,
	}
	if len(got) != len(want) {
		t.Fatalf("rejections = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rejections = %v, want %v", got, want)
		}
	}
}

func TestTerminalControlObserverPanicDoesNotBreakOwnerFailure(t *testing.T) {
	releasePanic := InstallTerminalControlObserver(func(TerminalControlRejection) { panic("observer panic") })
	defer releasePanic()
	called := false
	releaseHealthy := InstallTerminalControlObserver(func(TerminalControlRejection) { called = true })
	defer releaseHealthy()
	if err := WriteTerminalControl([]byte("no-owner")); !errors.Is(err, ErrNoTerminalControlOwner) {
		t.Fatalf("error = %v, want owner failure despite observer panic", err)
	}
	if !called {
		t.Fatal("panicking observer suppressed another subscriber")
	}
}
