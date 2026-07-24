package tui

import (
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	gtui "github.com/grindlemire/go-tui"
)

func TestIdleCtrlCRequiresSecondPressToExit(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	root.now = func() time.Time { return now }
	exits := 0
	root.onExit = func() { exits++ }
	ctrlC := gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'c', Mod: gtui.ModCtrl}

	dispatchRootKeyForTest(t, root, ctrlC)
	if exits != 0 {
		t.Fatal("first idle Ctrl+C exited")
	}
	if got := root.copyFeedback.Get(); got != i18n.Text(state.Language.Get(), i18n.KeyTUIExitConfirm) {
		t.Fatalf("first idle Ctrl+C feedback = %q", got)
	}

	now = now.Add(time.Second)
	dispatchRootKeyForTest(t, root, ctrlC)
	if exits != 1 {
		t.Fatalf("second idle Ctrl+C exits = %d, want 1", exits)
	}
}

func TestWorkingCtrlCCancelsWithoutExiting(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	cancelled := 0
	state.SetQueryCancel(func() { cancelled++ })
	exits := 0
	root.onExit = func() { exits++ }

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'c', Mod: gtui.ModCtrl})
	if cancelled != 1 || exits != 0 {
		t.Fatalf("working Ctrl+C cancelled=%d exits=%d", cancelled, exits)
	}
}

func TestWorkingCtrlCCancelsThroughDecisionOverlay(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	cancelled := 0
	state.SetQueryCancel(func() { cancelled++ })
	state.DecisionReq.Set(&DecisionRequest{DecisionID: "decision-1"})

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'c', Mod: gtui.ModCtrl})
	if cancelled != 1 {
		t.Fatalf("decision-overlay Ctrl+C cancellations = %d, want 1", cancelled)
	}
}

func TestEscapePromotesQueuedInputWhileWorking(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	state.SetQueryCancel(func() {})
	state.QueuedInputCount.Set(1)
	steered := 0
	root.onSteerQueued = func() { steered++ }

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEscape})
	if steered != 1 {
		t.Fatalf("Escape steering calls = %d, want 1", steered)
	}
}
