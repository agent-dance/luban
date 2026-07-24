package tui

import "testing"

func TestNotificationSessionSwitchTOCTOU(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session-a")
	renderer := &TuiRenderer{
		state: state,
		enqueue: func(update func()) bool {
			update()
			return true
		},
	}
	if !renderer.TryInfoForVisibleSession("session-a", "first") {
		t.Fatal("visible-session notification was not committed")
	}
	state.SessionID.Set("session-b")
	if renderer.TryInfoForVisibleSession("session-a", "stale") {
		t.Fatal("stale notification was retargeted after session switch")
	}
	messages := state.Messages.Get()
	if len(messages) != 1 || messages[0].Text != "first" {
		t.Fatalf("notification projection = %+v", messages)
	}
}
