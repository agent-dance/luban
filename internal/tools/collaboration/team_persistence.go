package collaboration

import (
	"strings"
	"time"

	"github.com/agent-dance/luban/swarm"
)

func buildPersistedTeamConfig(
	teamName string,
	description string,
	leadAgentID string,
	owner RuntimeIdentity,
	agentType string,
) *swarm.TeamConfig {
	joinedAt := time.Now().UnixMilli()
	agentType = strings.TrimSpace(agentType)
	if agentType == "" {
		agentType = teamLeadName
	}
	return &swarm.TeamConfig{
		Name:          teamName,
		Description:   description,
		CreatedAt:     joinedAt,
		LeadAgentID:   leadAgentID,
		LeadSessionID: owner.SessionID,
		Members: []swarm.TeamMember{{
			AgentID:       leadAgentID,
			Name:          teamLeadName,
			AgentType:     agentType,
			Model:         owner.Model,
			JoinedAt:      joinedAt,
			CWD:           owner.ProjectRoot,
			Subscriptions: []string{},
			IsActive:      true,
		}},
	}
}

func activeNonLeadTeamMembers(storageName string) ([]string, error) {
	config, err := swarm.LoadTeamConfig(storageName)
	if err != nil {
		return nil, err
	}
	active := make([]string, 0, len(config.Members))
	for _, member := range config.Members {
		if strings.EqualFold(member.Name, teamLeadName) || !member.IsActive {
			continue
		}
		active = append(active, member.Name)
	}
	return active, nil
}
