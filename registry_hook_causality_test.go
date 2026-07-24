package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/tools"
)

func TestRegistryDepsRoutesHookRunnerToTaskUpdate(t *testing.T) {
	t.Setenv("CLAUDE_HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "registry-task-update-hook")
	root := t.TempDir()
	store := tools.NewTaskStore()
	store.SetScopeResolver(tools.NewRuntimeScope(root, true))
	created, err := tools.NewTaskCreateTool(store).Execute(context.Background(), map[string]any{
		"subject":     "finish",
		"description": "registry hook routing",
	})
	if err != nil || created.IsError {
		t.Fatalf("seed TaskCreate result=%#v err=%v", created, err)
	}
	taskID := created.Data.(tools.TaskCreateResult).Task.ID

	update := tools.NewTaskUpdateTool(store)
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
