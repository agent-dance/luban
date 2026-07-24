package ui

import (
	"bytes"
	"sync/atomic"
	"testing"
	"time"
)

// countingWriter counts how many individual Write calls are made to the
// underlying writer. Used to verify batching behaviour.
type countingWriter struct {
	buf   bytes.Buffer
	calls atomic.Int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	cw.calls.Add(1)
	return cw.buf.Write(p)
}

func TestBufferedWriter_BatchesRapidWrites(t *testing.T) {
	cw := &countingWriter{}
	bw := NewBufferedWriter(cw)

	// Fire 50 rapid writes within a single frame interval.
	const n = 50
	for i := 0; i < n; i++ {
		_, err := bw.Write([]byte("x"))
		if err != nil {
			t.Fatalf("Write error: %v", err)
		}
	}

	// Wait for at least one flush cycle (>16 ms).
	time.Sleep(50 * time.Millisecond)
	if err := bw.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// All bytes must arrive.
	got := cw.buf.String()
	if len(got) != n {
		t.Errorf("expected %d bytes in sink, got %d", n, len(got))
	}

	// Underlying Write calls should be far fewer than the number of writes.
	calls := cw.calls.Load()
	if calls >= n {
		t.Errorf("expected batching: underlying Write calls = %d, want < %d", calls, n)
	}
}

func TestBufferedWriter_FlushDeliversPendingData(t *testing.T) {
	cw := &countingWriter{}
	bw := NewBufferedWriter(cw)

	_, _ = bw.Write([]byte("hello"))
	_, _ = bw.Write([]byte(" world"))

	// Explicit flush before the timer fires.
	if err := bw.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if got := cw.buf.String(); got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestBufferedWriter_CloseFlushesRemaining(t *testing.T) {
	cw := &countingWriter{}
	bw := NewBufferedWriter(cw)

	_, _ = bw.Write([]byte("close-me"))

	if err := bw.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if got := cw.buf.String(); got != "close-me" {
		t.Errorf("expected 'close-me', got %q", got)
	}
}

func TestBufferedWriter_MultipleFlushesIdempotent(t *testing.T) {
	cw := &countingWriter{}
	bw := NewBufferedWriter(cw)

	_, _ = bw.Write([]byte("once"))
	_ = bw.Flush()
	_ = bw.Flush() // second flush should be a no-op
	_ = bw.Close()

	if got := cw.buf.String(); got != "once" {
		t.Errorf("expected 'once', got %q", got)
	}
}

func TestBufferedWriter_WriteAfterFlush(t *testing.T) {
	cw := &countingWriter{}
	bw := NewBufferedWriter(cw)

	_, _ = bw.Write([]byte("a"))
	_ = bw.Flush()
	_, _ = bw.Write([]byte("b"))
	_ = bw.Close()

	if got := cw.buf.String(); got != "ab" {
		t.Errorf("expected 'ab', got %q", got)
	}
}
