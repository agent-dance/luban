package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	gtui "github.com/grindlemire/go-tui"
)

func TestRootMouseDoesNotConsumeTranscriptDrag(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	events := []gtui.MouseEvent{
		{Button: gtui.MouseLeft, Action: gtui.MousePress, X: 12, Y: 4},
		{Button: gtui.MouseLeft, Action: gtui.MouseDrag, X: 24, Y: 6},
		{Button: gtui.MouseLeft, Action: gtui.MouseRelease, X: 24, Y: 6},
	}
	for _, event := range events {
		if root.HandleMouse(event) {
			t.Fatalf("transcript mouse event was consumed: %+v", event)
		}
	}
}

func TestRootMouseShowsNativeSelectionHintForUnmodifiedTranscriptDrag(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("LC_TERMINAL", "")
	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	state.Messages.Set([]Message{{Kind: MsgAssistant, Text: "message"}})
	root := NewRootComponent(state, nil, nil)
	area := root.renderMessageArea(5)
	area.Render(gtui.NewBuffer(80, 5), 80, 5)

	event := gtui.MouseEvent{Button: gtui.MouseLeft, Action: gtui.MouseDrag, X: 4, Y: 2}
	if root.HandleMouse(event) {
		t.Fatal("transcript drag was consumed while showing the selection hint")
	}
	if !root.transcriptSelectionHintVisible.Get() {
		t.Fatal("unmodified transcript drag did not show the native-selection hint")
	}
	status := collectElementText(root.renderStatusBar(40))
	want := i18n.Text(i18n.LangZH, i18n.KeyTranscriptSelectionHintOption)
	if !strings.Contains(status, want) {
		t.Fatalf("selection hint status = %q, want %q", status, want)
	}
}

func TestRootMouseModifiedTranscriptDragDoesNotShowSelectionHint(t *testing.T) {
	for _, modifier := range []gtui.Modifier{gtui.ModShift, gtui.ModAlt} {
		state := NewAppState()
		state.Messages.Set([]Message{{Kind: MsgAssistant, Text: "message"}})
		root := NewRootComponent(state, nil, nil)
		area := root.renderMessageArea(5)
		area.Render(gtui.NewBuffer(80, 5), 80, 5)

		event := gtui.MouseEvent{Button: gtui.MouseLeft, Action: gtui.MouseDrag, X: 4, Y: 2, Mod: modifier}
		if root.HandleMouse(event) {
			t.Fatalf("modified transcript drag was consumed: %+v", event)
		}
		if root.transcriptSelectionHintVisible.Get() {
			t.Fatalf("modified transcript drag showed the selection hint: %+v", event)
		}
	}
}

func TestNativeSelectionHintUsesGenericCopyOutsideITerm(t *testing.T) {
	if got := nativeSelectionHintKey("vscode", ""); got != i18n.KeyTranscriptSelectionHintGeneric {
		t.Fatalf("native selection hint key = %q, want generic", got)
	}
	if got := nativeSelectionHintKey("", "iTerm2"); got != i18n.KeyTranscriptSelectionHintOption {
		t.Fatalf("native selection hint key = %q, want Option", got)
	}
}

func TestRootMouseModifierClickDoesNotToggleToolSegment(t *testing.T) {
	state := NewAppState()
	messages := []Message{
		segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	area := root.renderMessageArea(10)
	area.Render(gtui.NewBuffer(80, 10), 80, 10)

	segment := requireSegmentItem(t, BuildTranscriptToolSegments(messages)[0])
	header := root.segmentRefs.Get(segment.ID)
	if header == nil || header.Rect().IsEmpty() {
		t.Fatalf("segment header ref was not laid out: %#v", header)
	}
	for _, modifier := range []gtui.Modifier{gtui.ModShift, gtui.ModAlt} {
		event := gtui.MouseEvent{
			Button: gtui.MouseLeft,
			Action: gtui.MousePress,
			X:      header.Rect().X,
			Y:      header.Rect().Y,
			Mod:    modifier,
		}
		if root.HandleMouse(event) {
			t.Fatalf("modified segment click was consumed: %+v", event)
		}
		if state.ToolSegmentExpanded(segment.ID) {
			t.Fatalf("modified segment click toggled %q", segment.ID)
		}
	}
}

func TestRootMouseWheelStillScrollsTranscript(t *testing.T) {
	state := NewAppState()
	messages := make([]Message, 80)
	for i := range messages {
		messages[i] = Message{Kind: MsgAssistant, Text: "message"}
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	root.stickToBottom.Set(false)
	area := root.renderMessageArea(5)
	area.Render(gtui.NewBuffer(80, 5), 80, 5)

	_, maxY := root.contentRef.El().MaxScroll()
	if maxY < 6 {
		t.Fatalf("test transcript max scroll = %d, want at least 6", maxY)
	}
	start := maxY / 2
	area.ScrollTo(0, start)
	root.scrollY.Set(start)

	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseWheelDown}) {
		t.Fatal("mouse wheel down was not consumed")
	}
	wantDown := start + 3
	if wantDown > maxY {
		wantDown = maxY
	}
	if got := root.scrollY.Get(); got != wantDown {
		t.Fatalf("wheel down scroll = %d, want %d", got, wantDown)
	}

	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseWheelUp}) {
		t.Fatal("mouse wheel up was not consumed")
	}
	if got, want := root.scrollY.Get(), wantDown-3; got != want {
		t.Fatalf("wheel up scroll = %d, want %d", got, want)
	}
}

func TestRootMouseFirstWheelUpStartsFromStickyBottom(t *testing.T) {
	state := NewAppState()
	messages := make([]Message, 80)
	for i := range messages {
		messages[i] = Message{Kind: MsgAssistant, Text: "message"}
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	area := root.renderMessageArea(5)
	area.Render(gtui.NewBuffer(80, 5), 80, 5)

	_, maxY := area.MaxScroll()
	_, actualY := area.ScrollOffset()
	if maxY < 3 || actualY != maxY {
		t.Fatalf("sticky transcript offset = %d of %d, want bottom with room to scroll", actualY, maxY)
	}

	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseWheelUp}) {
		t.Fatal("first mouse wheel up was not consumed")
	}
	want := maxY - 3
	if got := root.scrollY.Get(); got != want {
		t.Fatalf("first wheel up state offset = %d, want %d", got, want)
	}
	if _, got := area.ScrollOffset(); got != want {
		t.Fatalf("first wheel up element offset = %d, want %d", got, want)
	}
}
