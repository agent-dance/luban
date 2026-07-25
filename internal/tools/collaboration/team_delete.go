package collaboration

import (
	"context"
	"errors"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/skillauthority"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

type teamDeleteInput struct{}

type TeamDeleteTool struct {
	manager *TeamManager
}

func NewTeamDeleteTool(manager *TeamManager) *TeamDeleteTool {
	return &TeamDeleteTool{manager: manager}
}

func (*TeamDeleteTool) Name() string { return "TeamDelete" }

func (*TeamDeleteTool) Description() string {
	return runtimeText(i18n.KeyToolTeamDeleteDescription)
}

func (*TeamDeleteTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, Destructive: true}
}

func (*TeamDeleteTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}

func (tool *TeamDeleteTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if tool == nil || tool.manager == nil {
		return errorResult(i18n.NewError(i18n.KeyToolCollaborationManagerRequired)), nil
	}
	if _, toolErr := toolbase.ParseStrictInputOrError[teamDeleteInput](input); toolErr != nil {
		return *toolErr, nil
	}
	var result types.ToolResult
	err := tool.manager.withMutation(func() error {
		var executeErr error
		result, executeErr = tool.execute(ctx)
		return executeErr
	})
	return result, err
}

func (tool *TeamDeleteTool) execute(ctx context.Context) (types.ToolResult, error) {
	manager := tool.manager
	authority, err := skillauthority.Capture(ctx, manager.skills)
	if err != nil {
		return errorResult(err), nil
	}
	runtime := manager.runtimeIdentitySnapshot()
	if runtime.SessionID == "" || runtime.ProjectRoot == "" {
		return errorResult(i18n.NewError(i18n.KeyToolCollaborationRuntimeIdentityIncomplete)), nil
	}
	if err := authority.ValidateRuntime(types.ToolRuntimeContext{
		SessionID:   runtime.SessionID,
		ProjectRoot: runtime.ProjectRoot,
	}); err != nil {
		return errorResult(err), nil
	}
	owner := teamOwnerKey{SessionID: runtime.SessionID, ProjectRoot: runtime.ProjectRoot}

	manager.mu.Lock()
	teamID := manager.activeByOwner[owner]
	info := manager.teams[teamID]
	manager.mu.Unlock()
	if info == nil || !teamInfoOwnedBy(info, owner) {
		return responseJSON(map[string]any{
			"success": true,
			"message": runtimeText(i18n.KeyToolTeamDeleteNothingToDelete),
		})
	}
	activeMembers, err := activeNonLeadTeamMembers(info.StorageName)
	if err != nil {
		return swarmErrorResult(err), nil
	}
	if len(activeMembers) != 0 {
		return responseJSON(map[string]any{
			"success":   false,
			"message":   runtimeFormat(i18n.KeyToolTeamDeleteActiveMembersBlocked, len(activeMembers), strings.Join(activeMembers, ", ")),
			"team_name": info.Name,
		})
	}

	durableBackup, err := swarm.LoadTeamConfig(info.StorageName)
	if err != nil {
		return swarmErrorResult(err), nil
	}
	if err := swarm.DeleteTeamConfig(info.StorageName); err != nil {
		return swarmErrorResult(err), nil
	}
	commitErr := authority.WithGenerationLease(manager.skills, func() error {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		if manager.currentTeamOwnerLocked() != owner || manager.activeByOwner[owner] != teamID {
			return i18n.WrapInternalError(
				i18n.KeyLoopQueryValidateSkillGenerationFailed,
				skills.ErrSkillProjectGenerationChanged,
			)
		}
		delete(manager.teams, teamID)
		delete(manager.activeByOwner, owner)
		return nil
	})
	if commitErr != nil {
		if restoreErr := swarm.CreateTeamConfigAs(info.StorageName, durableBackup); restoreErr != nil {
			return errorResult(i18n.WrapInternalError(
				i18n.KeyAuxSwarmFailed,
				errors.Join(commitErr, restoreErr),
			)), nil
		}
		return errorResult(commitErr), nil
	}
	manager.notifyTaskListChanged()

	if lifecycle := manager.lifecycleForOwner(owner); lifecycle != nil {
		_ = lifecycle.Publish(ctx, runtimestore.RuntimeLifecycleEvent{
			Type:      runtimestore.LifecycleTeamDelete,
			EntityID:  teamID,
			ToolName:  "TeamDelete",
			Status:    "deleted",
			SessionID: owner.SessionID,
			Payload: map[string]any{
				"name":               info.Name,
				"storage_name":       info.StorageName,
				"owner_project_root": owner.ProjectRoot,
			},
		})
	}
	return responseJSON(map[string]any{
		"success":   true,
		"message":   runtimeFormat(i18n.KeyToolTeamDeleteCompleted, info.Name),
		"team_name": info.Name,
	})
}
