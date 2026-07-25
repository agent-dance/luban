package tui

import (
	"testing"

	gtui "github.com/grindlemire/go-tui"
)

func TestForkPickerDefaultsToLatestAndClamps(t *testing.T) {
	picker := &ForkPickerState{Entries: []ForkEntry{{MessageEnd: 8}, {MessageEnd: 4}}}
	if picker.Selected != 0 || picker.Entries[picker.Selected].MessageEnd != 8 {
		t.Fatalf("default selection = %d, want newest entry", picker.Selected)
	}
	picker.Selected = 20
	picker.clamp()
	if picker.Selected != 1 {
		t.Fatalf("high selection = %d, want 1", picker.Selected)
	}
	picker.Selected = -1
	picker.clamp()
	if picker.Selected != 0 {
		t.Fatalf("low selection = %d, want 0", picker.Selected)
	}
}

func TestForkPickerVisibleRangeKeepsSelectionCentered(t *testing.T) {
	start, end := forkPickerVisibleRange(20, 10, 7)
	if start != 7 || end != 14 {
		t.Fatalf("middle range = %d:%d, want 7:14", start, end)
	}
	start, end = forkPickerVisibleRange(20, 19, 7)
	if start != 13 || end != 20 {
		t.Fatalf("tail range = %d:%d, want 13:20", start, end)
	}
}

func TestForkPickerKeyMapSelectsAndCancelsWithoutInputLeak(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	selectedEnd := 0
	cancelled := false
	state.ForkPicker.Set(&ForkPickerState{
		Visible:  true,
		Entries:  []ForkEntry{{MessageEnd: 8}, {MessageEnd: 4}},
		OnSelect: func(entry ForkEntry) { selectedEnd = entry.MessageEnd },
		OnCancel: func() { cancelled = true },
	})

	if stopped := dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyDown}); !stopped {
		t.Fatal("fork Down did not stop propagation")
	}
	if got := state.ForkPicker.Get().Selected; got != 1 {
		t.Fatalf("selected after Down = %d, want 1", got)
	}
	if stopped := dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'x'}); !stopped {
		t.Fatal("fork picker rune leaked to the input")
	}
	if stopped := dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEnter}); !stopped {
		t.Fatal("fork Enter did not stop propagation")
	}
	if selectedEnd != 4 || state.ForkPicker.Get() != nil {
		t.Fatalf("selected end=%d picker=%+v, want 4 and closed", selectedEnd, state.ForkPicker.Get())
	}

	state.ForkPicker.Set(&ForkPickerState{Visible: true, Entries: []ForkEntry{{MessageEnd: 8}}, OnCancel: func() { cancelled = true }})
	if stopped := dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEscape}); !stopped {
		t.Fatal("fork Escape did not stop propagation")
	}
	if !cancelled || state.ForkPicker.Get() != nil {
		t.Fatalf("cancelled=%t picker=%+v, want cancelled and closed", cancelled, state.ForkPicker.Get())
	}
}
