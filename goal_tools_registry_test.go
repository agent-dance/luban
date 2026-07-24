package main

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
)

type registryGoalRuntime struct {
	current *goal.Goal
}

func (r *registryGoalRuntime) LoadGoal() (*goal.Goal, error) {
	if r.current == nil {
		return nil, nil
	}
	current := *r.current
	return &current, nil
}

func (r *registryGoalRuntime) SaveGoal(next goal.Goal) error {
	saved := next
	r.current = &saved
	return nil
}

func TestSetupRegistryRegistersGoalToolsAndInjectsSharedRuntime(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	if deps.CronStore != nil {
		t.Cleanup(deps.CronStore.Stop)
	}
	t.Cleanup(deps.StopWebFetchCache)

	registered := map[string]any{
		"GetGoal":    deps.GetGoalTool,
		"CreateGoal": deps.CreateGoalTool,
		"UpdateGoal": deps.UpdateGoalTool,
	}
	for name, dependency := range registered {
		if dependency == nil {
			t.Fatalf("%s dependency is nil", name)
		}
		if got := deps.Registry.Get(name); got != dependency {
			t.Fatalf("registered %s = %T, dependency = %T", name, got, dependency)
		}
	}

	runtime := &registryGoalRuntime{}
	deps.SetGoalRuntime(runtime)
	result, err := deps.Registry.ExecuteToolWithError(context.Background(), "CreateGoal", map[string]any{
		"objective": "verify root goal registration", "acceptance_criteria": []string{"root goal registration is verified"},
	})
	if err != nil || result.IsError {
		t.Fatalf("CreateGoal through root registry: result=%+v err=%v", result, err)
	}
	if runtime.current == nil || runtime.current.Objective != "verify root goal registration" || runtime.current.Status != goal.StatusActive {
		t.Fatalf("injected goal runtime state = %+v", runtime.current)
	}
}
