package loop

import (
	"context"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func executeToolsConcurrently(ctx context.Context, registry *registry.Registry, runner *hooks.Runner, permissions permission.PermissionHandler, sessionID string, execution executioncontract.ToolExecutionContext, toolUses []types.ToolUseBlock, onResult func(int, types.ToolResultBlock)) ([]types.ToolResultBlock, []string, error) {
	detailed, err := executeToolsConcurrentlyDetailed(ctx, registry, runner, permissions, sessionID, execution, toolUses, onResult)
	return detailed.Results, detailed.Reminders, err
}
