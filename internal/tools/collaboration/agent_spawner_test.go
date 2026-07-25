package collaboration

import (
	"context"
	"strings"
	"testing"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

func TestUniqueTeammateNameUsesDurableMembers(t *testing.T) {
	config := &swarm.TeamConfig{Members: []swarm.TeamMember{
		{Name: "Builder"},
		{Name: "builder-2"},
	}}

	if got := uniqueTeammateName(config, "builder"); got != "builder-3" {
		t.Fatalf("unique teammate name = %q, want %q", got, "builder-3")
	}
}

func TestAgentCollaborationSpawnerUsesCurrentDurableTeam(t *testing.T) {
	manager, identity, ctx := newFocusedTeamManager(t, "session-spawner")
	created, err := NewTeamCreateTool(manager).Execute(ctx, map[string]any{"team_name": "alpha"})
	if err != nil || created.IsError {
		t.Fatalf("TeamCreate error=%v result=%#v", err, created)
	}
	spawner := NewAgentCollaborationSpawner(manager)
	if !spawner.TeamExists("alpha") || spawner.TeamExists("other") {
		t.Fatalf("current durable team lookup is not owner-scoped")
	}

	started := false
	result, err := spawner.SpawnTeammate(ctx, agentcontract.TeammateSpawnRequest{
		SpawnID: "spawn-1", TeamName: "alpha", ParentModel: identity.Model,
		Input: agentcontract.Input{Name: "Builder", SubagentType: "reviewer"},
	}, func(_ context.Context, assigned agentcontract.TeammateIdentity) (agentcontract.TeammateLaunch, error) {
		if assigned.Name != "Builder" || assigned.Team != "alpha" || assigned.AgentID != "Builder@alpha" {
			t.Fatalf("assigned identity = %#v", assigned)
		}
		return agentcontract.TeammateLaunch{
			Result: types.ToolResult{Content: "prepared", Outcome: types.ToolOutcomeSucceeded},
			CWD:    identity.ProjectRoot, Model: identity.Model,
			Start: func() error { started = true; return nil },
		}, nil
	})
	if err != nil || result.IsError || !started {
		t.Fatalf("SpawnTeammate error=%v result=%#v started=%t", err, result, started)
	}
	config, err := swarm.LoadTeamConfig("alpha")
	if err != nil {
		t.Fatal(err)
	}
	member, ok := teamMemberByIdentity(config, "Builder@alpha")
	if !ok || member.Lifecycle != "active" || !member.IsActive || member.BackendType != "in-process" {
		t.Fatalf("durable teammate = %#v", member)
	}
	if !strings.EqualFold(member.AgentType, "reviewer") {
		t.Fatalf("agent type = %q", member.AgentType)
	}
}

func TestAgentCollaborationSpawnerRejectsNonCurrentDurableTeam(t *testing.T) {
	manager, _, ctx := newFocusedTeamManager(t, "session-current")
	created, err := NewTeamCreateTool(manager).Execute(ctx, map[string]any{"team_name": "alpha"})
	if err != nil || created.IsError {
		t.Fatalf("TeamCreate error=%v result=%#v", err, created)
	}
	spawner := NewAgentCollaborationSpawner(manager)
	called := false
	result, err := spawner.SpawnTeammate(ctx, agentcontract.TeammateSpawnRequest{
		SpawnID: "spawn-other", TeamName: "other", Input: agentcontract.Input{Name: "worker"},
	}, func(context.Context, agentcontract.TeammateIdentity) (agentcontract.TeammateLaunch, error) {
		called = true
		return agentcontract.TeammateLaunch{}, nil
	})
	if err != nil || !result.IsError || called {
		t.Fatalf("cross-team spawn error=%v result=%#v launcher_called=%t", err, result, called)
	}
}

func TestAgentCollaborationSpawnerFailsClosedWithoutDependencies(t *testing.T) {
	var spawner *agentCollaborationSpawner
	result, err := spawner.SpawnTeammate(
		context.Background(),
		agentcontract.TeammateSpawnRequest{},
		nil,
	)
	if err != nil {
		t.Fatalf("SpawnTeammate returned infrastructure error: %v", err)
	}
	if !result.IsError || result.Outcome != types.ToolOutcomeFailed || result.Content == "" {
		t.Fatalf("SpawnTeammate result = %#v, want a failed tool result", result)
	}
	if spawner.CurrentTeamName() != "" {
		t.Fatal("nil spawner reported an active team")
	}
	if spawner.TeamExists("team") {
		t.Fatal("nil spawner reported a durable team")
	}
}
