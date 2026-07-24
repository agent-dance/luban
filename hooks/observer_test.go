package hooks

import (
	"context"
	"testing"
)

func TestRunDetailedObservedEmitsEachConfigWithCorrelation(t *testing.T) {
	runner := NewRunner([]Hook{
		{Type: HookTaskCreated, Command: `printf 'first'`},
		{Type: HookTaskCreated, Command: `printf 'second'`},
		{Type: HookTaskCompleted, Command: `printf 'ignored'`},
	})

	var observed []HookExecution
	ctx := WithExecutionObserver(context.Background(), func(_ HookType, execution HookExecution) {
		observed = append(observed, execution)
	})
	ctx = WithCorrelation(ctx, HookInput{
		SessionID:  "session-observer",
		TurnID:     "turn-observer",
		WorkUnitID: "work-observer",
		AgentID:    "actor-observer",
		AgentType:  "reviewer",
		ToolName:   "TaskCreate",
		ToolUseID:  "tool-observer",
	})

	executions := runner.RunDetailedObserved(ctx, HookTaskCreated, HookInput{TaskID: "task-7"})
	if len(executions) != 2 || len(observed) != 2 {
		t.Fatalf("executions=%d observed=%d, want two matching configs", len(executions), len(observed))
	}
	for index, execution := range observed {
		wantConfig := "config-" + string(rune('1'+index))
		if execution.ConfigID != wantConfig || execution.Input.HookConfigID != wantConfig {
			t.Fatalf("execution %d config identity = %#v", index, execution)
		}
		input := execution.Input
		if input.SessionID != "session-observer" || input.TurnID != "turn-observer" || input.WorkUnitID != "work-observer" ||
			input.AgentID != "actor-observer" || input.AgentType != "reviewer" || input.ToolName != "TaskCreate" ||
			input.ToolUseID != "tool-observer" || input.TaskID != "task-7" {
			t.Fatalf("execution %d lost correlation: %#v", index, input)
		}
		if execution.ExecutionID == "" || execution.Input.HookExecutionID != execution.ExecutionID {
			t.Fatalf("execution %d lost execution identity: %#v", index, execution)
		}
	}
	if observed[0].ExecutionID == observed[1].ExecutionID {
		t.Fatalf("actual configs reused execution ID %q", observed[0].ExecutionID)
	}

	// Observer records must be detached from returned control-flow values.
	executions[0].Input.TaskID = "mutated"
	if observed[0].Input.TaskID != "task-7" {
		t.Fatalf("observer retained mutable execution storage: %#v", observed[0])
	}
}

func TestRunBlockingDetailedPreservesBlockingExecutionEvidence(t *testing.T) {
	runner := NewRunner([]Hook{{
		Type:    HookTaskCompleted,
		Command: `printf 'policy detail' >&2; exit 2`,
	}})
	ctx := WithCorrelation(context.Background(), HookInput{SessionID: "session-block", TaskID: "task-block"})

	executions, err := runner.RunBlockingDetailed(ctx, HookTaskCompleted, HookInput{})
	if err == nil {
		t.Fatal("blocking detailed hook returned nil error")
	}
	if len(executions) != 1 || executions[0].Output.Stderr != "policy detail" || executions[0].Output.ExitCode != 2 {
		t.Fatalf("blocking execution evidence = %#v", executions)
	}
}

func TestRepeatedActualHookExecutionsReceiveUniqueExecutionIDs(t *testing.T) {
	runner := NewRunner([]Hook{{Type: HookSubagentStop, Command: `printf 'continue'`}})
	input := HookInput{
		SessionID:  "session-repeat",
		TurnID:     "turn-repeat",
		WorkUnitID: "work-repeat",
		AgentID:    "agent-repeat",
		AgentType:  "reviewer",
		ToolUseID:  "tool-repeat",
	}

	first := runner.RunDetailedObserved(context.Background(), HookSubagentStop, input)
	second := runner.RunDetailedObserved(context.Background(), HookSubagentStop, input)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("executions = %d then %d, want one each", len(first), len(second))
	}
	if first[0].ExecutionID == second[0].ExecutionID {
		t.Fatalf("repeated actual executions reused ID %q", first[0].ExecutionID)
	}
	if first[0].ConfigID != second[0].ConfigID {
		t.Fatalf("stable config identity changed across runs: first=%q second=%q", first[0].ConfigID, second[0].ConfigID)
	}
	if first[0].Input.HookExecutionID != first[0].ExecutionID || second[0].Input.HookExecutionID != second[0].ExecutionID {
		t.Fatalf("stdin/evidence execution IDs diverged: first=%#v second=%#v", first[0], second[0])
	}
}
