package approvalcommit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestReceiptIsBoundAndConsumedExactlyOnce(t *testing.T) {
	input := map[string]any{"command": "pwd"}
	ctx := Bind(context.Background(), "Bash", input, "shell.read")

	if got := Consume(ctx, "Bash", input, "shell.read"); got != PermissionCommitValid {
		t.Fatalf("first consume = %v, want valid", got)
	}
	if got := Consume(ctx, "Bash", input, "shell.read"); got != PermissionCommitInvalid {
		t.Fatalf("second consume = %v, want invalid", got)
	}
	if got := Consume(context.Background(), "Bash", input, "shell.read"); got != PermissionCommitAbsent {
		t.Fatalf("unbound consume = %v, want absent", got)
	}
}

func TestReceiptRejectsCrossedIdentity(t *testing.T) {
	tests := []struct {
		name       string
		toolName   string
		input      map[string]any
		policyCode string
	}{
		{name: "tool", toolName: "PowerShell", input: map[string]any{"command": "pwd"}, policyCode: "shell.read"},
		{name: "input", toolName: "Bash", input: map[string]any{"command": "whoami"}, policyCode: "shell.read"},
		{name: "policy", toolName: "Bash", input: map[string]any{"command": "pwd"}, policyCode: "shell.write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := Bind(context.Background(), "Bash", map[string]any{"command": "pwd"}, "shell.read")
			if got := Consume(ctx, tt.toolName, tt.input, tt.policyCode); got != PermissionCommitInvalid {
				t.Fatalf("consume = %v, want invalid", got)
			}
		})
	}
}

func TestReceiptCannotValidateUnhashableInput(t *testing.T) {
	input := map[string]any{"invalid": make(chan struct{})}
	ctx := Bind(context.Background(), "Bash", input, "shell.read")
	if got := Consume(ctx, "Bash", input, "shell.read"); got != PermissionCommitInvalid {
		t.Fatalf("consume = %v, want invalid", got)
	}
}

func TestReceiptConcurrentConsumeHasSingleWinner(t *testing.T) {
	input := map[string]any{"command": "pwd"}
	ctx := Bind(context.Background(), "Bash", input, "shell.read")
	var valid atomic.Int32
	var invalid atomic.Int32
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			switch Consume(ctx, "Bash", input, "shell.read") {
			case PermissionCommitValid:
				valid.Add(1)
			case PermissionCommitInvalid:
				invalid.Add(1)
			}
		}()
	}
	wait.Wait()
	if got := valid.Load(); got != 1 {
		t.Fatalf("valid consumes = %d, want 1", got)
	}
	if got := invalid.Load(); got != 31 {
		t.Fatalf("invalid consumes = %d, want 31", got)
	}
}
