package tools

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/types"
)

func causalToolContext(ctx context.Context, toolName, toolUseID string) context.Context {
	return loop.WithToolExecutionContext(ctx, loop.ToolExecutionContext{
		SessionID:  "session-causal",
		TurnID:     "turn-causal",
		WorkUnitID: "work-causal",
		ActorID:    "actor-parent",
		ActorType:  "assistant",
		ToolUse: types.ToolUseBlock{
			ID:   toolUseID,
			Name: toolName,
		},
	})
}

func TestTaskCreateHookEvidenceCarriesToolCausality(t *testing.T) {
	t.Setenv("CLAUDE_HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "task-create-causal")
	store := NewTaskStore()
	store.SetScopeResolver(NewRuntimeScope(t.TempDir(), true))
	tool := NewTaskCreateTool(store)
	tool.SetHookRunner(hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookTaskCreated,
		Command: `printf 'created evidence'`,
	}}))

	var observed []hooks.HookExecution
	ctx := hooks.WithExecutionObserver(context.Background(), func(_ hooks.HookType, execution hooks.HookExecution) {
		observed = append(observed, execution)
	})
	ctx = causalToolContext(ctx, "TaskCreate", "tool-task-create")
	result, err := tool.Execute(ctx, map[string]any{"subject": "ship", "description": "causal task"})
	if err != nil || result.IsError {
		t.Fatalf("TaskCreate result=%#v err=%v", result, err)
	}
	if len(observed) != 1 {
		t.Fatalf("TaskCreated observed executions=%d, want one", len(observed))
	}
	input := observed[0].Input
	if input.SessionID != "session-causal" || input.TurnID != "turn-causal" || input.WorkUnitID != "work-causal" ||
		input.AgentID != "actor-parent" || input.AgentType != "assistant" || input.ToolName != "TaskCreate" || input.ToolUseID != "tool-task-create" || input.TaskID == "" {
		t.Fatalf("TaskCreated evidence lost causality: %#v", input)
	}
	if observed[0].Output.Stdout != "created evidence" {
		t.Fatalf("TaskCreated raw evidence = %#v", observed[0].Output)
	}
}

func TestTaskCompletedHookEvidenceCarriesToolCausality(t *testing.T) {
	t.Setenv("CLAUDE_HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "task-complete-causal")
	store := NewTaskStore()
	store.SetScopeResolver(NewRuntimeScope(t.TempDir(), true))
	created, err := NewTaskCreateTool(store).Execute(context.Background(), map[string]any{"subject": "verify", "description": "causal completion"})
	if err != nil || created.IsError {
		t.Fatalf("seed TaskCreate result=%#v err=%v", created, err)
	}
	createdData := created.Data.(TaskCreateResult)

	tool := NewTaskUpdateTool(store)
	tool.SetHookRunner(hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookTaskCompleted,
		Command: `printf 'completed evidence'`,
	}}))
	var observed []hooks.HookExecution
	ctx := hooks.WithExecutionObserver(context.Background(), func(_ hooks.HookType, execution hooks.HookExecution) {
		observed = append(observed, execution)
	})
	ctx = causalToolContext(ctx, "TaskUpdate", "tool-task-update")
	result, err := tool.Execute(ctx, map[string]any{"taskId": createdData.Task.ID, "status": "completed"})
	if err != nil || result.IsError {
		t.Fatalf("TaskUpdate result=%#v err=%v", result, err)
	}
	if len(observed) != 1 {
		t.Fatalf("TaskCompleted observed executions=%d, want one", len(observed))
	}
	input := observed[0].Input
	if input.SessionID != "session-causal" || input.TurnID != "turn-causal" || input.WorkUnitID != "work-causal" ||
		input.ToolName != "TaskUpdate" || input.ToolUseID != "tool-task-update" || input.TaskID != createdData.Task.ID {
		t.Fatalf("TaskCompleted evidence lost causality: %#v", input)
	}
}

func TestSubagentLifecycleHookEvidenceCarriesParentToolAndChildActor(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookSubagentStart, Matcher: "reviewer", Command: `printf 'start evidence'`},
		{Type: hooks.HookSubagentStop, Matcher: "reviewer", Command: `printf 'stop policy' >&2; exit 2`},
	})
	var hookTypes []hooks.HookType
	var observed []hooks.HookExecution
	ctx := hooks.WithExecutionObserver(context.Background(), func(hookType hooks.HookType, execution hooks.HookExecution) {
		hookTypes = append(hookTypes, hookType)
		observed = append(observed, execution)
	})
	ctx = causalToolContext(ctx, "Agent", "tool-agent")

	if got := subagentStartHookContext(ctx, runner, "agent-child", "reviewer"); got != "start evidence" {
		t.Fatalf("SubagentStart context = %q, want existing reminder semantics", got)
	}
	if got := subagentStopHookContinuation(ctx, runner, "agent-child", "reviewer", "/tmp/transcript", "finished"); got != "stop policy" {
		t.Fatalf("SubagentStop continuation = %q, want policy", got)
	}
	if len(observed) != 2 || hookTypes[0] != hooks.HookSubagentStart || hookTypes[1] != hooks.HookSubagentStop {
		t.Fatalf("observed lifecycle executions=%#v types=%#v", observed, hookTypes)
	}
	for _, execution := range observed {
		input := execution.Input
		if input.SessionID != "session-causal" || input.TurnID != "turn-causal" || input.WorkUnitID != "work-causal" ||
			input.ToolName != "Agent" || input.ToolUseID != "tool-agent" || input.AgentID != "agent-child" || input.AgentType != "reviewer" {
			t.Fatalf("subagent evidence lost causality: %#v", input)
		}
	}
	if observed[1].Input.AgentTranscriptPath != "/tmp/transcript" || observed[1].Input.LastAssistantMessage != "finished" || observed[1].Output.Stderr != "stop policy" {
		t.Fatalf("SubagentStop evidence incomplete: %#v", observed[1])
	}
}
