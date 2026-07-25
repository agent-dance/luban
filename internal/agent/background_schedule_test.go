package agent

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
)

func TestStartScheduledAgentTaskIsDurablyIdempotent(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	runner := func(ctx context.Context, _ string, _ agentcontract.Input, _ io.Writer) (string, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	input := agentcontract.Input{Description: "scheduled test", Prompt: "do the work", CWD: root}

	const callers = 8
	var wait sync.WaitGroup
	wait.Add(callers)
	ids := make(chan string, callers)
	errs := make(chan error, callers)
	for range callers {
		go func() {
			defer wait.Done()
			snapshot, err := manager.StartScheduledAgentTask(context.Background(), "job-1:2026-07-25T00:00:00Z", "schedule-agent", input, runner)
			if err != nil {
				errs <- err
				return
			}
			ids <- snapshot.ID
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("StartScheduledAgentTask: %v", err)
	}
	close(ids)
	wantID := scheduledTaskID(root, "job-1:2026-07-25T00:00:00Z")
	for id := range ids {
		if id != wantID {
			t.Fatalf("task ID = %q, want %q", id, wantID)
		}
	}
	<-started
	if got := calls.Load(); got != 1 {
		t.Fatalf("runner calls = %d, want 1", got)
	}
	close(release)
	if snapshot, status := manager.Wait(wantID, 5*time.Second); status != "success" || snapshot.Status != "completed" {
		t.Fatalf("Wait = status %q snapshot %#v", status, snapshot)
	}
	record, ok := runtimestore.NewRuntimeTaskStore(root).Get(wantID)
	if !ok || record.BatchID != scheduledDeliveryBatchPrefix+"job-1:2026-07-25T00:00:00Z" {
		t.Fatalf("stable delivery claim was not persisted: %#v", record)
	}
}

func TestResumeScheduledAgentTasksRestartsOnlyInterruptedDelivery(t *testing.T) {
	root := t.TempDir()
	deliveryID := "job-2:2026-07-25T01:00:00Z"
	taskID := scheduledTaskID(root, deliveryID)
	input := agentcontract.Input{Description: "scheduled resume", Prompt: "resume the work", CWD: root}
	if err := runtimestore.NewRuntimeTaskStore(root).Save(runtimestore.RuntimeTaskRecord{
		ID: taskID, Type: agentcontract.TaskTypeLocalAgent, Status: "running",
		StartedAt: time.Now().Add(-time.Minute).UTC(), OwnerPID: 1<<31 - 1,
		AgentAlias: "schedule-agent", AgentInput: &input,
		BatchID: scheduledDeliveryBatchPrefix + deliveryID, Attempt: 1,
	}); err != nil {
		t.Fatalf("seed interrupted record: %v", err)
	}

	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	var calls atomic.Int32
	runner := func(_ context.Context, agentID string, got agentcontract.Input, _ io.Writer) (string, error) {
		calls.Add(1)
		if agentID != "schedule-agent" || got.Prompt != input.Prompt || got.CWD != root {
			t.Errorf("resumed identity/input = %q %#v", agentID, got)
		}
		return "resumed", nil
	}
	if err := manager.ResumeScheduledAgentTasks(context.Background(), runner); err != nil {
		t.Fatalf("ResumeScheduledAgentTasks: %v", err)
	}
	if snapshot, status := manager.Wait(taskID, 5*time.Second); status != "success" || snapshot.Status != "completed" {
		t.Fatalf("Wait = status %q snapshot %#v", status, snapshot)
	}
	if err := manager.ResumeScheduledAgentTasks(context.Background(), runner); err != nil {
		t.Fatalf("second ResumeScheduledAgentTasks: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("runner calls after terminal replay = %d, want 1", got)
	}
}

func TestAcceptedScheduledAgentTaskSurvivesSchedulerContextCancellation(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	parent, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	snapshot, err := manager.StartScheduledAgentTask(parent, "rebind-delivery", "schedule-agent", agentcontract.Input{
		Prompt: "finish after rebind", CWD: root,
	}, func(ctx context.Context, _ string, _ agentcontract.Input, _ io.Writer) (string, error) {
		close(started)
		select {
		case <-release:
			return "finished", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	if err != nil {
		t.Fatalf("StartScheduledAgentTask: %v", err)
	}
	<-started
	cancel()
	close(release)
	if completed, status := manager.Wait(snapshot.ID, 5*time.Second); status != "success" || completed.Status != "completed" {
		t.Fatalf("accepted task canceled by scheduler context: status=%q snapshot=%#v", status, completed)
	}
}

func TestScheduledAgentTaskResumesAfterManagerShutdown(t *testing.T) {
	root := t.TempDir()
	first := NewBackgroundTaskManager(root)
	started := make(chan struct{})
	snapshot, err := first.StartScheduledAgentTask(context.Background(), "shutdown-delivery", "schedule-agent", agentcontract.Input{
		Prompt: "resume after shutdown", CWD: root,
	}, func(ctx context.Context, _ string, _ agentcontract.Input, _ io.Writer) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatalf("StartScheduledAgentTask: %v", err)
	}
	<-started
	_ = first.Shutdown(context.Background())
	record, ok := runtimestore.NewRuntimeTaskStore(root).Get(snapshot.ID)
	if !ok || record.TerminalReason != interruptedAgentTerminalReason {
		t.Fatalf("shutdown delivery is not resumable: %#v", record)
	}

	second := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = second.Shutdown(context.Background()) })
	var calls atomic.Int32
	if err := second.ResumeScheduledAgentTasks(context.Background(), func(context.Context, string, agentcontract.Input, io.Writer) (string, error) {
		calls.Add(1)
		return "resumed", nil
	}); err != nil {
		t.Fatalf("ResumeScheduledAgentTasks: %v", err)
	}
	if completed, status := second.Wait(snapshot.ID, 5*time.Second); status != "success" || completed.Status != "completed" {
		t.Fatalf("resumed task did not complete: status=%q snapshot=%#v", status, completed)
	}
	if calls.Load() != 1 {
		t.Fatalf("resume calls = %d, want 1", calls.Load())
	}
}

func TestStartScheduledAgentTaskDoesNotRunBeforeDurableClaim(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	runtimeDir := filepath.Join(root, ".luban-code", "runtime-tasks")
	if err := os.RemoveAll(runtimeDir); err != nil {
		t.Fatalf("remove runtime directory: %v", err)
	}
	if err := os.Symlink(t.TempDir(), runtimeDir); err != nil {
		t.Fatalf("replace runtime directory with symlink: %v", err)
	}
	var calls atomic.Int32
	_, err := manager.StartScheduledAgentTask(context.Background(), "delivery", "schedule-agent", agentcontract.Input{
		Prompt: "must not run", CWD: root,
	}, func(context.Context, string, agentcontract.Input, io.Writer) (string, error) {
		calls.Add(1)
		return "", nil
	})
	if err == nil {
		t.Fatal("StartScheduledAgentTask succeeded without a durable claim")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("runner calls = %d, want 0", got)
	}
}
