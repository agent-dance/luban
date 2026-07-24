package tui

import (
	"strings"
	"time"
	"unicode/utf8"
)

// TextArea is a multi-line text input with word wrapping and cursor management.
// It implements Component, KeyListener, WatcherProvider, and Focusable interfaces.
type TextArea struct {
	// Configuration (set via options, immutable after construction)
	width            int
	maxHeight        int
	border           BorderStyle
	textStyle        Style
	placeholder      string
	placeholderStyle Style
	cursorRune       rune
	focusColor       *Color
	borderGradient   *Gradient
	focusGradient    *Gradient
	autoFocus        bool
	submitKey        Key
	onSubmit         func(string)
	atomicRanges     func(string) []TextAreaAtomicRange
	onTextChange     func(string)

	// Reactive state
	text                  *State[string]
	cursorPos             *State[int]
	cursorExternallyBound bool
	// selectionAnchor stores the fixed end of an input selection. The cursor is
	// the moving end; -1 means there is no selection.
	selectionAnchor int
	selectionText   string
	selectionCursor int
	blink           *State[bool]
	focused         *State[bool]
	renderedElement *Element
	mouseNow        func() time.Time
	lastClickAt     time.Time
	lastClickX      int
	lastClickY      int
	lastClickText   string
	lastClickPos    int
	clickCount      int
}

// Interface assertions
var (
	_ Component       = (*TextArea)(nil)
	_ KeyListener     = (*TextArea)(nil)
	_ PasteListener   = (*TextArea)(nil)
	_ MouseListener   = (*TextArea)(nil)
	_ WatcherProvider = (*TextArea)(nil)
	_ Focusable       = (*TextArea)(nil)
	_ AppBinder       = (*TextArea)(nil)
)

// BindApp binds this TextArea's internal States to the given app.
func (t *TextArea) BindApp(app *App) {
	t.text.BindApp(app)
	t.cursorPos.BindApp(app)
	t.blink.BindApp(app)
	t.focused.BindApp(app)
}

// NewTextArea creates a new multi-line text input.
func NewTextArea(opts ...TextAreaOption) *TextArea {
	t := &TextArea{
		// Defaults
		width:            40,
		maxHeight:        0, // unlimited
		border:           BorderNone,
		textStyle:        Style{},
		placeholder:      "",
		placeholderStyle: Style{}.Dim(),
		cursorRune:       '▌',
		submitKey:        KeyEnter,

		// State
		text:            NewState(""),
		cursorPos:       NewState(0),
		selectionAnchor: -1,
		blink:           NewState(true),
		focused:         NewState(false),
		mouseNow:        time.Now,
	}
	for _, opt := range opts {
		opt(t)
	}
	t.setCursorPosition(t.cursorPos.Get())
	return t
}

// --- State Access ---

// Text returns the current text content.
func (t *TextArea) Text() string {
	return t.text.Get()
}

// SetText sets the text and moves cursor to end.
func (t *TextArea) SetText(s string) {
	t.clearSelection()
	t.setText(s)
	t.setCursorPosition(utf8.RuneCountInString(s))
}

// SetCursorPosition moves the cursor to a rune offset, clamped to the current
// text. This is useful when restoring editor state such as prompt history.
func (t *TextArea) SetCursorPosition(pos int) {
	t.clearSelection()
	t.setCursorPosition(textAreaBoundaryAtOrBefore(t.text.Get(), pos))
}

// CursorPosition returns the cursor's clamped rune offset.
func (t *TextArea) CursorPosition() int {
	return t.clampCursorPos()
}

// SelectionRange returns the selected rune range in reading order. The end is
// exclusive. ok is false when no non-empty selection exists.
func (t *TextArea) SelectionRange() (start, end int, ok bool) {
	anchor := t.selectionAnchor
	cursor := t.clampCursorPos()
	if anchor >= 0 && (t.selectionText != t.text.Get() || t.selectionCursor != cursor) {
		t.clearSelection()
		return 0, 0, false
	}
	maximum := utf8.RuneCountInString(t.text.Get())
	if anchor < 0 {
		return 0, 0, false
	}
	if anchor > maximum {
		anchor = maximum
	}
	if anchor == cursor {
		return 0, 0, false
	}
	if anchor < cursor {
		return anchor, cursor, true
	}
	return cursor, anchor, true
}

// SelectedText returns the currently selected input text.
func (t *TextArea) SelectedText() string {
	start, end, ok := t.SelectionRange()
	if !ok {
		return ""
	}
	return string([]rune(t.text.Get())[start:end])
}

// ClearSelection collapses any active selection without moving the cursor.
func (t *TextArea) ClearSelection() {
	t.clearSelection()
}

// DeleteSelection removes the selected text and reports whether anything was
// deleted.
func (t *TextArea) DeleteSelection() bool {
	return t.deleteSelection()
}

// SelectAll selects the complete input buffer. Terminals that report the
// Super modifier use this for Command+A on macOS.
func (t *TextArea) SelectAll() {
	end := utf8.RuneCountInString(t.text.Get())
	if end == 0 {
		t.clearSelection()
		t.setCursorPosition(0)
		return
	}
	t.selectionAnchor = 0
	t.selectionText = t.text.Get()
	t.selectionCursor = end
	t.setCursorPosition(end)
}

// CursorLine returns the cursor's zero-based visual row after wrapping.
func (t *TextArea) CursorLine() int {
	line, _ := t.cursorRowCol(t.wrapText())
	return line
}

// LineCount returns the number of visual rows after wrapping.
func (t *TextArea) LineCount() int {
	return len(t.wrapText())
}

// InsertText inserts text at the current cursor position and advances the
// cursor by the inserted rune count.
func (t *TextArea) InsertText(s string) {
	if s == "" {
		return
	}
	runes := []rune(t.text.Get())
	start, end, selected := t.SelectionRange()
	pos := t.clampCursorPos()
	if selected {
		start, end = t.expandRangeForAtomicRanges(start, end)
		pos = start
	} else {
		start, end = pos, pos
	}
	inserted := []rune(s)
	newRunes := make([]rune, 0, len(runes)-(end-start)+len(inserted))
	newRunes = append(newRunes, runes[:start]...)
	newRunes = append(newRunes, inserted...)
	newRunes = append(newRunes, runes[end:]...)
	t.clearSelection()
	t.setText(string(newRunes))
	t.setCursorPosition(pos + len(inserted))
	t.blink.Set(true)
}

// Clear clears the text area.
func (t *TextArea) Clear() {
	t.clearSelection()
	t.setText("")
	t.setCursorPosition(0)
}

// Height returns the total rendered height including border.
func (t *TextArea) Height() int {
	lines := t.wrapText()
	height := len(lines)
	if height < 1 {
		height = 1
	}
	if t.maxHeight > 0 && height > t.maxHeight {
		height = t.maxHeight
	}
	if t.border != BorderNone {
		height += 2
	}
	return height
}

// --- Component Interface ---

// Render returns the element tree for the text area.
func (t *TextArea) Render(app *App) *Element {
	lines := t.wrapText()
	height := len(lines)
	if height < 1 {
		height = 1
	}
	if t.maxHeight > 0 && height > t.maxHeight {
		height = t.maxHeight
	}

	// Account for border
	totalHeight := height
	if t.border != BorderNone {
		totalHeight += 2
	}

	opts := []Option{
		WithDirection(Column),
		WithHeight(totalHeight),
		WithFocusable(true),
		WithAutoFocus(t.autoFocus),
	}
	if t.width > 0 {
		opts = append(opts, WithWidth(t.width))
	}
	if t.border != BorderNone {
		opts = append(opts, WithBorder(t.border))
		if t.focused.Get() {
			if t.focusGradient != nil {
				opts = append(opts, WithBorderGradient(*t.focusGradient))
			} else if t.focusColor != nil {
				opts = append(opts, WithBorderStyle(NewStyle().Foreground(*t.focusColor)))
			}
		} else if t.borderGradient != nil {
			opts = append(opts, WithBorderGradient(*t.borderGradient))
		}
	}
	root := New(opts...)

	// Wire Element focus/blur to component focus/blur
	root.SetOnFocus(func(e *Element) {
		t.Focus()
	})
	root.SetOnBlur(func(e *Element) {
		t.Blur()
	})

	// Render placeholder or content
	if t.text.Get() == "" && t.placeholder != "" && !t.focused.Get() {
		root.AddChild(New(WithText(t.placeholder), WithTextStyle(t.placeholderStyle)))
	} else {
		start := t.visibleLineStart(lines)
		for i := start; i < start+height; i++ {
			if _, _, selected := t.SelectionRange(); selected {
				root.AddChild(New(WithStyledSpans(t.lineWithSelection(i)), WithWrap(false)))
				continue
			}
			root.AddChild(New(WithText(t.lineWithCursor(i)), WithTextStyle(t.textStyle), WithWrap(false)))
		}
	}

	t.renderedElement = root
	return root
}

// HandleMouse moves the cursor on one click, selects the clicked word on two,
// and selects the complete buffer on three. Modifier keys are intentionally
// accepted when the terminal reports them; terminals that reserve a modifier
// for native selection simply do not send that event.
func (t *TextArea) HandleMouse(me MouseEvent) bool {
	if me.Button != MouseLeft || me.Action != MousePress {
		return false
	}
	if t.renderedElement == nil {
		t.resetClickSequence()
		return false
	}
	content := t.renderedElement.ContentRect()
	if !content.Contains(me.X, me.Y) {
		t.resetClickSequence()
		return false
	}

	lines := t.wrapText()
	row := t.visibleLineStart(lines) + me.Y - content.Y
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		row = len(lines) - 1
	}
	cellColumn := me.X - content.X
	runeColumn := textAreaRuneColumn(lines[row], cellColumn)
	position := t.posFromRowCol(lines, row, runeColumn)

	t.Focus()
	clickCount := t.recordClick(me.X, me.Y, position)
	if clickCount > 1 {
		position = t.lastClickPos
	}
	switch clickCount {
	case 2:
		t.selectWordAt(position)
	case 3:
		t.SelectAll()
		t.resetClickSequence()
	default:
		t.SetCursorPosition(t.atomicBoundaryAtOrBefore(position))
	}
	t.blink.Set(true)
	return true
}

const textAreaMultiClickInterval = 500 * time.Millisecond

func (t *TextArea) recordClick(x, y, pos int) int {
	now := time.Now()
	if t.mouseNow != nil {
		now = t.mouseNow()
	}
	elapsed := now.Sub(t.lastClickAt)
	text := t.text.Get()
	if t.clickCount == 0 || x != t.lastClickX || y != t.lastClickY || text != t.lastClickText || elapsed < 0 || elapsed > textAreaMultiClickInterval {
		t.clickCount = 1
		t.lastClickPos = pos
	} else {
		t.clickCount++
	}
	t.lastClickAt = now
	t.lastClickX = x
	t.lastClickY = y
	t.lastClickText = text
	return t.clickCount
}

func (t *TextArea) resetClickSequence() {
	t.lastClickAt = time.Time{}
	t.lastClickX = 0
	t.lastClickY = 0
	t.lastClickText = ""
	t.lastClickPos = 0
	t.clickCount = 0
}

func (t *TextArea) visibleLineStart(lines []string) int {
	if t.maxHeight <= 0 || len(lines) <= t.maxHeight {
		return 0
	}
	cursorRow, _ := t.cursorRowCol(lines)
	start := cursorRow - t.maxHeight + 1
	if start < 0 {
		return 0
	}
	if maximum := len(lines) - t.maxHeight; start > maximum {
		return maximum
	}
	return start
}

// --- Focusable Interface ---

// IsFocusable returns true since TextArea can receive focus.
func (t *TextArea) IsFocusable() bool {
	return true
}

// IsTabStop returns true since TextArea participates in Tab navigation.
func (t *TextArea) IsTabStop() bool {
	return true
}

// Focus is called when the text area gains focus. Idempotent.
func (t *TextArea) Focus() {
	if t.focused.Get() {
		return
	}
	t.focused.Set(true)
	t.blink.Set(true)
}

// Blur is called when the text area loses focus. Idempotent.
func (t *TextArea) Blur() {
	if !t.focused.Get() {
		return
	}
	t.focused.Set(false)
}

// IsFocused returns whether this text area is currently focused.
func (t *TextArea) IsFocused() bool {
	return t.focused.Get()
}

// HandleEvent processes keyboard events.
func (t *TextArea) HandleEvent(e Event) bool {
	if paste, ok := e.(PasteEvent); ok {
		return t.HandlePaste(paste)
	}

	ke, ok := e.(KeyEvent)
	if !ok {
		return false
	}

	for _, binding := range t.KeyMap() {
		entry := dispatchEntry{pattern: binding.Pattern}
		if entry.matchesKey(ke) {
			binding.Handler(ke)
			return binding.Stop
		}
	}
	return false
}

// HandlePaste inserts a complete bracketed-paste payload at the cursor.
func (t *TextArea) HandlePaste(paste PasteEvent) bool {
	t.InsertText(paste.Text)
	return true
}

// --- KeyListener Interface ---

// KeyMap returns the key bindings for the text area.
func (t *TextArea) KeyMap() KeyMap {
	km := KeyMap{
		// Readline/Emacs editing commands. They live in the editor component so
		// every TextArea gets the same behavior without application-level key
		// handlers shadowing one another.
		OnFocused(KeyCtrlA, t.moveLogicalLineStart),
		OnFocused(KeyCtrlB, t.moveLeft),
		OnFocused(KeyCtrlE, t.moveLogicalLineEnd),
		OnFocused(KeyCtrlF, t.moveRight),
		OnFocused(KeyCtrlH, t.backspace),
		OnFocused(KeyCtrlK, t.deleteToLogicalLineEnd),
		OnFocused(KeyCtrlN, t.moveDown),
		OnFocused(KeyCtrlP, t.moveUp),
		OnFocused(KeyCtrlU, t.deleteToLogicalLineStart),
		OnFocused(KeyCtrlW, t.deleteWhitespaceDelimitedWordBackward),
		OnFocused(Rune('b').Alt(), t.moveWordBackward),
		OnFocused(Rune('f').Alt(), t.moveWordForward),
		OnFocused(Rune('d').Alt(), t.deleteWordForward),
		OnFocused(KeyBackspace.Alt(), t.deleteWordBackward),

		// Platform-style navigation and selection. Super is Command on macOS.
		// It is available when the host terminal reports the modifier instead
		// of consuming the shortcut itself.
		OnFocused(Rune('a').Super(), t.selectAll),
		OnFocused(KeyLeft.Super(), t.moveLogicalLineStart),
		OnFocused(KeyRight.Super(), t.moveLogicalLineEnd),
		OnFocused(KeyUp.Super(), t.moveBufferStart),
		OnFocused(KeyDown.Super(), t.moveBufferEnd),
		OnFocused(KeyBackspace.Super(), t.deleteToLogicalLineStart),
		OnFocused(KeyLeft.Alt(), t.moveWordBackward),
		OnFocused(KeyRight.Alt(), t.moveWordForward),
		OnFocused(KeyBackspace.Ctrl(), t.deleteWordBackward),
		OnFocused(KeyDelete.Ctrl(), t.deleteWordForward),

		OnFocused(KeyLeft.Shift(), t.selectLeft),
		OnFocused(KeyRight.Shift(), t.selectRight),
		OnFocused(KeyUp.Shift(), t.selectUp),
		OnFocused(KeyDown.Shift(), t.selectDown),
		OnFocused(KeyHome.Shift(), t.selectHome),
		OnFocused(KeyEnd.Shift(), t.selectEnd),
		OnFocused(KeyLeft.Alt().Shift(), t.selectWordBackward),
		OnFocused(KeyRight.Alt().Shift(), t.selectWordForward),
		OnFocused(KeyLeft.Ctrl().Shift(), t.selectWordBackward),
		OnFocused(KeyRight.Ctrl().Shift(), t.selectWordForward),
		OnFocused(KeyLeft.Super().Shift(), t.selectLogicalLineStart),
		OnFocused(KeyRight.Super().Shift(), t.selectLogicalLineEnd),
		OnFocused(KeyUp.Super().Shift(), t.selectBufferStart),
		OnFocused(KeyDown.Super().Shift(), t.selectBufferEnd),

		OnFocused(AnyRune, t.insertChar),
		OnFocused(KeyBackspace, t.backspace),
		OnFocused(KeyDelete, t.delete),
		OnFocused(KeyLeft, t.moveLeft),
		OnFocused(KeyRight, t.moveRight),
		OnFocused(KeyUp, t.moveUp),
		OnFocused(KeyDown, t.moveDown),
		OnFocused(KeyHome, t.moveHome),
		OnFocused(KeyEnd, t.moveEnd),
	}

	if t.submitKey == KeyEnter {
		km = append(km,
			OnFocused(Rune('j').Ctrl(), t.insertNewline),
			OnFocused(KeyEnter, t.submit),
		)
	} else {
		km = append(km,
			OnFocused(KeyEnter, t.insertNewline),
			OnFocused(t.submitKey, t.submit),
		)
	}

	km = append(km,
		OnFocused(KeyEscape, func(ke KeyEvent) {
			if app := ke.App(); app != nil {
				app.BlurFocused()
			}
		}),
	)

	return km
}

// --- WatcherProvider Interface ---

// Watchers returns watchers for cursor blink.
func (t *TextArea) Watchers() []Watcher {
	return []Watcher{
		OnTimer(500*time.Millisecond, func() {
			if t.focused.Get() {
				t.blink.Set(!t.blink.Get())
			}
		}),
	}
}

// --- Key Handlers ---

// insertChar inserts a character at the cursor position.
func (t *TextArea) insertChar(ke KeyEvent) {
	t.InsertText(string(ke.Rune))
}

// insertNewline inserts a newline character at the cursor position.
func (t *TextArea) insertNewline(ke KeyEvent) {
	t.InsertText("\n")
}

// backspace deletes the character before the cursor.
func (t *TextArea) backspace(ke KeyEvent) {
	if t.deleteSelection() {
		return
	}
	pos := t.clampCursorPos()
	if pos > 0 {
		start := t.previousCursorBoundary(pos)
		t.deleteRange(start, pos)
	}
}

// delete deletes the character at the cursor.
func (t *TextArea) delete(ke KeyEvent) {
	if t.deleteSelection() {
		return
	}
	runes := []rune(t.text.Get())
	pos := t.clampCursorPos()
	if pos < len(runes) {
		end := t.nextCursorBoundary(pos)
		t.deleteRange(pos, end)
	}
}

// moveLeft moves cursor left.
func (t *TextArea) moveLeft(ke KeyEvent) {
	if start, _, ok := t.SelectionRange(); ok {
		t.clearSelection()
		t.setCursorPosition(start)
		return
	}
	t.clearSelection()
	pos := t.clampCursorPos()
	if pos > 0 {
		t.setCursorPosition(t.previousCursorBoundary(pos))
		t.blink.Set(true)
	}
}

// moveRight moves cursor right.
func (t *TextArea) moveRight(ke KeyEvent) {
	if _, end, ok := t.SelectionRange(); ok {
		t.clearSelection()
		t.setCursorPosition(end)
		return
	}
	t.clearSelection()
	pos := t.clampCursorPos()
	if pos < utf8.RuneCountInString(t.text.Get()) {
		t.setCursorPosition(t.nextCursorBoundary(pos))
		t.blink.Set(true)
	}
}

// moveUp moves cursor up one line.
func (t *TextArea) moveUp(ke KeyEvent) {
	t.clearSelection()
	lines := t.wrapText()
	row, col := t.cursorRowCol(lines)
	if row > 0 {
		prevLine := lines[row-1]
		cellColumn := textAreaCellColumn(lines[row], col)
		t.setCursorPosition(t.posFromRowCol(lines, row-1, textAreaRuneColumn(prevLine, cellColumn)))
		t.blink.Set(true)
	}
}

// moveDown moves cursor down one line.
func (t *TextArea) moveDown(ke KeyEvent) {
	t.clearSelection()
	lines := t.wrapText()
	row, col := t.cursorRowCol(lines)
	if row < len(lines)-1 {
		nextLine := lines[row+1]
		cellColumn := textAreaCellColumn(lines[row], col)
		t.setCursorPosition(t.posFromRowCol(lines, row+1, textAreaRuneColumn(nextLine, cellColumn)))
		t.blink.Set(true)
	}
}

// moveHome moves cursor to start of current line.
func (t *TextArea) moveHome(ke KeyEvent) {
	t.clearSelection()
	lines := t.wrapText()
	row, _ := t.cursorRowCol(lines)
	t.setCursorPosition(t.posFromRowCol(lines, row, 0))
	t.blink.Set(true)
}

// moveEnd moves cursor to end of current line.
func (t *TextArea) moveEnd(ke KeyEvent) {
	t.clearSelection()
	lines := t.wrapText()
	row, _ := t.cursorRowCol(lines)
	t.setCursorPosition(t.posFromRowCol(lines, row, utf8.RuneCountInString(lines[row])))
	t.blink.Set(true)
}

// submit calls the onSubmit callback.
func (t *TextArea) submit(ke KeyEvent) {
	if t.onSubmit != nil {
		t.onSubmit(t.text.Get())
	}
}

// --- Text Wrapping and Cursor Position ---

// wrapText wraps the text to fit within width, respecting embedded newlines.
// Width is measured in terminal cell units (CJK characters count as 2 cells).
func (t *TextArea) wrapText() []string {
	text := t.text.Get()
	if text == "" {
		return []string{""}
	}

	var lines []string

	// Split on embedded newlines first
	paragraphs := strings.Split(text, "\n")

	for _, para := range paragraphs {
		if para == "" {
			lines = append(lines, "")
			continue
		}

		// Wrap this paragraph to width using cell widths
		var currentLine strings.Builder
		currentWidth := 0
		for _, grapheme := range textAreaGraphemes(para) {
			rw := grapheme.width
			if t.width > 0 && currentWidth+rw > t.width && currentWidth > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
				currentWidth = 0
			}
			currentLine.WriteString(grapheme.text)
			currentWidth += rw
		}
		lines = append(lines, currentLine.String())
	}

	return lines
}

// cursorRowCol returns the row and column of the cursor.
// Column is measured in rune count (not cell width) for cursor positioning.
func (t *TextArea) cursorRowCol(lines []string) (row, col int) {
	pos := t.clampCursorPos()
	for lineIndex, line := range lines {
		start, end, newline := t.visualLineRuneRange(lines, lineIndex)
		if pos < end {
			return lineIndex, pos - start
		}
		if pos == end {
			if !newline && lineIndex < len(lines)-1 {
				return lineIndex + 1, 0
			}
			return lineIndex, utf8.RuneCountInString(line)
		}
	}
	if len(lines) == 0 {
		return 0, 0
	}
	return len(lines) - 1, utf8.RuneCountInString(lines[len(lines)-1])
}

// posFromRowCol converts row/col back to absolute position.
func (t *TextArea) posFromRowCol(lines []string, targetRow, targetCol int) int {
	if targetRow < 0 {
		targetRow = 0
	}
	if targetRow >= len(lines) {
		return utf8.RuneCountInString(t.text.Get())
	}
	start, end, _ := t.visualLineRuneRange(lines, targetRow)
	if targetCol < 0 {
		targetCol = 0
	}
	if maximum := end - start; targetCol > maximum {
		targetCol = maximum
	}
	return start + targetCol
}

func textAreaCellColumn(line string, runeColumn int) int {
	column := 0
	runeOffset := 0
	for _, grapheme := range textAreaGraphemes(line) {
		if runeOffset >= runeColumn {
			break
		}
		column += grapheme.width
		runeOffset += grapheme.runeCount
	}
	return column
}

func textAreaRuneColumn(line string, cellColumn int) int {
	column := 0
	runeOffset := 0
	for _, grapheme := range textAreaGraphemes(line) {
		if column+grapheme.width > cellColumn {
			return runeOffset
		}
		column += grapheme.width
		runeOffset += grapheme.runeCount
	}
	return runeOffset
}

// lineWithCursor returns a line with the cursor character inserted.
func (t *TextArea) lineWithCursor(lineIdx int) string {
	lines := t.wrapText()
	if lineIdx >= len(lines) {
		return " "
	}

	row, col := t.cursorRowCol(lines)
	line := lines[lineIdx]

	if lineIdx == row && t.focused.Get() {
		cursor := string(t.cursorRune)
		if !t.blink.Get() {
			cursor = " "
		}
		runes := []rune(line)
		if col >= len(runes) {
			return line + cursor
		}
		withCursor := append(runes[:col], append([]rune{t.cursorRune}, runes[col:]...)...)
		if !t.blink.Get() {
			withCursor[col] = ' '
		}
		return string(withCursor)
	}

	if line == "" {
		return " "
	}
	return line
}

func (t *TextArea) clampCursorPos() int {
	pos := t.cursorPos.Get()
	if pos < 0 {
		return 0
	}
	max := utf8.RuneCountInString(t.text.Get())
	if pos > max {
		pos = max
	}
	return t.atomicBoundaryAtOrBefore(textAreaBoundaryAtOrBefore(t.text.Get(), pos))
}
