package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/registry"
)

type fakeTeammateContextProvider struct {
	ctx TeammateContext
	ok  bool
	err error
}

func (p fakeTeammateContextProvider) CurrentTeammateContext(context.Context) (TeammateContext, bool, error) {
	return p.ctx, p.ok, p.err
}

func TestTaskCompletedHookReceivesTaskAndTeammateContextBeforeIdle(t *testing.T) {
	dir := t.TempDir()
	orderPath := filepath.Join(dir, "order.txt")
	taskInputPath := filepath.Join(dir, "task-completed.json")
	idleInputPath := filepath.Join(dir, "teammate-idle.json")
	runner := hooks.NewRunner([]hooks.Hook{
		{
			Type:    hooks.HookTaskCompleted,
			Command: fmt.Sprintf(`cat > %q; printf 'task-completed\n' >> %q`, taskInputPath, orderPath),
		},
		{
			Type:    hooks.HookTeammateIdle,
			Command: fmt.Sprintf(`cat > %q; printf 'teammate-idle\n' >> %q`, idleInputPath, orderPath),
		},
	})
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("done")}})
	q := New(prov, registry.New(), Config{
		MaxTurns:   3,
		MaxTokens:  1024,
		HookRunner: runner,
		TeammateContext: fakeTeammateContextProvider{
			ok: true,
			ctx: TeammateContext{
				TeammateName: "builder",
				TeamName:     "alpha",
				Tasks: []TeammateTask{
					{ID: "skip-owner", Subject: "skip", Owner: "reviewer", Status: "in_progress"},
					{ID: "42", Subject: "ship feature", Description: "finish parity", Owner: "builder", Status: "in_progress"},
					{ID: "skip-status", Subject: "skip", Owner: "builder", Status: "pending"},
				},
			},
		},
	})

	if err := q.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	orderData, err := os.ReadFile(orderPath)
	if err != nil {
		t.Fatalf("read order: %v", err)
	}
	if got, want := string(orderData), "task-completed\nteammate-idle\n"; got != want {
		t.Fatalf("hook order = %q, want %q", got, want)
	}

	taskInput := readHookInput(t, taskInputPath)
	if taskInput.HookEventName != hooks.HookTaskCompleted {
		t.Fatalf("task hook_event_name = %q, want TaskCompleted", taskInput.HookEventName)
	}
	if taskInput.TaskID != "42" || taskInput.TaskSubject != "ship feature" || taskInput.TaskDescription != "finish parity" {
		t.Fatalf("task fields = %#v", taskInput)
	}
	if taskInput.TeammateName != "builder" || taskInput.TeamName != "alpha" {
		t.Fatalf("teammate fields = teammate %q team %q", taskInput.TeammateName, taskInput.TeamName)
	}
	if taskInput.TaskOwner != "builder" || taskInput.Owner != "builder" {
		t.Fatalf("owner fields = task_owner %q owner %q, want builder", taskInput.TaskOwner, taskInput.Owner)
	}

	idleInput := readHookInput(t, idleInputPath)
	if idleInput.HookEventName != hooks.HookTeammateIdle {
		t.Fatalf("idle hook_event_name = %q, want TeammateIdle", idleInput.HookEventName)
	}
	if idleInput.TeammateName != "builder" || idleInput.TeamName != "alpha" {
		t.Fatalf("idle teammate fields = teammate %q team %q", idleInput.TeammateName, idleInput.TeamName)
	}
}

func TestTaskCompletedEmitsEvidenceForEachTask(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookTaskCompleted,
		Command: "true",
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("done")}})
	q := New(prov, registry.New(), Config{
		MaxTurns:   3,
		MaxTokens:  1024,
		HookRunner: runner,
		SessionID:  "session-tasks",
		TeammateContext: fakeTeammateContextProvider{
			ok: true,
			ctx: TeammateContext{
				TeammateName: "builder",
				TeamName:     "alpha",
				Tasks: []TeammateTask{
					{ID: "task-a", Subject: "first", Owner: "builder", Status: "in_progress"},
					{ID: "task-b", Subject: "second", Owner: "builder", Status: "in_progress"},
				},
			},
		},
	})

	var summaries []Event
	if err := q.Run(context.Background(), "hi", func(event Event) {
		if event.Type == EventHookSummary {
			summaries = append(summaries, event)
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("TaskCompleted summaries = %d, want one per task: %#v", len(summaries), summaries)
	}
	seenTasks := map[string]bool{}
	seenExecutions := map[string]bool{}
	for _, event := range summaries {
		if event.HookSummary == nil || event.HookSummary.HookName != string(hooks.HookTaskCompleted) || event.HookSummary.Status != "passed" {
			t.Fatalf("unexpected TaskCompleted summary: %#v", event)
		}
		taskID, _ := event.HookSummary.Metadata["task_id"].(string)
		seenTasks[taskID] = true
		if seenExecutions[event.HookSummary.HookExecutionID] {
			t.Fatalf("TaskCompleted execution ID reused: %q", event.HookSummary.HookExecutionID)
		}
		seenExecutions[event.HookSummary.HookExecutionID] = true
		if got := event.HookSummary.Metadata["config_id"]; got != "config-1" {
			t.Fatalf("TaskCompleted config_id = %#v, want config-1", got)
		}
	}
	if !seenTasks["task-a"] || !seenTasks["task-b"] {
		t.Fatalf("TaskCompleted evidence lost task identity: %#v", seenTasks)
	}
}

func TestTaskCompletedPreventContinuationStopsLoop(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookTaskCompleted,
		Command: `printf '%s\n' '{"preventContinuation":true,"stopReason":"task complete gate"}'`,
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: parityTextEvents("done")},
		{Events: parityTextEvents("unexpected")},
	})
	q := New(prov, registry.New(), Config{
		MaxTurns:        3,
		MaxTokens:       1024,
		HookRunner:      runner,
		TeammateContext: teammateContextWithInProgressTask(),
	})

	if err := q.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want 1", len(prov.Calls))
	}
}

func TestTeammateIdleBlockingContinuesNextTurn(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookTeammateIdle,
		Command: `printf 'pick up another task' >&2; exit 2`,
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: parityTextEvents("idle")},
		{Events: parityTextEvents("continued")},
	})
	q := New(prov, registry.New(), Config{
		MaxTurns:        3,
		MaxTokens:       1024,
		HookRunner:      runner,
		TeammateContext: teammateContextWithInProgressTask(),
	})

	if err := q.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.Calls))
	}
	if got := joinedMessageText(prov.Calls[1].Messages); !strings.Contains(got, "TeammateIdle hook feedback:\npick up another task") {
		t.Fatalf("second request messages missing teammate idle feedback: %q", got)
	}
}

func TestTaskCompletedBlockingContinuesNextTurn(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookTaskCompleted,
		Command: `printf 'finish the handoff note' >&2; exit 2`,
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: parityTextEvents("complete")},
		{Events: parityTextEvents("continued")},
	})
	q := New(prov, registry.New(), Config{
		MaxTurns:        3,
		MaxTokens:       1024,
		HookRunner:      runner,
		TeammateContext: teammateContextWithInProgressTask(),
	})

	if err := q.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.Calls))
	}
	if got := joinedMessageText(prov.Calls[1].Messages); !strings.Contains(got, "TaskCompleted hook feedback:\nfinish the handoff note") {
		t.Fatalf("second request messages missing task completed feedback: %q", got)
	}
}

func teammateContextWithInProgressTask() fakeTeammateContextProvider {
	return fakeTeammateContextProvider{
		ok: true,
		ctx: TeammateContext{
			TeammateName: "builder",
			TeamName:     "alpha",
			Tasks: []TeammateTask{
				{ID: "42", Subject: "ship feature", Description: "finish parity", Owner: "builder", Status: "in_progress"},
			},
		},
	}
}
