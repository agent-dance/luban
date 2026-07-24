//go:build windows

package tui

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
)

var cancelSynchronousIO = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")

const windowsReadBufferSize = 256

type windowsReadResult struct {
	data []byte
	err  error
}

// activeWindowsRead owns one blocking input operation. The worker is pinned to
// an OS thread so synchronous console and pipe reads can be cancelled reliably.
type activeWindowsRead struct {
	result      chan windowsReadResult
	done        chan struct{}
	threadReady chan struct{}
	thread      windows.Handle
	cancelMu    sync.Mutex
}

// stdinReader implements EventReader for Windows terminals.
type stdinReader struct {
	inputHandle windows.Handle
	partialBuf  []byte
	pending     []Event
	interruptC  chan struct{}
	readFn      func([]byte) (int, error)
	cancelHook  func()

	paused     atomic.Bool
	closed     atomic.Bool
	generation atomic.Uint64
	readMu     sync.Mutex
	controlMu  sync.Mutex
	activeMu   sync.Mutex
	active     *activeWindowsRead
}

var _ InterruptibleReader = (*stdinReader)(nil)
var _ PausableReader = (*stdinReader)(nil)

// NewEventReader creates an EventReader for the given terminal input.
func NewEventReader(in *os.File) (EventReader, error) {
	return &stdinReader{
		inputHandle: windows.Handle(in.Fd()),
		interruptC:  make(chan struct{}, 1),
		readFn:      in.Read,
	}, nil
}

// Pause establishes a new input generation and waits until the previous read
// owner has exited. Input parsed by an older generation is discarded.
func (r *stdinReader) Pause() {
	r.controlMu.Lock()
	defer r.controlMu.Unlock()

	r.paused.Store(true)
	r.generation.Add(1)
	r.cancelActiveRead()

	r.readMu.Lock()
	r.pending = nil
	r.partialBuf = nil
	r.drainInterrupt()
	// A concurrent Resume waits on controlMu, but keep the state transition
	// explicit at the point where no previous-generation poll remains.
	r.paused.Store(true)
	r.readMu.Unlock()
}

// Resume allows PollEvent to read stdin again unless the reader is closed.
func (r *stdinReader) Resume() {
	r.controlMu.Lock()
	defer r.controlMu.Unlock()
	if !r.closed.Load() {
		r.paused.Store(false)
	}
}

func (r *stdinReader) PollEvent(timeout time.Duration) (Event, bool) {
	r.readMu.Lock()
	defer r.readMu.Unlock()

	generation := r.generation.Load()
	if !r.pollGenerationCurrent(generation) {
		return nil, false
	}
	if len(r.pending) > 0 {
		if !r.pollGenerationCurrent(generation) {
			return nil, false
		}
		ev := r.pending[0]
		r.pending = r.pending[1:]
		return ev, true
	}
	for {
		pendingEscape := hasPendingEscapePrefix(r.partialBuf)
		pollTimeout := timeout
		if pendingEscape {
			pollTimeout = escapePollTimeout(timeout)
		}
		if pollTimeout == 0 {
			if pendingEscape {
				r.partialBuf = nil
				return KeyEvent{Key: KeyEscape}, true
			}
			return nil, false
		}

		read, ok := r.beginRead()
		if !ok {
			return nil, false
		}
		go r.runRead(read)

		result := r.waitRead(read, pollTimeout)
		r.finishRead(read)
		if !r.pollGenerationCurrent(generation) {
			return nil, false
		}
		if result.err != nil && len(result.data) == 0 {
			if pendingEscape {
				r.partialBuf = nil
				return KeyEvent{Key: KeyEscape}, true
			}
			return nil, false
		}
		if len(result.data) > 0 {
			r.parseRead(result.data)
		}
		if !r.pollGenerationCurrent(generation) {
			return nil, false
		}
		if len(r.pending) > 0 {
			ev := r.pending[0]
			r.pending = r.pending[1:]
			return ev, true
		}
		if !hasPendingEscapeSequence(r.partialBuf) {
			return nil, false
		}
	}
}

func (r *stdinReader) pollGenerationCurrent(generation uint64) bool {
	return !r.paused.Load() && !r.closed.Load() && r.generation.Load() == generation
}

func (r *stdinReader) beginRead() (*activeWindowsRead, bool) {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	if r.paused.Load() || r.closed.Load() {
		return nil, false
	}
	// Interrupt only applies to a read that is already being established.
	// A signal queued with no owner must not cancel a later generation.
	r.drainInterrupt()
	read := &activeWindowsRead{
		result:      make(chan windowsReadResult, 1),
		done:        make(chan struct{}),
		threadReady: make(chan struct{}),
	}
	r.active = read
	return read, true
}

func (r *stdinReader) runRead(read *activeWindowsRead) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	process := windows.CurrentProcess()
	err := windows.DuplicateHandle(
		process,
		windows.CurrentThread(),
		process,
		&read.thread,
		0,
		false,
		windows.DUPLICATE_SAME_ACCESS,
	)
	close(read.threadReady)
	if err != nil {
		read.result <- windowsReadResult{err: err}
		close(read.done)
		return
	}

	buffer := make([]byte, windowsReadBufferSize)
	n, err := r.readFn(buffer)
	read.result <- windowsReadResult{data: buffer[:n], err: err}
	close(read.done)
}

func (r *stdinReader) waitRead(read *activeWindowsRead, timeout time.Duration) windowsReadResult {
	if timeout < 0 {
		select {
		case result := <-read.result:
			return result
		case <-r.interruptC:
			r.cancelRead(read)
			return <-read.result
		}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-read.result:
		return result
	case <-r.interruptC:
		r.cancelRead(read)
		return <-read.result
	case <-timer.C:
		r.cancelRead(read)
		return <-read.result
	}
}

func (r *stdinReader) cancelActiveRead() {
	r.activeMu.Lock()
	read := r.active
	r.activeMu.Unlock()
	if read != nil {
		r.cancelRead(read)
	}
}

// cancelRead retries until the registered operation exits. This closes the
// cancel-before-submit gap: an early ERROR_NOT_FOUND does not imply that the
// worker cannot submit its read a moment later.
func (r *stdinReader) cancelRead(read *activeWindowsRead) {
	select {
	case <-read.threadReady:
	case <-read.done:
		return
	}
	if read.thread == 0 {
		<-read.done
		return
	}

	for {
		select {
		case <-read.done:
			return
		default:
		}

		read.cancelMu.Lock()
		// Console and blocking-pipe reads need thread cancellation; ConPTY and
		// other overlapped handles need operation cancellation. Cover both.
		_, _, _ = cancelSynchronousIO.Call(uintptr(read.thread))
		_ = windows.CancelIoEx(r.inputHandle, nil)
		if r.cancelHook != nil {
			r.cancelHook()
		}
		read.cancelMu.Unlock()

		time.Sleep(100 * time.Microsecond)
	}
}

func (r *stdinReader) finishRead(read *activeWindowsRead) {
	<-read.done
	read.cancelMu.Lock()
	if read.thread != 0 {
		_ = windows.CloseHandle(read.thread)
		read.thread = 0
	}
	read.cancelMu.Unlock()

	r.activeMu.Lock()
	if r.active == read {
		r.active = nil
	}
	r.activeMu.Unlock()
}

func (r *stdinReader) parseRead(read []byte) {
	data := read
	if len(r.partialBuf) > 0 {
		data = append(r.partialBuf, read...)
		r.partialBuf = nil
	}
	events, remaining := parseInputWithRemainder(data)
	if len(remaining) > 0 {
		r.partialBuf = append([]byte(nil), remaining...)
	}
	r.pending = events
}

func (r *stdinReader) Close() error {
	if !r.closed.CompareAndSwap(false, true) {
		return nil
	}

	r.controlMu.Lock()
	defer r.controlMu.Unlock()
	r.paused.Store(true)
	r.generation.Add(1)
	r.cancelActiveRead()
	r.readMu.Lock()
	r.pending = nil
	r.partialBuf = nil
	r.readMu.Unlock()
	return nil
}

func (r *stdinReader) EnableInterrupt() error { return nil }

func (r *stdinReader) Interrupt() error {
	select {
	case r.interruptC <- struct{}{}:
	default:
	}
	return nil
}

func (r *stdinReader) drainInterrupt() {
	for {
		select {
		case <-r.interruptC:
		default:
			return
		}
	}
}
