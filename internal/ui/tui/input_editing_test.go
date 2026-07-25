package tui

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
	gtui "github.com/grindlemire/go-tui"
)

func TestComposerInheritsTextAreaEditingCommands(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	root.input.SetText("first\nsecond word")
	root.input.SetCursorPosition(len([]rune("first\nsecond")))
	root.input.Focus()

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'a', Mod: gtui.ModCtrl})
	if got, want := root.input.CursorPosition(), len([]rune("first\n")); got != want {
		t.Fatalf("composer Ctrl+A cursor = %d, want %d", got, want)
	}

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'e', Mod: gtui.ModCtrl})
	if got, want := root.input.CursorPosition(), len([]rune("first\nsecond word")); got != want {
		t.Fatalf("composer Ctrl+E cursor = %d, want %d", got, want)
	}

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'w', Mod: gtui.ModCtrl})
	if got, want := root.input.Text(), "first\nsecond "; got != want {
		t.Fatalf("composer Ctrl+W text = %q, want %q", got, want)
	}
}

func TestComposerCtrlPAndCtrlNFollowVerticalEditingBeforeHistory(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	root.input.SetText("first\nsecond")
	root.input.SetCursorPosition(len([]rune("first\nsec")))
	root.input.Focus()

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'p', Mod: gtui.ModCtrl})
	if got := root.input.CursorLine(); got != 0 {
		t.Fatalf("composer Ctrl+P cursor line = %d, want 0", got)
	}

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'n', Mod: gtui.ModCtrl})
	if got := root.input.CursorLine(); got != 1 {
		t.Fatalf("composer Ctrl+N cursor line = %d, want 1", got)
	}
}

func TestComposerSuperASelectsAndReplacesDraft(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	root.input.SetText("draft 界")
	root.input.Focus()

	if handled := root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'a', Mod: gtui.ModSuper}); !handled {
		t.Fatal("composer Super+A was not handled")
	}
	if got := root.input.SelectedText(); got != "draft 界" {
		t.Fatalf("composer selected text = %q, want full draft", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'x'})
	if got := root.input.Text(); got != "x" {
		t.Fatalf("composer replacement = %q, want x", got)
	}
}

func TestComposerPreservesShiftArrowTranscriptBindings(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	root.input.SetText("first\nsecond")
	root.input.Focus()

	for _, event := range []gtui.KeyEvent{
		{Key: gtui.KeyUp, Mod: gtui.ModShift},
		{Key: gtui.KeyDown, Mod: gtui.ModShift},
	} {
		if handled := root.input.HandleEvent(event); handled {
			t.Fatalf("composer consumed product-level shortcut %+v", event)
		}
	}
}

func TestComposerSelectionClearsWhenSessionChanges(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	root.input.SetText("same draft")
	root.input.SelectAll()

	state.SessionID.Set("next-session")
	root.syncSessionViewFromState()
	if got := root.input.SelectedText(); got != "" {
		t.Fatalf("selection crossed session boundary: %q", got)
	}
}

func TestComposerCtrlPUsesHistoryAtInputBoundary(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "previous prompt")

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'p', Mod: gtui.ModCtrl})
	if got := root.input.Text(); got != "previous prompt" {
		t.Fatalf("Ctrl+P history recall = %q, want previous prompt", got)
	}
}

func TestComposerSelectionHasCopyAndCutPaths(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	root.input.SetText("draft")
	root.input.SelectAll()
	var copied string
	root.clipboardWriter = func(_ i18n.Language, text string) error {
		copied = text
		return nil
	}

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'c', Mod: gtui.ModCtrl})
	if copied != "draft" || root.input.Text() != "draft" {
		t.Fatalf("Ctrl+C copied %q with remaining input %q", copied, root.input.Text())
	}

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'x', Mod: gtui.ModSuper})
	if copied != "draft" || root.input.Text() != "" {
		t.Fatalf("Super+X copied %q with remaining input %q", copied, root.input.Text())
	}
}
