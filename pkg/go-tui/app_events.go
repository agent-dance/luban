package tui

import "time"

// Dispatch routes a single event through go-tui's dispatch system.
// KeyEvent goes through the dispatch table, then falls through to the focus
// manager.
// PasteEvent is sent to the focused PasteListener component.
// MouseEvent is translated for inline mode, dispatched to MouseListener components,
// then hit-tested against elements.
// ResizeEvent updates buffer dimensions.
// UpdateEvent executes the queued closure.
// Returns true if the event was consumed.
func (a *App) Dispatch(event Event) bool {
	if queued, ok := event.(queuedInputEvent); ok {
		if queued.generation != a.inputGeneration.Load() {
			return true
		}
		event = queued.event
	}
	switch e := event.(type) {
	case UpdateEvent:
		if e.fn != nil {
			e.fn()
		}
		return true

	case KeyEvent:
		e.app = a
		if a.dispatchTable != nil {
			if a.dispatchTable.dispatch(e) {
				return true
			}
		}
		// Ctrl+Z fallback if not consumed.
		if e.Key == KeyRune && e.Rune == 'z' && e.Mod == ModCtrl {
			a.suspend()
			return true
		}
		return a.focus.Dispatch(e)

	case PasteEvent:
		if a.dispatchPasteToComponents(e) {
			return true
		}
		return a.focus.Dispatch(e)

	case MouseEvent:
		e.app = a
		// Inline mode: translate terminal-space Y to buffer-space Y
		if !a.inAlternateScreen && a.inlineHeight > 0 {
			e.Y -= a.inlineStartRow
			if e.Y < 0 || e.Y >= a.inlineHeight {
				return false
			}
		}
		// Component model: dispatch to MouseListener components first
		if a.dispatchMouseToComponents(e) {
			return true
		}
		// Element hit testing
		if a.root == nil {
			return false
		}
		if target := a.root.ElementAtPoint(e.X, e.Y); target != nil {
			return target.HandleEvent(e)
		}
		return false

	case ResizeEvent:
		if a.inAlternateScreen {
			a.buffer.Resize(e.Width, e.Height)
		} else if a.inlineHeight > 0 {
			a.syncInlineGeometryOnResize(e.Width, e.Height)
		} else {
			a.buffer.Resize(e.Width, e.Height)
		}
		if a.root != nil {
			a.root.MarkDirty()
		}
		a.needsFullRedraw = true
		return true
	}

	// Unknown event types fall through to the focus manager.
	return a.focus.Dispatch(event)
}

// dispatchPasteToComponents sends a paste to the focused component. Paste is
// not represented as individual key events because embedded newlines must not
// trigger submit bindings.
func (a *App) dispatchPasteToComponents(pe PasteEvent) bool {
	if a.rootComponent == nil || a.root == nil {
		return false
	}
	consumed := false
	walkComponents(a.rootComponent, a.root, func(comp Component) {
		if consumed {
			return
		}
		focused, ok := comp.(focusQuerier)
		if !ok || !focused.IsFocused() {
			return
		}
		if listener, ok := comp.(PasteListener); ok {
			consumed = listener.HandlePaste(pe)
		}
	})
	return consumed
}

// dispatchMouseToComponents walks the component tree and dispatches a mouse
// event to all MouseListener components. Returns true if any consumed it.
func (a *App) dispatchMouseToComponents(me MouseEvent) bool {
	if a.root == nil {
		return false
	}
	consumed := false
	walkComponents(a.rootComponent, a.root, func(comp Component) {
		if consumed {
			return
		}
		if ml, ok := comp.(MouseListener); ok {
			if ml.HandleMouse(me) {
				consumed = true
			}
		}
	})
	return consumed
}

// readInputEvents reads terminal input in a goroutine and queues events.
func (a *App) readInputEvents() {
	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		generation := a.inputGeneration.Load()
		event, ok := a.reader.PollEvent(a.inputLatency)
		if !ok {
			select {
			case <-a.stopCh:
				return
			case <-time.After(time.Millisecond):
			}
			continue
		}

		a.inputHandoffMu.RLock()
		if generation != a.inputGeneration.Load() || a.externalActive.Load() || a.selfSuspended.Load() || a.terminalSuspended.Load() || a.stopping.Load() {
			a.inputHandoffMu.RUnlock()
			continue
		}
		select {
		case a.inputEvents <- queuedInputEvent{event: event, generation: generation}:
			a.inputHandoffMu.RUnlock()
		case <-a.stopCh:
			a.inputHandoffMu.RUnlock()
			return
		default:
			// Bounded input queues must not block a terminal handoff initiated
			// by the event loop. Dropping overload input is safer than routing
			// a pre-handoff event into the post-external composer.
			a.inputHandoffMu.RUnlock()
		}
	}
}

func (a *App) syncInlineGeometryOnResize(width, termHeight int) {
	a.inlineStartRow = termHeight - a.inlineHeight
	if a.inlineStartRow < 0 {
		a.inlineStartRow = 0
	}
	if a.buffer.Width() == width {
		return
	}

	a.buffer.Resize(width, a.inlineHeight)
	a.invalidateInlineLayoutForWidthChange(a.inlineStartRow)
}
