package sdk

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestSetToolApproval_Allow verifies that a registered ToolApprovalFunc that
// returns PermissionAllow causes Check() to approve without a bridge round-trip.
func TestSetToolApproval_Allow(t *testing.T) {
	t.Parallel()

	pr, pw := newPipe()
	srv := NewSDKServer(newMockEngine(nil, "claude-3-5-sonnet-20241022"), pr, pw, InitialPermissionBridge)

	srv.SetToolApproval(func(toolName string, input map[string]any) PermissionDecision {
		return PermissionAllow
	})

	h := srv.permissionHandler()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	decision, err := h.Check(ctx, PermissionRequest{ToolName: "Bash", Input: map[string]any{"command": "ls"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != PermissionAllow {
		t.Fatalf("expected PermissionAllow, got %v", decision)
	}
}

// TestSetToolApproval_Deny verifies that a ToolApprovalFunc returning
// PermissionDeny causes Check() to deny without a bridge round-trip.
func TestSetToolApproval_Deny(t *testing.T) {
	t.Parallel()

	pr, pw := newPipe()
	srv := NewSDKServer(newMockEngine(nil, "claude-3-5-sonnet-20241022"), pr, pw, InitialPermissionBridge)

	srv.SetToolApproval(func(toolName string, input map[string]any) PermissionDecision {
		return PermissionDeny
	})

	h := srv.permissionHandler()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	decision, err := h.Check(ctx, PermissionRequest{ToolName: "Write", Input: map[string]any{"file_path": "/etc/passwd"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != PermissionDeny {
		t.Fatalf("expected PermissionDeny, got %v", decision)
	}
}

// TestSetToolApproval_Abstain verifies that PermissionAbstain falls through to
// the bridge (which will time-out here, returning ctx error).
func TestSetToolApproval_Abstain(t *testing.T) {
	t.Parallel()

	pr, pw := newPipe()
	srv := NewSDKServer(newMockEngine(nil, "claude-3-5-sonnet-20241022"), pr, pw, InitialPermissionBridge)

	srv.SetToolApproval(func(toolName string, _ map[string]any) PermissionDecision {
		return PermissionAbstain // let bridge handle it
	})

	h := srv.permissionHandler()
	// Very short timeout so the bridge times-out quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := h.Check(ctx, PermissionRequest{ToolName: "Bash", Input: map[string]any{"command": "echo hi"}})
	if err == nil {
		t.Fatal("expected a timeout/ctx error when bridge has no responder, got nil")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected context error, got: %v", err)
	}
}

// TestSetToolApproval_Nil verifies that unsetting the callback (nil) restores
// the bridge-based flow (which times out when no client responds).
func TestSetToolApproval_Nil(t *testing.T) {
	t.Parallel()

	pr, pw := newPipe()
	srv := NewSDKServer(newMockEngine(nil, "claude-3-5-sonnet-20241022"), pr, pw, InitialPermissionBridge)

	// First register a callback, then clear it.
	srv.SetToolApproval(func(_ string, _ map[string]any) PermissionDecision {
		return PermissionAllow
	})
	srv.SetToolApproval(nil) // clear

	h := srv.permissionHandler()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := h.Check(ctx, PermissionRequest{ToolName: "Bash", Input: map[string]any{"command": "echo hi"}})
	if err == nil {
		t.Fatal("expected context/timeout error after clearing callback, got nil")
	}
}

// TestSetToolApproval_ToolSpecific verifies that the callback receives the
// correct tool name and input.
func TestSetToolApproval_ToolSpecific(t *testing.T) {
	t.Parallel()

	pr, pw := newPipe()
	srv := NewSDKServer(newMockEngine(nil, "claude-3-5-sonnet-20241022"), pr, pw, InitialPermissionBridge)

	var gotTool string
	var gotCmd string
	srv.SetToolApproval(func(toolName string, input map[string]any) PermissionDecision {
		gotTool = toolName
		if cmd, ok := input["command"].(string); ok {
			gotCmd = cmd
		}
		return PermissionAllow
	})

	h := srv.permissionHandler()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wantTool := "Bash"
	wantCmd := "ls -la"
	_, err := h.Check(ctx, PermissionRequest{
		ToolName: wantTool,
		Input:    map[string]any{"command": wantCmd},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTool != wantTool {
		t.Errorf("tool name: got %q, want %q", gotTool, wantTool)
	}
	if gotCmd != wantCmd {
		t.Errorf("command: got %q, want %q", gotCmd, wantCmd)
	}
}

// newPipe returns a connected pair of io.ReadCloser / io.WriteCloser backed by
// strings.NewReader so the server doesn't block on stdin reads in these tests.
func newPipe() (*strings.Reader, *strings.Builder) {
	return strings.NewReader(""), &strings.Builder{}
}
