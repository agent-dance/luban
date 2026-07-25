package collaboration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

type agentCollaborationSpawner struct {
	manager *TeamManager
}

func NewAgentCollaborationSpawner(manager *TeamManager) agentcontract.CollaborationSpawner {
	return &agentCollaborationSpawner{manager: manager}
}

func (s *agentCollaborationSpawner) CurrentTeamName() string {
	if s == nil || s.manager == nil {
		return ""
	}
	return s.manager.CurrentTeamName()
}

func (s *agentCollaborationSpawner) TeamExists(teamName string) bool {
	if s == nil || s.manager == nil {
		return false
	}
	team, ok := s.manager.currentTeamSnapshot()
	if !ok || strings.TrimSpace(teamName) != team.Name {
		return false
	}
	config, err := swarm.LoadTeamConfig(team.StorageName)
	return err == nil && validDurableTeamConfig(team, config)
}

func (s *agentCollaborationSpawner) SpawnTeammate(
	ctx context.Context,
	request agentcontract.TeammateSpawnRequest,
	launcher agentcontract.TeammateLauncher,
) (types.ToolResult, error) {
	if s == nil || s.manager == nil || launcher == nil {
		return errorResult(i18n.NewError(i18n.KeyToolAgentTeamManagerRequired)), nil
	}

	var result types.ToolResult
	err := s.manager.withMutation(func() error {
		var spawnErr error
		result, spawnErr = s.spawnTeammate(ctx, request, launcher)
		return spawnErr
	})
	return result, err
}

func (s *agentCollaborationSpawner) spawnTeammate(
	ctx context.Context,
	request agentcontract.TeammateSpawnRequest,
	launcher agentcontract.TeammateLauncher,
) (types.ToolResult, error) {
	team, ok := s.manager.currentTeamSnapshot()
	if !ok || strings.TrimSpace(request.TeamName) != team.Name {
		return errorResult(i18n.NewError(i18n.KeyToolAgentTeamMissing, request.TeamName)), nil
	}
	storageName := team.StorageName
	config, err := swarm.LoadTeamConfig(storageName)
	if err != nil || !validDurableTeamConfig(team, config) {
		return errorResult(i18n.NewError(i18n.KeyToolAgentTeamMissing, request.TeamName)), nil
	}

	spawnID := strings.TrimSpace(request.SpawnID)
	var identity agentcontract.TeammateIdentity
	if _, err := swarm.UpdateTeamConfig(ctx, storageName, func(config *swarm.TeamConfig) error {
		name := uniqueTeammateName(config, request.Input.Name)
		identity = agentcontract.TeammateIdentity{
			AgentID: fmt.Sprintf("%s@%s", name, config.Name),
			Name:    name,
			Team:    config.Name,
		}
		config.Members = append(config.Members, swarm.TeamMember{
			AgentID:       identity.AgentID,
			Name:          identity.Name,
			AgentType:     firstNonEmpty(request.Input.SubagentType, "general-purpose"),
			Model:         request.ParentModel,
			JoinedAt:      time.Now().UnixMilli(),
			TmuxPaneID:    identity.AgentID,
			BackendType:   "in-process",
			CWD:           team.Owner.ProjectRoot,
			Subscriptions: []string{},
			IsActive:      true,
			SpawnID:       spawnID,
			Lifecycle:     "spawning",
		})
		return nil
	}); err != nil {
		return errorResult(i18n.NewError(
			i18n.KeyToolAgentPersistTeammateFailed,
			swarm.UserFacingError(i18n.DetectOrLoadLanguage(), err),
		)), nil
	}

	launch, err := launcher(ctx, identity)
	if err != nil {
		return types.ToolResult{}, s.rollbackSpawn(ctx, storageName, spawnID, launch, err)
	}
	if _, err := swarm.UpdateTeamConfig(ctx, storageName, func(config *swarm.TeamConfig) error {
		for index := range config.Members {
			member := &config.Members[index]
			if member.SpawnID != spawnID {
				continue
			}
			member.CWD = firstNonEmpty(launch.CWD, team.Owner.ProjectRoot)
			member.Model = firstNonEmpty(launch.Model, request.ParentModel)
			member.Lifecycle = "active"
			member.IsActive = true
			return nil
		}
		return i18n.NewError(i18n.KeyToolCollaborationSpawnReservationMissing, spawnID)
	}); err != nil {
		return types.ToolResult{}, s.rollbackSpawn(ctx, storageName, spawnID, launch, err)
	}
	if launch.Start != nil {
		if err := launch.Start(); err != nil {
			return types.ToolResult{}, s.rollbackSpawn(ctx, storageName, spawnID, launch, err)
		}
	}
	return launch.Result, nil
}

func (s *agentCollaborationSpawner) rollbackSpawn(
	ctx context.Context,
	storageName string,
	spawnID string,
	launch agentcontract.TeammateLaunch,
	primary error,
) error {
	rollbackCtx := context.WithoutCancel(ctx)
	var rollbackErr error
	if launch.Rollback != nil {
		rollbackErr = launch.Rollback()
	}
	_, configErr := swarm.UpdateTeamConfig(rollbackCtx, storageName, func(config *swarm.TeamConfig) error {
		for index := range config.Members {
			if config.Members[index].SpawnID != spawnID {
				continue
			}
			config.Members = append(config.Members[:index], config.Members[index+1:]...)
			return nil
		}
		return i18n.NewError(i18n.KeyToolCollaborationSpawnReservationMissing, spawnID)
	})
	return errors.Join(primary, rollbackErr, configErr)
}

func uniqueTeammateName(config *swarm.TeamConfig, requested string) string {
	base := sanitizeSwarmName(requested, "agent")
	seen := make(map[string]struct{})
	if config != nil {
		for _, member := range config.Members {
			seen[strings.ToLower(strings.TrimSpace(member.Name))] = struct{}{}
		}
	}
	if _, exists := seen[strings.ToLower(base)]; !exists {
		return base
	}
	for index := 2; index < 1000; index++ {
		candidate := fmt.Sprintf("%s-%d", base, index)
		if _, exists := seen[strings.ToLower(candidate)]; !exists {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}
