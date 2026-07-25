package permissions

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/permission"
)

func TestBackgroundDefaultPermissionCannotFollowForegroundAllowAll(t *testing.T) {
	checker := NewChecker(ModeRuleBased, []Rule{{Tool: "Write", Decision: DecisionDeny}})
	handler := NewCLIPermissionHandler(checker)

	// Simulate the foreground switching to bypassPermissions after a background
	// agent captured the explicit default mode.
	if err := checker.SetModeFromUser(ModeAllowAll); err != nil {
		t.Fatal(err)
	}
	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		ToolName: "Write", Input: map[string]any{"file_path": "result.txt"}, Mode: "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision != permission.PermissionDeny {
		t.Fatalf("background default permission followed foreground AllowAll: %v", decision)
	}
}
