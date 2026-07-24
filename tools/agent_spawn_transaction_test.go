package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

func newTransactionalTeammateTool(t *testing.T) (*AgentTool, *TeamManager, *BackgroundTaskManager) {
	t.Helper()
	mgr := newTestManager(t)
	background := NewBackgroundTaskManager(mgr.CWD)
	t.Cleanup(background.Shutdown)
	p := &mockProvider{responses: []string{"done"}}
	reg := registry.New()
	mgr.Provider = p
	mgr.Registry = reg
	mgr.Background = background
	created, err := NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{"team_name": "alpha"})
	if err != nil || created.IsError {
		t.Fatalf("create team: result=%#v err=%v", created, err)
	}
	tool := &AgentTool{Provider: p, Registry: reg, Background: background, TeamManager: mgr}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		ProjectRoot: mgr.CWD, AllowedDirs: []string{mgr.CWD}, PermissionMode: "plan",
	}})
	return tool, mgr, background
}

func TestTeammateSpawnFaultsRollbackReservationTaskAndAgent(t *testing.T) {
	stages := []string{"config_reserve", "agent_create", "response_prepare", "config_activate", "task_enqueue"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			tool, mgr, background := newTransactionalTeammateTool(t)
			tool.teammateSpawnFault = func(current string) error {
				if current == stage {
					return fmt.Errorf("injected %s failure", stage)
				}
				return nil
			}
			result, err := tool.Execute(context.Background(), agentExecuteInput("build", map[string]any{
				"name": "Builder", "team_name": "alpha",
			}))
			if err != nil || !result.IsError {
				t.Fatalf("spawn result=%#v err=%v", result, err)
			}
			cfg, err := swarm.LoadTeamConfig("alpha")
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.Members) != 1 || cfg.Members[0].Name != teamLeadName {
				t.Fatalf("orphan team member after %s: %#v", stage, cfg.Members)
			}
			if _, ok := background.ResolveAgentTarget("Builder@alpha"); ok {
				t.Fatalf("orphan retained agent after %s", stage)
			}
			for _, snapshot := range background.InMemorySnapshots() {
				if snapshot.ID == "Builder@alpha" {
					t.Fatalf("orphan live task after %s: %#v", stage, snapshot)
				}
			}
			info := mgr.currentTeamInfo()
			if info == nil || len(info.Agents) != 1 {
				t.Fatalf("manager inventory after %s: %#v", stage, info)
			}
		})
	}
}

func TestRollbackRegisteredAgentSessionIsRetryableAndCleansOnce(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	var cleanups atomic.Int32
	session, _, err := manager.RegisterAgentSession(
		"agent-rollback", "alias", "prompt", "description", AgentInput{Prompt: "prompt"}, nil,
		agentSessionMetadata{CWD: root}, func() { cleanups.Add(1) }, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.rollbackRegisteredAgentSession("agent-rollback", session); err != nil {
		t.Fatal(err)
	}
	if err := manager.rollbackRegisteredAgentSession("agent-rollback", session); err != nil {
		t.Fatal(err)
	}
	if cleanups.Load() != 1 {
		t.Fatalf("cleanup count=%d", cleanups.Load())
	}
	if _, ok := manager.ResolveAgentTarget("agent-rollback"); ok {
		t.Fatal("rolled back agent remains addressable")
	}
	if _, ok := manager.ResolveAgentTarget("alias"); ok {
		t.Fatal("rolled back alias remains addressable")
	}
}

func TestConcurrentSameNameTeammateSpawnsReserveUniqueAgents(t *testing.T) {
	tool, _, background := newTransactionalTeammateTool(t)
	const count = 12
	start := make(chan struct{})
	results := make(chan error, count)
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := tool.Execute(context.Background(), agentExecuteInput("build", map[string]any{
				"name": "Builder", "team_name": "alpha",
			}))
			if err != nil {
				results <- err
				return
			}
			if result.IsError {
				results <- errors.New(result.Content)
				return
			}
			results <- nil
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := swarm.LoadTeamConfig("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Members) != count+1 {
		t.Fatalf("members=%d want=%d: %#v", len(cfg.Members), count+1, cfg.Members)
	}
	ids := make(map[string]struct{}, count)
	spawnIDs := make(map[string]struct{}, count)
	for _, member := range cfg.Members {
		if member.Name == teamLeadName {
			continue
		}
		if _, duplicate := ids[member.AgentID]; duplicate {
			t.Fatalf("duplicate agent id %s", member.AgentID)
		}
		ids[member.AgentID] = struct{}{}
		if member.SpawnID == "" {
			t.Fatalf("missing spawn id: %#v", member)
		}
		if _, duplicate := spawnIDs[member.SpawnID]; duplicate {
			t.Fatalf("duplicate spawn id %s", member.SpawnID)
		}
		spawnIDs[member.SpawnID] = struct{}{}
		if member.Lifecycle != "active" || !member.IsActive {
			t.Fatalf("member did not commit active: %#v", member)
		}
		if _, ok := background.ResolveAgentTarget(member.AgentID); !ok {
			t.Fatalf("member %s has no retained agent", member.AgentID)
		}
	}
}

func TestConcurrentTeamCreateFailureCannotDeleteWinnerConfig(t *testing.T) {
	mgr := newTestManager(t)
	start := make(chan struct{})
	results := make(chan types.ToolResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{"team_name": "alpha"})
			results <- result
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var successes int
	for result := range results {
		if !result.IsError {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful creates=%d", successes)
	}
	config, err := swarm.LoadTeamConfig("alpha")
	if err != nil || config.LeadAgentID == "" {
		t.Fatalf("winner config was deleted: %#v err=%v", config, err)
	}
	if info := mgr.currentTeamInfo(); info == nil || info.Name != "alpha" {
		t.Fatalf("winner manager state=%#v", info)
	}
}
