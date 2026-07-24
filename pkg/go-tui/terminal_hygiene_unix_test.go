//go:build !windows

package tui

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestMouseTrackingIncludesButtonMotionAndSymmetricDisable(t *testing.T) {
	builder := newEscBuilder(64)
	builder.EnableMouse()

	wantEnable := "\x1b[?1000h\x1b[?1002h\x1b[?1006h"
	if got := string(builder.Bytes()); got != wantEnable {
		t.Fatalf("EnableMouse() = %q, want %q", got, wantEnable)
	}

	builder.Reset()
	builder.DisableMouse()

	wantDisable := "\x1b[?1006l\x1b[?1002l\x1b[?1000l"
	if got := string(builder.Bytes()); got != wantDisable {
		t.Fatalf("DisableMouse() = %q, want %q", got, wantDisable)
	}
}

func TestMouseTrackingFollowsAppMouseConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name         string
		mouseEnabled bool
		wantEnable   bool
		wantDisable  bool
	}{
		{name: "enabled", mouseEnabled: true, wantEnable: true, wantDisable: true},
		{name: "disabled", mouseEnabled: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			term := newRecordingTerminal(80, 24)
			term.inRawMode = true
			term.inAltScreen = true
			term.mouseEnabled = tt.mouseEnabled
			term.cursorHidden = true

			app := newTerminalHygieneTestApp(term, &MockEventReader{})
			app.mouseEnabled = tt.mouseEnabled

			app.suspendTerminal()
			app.resumeTerminal()

			if got := containsString(term.calls, "EnableMouse"); got != tt.wantEnable {
				t.Fatalf("resume EnableMouse presence = %v, want %v; calls=%v", got, tt.wantEnable, term.calls)
			}
			if got := containsString(term.calls, "DisableMouse"); got != tt.wantDisable {
				t.Fatalf("suspend DisableMouse presence = %v, want %v; calls=%v", got, tt.wantDisable, term.calls)
			}
		})
	}
}

func TestTerminalTeardownUsesActualMouseCaptureNotMutablePreference(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen, term.mouseEnabled, term.cursorHidden = true, true, true, true
	app := newTerminalHygieneTestApp(term, &MockEventReader{})
	app.mouseEnabled = false
	app.mouseCaptured.Store(true)

	app.suspendTerminal()
	if term.mouseEnabled {
		t.Fatal("terminal mouse capture survived teardown after preference changed")
	}
	if !containsString(term.calls, "DisableMouse") {
		t.Fatalf("terminal teardown omitted DisableMouse: %v", term.calls)
	}
}

func TestTerminationSignalsRestoreTerminal(t *testing.T) {
	if os.Getenv("GO_TUI_TERMINATION_HELPER") == "1" {
		runTerminationSignalHelper(t)
		return
	}

	for _, sig := range []string{"TERM", "HUP"} {
		t.Run(sig, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestTerminationSignalsRestoreTerminal$")
			cmd.Env = append(os.Environ(),
				"GO_TUI_TERMINATION_HELPER=1",
				"GO_TUI_TERMINATION_SIGNAL="+sig,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s helper did not shut down cleanly: %v\n%s", sig, err, output)
			}
		})
	}
}

func runTerminationSignalHelper(t *testing.T) {
	sig := syscall.SIGTERM
	if os.Getenv("GO_TUI_TERMINATION_SIGNAL") == "HUP" {
		sig = syscall.SIGHUP
	}

	term := newRecordingTerminal(80, 24)
	term.inRawMode = true
	term.inAltScreen = true
	term.mouseEnabled = true
	term.cursorHidden = true

	app := newTerminalHygieneTestApp(term, newExclusiveReader())
	app.mouseEnabled = true
	if err := app.Open(); err != nil {
		t.Fatalf("Open() failed: %v", err)
	}

	if err := syscall.Kill(syscall.Getpid(), sig); err != nil {
		t.Fatalf("send %v: %v", sig, err)
	}

	select {
	case <-app.StopCh():
	case <-time.After(time.Second):
		t.Fatalf("signal %v did not stop app", sig)
	}

	if err := app.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	assertTerminalReleased(t, term)
}

func TestPanicPathRestoresTerminal(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode = true
	term.inAltScreen = true
	term.mouseEnabled = true
	term.cursorHidden = true

	app := newTerminalHygieneTestApp(term, &MockEventReader{})
	app.mouseEnabled = true

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		defer app.Close()
		panic("test panic")
	}()

	assertTerminalReleased(t, term)
}

type initialRenderPanicComponent struct{}

func (initialRenderPanicComponent) Render(*App) *Element { panic("initial render panic") }

func TestOpenInitialRenderPanicRestoresTerminal(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode = true
	term.inAltScreen = true
	term.mouseEnabled = true
	term.cursorHidden = true
	app := newTerminalHygieneTestApp(term, &MockEventReader{})
	app.mouseEnabled = true
	app.inAlternateScreen = true
	app.rootComponent = initialRenderPanicComponent{}

	func() {
		defer func() {
			if got := recover(); got != "initial render panic" {
				t.Fatalf("Open() panic = %v, want initial render panic", got)
			}
		}()
		_ = app.Open()
	}()

	assertTerminalReleased(t, term)
}

func TestPendingRootPanicDuringConstructionRestoresTerminal(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen, term.mouseEnabled, term.cursorHidden = true, true, true, true
	app := newTerminalHygieneTestApp(term, &MockEventReader{})
	app.mouseEnabled = true
	app.inAlternateScreen = true
	app.pendingRootApply = func(*App) { panic("pending root panic") }
	func() {
		defer func() {
			if got := recover(); got != "pending root panic" {
				t.Fatalf("panic = %v", got)
			}
		}()
		app.applyPendingRoot()
	}()
	assertTerminalReleased(t, term)
}

func TestRunExternalPausesInputAndRestoresTerminal(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode = true
	term.inAltScreen = true
	term.mouseEnabled = true
	term.cursorHidden = true

	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	app.mouseEnabled = true
	go app.readInputEvents()
	reader.waitUntilPolling(t)

	wantErr := errors.New("editor exited unsuccessfully")
	runner, ok := any(app).(interface {
		RunExternal(func() error) error
	})
	if !ok {
		t.Fatal("App must implement RunExternal(func() error) error")
	}
	err := runner.RunExternal(func() error {
		if !reader.paused.Load() {
			t.Fatal("external process started before stdin reader was paused")
		}
		if reader.polling.Load() {
			t.Fatal("external process overlaps an active stdin PollEvent")
		}
		assertTerminalReleased(t, term)
		calls := len(term.calls)
		app.RenderFull()
		if len(term.calls) != calls {
			t.Fatalf("render wrote to terminal while external process owned it: %v", term.calls[calls:])
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunExternal() error = %v, want %v", err, wantErr)
	}

	if reader.paused.Load() {
		t.Fatal("stdin reader remained paused after external process")
	}
	assertTerminalActive(t, term)
	if !app.needsFullRedraw {
		t.Fatal("external process return must force a full redraw")
	}
	app.Stop()
}

func TestRunExternalPanicStillRestoresTerminal(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode = true
	term.inAltScreen = true
	term.mouseEnabled = true
	term.cursorHidden = true

	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	app.mouseEnabled = true
	runner, ok := any(app).(interface {
		RunExternal(func() error) error
	})
	if !ok {
		t.Fatal("App must implement RunExternal(func() error) error")
	}

	func() {
		defer func() {
			if got := recover(); got != "editor panic" {
				t.Fatalf("panic = %v, want editor panic", got)
			}
		}()
		_ = runner.RunExternal(func() error {
			panic("editor panic")
		})
	}()

	if reader.paused.Load() {
		t.Fatal("stdin reader remained paused after panic")
	}
	assertTerminalActive(t, term)
}

type exitRawErrorTerminal struct {
	*recordingTerminal
	fail bool
}

func (t *exitRawErrorTerminal) ExitRawMode() error {
	_ = t.recordingTerminal.ExitRawMode()
	if t.fail {
		t.fail = false
		return errors.New("exit raw failed")
	}
	return nil
}

func TestRunExternalTeardownErrorBestEffortRestoresTerminal(t *testing.T) {
	base := newRecordingTerminal(80, 24)
	base.inRawMode, base.inAltScreen, base.mouseEnabled, base.cursorHidden = true, true, true, true
	term := &exitRawErrorTerminal{recordingTerminal: base, fail: true}
	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	app.mouseEnabled = true
	called := false
	err := app.RunExternal(func() error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "exit raw failed") {
		t.Fatalf("RunExternal error = %v", err)
	}
	if called {
		t.Fatal("external process ran after terminal teardown failed")
	}
	if reader.paused.Load() || app.externalActive.Load() {
		t.Fatal("teardown failure leaked external ownership")
	}
	assertTerminalActive(t, base)
}

func TestRunExternalAllowsAppAPIReentryWithoutTerminalDeadlock(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen, term.mouseEnabled, term.cursorHidden = true, true, true, true
	app := newTerminalHygieneTestApp(term, newExclusiveReader())
	app.mouseEnabled = true
	done := make(chan error, 1)
	go func() {
		done <- app.RunExternal(func() error {
			app.SetMouseEnabled(false)
			return nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunExternal deadlocked when fn called SetMouseEnabled")
	}
	if app.MouseEnabled() || term.mouseEnabled {
		t.Fatal("mouse configuration changed during external process was not applied on resume")
	}
}

func TestRunExternalCallbackPanicReleasesOwnership(t *testing.T) {
	for _, callback := range []string{"suspend", "resume"} {
		t.Run(callback, func(t *testing.T) {
			term := newRecordingTerminal(80, 24)
			term.inRawMode, term.inAltScreen, term.mouseEnabled, term.cursorHidden = true, true, true, true
			reader := newExclusiveReader()
			app := newTerminalHygieneTestApp(term, reader)
			app.mouseEnabled = true
			if callback == "suspend" {
				app.onSuspend = func() { panic("callback panic") }
			} else {
				app.onResume = func() { panic("callback panic") }
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("expected callback panic")
					}
				}()
				_ = app.RunExternal(func() error { return nil })
			}()
			if reader.paused.Load() || app.externalActive.Load() {
				t.Fatal("callback panic leaked external ownership")
			}
			if callback == "resume" {
				assertTerminalActive(t, term)
			}
		})
	}
}

type blockingFlushTerminal struct {
	*recordingTerminal
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (t *blockingFlushTerminal) Flush(changes []CellChange) {
	t.once.Do(func() { close(t.entered) })
	<-t.release
	t.recordingTerminal.Flush(changes)
}

func TestRunExternalWaitsForInFlightRenderOwnership(t *testing.T) {
	base := newRecordingTerminal(80, 24)
	base.inRawMode, base.inAltScreen = true, true
	term := &blockingFlushTerminal{recordingTerminal: base, entered: make(chan struct{}), release: make(chan struct{})}
	app := newTerminalHygieneTestApp(term, nil)
	app.root = New(WithText("render"))
	app.needsFullRedraw = true
	renderDone := make(chan struct{})
	go func() { app.RenderFull(); close(renderDone) }()
	select {
	case <-term.entered:
	case <-time.After(time.Second):
		t.Fatal("render did not enter terminal flush")
	}
	externalEntered := make(chan struct{})
	externalDone := make(chan error, 1)
	go func() {
		externalDone <- app.RunExternal(func() error { close(externalEntered); return nil })
	}()
	select {
	case <-externalEntered:
		t.Fatal("external process started while render still owned terminal")
	case <-time.After(20 * time.Millisecond):
	}
	close(term.release)
	select {
	case <-renderDone:
	case <-time.After(time.Second):
		t.Fatal("render did not finish")
	}
	select {
	case <-externalEntered:
	case <-time.After(time.Second):
		t.Fatal("external process did not start after render released terminal")
	}
	if err := <-externalDone; err != nil {
		t.Fatal(err)
	}
}

func TestRunExternalRejectsNonPausableReader(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	app := newTerminalHygieneTestApp(term, &MockEventReader{})
	called := false
	err := app.RunExternal(func() error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "pausable") {
		t.Fatalf("RunExternal error = %v", err)
	}
	if called {
		t.Fatal("external process ran without exclusive stdin support")
	}
}

func TestUnixReaderPauseWaitsForPollAndDoesNotStealResumedInput(t *testing.T) {
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readFile.Close()
	defer writeFile.Close()
	readerInterface, err := NewEventReader(readFile)
	if err != nil {
		t.Fatal(err)
	}
	reader := readerInterface.(*stdinReader)
	if err := reader.EnableInterrupt(); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	pollDone := make(chan struct{})
	go func() { _, _ = reader.PollEvent(InputLatencyBlocking); close(pollDone) }()
	time.Sleep(10 * time.Millisecond)
	reader.Pause()
	select {
	case <-pollDone:
	case <-time.After(time.Second):
		t.Fatal("Pause returned without active PollEvent exiting")
	}
	reader.Resume()
	if _, err := writeFile.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	event, ok := reader.PollEvent(time.Second)
	key, keyOK := event.(KeyEvent)
	if !ok || !keyOK || key.Rune != 'x' {
		t.Fatalf("resumed input = %#v, ok=%v", event, ok)
	}
}

func TestSuspendOnSuspendCallbackCanCloseApp(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen = true, true
	app := newTerminalHygieneTestApp(term, newExclusiveReader())
	app.onSuspend = func() {
		if err := app.Close(); err != nil {
			t.Errorf("Close from onSuspend: %v", err)
		}
	}

	done := make(chan struct{})
	go func() {
		app.suspend()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("onSuspend callback deadlocked while closing app")
	}
	if !app.stopping.Load() || app.selfSuspended.Load() {
		t.Fatalf("suspend lifecycle after callback close: stopping=%v suspended=%v", app.stopping.Load(), app.selfSuspended.Load())
	}
}

func TestSuspendKeepsReaderPausedUntilRawModeIsRestored(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen = true, true
	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	originalSuspend := sendProcessSuspend
	defer func() { sendProcessSuspend = originalSuspend }()
	sendProcessSuspend = func() error {
		if !reader.paused.Load() {
			t.Error("reader resumed while process terminal was suspended")
		}
		if term.inRawMode {
			t.Error("terminal remained raw at process suspend boundary")
		}
		return nil
	}

	app.suspend()
	if reader.paused.Load() {
		t.Fatal("reader remained paused after raw-mode restoration")
	}
	if !term.inRawMode {
		t.Fatal("terminal raw mode was not restored")
	}
}

type failingSuspendRecoveryTerminal struct{ *recordingTerminal }

func (t *failingSuspendRecoveryTerminal) ExitRawMode() error {
	t.calls = append(t.calls, "ExitRawMode")
	t.inRawMode = false
	return errors.New("exit raw failed")
}

func (t *failingSuspendRecoveryTerminal) EnterRawMode() error {
	t.calls = append(t.calls, "EnterRawMode")
	return errors.New("enter raw failed")
}

func TestSuspendTeardownRecoveryFailureStopsWithReaderPaused(t *testing.T) {
	term := &failingSuspendRecoveryTerminal{recordingTerminal: newRecordingTerminal(80, 24)}
	term.inRawMode, term.inAltScreen = true, true
	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)

	app.suspend()
	if !app.stopping.Load() {
		t.Fatal("failed terminal recovery did not stop app")
	}
	if !reader.paused.Load() {
		t.Fatal("reader resumed against an unrecovered terminal")
	}
	if err := app.lifecycleError(); err == nil || !strings.Contains(err.Error(), "release terminal for suspend") {
		t.Fatalf("lifecycle error = %v", err)
	}
}

type handoffRaceReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	paused  atomic.Bool
	reads   atomic.Int32
}

func newHandoffRaceReader() *handoffRaceReader {
	return &handoffRaceReader{started: make(chan struct{}), release: make(chan struct{})}
}

func (r *handoffRaceReader) PollEvent(time.Duration) (Event, bool) {
	if r.reads.Add(1) > 1 {
		time.Sleep(50 * time.Millisecond)
		return nil, false
	}
	r.once.Do(func() { close(r.started) })
	<-r.release
	return KeyEvent{Key: KeyRune, Rune: 'x'}, true
}
func (r *handoffRaceReader) Close() error           { return nil }
func (r *handoffRaceReader) EnableInterrupt() error { return nil }
func (r *handoffRaceReader) Interrupt() error       { return nil }
func (r *handoffRaceReader) Pause() {
	r.paused.Store(true)
	select {
	case <-r.release:
	default:
		close(r.release)
	}
}
func (r *handoffRaceReader) Resume() { r.paused.Store(false) }

func TestRunExternalDropsEventReturnedAtInputHandoffBoundary(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen = true, true
	reader := newHandoffRaceReader()
	app := newTerminalHygieneTestApp(term, reader)
	go app.readInputEvents()
	<-reader.started
	if err := app.RunExternal(func() error {
		select {
		case event := <-app.inputEvents:
			t.Fatalf("pre-handoff event reached external lease: %#v", event)
		default:
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	select {
	case event := <-app.inputEvents:
		t.Fatalf("pre-handoff event leaked after resume: %#v", event)
	default:
	}
	app.Stop()
}

func TestRunExternalDropsInputAlreadyQueuedBeforeHandoffWithoutDroppingUpdates(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	term.inRawMode, term.inAltScreen = true, true
	reader := newExclusiveReader()
	app := newTerminalHygieneTestApp(term, reader)
	var keyCalls, updateCalls atomic.Int32
	app.globalKeyHandler = func(KeyEvent) bool {
		keyCalls.Add(1)
		return true
	}
	app.merged <- queuedInputEvent{event: KeyEvent{Key: KeyRune, Rune: 'x'}, generation: app.inputGeneration.Load()}
	app.merged <- UpdateEvent{fn: func() { updateCalls.Add(1) }}

	if err := app.RunExternal(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	app.DispatchEvents()
	if got := keyCalls.Load(); got != 0 {
		t.Fatalf("queued pre-handoff key was dispatched after resume: calls=%d", got)
	}
	if got := updateCalls.Load(); got != 1 {
		t.Fatalf("non-input update was dropped at handoff: calls=%d", got)
	}
	app.Stop()
}

func TestUnpairedSIGCONTDoesNotResumeActiveTerminal(t *testing.T) {
	term := newRecordingTerminal(80, 24)
	app := newTerminalHygieneTestApp(term, &MockEventReader{})
	cleanup := app.registerSuspendSignals()
	defer cleanup()
	before := len(term.calls)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if got := term.calls[before:]; len(got) != 0 {
		t.Fatalf("unpaired SIGCONT mutated active terminal: %v", got)
	}
	app.Stop()
}

func newTerminalHygieneTestApp(term Terminal, reader EventReader) *App {
	return &App{
		terminal:       term,
		reader:         reader,
		buffer:         NewBuffer(80, 24),
		focus:          newFocusManager(),
		inputEvents:    make(chan Event, 16),
		updates:        make(chan Event, 16),
		merged:         make(chan Event, 16),
		watcherQueue:   make(chan func(), 16),
		stopCh:         make(chan struct{}),
		mounts:         newMountState(),
		batch:          newBatchContext(),
		frameDuration:  time.Millisecond,
		inputLatency:   InputLatencyBlocking,
		eventQueueSize: 16,
	}
}

func assertTerminalReleased(t *testing.T, term *recordingTerminal) {
	t.Helper()
	if term.inRawMode {
		t.Error("terminal remains in raw mode")
	}
	if term.inAltScreen {
		t.Error("terminal remains in alternate screen")
	}
	if term.mouseEnabled {
		t.Error("terminal mouse tracking remains enabled")
	}
	if term.cursorHidden {
		t.Error("terminal cursor remains hidden")
	}
}

func assertTerminalActive(t *testing.T, term *recordingTerminal) {
	t.Helper()
	if !term.inRawMode {
		t.Error("terminal raw mode was not restored")
	}
	if !term.inAltScreen {
		t.Error("terminal alternate screen was not restored")
	}
	if !term.mouseEnabled {
		t.Error("terminal mouse tracking was not restored")
	}
	if !term.cursorHidden {
		t.Error("terminal hidden cursor state was not restored")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type exclusiveReader struct {
	paused  atomic.Bool
	polling atomic.Bool
	wake    chan struct{}
	entered chan struct{}
	once    sync.Once
}

var _ InterruptibleReader = (*exclusiveReader)(nil)
var _ PausableReader = (*exclusiveReader)(nil)

func newExclusiveReader() *exclusiveReader {
	return &exclusiveReader{
		wake:    make(chan struct{}, 1),
		entered: make(chan struct{}),
	}
}

func (r *exclusiveReader) PollEvent(time.Duration) (Event, bool) {
	if r.paused.Load() {
		return nil, false
	}
	r.polling.Store(true)
	r.once.Do(func() { close(r.entered) })
	<-r.wake
	r.polling.Store(false)
	return nil, false
}

func (r *exclusiveReader) Pause() {
	r.paused.Store(true)
	r.wake <- struct{}{}
	for r.polling.Load() {
		time.Sleep(time.Millisecond)
	}
}

func (r *exclusiveReader) Resume() { r.paused.Store(false) }

func (r *exclusiveReader) EnableInterrupt() error { return nil }

func (r *exclusiveReader) Interrupt() error {
	select {
	case r.wake <- struct{}{}:
	default:
	}
	return nil
}

func (r *exclusiveReader) Close() error { return nil }

func (r *exclusiveReader) waitUntilPolling(t *testing.T) {
	t.Helper()
	select {
	case <-r.entered:
	case <-time.After(time.Second):
		t.Fatal("stdin reader did not start polling")
	}
}
