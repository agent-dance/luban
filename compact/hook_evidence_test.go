package compact

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/types"
)

func TestCompactLifecycleHooksEmitCorrelatedExecutionEvidence(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookPreCompact, Command: `printf '{"user_display_message":"pre"}'`},
		{Type: hooks.HookSessionStart, Command: `printf '{"system_reminder":"resume"}'`},
		{Type: hooks.HookPostCompact, Command: `printf '{"user_display_message":"post"}'`},
	})
	compactor := &SummaryCompactor{
		KeepRecent: 1,
		HookRunner: runner,
		SummarizeMessages: func(context.Context, []types.Message, string) (string, error) {
			return "exact compact summary", nil
		},
	}
	var hookTypes []hooks.HookType
	var observed []hooks.HookExecution
	ctx := hooks.WithExecutionObserver(context.Background(), func(hookType hooks.HookType, execution hooks.HookExecution) {
		hookTypes = append(hookTypes, hookType)
		observed = append(observed, execution)
	})
	ctx = hooks.WithCorrelation(ctx, hooks.HookInput{
		SessionID:  "session-compact",
		TurnID:     "turn-compact",
		WorkUnitID: "work-compact",
		AgentID:    "actor-compact",
		AgentType:  "assistant",
	})

	result, err := compactor.CompactWithTrigger(ctx, []types.Message{
		types.UserMessage("old one"),
		types.AssistantMessage("old two"),
		types.UserMessage("tail"),
	}, 1, "manual")
	if err != nil {
		t.Fatalf("CompactWithTrigger: %v", err)
	}
	if result.UserDisplayMessage != "pre\npost" {
		t.Fatalf("display message = %q", result.UserDisplayMessage)
	}
	if len(observed) != 3 {
		t.Fatalf("compact hook executions=%d, want PreCompact/SessionStart/PostCompact", len(observed))
	}
	wantTypes := []hooks.HookType{hooks.HookPreCompact, hooks.HookSessionStart, hooks.HookPostCompact}
	seenIDs := map[string]bool{}
	for index, execution := range observed {
		if hookTypes[index] != wantTypes[index] {
			t.Fatalf("hook order=%#v", hookTypes)
		}
		input := execution.Input
		if input.SessionID != "session-compact" || input.TurnID != "turn-compact" || input.WorkUnitID != "work-compact" ||
			input.AgentID != "actor-compact" || input.AgentType != "assistant" {
			t.Fatalf("compact hook %s lost correlation: %#v", hookTypes[index], input)
		}
		if execution.ConfigID == "" || execution.ExecutionID == "" || input.HookExecutionID != execution.ExecutionID || input.HookConfigID != execution.ConfigID {
			t.Fatalf("compact hook %s lost config/execution identity: %#v", hookTypes[index], execution)
		}
		if seenIDs[execution.ExecutionID] {
			t.Fatalf("compact hook execution ID reused: %q", execution.ExecutionID)
		}
		seenIDs[execution.ExecutionID] = true
		if execution.Output.Stdout == "" || execution.Output.StdoutBytes == 0 {
			t.Fatalf("compact hook %s lost raw evidence: %#v", hookTypes[index], execution.Output)
		}
	}
	if observed[0].Input.Trigger != "manual" || observed[2].Input.Trigger != "manual" || observed[2].Input.CompactSummary != "exact compact summary" {
		t.Fatalf("compact-specific evidence incomplete: pre=%#v post=%#v", observed[0].Input, observed[2].Input)
	}
}
