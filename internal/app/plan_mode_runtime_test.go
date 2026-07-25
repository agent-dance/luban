package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	runtimescope "github.com/agent-dance/luban/internal/runtime/scope"
	toolinteraction "github.com/agent-dance/luban/internal/tools/interaction"
	"github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/permissions"
)

func newAppTestPlanState(t *testing.T, root string) *toolinteraction.PlanState {
	t.Helper()
	state, err := toolinteraction.NewPlanState(root)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestPlanModeRuntimeUsesRealPermissionCheckerAndRestores(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	checker := permissions.NewChecker(permissions.ModeAllowAll, nil)
	scope := runtimescope.NewRuntimeScope(root, true)
	state := newAppTestPlanState(t, root)
	enter := toolinteraction.NewEnterPlanModeTool(state, scope)
	if err := bindPlanModePermissionDispatcher(scope, state, checker); err != nil {
		t.Fatalf("bind dispatcher: %v", err)
	}

	result, err := enter.Execute(context.Background(), map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("EnterPlanMode: err=%v result=%#v", err, result)
	}
	if checker.Mode() != permissions.ModeAskAlways || scope.PermissionMode() != "plan" {
		t.Fatalf("plan transition did not update real checker/runtime: checker=%v runtime=%q", checker.Mode(), scope.PermissionMode())
	}

	if err := state.ExitForModeSwitch("bypassPermissions"); err != nil {
		t.Fatal(err)
	}
	if checker.Mode() != permissions.ModeAllowAll || scope.PermissionMode() != "bypassPermissions" {
		t.Fatalf("exit did not restore real checker/runtime: checker=%v runtime=%q", checker.Mode(), scope.PermissionMode())
	}
}

func TestPlanModeRuntimeResumeReappliesPlanModeAndPublishesUIFlag(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	firstScope := runtimescope.NewRuntimeScope(root, true)
	firstScope.SetPermissionModeDispatcher(func() string { return "bypassPermissions" }, func(string) error { return nil })
	firstState := newAppTestPlanState(t, root)
	result, err := toolinteraction.NewEnterPlanModeTool(firstState, firstScope).Execute(context.Background(), map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("initial EnterPlanMode: err=%v result=%#v", err, result)
	}

	resumed := newAppTestPlanState(t, root)
	checker := permissions.NewChecker(permissions.ModeAllowAll, nil)
	scope := runtimescope.NewRuntimeScope(root, true)
	_ = toolinteraction.NewEnterPlanModeTool(resumed, scope) // binds exit restoration for the resumed state
	var observed []string
	scope.SetPermissionModeObserver(func(mode string) { observed = append(observed, mode) })
	observed = nil
	if err := bindPlanModePermissionDispatcher(scope, resumed, checker); err != nil {
		t.Fatalf("bind resumed dispatcher: %v", err)
	}
	if !resumed.IsActive() || checker.Mode() != permissions.ModeAskAlways || scope.PermissionMode() != "plan" {
		t.Fatalf("resume did not reapply active plan mode: active=%v checker=%v runtime=%q", resumed.IsActive(), checker.Mode(), scope.PermissionMode())
	}
	if len(observed) == 0 || observed[len(observed)-1] != "plan" {
		t.Fatalf("UI observer did not receive resumed plan mode: %#v", observed)
	}

	if err := resumed.ExitForModeSwitch("bypassPermissions"); err != nil {
		t.Fatal(err)
	}
	if checker.Mode() != permissions.ModeAllowAll || scope.PermissionMode() != "bypassPermissions" {
		t.Fatalf("resumed exit did not restore pre-plan mode: checker=%v runtime=%q", checker.Mode(), scope.PermissionMode())
	}
}

func TestRuntimeObserverSkipsAlreadyPublishedMode(t *testing.T) {
	state := tui.NewAppState()
	state.Mode.Set(tui.ModePlanEdit)
	enqueued := false
	publishTUIRuntimePermissionMode(state, func(fn func()) bool {
		enqueued = true
		fn()
		return true
	}, "plan")
	if enqueued {
		t.Fatal("observer re-enqueued a Shift+Tab mode already published by the event loop")
	}
}

func TestModeSwitchDoesNotInterleaveWithSessionCommit(t *testing.T) {
	transitionMu := &sync.Mutex{}
	transitionMu.Lock()
	defer transitionMu.Unlock()
	state := tui.NewAppState()
	state.Mode.Set(tui.ModePlanEdit) // Root published Ask -> Plan before callback.
	err := applyTUIInteractionModeAtSessionBoundary(TUIREPLConfig{SessionTransitionMu: transitionMu}, state, tui.ModePlanEdit)
	if err == nil {
		t.Fatal("mode switch entered an in-flight session commit")
	}
	if state.Mode.Get() != tui.ModeAskEdit {
		t.Fatalf("busy session commit did not restore previous mode: %v", state.Mode.Get())
	}
}

func TestPlanModeTransitionFailureRollsBackPublishedPresentation(t *testing.T) {
	root := t.TempDir()
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	scope := runtimescope.NewRuntimeScope(root, true)
	plan := newAppTestPlanState(t, root)
	_ = toolinteraction.NewEnterPlanModeTool(plan, scope)
	scope.SetPermissionModeDispatcher(func() string { return "default" }, func(mode string) error {
		if mode == "plan" {
			return errors.New("plan transition rejected")
		}
		return setPermissionCheckerMode(checker, mode)
	})
	state := tui.NewAppState()
	state.Mode.Set(tui.ModePlanEdit) // Root publishes the requested mode first.
	err := applyTUIInteractionMode(TUIREPLConfig{RuntimeScope: scope, PermChecker: checker, PlanState: plan}, state, tui.ModePlanEdit)
	if err == nil {
		t.Fatal("plan transition unexpectedly succeeded")
	}
	if state.Mode.Get() != tui.ModeAskEdit || plan.IsActive() || scope.PermissionMode() != "default" || checker.Mode() != permissions.ModeAskAlways {
		t.Fatalf("failed transition left half-published state: ui=%v plan=%v runtime=%q checker=%v", state.Mode.Get(), plan.IsActive(), scope.PermissionMode(), checker.Mode())
	}
}

func TestPlanModePersistenceFailureNeverPublishesPlan(t *testing.T) {
	base := t.TempDir()
	invalidRoot := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(invalidRoot, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := toolinteraction.NewPlanState(invalidRoot); err == nil {
		t.Fatal("PlanState construction ignored invalid project root")
	}
}

func TestRuntimeScopeRestorePermissionModeDefersUIPublication(t *testing.T) {
	root := t.TempDir()
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	scope := runtimescope.NewRuntimeScope(root, true)
	scope.SetPermissionModeDispatcher(func() string { return "default" }, func(mode string) error {
		if mode == "bypassPermissions" {
			return checker.SetModeFromUser(permissions.ModeAllowAll)
		}
		return checker.SetMode(permissions.ModeAskAlways)
	})
	var observed []string
	scope.SetPermissionModeObserver(func(mode string) { observed = append(observed, mode) })
	observed = nil

	if err := scope.RestorePermissionMode("bypassPermissions"); err != nil {
		t.Fatalf("RestorePermissionMode: %v", err)
	}
	if checker.Mode() != permissions.ModeAllowAll || scope.PermissionMode() != "bypassPermissions" {
		t.Fatalf("restore did not update runtime: checker=%v runtime=%q", checker.Mode(), scope.PermissionMode())
	}
	if len(observed) != 0 {
		t.Fatalf("restore published presentation before session snapshot: %#v", observed)
	}

	if err := scope.TransitionPermissionMode("default"); err != nil {
		t.Fatalf("TransitionPermissionMode: %v", err)
	}
	if len(observed) != 1 || observed[0] != "default" {
		t.Fatalf("interactive transition did not publish UI mode: %#v", observed)
	}
}

func TestSessionRestoreOverridesPrePlanModeWithoutIntermediatePublication(t *testing.T) {
	root := t.TempDir()
	checker := permissions.NewChecker(permissions.ModeAllowAll, nil)
	scope := runtimescope.NewRuntimeScope(root, true)
	state := newAppTestPlanState(t, root)
	enter := toolinteraction.NewEnterPlanModeTool(state, scope)
	if err := bindPlanModePermissionDispatcher(scope, state, checker); err != nil {
		t.Fatalf("bind dispatcher: %v", err)
	}
	if result, err := enter.Execute(context.Background(), map[string]any{}); err != nil || result.IsError {
		t.Fatalf("EnterPlanMode: err=%v result=%#v", err, result)
	}
	var observed []string
	scope.SetPermissionModeObserver(func(mode string) { observed = append(observed, mode) })
	observed = nil

	if err := applyTUISessionPermissionMode(TUIREPLConfig{RuntimeScope: scope, PermChecker: checker, PlanState: state}, tui.ModeAutoEdit); err != nil {
		t.Fatalf("apply session mode: %v", err)
	}
	if state.IsActive() || checker.Mode() != permissions.ModeAllowAll || scope.PermissionMode() != "bypassPermissions" {
		t.Fatalf("session restore disagrees: active=%v checker=%v runtime=%q", state.IsActive(), checker.Mode(), scope.PermissionMode())
	}
	if len(observed) != 0 {
		t.Fatalf("session restore exposed an intermediate mode: %#v", observed)
	}
}
