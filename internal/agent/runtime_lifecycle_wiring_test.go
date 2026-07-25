package agent

import (
	"context"
	"testing"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
)

func TestLifecycleBackgroundProductionPathDurablyResumes(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	task := &BackgroundTask{
		ID:          "agent-1",
		Type:        agentcontract.TaskTypeLocalAgent,
		Status:      "running",
		Description: "inspect state",
		StartedAt:   time.Now().UTC(),
	}
	manager.registerTask(task)

	active, err := runtimestore.NewRuntimeLifecycle(root).ActiveState()
	if err != nil {
		t.Fatal(err)
	}
	if !agentLifecycleHasType(active, runtimestore.LifecycleTaskCreated) || !agentLifecycleHasType(active, runtimestore.LifecycleToolStart) {
		t.Fatalf("production registration did not publish task/tool start: %#v", active)
	}

	resumed := NewBackgroundTaskManager(root)
	snapshots := resumed.Snapshots()
	if len(snapshots) != 1 || snapshots[0].ID != task.ID || snapshots[0].Status != "running" {
		t.Fatalf("persisted task was not consumed by compaction after resume: %#v", snapshots)
	}

	task.mu.Lock()
	task.Status = "completed"
	code := 0
	task.ExitCode = &code
	finishedAt := time.Now().UTC()
	task.FinishedAt = &finishedAt
	task.mu.Unlock()
	manager.persistCurrentTask(task)
	manager.emitTaskCompletionNotification(context.Background(), task, "completed", 0)

	active, err = runtimestore.NewRuntimeLifecycle(root).ActiveState()
	if err != nil {
		t.Fatal(err)
	}
	if agentLifecycleHasEntity(active, task.ID) {
		t.Fatalf("terminal production path left task/tool active after resume: %#v", active)
	}
}

func agentLifecycleHasType(events []runtimestore.RuntimeLifecycleEvent, eventType runtimestore.RuntimeLifecycleEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func agentLifecycleHasEntity(events []runtimestore.RuntimeLifecycleEvent, entityID string) bool {
	for _, event := range events {
		if event.EntityID == entityID {
			return true
		}
	}
	return false
}
