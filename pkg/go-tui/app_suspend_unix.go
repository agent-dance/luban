//go:build !windows

package tui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// suspendTerminal tears down terminal state before process suspension.
// Must be called from the main event loop.
func (a *App) suspendTerminal() {
	if a.onSuspend != nil {
		a.onSuspend()
	}
	a.terminalMu.Lock()
	_ = a.suspendTerminalChecked()
	a.terminalMu.Unlock()
}

func (a *App) suspendTerminalChecked() error {
	a.disableMouseCapture(a.mouseCaptureEnabled())
	setBracketedPaste(a.terminal, false)

	a.terminal.ShowCursor()

	if a.inAlternateScreen {
		// Dynamic alternate screen overlay: exit overlay first, then
		// handle the underlying mode (inline or full-screen).
		a.terminal.ExitAltScreen()
		if a.savedInlineHeight > 0 {
			a.terminal.SetCursor(0, a.savedInlineStartRow)
			a.terminal.ClearToEnd()
		}
	} else if a.inlineHeight > 0 {
		// Inline mode: clear the widget area and position the cursor there.
		// The scrollback history above the widget is untouched. Shell job
		// control messages ("Stopped", "fg") appear where the widget was.
		// On resume, the widget redraws at the recalculated bottom position.
		a.terminal.SetCursor(0, a.inlineStartRow)
		a.terminal.ClearToEnd()
	} else {
		// Full-screen mode: exit alternate screen
		a.terminal.ExitAltScreen()
	}

	// Disable Kitty keyboard protocol (pop from stack)
	a.terminal.DisableKittyKeyboard()

	err := a.terminal.ExitRawMode()
	a.terminalSuspended.Store(true)
	return err
}

// resumeTerminal restores terminal state after process resumption.
// Must be called from the main event loop.
func (a *App) resumeTerminal() {
	a.terminalSuspended.Store(true)
	if !a.legacyKeyboard {
		a.kittyKeyboard = true
	}
	a.terminalMu.Lock()
	err := a.resumeTerminalChecked()
	a.terminalMu.Unlock()
	a.finishSuspendResume(err)
}

func (a *App) resumeTerminalChecked() error {
	if !a.terminalSuspended.Load() {
		return nil
	}
	if err := a.terminal.EnterRawMode(); err != nil {
		return err
	}

	// Re-enable Kitty keyboard protocol. We use EnableKittyKeyboard (push
	// without query) instead of NegotiateKittyKeyboard to avoid a stdin
	// query/response race: after SIGCONT the terminal may be slow to respond,
	// and a late response leaks onto stdin where the reader parses it as
	// keypresses (e.g., "[?1u" typed into a textarea). Since we already
	// negotiated successfully at startup, we know the terminal supports it.
	if a.kittyKeyboard {
		a.terminal.EnableKittyKeyboard()
	}

	if a.inAlternateScreen {
		// Dynamic alternate screen overlay: recalculate saved inline
		// geometry (if the underlying mode was inline), then re-enter
		// the overlay alt screen.
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
		// Inline mode: the shell printed job control messages while stopped.
		// Recalculate where the widget should be drawn.
		_, termHeight := a.terminal.Size()
		a.inlineStartRow = termHeight - a.inlineHeight
		if a.inlineStartRow < 0 {
			a.inlineStartRow = 0
		}
		// Reset style tracking: the terminal's SGR state is unknown after
		// going through cooked mode and shell interaction. Without this,
		// Flush may skip emitting style codes for cells whose style matches
		// the stale lastStyle, producing wrong colors on the first frame.
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

// suspend performs the full suspend sequence: tear down terminal, send SIGTSTP.
// Must be called from the main event loop (via events channel).
//
// We never register signal.Notify for SIGTSTP, so its disposition remains at
// the OS default (stop the process). signal.Reset after Notify doesn't reliably
// restore SIG_DFL in Go's runtime, so avoiding Notify entirely is the fix.
func (a *App) suspend() {
	a.externalMu.Lock()
	if a.externalActive.Load() || a.selfSuspended.Load() || a.stopping.Load() {
		a.externalMu.Unlock()
		return
	}
	pausable, canPause := a.reader.(PausableReader)
	if a.reader != nil && !canPause {
		a.externalMu.Unlock()
		return
	}
	a.selfSuspended.Store(true)
	a.selfResumeSignal.Store(true)
	a.inputHandoffMu.Lock()
	a.inputGeneration.Add(1)
	if canPause {
		pausable.Pause()
	}
	a.inputHandoffMu.Unlock()
	a.externalMu.Unlock()

	if a.onSuspend != nil {
		a.onSuspend()
	}

	a.externalMu.Lock()
	if a.stopping.Load() {
		a.selfSuspended.Store(false)
		a.selfResumeSignal.Store(false)
		a.externalMu.Unlock()
		return
	}
	a.terminalMu.Lock()
	if err := a.suspendTerminalChecked(); err != nil {
		a.selfSuspended.Store(false)
		a.selfResumeSignal.Store(false)
		recoveryErr := a.resumeTerminalChecked()
		a.terminalMu.Unlock()
		a.inputHandoffMu.Lock()
		if recoveryErr == nil && canPause && !a.stopping.Load() {
			a.inputGeneration.Add(1)
			pausable.Resume()
		}
		a.inputHandoffMu.Unlock()
		a.externalMu.Unlock()
		a.recordLifecycleError(fmt.Errorf("release terminal for suspend: %w", err))
		if recoveryErr != nil {
			a.recordLifecycleError(fmt.Errorf("recover terminal after failed suspend: %w", recoveryErr))
			a.Stop()
		}
		return
	}

	// Stop the process. Execution pauses here until SIGCONT.
	// SIGTSTP disposition is SIG_DFL since we never called signal.Notify for it.
	suspendErr := sendProcessSuspend()

	// Process has been resumed by SIGCONT.
	// Resume inline to avoid a race with the event queue.
	resumeErr := a.resumeTerminalChecked()
	a.terminalMu.Unlock()

	// The paired SIGCONT may be delivered before or after this goroutine
	// resumes; selfResumeSignal lets the handler consume either ordering.
	a.selfSuspended.Store(false)
	if suspendErr != nil {
		a.selfResumeSignal.Store(false)
	}
	a.inputHandoffMu.Lock()
	if resumeErr == nil && canPause && !a.stopping.Load() {
		a.inputGeneration.Add(1)
		pausable.Resume()
	}
	a.inputHandoffMu.Unlock()
	a.externalMu.Unlock()
	if suspendErr != nil {
		a.recordLifecycleError(fmt.Errorf("suspend process: %w", suspendErr))
	}
	a.finishSuspendResume(resumeErr)
}

func (a *App) finishSuspendResume(resumeErr error) {
	if resumeErr != nil {
		a.recordLifecycleError(fmt.Errorf("restore terminal after suspend: %w", resumeErr))
		a.Stop()
		return
	}
	if a.onResume != nil {
		a.onResume()
	}
}

// Suspend programmatically triggers a suspend (same as Ctrl+Z).
// Safe to call from any goroutine.
func (a *App) Suspend() {
	if a.externalActive.Load() || a.selfSuspended.Load() || a.stopping.Load() {
		return
	}
	select {
	case a.updates <- UpdateEvent{fn: func() { a.suspend() }}:
	case <-a.stopCh:
	}
}

// registerSuspendSignals consumes the SIGCONT paired with a self-suspend.
// Unpaired SIGCONT signals never mutate terminal state.
// Returns a cleanup function to call when the app stops.
func (a *App) registerSuspendSignals() func() {
	contCh := make(chan os.Signal, 1)
	signal.Notify(contCh, syscall.SIGCONT)

	go func() {
		for {
			select {
			case <-contCh:
				if a.selfResumeSignal.CompareAndSwap(true, false) {
					a.selfSuspended.Store(false)
					continue
				}
				// Use CompareAndSwap to avoid a race with suspend().
				// After SIGCONT, both this goroutine and the main goroutine
				// (in suspend()) resume simultaneously. If we used Load(),
				// suspend() might clear the flag before we check it, causing
				// a spurious double-resume. CAS atomically checks and clears,
				// so exactly one side wins.
				if a.selfSuspended.CompareAndSwap(true, false) {
					// Self-initiated suspend: suspend() calls
					// resumeTerminal() inline. Nothing to do here.
					continue
				}
				// External SIGTSTP cannot run terminal teardown before the
				// process stops, so there is no safe state to restore here.
				// Ignore its SIGCONT rather than guessing and corrupting raw state.
			case <-a.stopCh:
				return
			}
		}
	}()

	return func() {
		signal.Stop(contCh)
	}
}
