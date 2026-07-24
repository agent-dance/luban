package loop

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/hooks"
)

func TestHookExecutionEventEmitterPreservesIdentityAndRawEvidence(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookTaskCreated, Command: `printf 'passed raw'`},
		{Type: hooks.HookTaskCreated, Command: `printf '%s' '{"block":true,"system_reminder":"policy blocked"}'`},
	})
	var events []Event
	ctx := withHookExecutionEventEmitter(context.Background(), func(event Event) {
		events = append(events, event)
	})
	ctx = hooks.WithCorrelation(ctx, hooks.HookInput{
		SessionID:   "session-emitter",
		ProjectRoot: "/workspace/emitter",
		TurnID:      "turn-emitter",
		WorkUnitID:  "work-emitter",
		AgentID:     "actor-emitter",
		AgentType:   "reviewer",
		ToolName:    "TaskCreate",
		ToolUseID:   "tool-emitter",
		TaskID:      "task-emitter",
	})

	runner.RunDetailedObserved(ctx, hooks.HookTaskCreated, hooks.HookInput{})
	if len(events) != 2 {
		t.Fatalf("hook summary events=%d, want one per actual config", len(events))
	}
	for index, event := range events {
		if event.Type != EventHookSummary || event.ProjectRoot != "/workspace/emitter" || event.TurnID != "turn-emitter" || event.WorkUnitID != "work-emitter" ||
			event.ActorID != "actor-emitter" || event.ActorType != "reviewer" || event.HookSummary == nil ||
			event.ToolUseID != "tool-emitter" || event.HookSummary.ToolUseID != "tool-emitter" || event.HookSummary.HookExecutionID == "" {
			t.Fatalf("event %d lost stable identity: %#v", index, event)
		}
		if event.HookSummary.Metadata["task_id"] != "task-emitter" {
			t.Fatalf("event %d lost task identity: %#v", index, event.HookSummary.Metadata)
		}
	}
	if events[0].HookSummary.Status != "passed" || events[1].HookSummary.Status != "blocked" || events[1].HookSummary.Summary != "policy blocked" {
		t.Fatalf("hook statuses = %#v", events)
	}
	firstOutput, ok := events[0].HookSummary.Metadata["hook_output"].(hooks.HookOutput)
	if !ok || firstOutput.Stdout != "passed raw" || firstOutput.StdoutBytes != int64(len("passed raw")) {
		t.Fatalf("raw hook evidence missing: %#v", events[0].HookSummary.Metadata["hook_output"])
	}
	if events[0].HookSummary.HookExecutionID == events[1].HookSummary.HookExecutionID {
		t.Fatalf("config executions reused ID %q", events[0].HookSummary.HookExecutionID)
	}
}
