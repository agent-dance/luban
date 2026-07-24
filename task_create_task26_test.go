package main

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	tuit "github.com/agent-dance/luban/tui"
)

func TestTaskCreateTask26SetupRegistryMutualGating(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		enableTasks string
		wantTaskV2  bool
	}{
		{name: "interactive", interactive: true, wantTaskV2: true},
		{name: "non-interactive", interactive: false, wantTaskV2: false},
		{name: "non-interactive forced", interactive: false, enableTasks: "yes", wantTaskV2: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CODE_ENABLE_TASKS", tc.enableTasks)
			deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil, tc.interactive)
			if deps.CronStore != nil {
				t.Cleanup(deps.CronStore.Stop)
			}
			enabled := make(map[string]bool)
			for _, tool := range deps.Registry.EnabledTools() {
				enabled[tool.Name()] = true
			}
			for _, name := range []string{"TaskCreate", "TaskGet", "TaskUpdate", "TaskList"} {
				if enabled[name] != tc.wantTaskV2 {
					t.Errorf("%s enabled=%v, want %v; all=%v", name, enabled[name], tc.wantTaskV2, enabled)
				}
			}
			if enabled["TodoWrite"] == tc.wantTaskV2 {
				t.Errorf("TodoWrite enabled=%v, want inverse of task-v2=%v", enabled["TodoWrite"], tc.wantTaskV2)
			}
			if !enabled["TaskStop"] || !enabled["TaskOutput"] {
				t.Errorf("TaskStop/TaskOutput must be gate-independent: %v", enabled)
			}
		})
	}
}

func TestTaskCreateTask26ShowsCollapsedInteractiveViewBinding(t *testing.T) {
	t.Setenv("CLAUDE_HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "task26-view")
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil, true)
	if deps.CronStore != nil {
		t.Cleanup(deps.CronStore.Stop)
	}
	tool := deps.TaskCreateTool
	state := tuit.NewAppState()
	unbind := bindTaskCreateViewState(tool, state)

	result, err := tool.Execute(context.Background(), map[string]any{
		"subject": "show tasks", "description": "expand interactive task view",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%+v err=%v", result, err)
	}
	if state.ExpandedView.Get() != "" || state.TaskListRevision.Get() != 1 {
		t.Fatalf("task view state = expanded:%q revision:%d", state.ExpandedView.Get(), state.TaskListRevision.Get())
	}
	items := state.TaskViewItems.Get()
	if len(items) != 1 || items[0].ID != "1" || items[0].Subject != "show tasks" || items[0].Status != "pending" {
		t.Fatalf("renderable task view snapshot = %+v", items)
	}
	updated, err := deps.Registry.ExecuteToolWithError(context.Background(), "TaskUpdate", map[string]any{"taskId": "1", "status": "in_progress"})
	if err != nil || updated.IsError {
		t.Fatalf("TaskUpdate: result=%+v err=%v", updated, err)
	}
	if items = state.TaskViewItems.Get(); len(items) != 1 || items[0].Status != "in_progress" || state.TaskListRevision.Get() != 2 {
		t.Fatalf("updated task view snapshot=%+v revision=%d", items, state.TaskListRevision.Get())
	}
	deleted, err := deps.Registry.ExecuteToolWithError(context.Background(), "TaskUpdate", map[string]any{"taskId": "1", "status": "deleted"})
	if err != nil || deleted.IsError {
		t.Fatalf("TaskUpdate delete: result=%+v err=%v", deleted, err)
	}
	if items = state.TaskViewItems.Get(); len(items) != 0 || state.TaskListRevision.Get() != 3 {
		t.Fatalf("deleted task view snapshot=%+v revision=%d", items, state.TaskListRevision.Get())
	}

	unbind()
	_, _ = tool.Execute(context.Background(), map[string]any{
		"subject": "after unbind", "description": "must not touch closed UI",
	})
	if state.TaskListRevision.Get() != 3 {
		t.Fatalf("unbound UI received refresh: %d", state.TaskListRevision.Get())
	}
}

func TestTaskCreateTask26RegistryDepsHookWiringAndRollback(t *testing.T) {
	t.Setenv("CLAUDE_HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "task26-runtime-hook")
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil, true)
	if deps.CronStore != nil {
		t.Cleanup(deps.CronStore.Stop)
	}
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookTaskCreated, Command: `printf '%s' '{"block":true,"system_reminder":"runtime policy"}'`, Timeout: 2,
	}})
	deps.SetHookRunner(runner)
	if deps.TaskCreateTool.HookRunner != runner || deps.AgentTool.SessionRuntime().HookRunner != runner || deps.TeamManager.SessionRuntime().HookRunner != runner {
		t.Fatal("registry hook consumers were not updated together")
	}
	result, err := deps.Registry.ExecuteToolWithError(context.Background(), "TaskCreate", map[string]any{
		"subject": "must be rejected", "description": "production registry hook wiring",
	})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "TaskCreated hook feedback:\nruntime policy") {
		t.Fatalf("registry TaskCreated block: result=%+v err=%v", result, err)
	}
}
