//go:build windows

package tui

func (a *App) suspendTerminalChecked() error {
	a.disableMouseCapture(a.mouseCaptureEnabled())
	setBracketedPaste(a.terminal, false)
	a.terminal.ShowCursor()
	if a.inAlternateScreen {
		a.terminal.ExitAltScreen()
	} else if a.inlineHeight > 0 {
		a.terminal.SetCursor(0, a.inlineStartRow)
		a.terminal.ClearToEnd()
	} else {
		a.terminal.ExitAltScreen()
	}
	a.terminal.DisableKittyKeyboard()
	err := a.terminal.ExitRawMode()
	a.terminalSuspended.Store(true)
	return err
}

func (a *App) resumeTerminalChecked() error {
	if !a.terminalSuspended.Load() {
		return nil
	}
	if err := a.terminal.EnterRawMode(); err != nil {
		return err
	}
	if a.kittyKeyboard {
		a.terminal.EnableKittyKeyboard()
	}
	if a.inAlternateScreen {
		if a.savedInlineHeight > 0 {
			_, termHeight := a.terminal.Size()
			a.savedInlineStartRow = termHeight - a.savedInlineHeight
			if a.savedInlineStartRow < 0 {
				a.savedInlineStartRow = 0
			}
		}
		a.terminal.EnterAltScreen()
		a.terminal.Clear()
	} else if a.inlineHeight > 0 {
		_, termHeight := a.terminal.Size()
		a.inlineStartRow = termHeight - a.inlineHeight
		if a.inlineStartRow < 0 {
			a.inlineStartRow = 0
		}
		a.terminal.ResetStyle()
	} else {
		a.terminal.EnterAltScreen()
		a.terminal.Clear()
	}
	if !a.cursorVisible {
		a.terminal.HideCursor()
	}
	if a.mouseCaptureEnabled() {
		a.enableMouseCapture()
	}
	setBracketedPaste(a.terminal, true)
	a.needsFullRedraw = true
	a.MarkDirty()
	a.terminalSuspended.Store(false)
	return nil
}

// suspend is a no-op on Windows.
func (a *App) suspend() {}

// Suspend is a no-op on Windows.
func (a *App) Suspend() {}

// registerSuspendSignals is a no-op on Windows. Returns a no-op cleanup.
func (a *App) registerSuspendSignals() func() {
	return func() {}
}
