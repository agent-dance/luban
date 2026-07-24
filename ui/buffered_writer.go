package ui

import (
	"bytes"
	"io"
	"sync"
	"time"
)

// BufferedWriter batches writes and flushes to the underlying writer at a
// fixed interval (default 16 ms ≈ 60 fps) to reduce terminal flicker during
// rapid streaming output.
type BufferedWriter struct {
	w        io.Writer
	mu       sync.Mutex
	buf      bytes.Buffer
	timer    *time.Timer
	interval time.Duration
}

// NewBufferedWriter creates a BufferedWriter that flushes to w at ~60 fps.
func NewBufferedWriter(w io.Writer) *BufferedWriter {
	return &BufferedWriter{
		w:        w,
		interval: 16 * time.Millisecond,
	}
}

// Write appends p to the internal buffer and schedules a flush if one is not
// already pending.
func (bw *BufferedWriter) Write(p []byte) (int, error) {
	bw.mu.Lock()
	n, err := bw.buf.Write(p)
	if err == nil && bw.timer == nil {
		bw.timer = time.AfterFunc(bw.interval, func() {
			_ = bw.Flush()
		})
	}
	bw.mu.Unlock()
	return n, err
}

// Flush immediately writes all buffered data to the underlying writer and
// cancels any pending timer.
func (bw *BufferedWriter) Flush() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	if bw.timer != nil {
		bw.timer.Stop()
		bw.timer = nil
	}
	if bw.buf.Len() == 0 {
		return nil
	}
	data := bw.buf.Bytes()
	bw.buf.Reset()
	_, err := bw.w.Write(data)
	return err
}

// Close flushes any remaining buffered data and stops the timer.
func (bw *BufferedWriter) Close() error {
	return bw.Flush()
}
