package tui

import "testing"

type modifiedShortcutDispatchRoot struct {
	input   *TextArea
	toggles int
}

func (r *modifiedShortcutDispatchRoot) Render(app *App) *Element {
	root := New(WithDirection(Column), WithWidthPercent(100), WithHeightPercent(100))
	root.AddChild(app.MountPersistent(r, 0, func() Component { return r.input }))
	return root
}

func (r *modifiedShortcutDispatchRoot) KeyMap() KeyMap {
	return KeyMap{
		OnPreemptStop(KeyCtrlG, func(KeyEvent) { r.toggles++ }),
		OnPreemptStop(Rune('g').Alt(), func(KeyEvent) { r.toggles++ }),
	}
}

func newModifiedShortcutDispatchApp(root *modifiedShortcutDispatchRoot) *App {
	app := &App{
		terminal:     NewMockTerminal(80, 24),
		focus:        newFocusManager(),
		buffer:       NewBuffer(80, 24),
		merged:       make(chan Event, 16),
		watcherQueue: make(chan func(), 16),
		stopCh:       make(chan struct{}),
		mounts:       newMountState(),
		batch:        newBatchContext(),
	}
	app.SetRootComponent(root)
	app.Render()
	app.rebuildDispatchTable()
	root.input.Focus()
	return app
}

func TestModifiedToolSegmentShortcutsParseAndDispatchAheadOfFocusedTextArea(t *testing.T) {
	root := &modifiedShortcutDispatchRoot{input: NewTextArea()}
	app := newModifiedShortcutDispatchApp(root)

	inputs := [][]byte{
		{0x07},                // portable Ctrl+G
		[]byte("\x1bg"),       // legacy Alt+G in one read
		[]byte("\x1b[103;3u"), // Kitty Alt+G
	}
	for _, input := range inputs {
		events := parseInput(input)
		if len(events) != 1 {
			t.Fatalf("parseInput(%x) produced %d events", input, len(events))
		}
		if !app.Dispatch(events[0]) {
			t.Fatalf("shortcut event from %x was not consumed", input)
		}
	}
	if root.toggles != len(inputs) {
		t.Fatalf("toggle count=%d, want %d", root.toggles, len(inputs))
	}
	if text := root.input.Text(); text != "" {
		t.Fatalf("shortcut bytes leaked into focused text area: %q", text)
	}
}
