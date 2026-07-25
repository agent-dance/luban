package shell_test

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	shell "github.com/agent-dance/luban/internal/tools/shell"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func executeApprovedBashForTest(t *testing.T, ctx context.Context, tool *shell.BashTool, input map[string]any) (types.ToolResultBlock, error) {
	t.Helper()
	reg := registry.New()
	reg.Register(tool)
	preflight, err := reg.CheckToolPermissions(ctx, tool.Name(), input, types.ToolPermissionRequest{
		SessionID: "bash-test-session", TurnID: "bash-test-turn", ToolUseID: "bash-test-use", ApprovalEpoch: "bash-test-epoch",
	})
	if err != nil {
		return types.ToolResultBlock{}, err
	}
	if preflight.Behavior == types.PermissionBehaviorDeny {
		return reg.ExecuteToolWithError(ctx, tool.Name(), input)
	}
	token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, tool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	if token == "" {
		t.Fatalf("Bash permission preflight did not produce an execution receipt: %+v", preflight)
	}
	approved := approvalcommit.WithPending(ctx, approvalcommit.Pending{
		Token: token, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})
	return reg.ExecuteToolWithError(approved, tool.Name(), input)
}
