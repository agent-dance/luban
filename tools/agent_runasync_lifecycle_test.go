package tools

import (
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestFinalizeAsyncAgentLifecycle_RunsAllSteps(t *testing.T) {
	mgr := NewBackgroundTaskManager(t.TempDir())
	defer mgr.Shutdown()

	// Tag a fake running shell task so cleanup has work to do.
	now := time.Now().UTC()
	mgr.mu.Lock()
	mgr.tasks["t1"] = &BackgroundTask{
		ID: "t1", Type: backgroundTaskTypeLocalBash, Status: "running",
		OwnerAgentID: "agent_x", StartedAt: now, done: make(chan struct{}),
	}
	close(mgr.tasks["t1"].done)
	mgr.mu.Unlock()

	var (
		mu    sync.Mutex
		got   AsyncAgentNotification
		count int
	)
	SetAsyncAgentNotificationSink(AsyncAgentNotificationSinkFunc(func(n AsyncAgentNotification) {
		mu.Lock()
		defer mu.Unlock()
		got = n
		count++
	}))
	t.Cleanup(func() { SetAsyncAgentNotificationSink(nil) })

	transcript := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "Done — all checks passed."},
		}},
	}
	res := FinalizeAsyncAgentLifecycle(mgr, "agent_x", "explore", "auto", "completed", "/tmp/wt", transcript)
	if res.Verdict != HandoffContinue {
		t.Fatalf("expected HandoffContinue, got %v", res.Verdict)
	}
	if !sliceHasString(res.KilledShellTasks, "t1") {
		t.Fatalf("expected t1 killed, got %v", res.KilledShellTasks)
	}
	if !res.NotifiedSink {
		t.Fatalf("expected notification fired")
	}
	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("expected exactly one notification, got %d", count)
	}
	if got.AgentID != "agent_x" || got.AgentType != "explore" || got.Status != "completed" {
		t.Fatalf("unexpected notification payload: %+v", got)
	}
	if got.WorktreePath != "/tmp/wt" {
		t.Fatalf("worktree path lost: %q", got.WorktreePath)
	}
	if got.Summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestBackgroundTaskSnapshotPreservesActorIdentity(t *testing.T) {
	task := &BackgroundTask{ID: "task", OwnerSessionID: "session", OwnerAgentID: "agent-42", AgentAlias: "builder"}
	snapshot := task.snapshot()
	if snapshot.OwnerAgentID != "agent-42" || snapshot.AgentAlias != "builder" {
		t.Fatalf("live task actor identity = %+v", snapshot)
	}
	restored := snapshotFromRecord(task.recordLocked())
	if restored.OwnerAgentID != "agent-42" || restored.AgentAlias != "builder" {
		t.Fatalf("persisted task actor identity = %+v", restored)
	}
}

func sliceHasString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func TestFinalizeAsyncAgentLifecycle_NoSinkDoesNotPanic(t *testing.T) {
	SetAsyncAgentNotificationSink(nil)
	mgr := NewBackgroundTaskManager(t.TempDir())
	defer mgr.Shutdown()
	res := FinalizeAsyncAgentLifecycle(mgr, "a", "b", "default", "killed", "", nil)
	if res.NotifiedSink {
		t.Fatalf("expected NotifiedSink=false when no sink configured")
	}
}
