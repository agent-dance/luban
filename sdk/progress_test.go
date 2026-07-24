package sdk

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewProgressEmitter_DefaultBuffer(t *testing.T) {
	e := NewProgressEmitter(0)
	if e == nil {
		t.Fatal("NewProgressEmitter returned nil")
	}
	if cap(e.ch) != defaultProgressBufferSize {
		t.Errorf("expected buffer %d, got %d", defaultProgressBufferSize, cap(e.ch))
	}
}

func TestNewProgressEmitter_CustomBuffer(t *testing.T) {
	e := NewProgressEmitter(8)
	if cap(e.ch) != 8 {
		t.Errorf("expected buffer 8, got %d", cap(e.ch))
	}
}

func TestProgressEmitter_EmitAndReceive(t *testing.T) {
	e := NewProgressEmitter(4)

	evt := ToolProgressEvent{
		ToolName: "bash",
		Status:   "started",
		Progress: 0.0,
		Message:  "starting",
	}

	if !e.Emit(evt) {
		t.Fatal("Emit should return true on buffered channel with space")
	}

	select {
	case got := <-e.Events():
		if got.ToolName != "bash" {
			t.Errorf("unexpected ToolName: %q", got.ToolName)
		}
		if got.Status != "started" {
			t.Errorf("unexpected Status: %q", got.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestProgressEmitter_EmitDropsWhenFull(t *testing.T) {
	e := NewProgressEmitter(2)

	fill := ToolProgressEvent{ToolName: "tool", Status: "running", Progress: 0.5}
	e.Emit(fill)
	e.Emit(fill)

	// Buffer is now full; Emit should return false.
	dropped := ToolProgressEvent{ToolName: "tool", Status: "running", Progress: 0.9, Message: "should drop"}
	if e.Emit(dropped) {
		t.Error("Emit should return false when buffer is full")
	}
}

func TestProgressEmitter_MultipleEvents(t *testing.T) {
	e := NewProgressEmitter(10)

	statuses := []string{"started", "running", "running", "completed"}
	for i, s := range statuses {
		progress := float64(i) / float64(len(statuses)-1)
		e.Emit(ToolProgressEvent{ToolName: "write_file", Status: s, Progress: progress})
	}
	e.Close()

	var got []ToolProgressEvent
	for evt := range e.Events() {
		got = append(got, evt)
	}

	if len(got) != len(statuses) {
		t.Fatalf("expected %d events, got %d", len(statuses), len(got))
	}
	if got[0].Status != "started" {
		t.Errorf("first event should be started, got %q", got[0].Status)
	}
	if got[len(got)-1].Status != "completed" {
		t.Errorf("last event should be completed, got %q", got[len(got)-1].Status)
	}
}

func TestProgressEmitter_CloseSignalsConsumer(t *testing.T) {
	e := NewProgressEmitter(4)
	e.Emit(ToolProgressEvent{ToolName: "tool", Status: "completed", Progress: 1.0})
	e.Close()

	count := 0
	for range e.Events() {
		count++
	}
	if count != 1 {
		t.Errorf("expected 1 event after close, got %d", count)
	}
}

func TestProgressEmitter_EmitBlocking_Cancelled(t *testing.T) {
	e := NewProgressEmitter(1)
	// Fill the single slot.
	e.Emit(ToolProgressEvent{ToolName: "tool", Status: "running"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := e.EmitBlocking(ctx, ToolProgressEvent{ToolName: "tool", Status: "running"})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestProgressEmitter_EmitBlocking_Success(t *testing.T) {
	e := NewProgressEmitter(2)

	ctx := context.Background()
	err := e.EmitBlocking(ctx, ToolProgressEvent{ToolName: "grep", Status: "started"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case got := <-e.Events():
		if got.ToolName != "grep" {
			t.Errorf("expected ToolName grep, got %q", got.ToolName)
		}
	default:
		t.Fatal("expected event in channel")
	}
}

func TestProgressEmitter_DoubleCloseSafe(t *testing.T) {
	e := NewProgressEmitter(4)
	e.Emit(ToolProgressEvent{ToolName: "tool", Status: "started"})

	// First close should succeed silently.
	e.Close()
	// Second close must not panic.
	e.Close()
}

func TestProgressEmitter_DoubleCloseConcurrent(t *testing.T) {
	e := NewProgressEmitter(4)

	// Multiple concurrent Close calls must not panic or race.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Close()
		}()
	}
	wg.Wait()
}

func TestProgressEmitter_EventsInterface(t *testing.T) {
	var emitter ProgressEmitter = NewProgressEmitter(4)

	emitter.Emit(ToolProgressEvent{ToolName: "read_file", Status: "completed", Progress: 1.0})

	ch := emitter.Events()
	select {
	case evt := <-ch:
		if evt.ToolName != "read_file" {
			t.Errorf("unexpected ToolName: %q", evt.ToolName)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading from emitter channel")
	}
}

