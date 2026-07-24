package main

import (
	"log"
	"log/slog"
	"sync"
)

const interactiveDiagnosticCapacity = 256 << 10

// boundedDiagnosticWriter retains private process diagnostics in memory while
// an interactive terminal surface owns stdout/stderr. It prevents standard
// loggers from corrupting the alternate-screen renderer without exposing raw
// causes or metadata through user-visible UI.
type boundedDiagnosticWriter struct {
	mu       sync.Mutex
	capacity int
	data     []byte
}

func newBoundedDiagnosticWriter(capacity int) *boundedDiagnosticWriter {
	if capacity <= 0 {
		capacity = interactiveDiagnosticCapacity
	}
	return &boundedDiagnosticWriter{capacity: capacity}
}

func (w *boundedDiagnosticWriter) Write(value []byte) (int, error) {
	if w == nil {
		return len(value), nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	originalLength := len(value)
	if len(value) >= w.capacity {
		w.data = append(w.data[:0], value[len(value)-w.capacity:]...)
		return originalLength, nil
	}
	w.data = append(w.data, value...)
	if overflow := len(w.data) - w.capacity; overflow > 0 {
		copy(w.data, w.data[overflow:])
		w.data = w.data[:w.capacity]
	}
	return originalLength, nil
}

func (w *boundedDiagnosticWriter) Snapshot() []byte {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.data...)
}

// installInteractiveDiagnosticLogger redirects both standard logging APIs to
// a private bounded sink. The returned restore function is primarily useful to
// tests and to transports that release interactive ownership in-process.
func installInteractiveDiagnosticLogger() (*boundedDiagnosticWriter, func()) {
	sink := newBoundedDiagnosticWriter(interactiveDiagnosticCapacity)
	previousSlog := slog.Default()
	previousLogWriter := log.Writer()
	previousLogFlags := log.Flags()
	previousLogPrefix := log.Prefix()
	slog.SetDefault(slog.New(slog.NewTextHandler(sink, nil)))
	log.SetOutput(sink)
	return sink, func() {
		slog.SetDefault(previousSlog)
		log.SetOutput(previousLogWriter)
		log.SetFlags(previousLogFlags)
		log.SetPrefix(previousLogPrefix)
	}
}
