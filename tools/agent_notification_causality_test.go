package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestAgentToolBackgroundNotificationPreservesParentHookCausality(t *testing.T) {
	projectRoot := t.TempDir()
	manager := NewBackgroundTaskManager(projectRoot)
	t.Cleanup(manager.Shutdown)
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookNotification, Command: `printf 'notification evidence'`, Timeout: 1,
	}})
	tool := &AgentTool{
		Provider:   &captureAgentProvider{responses: []string{"background complete"}},
		Registry:   registry.New(),
		Background: manager,
		HookRunner: runner,
	}

	observed := make(chan hooks.HookExecution, 1)
	ctx := hooks.WithExecutionObserver(context.Background(), func(hookType hooks.HookType, execution hooks.HookExecution) {
		if hookType == hooks.HookNotification {
			observed <- execution
		}
	})
	ctx = loop.WithToolExecutionContext(ctx, loop.ToolExecutionContext{
		SessionID:   "parent-session",
		ProjectRoot: projectRoot,
		CWD:         filepath.Join(projectRoot, "nested", "cwd"),
		TurnID:      "parent-turn",
		WorkUnitID:  "parent-work",
		ActorID:     "parent-actor",
		ActorType:   "assistant",
		ToolUse:     types.ToolUseBlock{ID: "parent-agent-tool-use", Name: "Agent"},
	})
	result, err := tool.Execute(ctx, agentExecuteInput("finish in background", map[string]any{
		"run_in_background": true,
	}))
	if err != nil || result.IsError {
		t.Fatalf("Agent.Execute result=%#v err=%v", result, err)
	}
	var launch agentAsyncToolResult
	if err := json.Unmarshal([]byte(result.Content), &launch); err != nil {
		t.Fatalf("decode async launch: %v\n%s", err, result.Content)
	}
	if launch.AgentID == "" {
		t.Fatalf("async launch omitted agent ID: %#v", launch)
	}
	if _, status := manager.Wait(launch.AgentID, 2*time.Second); status != "success" {
		t.Fatalf("background agent wait status = %q", status)
	}

	select {
	case execution := <-observed:
		input := execution.Input
		if input.SessionID != "parent-session" || filepath.Clean(input.ProjectRoot) != filepath.Clean(projectRoot) ||
			input.TurnID != "parent-turn" || input.WorkUnitID != "parent-work" ||
			input.AgentID != "parent-actor" || input.AgentType != "assistant" ||
			input.ToolName != "Agent" || input.ToolUseID != "parent-agent-tool-use" || input.TaskID != launch.AgentID {
			t.Fatalf("background Notification lost parent causality: %#v", input)
		}
		if execution.ExecutionID == "" || execution.ConfigID == "" || execution.Output.Stdout != "notification evidence" {
			t.Fatalf("background Notification evidence incomplete: %#v", execution)
		}
	case <-time.After(time.Second):
		t.Fatal("background Notification execution never reached the parent observer")
	}
}
