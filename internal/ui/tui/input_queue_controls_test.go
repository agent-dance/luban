package tui

import (
	"strings"
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
	root.input.Focus()
	state.SetQueryCancel(func() {})
	state.QueuedInputTexts.Set([]string{"queued"})
	steered := 0
	root.onSteerQueued = func() { steered++ }

	escape := gtui.KeyEvent{Key: gtui.KeyEscape}
	if handled := root.input.HandleEvent(escape); handled {
		t.Fatal("focused composer consumed Escape before queued-input steering")
	}
	dispatchRootKeyForTest(t, root, escape)
	if steered != 1 {
		t.Fatalf("Escape steering calls = %d, want 1", steered)
	}
}

func TestQueuedInputsRenderAboveStatusOnePerLineAndTruncate(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.QueuedInputTexts.Set([]string{"first queued message", strings.Repeat("界", 30)})
	root := NewRootComponent(state, nil, nil)

	const width = 24
	rendered := renderElementText(root.renderAtSize(nil, width, 16), width, 16)
	lines := strings.Split(rendered, "\n")
	firstRow, secondRow, statusRow := -1, -1, -1
	for index, line := range lines {
		switch {
		case strings.Contains(line, "first queued message"):
			firstRow = index
		case strings.Contains(line, "界"):
			secondRow = index
		case strings.Contains(line, "Auto mode"):
			statusRow = index
		}
	}
	if firstRow < 0 || secondRow != firstRow+1 || statusRow != secondRow+1 {
		t.Fatalf("queued rows must appear in FIFO order immediately above status: first=%d second=%d status=%d\n%s", firstRow, secondRow, statusRow, rendered)
	}
	if !strings.Contains(lines[secondRow], "…") {
		t.Fatalf("long queued preview was not truncated with an ellipsis: %q", lines[secondRow])
	}
}

func TestQueuedInputPreviewSanitizesControlsToOneLine(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	rendered := collectElementText(root.renderQueuedInputs([]string{"first\nsecond\t\x1b[31m"}))
	if rendered != "› first second [31m" {
		t.Fatalf("queued input preview = %q, want one sanitized line", rendered)
	}
}
