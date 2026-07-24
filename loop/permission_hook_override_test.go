package loop

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type hookGrantCheckedPermissionHandler struct {
	called bool
}

func (h *hookGrantCheckedPermissionHandler) Check(context.Context, PermissionRequest) (PermissionDecision, error) {
	h.called = true
	return PermissionDeny, nil
}

func (*hookGrantCheckedPermissionHandler) CheckHookGrantedPermissions() bool { return true }

func TestMandatorySubagentHandlerRejectsHookPermissionGrant(t *testing.T) {
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Echo"})
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookPreToolUse, Command: `printf '{"permissionBehavior":"allow"}'`, Timeout: 5,
	}})
	handler := &hookGrantCheckedPermissionHandler{}
	results, _, err := executeToolsConcurrently(context.Background(), reg, runner, handler, "child", ToolExecutionContext{}, []types.ToolUseBlock{{
		Type: types.ContentTypeToolUse, ID: "tool_1", Name: "Echo", Input: map[string]any{},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !handler.called {
		t.Fatal("hook allow bypassed mandatory subagent permission handler")
	}
	if len(results) != 1 || !results[0].IsError {
		t.Fatalf("hook grant executed despite inherited denial: %#v", results)
	}
}
