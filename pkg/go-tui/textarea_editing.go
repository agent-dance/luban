package tui

import (
	"sort"
	"unicode"
	"unicode/utf8"
)

// setCursorPosition moves the cursor without changing an active selection.
// Public cursor movement uses SetCursorPosition, which clears the selection.
func (t *TextArea) setCursorPosition(pos int) {
	if pos < 0 {
		pos = 0
	}
	if maximum := utf8.RuneCountInString(t.text.Get()); pos > maximum {
		pos = maximum
	}
	pos = textAreaBoundaryAtOrBefore(t.text.Get(), pos)
	pos = t.atomicBoundaryAtOrBefore(pos)
	t.cursorPos.Set(pos)
	t.blink.Set(true)
}

func (t *TextArea) setText(text string) {
	if t.text.Get() == text {
		return
	}
	t.text.Set(text)
	if t.onTextChange != nil {
		t.onTextChange(text)
	}
}

func (t *TextArea) normalizedAtomicRanges() []TextAreaAtomicRange {
	if t.atomicRanges == nil {
		return nil
	}
	maximum := utf8.RuneCountInString(t.text.Get())
	ranges := append([]TextAreaAtomicRange(nil), t.atomicRanges(t.text.Get())...)
	valid := ranges[:0]
	for _, span := range ranges {
		if span.Start < 0 {
			span.Start = 0
		}
		if span.End > maximum {
			span.End = maximum
		}
		if span.Start < span.End {
			valid = append(valid, span)
		}
	}
	sort.Slice(valid, func(i, j int) bool {
		if valid[i].Start == valid[j].Start {
			return valid[i].End < valid[j].End
		}
		return valid[i].Start < valid[j].Start
	})
	return valid
}

func (t *TextArea) atomicBoundaryAtOrBefore(pos int) int {
	for _, span := range t.normalizedAtomicRanges() {
		if pos <= span.Start {
			break
		}
		if pos < span.End {
			return span.Start
		}
	}
	return pos
}

func (t *TextArea) atomicBoundaryAtOrAfter(pos int) int {
	for _, span := range t.normalizedAtomicRanges() {
		if pos <= span.Start {
			break
		}
		if pos < span.End {
			return span.End
		}
	}
	return pos
}

func (t *TextArea) previousCursorBoundary(pos int) int {
	for _, span := range t.normalizedAtomicRanges() {
		if pos > span.Start && pos <= span.End {
			return span.Start
		}
	}
	return textAreaPreviousGraphemeBoundary(t.text.Get(), pos)
}

func (t *TextArea) nextCursorBoundary(pos int) int {
	for _, span := range t.normalizedAtomicRanges() {
		if pos >= span.Start && pos < span.End {
			return span.End
		}
	}
	return textAreaNextGraphemeBoundary(t.text.Get(), pos)
}

func (t *TextArea) expandRangeForAtomicRanges(start, end int) (int, int) {
	if start > end {
		start, end = end, start
	}
	for {
		changed := false
		for _, span := range t.normalizedAtomicRanges() {
			if start < span.End && end > span.Start {
				if span.Start < start {
					start = span.Start
					changed = true
				}
				if span.End > end {
					end = span.End
					changed = true
				}
			}
		}
		if !changed {
			return start, end
		}
	}
}

func (t *TextArea) clearSelection() {
	t.selectionAnchor = -1
	t.selectionText = ""
	t.selectionCursor = 0
}

func (t *TextArea) deleteSelection() bool {
	start, end, ok := t.SelectionRange()
	if !ok {
		return false
	}
	start, end = t.expandRangeForAtomicRanges(start, end)
	runes := []rune(t.text.Get())
	updated := make([]rune, 0, len(runes)-(end-start))
	updated = append(updated, runes[:start]...)
	updated = append(updated, runes[end:]...)
	t.clearSelection()
	t.setText(string(updated))
	t.setCursorPosition(start)
	t.blink.Set(true)
	return true
}

func (t *TextArea) selectAll(KeyEvent) {
	t.SelectAll()
}

func (t *TextArea) moveBufferStart(KeyEvent) {
	t.clearSelection()
	t.setCursorPosition(0)
}

func (t *TextArea) moveBufferEnd(KeyEvent) {
	t.clearSelection()
	t.setCursorPosition(utf8.RuneCountInString(t.text.Get()))
}

func (t *TextArea) logicalLineStart(pos int) int {
	runes := []rune(t.text.Get())
	if pos > len(runes) {
		pos = len(runes)
	}
	for pos > 0 && runes[pos-1] != '\n' {
		pos--
	}
	return pos
}

func (t *TextArea) logicalLineEnd(pos int) int {
	runes := []rune(t.text.Get())
	if pos < 0 {
		pos = 0
	}
	for pos < len(runes) && runes[pos] != '\n' {
		pos++
	}
	return pos
}

func (t *TextArea) moveLogicalLineStart(KeyEvent) {
	t.clearSelection()
	t.setCursorPosition(t.logicalLineStart(t.clampCursorPos()))
}

func (t *TextArea) moveLogicalLineEnd(KeyEvent) {
	t.clearSelection()
	t.setCursorPosition(t.logicalLineEnd(t.clampCursorPos()))
}

type textareaWordClass uint8

const (
	textareaWhitespace textareaWordClass = iota
	textareaWord
	textareaPunctuation
)

func classifyTextAreaRune(r rune) textareaWordClass {
	if unicode.IsSpace(r) {
		return textareaWhitespace
	}
	if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_' {
		return textareaWord
	}
	return textareaPunctuation
}

func (t *TextArea) selectWordAt(pos int) {
	runes := []rune(t.text.Get())
	if pos < 0 {
		pos = 0
	}
	if pos >= len(runes) || runes[pos] == '\n' {
		t.clearSelection()
		t.setCursorPosition(pos)
		return
	}

	class := classifyTextAreaRune(runes[pos])
	start := pos
	for start > 0 && runes[start-1] != '\n' && classifyTextAreaRune(runes[start-1]) == class {
		start--
	}
	end := pos + 1
	for end < len(runes) && runes[end] != '\n' && classifyTextAreaRune(runes[end]) == class {
		end++
	}
	t.selectRange(start, end)
}

func (t *TextArea) selectRange(start, end int) {
	text := t.text.Get()
	maximum := utf8.RuneCountInString(text)
	if start < 0 {
		start = 0
	}
	if end > maximum {
		end = maximum
	}
	start = textAreaBoundaryAtOrBefore(text, start)
	end = textAreaBoundaryAtOrAfter(text, end)
	start, end = t.expandRangeForAtomicRanges(start, end)
	if start >= end {
		t.clearSelection()
		t.setCursorPosition(start)
		return
	}

	t.selectionAnchor = start
	t.selectionText = text
	t.setCursorPosition(end)
	t.selectionCursor = t.clampCursorPos()
}

// previousWordBoundary follows common readline/Option+Left behavior: skip
// whitespace first, then cross one contiguous word or punctuation run.
func previousWordBoundary(runes []rune, pos int) int {
	if pos > len(runes) {
		pos = len(runes)
	}
	for pos > 0 && classifyTextAreaRune(runes[pos-1]) == textareaWhitespace {
		pos--
	}
	if pos == 0 {
		return 0
	}
	class := classifyTextAreaRune(runes[pos-1])
	for pos > 0 && classifyTextAreaRune(runes[pos-1]) == class {
		pos--
	}
	return pos
}

// previousWhitespaceDelimitedWordBoundary implements readline's Ctrl+W
// (unix-word-rubout): whitespace separates words, while punctuation remains
// part of the token being removed.
func previousWhitespaceDelimitedWordBoundary(runes []rune, pos int) int {
	if pos > len(runes) {
		pos = len(runes)
	}
	for pos > 0 && unicode.IsSpace(runes[pos-1]) {
		pos--
	}
	for pos > 0 && !unicode.IsSpace(runes[pos-1]) {
		pos--
	}
	return pos
}

// nextWordBoundary crosses the current word/punctuation run, or skips leading
// whitespace and then crosses the following run.
func nextWordBoundary(runes []rune, pos int) int {
	if pos < 0 {
		pos = 0
	}
	for pos < len(runes) && classifyTextAreaRune(runes[pos]) == textareaWhitespace {
		pos++
	}
	if pos == len(runes) {
		return pos
	}
	class := classifyTextAreaRune(runes[pos])
	for pos < len(runes) && classifyTextAreaRune(runes[pos]) == class {
		pos++
	}
	return pos
}

func (t *TextArea) moveWordBackward(KeyEvent) {
	if start, _, ok := t.SelectionRange(); ok {
		t.clearSelection()
		t.setCursorPosition(start)
		return
	}
	t.clearSelection()
	runes := []rune(t.text.Get())
	t.setCursorPosition(t.atomicBoundaryAtOrBefore(previousWordBoundary(runes, t.clampCursorPos())))
}

func (t *TextArea) moveWordForward(KeyEvent) {
	if _, end, ok := t.SelectionRange(); ok {
		t.clearSelection()
		t.setCursorPosition(end)
		return
	}
	t.clearSelection()
	runes := []rune(t.text.Get())
	t.setCursorPosition(t.atomicBoundaryAtOrAfter(nextWordBoundary(runes, t.clampCursorPos())))
}

func (t *TextArea) deleteRange(start, end int) {
	text := t.text.Get()
	start = textAreaBoundaryAtOrBefore(text, start)
	end = textAreaBoundaryAtOrAfter(text, end)
	start, end = t.expandRangeForAtomicRanges(start, end)
	runes := []rune(t.text.Get())
	if start < 0 {
		start = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	if start >= end {
		return
	}
	updated := make([]rune, 0, len(runes)-(end-start))
	updated = append(updated, runes[:start]...)
	updated = append(updated, runes[end:]...)
	t.clearSelection()
	t.setText(string(updated))
	t.setCursorPosition(start)
	t.blink.Set(true)
}

func (t *TextArea) deleteWordBackward(KeyEvent) {
	if t.deleteSelection() {
		return
	}
	runes := []rune(t.text.Get())
	end := t.clampCursorPos()
	t.deleteRange(previousWordBoundary(runes, end), end)
}

func (t *TextArea) deleteWhitespaceDelimitedWordBackward(KeyEvent) {
	if t.deleteSelection() {
		return
	}
	runes := []rune(t.text.Get())
	end := t.clampCursorPos()
	t.deleteRange(previousWhitespaceDelimitedWordBoundary(runes, end), end)
}

func (t *TextArea) deleteWordForward(KeyEvent) {
	if t.deleteSelection() {
		return
	}
	runes := []rune(t.text.Get())
	start := t.clampCursorPos()
	t.deleteRange(start, nextWordBoundary(runes, start))
}

func (t *TextArea) deleteToLogicalLineStart(KeyEvent) {
	if t.deleteSelection() {
		return
	}
	end := t.clampCursorPos()
	t.deleteRange(t.logicalLineStart(end), end)
}

func (t *TextArea) deleteToLogicalLineEnd(KeyEvent) {
	if t.deleteSelection() {
		return
	}
	start := t.clampCursorPos()
	end := t.logicalLineEnd(start)
	runes := []rune(t.text.Get())
	if start == end && end < len(runes) && runes[end] == '\n' {
		end++
	}
	t.deleteRange(start, end)
}

func (t *TextArea) beginSelection() {
	if _, _, ok := t.SelectionRange(); !ok && t.selectionAnchor >= 0 {
		t.clearSelection()
	}
	if t.selectionAnchor < 0 {
		t.selectionAnchor = t.clampCursorPos()
		t.selectionText = t.text.Get()
		t.selectionCursor = t.clampCursorPos()
	}
}

func (t *TextArea) selectTo(pos int) {
	t.beginSelection()
	t.setCursorPosition(pos)
	t.selectionCursor = t.clampCursorPos()
	if t.selectionCursor == t.selectionAnchor {
		t.clearSelection()
	}
}

func (t *TextArea) selectLeft(KeyEvent) {
	t.selectTo(t.previousCursorBoundary(t.clampCursorPos()))
}

func (t *TextArea) selectRight(KeyEvent) {
	t.selectTo(t.nextCursorBoundary(t.clampCursorPos()))
}

func (t *TextArea) selectUp(KeyEvent) {
	t.moveVerticallySelecting(-1)
}

func (t *TextArea) selectDown(KeyEvent) {
	t.moveVerticallySelecting(1)
}

func (t *TextArea) moveVerticallySelecting(delta int) {
	lines := t.wrapText()
	row, col := t.cursorRowCol(lines)
	target := row + delta
	if target < 0 || target >= len(lines) {
		return
	}
	cellColumn := textAreaCellColumn(lines[row], col)
	col = textAreaRuneColumn(lines[target], cellColumn)
	t.selectTo(t.posFromRowCol(lines, target, col))
}

func (t *TextArea) selectHome(KeyEvent) {
	lines := t.wrapText()
	row, _ := t.cursorRowCol(lines)
	t.selectTo(t.posFromRowCol(lines, row, 0))
}

func (t *TextArea) selectEnd(KeyEvent) {
	lines := t.wrapText()
	row, _ := t.cursorRowCol(lines)
	t.selectTo(t.posFromRowCol(lines, row, utf8.RuneCountInString(lines[row])))
}

func (t *TextArea) selectWordBackward(KeyEvent) {
	t.selectTo(t.atomicBoundaryAtOrBefore(previousWordBoundary([]rune(t.text.Get()), t.clampCursorPos())))
}

func (t *TextArea) selectWordForward(KeyEvent) {
	t.selectTo(t.atomicBoundaryAtOrAfter(nextWordBoundary([]rune(t.text.Get()), t.clampCursorPos())))
}

func (t *TextArea) selectLogicalLineStart(KeyEvent) {
	t.selectTo(t.logicalLineStart(t.clampCursorPos()))
}

func (t *TextArea) selectLogicalLineEnd(KeyEvent) {
	t.selectTo(t.logicalLineEnd(t.clampCursorPos()))
}

func (t *TextArea) selectBufferStart(KeyEvent) {
	t.selectTo(0)
}

func (t *TextArea) selectBufferEnd(KeyEvent) {
	t.selectTo(utf8.RuneCountInString(t.text.Get()))
}

func (t *TextArea) visualLineRuneRange(lines []string, target int) (start, end int, newline bool) {
	text := []rune(t.text.Get())
	pos := 0
	for lineIndex, line := range lines {
		lineStart := pos
		lineEnd := lineStart + utf8.RuneCountInString(line)
		if lineIndex == target {
			return lineStart, lineEnd, lineEnd < len(text) && text[lineEnd] == '\n'
		}
		pos = lineEnd
		if pos < len(text) && text[pos] == '\n' {
			pos++
		}
	}
	return len(text), len(text), false
}

func appendTextAreaSpan(spans []StyledSpan, text string, style Style) []StyledSpan {
	if text == "" {
		return spans
	}
	if len(spans) > 0 && spans[len(spans)-1].Style.Equal(style) {
		spans[len(spans)-1].Text += text
		return spans
	}
	return append(spans, StyledSpan{Text: text, Style: style})
}

// lineWithSelection renders a selected input line using rich spans so the
// selection remains visible without embedding terminal escape sequences in
// the text model.
func (t *TextArea) lineWithSelection(lineIndex int) []StyledSpan {
	lines := t.wrapText()
	if lineIndex < 0 || lineIndex >= len(lines) {
		return []StyledSpan{{Text: " ", Style: t.textStyle}}
	}
	selectionStart, selectionEnd, _ := t.SelectionRange()
	lineStart, lineEnd, hasNewline := t.visualLineRuneRange(lines, lineIndex)
	lineRunes := []rune(lines[lineIndex])
	selectedStyle := t.textStyle
	selectedStyle.Attrs ^= AttrReverse
	spans := make([]StyledSpan, 0, 4)

	for offset := 0; offset < len(lineRunes); offset++ {
		absolute := lineStart + offset
		style := t.textStyle
		if absolute >= selectionStart && absolute < selectionEnd {
			style = selectedStyle
		}
		spans = appendTextAreaSpan(spans, string(lineRunes[offset]), style)
	}

	if hasNewline && lineEnd >= selectionStart && lineEnd < selectionEnd {
		spans = appendTextAreaSpan(spans, " ", selectedStyle)
	}
	if len(spans) == 0 {
		spans = append(spans, StyledSpan{Text: " ", Style: t.textStyle})
	}
	return spans
}
