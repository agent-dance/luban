package loop

import (
	"context"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type goalBlockingTool struct {
	runtime *goalContinuationRuntime
}

func (*goalBlockingTool) Name() string        { return "BlockGoal" }
func (*goalBlockingTool) Description() string { return "mark the current goal blocked" }
func (*goalBlockingTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object", AdditionalProperties: false}
}
func (t *goalBlockingTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	current, err := t.runtime.LoadGoal()
	if err != nil {
		return types.ToolResult{}, err
	}
	next, err := goal.Block(*current, "blocked by model tool", time.Now())
	if err != nil {
		return types.ToolResult{}, err
	}
	if err := t.runtime.SaveGoal(next); err != nil {
		return types.ToolResult{}, err
	}
	return types.ToolResult{Content: "goal blocked"}, nil
}

func TestRunEmitsFinalGoalStatusAfterAutomaticAchievement(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: goalContinuationResult(true, "all checks passed", nil)}}}
	query := New(newParityFakeProvider([]parityProviderTurn{{Events: endTurnTextEvents("done", 10)}}), registry.New(), Config{
		MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	events := runGoalStatusQuery(t, query)
	assertFinalGoalStatusEvent(t, events, goal.StatusAchieved, current.Objective, "met")
}

func TestRunEmitsFinalGoalStatusAfterToolBlocksGoal(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	reg := registry.New()
	reg.Register(&goalBlockingTool{runtime: runtime})
	query := New(newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("goal-tool-1", "BlockGoal", `{}`, &types.Usage{OutputTokens: 10})},
		{Events: endTurnTextEvents("blocked", 5)},
	}), reg, Config{MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime})

	events := runGoalStatusQuery(t, query)
	assertFinalGoalStatusEvent(t, events, goal.StatusBlocked, current.Objective, "pending")
}

func TestRunEmitsFinalGoalStatusWhileGoalRemainsActive(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: goalContinuationResult(false, "more work remains", nil)}}}
	query := New(newParityFakeProvider([]parityProviderTurn{{Events: endTurnTextEvents("partial", 10)}}), registry.New(), Config{
		MaxTurns: 1, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	events := runGoalStatusQueryAllowError(query)
	assertFinalGoalStatusEvent(t, events, goal.StatusActive, current.Objective, "unmet")
}

func runGoalStatusQuery(t *testing.T, query *QueryLoop) []stream.Event {
	t.Helper()
	var events []stream.Event
	if err := query.Run(context.Background(), "finish", func(event stream.Event) { events = append(events, event) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return events
}

func runGoalStatusQueryAllowError(query *QueryLoop) []stream.Event {
	var events []stream.Event
	_ = query.Run(context.Background(), "finish", func(event stream.Event) { events = append(events, event) })
	return events
}

func assertFinalGoalStatusEvent(t *testing.T, events []stream.Event, wantStatus goal.Status, wantObjective, wantCriterionStatus string) {
	t.Helper()
	var statuses []stream.Event
	for _, event := range events {
		if event.Type == stream.EventGoalStatus {
			statuses = append(statuses, event)
		}
	}
	if len(statuses) != 1 {
		t.Fatalf("goal status events = %d, want 1: %+v", len(statuses), events)
	}
	got := statuses[0].GoalStatus
	if got == nil || got.Status != string(wantStatus) || got.Objective != wantObjective || got.Revision != 1 || len(got.Criteria) != 1 {
		t.Fatalf("goal status event = %+v, want status=%q objective=%q", got, wantStatus, wantObjective)
	}
	if got.Criteria[0].ID != "AC-1" || got.Criteria[0].Text == "" || got.Criteria[0].Status != wantCriterionStatus {
		t.Fatalf("goal criterion projection = %+v, want status=%q", got.Criteria[0], wantCriterionStatus)
	}
}
