package app

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

	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	listenOnce      sync.Once
	done            chan struct{}
}

// NewSignalHandler creates a signal handler that calls globalCancel for program exit.
func NewSignalHandler(globalCancel context.CancelFunc) *SignalHandler {
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	return &SignalHandler{
		globalCancel:    globalCancel,
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
		done:            make(chan struct{}),
	}
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

// Start begins processing signals and binds the listener to ctx. StopAndWait is
// the matching owner-side teardown operation.
func (s *SignalHandler) Start(ctx context.Context, sigCh <-chan os.Signal) {
	if s == nil {
		return
	}
	go s.listen(ctx, sigCh)
}

// listen processes OS signals until ctx is cancelled, Stop is called,
// or sigCh is closed. A handler owns at most one listener.
func (s *SignalHandler) listen(ctx context.Context, sigCh <-chan os.Signal) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.listenOnce.Do(func() {
		defer close(s.done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.lifecycleDone():
				return
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				if sig == syscall.SIGTERM {
					s.cancelGlobal()
					return
				}
				s.mu.Lock()
				cancelFn := s.queryCancelFn
				s.mu.Unlock()
				if cancelFn != nil {
					cancelFn()
				} else {
					s.cancelGlobal()
				}
			}
		}
	})
}

func (s *SignalHandler) stop() {
	if s != nil && s.lifecycleCancel != nil {
		s.lifecycleCancel()
	}
}

func (s *SignalHandler) doneSignal() <-chan struct{} {
	if s == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	return s.done
}

func (s *SignalHandler) wait() {
	<-s.doneSignal()
}

// StopAndWait releases the listener and waits for its goroutine to finish.
func (s *SignalHandler) StopAndWait() {
	s.stop()
	s.wait()
}

func (s *SignalHandler) lifecycleDone() <-chan struct{} {
	if s == nil || s.lifecycleCancel == nil {
		return nil
	}
	return s.lifecycleCtx.Done()
}

func (s *SignalHandler) cancelGlobal() {
	if s != nil && s.globalCancel != nil {
		s.globalCancel()
	}
}
