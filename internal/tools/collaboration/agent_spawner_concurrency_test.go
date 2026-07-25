package collaboration

import (
	"context"
	"fmt"
	"sync"
	"testing"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

func TestConcurrentTeammateReservationsAssignUniqueDurableIdentities(t *testing.T) {
	manager, identity, ctx := newFocusedTeamManager(t, "session-concurrent-spawn")
	created, err := NewTeamCreateTool(manager).Execute(ctx, map[string]any{"team_name": "alpha"})
	if err != nil || created.IsError {
		t.Fatalf("TeamCreate error=%v result=%#v", err, created)
	}
	spawner := NewAgentCollaborationSpawner(manager)

	const count = 12
	start := make(chan struct{})
	errs := make(chan error, count)
	var wait sync.WaitGroup
	for index := range count {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			spawnID := fmt.Sprintf("spawn-%d", index)
			result, err := spawner.SpawnTeammate(ctx, agentcontract.TeammateSpawnRequest{
				SpawnID: spawnID, TeamName: "alpha", ParentModel: identity.Model,
				Input: agentcontract.Input{Name: "Builder", SubagentType: "reviewer"},
			}, func(_ context.Context, assigned agentcontract.TeammateIdentity) (agentcontract.TeammateLaunch, error) {
				return agentcontract.TeammateLaunch{
					Result: types.ToolResult{Content: assigned.AgentID, Outcome: types.ToolOutcomeSucceeded},
					CWD:    identity.ProjectRoot, Model: identity.Model,
				}, nil
			})
			if err != nil {
				errs <- err
				return
			}
			if result.IsError {
				errs <- fmt.Errorf("spawn %s failed: %s", spawnID, result.Content)
				return
			}
			errs <- nil
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	config, err := swarm.LoadTeamConfig("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Members) != count+1 {
		t.Fatalf("members=%d, want %d: %#v", len(config.Members), count+1, config.Members)
	}
	agentIDs := make(map[string]struct{}, count)
	spawnIDs := make(map[string]struct{}, count)
	for _, member := range config.Members {
		if member.Name == teamLeadName {
			continue
		}
		if _, duplicate := agentIDs[member.AgentID]; duplicate {
			t.Fatalf("duplicate agent ID %q", member.AgentID)
		}
		agentIDs[member.AgentID] = struct{}{}
		if _, duplicate := spawnIDs[member.SpawnID]; member.SpawnID == "" || duplicate {
			t.Fatalf("invalid or duplicate spawn ID %q", member.SpawnID)
		}
		spawnIDs[member.SpawnID] = struct{}{}
		if member.Lifecycle != "active" || !member.IsActive {
			t.Fatalf("reservation did not commit active: %#v", member)
		}
	}
}
