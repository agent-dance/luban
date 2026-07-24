package tui

import "fmt"

// RunExternal temporarily gives an external program exclusive ownership of
// stdin and the terminal. State is restored even if fn errors or panics.
func (a *App) RunExternal(fn func() error) (err error) {
	if fn == nil {
		return nil
	}
	if a.stopping.Load() {
		return fmt.Errorf("app is stopping")
	}
	if a.externalActive.Load() || a.selfSuspended.Load() {
		return fmt.Errorf("external process is already active")
	}
	a.externalMu.Lock()
	externalLocked := true
	pausable, canPause := a.reader.(PausableReader)
	if a.reader != nil && !canPause {
		a.externalMu.Unlock()
		return fmt.Errorf("external process requires a pausable event reader")
	}
	if a.stopping.Load() {
		a.externalMu.Unlock()
		return fmt.Errorf("app is stopping")
	}
	if a.externalActive.Load() || a.selfSuspended.Load() {
		a.externalMu.Unlock()
		return fmt.Errorf("external process is already active")
	}
	a.inputHandoffMu.Lock()
	a.inputGeneration.Add(1)
	a.externalActive.Store(true)
	if canPause {
		pausable.Pause()
	}
	a.inputHandoffMu.Unlock()
	suspended := false
	defer func() {
		if !externalLocked {
			a.externalMu.Lock()
			externalLocked = true
		}
		var resumeErr error
		stopping := a.stopping.Load()
		resumeAttempted := suspended && !stopping
		if resumeAttempted {
			a.terminalMu.Lock()
			resumeErr = a.resumeTerminalChecked()
			a.terminalMu.Unlock()
		}
		// A failed raw-mode restore leaves the app without safe terminal or
		// stdin ownership. Keep the external lease until Close performs final
		// cleanup instead of re-enabling input against a cooked terminal.
		if stopping || !suspended || resumeErr == nil {
			a.inputHandoffMu.Lock()
			a.inputGeneration.Add(1)
			a.externalActive.Store(false)
			if canPause && !stopping {
				pausable.Resume()
			}
			a.inputHandoffMu.Unlock()
		}
		a.externalMu.Unlock()
		externalLocked = false
		if resumeAttempted && resumeErr == nil && !a.stopping.Load() && a.onResume != nil {
			a.onResume()
		}
		if resumeErr != nil {
			if err == nil {
				err = fmt.Errorf("restore terminal after external process: %w", resumeErr)
			} else {
				err = fmt.Errorf("%w (restore terminal: %v)", err, resumeErr)
			}
		}
	}()

	a.externalMu.Unlock()
	externalLocked = false
	if a.onSuspend != nil {
		a.onSuspend()
	}
	a.externalMu.Lock()
	externalLocked = true
	if a.stopping.Load() {
		return fmt.Errorf("app is stopping")
	}
	a.terminalMu.Lock()
	suspendErr := a.suspendTerminalChecked()
	suspended = a.terminalSuspended.Load()
	a.terminalMu.Unlock()
	if suspendErr != nil {
		return fmt.Errorf("release terminal for external process: %w", suspendErr)
	}
	a.externalMu.Unlock()
	externalLocked = false
	return fn()
}
