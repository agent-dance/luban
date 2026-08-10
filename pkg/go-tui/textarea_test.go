package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func atomicTokenRanges(token string) func(string) []TextAreaAtomicRange {
	return func(text string) []TextAreaAtomicRange {
		byteStart := strings.Index(text, token)
		if byteStart < 0 {
			return nil
		}
		start := utf8.RuneCountInString(text[:byteStart])
		return []TextAreaAtomicRange{{Start: start, End: start + utf8.RuneCountInString(token)}}
	}
}

func TestTextArea_SetText_UsesRuneCursorPosition(t *testing.T) {
	ta := NewTextArea()
	ta.BindApp(testApp)
	ta.SetText("a界")

	if got := ta.cursorPos.Get(); got != 2 {
		t.Fatalf("cursorPos = %d, want 2", got)
	}
}

func TestTextArea_CursorPosition_ReturnsRuneOffset(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("a界🙂")
	ta.SetCursorPosition(2)

	if got := ta.CursorPosition(); got != 2 {
		t.Fatalf("CursorPosition() = %d, want 2", got)
	}
}

func TestTextArea_Edit_MultibyteRunes(t *testing.T) {
	ta := NewTextArea()
	ta.BindApp(testApp)
	ta.SetText("a界")
	ta.cursorPos.Set(1)

	ta.insertChar(KeyEvent{Key: KeyRune, Rune: '🙂'})
	if got := ta.Text(); got != "a🙂界" {
		t.Fatalf("text after insert = %q, want %q", got, "a🙂界")
	}

	ta.backspace(KeyEvent{Key: KeyBackspace})
	if got := ta.Text(); got != "a界" {
		t.Fatalf("text after backspace = %q, want %q", got, "a界")
	}

	ta.delete(KeyEvent{Key: KeyDelete})
	if got := ta.Text(); got != "a" {
		t.Fatalf("text after delete = %q, want %q", got, "a")
	}
}

func TestTextArea_MoveRight_UsesRuneLength(t *testing.T) {
	ta := NewTextArea()
	ta.BindApp(testApp)
	ta.SetText("é界")
	ta.cursorPos.Set(0)

	ta.moveRight(KeyEvent{Key: KeyRight})
	ta.moveRight(KeyEvent{Key: KeyRight})
	ta.moveRight(KeyEvent{Key: KeyRight})

	if got := ta.cursorPos.Get(); got != 2 {
		t.Fatalf("cursorPos = %d, want 2", got)
	}
}

func TestTextAreaExternalCursorStateTracksEveryMutation(t *testing.T) {
	text := NewState("ab界")
	cursor := NewState(1)
	ta := NewTextArea(WithTextAreaValue(text), WithTextAreaCursorPosition(cursor))

	ta.InsertText("🙂")
	if got, want := cursor.Get(), 2; got != want {
		t.Fatalf("cursor after insert = %d, want %d", got, want)
	}
	ta.SetText("restored")
	if got, want := cursor.Get(), 8; got != want {
		t.Fatalf("cursor after SetText = %d, want %d", got, want)
	}
	ta.Clear()
	if got := cursor.Get(); got != 0 {
		t.Fatalf("cursor after Clear = %d, want 0", got)
	}
}

func TestTextAreaExternalValueAndCursorOptionsAreOrderIndependent(t *testing.T) {
	for _, options := range [][]TextAreaOption{
		{WithTextAreaValue(NewState("abcdef")), WithTextAreaCursorPosition(NewState(3))},
		{WithTextAreaCursorPosition(NewState(3)), WithTextAreaValue(NewState("abcdef"))},
	} {
		ta := NewTextArea(options...)
		if got := ta.CursorPosition(); got != 3 {
			t.Fatalf("external cursor after option application = %d, want 3", got)
		}
	}
}

func TestTextAreaAtomicRangeCursorMovementSkipsWholeToken(t *testing.T) {
	const token = "[Image #1]"
	ta := NewTextArea(WithTextAreaAtomicRanges(atomicTokenRanges(token)))
	ta.SetText("a" + token + "b")
	start := utf8.RuneCountInString("a")
	end := start + utf8.RuneCountInString(token)

	ta.SetCursorPosition(start)
	ta.HandleEvent(KeyEvent{Key: KeyRight})
	if got := ta.CursorPosition(); got != end {
		t.Fatalf("cursor after Right = %d, want token end %d", got, end)
	}
	ta.HandleEvent(KeyEvent{Key: KeyLeft})
	if got := ta.CursorPosition(); got != start {
		t.Fatalf("cursor after Left = %d, want token start %d", got, start)
	}
	ta.SetCursorPosition(start + 2)
	if got := ta.CursorPosition(); got != start {
		t.Fatalf("cursor inside token = %d, want snapped start %d", got, start)
	}
}

func TestTextAreaAtomicRangeDeletesAndReplacesWholeToken(t *testing.T) {
	const token = "[Image #1]"
	newTextArea := func() *TextArea {
		ta := NewTextArea(WithTextAreaAtomicRanges(atomicTokenRanges(token)))
		ta.SetText("a" + token + "b")
		return ta
	}
	start := utf8.RuneCountInString("a")
	end := start + utf8.RuneCountInString(token)

	backspace := newTextArea()
	backspace.SetCursorPosition(end)
	backspace.HandleEvent(KeyEvent{Key: KeyBackspace})
	if got := backspace.Text(); got != "ab" {
		t.Fatalf("text after Backspace = %q, want whole token removed", got)
	}

	forwardDelete := newTextArea()
	forwardDelete.SetCursorPosition(start)
	forwardDelete.HandleEvent(KeyEvent{Key: KeyDelete})
	if got := forwardDelete.Text(); got != "ab" {
		t.Fatalf("text after Delete = %q, want whole token removed", got)
	}

	replace := newTextArea()
	replace.SetCursorPosition(start)
	replace.HandleEvent(KeyEvent{Key: KeyRight, Mod: ModShift})
	if got := replace.SelectedText(); got != token {
		t.Fatalf("selected text = %q, want atomic token", got)
	}
	replace.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'x'})
	if got := replace.Text(); got != "axb" {
		t.Fatalf("text after replacement = %q, want %q", got, "axb")
	}
}

func TestTextAreaMaxHeightRendersCursorFollowingViewport(t *testing.T) {
	ta := NewTextArea(WithTextAreaWidth(20), WithTextAreaMaxHeight(2), WithTextAreaCursor('|'))
	ta.SetText("first\nsecond\nthird\nfourth")
	ta.SetCursorPosition(len([]rune("first\nsecond\nthird\nfo")))
	ta.Focus()

	rendered := ta.Render(nil)
	children := rendered.Children()
	if len(children) != 2 {
		t.Fatalf("visible lines = %d, want 2", len(children))
	}
	if got, want := children[0].Text(), "third"; got != want {
		t.Fatalf("first visible line = %q, want %q", got, want)
	}
	if got, want := children[1].Text(), "fo|urth"; got != want {
		t.Fatalf("cursor line = %q, want %q", got, want)
	}
}

func TestTextAreaReadlineLineEditing(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("first\nsecond line")
	ta.SetCursorPosition(len([]rune("first\nsec")))

	if handled := ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'a', Mod: ModCtrl}); !handled {
		t.Fatal("Ctrl+A was not handled")
	}
	if got, want := ta.CursorPosition(), len([]rune("first\n")); got != want {
		t.Fatalf("cursor after Ctrl+A = %d, want logical line start %d", got, want)
	}

	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'e', Mod: ModCtrl})
	if got, want := ta.CursorPosition(), len([]rune("first\nsecond line")); got != want {
		t.Fatalf("cursor after Ctrl+E = %d, want logical line end %d", got, want)
	}

	ta.SetCursorPosition(len([]rune("first\nsecond")))
	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'u', Mod: ModCtrl})
	if got, want := ta.Text(), "first\n line"; got != want {
		t.Fatalf("text after Ctrl+U = %q, want %q", got, want)
	}
	if got, want := ta.CursorPosition(), len([]rune("first\n")); got != want {
		t.Fatalf("cursor after Ctrl+U = %d, want %d", got, want)
	}

	ta.SetText("first\nsecond line")
	ta.SetCursorPosition(len([]rune("first\nsecond")))
	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'k', Mod: ModCtrl})
	if got, want := ta.Text(), "first\nsecond"; got != want {
		t.Fatalf("text after Ctrl+K = %q, want %q", got, want)
	}
}

func TestTextAreaWordEditingIsUnicodeAware(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("prefix  中文单词  suffix")
	ta.SetCursorPosition(len([]rune("prefix  中文单词  ")))

	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'w', Mod: ModCtrl})
	if got, want := ta.Text(), "prefix  suffix"; got != want {
		t.Fatalf("text after Ctrl+W = %q, want %q", got, want)
	}
	if got, want := ta.CursorPosition(), len([]rune("prefix  ")); got != want {
		t.Fatalf("cursor after Ctrl+W = %d, want %d", got, want)
	}

	ta.SetText("one, two")
	ta.SetCursorPosition(0)
	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'd', Mod: ModAlt})
	if got, want := ta.Text(), ", two"; got != want {
		t.Fatalf("text after Alt+D = %q, want %q", got, want)
	}

	ta.SetText("one  two")
	ta.SetCursorPosition(len([]rune("one  ")))
	ta.HandleEvent(KeyEvent{Key: KeyLeft, Mod: ModAlt})
	if got := ta.CursorPosition(); got != 0 {
		t.Fatalf("cursor after Alt+Left = %d, want 0", got)
	}
	ta.HandleEvent(KeyEvent{Key: KeyRight, Mod: ModAlt})
	if got, want := ta.CursorPosition(), len([]rune("one")); got != want {
		t.Fatalf("cursor after Alt+Right = %d, want %d", got, want)
	}
}

func TestTextAreaSuperASelectsAndTypingReplacesInput(t *testing.T) {
	ta := NewTextArea(WithTextAreaCursor('|'))
	ta.SetText("a界🙂")
	ta.Focus()

	if handled := ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'a', Mod: ModSuper}); !handled {
		t.Fatal("Super+A was not handled")
	}
	if start, end, ok := ta.SelectionRange(); !ok || start != 0 || end != 3 {
		t.Fatalf("selection = (%d, %d, %v), want (0, 3, true)", start, end, ok)
	}
	if got := ta.SelectedText(); got != "a界🙂" {
		t.Fatalf("selected text = %q, want full input", got)
	}

	rendered := ta.Render(nil)
	spans := rendered.Children()[0].StyledSpans()
	foundSelected := false
	for _, span := range spans {
		if span.Style.HasAttr(AttrReverse) && span.Text != "" {
			foundSelected = true
			break
		}
	}
	if !foundSelected {
		t.Fatalf("selected input was not rendered with reverse style: %#v", spans)
	}

	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'x'})
	if got := ta.Text(); got != "x" {
		t.Fatalf("typing over selection produced %q, want %q", got, "x")
	}
	if _, _, ok := ta.SelectionRange(); ok {
		t.Fatal("selection remained active after replacement")
	}
}

func TestTextAreaSelectionSupportsDeletePasteAndShiftNavigation(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("alpha beta")
	ta.SetCursorPosition(5)
	ta.HandleEvent(KeyEvent{Key: KeyRight, Mod: ModShift})
	ta.HandleEvent(KeyEvent{Key: KeyRight, Mod: ModShift})
	ta.HandleEvent(KeyEvent{Key: KeyRight, Mod: ModShift})
	if got := ta.SelectedText(); got != " be" {
		t.Fatalf("Shift+Right selected %q, want %q", got, " be")
	}
	ta.HandlePaste(PasteEvent{Text: "-"})
	if got := ta.Text(); got != "alpha-ta" {
		t.Fatalf("paste over selection = %q, want %q", got, "alpha-ta")
	}

	ta.SelectAll()
	ta.HandleEvent(KeyEvent{Key: KeyBackspace})
	if got := ta.Text(); got != "" {
		t.Fatalf("Backspace over selection = %q, want empty", got)
	}
}

func TestTextAreaDoesNotInsertUnhandledSuperRune(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("draft")

	if handled := ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'z', Mod: ModSuper}); handled {
		t.Fatal("unbound Super+Z was consumed as text")
	}
	if got := ta.Text(); got != "draft" {
		t.Fatalf("unbound Super+Z changed input to %q", got)
	}
}

func TestTextAreaCollapsedSelectionDoesNotReappearAfterBackspace(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("abc")
	ta.SetCursorPosition(1)
	ta.HandleEvent(KeyEvent{Key: KeyRight, Mod: ModShift})
	ta.HandleEvent(KeyEvent{Key: KeyLeft, Mod: ModShift})
	ta.HandleEvent(KeyEvent{Key: KeyBackspace})

	if _, _, ok := ta.SelectionRange(); ok {
		t.Fatal("collapsed selection reappeared after Backspace")
	}
	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'x'})
	if got, want := ta.Text(), "xbc"; got != want {
		t.Fatalf("text after collapsed selection edit = %q, want %q", got, want)
	}
}

func TestTextAreaExternalStateChangeInvalidatesSelection(t *testing.T) {
	text := NewState("abcdef")
	cursor := NewState(6)
	ta := NewTextArea(WithTextAreaValue(text), WithTextAreaCursorPosition(cursor))
	ta.SelectAll()

	text.Set("xy")
	cursor.Set(2)
	if _, _, ok := ta.SelectionRange(); ok {
		t.Fatal("selection survived an external editor-state replacement")
	}
	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'z'})
	if got, want := ta.Text(), "xyz"; got != want {
		t.Fatalf("typing after external replacement = %q, want %q", got, want)
	}
}

func TestTextAreaSoftWrapCoordinatesRoundTrip(t *testing.T) {
	ta := NewTextArea(WithTextAreaWidth(2))
	ta.SetText("abcd")
	ta.SetCursorPosition(0)
	ta.HandleEvent(KeyEvent{Key: KeyDown})
	if got := ta.CursorPosition(); got != 2 {
		t.Fatalf("Down to soft-wrapped row = %d, want 2", got)
	}

	ta.SetCursorPosition(3)
	ta.HandleEvent(KeyEvent{Key: KeyHome})
	if got := ta.CursorPosition(); got != 2 {
		t.Fatalf("Home on soft-wrapped row = %d, want 2", got)
	}
}

func TestTextAreaVerticalMovementUsesTerminalCellColumn(t *testing.T) {
	ta := NewTextArea(WithTextAreaWidth(20))
	ta.SetText("界a\nabcd")
	ta.SetCursorPosition(1) // After the double-width CJK rune: visual column 2.
	ta.HandleEvent(KeyEvent{Key: KeyDown})
	if got, want := ta.CursorPosition(), len([]rune("界a\nab")); got != want {
		t.Fatalf("CJK-aware Down cursor = %d, want %d", got, want)
	}
}

func TestTextAreaCtrlWUsesWhitespaceDelimitedReadlineWord(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("keep foo.bar  tail")
	ta.SetCursorPosition(len([]rune("keep foo.bar  ")))
	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'w', Mod: ModCtrl})
	if got, want := ta.Text(), "keep tail"; got != want {
		t.Fatalf("Ctrl+W punctuation token = %q, want %q", got, want)
	}
}

func TestTextAreaCtrlKAtLineEndDeletesNewline(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("first\nsecond")
	ta.SetCursorPosition(len([]rune("first")))
	ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'k', Mod: ModCtrl})
	if got, want := ta.Text(), "firstsecond"; got != want {
		t.Fatalf("Ctrl+K at line end = %q, want %q", got, want)
	}
}

func TestTextAreaDoesNotDowngradeHyperOrMetaRunes(t *testing.T) {
	ta := NewTextArea()
	ta.SetText("draft")
	for _, mod := range []Modifier{ModHyper, ModMeta, ModSuper | ModHyper} {
		if handled := ta.HandleEvent(KeyEvent{Key: KeyRune, Rune: 'a', Mod: mod}); handled {
			t.Fatalf("modified rune with %v was consumed as text", mod)
		}
	}
	if got := ta.Text(); got != "draft" {
		t.Fatalf("unsupported modified rune changed input to %q", got)
	}
}

func TestTextAreaEditingPreservesGraphemeClusters(t *testing.T) {
	for _, cluster := range []string{"e\u0301", "👩🏽‍💻", "🇨🇳"} {
		t.Run(cluster, func(t *testing.T) {
			ta := NewTextArea(WithTextAreaWidth(2))
			ta.SetText(cluster + "x")
			clusterEnd := len([]rune(cluster))
			ta.SetCursorPosition(clusterEnd)

			ta.HandleEvent(KeyEvent{Key: KeyLeft})
			if got := ta.CursorPosition(); got != 0 {
				t.Fatalf("Left split grapheme %q at rune %d", cluster, got)
			}
			ta.HandleEvent(KeyEvent{Key: KeyRight, Mod: ModShift})
			if got := ta.SelectedText(); got != cluster {
				t.Fatalf("Shift+Right selected %q, want grapheme %q", got, cluster)
			}
			ta.HandleEvent(KeyEvent{Key: KeyBackspace})
			if got := ta.Text(); got != "x" {
				t.Fatalf("Backspace over grapheme selection = %q, want x", got)
			}
		})
	}
}

func TestTextAreaWrapDoesNotSplitGraphemeCluster(t *testing.T) {
	ta := NewTextArea(WithTextAreaWidth(2))
	ta.SetText("👩🏽‍💻x")
	if got := ta.LineCount(); got != 2 {
		t.Fatalf("grapheme-aware line count = %d, want 2", got)
	}
}

func TestTextAreaSelectionContrastsWithReverseBaseStyle(t *testing.T) {
	ta := NewTextArea(WithTextAreaTextStyle(NewStyle().Reverse()))
	ta.SetText("abc")
	ta.SelectAll()
	spans := ta.Render(nil).Children()[0].StyledSpans()
	if len(spans) == 0 || spans[0].Style.HasAttr(AttrReverse) {
		t.Fatalf("selection did not toggle reverse base style: %#v", spans)
	}
}

func TestTextAreaDoubleClickSelectsUnicodeWord(t *testing.T) {
	ta := NewTextArea(WithTextAreaWidth(40))
	ta.SetText("one, 中文词!")
	content := renderTextAreaForMouseTest(ta, 40, 3)
	now := time.Unix(100, 0)
	ta.mouseNow = func() time.Time { return now }
	click := MouseEvent{Button: MouseLeft, Action: MousePress, X: content.X + 7, Y: content.Y}

	if !ta.HandleMouse(click) {
		t.Fatal("first input click was not handled")
	}
	now = now.Add(100 * time.Millisecond)
	if !ta.HandleMouse(click) {
		t.Fatal("second input click was not handled")
	}
	if got, want := ta.SelectedText(), "中文词"; got != want {
		t.Fatalf("double-click selection = %q, want %q", got, want)
	}
}

func TestTextAreaTripleClickSelectsAll(t *testing.T) {
	ta := NewTextArea(WithTextAreaWidth(40))
	ta.SetText("alpha beta\n第二行")
	content := renderTextAreaForMouseTest(ta, 40, 4)
	now := time.Unix(200, 0)
	ta.mouseNow = func() time.Time { return now }
	click := MouseEvent{Button: MouseLeft, Action: MousePress, X: content.X + 1, Y: content.Y}

	for index := 0; index < 3; index++ {
		if !ta.HandleMouse(click) {
			t.Fatalf("input click %d was not handled", index+1)
		}
		now = now.Add(100 * time.Millisecond)
	}
	if got, want := ta.SelectedText(), ta.Text(); got != want {
		t.Fatalf("triple-click selection = %q, want full input %q", got, want)
	}
}

func TestTextAreaDoubleClickKeepsInitialTargetWhenViewportMoves(t *testing.T) {
	ta := NewTextArea(WithTextAreaWidth(40), WithTextAreaMaxHeight(2))
	ta.SetText("alpha\nbeta\ngamma")
	content := renderTextAreaForMouseTest(ta, 40, 3)
	now := time.Unix(250, 0)
	ta.mouseNow = func() time.Time { return now }
	click := MouseEvent{Button: MouseLeft, Action: MousePress, X: content.X + 1, Y: content.Y}

	ta.HandleMouse(click)
	if got := ta.CursorPosition(); got != len([]rune("alpha\nb")) {
		t.Fatalf("first click cursor = %d, want beta row", got)
	}
	content = renderTextAreaForMouseTest(ta, 40, 3)
	click.X = content.X + 1
	click.Y = content.Y
	now = now.Add(100 * time.Millisecond)
	ta.HandleMouse(click)
	if got, want := ta.SelectedText(), "beta"; got != want {
		t.Fatalf("double-click selection after viewport move = %q, want %q", got, want)
	}
}

func TestTextAreaMultiClickRequiresSameCellWithinInterval(t *testing.T) {
	for _, test := range []struct {
		name        string
		secondX     int
		secondDelay time.Duration
	}{
		{name: "different cell", secondX: 7, secondDelay: 100 * time.Millisecond},
		{name: "expired interval", secondX: 1, secondDelay: textAreaMultiClickInterval + time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			ta := NewTextArea(WithTextAreaWidth(40))
			ta.SetText("alpha beta")
			content := renderTextAreaForMouseTest(ta, 40, 3)
			now := time.Unix(300, 0)
			ta.mouseNow = func() time.Time { return now }

			ta.HandleMouse(MouseEvent{Button: MouseLeft, Action: MousePress, X: content.X + 1, Y: content.Y})
			now = now.Add(test.secondDelay)
			ta.HandleMouse(MouseEvent{Button: MouseLeft, Action: MousePress, X: content.X + test.secondX, Y: content.Y})
			if got := ta.SelectedText(); got != "" {
				t.Fatalf("unrelated clicks selected %q", got)
			}
		})
	}
}

func TestTextAreaPositionAtPointHasNoCursorSideEffects(t *testing.T) {
	ta := NewTextArea(WithTextAreaWidth(5))
	ta.SetText("abcd中文")
	ta.SetCursorPosition(0)
	content := renderTextAreaForMouseTest(ta, 10, 3)

	position, ok := ta.PositionAtPoint(content.X+2, content.Y+1)
	if !ok {
		t.Fatal("visible wrapped cell was not resolved")
	}
	if want := len([]rune("abcd中")); position != want {
		t.Fatalf("wrapped position = %d, want %d", position, want)
	}
	if got := ta.CursorPosition(); got != 0 {
		t.Fatalf("position lookup moved cursor to %d", got)
	}
}

func renderTextAreaForMouseTest(ta *TextArea, width, height int) Rect {
	element := ta.Render(nil)
	element.Render(NewBuffer(width, height), width, height)
	return element.ContentRect()
}
