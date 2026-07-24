package tui

import (
	"testing"

	gtui "github.com/grindlemire/go-tui"
)

func TestSubmitWhileQueryRunningKeepsDraft(t *testing.T) {
	state := NewAppState()
	root := NewRootComponentWithAdmission(state, func(string) bool { return false }, nil)
	root.input.SetText("unfinished draft")
	root.input.SetCursorPosition(5)
	root.input.SelectAll()
	root.input.Focus()

	if handled := root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyEnter}); !handled {
		t.Fatal("Enter was not handled")
	}
	if got := root.input.Text(); got != "unfinished draft" {
		t.Fatalf("rejected submission cleared draft: %q", got)
	}
	if got := root.input.SelectedText(); got != "unfinished draft" {
		t.Fatalf("rejected submission changed selection: %q", got)
	}
	if !root.input.IsFocused() {
		t.Fatal("rejected submission changed composer focus")
	}
}

func TestCtrlCDuringAdmissionCancelsReservedQuery(t *testing.T) {
	state := NewAppState()
	cancelled := make(chan struct{})
	generation, ok := state.TryReserveQuery(func() { close(cancelled) })
	if !ok || generation == 0 {
		t.Fatal("failed to reserve initial query")
	}
	if _, admitted := state.TryReserveQuery(func() {}); admitted {
		t.Fatal("second query was admitted while reservation was active")
	}
	if !state.TryCancelQuery() {
		t.Fatal("Ctrl+C did not cancel the admission-time reservation")
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("reservation cancel function was not invoked")
	}
	if _, admitted := state.TryReserveQuery(func() {}); admitted {
		t.Fatal("cancelled query admitted a successor before terminal cleanup")
	}
	state.ClearQueryCancel(generation)
	if next, admitted := state.TryReserveQuery(func() {}); !admitted || next == generation {
		t.Fatalf("query slot was not released after cleanup: generation=%d admitted=%v", next, admitted)
	}
}
