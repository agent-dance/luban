package sdk

import (
	"context"
	"sync"
)

const defaultProgressBufferSize = 64

// ToolProgressEvent carries a progress update from a running tool.
// SDK consumers receive these via a ProgressEmitter's Events channel.
type ToolProgressEvent struct {
	// ToolName is the name of the tool emitting progress.
	ToolName string `json:"tool_name"`

	// Status is a short lifecycle descriptor: "started", "running",
	// "completed", or "error".
	Status string `json:"status"`

	// Progress is the fraction of completion in the range [0.0, 1.0].
	// Use -1 to indicate an indeterminate/unknown progress.
	Progress float64 `json:"progress"`

	// Message is an optional human-readable description of the current step.
	Message string `json:"message,omitempty"`
}

// ProgressEmitter is the interface for types that produce ToolProgressEvents.
type ProgressEmitter interface {
	// Emit sends a ToolProgressEvent to consumers.
	// Returns false if the channel is full and the event was dropped.
	Emit(evt ToolProgressEvent) bool

	// Events returns the read-only channel of ToolProgressEvents.
	Events() <-chan ToolProgressEvent

	// Close signals that no further events will be emitted.
	// Callers must not call Emit after Close.
	Close()
}

// ChanProgressEmitter is a channel-backed ProgressEmitter.
// Create one with NewProgressEmitter.
type ChanProgressEmitter struct {
	ch        chan ToolProgressEvent
	closeOnce sync.Once
}

// NewProgressEmitter returns a ChanProgressEmitter backed by a buffered channel.
// If bufSize <= 0 the default buffer size (64) is used.
func NewProgressEmitter(bufSize int) *ChanProgressEmitter {
	if bufSize <= 0 {
		bufSize = defaultProgressBufferSize
	}
	return &ChanProgressEmitter{ch: make(chan ToolProgressEvent, bufSize)}
}

// Emit sends evt to the buffered channel without blocking.
// Returns true when the event was enqueued; false when the buffer is full and
// the event is dropped to avoid blocking tool execution.
func (e *ChanProgressEmitter) Emit(evt ToolProgressEvent) bool {
	select {
	case e.ch <- evt:
		return true
	default:
		return false
	}
}

// EmitBlocking sends evt to the channel, blocking until the event is accepted
// or ctx is cancelled. Useful when the caller must not drop events.
func (e *ChanProgressEmitter) EmitBlocking(ctx context.Context, evt ToolProgressEvent) error {
	select {
	case e.ch <- evt:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Events returns the read-only channel of ToolProgressEvents.
func (e *ChanProgressEmitter) Events() <-chan ToolProgressEvent {
	return e.ch
}

// Close closes the underlying channel, signalling consumers that no further
// events will arrive. Safe to call multiple times; only the first call closes
// the channel.
func (e *ChanProgressEmitter) Close() {
	e.closeOnce.Do(func() { close(e.ch) })
}
