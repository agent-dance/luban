package tui

import (
	"errors"
	"testing"
)

func TestANSITerminalRetainsRawSnapshotWhenRestoreFails(t *testing.T) {
	wantErr := errors.New("restore failed")
	state := new(rawModeState)
	terminal := NewANSITerminalWithCaps(nil, nil, Capabilities{})
	terminal.rawState = state
	terminal.disableRawMode = func(got *rawModeState) error {
		if got != state {
			t.Fatalf("restored state = %p, want %p", got, state)
		}
		return wantErr
	}

	if err := terminal.ExitRawMode(); !errors.Is(err, wantErr) {
		t.Fatalf("ExitRawMode() error = %v, want %v", err, wantErr)
	}
	if terminal.rawState != state {
		t.Fatal("failed raw-mode restore discarded the only recovery snapshot")
	}
	terminal.enableRawMode = func(int) (*rawModeState, error) {
		t.Fatal("EnterRawMode replaced a retained recovery snapshot")
		return nil, nil
	}
	if err := terminal.EnterRawMode(); err != nil {
		t.Fatalf("EnterRawMode with retained snapshot: %v", err)
	}
	if terminal.rawState != state {
		t.Fatal("EnterRawMode replaced the retained recovery snapshot")
	}
	terminal.disableRawMode = func(*rawModeState) error { return nil }
	if err := terminal.ExitRawMode(); err != nil {
		t.Fatalf("retry ExitRawMode(): %v", err)
	}
	if terminal.rawState != nil {
		t.Fatal("successful raw-mode restore retained a stale snapshot")
	}
}
