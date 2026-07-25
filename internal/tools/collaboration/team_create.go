package collaboration

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/runtime/skillauthority"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

type teamCreateInput struct {
	TeamName    string `json:"team_name"`
	Description string `json:"description,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
}

type TeamCreateTool struct {
	manager *TeamManager
}

func NewTeamCreateTool(manager *TeamManager) *TeamCreateTool {
	return &TeamCreateTool{manager: manager}
}

func (*TeamCreateTool) Name() string { return "TeamCreate" }

func (*TeamCreateTool) Description() string {
	return runtimeText(i18n.KeyToolTeamCreateDescription)
}

func (*TeamCreateTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true}
}

func (*TeamCreateTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"team_name": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolTeamCreateTeamNameDescription),
			},
			"description": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolTeamCreatePurposeDescription),
			},
			"agent_type": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolTeamCreateAgentTypeDescription),
			},
		},
		"team_name",
	)
}

func (tool *TeamCreateTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if tool == nil || tool.manager == nil {
		return errorResult(i18n.NewError(i18n.KeyToolCollaborationManagerRequired)), nil
	}
	in, toolErr := toolbase.ParseStrictInputOrError[teamCreateInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	requestedName := strings.TrimSpace(in.TeamName)
	if requestedName == "" {
		return teamToolError(i18n.KeyToolRuntimeRequiredFieldMissing, "team_name"), nil
	}

	var result types.ToolResult
	err := tool.manager.withMutation(func() error {
		var executeErr error
		result, executeErr = tool.execute(ctx, in, requestedName)
		return executeErr
	})
	return result, err
}

func (tool *TeamCreateTool) execute(
	ctx context.Context,
	in teamCreateInput,
	requestedName string,
) (types.ToolResult, error) {
	manager := tool.manager
	authority, err := skillauthority.Capture(ctx, manager.skills)
	if err != nil {
		return errorResult(err), nil
	}
	runtime := manager.runtimeIdentitySnapshot()
	if runtime.SessionID == "" || runtime.ProjectRoot == "" {
		return errorResult(i18n.NewError(i18n.KeyToolCollaborationRuntimeIdentityIncomplete)), nil
	}
	if exec, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		if model := strings.TrimSpace(exec.Model); model != "" {
			runtime.Model = model
		}
	}
	if err := authority.ValidateRuntime(types.ToolRuntimeContext{
		SessionID:   runtime.SessionID,
		ProjectRoot: runtime.ProjectRoot,
	}); err != nil {
		return errorResult(err), nil
	}
	owner := teamOwnerKey{SessionID: runtime.SessionID, ProjectRoot: runtime.ProjectRoot}

	manager.mu.Lock()
	existingID := manager.activeByOwner[owner]
	existing := manager.teams[existingID]
	manager.mu.Unlock()
	if existing != nil && teamInfoOwnedBy(existing, owner) {
		return teamToolError(i18n.KeyToolTeamCreateAlreadyLeading, existing.Name), nil
	}

	finalName, err := uniqueTeamName(requestedName)
	if err != nil {
		return errorResult(err), nil
	}
	storageName := teamStorageName(finalName)
	leadAgentID := fmt.Sprintf("team-lead@%s", finalName)
	teamFilePath, err := swarm.TeamConfigPath(storageName)
	if err != nil {
		return swarmErrorResult(err), nil
	}
	config := buildPersistedTeamConfig(finalName, in.Description, leadAgentID, runtime, in.AgentType)
	if err := swarm.CreateTeamConfigAs(storageName, config); err != nil {
		return swarmErrorResult(err), nil
	}

	teamID := ""
	commitErr := authority.WithGenerationLease(manager.skills, func() error {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		if manager.currentTeamOwnerLocked() != owner {
			return i18n.WrapInternalError(
				i18n.KeyLoopQueryValidateSkillGenerationFailed,
				skills.ErrSkillProjectGenerationChanged,
			)
		}
		if currentID := manager.activeByOwner[owner]; manager.teams[currentID] != nil {
			return i18n.NewError(i18n.KeyToolTeamCreateAlreadyLeading, manager.teams[currentID].Name)
		}
		manager.nextTeamID++
		teamID = fmt.Sprintf("team-%d", manager.nextTeamID)
		manager.teams[teamID] = &teamInfo{
			ID:               teamID,
			Name:             finalName,
			StorageName:      storageName,
			OwnerSessionID:   owner.SessionID,
			OwnerProjectRoot: owner.ProjectRoot,
		}
		manager.activeByOwner[owner] = teamID
		return nil
	})
	if commitErr != nil {
		_ = swarm.DeleteTeamConfig(storageName)
		return errorResult(commitErr), nil
	}
	manager.notifyTaskListChanged()

	if lifecycle := manager.lifecycleForOwner(owner); lifecycle != nil {
		_ = lifecycle.Publish(ctx, runtimestore.RuntimeLifecycleEvent{
			Type:      runtimestore.LifecycleTeamCreate,
			EntityID:  teamID,
			ToolName:  "TeamCreate",
			Status:    "active",
			SessionID: owner.SessionID,
			Payload: map[string]any{
				"name":               finalName,
				"storage_name":       storageName,
				"owner_project_root": owner.ProjectRoot,
			},
		})
	}

	return responseJSON(map[string]any{
		"team_name":      finalName,
		"team_file_path": teamFilePath,
		"lead_agent_id":  leadAgentID,
	})
}
