package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestBackgroundAgentPinsExplicitDefaultPermissionMode(t *testing.T) {
	parent := &captureToolPermissionHandler{}
	handler := agentPermissionHandlerForSnapshot(types.ToolRuntimeContext{PermissionMode: "default"}, parent, approvalRouteFailClosed, agentProfile{})
	if handler == nil {
		t.Fatal("expected permission handler")
	}
	if _, err := handler.Check(context.Background(), loop.PermissionRequest{ToolName: "Read"}); err != nil {
		t.Fatal(err)
	}
	if len(parent.requests) != 1 || parent.requests[0].Mode != "default" {
		t.Fatalf("background permission request did not pin default mode: %#v", parent.requests)
	}
}

func TestAgentLaunchRuntimeSnapshotKeepsWorkspaceFieldsTogether(t *testing.T) {
	origin := t.TempDir()
	next := t.TempDir()
	barrier := &sync.RWMutex{}
	reg := registry.New()
	reg.Register(&BashTool{CWD: origin, OriginalCWD: origin, AllowedDirs: []string{origin}})
	tool := &AgentTool{Registry: reg}
	tool.SetSessionBarrier(barrier)
	tool.SetSessionRuntime(AgentSessionRuntime{
		System: "origin-system",
		ToolRuntime: types.ToolRuntimeContext{
			SessionID: "origin-session", ProjectRoot: origin, AllowedDirs: []string{origin}, PermissionMode: "default",
		},
	})

	snapshot := tool.captureLaunchRuntime()
	barrier.Lock()
	tool.SetSessionRuntime(AgentSessionRuntime{
		System: "next-system",
		ToolRuntime: types.ToolRuntimeContext{
			SessionID: "next-session", ProjectRoot: next, AllowedDirs: []string{next}, PermissionMode: "bypassPermissions",
		},
	})
	reg.Register(&BashTool{CWD: next, OriginalCWD: next, AllowedDirs: []string{next}})
	barrier.Unlock()

	if snapshot.session.System != "origin-system" || snapshot.session.ToolRuntime.SessionID != "origin-session" || snapshot.session.ToolRuntime.ProjectRoot != origin {
		t.Fatalf("launch snapshot mixed workspace fields: %+v", snapshot.session)
	}
	childBash, ok := snapshot.registry.Get("Bash").(*BashTool)
	if !ok || childBash.CWD != origin || len(childBash.AllowedDirs) != 1 || childBash.AllowedDirs[0] != origin {
		t.Fatalf("launch registry followed later workspace: %+v", childBash)
	}
}

func TestTeamLaunchRuntimeSnapshotKeepsWorkspaceFieldsTogether(t *testing.T) {
	origin := t.TempDir()
	next := t.TempDir()
	barrier := &sync.RWMutex{}
	reg := registry.New()
	reg.Register(&BashTool{CWD: origin, OriginalCWD: origin, AllowedDirs: []string{origin}})
	mgr := NewTeamManager(nil)
	mgr.Registry = reg
	mgr.SetSessionBarrier(barrier)
	mgr.SetSessionRuntime(TeamSessionRuntime{
		System: "origin-system", SessionID: "origin-session", CWD: origin,
		ToolRuntime: types.ToolRuntimeContext{SessionID: "origin-session", ProjectRoot: origin, AllowedDirs: []string{origin}, PermissionMode: "default"},
	})

	snapshot := mgr.captureLaunchRuntime()
	barrier.Lock()
	mgr.SetSessionRuntime(TeamSessionRuntime{
		System: "next-system", SessionID: "next-session", CWD: next,
		ToolRuntime: types.ToolRuntimeContext{SessionID: "next-session", ProjectRoot: next, AllowedDirs: []string{next}, PermissionMode: "bypassPermissions"},
	})
	reg.Register(&BashTool{CWD: next, OriginalCWD: next, AllowedDirs: []string{next}})
	barrier.Unlock()

	if snapshot.session.System != "origin-system" || snapshot.session.SessionID != "origin-session" || snapshot.session.CWD != origin || snapshot.session.ToolRuntime.ProjectRoot != origin {
		t.Fatalf("team launch snapshot mixed workspace fields: %+v", snapshot.session)
	}
	childBash, ok := snapshot.registry.Get("Bash").(*BashTool)
	if !ok || childBash.CWD != origin || len(childBash.AllowedDirs) != 1 || childBash.AllowedDirs[0] != origin {
		t.Fatalf("team launch registry followed later workspace: %+v", childBash)
	}
}

func TestAgentLaunchRuntimeSnapshotIsConsistentDuringSessionPublication(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	barrier := &sync.RWMutex{}
	reg := registry.New()
	reg.Register(&BashTool{CWD: rootA, OriginalCWD: rootA, AllowedDirs: []string{rootA}})
	tool := &AgentTool{Registry: reg}
	tool.SetSessionBarrier(barrier)
	tool.SetSessionRuntime(AgentSessionRuntime{System: "A", ToolRuntime: types.ToolRuntimeContext{ProjectRoot: rootA, AllowedDirs: []string{rootA}}})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			marker, root := "A", rootA
			if i%2 == 1 {
				marker, root = "B", rootB
			}
			barrier.Lock()
			tool.SetSessionRuntime(AgentSessionRuntime{System: marker, ToolRuntime: types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}}})
			reg.Register(&BashTool{CWD: root, OriginalCWD: root, AllowedDirs: []string{root}})
			barrier.Unlock()
		}
	}()
	for i := 0; i < 100; i++ {
		snapshot := tool.captureLaunchRuntime()
		bash, ok := snapshot.registry.Get("Bash").(*BashTool)
		if !ok {
			t.Fatalf("snapshot Bash = %T", snapshot.registry.Get("Bash"))
		}
		wantRoot := rootA
		if snapshot.session.System == "B" {
			wantRoot = rootB
		}
		if snapshot.session.ToolRuntime.ProjectRoot != wantRoot || bash.CWD != wantRoot || len(bash.AllowedDirs) != 1 || bash.AllowedDirs[0] != wantRoot {
			t.Fatalf("mixed launch snapshot: session=%+v bash=%+v", snapshot.session, bash)
		}
	}
	<-done
}

func TestPinnedTodoStoreConcurrentUpdatesAreLossless(t *testing.T) {
	root := t.TempDir()
	store := NewTodoStore(root).withAgentScope("agent-concurrent", root)
	const writers = 24
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, _, err := store.LoadAndSave(func(old []TodoItem) ([]TodoItem, error) {
				return append(old, TodoItem{Content: fmt.Sprintf("todo-%d", index), Status: "pending", ActiveForm: "waiting"}), nil
			})
			if err != nil {
				t.Errorf("LoadAndSave(%d): %v", index, err)
			}
		}(i)
	}
	wg.Wait()
	if got := len(store.Load()); got != writers {
		t.Fatalf("concurrent todo count = %d, want %d", got, writers)
	}
}

func TestPinnedBackgroundAgentTodoRemainsInOriginProject(t *testing.T) {
	origin := t.TempDir()
	foreground := t.TempDir()
	scope := NewRuntimeScope(origin, true)
	scope.SetSessionIDFunc(func() string { return "origin-session" })
	store := NewTodoStore(origin)
	store.SetScopeResolver(scope)

	reg := registry.New()
	reg.Register(NewTodoWriteTool(store))
	snapshot := scope.ToolRuntimeContext()
	childRuntime := agentRuntimeContextProvider{snapshot: snapshot, agentID: "agent-origin"}
	pinRegistryForAgentRuntime(reg, childRuntime, snapshot)

	// A foreground session switch after the background agent was launched must
	// not retarget the retained agent's TodoWrite store.
	scope.SetProjectRoot(foreground)
	scope.SetAllowedDirs([]string{foreground})
	scope.SetSessionIDFunc(func() string { return "foreground-session" })

	result, err := reg.Get("TodoWrite").Execute(context.Background(), map[string]any{
		"todos": []any{map[string]any{
			"content": "stay in origin", "status": "pending", "activeForm": "staying in origin",
		}},
	})
	if err != nil || result.IsError {
		t.Fatalf("TodoWrite result=%+v err=%v", result, err)
	}
	originPath := filepath.Join(origin, ".claude", "todos", "agent-origin.json")
	if _, err := os.Stat(originPath); err != nil {
		t.Fatalf("origin agent todo was not written at %q: %v", originPath, err)
	}
	foregroundPath := filepath.Join(foreground, ".claude", "todos", "agent-origin.json")
	if _, err := os.Stat(foregroundPath); !os.IsNotExist(err) {
		t.Fatalf("background todo followed foreground project to %q: %v", foregroundPath, err)
	}
}

func TestPinnedTeamTodoUsesOriginProjectAndAgentKey(t *testing.T) {
	origin := t.TempDir()
	foreground := t.TempDir()
	scope := NewRuntimeScope(origin, true)
	scope.SetSessionIDFunc(func() string { return "leader-session" })
	store := NewTodoStore(origin)
	store.SetScopeResolver(scope)

	teamRegistry := registry.New()
	teamRegistry.Register(NewTodoWriteTool(store))
	snapshot := scope.ToolRuntimeContext()
	teammateRuntime := agentRuntimeContextProvider{snapshot: snapshot, agentID: "reviewer@alpha"}
	pinRegistryForAgentRuntime(teamRegistry, teammateRuntime, snapshot)

	scope.SetProjectRoot(foreground)
	scope.SetSessionIDFunc(func() string { return "other-session" })

	result, err := teamRegistry.Get("TodoWrite").Execute(context.Background(), map[string]any{
		"todos": []any{map[string]any{
			"content": "team task", "status": "in_progress", "activeForm": "working team task",
		}},
	})
	if err != nil || result.IsError {
		t.Fatalf("TodoWrite result=%+v err=%v", result, err)
	}
	want := filepath.Join(origin, ".claude", "todos", sanitizeTaskPathComponent("reviewer@alpha")+".json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("team todo did not use origin project and agent key %q: %v", want, err)
	}
	wrong := filepath.Join(foreground, ".claude", "todos", "other-session.json")
	if _, err := os.Stat(wrong); !os.IsNotExist(err) {
		t.Fatalf("team todo followed foreground/session identity to %q: %v", wrong, err)
	}
}

func TestPinnedAgentPlanGateDoesNotFollowForegroundSession(t *testing.T) {
	origin := t.TempDir()
	foreground := t.TempDir()
	foregroundPlan := NewPlanState(foreground)
	reg := registry.New()
	parentWrite := &FileWriteTool{AllowedDirs: []string{origin}, PlanState: foregroundPlan}
	reg.Register(parentWrite)
	snapshot := types.ToolRuntimeContext{ProjectRoot: origin, AllowedDirs: []string{origin}, PermissionMode: "default"}
	childRuntime := agentRuntimeContextProvider{snapshot: snapshot, agentID: "agent-plan"}
	pinRegistryForAgentRuntime(reg, childRuntime, snapshot)

	// The foreground enters plan mode after the child registry exists. The
	// background child must keep its launch-time plan gate.
	foregroundPlan.enter(filepath.Join(foreground, "plan.md"))
	childWrite, ok := reg.Get("Write").(*FileWriteTool)
	if !ok {
		t.Fatalf("child Write = %T", reg.Get("Write"))
	}
	decision, err := childWrite.CheckPermissions(context.Background(), map[string]any{
		"file_path": filepath.Join(origin, "result.txt"), "content": "ok",
	}, types.ToolPermissionRequest{Runtime: childRuntime.ToolRuntimeContext()})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Behavior == types.PermissionBehaviorDeny {
		t.Fatalf("child plan gate followed foreground PlanState: %+v", decision)
	}
}

func TestPinnedAgentPlanRuntimeDoesNotTransitionForegroundPermission(t *testing.T) {
	origin := t.TempDir()
	foregroundRuntime := NewRuntimeScope(origin, true)
	plan := NewPlanState(origin)
	reg := registry.New()
	reg.Register(NewEnterPlanModeTool(plan, foregroundRuntime))
	snapshot := types.ToolRuntimeContext{ProjectRoot: origin, AllowedDirs: []string{origin}, PermissionMode: "default"}
	childRuntime := agentRuntimeContextProvider{snapshot: snapshot, agentID: "agent-plan-runtime"}
	pinRegistryForAgentRuntime(reg, childRuntime, snapshot)

	childEnter, ok := reg.Get("EnterPlanMode").(*EnterPlanModeTool)
	if !ok {
		t.Fatalf("child EnterPlanMode = %T", reg.Get("EnterPlanMode"))
	}
	if childEnter.Runtime == foregroundRuntime {
		t.Fatal("child EnterPlanMode retained foreground permission runtime")
	}
	if err := childEnter.Runtime.TransitionPermissionMode("plan"); err != nil {
		t.Fatal(err)
	}
	if got := foregroundRuntime.PermissionMode(); got != "default" {
		t.Fatalf("child plan transition changed foreground permission mode to %q", got)
	}
}
