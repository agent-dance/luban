package loop

import (
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/types"
)

func TestPermissionRequestPreservesToolLocalReadOnlyAllow(t *testing.T) {
	request := buildPermissionRequest("session", executioncontract.ToolExecutionContext{}, types.ToolUseBlock{
		ID: "toolu-read", Name: "Read", Input: map[string]any{"file_path": "/workspace/note.txt"},
	}, types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorPassthrough, ToolLocalReadOnlyAllow: true,
	})

	if !request.ToolLocalReadOnlyAllow {
		t.Fatal("permission request lost the tool-local read-only allow proof")
	}
}
