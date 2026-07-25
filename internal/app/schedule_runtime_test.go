package app

import (
	"context"
	"sync"
	"testing"
	"time"

	agentruntime "github.com/agent-dance/luban/internal/agent"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/internal/tools/schedule"
)

type recordingScheduledPromptRunner struct {
	mu      sync.Mutex
	calls   int
	agentID string
	input   agentcontract.Input
	started chan struct{}
	release chan struct{}
}

func (r *recordingScheduledPromptRunner) RunScheduledPrompt(ctx context.Context, agentID string, input agentcontract.Input) (string, error) {
	r.mu.Lock()
	r.calls++
	r.agentID = agentID
	r.input = input
	if r.calls == 1 {
		close(r.started)
	}
	r.mu.Unlock()
	select {
	case <-r.release:
		return "scheduled result", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestAppScheduleExecutorDurablyDeduplicatesDelivery(t *testing.T) {
	root := t.TempDir()
	background := agentruntime.NewBackgroundTaskManager(root)
	cleanupBackgroundTaskManager(t, background)
	runner := &recordingScheduledPromptRunner{started: make(chan struct{}), release: make(chan struct{})}
	executor := &appScheduleExecutor{agent: runner, background: background}
	execution := schedule.Execution{
		DeliveryID:  "job-1:2026-07-25T10:05:00Z",
		ScheduledAt: time.Date(2026, time.July, 25, 10, 5, 0, 0, time.UTC),
		Job:         schedule.Job{ID: "job-1", Prompt: "build a game", ProjectRoot: root},
	}
	if err := executor.Enqueue(context.Background(), execution); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	if err := executor.Enqueue(context.Background(), execution); err != nil {
		t.Fatalf("duplicate Enqueue: %v", err)
	}
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled Agent runner did not start")
	}
	runner.mu.Lock()
	calls, agentID, input := runner.calls, runner.agentID, runner.input
	runner.mu.Unlock()
	if calls != 1 {
		t.Fatalf("Agent calls = %d, want 1", calls)
	}
	if agentID != scheduleAgentID(root, execution.DeliveryID) || input.Prompt != execution.Job.Prompt || input.CWD != root {
		t.Fatalf("scheduled Agent identity/input = %q %#v", agentID, input)
	}
	close(runner.release)
}

func TestAppScheduleFireSinkUsesStableLifecycleIdentity(t *testing.T) {
	root := t.TempDir()
	event := schedule.FireEvent{
		DeliveryID: "job-2:2026-07-25T11:00:00Z", JobID: "job-2",
		Expression: "0 11 * * *", Recurring: true, Durable: true,
		ScheduledAt: time.Date(2026, time.July, 25, 11, 0, 0, 0, time.UTC), ProjectRoot: root,
	}
	sink := appScheduleFireSink{}
	if err := sink.PublishScheduleFire(context.Background(), event); err != nil {
		t.Fatalf("first fire: %v", err)
	}
	if err := sink.PublishScheduleFire(context.Background(), event); err != nil {
		t.Fatalf("duplicate fire: %v", err)
	}
	events, err := runtimestore.NewRuntimeLifecycle(root).Events()
	if err != nil {
		t.Fatalf("read lifecycle: %v", err)
	}
	if len(events) != 1 || events[0].ID != event.DeliveryID || events[0].EntityID != event.JobID {
		t.Fatalf("lifecycle events = %#v", events)
	}
	if _, leaked := events[0].Payload["prompt"]; leaked {
		t.Fatalf("schedule fire leaked prompt: %#v", events[0].Payload)
	}
}
