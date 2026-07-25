package goaltool

import (
	"context"
	"fmt"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/types"
)

type goalToolSessionRoute struct {
	sessionID   string
	projectRoot string
}

type atomicSessionRoutingGoalRuntime struct {
	fallback goalToolSessionRoute
	goals    map[goalToolSessionRoute]*goal.Goal

	fallbackLoads   int
	fallbackSaves   int
	fallbackUpdates int
	contextLoads    int
	contextSaves    int
	contextUpdates  int
}

func newAtomicSessionRoutingGoalRuntime(fallback goalToolSessionRoute) *atomicSessionRoutingGoalRuntime {
	return &atomicSessionRoutingGoalRuntime{
		fallback: fallback,
		goals:    make(map[goalToolSessionRoute]*goal.Goal),
	}
}

func (r *atomicSessionRoutingGoalRuntime) LoadGoal() (*goal.Goal, error) {
	r.fallbackLoads++
	return cloneSessionRoutingGoal(r.goals[r.fallback]), nil
}

func (r *atomicSessionRoutingGoalRuntime) SaveGoal(next goal.Goal) error {
	r.fallbackSaves++
	r.goals[r.fallback] = cloneSessionRoutingGoal(&next)
	return nil
}

func (r *atomicSessionRoutingGoalRuntime) UpdateGoal(update goal.UpdateFunc) (goal.Goal, error) {
	r.fallbackUpdates++
	return r.apply(r.fallback, update)
}

func (r *atomicSessionRoutingGoalRuntime) LoadGoalForContext(ctx context.Context) (*goal.Goal, error) {
	r.contextLoads++
	route, err := goalToolRouteFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return cloneSessionRoutingGoal(r.goals[route]), nil
}

func (r *atomicSessionRoutingGoalRuntime) SaveGoalForContext(ctx context.Context, next goal.Goal) error {
	r.contextSaves++
	route, err := goalToolRouteFromContext(ctx)
	if err != nil {
		return err
	}
	r.goals[route] = cloneSessionRoutingGoal(&next)
	return nil
}

func (r *atomicSessionRoutingGoalRuntime) UpdateGoalForContext(ctx context.Context, update goal.UpdateFunc) (goal.Goal, error) {
	r.contextUpdates++
	route, err := goalToolRouteFromContext(ctx)
	if err != nil {
		return goal.Goal{}, err
	}
	return r.apply(route, update)
}

func (r *atomicSessionRoutingGoalRuntime) apply(route goalToolSessionRoute, update goal.UpdateFunc) (goal.Goal, error) {
	next, err := update(cloneSessionRoutingGoal(r.goals[route]))
	if err != nil {
		return goal.Goal{}, err
	}
	r.goals[route] = cloneSessionRoutingGoal(&next)
	return next, nil
}

var _ GoalRuntime = (*atomicSessionRoutingGoalRuntime)(nil)
var _ ContextGoalRuntime = (*atomicSessionRoutingGoalRuntime)(nil)
var _ goal.Updater = (*atomicSessionRoutingGoalRuntime)(nil)
var _ goal.ContextUpdater = (*atomicSessionRoutingGoalRuntime)(nil)

func goalToolRouteFromContext(ctx context.Context) (goalToolSessionRoute, error) {
	exec, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok || exec.SessionID == "" || exec.ProjectRoot == "" {
		return goalToolSessionRoute{}, fmt.Errorf("goal tool execution context requires session ID and project root")
	}
	return goalToolSessionRoute{sessionID: exec.SessionID, projectRoot: exec.ProjectRoot}, nil
}

func cloneSessionRoutingGoal(current *goal.Goal) *goal.Goal {
	if current == nil {
		return nil
	}
	cloned := *current
	return &cloned
}

func TestGoalToolsRouteByToolExecutionContext(t *testing.T) {
	routeA := goalToolSessionRoute{sessionID: "session-a", projectRoot: "/workspace/a"}
	routeB := goalToolSessionRoute{sessionID: "session-b", projectRoot: "/workspace/b"}
	ctxA := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		SessionID:   routeA.sessionID,
		ProjectRoot: routeA.projectRoot,
		CWD:         routeA.projectRoot + "/nested",
	})

	t.Run("GetGoal reads the background session", func(t *testing.T) {
		runtime := newAtomicSessionRoutingGoalRuntime(routeB)
		goalA := goalToolTestGoal(t, goal.StatusActive)
		goalA.Objective = "session A goal"
		goalB := goalToolTestGoal(t, goal.StatusActive)
		goalB.Objective = "focused session B goal"
		runtime.goals[routeA] = &goalA
		runtime.goals[routeB] = &goalB

		result := executeGoalToolWithContext(t, ctxA, newGetGoalToolForTest(runtime), map[string]any{})
		got := goalFromSessionRoutingResult(t, result)
		if got == nil || got.Objective != goalA.Objective {
			t.Fatalf("GetGoal returned %+v, want session A goal %+v", got, goalA)
		}
		if runtime.contextLoads != 1 || runtime.contextUpdates != 0 || runtime.fallbackLoads != 0 || runtime.fallbackUpdates != 0 {
			t.Fatalf("context routing calls = loads:%d updates:%d fallback-loads:%d fallback-updates:%d", runtime.contextLoads, runtime.contextUpdates, runtime.fallbackLoads, runtime.fallbackUpdates)
		}
	})

	t.Run("CreateGoal writes the background session", func(t *testing.T) {
		runtime := newAtomicSessionRoutingGoalRuntime(routeB)
		goalB := goalToolTestGoal(t, goal.StatusActive)
		goalB.Objective = "focused session B goal"
		runtime.goals[routeB] = &goalB

		result := executeGoalToolWithContext(t, ctxA, newCreateGoalToolForTest(runtime), map[string]any{
			"objective": "new session A goal", "acceptance_criteria": []string{"session A verification passes"},
		})
		created := goalFromSessionRoutingResult(t, result)
		if created == nil || created.Objective != "new session A goal" {
			t.Fatalf("CreateGoal returned %+v", created)
		}
		if got := runtime.goals[routeA]; got == nil || got.Objective != "new session A goal" {
			t.Fatalf("session A persisted goal = %+v", got)
		}
		if got := runtime.goals[routeB]; got == nil || got.Objective != goalB.Objective || got.Status != goalB.Status {
			t.Fatalf("CreateGoal changed focused session B: got %+v, want %+v", got, goalB)
		}
		if runtime.contextUpdates != 1 || runtime.contextLoads != 0 || runtime.fallbackUpdates != 0 {
			t.Fatalf("context routing calls = loads:%d updates:%d fallback-updates:%d", runtime.contextLoads, runtime.contextUpdates, runtime.fallbackUpdates)
		}
	})

	t.Run("UpdateGoal writes the background session", func(t *testing.T) {
		runtime := newAtomicSessionRoutingGoalRuntime(routeB)
		goalA := goalToolTestGoal(t, goal.StatusActive)
		goalA.Objective = "session A goal"
		goalB := goalToolTestGoal(t, goal.StatusActive)
		goalB.Objective = "focused session B goal"
		runtime.goals[routeA] = &goalA
		runtime.goals[routeB] = &goalB

		result := executeGoalToolWithContext(t, ctxA, newUpdateGoalToolForTest(runtime), map[string]any{
			"status": "complete",
		})
		updated := goalFromSessionRoutingResult(t, result)
		if updated == nil || updated.Objective != goalA.Objective || updated.Status != goal.StatusAchieved {
			t.Fatalf("UpdateGoal returned %+v, want achieved session A goal", updated)
		}
		if got := runtime.goals[routeA]; got == nil || got.Status != goal.StatusAchieved {
			t.Fatalf("session A persisted goal = %+v, want achieved", got)
		}
		if got := runtime.goals[routeB]; got == nil || got.Objective != goalB.Objective || got.Status != goal.StatusActive {
			t.Fatalf("UpdateGoal changed focused session B: got %+v, want %+v", got, goalB)
		}
		if runtime.contextUpdates != 1 || runtime.contextLoads != 0 || runtime.fallbackUpdates != 0 {
			t.Fatalf("context routing calls = loads:%d updates:%d fallback-updates:%d", runtime.contextLoads, runtime.contextUpdates, runtime.fallbackUpdates)
		}
	})
}

func TestGoalToolsUseFocusedRuntimeWithoutExecutionContext(t *testing.T) {
	fallback := goalToolSessionRoute{sessionID: "focused-session", projectRoot: "/workspace/focused"}
	runtime := newAtomicSessionRoutingGoalRuntime(fallback)
	getTool, createTool, updateTool := NewGoalTools(runtime)

	createdResult := executeGoalToolWithContext(t, context.Background(), createTool, map[string]any{
		"objective": "fallback goal", "acceptance_criteria": []string{"fallback verification passes"},
	})
	created := goalFromSessionRoutingResult(t, createdResult)
	if created == nil || created.Objective != "fallback goal" {
		t.Fatalf("fallback CreateGoal returned %+v", created)
	}

	updatedResult := executeGoalToolWithContext(t, context.Background(), updateTool, map[string]any{
		"status": "blocked",
	})
	updated := goalFromSessionRoutingResult(t, updatedResult)
	if updated == nil || updated.Status != goal.StatusBlocked {
		t.Fatalf("fallback UpdateGoal returned %+v", updated)
	}

	getResult := executeGoalToolWithContext(t, context.Background(), getTool, map[string]any{})
	got := goalFromSessionRoutingResult(t, getResult)
	if got == nil || got.Objective != "fallback goal" || got.Status != goal.StatusBlocked {
		t.Fatalf("fallback GetGoal returned %+v", got)
	}
	if runtime.contextLoads != 0 || runtime.contextUpdates != 0 {
		t.Fatalf("context runtime used without execution context: loads=%d updates=%d", runtime.contextLoads, runtime.contextUpdates)
	}
	if runtime.fallbackLoads != 1 || runtime.fallbackUpdates != 2 {
		t.Fatalf("focused runtime calls: loads=%d updates=%d, want loads=1 updates=2", runtime.fallbackLoads, runtime.fallbackUpdates)
	}
}

func TestGoalToolExecutionContextAcceptsExactSessionProjectDirWithoutProjectRoot(t *testing.T) {
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		SessionID:         "session",
		SessionProjectDir: "/session-store/project",
	})
	if !hasGoalToolExecutionContext(ctx) {
		t.Fatal("exact session project namespace was not recognized as a routable Goal tool context")
	}

	partial := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{SessionID: "session"})
	if hasGoalToolExecutionContext(partial) {
		t.Fatal("context without a session project namespace or project root must not be routable")
	}
}

func TestGoalToolsPreferAtomicUpdaterForCreateAndUpdate(t *testing.T) {
	routeA := goalToolSessionRoute{sessionID: "session-a", projectRoot: "/workspace/a"}
	routeB := goalToolSessionRoute{sessionID: "session-b", projectRoot: "/workspace/b"}
	ctxA := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		SessionID:   routeA.sessionID,
		ProjectRoot: routeA.projectRoot,
		CWD:         routeA.projectRoot + "/nested",
	})

	t.Run("fallback updater", func(t *testing.T) {
		runtime := newAtomicSessionRoutingGoalRuntime(routeB)
		_, createTool, updateTool := NewGoalTools(runtime)

		executeGoalToolWithContext(t, context.Background(), createTool, map[string]any{
			"objective": "atomic fallback goal", "acceptance_criteria": []string{"atomic verification passes"},
		})
		executeGoalToolWithContext(t, context.Background(), updateTool, map[string]any{
			"status": "blocked",
		})

		got := runtime.goals[routeB]
		if got == nil || got.Objective != "atomic fallback goal" || got.Status != goal.StatusBlocked {
			t.Fatalf("fallback goal = %+v, want blocked atomic fallback goal", got)
		}
		if runtime.fallbackUpdates != 2 {
			t.Fatalf("fallback atomic updates = %d, want 2", runtime.fallbackUpdates)
		}
		if runtime.fallbackLoads != 0 || runtime.fallbackSaves != 0 {
			t.Fatalf("fallback legacy persistence used despite goal.Updater: loads=%d saves=%d", runtime.fallbackLoads, runtime.fallbackSaves)
		}
	})

	t.Run("context updater", func(t *testing.T) {
		runtime := newAtomicSessionRoutingGoalRuntime(routeB)
		goalB := goalToolTestGoal(t, goal.StatusActive)
		goalB.Objective = "focused session B goal"
		runtime.goals[routeB] = &goalB
		_, createTool, updateTool := NewGoalTools(runtime)

		executeGoalToolWithContext(t, ctxA, createTool, map[string]any{
			"objective": "atomic session A goal", "acceptance_criteria": []string{"session A verification passes"},
		})
		executeGoalToolWithContext(t, ctxA, updateTool, map[string]any{
			"status": "blocked",
		})

		gotA := runtime.goals[routeA]
		if gotA == nil || gotA.Objective != "atomic session A goal" || gotA.Status != goal.StatusBlocked {
			t.Fatalf("session A goal = %+v, want blocked atomic session A goal", gotA)
		}
		gotB := runtime.goals[routeB]
		if gotB == nil || gotB.Objective != goalB.Objective || gotB.Status != goal.StatusActive {
			t.Fatalf("context atomic update changed focused session B: got %+v, want %+v", gotB, goalB)
		}
		if runtime.contextUpdates != 2 {
			t.Fatalf("context atomic updates = %d, want 2", runtime.contextUpdates)
		}
		if runtime.contextLoads != 0 || runtime.contextSaves != 0 || runtime.fallbackLoads != 0 || runtime.fallbackSaves != 0 || runtime.fallbackUpdates != 0 {
			t.Fatalf("non-context or legacy persistence used despite goal.ContextUpdater: context loads=%d saves=%d; fallback loads=%d saves=%d updates=%d",
				runtime.contextLoads, runtime.contextSaves, runtime.fallbackLoads, runtime.fallbackSaves, runtime.fallbackUpdates)
		}
	})
}

func executeGoalToolWithContext(t *testing.T, ctx context.Context, tool types.Tool, input map[string]any) types.ToolResult {
	t.Helper()
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("%s Execute() error = %v", tool.Name(), err)
	}
	if result.IsError {
		t.Fatalf("%s returned tool error: %+v", tool.Name(), result)
	}
	return result
}

func goalFromSessionRoutingResult(t *testing.T, result types.ToolResult) *goal.Goal {
	t.Helper()
	data, ok := result.Data.(GoalToolResult)
	if !ok {
		t.Fatalf("goal tool result data = %T, want GoalToolResult", result.Data)
	}
	return data.Goal
}
