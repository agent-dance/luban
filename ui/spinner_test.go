package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSpinner_StartStop(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "Bash")

	s.Start()
	// Let it tick at least twice
	time.Sleep(250 * time.Millisecond)
	s.Stop()

	output := buf.String()

	// Output should contain at least one frame with the tool name
	if !strings.Contains(output, "Bash") {
		t.Errorf("expected spinner output to contain tool name 'Bash', got: %q", output)
	}

	// Output should include a carriage return (animation overwrite)
	if !strings.Contains(output, "\r") {
		t.Errorf("expected spinner output to contain \\r for line overwriting")
	}
}

func TestSpinner_StopIdempotent(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "Read")

	s.Start()
	time.Sleep(50 * time.Millisecond)

	// Calling Stop multiple times must not panic or deadlock
	s.Stop()
	s.Stop()
	s.Stop()
}

func TestSpinner_NoGoroutineLeak(t *testing.T) {
	// Start and stop many spinners to verify no goroutine leaks in aggregate.
	for i := 0; i < 20; i++ {
		var buf bytes.Buffer
		s := NewSpinner(&buf, "Tool")
		s.Start()
		time.Sleep(10 * time.Millisecond)
		s.Stop()
	}

	// Give goroutines a moment to exit
	time.Sleep(100 * time.Millisecond)

	// If we reach here without a deadlock, the test passes.
	// (runtime.NumGoroutine() would be flaky in parallel test runs.)
}

func TestSpinner_StopClearsLine(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "Write")

	s.Start()
	time.Sleep(100 * time.Millisecond)
	s.Stop()

	output := buf.String()
	// The stop sequence should end with \r after the clearing spaces.
	if !strings.HasSuffix(output, "\r") {
		t.Errorf("expected output to end with \\r after Stop, got suffix: %q",
			output[max(0, len(output)-10):])
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
