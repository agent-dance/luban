package tui

import (
	"unicode/utf8"

	gtui "github.com/grindlemire/go-tui"
)

var _ gtui.PasteListener = (*slashAwareTextArea)(nil)

// slashAwareTextArea intercepts focused navigation keys before go-tui's
// default TextArea handlers consume them. This is necessary because go-tui
// dispatches focused stop handlers before preemptive parent handlers.
type slashAwareTextArea struct {
	*gtui.TextArea

	hasSlashSuggestions func() bool
	moveSlashSelection  func(delta int)
	executeSlash        func()
	dismissSlash        func()
	handleHistoryUp     func() bool
	handleHistoryDown   func() bool
	hasPickerOverlay    func() bool // when true, KeyMap returns nil to let preempt modal handlers through
	handlePaste         func(text string)
	handleOverlayPaste  func(text string) bool
}

func newSlashAwareTextArea(
	hasSlashSuggestions func() bool,
	moveSlashSelection func(delta int),
	executeSlash func(),
	dismissSlash func(),
	handleHistoryUp func() bool,
	handleHistoryDown func() bool,
	hasPickerOverlay func() bool,
	handlePaste func(text string),
	opts ...gtui.TextAreaOption,
) *slashAwareTextArea {
	return &slashAwareTextArea{
		TextArea:            gtui.NewTextArea(opts...),
		hasSlashSuggestions: hasSlashSuggestions,
		moveSlashSelection:  moveSlashSelection,
		executeSlash:        executeSlash,
		dismissSlash:        dismissSlash,
		handleHistoryUp:     handleHistoryUp,
		handleHistoryDown:   handleHistoryDown,
		hasPickerOverlay:    hasPickerOverlay,
		handlePaste:         handlePaste,
	}
}

func (t *slashAwareTextArea) HandleEvent(e gtui.Event) bool {
	if pe, ok := e.(gtui.PasteEvent); ok {
		return t.HandlePaste(pe)
	}

	ke, ok := e.(gtui.KeyEvent)
	if !ok {
		return false
	}

	for _, binding := range t.KeyMap() {
		if slashAwareKeyMatches(binding.Pattern, ke) {
			binding.Handler(ke)
			return binding.Stop
		}
	}
	return false
}

func (t *slashAwareTextArea) HandlePaste(pe gtui.PasteEvent) bool {
	if t.hasPickerOverlay != nil && t.hasPickerOverlay() {
		if t.handleOverlayPaste != nil {
			return t.handleOverlayPaste(pe.Text)
		}
		return false
	}
	if t.handlePaste != nil {
		t.handlePaste(pe.Text)
	} else {
		t.TextArea.InsertText(pe.Text)
	}
	return true
}

func (t *slashAwareTextArea) KeyMap() gtui.KeyMap {
	// When a modal overlay is open, return nil
	// so that focused stop handlers don't consume keys in the priority pass
	// before the preemptive modal handlers can fire.
	if t.hasPickerOverlay != nil && t.hasPickerOverlay() {
		return nil
	}

	base := t.TextArea.KeyMap()
	moveUp := func(ke gtui.KeyEvent) {
		if t.hasSlashSuggestions != nil && t.hasSlashSuggestions() {
			t.moveSlashSelection(-1)
			return
		}
		if t.TextArea.CursorLine() == 0 {
			if t.TextArea.CursorPosition() > 0 {
				t.TextArea.SetCursorPosition(0)
				return
			}
			if t.handleHistoryUp != nil && t.handleHistoryUp() {
				return
			}
		}
		t.dispatchBaseKey(ke)
	}
	moveDown := func(ke gtui.KeyEvent) {
		if t.hasSlashSuggestions != nil && t.hasSlashSuggestions() {
			t.moveSlashSelection(1)
			return
		}
		if t.TextArea.CursorLine() == t.TextArea.LineCount()-1 {
			end := utf8.RuneCountInString(t.TextArea.Text())
			if t.TextArea.CursorPosition() < end {
				t.TextArea.SetCursorPosition(end)
				return
			}
			if t.handleHistoryDown != nil && t.handleHistoryDown() {
				return
			}
		}
		t.dispatchBaseKey(ke)
	}
	km := gtui.KeyMap{
		gtui.OnFocused(gtui.KeyUp, moveUp),
		gtui.OnFocused(gtui.KeyCtrlP, moveUp),
		gtui.OnFocused(gtui.KeyDown, moveDown),
		gtui.OnFocused(gtui.KeyCtrlN, moveDown),
		gtui.OnFocused(gtui.KeyEnter, func(ke gtui.KeyEvent) {
			if t.hasSlashSuggestions != nil && t.hasSlashSuggestions() {
				t.executeSlash()
				return
			}
			t.dispatchBaseKey(ke)
		}),
		gtui.OnFocused(gtui.KeyEscape, func(ke gtui.KeyEvent) {
			if t.hasSlashSuggestions != nil && t.hasSlashSuggestions() {
				t.dismissSlash()
				return
			}
			t.dispatchBaseKey(ke)
		}),
	}

	for _, binding := range base {
		if slashAwareShouldSkipBinding(binding.Pattern) {
			continue
		}
		km = append(km, binding)
	}

	return km
}

func (t *slashAwareTextArea) dispatchBaseKey(ke gtui.KeyEvent) {
	for _, binding := range t.TextArea.KeyMap() {
		if !slashAwareShouldSkipBinding(binding.Pattern) && slashAwareKeyMatches(binding.Pattern, ke) {
			binding.Handler(ke)
			return
		}
	}
}

func slashAwareShouldSkipBinding(pattern gtui.KeyPattern) bool {
	// Shift+Up/Down remain product-level transcript scrolling shortcuts in the
	// composer. Generic TextAreas still support vertical selection.
	if pattern.FocusRequired && pattern.Mod == gtui.ModShift &&
		(pattern.Key == gtui.KeyUp || pattern.Key == gtui.KeyDown) {
		return true
	}
	return pattern.FocusRequired &&
		pattern.Mod == 0 &&
		pattern.ExcludeMods == 0 &&
		!pattern.AnyKey &&
		!pattern.AnyRune &&
		pattern.Rune == 0 &&
		(pattern.Key == gtui.KeyUp ||
			pattern.Key == gtui.KeyDown ||
			pattern.Key == gtui.KeyEnter ||
			pattern.Key == gtui.KeyEscape)
}

func slashAwareKeyMatches(pattern gtui.KeyPattern, ke gtui.KeyEvent) bool {
	if pattern.AnyKey {
		return true
	}
	if pattern.ExcludeMods != 0 && ke.Mod&pattern.ExcludeMods != 0 {
		return false
	}
	if pattern.Mod != 0 && ke.Mod != pattern.Mod {
		return false
	}
	if pattern.AnyRune && ke.Key == gtui.KeyRune {
		return true
	}
	if pattern.Rune != 0 && ke.Key == gtui.KeyRune && ke.Rune == pattern.Rune {
		return true
	}
	if pattern.Key != 0 && ke.Key == pattern.Key {
		return true
	}
	return false
}
