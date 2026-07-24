package tui

// TextAreaAtomicRange is a half-open rune range that the editor treats as one
// indivisible item. Cursor movement skips across it, and any deletion touching
// it removes the complete range.
type TextAreaAtomicRange struct {
	Start int
	End   int
}

// TextAreaOption configures a TextArea.
type TextAreaOption func(*TextArea)

// --- Sizing Options ---

// WithTextAreaWidth sets the text area width in characters.
func WithTextAreaWidth(cells int) TextAreaOption {
	return func(t *TextArea) {
		t.width = cells
	}
}

// WithTextAreaMaxHeight sets the maximum height in rows (0 = unlimited).
func WithTextAreaMaxHeight(rows int) TextAreaOption {
	return func(t *TextArea) {
		t.maxHeight = rows
	}
}

// --- Visual Options ---

// WithTextAreaBorder sets the border style.
func WithTextAreaBorder(b BorderStyle) TextAreaOption {
	return func(t *TextArea) {
		t.border = b
	}
}

// WithTextAreaTextStyle sets the text style.
func WithTextAreaTextStyle(s Style) TextAreaOption {
	return func(t *TextArea) {
		t.textStyle = s
	}
}

// WithTextAreaPlaceholder sets placeholder text shown when empty and unfocused.
func WithTextAreaPlaceholder(text string) TextAreaOption {
	return func(t *TextArea) {
		t.placeholder = text
	}
}

// WithTextAreaPlaceholderStyle sets the placeholder text style (defaults to dim).
func WithTextAreaPlaceholderStyle(s Style) TextAreaOption {
	return func(t *TextArea) {
		t.placeholderStyle = s
	}
}

// WithTextAreaCursor sets the cursor character (defaults to '▌').
func WithTextAreaCursor(r rune) TextAreaOption {
	return func(t *TextArea) {
		t.cursorRune = r
	}
}

// WithTextAreaFocusColor sets the border color when focused.
func WithTextAreaFocusColor(c Color) TextAreaOption {
	return func(t *TextArea) {
		t.focusColor = &c
	}
}

// WithTextAreaBorderGradient sets a gradient for the border color when unfocused.
func WithTextAreaBorderGradient(g Gradient) TextAreaOption {
	return func(t *TextArea) {
		t.borderGradient = &g
	}
}

// WithTextAreaFocusGradient sets a gradient for the border color when focused.
// Takes priority over focusColor when set.
func WithTextAreaFocusGradient(g Gradient) TextAreaOption {
	return func(t *TextArea) {
		t.focusGradient = &g
	}
}

// --- Behavior Options ---

// WithTextAreaSubmitKey sets the key that triggers submit.
// Default is KeyEnter (Enter submits, Ctrl+J inserts newline).
// For long-form text, use a different key (e.g. a function key) so Enter inserts newline.
func WithTextAreaSubmitKey(k Key) TextAreaOption {
	return func(t *TextArea) {
		t.submitKey = k
	}
}

// WithTextAreaValue binds the TextArea to an external State for its text content.
// The TextArea reads from and writes to this state directly, enabling reactive
// two-way binding between the TextArea and the parent component.
func WithTextAreaValue(state *State[string]) TextAreaOption {
	return func(t *TextArea) {
		t.text = state
		if !t.cursorExternallyBound {
			t.cursorPos = NewState(len([]rune(state.Get())))
		}
	}
}

// WithTextAreaCursorPosition binds the TextArea cursor to an external State.
// The state stores a rune offset, matching CursorPosition and
// SetCursorPosition. It is useful when the parent component must persist the
// complete editor state rather than only its text value.
func WithTextAreaCursorPosition(state *State[int]) TextAreaOption {
	return func(t *TextArea) {
		if state == nil {
			return
		}
		t.cursorPos = state
		t.cursorExternallyBound = true
	}
}

// WithTextAreaAutoFocus sets whether the text area should automatically
// receive focus when the element tree is first applied.
func WithTextAreaAutoFocus(auto bool) TextAreaOption {
	return func(t *TextArea) {
		t.autoFocus = auto
	}
}

// WithTextAreaOnSubmit sets the callback called when the submit key is pressed.
func WithTextAreaOnSubmit(fn func(string)) TextAreaOption {
	return func(t *TextArea) {
		t.onSubmit = fn
	}
}

// WithTextAreaAtomicRanges supplies the current indivisible ranges for the
// editor text. The callback is evaluated on demand so ranges can follow edits.
func WithTextAreaAtomicRanges(fn func(string) []TextAreaAtomicRange) TextAreaOption {
	return func(t *TextArea) {
		t.atomicRanges = fn
	}
}

// WithTextAreaOnTextChange runs synchronously after the editor changes text.
func WithTextAreaOnTextChange(fn func(string)) TextAreaOption {
	return func(t *TextArea) {
		t.onTextChange = fn
	}
}
