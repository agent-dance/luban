package hooks

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBlockingTransitionRollsBackBlockedMutation(t *testing.T) {
	runner := NewRunner([]Hook{{
		Type:    HookTaskCreated,
		Command: `printf '%s' '{"block":true,"system_reminder":"policy rejected task"}'`,
		Timeout: 1,
	}})

	state := "initial"
	err := runner.RunBlockingTransition(
		context.Background(),
		HookTaskCreated,
		HookInput{TaskID: "42", TaskSubject: "ship"},
		func() error {
			state = "partial"
			return nil
		},
		func() error {
			state = "initial"
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "policy rejected task") {
		t.Fatalf("expected blocking hook error, got %v", err)
	}
	if state != "initial" {
		t.Fatalf("blocked transition left partial state: %q", state)
	}
}

func TestBlockingTransitionRollsBackWhenHookContextIsAborted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	state := "initial"
	err := NewRunner(nil).RunBlockingTransition(
		ctx,
		HookTaskCompleted,
		HookInput{TaskID: "42"},
		func() error {
			state = "partial"
			return nil
		},
		func() error {
			state = "initial"
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if state != "initial" {
		t.Fatalf("aborted transition left partial state: %q", state)
	}
}

func TestBlockingTransitionReportsRollbackFailure(t *testing.T) {
	runner := NewRunner([]Hook{{
		Type:    HookTaskCreated,
		Command: `printf '%s' '{"block":true,"system_reminder":"blocked"}'`,
		Timeout: 1,
	}})

	err := runner.RunBlockingTransition(
		context.Background(),
		HookTaskCreated,
		HookInput{TaskID: "42"},
		func() error { return nil },
		func() error { return errors.New("rollback failed") },
	)
	if err == nil || !strings.Contains(err.Error(), "blocked") || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("expected hook and rollback failures, got %v", err)
	}
}
