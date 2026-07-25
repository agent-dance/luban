package interaction

import (
	"context"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	permissionModeDefault = "default"
	permissionModePlan    = "plan"
)

// planModePermissionRuntime is the session dispatcher shared by tool
// visibility, permission enforcement, and UI mode state.
type planModePermissionRuntime interface {
	types.ToolRuntimeContextProvider
	TransitionPermissionMode(string) error
	RestorePermissionMode(string) error
}

func (t *EnterPlanModeTool) runtimeContext() types.ToolRuntimeContext {
	if t == nil || t.Runtime == nil {
		return types.ToolRuntimeContext{}
	}
	return t.Runtime.ToolRuntimeContext()
}

func (t *EnterPlanModeTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return strings.TrimSpace(runtime.AgentID) == ""
}

func (t *EnterPlanModeTool) validateRuntimeContext() (types.ToolRuntimeContext, error) {
	runtime := t.runtimeContext()
	if strings.TrimSpace(runtime.AgentID) != "" {
		return runtime, i18n.NewError(i18n.KeyToolPlanModeAgentContext)
	}
	return runtime, nil
}

func (t *EnterPlanModeTool) transitionToPlanMode(ctx context.Context, runtime types.ToolRuntimeContext) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	previousMode := strings.TrimSpace(runtime.PermissionMode)
	if previousMode == "" {
		previousMode = permissionModeDefault
	}
	snapshot := map[string]any{
		"permission_mode": previousMode,
	}
	if t.Runtime != nil {
		if err := t.Runtime.TransitionPermissionMode(permissionModePlan); err != nil {
			return nil, i18n.WrapError(i18n.KeyToolPlanModeEnterPermission, err)
		}
	}
	return snapshot, nil
}
