package tools

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

var subagentPermissionTransitionTools = []string{
	"EnterPlanMode",
	"ExitPlanMode",
	"AskUserQuestion",
	"EnterWorktree",
	"ExitWorktree",
}

type permissionTransitionVisibilityProvider struct {
	mu     sync.Mutex
	params []provider.Params
}

func (*permissionTransitionVisibilityProvider) Name() string {
	return "permission-transition-visibility"
}

func (*permissionTransitionVisibilityProvider) ModelID() string {
	return "permission-transition-visibility-model"
}

func (p *permissionTransitionVisibilityProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	p.params = append(p.params, params)
	p.mu.Unlock()

	return eventStream(agentTextEvents("done")), nil
}

func (p *permissionTransitionVisibilityProvider) firstToolNames(t *testing.T) map[string]bool {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.params) == 0 {
		t.Fatal("provider received no request")
	}
	names := make(map[string]bool, len(p.params[0].Tools))
	for _, definition := range p.params[0].Tools {
		names[definition.Name] = true
	}
	return names
}

func assertPermissionTransitionToolsHidden(t *testing.T, names map[string]bool) {
	t.Helper()
	for _, name := range subagentPermissionTransitionTools {
		if names[name] {
			t.Errorf("subagent provider request exposed permission interaction tool %q: %#v", name, names)
		}
	}
}

func permissionTransitionSourceRegistry() *registry.Registry {
	reg := registry.New()
	reg.Register(fakeTool{name: "Read"})
	for _, name := range subagentPermissionTransitionTools {
		reg.Register(fakeTool{name: name})
	}
	return reg
}

func TestForkExactToolRegistryStillHidesPermissionTransitionTools(t *testing.T) {
	t.Setenv("FORK_SUBAGENT", "1")
	t.Setenv("CLAUDE_CODE_FORK_SUBAGENT", "")

	root := t.TempDir()
	background := NewBackgroundTaskManager(root)
	t.Cleanup(background.Shutdown)
	provider := &permissionTransitionVisibilityProvider{}
	tool := &AgentTool{
		Provider:   provider,
		Registry:   permissionTransitionSourceRegistry(),
		Background: background,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		SessionID:      "fork-parent-session",
		ProjectRoot:    root,
		AllowedDirs:    []string{root},
		PermissionMode: "plan",
	}})
	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		Messages: []types.Message{types.UserMessage("parent context")},
		ToolUse: types.ToolUseBlock{
			Type: types.ContentTypeToolUse,
			ID:   "toolu_fork_permission_visibility",
			Name: "Agent",
		},
	})

	result, err := tool.Execute(ctx, map[string]any{
		"description": "inspect permissions",
		"prompt":      "Inspect without changing permission mode.",
	})
	if err != nil {
		t.Fatalf("fork Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("fork Execute returned tool error: %s", result.Content)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		provider.mu.Lock()
		called := len(provider.params) > 0
		provider.mu.Unlock()
		if called {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fork provider was not called")
		}
		time.Sleep(time.Millisecond)
	}

	assertPermissionTransitionToolsHidden(t, provider.firstToolNames(t))
}

func TestTeamWorkerRegistryHidesPermissionTransitionTools(t *testing.T) {
	root := t.TempDir()
	provider := &permissionTransitionVisibilityProvider{}
	mgr := newTestManagerForHome(t, root)
	mgr.Provider = provider
	mgr.Registry = permissionTransitionSourceRegistry()
	mgr.System = "team permission visibility test"
	mgr.SetSessionRuntime(TeamSessionRuntime{
		System:    mgr.System,
		SessionID: "team-parent-session",
		CWD:       root,
		ToolRuntime: types.ToolRuntimeContext{
			SessionID:      "team-parent-session",
			ProjectRoot:    root,
			AllowedDirs:    []string{root},
			PermissionMode: "plan",
		},
	})

	result, err := NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{
		"team_name":  "permission-visibility",
		"agent_type": "executor",
	})
	if err != nil {
		t.Fatalf("TeamCreate: %v", err)
	}
	if result.IsError {
		t.Fatalf("TeamCreate returned tool error: %s", result.Content)
	}
	mgr.coordinator.AddTask("Inspect without changing permission mode.", 1)
	dispatched := mgr.coordinator.Dispatch(context.Background())
	if len(dispatched) != 1 {
		t.Fatalf("dispatched task count = %d, want 1", len(dispatched))
	}
	if dispatched[0].Error != nil {
		t.Fatalf("team worker failed: %v", dispatched[0].Error)
	}

	assertPermissionTransitionToolsHidden(t, provider.firstToolNames(t))
}
