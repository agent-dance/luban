package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type goalContinuationRuntime struct {
	mu        sync.Mutex
	current   *goal.Goal
	loadErr   error
	loadErrAt int
	saveErr   error
	loads     int
	saves     int
	history   []goal.Goal
}

func (r *goalContinuationRuntime) LoadGoal() (*goal.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loads++
	if r.loadErr != nil && (r.loadErrAt == 0 || r.loads == r.loadErrAt) {
		return nil, r.loadErr
	}
	if r.current == nil {
		return nil, nil
	}
	current := *r.current
	return &current, nil
}

func (r *goalContinuationRuntime) SaveGoal(next goal.Goal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saves++
	if r.saveErr != nil {
		return r.saveErr
	}
	current := next
	r.current = &current
	r.history = append(r.history, next)
	return nil
}

func (r *goalContinuationRuntime) UpdateGoal(update goal.UpdateFunc) (goal.Goal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loads++
	if r.loadErr != nil && (r.loadErrAt == 0 || r.loads == r.loadErrAt) {
		return goal.Goal{}, r.loadErr
	}
	var current *goal.Goal
	if r.current != nil {
		copy := *r.current
		current = &copy
	}
	next, err := update(current)
	if err != nil {
		return goal.Goal{}, err
	}
	r.saves++
	if r.saveErr != nil {
		return goal.Goal{}, r.saveErr
	}
	saved := next
	r.current = &saved
	r.history = append(r.history, next)
	return next, nil
}

func (r *goalContinuationRuntime) snapshot(t *testing.T) goal.Goal {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		t.Fatal("goal runtime has no current goal")
	}
	return *r.current
}

func (r *goalContinuationRuntime) savedHistory() []goal.Goal {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]goal.Goal(nil), r.history...)
}

type goalContinuationEvaluatorStep struct {
	result GoalEvaluationResult
	err    error
}

type goalContinuationEvaluator struct {
	mu       sync.Mutex
	steps    []goalContinuationEvaluatorStep
	requests []GoalEvaluationRequest
}

func (e *goalContinuationEvaluator) Evaluate(_ context.Context, request GoalEvaluationRequest) (GoalEvaluationResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.requests = append(e.requests, newGoalEvaluationRequest(goal.Goal{
		Objective: request.Objective, Revision: request.Revision, Status: goal.StatusActive,
		AcceptanceCriteria: append([]goal.AcceptanceCriterion(nil), request.AcceptanceCriteria...),
	}, request.Messages))
	index := len(e.requests) - 1
	if index >= len(e.steps) {
		return GoalEvaluationResult{}, fmt.Errorf("unexpected goal evaluation call %d", index+1)
	}
	return e.steps[index].result, e.steps[index].err
}

func (e *goalContinuationEvaluator) GoalEvaluatorForModel(string) GoalEvaluator { return e }

func (e *goalContinuationEvaluator) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.requests)
}

func activeGoalForContinuation(t *testing.T, tokenBudget int) goal.Goal {
	t.Helper()
	current, err := goal.CreateWithCriteria(
		"finish every acceptance check",
		[]string{"finish every acceptance check"},
		tokenBudget,
		time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return current
}

func goalContinuationResult(met bool, reason string, usage *types.Usage) GoalEvaluationResult {
	return GoalEvaluationResult{
		Criteria: []GoalCriterionEvaluationResult{{ID: "AC-1", Met: met, Reason: reason}},
		Reason:   reason, Usage: usage,
	}
}

func TestGoalContinuationUnmetAppendsReasonAndPersistsProgress(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{
		{result: goalContinuationResult(false, "integration checks still fail", &types.Usage{InputTokens: 900, OutputTokens: 700})},
		{result: goalContinuationResult(true, "all acceptance checks pass", &types.Usage{InputTokens: 800, OutputTokens: 600})},
	}}
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("draft", 40)},
		{Events: endTurnTextEvents("verified", 60)},
	})
	query := New(provider, registry.New(), Config{
		MaxTurns: 5, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	if err := query.Run(context.Background(), "finish the work", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(provider.Calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.Calls))
	}
	last := provider.Calls[1].Messages[len(provider.Calls[1].Messages)-1]
	if last.Role != types.RoleUser || !strings.Contains(last.GetText(), "integration checks still fail") {
		t.Fatalf("continuation message = %#v, want evaluator reason", last)
	}

	history := runtime.savedHistory()
	if len(history) != 4 {
		t.Fatalf("persisted goal versions = %d, want 4", len(history))
	}
	if accounted := history[0]; accounted.TurnCount != 1 || accounted.Usage != 40 || accounted.LastEvaluatorReason != "" {
		t.Fatalf("first turn accounting = %+v", accounted)
	}
	first := history[1]
	if first.Status != goal.StatusActive || first.TurnCount != 1 || first.Usage != 40 || first.LastEvaluatorReason != "integration checks still fail" {
		t.Fatalf("first persisted goal = %+v", first)
	}
	if accounted := history[2]; accounted.TurnCount != 2 || accounted.Usage != 100 || accounted.LastEvaluatorReason != "integration checks still fail" {
		t.Fatalf("second turn accounting = %+v", accounted)
	}
	final := history[3]
	if final.Status != goal.StatusAchieved || final.TurnCount != 2 || final.Usage != 100 || final.LastEvaluatorReason != "all acceptance checks pass" {
		t.Fatalf("final persisted goal = %+v", final)
	}
	if final.AchievedAt == nil {
		t.Fatal("met goal was not timestamped as achieved")
	}
}

func TestGoalContinuationMetMarksAchievedWithoutAnotherProviderTurn(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{
		result: goalContinuationResult(true, "the transcript proves completion", &types.Usage{OutputTokens: 999}),
	}}}
	provider := newParityFakeProvider([]parityProviderTurn{{Events: endTurnTextEvents("done", 75)}})
	query := New(provider, registry.New(), Config{
		MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	var events []stream.Event
	if err := query.Run(context.Background(), "finish", func(event stream.Event) { events = append(events, event) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(provider.Calls) != 1 || evaluator.callCount() != 1 {
		t.Fatalf("provider/evaluator calls = %d/%d, want 1/1", len(provider.Calls), evaluator.callCount())
	}
	final := runtime.snapshot(t)
	if final.Status != goal.StatusAchieved || final.TurnCount != 1 || final.Usage != 75 || final.LastEvaluatorReason != "the transcript proves completion" {
		t.Fatalf("achieved goal = %+v", final)
	}
	assertGoalEvaluationUsageEvent(t, events, types.Usage{OutputTokens: 999}, 75)
}

func TestGoalContinuationDerivesCompletionFromEveryStructuredCriterion(t *testing.T) {
	current, err := goal.CreateWithCriteria("ship release", []string{"tests pass", "docs updated"}, 0,
		time.Date(2026, time.July, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: GoalEvaluationResult{
		Criteria: []GoalCriterionEvaluationResult{
			{ID: "AC-1", Met: true, Reason: "focused tests passed"},
			{ID: "AC-2", Met: false, Reason: "documentation evidence is missing"},
		},
		Reason: "documentation remains",
	}}}}
	provider := newParityFakeProvider([]parityProviderTurn{{Events: endTurnTextEvents("partial", 20)}})
	query := New(provider, registry.New(), Config{
		MaxTurns: 1, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})
	_ = query.Run(context.Background(), "finish", func(stream.Event) {})

	final := runtime.snapshot(t)
	if final.Status != goal.StatusActive || final.LastAcceptanceEvaluation == nil || goal.AcceptanceCriteriaMet(final) {
		t.Fatalf("partially accepted goal = %+v", final)
	}
	if len(final.LastAcceptanceEvaluation.Criteria) != 2 || final.LastAcceptanceEvaluation.Criteria[1].Met {
		t.Fatalf("criterion evaluation = %+v", final.LastAcceptanceEvaluation)
	}
	if len(evaluator.requests) != 1 || len(evaluator.requests[0].AcceptanceCriteria) != 2 || evaluator.requests[0].Revision != 1 {
		t.Fatalf("evaluator request = %+v", evaluator.requests)
	}
}

func TestGoalContinuationPausedTerminalAndAbsentGoalsDoNotEvaluate(t *testing.T) {
	statuses := []struct {
		name    string
		current *goal.Goal
	}{
		{name: "absent"},
		{name: "paused", current: &goal.Goal{Objective: "pause", Status: goal.StatusPaused}},
		{name: "achieved", current: &goal.Goal{Objective: "done", Status: goal.StatusAchieved}},
		{name: "blocked", current: &goal.Goal{Objective: "blocked", Status: goal.StatusBlocked}},
		{name: "cleared", current: &goal.Goal{Objective: "cleared", Status: goal.StatusCleared}},
	}

	for _, tt := range statuses {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &goalContinuationRuntime{current: tt.current}
			evaluator := &goalContinuationEvaluator{}
			provider := newParityFakeProvider([]parityProviderTurn{{Events: endTurnTextEvents("ordinary stop", 10)}})
			query := New(provider, registry.New(), Config{
				MaxTurns: 2, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
			})

			if err := query.Run(context.Background(), "respond", func(stream.Event) {}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if evaluator.callCount() != 0 || runtime.saves != 0 || len(provider.Calls) != 1 {
				t.Fatalf("evaluator/saves/provider = %d/%d/%d, want 0/0/1", evaluator.callCount(), runtime.saves, len(provider.Calls))
			}
		})
	}
}

func TestGoalContinuationStopPreventTakesPriorityOverEvaluator(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: goalContinuationResult(false, "should not run", nil)}}}
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookStop, Command: testHookOutputCommand(`{"preventContinuation":true,"stopReason":"external stop"}`), Timeout: 5,
	}})
	provider := newParityFakeProvider([]parityProviderTurn{{Events: endTurnTextEvents("stop now", 10)}})
	query := New(provider, registry.New(), Config{
		MaxTurns: 3, MaxTokens: 1024, HookRunner: runner,
		GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	if err := query.Run(context.Background(), "respond", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if evaluator.callCount() != 0 || runtime.saves != 1 || len(provider.Calls) != 1 {
		t.Fatalf("evaluator/saves/provider = %d/%d/%d, want 0/1/1", evaluator.callCount(), runtime.saves, len(provider.Calls))
	}
	if final := runtime.snapshot(t); final.Usage != 10 || final.TurnCount != 1 {
		t.Fatalf("stop-prevent goal progress = %+v", final)
	}
}

func TestGoalContinuationStopBlockFeedbackRunsBeforeEvaluator(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookStop, Command: testBlockingHookCommand("revise before goal evaluation", true), Timeout: 5,
	}})
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: goalContinuationResult(true, "revised answer satisfies the goal", nil)}}}
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("draft", 10)},
		{Events: endTurnTextEvents("revised", 20)},
	})
	query := New(provider, registry.New(), Config{
		MaxTurns: 3, MaxTokens: 1024, HookRunner: runner,
		GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	if err := query.Run(context.Background(), "respond", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if evaluator.callCount() != 1 || len(provider.Calls) != 2 {
		t.Fatalf("evaluator/provider calls = %d/%d, want 1/2", evaluator.callCount(), len(provider.Calls))
	}
	if got := joinedMessageText(provider.Calls[1].Messages); !strings.Contains(got, "Stop hook feedback:\nrevise before goal evaluation") {
		t.Fatalf("second request omitted Stop feedback: %q", got)
	}
}

func TestGoalContinuationCannotBypassMaxTurns(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{
		{result: goalContinuationResult(false, "first gap", nil)},
		{result: goalContinuationResult(false, "second gap", nil)},
	}}
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("one", 10)},
		{Events: endTurnTextEvents("two", 15)},
		{Events: endTurnTextEvents("must not run", 20)},
	})
	query := New(provider, registry.New(), Config{
		MaxTurns: 2, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	err := query.Run(context.Background(), "finish", func(stream.Event) {})
	var maxTurnsErr *MaxTurnsError
	if !errors.As(err, &maxTurnsErr) {
		t.Fatalf("Run error = %v, want MaxTurnsError", err)
	}
	if len(provider.Calls) != 2 || evaluator.callCount() != 2 {
		t.Fatalf("provider/evaluator calls = %d/%d, want 2/2", len(provider.Calls), evaluator.callCount())
	}
	final := runtime.snapshot(t)
	if final.Status != goal.StatusActive || final.TurnCount != 2 || final.Usage != 25 || final.LastEvaluatorReason != "second gap" {
		t.Fatalf("max-turn goal progress = %+v", final)
	}
}

func TestGoalContinuationEvaluatorErrorFailsClosed(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{
		result: GoalEvaluationResult{Usage: &types.Usage{InputTokens: 321, OutputTokens: 17}},
		err:    errors.New("evaluator unavailable"),
	}}}
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("answer", 10)},
		{Events: endTurnTextEvents("must not continue", 10)},
	})
	query := New(provider, registry.New(), Config{
		MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})
	var events []stream.Event
	runErr := query.Run(context.Background(), "finish", func(event stream.Event) { events = append(events, event) })

	if len(provider.Calls) != 1 || evaluator.callCount() != 1 || runtime.saves != 2 {
		t.Fatalf("provider/evaluator/saves = %d/%d/%d, want 1/1/2", len(provider.Calls), evaluator.callCount(), runtime.saves)
	}
	final := runtime.snapshot(t)
	if final.Status != goal.StatusActive || final.TurnCount != 1 || final.Usage != 10 || !strings.Contains(final.LastEvaluatorReason, "evaluator unavailable") {
		t.Fatalf("evaluator-failed goal progress = %+v", final)
	}
	if runErr == nil && !hasGoalContinuationFailure(events, "evaluation") {
		t.Fatalf("evaluator failure was neither returned nor surfaced: events=%+v", events)
	}
	assertGoalEvaluationUsageEvent(t, events, types.Usage{InputTokens: 321, OutputTokens: 17}, 10)
}

func TestGoalContinuationPersistErrorFailsClosed(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current, saveErr: errors.New("session store unavailable")}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: goalContinuationResult(false, "work remains", nil)}}}
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("answer", 10)},
		{Events: endTurnTextEvents("must not continue", 10)},
	})
	query := New(provider, registry.New(), Config{
		MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})
	var events []stream.Event
	runErr := query.Run(context.Background(), "finish", func(event stream.Event) { events = append(events, event) })

	if len(provider.Calls) != 1 || evaluator.callCount() != 0 || runtime.saves != 1 {
		t.Fatalf("provider/evaluator/save attempts = %d/%d/%d, want 1/0/1", len(provider.Calls), evaluator.callCount(), runtime.saves)
	}
	if runErr == nil && !hasGoalContinuationFailure(events, "goal") {
		t.Fatalf("persistence failure was neither returned nor surfaced: events=%+v", events)
	}
}

type goalCompletionTool struct {
	runtime *goalContinuationRuntime
}

func (*goalCompletionTool) Name() string        { return "CompleteGoal" }
func (*goalCompletionTool) Description() string { return "mark the current goal complete" }
func (*goalCompletionTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object", AdditionalProperties: false}
}
func (t *goalCompletionTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	current, err := t.runtime.LoadGoal()
	if err != nil {
		return types.ToolResult{}, err
	}
	if current == nil {
		return types.ToolResult{Content: "no goal", IsError: true}, nil
	}
	now := time.Now()
	criteria := current.Criteria()
	results := make([]goal.AcceptanceCriterionEvaluation, 0, len(criteria))
	for _, criterion := range criteria {
		results = append(results, goal.AcceptanceCriterionEvaluation{
			CriterionID: criterion.ID, Met: true, Reason: "completed by model tool",
		})
	}
	next, err := goal.RecordAcceptanceEvaluation(*current, goal.Normalize(*current).Revision, results, "completed by model tool", now)
	if err != nil {
		return types.ToolResult{}, err
	}
	next, err = goal.Achieve(next, "completed by model tool", now)
	if err != nil {
		return types.ToolResult{}, err
	}
	if err := t.runtime.SaveGoal(next); err != nil {
		return types.ToolResult{}, err
	}
	return types.ToolResult{Content: "goal completed"}, nil
}

type goalPlanRestartTool struct{}

func (goalPlanRestartTool) Name() string        { return "ExitPlanMode" }
func (goalPlanRestartTool) Description() string { return "restart from an approved plan" }
func (goalPlanRestartTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object", AdditionalProperties: false}
}
func (goalPlanRestartTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{
		Content: "User approved the plan.",
		Metadata: map[string]string{
			"clearContext":     "true",
			"restartExecution": "true",
		},
	}, nil
}

func TestGoalContinuationToolCompletionInSameRunSkipsEvaluator(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: goalContinuationResult(false, "stale evaluator should not run", nil)}}}
	reg := registry.New()
	reg.Register(&goalCompletionTool{runtime: runtime})
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("goal-tool-1", "CompleteGoal", `{}`, &types.Usage{OutputTokens: 20})},
		{Events: endTurnTextEvents("completion recorded", 15)},
	})
	query := New(provider, reg, Config{
		MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	if err := query.Run(context.Background(), "finish", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(provider.Calls) != 2 || evaluator.callCount() != 0 {
		t.Fatalf("provider/evaluator calls = %d/%d, want 2/0", len(provider.Calls), evaluator.callCount())
	}
	if final := runtime.snapshot(t); final.Status != goal.StatusAchieved || final.LastEvaluatorReason != "completed by model tool" {
		t.Fatalf("tool-updated goal = %+v", final)
	}
}

func TestGoalContinuationCountsToolUseAndTerminalAssistantUsage(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: goalContinuationResult(true, "tool result and final answer prove completion", nil)}}}
	reg := registry.New()
	reg.Register(parityTool{name: "Echo", content: "tool finished"})
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("goal-tool-usage-1", "Echo", `{}`, &types.Usage{OutputTokens: 25})},
		{Events: endTurnTextEvents("finished", 35)},
	})
	query := New(provider, reg, Config{
		MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	if err := query.Run(context.Background(), "finish", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	final := runtime.snapshot(t)
	if final.Status != goal.StatusAchieved || final.Usage != 60 || final.TurnCount != 2 {
		t.Fatalf("goal progress = %+v, want achieved with usage 60 across 2 assistant turns", final)
	}
}

func TestGoalContinuationToolUseTurnsStopAtGoalTokenBudget(t *testing.T) {
	current := activeGoalForContinuation(t, 30)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{}
	reg := registry.New()
	reg.Register(parityTool{name: "Echo", content: "tool finished"})
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("goal-tool-budget-1", "Echo", `{}`, &types.Usage{OutputTokens: 15})},
		{Events: parityToolUseEventsWithUsage("goal-tool-budget-2", "Echo", `{}`, &types.Usage{OutputTokens: 15})},
		{Events: endTurnTextEvents("must not run", 10)},
	})
	query := New(provider, reg, Config{
		MaxTurns: 4, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})
	var events []stream.Event
	if err := query.Run(context.Background(), "finish", func(event stream.Event) { events = append(events, event) }); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(provider.Calls) != 2 || evaluator.callCount() != 0 {
		t.Fatalf("provider/evaluator calls = %d/%d, want 2/0", len(provider.Calls), evaluator.callCount())
	}
	final := runtime.snapshot(t)
	if final.Status != goal.StatusActive || final.Usage != 30 || final.TurnCount != 2 {
		t.Fatalf("budget-exhausted tool progress = %+v, want active usage 30 across 2 turns", final)
	}
	if !hasGoalContinuationFailure(events, "budget") {
		t.Fatalf("goal budget exhaustion was not visible: events=%+v", events)
	}
	if !transcriptHasToolResult(query.Messages(), "goal-tool-budget-2") {
		t.Fatalf("budget stop left an orphaned tool_use without its tool_result: %+v", query.Messages())
	}
}

func TestGoalContinuationStopHookBlockingCannotBypassGoalTokenBudget(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookStop, Command: testBlockingHookCommand("blocking feedback", false), Timeout: 5,
	}})
	current := activeGoalForContinuation(t, 10)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{}
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("budget edge", 10)},
		{Events: endTurnTextEvents("must not run", 10)},
	})
	query := New(provider, registry.New(), Config{
		MaxTurns: 3, MaxTokens: 1024, HookRunner: runner,
		GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	var events []stream.Event
	if err := query.Run(context.Background(), "respond", func(event stream.Event) { events = append(events, event) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(provider.Calls) != 1 || evaluator.callCount() != 0 {
		t.Fatalf("provider/evaluator calls = %d/%d, want 1/0", len(provider.Calls), evaluator.callCount())
	}
	final := runtime.snapshot(t)
	if final.Usage != 10 || final.TurnCount != 1 || final.Status != goal.StatusActive {
		t.Fatalf("budget-exhausted stop-hook progress = %+v", final)
	}
	if !hasGoalContinuationFailure(events, "budget") {
		t.Fatalf("goal budget exhaustion was not visible: events=%+v", events)
	}
}

func TestGoalContinuationPlanRestartCannotBypassGoalTokenBudget(t *testing.T) {
	current := activeGoalForContinuation(t, 10)
	runtime := &goalContinuationRuntime{current: &current}
	reg := registry.New()
	reg.Register(goalPlanRestartTool{})
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("goal-plan-budget", "ExitPlanMode", `{}`, &types.Usage{OutputTokens: 10})},
		{Events: endTurnTextEvents("must not run", 10)},
	})
	query := New(provider, reg, Config{
		MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime,
	})

	var events []stream.Event
	if err := query.Run(context.Background(), "plan", func(event stream.Event) { events = append(events, event) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(provider.Calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.Calls))
	}
	if got := runtime.snapshot(t); got.Usage != 10 || got.TurnCount != 1 || got.Status != goal.StatusActive {
		t.Fatalf("budget-exhausted plan restart progress = %+v", got)
	}
	if !hasGoalContinuationFailure(events, "budget") {
		t.Fatalf("goal budget exhaustion was not visible: events=%+v", events)
	}
}

func TestGoalContinuationMaxTokensRecoveryCannotBypassGoalTokenBudget(t *testing.T) {
	for _, tc := range []struct {
		name  string
		model string
	}{
		{name: "escalation", model: "claude-sonnet-4-6"},
		{name: "recovery message", model: "test-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := activeGoalForContinuation(t, 10)
			runtime := &goalContinuationRuntime{current: &current}
			provider := newParityFakeProvider([]parityProviderTurn{
				{Events: maxTokensTextEvents("truncated", 10)},
				{Events: endTurnTextEvents("must not run", 10)},
			})
			query := New(provider, registry.New(), Config{
				Model: tc.model, MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime,
			})

			var events []stream.Event
			if err := query.Run(context.Background(), "respond", func(event stream.Event) { events = append(events, event) }); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(provider.Calls) != 1 {
				t.Fatalf("provider calls = %d, want 1", len(provider.Calls))
			}
			if got := runtime.snapshot(t); got.Usage != 10 || got.TurnCount != 1 || got.Status != goal.StatusActive {
				t.Fatalf("budget-exhausted max_tokens progress = %+v", got)
			}
			if !hasGoalContinuationFailure(events, "budget") {
				t.Fatalf("goal budget exhaustion was not visible: events=%+v", events)
			}
		})
	}
}

func TestGoalContinuationToolResultPairedWhenGoalUsagePersistenceFails(t *testing.T) {
	current := activeGoalForContinuation(t, 0)
	runtime := &goalContinuationRuntime{current: &current, saveErr: errors.New("session store unavailable")}
	reg := registry.New()
	reg.Register(parityTool{name: "Echo", content: "tool finished"})
	provider := newParityFakeProvider([]parityProviderTurn{{
		Events: parityToolUseEventsWithUsage("goal-save-failure-tool", "Echo", `{}`, &types.Usage{OutputTokens: 5}),
	}})
	query := New(provider, reg, Config{
		MaxTurns: 2, MaxTokens: 1024, GoalRuntime: runtime,
	})

	if err := query.Run(context.Background(), "respond", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !transcriptHasToolResult(query.Messages(), "goal-save-failure-tool") {
		t.Fatalf("goal persistence failure left an orphaned tool_use: %+v", query.Messages())
	}
}

func TestGoalContinuationToolResultPairedWhenPostToolBudgetLoadFails(t *testing.T) {
	current := activeGoalForContinuation(t, 100)
	runtime := &goalContinuationRuntime{
		current:   &current,
		loadErr:   errors.New("session store unavailable"),
		loadErrAt: 4,
	}
	reg := registry.New()
	reg.Register(parityTool{name: "Echo", content: "tool finished"})
	provider := newParityFakeProvider([]parityProviderTurn{{
		Events: parityToolUseEventsWithUsage("goal-budget-load-failure-tool", "Echo", `{}`, &types.Usage{OutputTokens: 5}),
	}})
	query := New(provider, reg, Config{
		MaxTurns: 2, MaxTokens: 1024, GoalRuntime: runtime,
	})

	if err := query.Run(context.Background(), "respond", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !transcriptHasToolResult(query.Messages(), "goal-budget-load-failure-tool") {
		t.Fatalf("post-tool goal load failure left an orphaned tool_use: %+v", query.Messages())
	}
}

func transcriptHasToolResult(messages []types.Message, toolUseID string) bool {
	for _, message := range messages {
		for _, block := range message.Content {
			result, ok := block.(types.ToolResultBlock)
			if ok && result.ToolUseID == toolUseID {
				return true
			}
		}
	}
	return false
}

func TestGoalContinuationRejectsInvalidEvaluatorReasons(t *testing.T) {
	tests := []struct {
		name   string
		result GoalEvaluationResult
		err    error
	}{
		{name: "empty", result: goalContinuationResult(true, " \n\t ", nil)},
		{name: "too long", result: goalContinuationResult(true, strings.Repeat("界", 513), nil)},
		{name: "unbounded error", err: errors.New(strings.Repeat("界", 1000))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := activeGoalForContinuation(t, 0)
			runtime := &goalContinuationRuntime{current: &current}
			evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{
				result: tt.result,
				err:    tt.err,
			}}}
			provider := newParityFakeProvider([]parityProviderTurn{
				{Events: endTurnTextEvents("answer", 7)},
				{Events: endTurnTextEvents("must not continue", 7)},
			})
			query := New(provider, registry.New(), Config{
				MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
			})

			if err := query.Run(context.Background(), "finish", func(stream.Event) {}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(provider.Calls) != 1 || evaluator.callCount() != 1 {
				t.Fatalf("provider/evaluator calls = %d/%d, want 1/1", len(provider.Calls), evaluator.callCount())
			}
			final := runtime.snapshot(t)
			if final.Status != goal.StatusActive || final.Usage != 7 || final.TurnCount != 1 {
				t.Fatalf("fail-closed goal progress = %+v", final)
			}
			if reason := final.LastEvaluatorReason; reason == "" || utf8.RuneCountInString(reason) > goalEvaluatorMaxReasonRunes {
				t.Fatalf("persisted failure reason has %d characters: %q", utf8.RuneCountInString(reason), reason)
			}
			if final.LastEvaluatorReasonKind != goal.EvaluatorReasonFailed {
				t.Fatalf("persisted failure kind = %q, want %q", final.LastEvaluatorReasonKind, goal.EvaluatorReasonFailed)
			}
			if tt.err != nil && final.LastEvaluatorReasonDetail == "" {
				t.Fatalf("persisted failure omitted bounded raw detail: %+v", final)
			}
		})
	}
}

func TestGoalContinuationGoalTokenBudgetExhaustionStopsUnmetGoal(t *testing.T) {
	current := activeGoalForContinuation(t, 100)
	current.Usage = 90
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: goalContinuationResult(false, "more work would exceed the goal budget", nil)}}}
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("budget edge", 10)},
		{Events: endTurnTextEvents("must not continue", 10)},
	})
	query := New(provider, registry.New(), Config{
		MaxTurns: 3, MaxTokens: 1024, GoalRuntime: runtime, GoalEvaluator: evaluator,
	})
	var events []stream.Event
	if err := query.Run(context.Background(), "finish", func(event stream.Event) { events = append(events, event) }); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(provider.Calls) != 1 || evaluator.callCount() != 1 {
		t.Fatalf("provider/evaluator calls = %d/%d, want 1/1", len(provider.Calls), evaluator.callCount())
	}
	final := runtime.snapshot(t)
	if final.Status != goal.StatusActive || final.Usage != 100 || final.TurnCount != 1 || final.LastEvaluatorReason != "more work would exceed the goal budget" {
		t.Fatalf("budget-exhausted goal = %+v", final)
	}
	if !hasGoalContinuationFailure(events, "budget") {
		t.Fatalf("goal budget exhaustion was not visible: events=%+v", events)
	}
}

func TestGoalContinuationDoesNotBypassExistingTokenBudgetStop(t *testing.T) {
	current := activeGoalForContinuation(t, 1000)
	runtime := &goalContinuationRuntime{current: &current}
	evaluator := &goalContinuationEvaluator{steps: []goalContinuationEvaluatorStep{{result: goalContinuationResult(false, "goal evaluator requests more work", nil)}}}
	provider := newParityFakeProvider([]parityProviderTurn{
		{Events: endTurnTextEvents("existing budget exhausted", 100)},
		{Events: endTurnTextEvents("must not continue", 10)},
	})
	query := New(provider, registry.New(), Config{
		MaxTurns: 3, MaxTokens: 1024, TokenBudget: 100,
		GoalRuntime: runtime, GoalEvaluator: evaluator,
	})

	if err := query.Run(context.Background(), "finish", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(provider.Calls) != 1 || evaluator.callCount() != 1 {
		t.Fatalf("provider/evaluator calls = %d/%d, want 1/1", len(provider.Calls), evaluator.callCount())
	}
	final := runtime.snapshot(t)
	if final.Status != goal.StatusActive || final.Usage != 100 || final.TurnCount != 1 || final.LastEvaluatorReason != "goal evaluator requests more work" {
		t.Fatalf("existing-budget goal progress = %+v", final)
	}
}

func hasGoalContinuationFailure(events []stream.Event, contains string) bool {
	contains = strings.ToLower(contains)
	for _, event := range events {
		if event.Type != stream.EventError && event.Type != stream.EventSystemWarning {
			continue
		}
		text := event.Text
		if event.Type == stream.EventSystemWarning {
			text = projectedSystemWarningText(event)
		}
		if strings.Contains(strings.ToLower(text), contains) {
			return true
		}
	}
	return false
}

func assertGoalEvaluationUsageEvent(t *testing.T, events []stream.Event, wantEvaluator types.Usage, wantTurnOutput int) {
	t.Helper()
	var evaluatorEvents []stream.Event
	var turnEnds []stream.Event
	for _, event := range events {
		switch event.Type {
		case stream.EventGoalEvaluation:
			evaluatorEvents = append(evaluatorEvents, event)
		case stream.EventTurnEnd:
			turnEnds = append(turnEnds, event)
		}
	}
	if len(evaluatorEvents) != 1 {
		t.Fatalf("goal evaluation events = %d, want 1: %+v", len(evaluatorEvents), events)
	}
	evaluatorEvent := evaluatorEvents[0]
	if evaluatorEvent.Usage == nil || *evaluatorEvent.Usage != wantEvaluator {
		t.Fatalf("goal evaluation usage = %+v, want %+v", evaluatorEvent.Usage, wantEvaluator)
	}
	if evaluatorEvent.Metadata["kind"] != "goal_evaluator" {
		t.Fatalf("goal evaluation metadata = %+v", evaluatorEvent.Metadata)
	}
	if usageID, _ := evaluatorEvent.Metadata["usage_id"].(string); !strings.HasPrefix(usageID, "goal_evaluation:") {
		t.Fatalf("goal evaluation lacks a stable accounting identity: %+v", evaluatorEvent.Metadata)
	}
	if len(turnEnds) != 1 || turnEnds[0].Usage == nil || turnEnds[0].Usage.OutputTokens != wantTurnOutput {
		t.Fatalf("main turn_end usage was replaced or merged: %+v", turnEnds)
	}
}
