package app

import (
	"errors"
	"testing"

	agentruntime "github.com/agent-dance/luban/internal/agent"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
)

type compactBackgroundSource []agentcontract.TaskSnapshot

func (s compactBackgroundSource) Snapshots() []agentcontract.TaskSnapshot {
	return append([]agentcontract.TaskSnapshot(nil), s...)
}

type compactAgentDefinitionSource struct {
	definitions []agentruntime.AgentDefinition
	err         error
}

func (s compactAgentDefinitionSource) LoadAgentDefinitionsForRuntime(string) ([]agentruntime.AgentDefinition, error) {
	return append([]agentruntime.AgentDefinition(nil), s.definitions...), s.err
}

func TestBackgroundTaskCompactAdapterProjectsNeutralSnapshots(t *testing.T) {
	provider := appBackgroundTaskCompactAdapter{source: compactBackgroundSource{
		{ID: "", Status: "ignored"},
		{ID: "task-1", Type: agentcontract.TaskTypeLocalAgent, Status: "running", Description: "inspect", Result: "pending"},
	}}
	got := provider.PostCompactBackgroundTasks()
	if len(got) != 1 || got[0].ID != "task-1" || got[0].Type != agentcontract.TaskTypeLocalAgent || got[0].Result != "pending" {
		t.Fatalf("projected snapshots = %#v", got)
	}
}

func TestAgentDefinitionCompactAdapterResolvesSourcePriority(t *testing.T) {
	provider := appAgentDefinitionCompactAdapter{source: compactAgentDefinitionSource{definitions: []agentruntime.AgentDefinition{
		{Name: "Explore", WhenToUse: "builtin", Source: "builtin"},
		{Name: "Other", WhenToUse: "other", Source: "user"},
		{Name: "explore", WhenToUse: "project", Source: "project"},
	}}}
	got := provider.PostCompactAgentDefinitions("/workspace")
	if len(got) != 2 || got[0].WhenToUse != "project" || got[0].Source != "project" || got[1].Name != "Other" {
		t.Fatalf("projected definitions = %#v", got)
	}
}

func TestAgentDefinitionCompactAdapterTreatsLoadFailureAsOptionalState(t *testing.T) {
	provider := appAgentDefinitionCompactAdapter{source: compactAgentDefinitionSource{err: errors.New("unavailable")}}
	if got := provider.PostCompactAgentDefinitions("/workspace"); got != nil {
		t.Fatalf("definitions after load failure = %#v, want nil", got)
	}
}
