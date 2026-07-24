package tui

import (
	"testing"

	gtui "github.com/grindlemire/go-tui"
)

func toolSegmentShortcutBinding(t *testing.T, root *RootComponent, event gtui.KeyEvent) gtui.KeyBinding {
	t.Helper()
	for _, binding := range root.KeyMap() {
		if slashAwareKeyMatches(binding.Pattern, event) {
			return binding
		}
	}
	t.Fatalf("tool-segment shortcut has no binding for %+v", event)
	return gtui.KeyBinding{}
}

func TestToolSegmentShortcutsPreemptFocusedComposer(t *testing.T) {
	state := NewAppState()
	messages := []Message{
		segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("Inspection complete."),
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	root.input.Focus()
	segment := requireSegmentItem(t, BuildTranscriptToolSegments(messages)[0])

	events := []gtui.KeyEvent{
		{Key: gtui.KeyRune, Rune: 'g', Mod: gtui.ModCtrl},
		{Key: gtui.KeyRune, Rune: 'g', Mod: gtui.ModAlt},
	}
	for index, event := range events {
		for _, inputBinding := range root.input.KeyMap() {
			if slashAwareKeyMatches(inputBinding.Pattern, event) {
				t.Fatalf("focused composer captured tool-segment shortcut %+v", event)
			}
		}

		binding := toolSegmentShortcutBinding(t, root, event)
		if !binding.Preempt || !binding.Stop {
			t.Fatalf("shortcut %+v is not preemptive stop: %+v", event, binding)
		}
		binding.Handler(event)
		wantExpanded := index == 0
		if got := state.ToolSegmentExpanded(segment.ID); got != wantExpanded {
			t.Fatalf("shortcut %+v expanded=%v, want %v", event, got, wantExpanded)
		}
	}
	if text := root.input.Text(); text != "" {
		t.Fatalf("tool-segment shortcuts leaked into composer: %q", text)
	}
}

func TestToolSegmentHeaderWholeRowUsesMountedScreenCoordinates(t *testing.T) {
	state := NewAppState()
	messages := []Message{
		segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("Inspection complete."),
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	frame := root.renderAtSize(nil, 80, 24)
	frame.Render(gtui.NewBuffer(80, 24), 80, 24)

	segment := requireSegmentItem(t, BuildTranscriptToolSegments(messages)[0])
	header := root.segmentRefs.Get(segment.ID)
	viewport := root.contentRef.El()
	if header == nil || viewport == nil {
		t.Fatalf("missing mounted interaction refs: header=%v viewport=%v", header, viewport)
	}
	sx, sy := viewport.ScrollOffset()
	visible := header.Rect().Translate(viewport.ContentRect().X-sx, viewport.ContentRect().Y-sy).Intersect(viewport.ContentRect())
	if visible.Width < 10 || visible.Height != 1 {
		t.Fatalf("header visible screen rect = %+v, want a full row", visible)
	}
	// Click near the far right of the header to prove the entire row, not only
	// the disclosure glyph, is an interaction target.
	x, y := visible.Right()-2, visible.Y
	if !header.ContainsPoint(x, y) {
		t.Fatalf("header missed its rendered screen point (%d,%d); content=%+v viewport=%+v", x, y, header.Rect(), viewport.ContentRect())
	}
	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseLeft, Action: gtui.MousePress, X: x, Y: y}) {
		t.Fatal("mounted header row click was not consumed")
	}
	if !state.ToolSegmentExpanded(segment.ID) {
		t.Fatal("mounted header row click did not expand the segment")
	}
}
