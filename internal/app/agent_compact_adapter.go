package app

import (
	"strings"

	agentruntime "github.com/agent-dance/luban/internal/agent"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/runtime/compact"
)

// appBackgroundTaskCompactAdapter projects the Agent domain's neutral task
// snapshots into compact's attachment DTO. The Agent runtime must not import
// the compaction implementation merely to satisfy composition wiring.
type appBackgroundTaskCompactAdapter struct {
	source interface {
		Snapshots() []agentcontract.TaskSnapshot
	}
}

func (a appBackgroundTaskCompactAdapter) PostCompactBackgroundTasks() []compact.BackgroundTaskSnapshot {
	if a.source == nil {
		return nil
	}
	snapshots := a.source.Snapshots()
	out := make([]compact.BackgroundTaskSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if strings.TrimSpace(snapshot.ID) == "" {
			continue
		}
		out = append(out, compact.BackgroundTaskSnapshot{
			ID:          snapshot.ID,
			Type:        snapshot.Type,
			Status:      snapshot.Status,
			Description: snapshot.Description,
			Command:     snapshot.Command,
			Prompt:      snapshot.Prompt,
			Error:       snapshot.Error,
			Result:      snapshot.Result,
		})
	}
	return out
}

// appAgentDefinitionCompactAdapter keeps Agent profile loading in its owning
// domain and limits compact to the three fields it presents after compression.
type appAgentDefinitionCompactAdapter struct {
	source interface {
		LoadAgentDefinitionsForRuntime(string) ([]agentruntime.AgentDefinition, error)
	}
}

func (a appAgentDefinitionCompactAdapter) PostCompactAgentDefinitions(cwd string) []compact.AgentDefinitionSnapshot {
	if a.source == nil {
		return nil
	}
	definitions, err := a.source.LoadAgentDefinitionsForRuntime(cwd)
	if err != nil {
		return nil
	}
	definitions = resolveCompactAgentDefinitionOverrides(definitions)
	out := make([]compact.AgentDefinitionSnapshot, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, compact.AgentDefinitionSnapshot{
			Name:      definition.Name,
			WhenToUse: definition.WhenToUse,
			Source:    definition.Source,
		})
	}
	return out
}

func resolveCompactAgentDefinitionOverrides(definitions []agentruntime.AgentDefinition) []agentruntime.AgentDefinition {
	if len(definitions) == 0 {
		return definitions
	}
	rank := func(source string) int {
		switch strings.ToLower(strings.TrimSpace(source)) {
		case "project":
			return 0
		case "user":
			return 1
		case "plugin":
			return 2
		case "managed":
			return 3
		case "builtin":
			return 4
		default:
			return 5
		}
	}
	winners := make(map[string]agentruntime.AgentDefinition, len(definitions))
	winnerRanks := make(map[string]int, len(definitions))
	order := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		key := strings.ToLower(strings.TrimSpace(definition.Name))
		if key == "" {
			continue
		}
		candidateRank := rank(definition.Source)
		if existingRank, exists := winnerRanks[key]; exists {
			if candidateRank >= existingRank {
				continue
			}
		} else {
			order = append(order, key)
		}
		winners[key] = definition
		winnerRanks[key] = candidateRank
	}
	resolved := make([]agentruntime.AgentDefinition, 0, len(order))
	for _, key := range order {
		resolved = append(resolved, winners[key])
	}
	return resolved
}
