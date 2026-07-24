package ui

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// spinnerFrames is the braille spinner animation sequence.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerLineWidth is the fixed width used to overwrite the previous line cleanly.
const spinnerLineWidth = 60

// Spinner animates a braille spinner with elapsed time while a tool is running.
type Spinner struct {
	w       io.Writer
	name    string
	start   time.Time
	done    chan struct{}
	mu      sync.Mutex
	stopped bool
}

// NewSpinner creates a Spinner that writes to w for the named tool.
func NewSpinner(w io.Writer, toolName string) *Spinner {
	return &Spinner{
		w:    w,
		name: toolName,
		done: make(chan struct{}),
	}
}

// Start launches the spinner animation in a background goroutine.
// The goroutine writes frames like "⠙ Running Bash... (1.2s)" every 80ms,
// overwriting the previous frame via a carriage return.
func (s *Spinner) Start() {
	s.start = time.Now()
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		frame := 0
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				elapsed := time.Since(s.start).Seconds()
				line := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyPresentationRunning,
					spinnerFrames[frame%len(spinnerFrames)], s.name, elapsed)
				s.mu.Lock()
				if !s.stopped {
					fmt.Fprintf(s.w, "\r%-*s", spinnerLineWidth, line)
				}
				s.mu.Unlock()
				frame++
			}
		}
	}()
}

// Stop halts the animation and clears the spinner line. Safe to call multiple times.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.done)
	// Overwrite the spinner line with spaces, then return the cursor to column 0.
	fmt.Fprintf(s.w, "\r%*s\r", spinnerLineWidth, "")
	s.mu.Unlock()
}
