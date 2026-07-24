package tools

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

func TestTask23RetainedAgentRejectsDifferentSessionProjectOwnerAndAcceptsExactOwner(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	background := NewBackgroundTaskManager(rootA)
	t.Cleanup(background.Shutdown)

	agentProvider := &task23TeamSkillProvider{steps: [][]types.StreamEvent{
		task23TeamSkillTextEvents("retained A completed"),
	}}
	agentLoop := loop.New(agentProvider, registry.New(), loop.Config{
		MaxTurns: 1, SessionID: "retained-agent", ProjectRoot: rootA, CWD: rootA,
	})
	launchContext := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		SessionID: "session-a", SessionProjectDir: "project-a", ProjectRoot: rootA, CWD: rootA,
	})
	_, registered, err := background.RegisterAgentSession(
		"retained-agent", "retained-a", "initial", "retained A",
		AgentInput{Prompt: "initial", Description: "retained A"},
		agentLoop, agentSessionMetadata{}, func() {}, nil, launchContext,
	)
	if err != nil {
		t.Fatalf("register retained A: %v", err)
	}
	if registered.OwnerSessionID != "session-a" || !sameRetainedAgentOwnerRoot(registered.OwnerProjectRoot, rootA) {
		t.Fatalf("registered owner = session %q root %q", registered.OwnerSessionID, registered.OwnerProjectRoot)
	}

	teamManager := NewTeamManager(nil)
	teamManager.Background = background
	teamManager.SetSessionRuntime(TeamSessionRuntime{
		SessionID: "session-b", CWD: rootB,
		ToolRuntime: types.ToolRuntimeContext{SessionID: "session-b", ProjectRoot: rootB, AllowedDirs: []string{rootB}},
	})
	bProvider := &task23TeamSkillProvider{steps: [][]types.StreamEvent{
		task23GenericTeamToolEvents("send-b-to-a", "SendMessage", map[string]any{
			"to": "retained-a", "summary": "cross owner", "message": "must not run",
		}),
		task23TeamSkillTextEvents("B observed rejection"),
	}}
	bRegistry := registry.New()
	bRegistry.Register(NewSendMessageTool(teamManager))
	bLoop := loop.New(bProvider, bRegistry, loop.Config{
		MaxTurns: 2, SessionID: "session-b", SessionProjectDir: "project-b", ProjectRoot: rootB, CWD: rootB,
	})
	if err := bLoop.Run(context.Background(), "try A agent", func(loop.Event) {}); err != nil {
		t.Fatalf("B owner query: %v", err)
	}
	blocked, ok := background.Snapshot("retained-agent")
	if !ok || blocked.QueuedPrompts != 0 || blocked.Status == "running" {
		t.Fatalf("B mutated retained A: %+v found=%v", blocked, ok)
	}
	if calls := agentProvider.Calls(); len(calls) != 0 {
		t.Fatalf("B reached retained A provider: %d calls", len(calls))
	}

	background.SetProjectRoot(rootA)
	teamManager.SetSessionRuntime(TeamSessionRuntime{
		SessionID: "session-a", CWD: rootA,
		ToolRuntime: types.ToolRuntimeContext{SessionID: "session-a", ProjectRoot: rootA, AllowedDirs: []string{rootA}},
	})
	aProvider := &task23TeamSkillProvider{steps: [][]types.StreamEvent{
		task23GenericTeamToolEvents("send-a-to-a", "SendMessage", map[string]any{
			"to": "retained-a", "summary": "same owner", "message": "continue safely",
		}),
		task23TeamSkillTextEvents("A resumed its agent"),
	}}
	aRegistry := registry.New()
	aRegistry.Register(NewSendMessageTool(teamManager))
	aLoop := loop.New(aProvider, aRegistry, loop.Config{
		MaxTurns: 2, SessionID: "session-a", SessionProjectDir: "project-a", ProjectRoot: rootA, CWD: rootA,
	})
	if err := aLoop.Run(context.Background(), "resume A agent", func(loop.Event) {}); err != nil {
		t.Fatalf("A owner query: %v", err)
	}
	if finished, status := background.Wait("retained-agent", 5*time.Second); status != "success" || finished.Status != "completed" {
		t.Fatalf("same-owner retained run = status %q snapshot %+v", status, finished)
	}
	if calls := agentProvider.Calls(); len(calls) != 1 {
		t.Fatalf("same owner provider calls = %d, want 1", len(calls))
	}
}

func TestTask23PersistedRetainedAgentRejectsDifferentSessionBeforeRestore(t *testing.T) {
	root := t.TempDir()
	background := NewBackgroundTaskManager(root)
	t.Cleanup(background.Shutdown)
	const agentID = "persisted-agent-a"
	if err := NewRuntimeTaskStore(root).Save(RuntimeTaskRecord{
		ID: agentID, Type: backgroundTaskTypeLocalAgent, Status: "completed",
		OwnerSessionID: "session-a", OwnerProjectRoot: root,
	}); err != nil {
		t.Fatal(err)
	}
	factoryCalls := 0
	background.SetAgentSessionFactory(func(string, RuntimeTaskRecord) (*backgroundAgentSession, *BackgroundTaskSnapshot, error) {
		factoryCalls++
		return nil, nil, errors.New("restore must remain unreachable")
	})
	teamManager := NewTeamManager(nil)
	teamManager.Background = background
	teamManager.SetSessionRuntime(TeamSessionRuntime{
		SessionID: "session-b", CWD: root,
		ToolRuntime: types.ToolRuntimeContext{SessionID: "session-b", ProjectRoot: root, AllowedDirs: []string{root}},
	})
	provider := &task23TeamSkillProvider{steps: [][]types.StreamEvent{
		task23GenericTeamToolEvents("restore-b-to-a", "SendMessage", map[string]any{
			"to": agentID, "summary": "cross session restore", "message": "must not restore",
		}),
		task23TeamSkillTextEvents("restore rejected"),
	}}
	reg := registry.New()
	reg.Register(NewSendMessageTool(teamManager))
	query := loop.New(provider, reg, loop.Config{
		MaxTurns: 2, SessionID: "session-b", SessionProjectDir: root, ProjectRoot: root, CWD: root,
	})
	if err := query.Run(context.Background(), "try persisted A", func(loop.Event) {}); err != nil {
		t.Fatal(err)
	}
	if factoryCalls != 0 {
		t.Fatalf("cross-session request invoked restore factory %d times", factoryCalls)
	}
}

func TestTask23TeamOwnerCannotDispatchAcrossSessionProject(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := newTestManagerForHome(t, t.TempDir())
	manager.SetProjectRoot(rootA)
	manager.SetSessionRuntime(TeamSessionRuntime{
		SessionID: "session-a", CWD: rootA,
		ToolRuntime: types.ToolRuntimeContext{SessionID: "session-a", ProjectRoot: rootA, AllowedDirs: []string{rootA}},
	})
	created, err := NewTeamCreateTool(manager).Execute(context.Background(), map[string]any{
		"team_name": "task23-owner-team", "agent_type": "executor",
	})
	if err != nil || created.IsError {
		t.Fatalf("create A team = %+v err=%v", created, err)
	}
	manager.mu.Lock()
	teamID := manager.activeTeamID
	info := manager.teams[teamID]
	manager.mu.Unlock()
	if info == nil || info.coordinator == nil {
		t.Fatalf("created team state = id %q info %+v", teamID, info)
	}
	t.Cleanup(func() { _ = swarm.DeleteTeamConfig(info.StorageName) })

	manager.SetProjectRoot(rootB)
	manager.SetSessionRuntime(TeamSessionRuntime{
		SessionID: "session-b", CWD: rootB,
		ToolRuntime: types.ToolRuntimeContext{SessionID: "session-b", ProjectRoot: rootB, AllowedDirs: []string{rootB}},
	})
	denied, err := NewTeamDispatchTool(manager).Execute(context.Background(), map[string]any{
		"team_id": teamID,
		"tasks":   []any{map[string]any{"description": "B must not enqueue on A"}},
	})
	if err != nil || !denied.IsError {
		t.Fatalf("B cross-owner dispatch = %+v err=%v", denied, err)
	}
	if tasks := info.coordinator.GetTasks(); len(tasks) != 0 {
		t.Fatalf("B queued work on A coordinator: %+v", tasks)
	}

	manager.SetProjectRoot(rootA)
	manager.SetSessionRuntime(TeamSessionRuntime{
		SessionID: "session-a", CWD: rootA,
		ToolRuntime: types.ToolRuntimeContext{SessionID: "session-a", ProjectRoot: rootA, AllowedDirs: []string{rootA}},
	})
	accepted, err := NewTeamDispatchTool(manager).Execute(context.Background(), map[string]any{
		"team_id": teamID,
		"tasks":   []any{map[string]any{"description": "A dispatches on A"}},
	})
	if err != nil || accepted.IsError {
		t.Fatalf("A same-owner dispatch = %+v err=%v", accepted, err)
	}
	if tasks := info.coordinator.GetTasks(); len(tasks) != 1 || tasks[0].Description != "A dispatches on A" {
		t.Fatalf("A coordinator tasks = %+v", tasks)
	}

	stateA, err := NewRuntimeLifecycle(rootA).ActiveState()
	if err != nil {
		t.Fatal(err)
	}
	foundA := false
	for _, event := range stateA {
		if event.Type == LifecycleTeamCreate && event.EntityID == teamID && event.SessionID == "session-a" {
			foundA = true
		}
	}
	if !foundA {
		t.Fatalf("A lifecycle lost its team creation: %+v", stateA)
	}
	stateB, err := NewRuntimeLifecycle(rootB).ActiveState()
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range stateB {
		if event.EntityID == teamID {
			t.Fatalf("A team leaked into B lifecycle: %+v", event)
		}
	}
}

func TestTask23AgentTeamRegistrationLeaseBlocksProjectRetarget(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeTask23TeamSkill(t, filepath.Join(rootA, ".luban-code", "skills"), "lease-a")
	writeTask23TeamSkill(t, filepath.Join(rootB, ".luban-code", "skills"), "lease-b")
	store, err := skills.NewFileOverrideStore(rootA, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := skills.ProjectDirs(rootA)
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManagerWithOverrideStore(store, dirs...)
	bindingA, err := manager.SnapshotBinding("lease-owner")
	if err != nil {
		t.Fatal(err)
	}
	planB, err := manager.PrepareProjectSources(rootB)
	if err != nil {
		t.Fatal(err)
	}
	authority := toolSkillAuthority{generation: bindingA.ProjectGeneration, pinned: true}
	leaseEntered := make(chan struct{})
	releaseLease := make(chan struct{})
	leaseDone := make(chan error, 1)
	go func() {
		leaseDone <- authority.withGenerationLease(manager, func() error {
			close(leaseEntered)
			<-releaseLease
			return nil
		})
	}()
	<-leaseEntered

	retargetDone := make(chan error, 1)
	go func() { retargetDone <- manager.ApplyProjectSources(planB) }()
	select {
	case err := <-retargetDone:
		t.Fatalf("retarget crossed Agent/Team registration lease: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseLease)
	if err := <-leaseDone; err != nil {
		t.Fatal(err)
	}
	if err := <-retargetDone; err != nil {
		t.Fatal(err)
	}
	if err := authority.withGenerationLease(manager, func() error {
		t.Fatal("stale registration callback executed")
		return nil
	}); !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
		t.Fatalf("stale registration lease error = %v", err)
	}
}
