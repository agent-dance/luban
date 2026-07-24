package main

import (
	"context"
	"os"
	"sync"
	"syscall"
)

// SignalHandler manages the two-tier signal handling:
//   - SIGINT during a query → cancel just that query
//   - SIGINT at the prompt (no query) → exit the program
//   - SIGTERM → always exit the program
type SignalHandler struct {
	globalCancel  context.CancelFunc
	mu            sync.Mutex
	queryCancelFn context.CancelFunc
}

// NewSignalHandler creates a signal handler that calls globalCancel for program exit.
func NewSignalHandler(globalCancel context.CancelFunc) *SignalHandler {
	return &SignalHandler{globalCancel: globalCancel}
}

// SetQueryCancel sets the cancel function for the currently running query.
func (s *SignalHandler) SetQueryCancel(fn context.CancelFunc) {
	s.mu.Lock()
	s.queryCancelFn = fn
	s.mu.Unlock()
}

// ClearQueryCancel clears the query cancel function (called when a query finishes).
func (s *SignalHandler) ClearQueryCancel() {
	s.mu.Lock()
	s.queryCancelFn = nil
	s.mu.Unlock()
}

// Listen processes OS signals. Run in a goroutine.
func (s *SignalHandler) Listen(sigCh <-chan os.Signal) {
	for sig := range sigCh {
		if sig == syscall.SIGTERM {
			s.globalCancel()
			return
		}
		s.mu.Lock()
		cancelFn := s.queryCancelFn
		s.mu.Unlock()
		if cancelFn != nil {
			cancelFn()
		} else {
			s.globalCancel()
		}
	}
}
