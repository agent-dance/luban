package hooks

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/agent-dance/luban/brand"
	gotui "github.com/grindlemire/go-tui"
)

// TestNotificationHookNeverBlocks verifies that notification hooks always
// return a non-blocking output regardless of the platform or outcome.
func TestNotificationHookNeverBlocks(t *testing.T) {
	hook := Hook{
		Type: HookNotification,
		Kind: HookKindNotification,
	}
	input := HookInput{
		Type:    HookNotification,
		Message: "task complete",
		Title:   brand.DisplayName,
	}

	output := executeNotificationHook(context.Background(), hook, input)

	if output.Block {
		t.Error("notification hook must never block tool execution")
	}
}

func TestNotificationHookDefaultMessage(t *testing.T) {
	hook := Hook{Kind: HookKindNotification}
	// Empty message — should use the hook type as fallback
	input := HookInput{Type: HookNotification}

	// Should not panic or block
	output := executeNotificationHook(context.Background(), hook, input)
	if output.Block {
		t.Error("expected non-blocking output")
	}
}

func TestNotificationHookCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hook := Hook{Kind: HookKindNotification}
	input := HookInput{Type: HookNotification, Message: "done", Title: "Test"}

	// A cancelled context must not panic and must return non-blocking.
	output := executeNotificationHook(ctx, hook, input)
	if output.Block {
		t.Error("cancelled context: notification must still be non-blocking")
	}
}

// TestNotificationHookTimeoutEnforced verifies that executeNotificationHook
// applies a 5-second internal timeout even when the caller passes a long-lived
// context. We can't easily force the OS notification to hang, so we verify the
// function returns within a generous bound that proves no indefinite hang.
func TestNotificationHookTimeoutEnforced(t *testing.T) {
	// Use a context with no deadline — the function must impose its own.
	ctx := context.Background()
	hook := Hook{Kind: HookKindNotification}
	input := HookInput{Type: HookNotification, Message: "timeout test", Title: "Test"}

	start := time.Now()
	output := executeNotificationHook(ctx, hook, input)
	elapsed := time.Since(start)

	// The internal timeout is 5s; allow 8s for slow CI machines plus
	// the fact that the notification command itself may finish quickly.
	if elapsed > 8*time.Second {
		t.Errorf("notification hook took %v; expected to return within 8s (internal 5s timeout)", elapsed)
	}
	// Still must not block tool execution.
	if output.Block {
		t.Error("notification hook must never block tool execution")
	}
}

type notificationControlSink struct {
	sequence []byte
}

func (s *notificationControlSink) WriteTerminalControl(sequence []byte) error {
	s.sequence = append([]byte(nil), sequence...)
	return nil
}

func TestBellNotificationUsesTerminalOwnerWithoutStderr(t *testing.T) {
	sink := &notificationControlSink{}
	release := gotui.InstallTerminalControlSink(sink)
	t.Cleanup(release)

	stderr := captureNotificationStderr(t, sendBellNotification)
	if stderr != "" {
		t.Fatalf("bell wrote ordinary stderr text: %q", stderr)
	}
	if got := string(sink.sequence); got != "\a" {
		t.Fatalf("terminal-control sequence = %q, want bell", got)
	}
}

func TestBellNotificationWithoutOwnerDoesNotWriteStderr(t *testing.T) {
	stderr := captureNotificationStderr(t, sendBellNotification)
	if stderr != "" {
		t.Fatalf("ownerless bell wrote stderr: %q", stderr)
	}
}

func captureNotificationStderr(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stderr
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = write
	defer func() { os.Stderr = original }()

	fn()
	if err := write.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	var captured bytes.Buffer
	if _, err := captured.ReadFrom(read); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := read.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return captured.String()
}
