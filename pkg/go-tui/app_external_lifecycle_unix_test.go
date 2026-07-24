//go:build !windows

package tui

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type enterRawErrorTerminal struct {
	*recordingTerminal
	failEnter bool
}

func (t *enterRawErrorTerminal) EnterRawMode() error {
	t.calls = append(t.calls, "EnterRawMode")
	if t.failEnter {
		return errors.New("enter raw failed")
	}
	return t.MockTerminal.EnterRawMode()
}

func TestRunExternalResumeFailureRetainsExclusiveOwnership(t *testing.T) {
	base := newRecordingTerminal(80, 24)
	base.inRawMode, base.inAltScreen, base.mouseEnabled, base.cursorHidden = true, true, true, true
	term := &enterRawErrorTerminal{recordingTerminal: base, failEnter: true}
	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	app.mouseEnabled = true
	resumeCalled := false
	app.onResume = func() { resumeCalled = true }

	called := false
	err := app.RunExternal(func() error {
		called = true
		return nil
	})
	if !called {
		t.Fatal("external callback did not run")
	}
	if err == nil || !strings.Contains(err.Error(), "enter raw failed") {
		t.Fatalf("RunExternal() error = %v, want raw-mode restore failure", err)
	}
	if !app.externalActive.Load() {
		t.Fatal("failed terminal restore released external ownership")
	}
	if !reader.paused.Load() {
		t.Fatal("failed terminal restore resumed stdin reader")
	}
	if !app.terminalSuspended.Load() {
		t.Fatal("failed terminal restore marked the terminal active")
	}
	if resumeCalled {
		t.Fatal("failed terminal restore called onResume")
	}
	assertTerminalReleased(t, base)

	if err := app.Close(); err != nil {
		t.Fatalf("Close() after failed external restore: %v", err)
	}
	if app.externalActive.Load() {
		t.Fatal("Close() did not release failed external ownership")
	}
	if !reader.paused.Load() {
		t.Fatal("Close() resumed a reader after terminal restore failure")
	}
}

func TestRunExternalCallbackCanCloseApp(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen, term.mouseEnabled, term.cursorHidden = true, true, true, true
	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	app.mouseEnabled = true

	done := make(chan error, 1)
	go func() {
		done <- app.RunExternal(func() error {
			return app.Close()
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunExternal() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunExternal deadlocked when its callback called Close")
	}
	if !app.stopping.Load() {
		t.Fatal("callback Close() did not stop the app")
	}
	if app.externalActive.Load() {
		t.Fatal("callback Close() leaked external ownership")
	}
	if !reader.paused.Load() {
		t.Fatal("callback Close() resumed the closed input reader")
	}
	assertTerminalReleased(t, term)
}

func TestRunExternalOnSuspendCanCloseApp(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen, term.mouseEnabled, term.cursorHidden = true, true, true, true
	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	app.mouseEnabled = true
	var closeErr error
	app.onSuspend = func() {
		closeErr = app.Close()
	}

	callbackCalled := false
	done := make(chan error, 1)
	go func() {
		done <- app.RunExternal(func() error {
			callbackCalled = true
			return nil
		})
	}()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "app is stopping") {
			t.Fatalf("RunExternal() error = %v, want app stopping", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunExternal deadlocked when onSuspend called Close")
	}
	if closeErr != nil {
		t.Fatalf("onSuspend Close() error = %v", closeErr)
	}
	if callbackCalled {
		t.Fatal("external callback ran after onSuspend closed the app")
	}
	if !app.stopping.Load() {
		t.Fatal("onSuspend Close() did not stop the app")
	}
	if app.externalActive.Load() {
		t.Fatal("onSuspend Close() leaked external ownership")
	}
	if !reader.paused.Load() {
		t.Fatal("onSuspend Close() resumed the closed input reader")
	}
	assertTerminalReleased(t, term)
}

func TestSuspendRejectedDuringRunExternal(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen, term.mouseEnabled, term.cursorHidden = true, true, true, true
	app := newTerminalHygieneTestApp(term, newExclusiveReader())
	app.mouseEnabled = true

	externalEntered := make(chan struct{})
	releaseExternal := make(chan struct{})
	externalDone := make(chan error, 1)
	go func() {
		externalDone <- app.RunExternal(func() error {
			close(externalEntered)
			<-releaseExternal
			return nil
		})
	}()

	select {
	case <-externalEntered:
	case <-time.After(time.Second):
		t.Fatal("external callback did not start")
	}
	before := len(term.calls)
	app.Suspend()
	if got := len(app.updates); got != 0 {
		t.Fatalf("Suspend() queued %d event(s) while external process owned the terminal", got)
	}
	if got := len(term.calls); got != before {
		t.Fatalf("Suspend() touched terminal during external process: %v", term.calls[before:])
	}
	if app.selfSuspended.Load() {
		t.Fatal("Suspend() claimed self-suspension during external process")
	}

	close(releaseExternal)
	select {
	case err := <-externalDone:
		if err != nil {
			t.Fatalf("RunExternal() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunExternal did not finish")
	}
}

func TestSuspendResumeFailureStopsWithoutReusingTerminal(t *testing.T) {
	base := newRecordingTerminal(80, 24)
	term := &enterRawErrorTerminal{recordingTerminal: base, failEnter: true}
	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	app.terminalSuspended.Store(true)
	app.dirty.Store(true)
	resumeCalled := false
	app.onResume = func() { resumeCalled = true }
	go app.readInputEvents()
	reader.waitUntilPolling(t)

	app.terminalMu.Lock()
	resumeErr := app.resumeTerminalChecked()
	app.terminalMu.Unlock()
	app.finishSuspendResume(resumeErr)

	if !app.stopping.Load() {
		t.Fatal("failed suspend resume did not stop the app")
	}
	select {
	case <-app.stopCh:
	default:
		t.Fatal("failed suspend resume left the event loop running")
	}
	if !app.terminalSuspended.Load() {
		t.Fatal("failed suspend resume marked the terminal active")
	}
	if resumeCalled {
		t.Fatal("failed suspend resume called onResume")
	}
	deadline := time.Now().Add(time.Second)
	for reader.polling.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if reader.polling.Load() {
		t.Fatal("failed suspend resume left the stdin reader polling")
	}
	if base.inRawMode {
		t.Fatal("failed suspend resume left terminal in raw mode")
	}

	before := len(base.calls)
	app.Render()
	app.RenderFull()
	if got := base.calls[before:]; len(got) != 0 {
		t.Fatalf("render touched terminal after failed suspend resume: %v", got)
	}

	err := app.Close()
	if err == nil || !strings.Contains(err.Error(), "restore terminal after suspend: enter raw failed") {
		t.Fatalf("Close() error = %v, want suspend resume failure", err)
	}
}

func TestRunReturnsLifecycleFailure(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	wantErr := errors.New("terminal lifecycle failed")
	app.recordLifecycleError(wantErr)
	app.Stop()

	if err := app.Run(); !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
}
