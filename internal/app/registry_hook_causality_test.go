package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/hooks"
	agentruntime "github.com/agent-dance/luban/internal/agent"
	runtimescope "github.com/agent-dance/luban/internal/runtime/scope"
	taskstore "github.com/agent-dance/luban/internal/store/tasks"
	tooltasks "github.com/agent-dance/luban/internal/tools/tasks"
)

func TestRegistryDepsRoutesHookRunnerToTaskUpdate(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("LUBAN_CODE_TASK_LIST_ID", "registry-task-update-hook")
	root := t.TempDir()
	scope := runtimescope.NewRuntimeScope(root, true)
	store := taskstore.New(scope.TaskListID)
	created, err := tooltasks.NewTaskCreateTool(store, scope).Execute(context.Background(), map[string]any{
		"subject":     "finish",
		"description": "registry hook routing",
	})
	if err != nil || created.IsError {
		t.Fatalf("seed TaskCreate result=%#v err=%v", created, err)
	}
	taskID := created.Data.(tooltasks.TaskCreateResult).Task.ID

	update := tooltasks.NewTaskUpdateTool(store, scope, agentruntime.VerificationAgentEnabled)
	deps := &RegistryDeps{TaskUpdateTool: update}
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookTaskCompleted,
		Command: `printf '%s' '{"block":true,"system_reminder":"registry policy"}'`,
	}})
	deps.SetHookRunner(runner)

	result, err := update.Execute(context.Background(), map[string]any{"taskId": taskID, "status": "completed"})
	if err != nil {
		t.Fatalf("TaskUpdate: %v", err)
	}
	if !strings.Contains(result.Content, "registry policy") {
		t.Fatalf("TaskUpdate bypassed registry hook runner: %#v", result)
	}
}
