package tui

import (
	"strings"
	"testing"
)

func TestParseInputBracketedPasteProducesSingleEvent(t *testing.T) {
	input := []byte("a" + bracketedPasteStart + "first\r\nsecond\nthird" + bracketedPasteEnd + "b")
	events := parseInput(input)
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	if key, ok := events[0].(KeyEvent); !ok || key.Rune != 'a' {
		t.Fatalf("first event = %#v, want rune a", events[0])
	}
	paste, ok := events[1].(PasteEvent)
	if !ok {
		t.Fatalf("second event = %T, want PasteEvent", events[1])
	}
	if paste.Text != "first\r\nsecond\nthird" {
		t.Fatalf("paste text = %q", paste.Text)
	}
	if key, ok := events[2].(KeyEvent); !ok || key.Rune != 'b' {
		t.Fatalf("third event = %#v, want rune b", events[2])
	}
}

func TestParseInputWithRemainderBuffersLargeBracketedPaste(t *testing.T) {
	payload := strings.Repeat("界", 140) + "\r\nlast line"
	firstChunk := []byte("x" + bracketedPasteStart + payload[:250])
	events, remaining := parseInputWithRemainder(firstChunk)
	if len(events) != 1 {
		t.Fatalf("first chunk events = %d, want only leading rune", len(events))
	}
	if len(remaining) == 0 {
		t.Fatal("expected partial paste to be buffered")
	}

	combined := append(remaining, []byte(payload[250:]+bracketedPasteEnd+"z")...)
	events, remaining = parseInputWithRemainder(combined)
	if len(remaining) != 0 {
		t.Fatalf("remaining bytes = %d, want 0", len(remaining))
	}
	if len(events) != 2 {
		t.Fatalf("completed chunk events = %d, want paste and trailing rune", len(events))
	}
	paste, ok := events[0].(PasteEvent)
	if !ok || paste.Text != payload {
		t.Fatalf("paste = %#v, want complete payload", events[0])
	}
	if key, ok := events[1].(KeyEvent); !ok || key.Rune != 'z' {
		t.Fatalf("trailing event = %#v, want rune z", events[1])
	}
}

func TestParseInputWithRemainderBuffersSplitPasteMarker(t *testing.T) {
	events, remaining := parseInputWithRemainder([]byte("q\x1b[20"))
	if len(events) != 1 || len(remaining) == 0 {
		t.Fatalf("events=%d remaining=%q, want q plus partial marker", len(events), remaining)
	}

	combined := append(remaining, []byte("0~one\ntwo"+bracketedPasteEnd)...)
	events, remaining = parseInputWithRemainder(combined)
	if len(remaining) != 0 || len(events) != 1 {
		t.Fatalf("events=%d remaining=%q, want one completed paste", len(events), remaining)
	}
	paste, ok := events[0].(PasteEvent)
	if !ok || paste.Text != "one\ntwo" {
		t.Fatalf("event = %#v, want complete paste", events[0])
	}
}

func TestTextAreaInsertTextAtCursor(t *testing.T) {
	area := NewTextArea()
	area.SetText("ab界")
	area.cursorPos.Set(1)
	area.InsertText("XY")
	if got := area.Text(); got != "aXYb界" {
		t.Fatalf("text = %q, want %q", got, "aXYb界")
	}
	if got := area.cursorPos.Get(); got != 3 {
		t.Fatalf("cursor = %d, want 3", got)
	}
}

func TestAppDispatchesPasteToFocusedComponent(t *testing.T) {
	area := NewTextArea()
	area.Focus()
	app := &App{
		focus:         newFocusManager(),
		buffer:        NewBuffer(80, 24),
		rootComponent: area,
	}
	app.root = area.Render(app)

	if handled := app.Dispatch(PasteEvent{Text: "one\ntwo"}); !handled {
		t.Fatal("expected focused component to handle paste")
	}
	if got := area.Text(); got != "one\ntwo" {
		t.Fatalf("text = %q, want pasted content", got)
	}
}
