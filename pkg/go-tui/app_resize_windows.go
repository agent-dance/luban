//go:build windows

package tui

import (
	"sync"
	"time"
)

// registerResizeSignal is a no-op on Windows (no SIGWINCH support).
// Returns a no-op cleanup.
func (a *App) registerResizeSignal() func() {
	done := make(chan struct{})
	w, h := a.terminal.Size()
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				nextW, nextH := a.terminal.Size()
				if nextW == w && nextH == h {
					continue
				}
				w, h = nextW, nextH
				select {
				case a.inputEvents <- queuedInputEvent{event: ResizeEvent{Width: w, Height: h}, generation: a.inputGeneration.Load()}:
				case <-a.stopCh:
					return
				}
			case <-done:
				return
			case <-a.stopCh:
				return
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}
