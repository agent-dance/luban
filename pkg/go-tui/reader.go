//go:build !windows

package tui

import (
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// stdinReader implements EventReader for a real terminal.
type stdinReader struct {
	input      *os.File // retain caller-owned file so its finalizer cannot close fd during polling
	fd         int      // stdin file descriptor
	buf        []byte   // Read buffer for escape sequences
	partialBuf []byte   // Buffer for incomplete UTF-8 sequences
	pending    []Event  // Parsed events waiting to be returned

	// Interrupt mechanism for blocking mode
	interruptPipe    [2]int // [0]=read, [1]=write
	hasInterrupt     atomic.Bool
	interruptPending atomic.Bool
	interruptMu      sync.Mutex

	paused     atomic.Bool // When true, PollEvent returns immediately
	closed     atomic.Bool
	generation atomic.Uint64
	controlMu  sync.Mutex
	pollMu     sync.Mutex
}

// Ensure stdinReader implements InterruptibleReader and PausableReader.
var _ InterruptibleReader = (*stdinReader)(nil)
var _ PausableReader = (*stdinReader)(nil)

// NewEventReader creates an EventReader for the given terminal input.
// The terminal should already be in raw mode.
func NewEventReader(in *os.File) (EventReader, error) {
	r := &stdinReader{
		input: in,
		fd:    int(in.Fd()),
		buf:   make([]byte, 256),
	}
	return r, nil
}

// Pause causes PollEvent to return immediately without reading stdin.
// Interrupts any in-progress blocking read via the interrupt pipe.
// If EnableInterrupt has not been called, the in-progress PollEvent will
// unblock only on its next timeout cycle; Kitty negotiation may race
// with the stuck read until then.
func (r *stdinReader) Pause() {
	r.controlMu.Lock()
	defer r.controlMu.Unlock()

	r.paused.Store(true)
	r.generation.Add(1)
	if r.hasInterrupt.Load() {
		r.Interrupt()
	}
	r.pollMu.Lock()
	r.pending = nil
	r.partialBuf = nil
	// Reassert the pause after the active poll has exited so a concurrent
	// Resume cannot make Pause return while input polling is still enabled.
	r.paused.Store(true)
	r.pollMu.Unlock()
}

// Resume allows PollEvent to read stdin again.
func (r *stdinReader) Resume() {
	r.controlMu.Lock()
	defer r.controlMu.Unlock()
	if !r.closed.Load() {
		r.paused.Store(false)
	}
}

// PollEvent reads the next event with a timeout.
// Returns (event, true) if an event was read, or (nil, false) on timeout.
func (r *stdinReader) PollEvent(timeout time.Duration) (Event, bool) {
	r.pollMu.Lock()
	defer r.pollMu.Unlock()
	generation := r.generation.Load()
	if !r.pollGenerationCurrent(generation) {
		return nil, false
	}

	// Return pending events first
	if len(r.pending) > 0 {
		if !r.pollGenerationCurrent(generation) {
			return nil, false
		}
		ev := r.pending[0]
		r.pending = r.pending[1:]
		return ev, true
	}

	for {
		// A legacy Alt key is encoded as ESC followed by a printable byte. Those
		// bytes may arrive in separate reads, so give a buffered ESC a short
		// disambiguation window before publishing it as a standalone Escape key.
		pendingEscape := hasPendingEscapePrefix(r.partialBuf)
		pollTimeout := timeout
		if pendingEscape {
			pollTimeout = escapePollTimeout(timeout)
		}

		// Use select() with timeout for non-blocking stdin check.
		var ready bool
		var err error
		if r.hasInterrupt.Load() {
			var interrupted bool
			ready, interrupted, err = selectWithTimeoutAndInterrupt(r.fd, r.interruptPipe[0], pollTimeout)
			if interrupted {
				r.interruptPending.Store(false)
				return nil, false
			}
		} else {
			ready, err = selectWithTimeout(r.fd, pollTimeout)
		}

		if err != nil || !ready {
			if err == nil && pendingEscape && r.pollGenerationCurrent(generation) {
				r.partialBuf = nil
				return KeyEvent{Key: KeyEscape}, true
			}
			return nil, false
		}

		// Read available bytes.
		n, err := syscall.Read(r.fd, r.buf)
		if err != nil || n == 0 {
			return nil, false
		}
		if !r.pollGenerationCurrent(generation) {
			return nil, false
		}

		// Combine with any incomplete sequence from the previous read.
		data := r.buf[:n]
		if len(r.partialBuf) > 0 {
			data = append(r.partialBuf, data...)
			r.partialBuf = nil
		}

		events, remaining := parseInputWithRemainder(data)
		if len(remaining) > 0 {
			r.partialBuf = append([]byte(nil), remaining...)
		}
		r.pending = events
		if r.pollGenerationCurrent(generation) && len(r.pending) > 0 {
			ev := r.pending[0]
			r.pending = r.pending[1:]
			return ev, true
		}
		if !hasPendingEscapeSequence(r.partialBuf) {
			return nil, false
		}
		// Stay in this PollEvent while an escape sequence is incomplete. Lone
		// ESC uses the short ambiguity window above; CSI/Kitty/mouse sequences
		// retain the caller's timeout and are reassembled across reads.
	}
}

func (r *stdinReader) pollGenerationCurrent(generation uint64) bool {
	return !r.paused.Load() && !r.closed.Load() && r.generation.Load() == generation
}

// Close releases resources.
func (r *stdinReader) Close() error {
	if !r.closed.CompareAndSwap(false, true) {
		return nil
	}
	r.paused.Store(true)
	_ = r.signalInterrupt()
	r.pollMu.Lock()
	defer r.pollMu.Unlock()
	r.interruptMu.Lock()
	defer r.interruptMu.Unlock()
	if r.hasInterrupt.Swap(false) {
		syscall.Close(r.interruptPipe[0])
		syscall.Close(r.interruptPipe[1])
		r.interruptPending.Store(false)
	}
	r.input = nil
	return nil
}

// EnableInterrupt sets up the interrupt mechanism using a self-pipe.
// This allows Interrupt() to wake up a blocking PollEvent call.
func (r *stdinReader) EnableInterrupt() error {
	r.interruptMu.Lock()
	defer r.interruptMu.Unlock()
	if r.closed.Load() {
		return os.ErrClosed
	}
	if r.hasInterrupt.Load() {
		return nil // Already enabled
	}
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		return err
	}
	if err := syscall.SetNonblock(fds[1], true); err != nil {
		syscall.Close(fds[0])
		syscall.Close(fds[1])
		return err
	}
	r.interruptPipe = fds
	r.hasInterrupt.Store(true)
	return nil
}

// Interrupt wakes up a blocking PollEvent call by writing to the interrupt pipe.
func (r *stdinReader) Interrupt() error {
	if r.closed.Load() {
		return nil
	}
	return r.signalInterrupt()
}

// signalInterrupt keeps at most one wakeup pending. A wakeup is level-triggered:
// callers only need the active or next PollEvent to return, not one pipe byte per
// call. The non-blocking write is a final guard if the kernel pipe is already full.
func (r *stdinReader) signalInterrupt() error {
	if !r.hasInterrupt.Load() || !r.interruptPending.CompareAndSwap(false, true) {
		return nil
	}

	r.interruptMu.Lock()
	defer r.interruptMu.Unlock()
	if !r.hasInterrupt.Load() {
		r.interruptPending.Store(false)
		return nil
	}
	_, err := syscall.Write(r.interruptPipe[1], []byte{0})
	if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
		return nil
	}
	if err != nil {
		r.interruptPending.Store(false)
	}
	return err
}
