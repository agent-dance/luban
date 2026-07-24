package tools

import (
	"testing"
	"time"
)

func TestBackgroundTaskManager_TagAndCleanupShellTasksForAgent(t *testing.T) {
	mgr := NewBackgroundTaskManager(t.TempDir())
	defer mgr.Shutdown()

	// Manually inject two running shell tasks owned by agent_a, plus an
	// agent-type task that must NOT be killed and a finished task that
	// must be skipped.
	now := time.Now().UTC()
	mgr.mu.Lock()
	mgr.tasks["t1"] = &BackgroundTask{
		ID: "t1", Type: backgroundTaskTypeLocalBash, Status: "running",
		OwnerAgentID: "agent_a", StartedAt: now, done: make(chan struct{}),
	}
	mgr.tasks["t2"] = &BackgroundTask{
		ID: "t2", Type: backgroundTaskTypeLocalBash, Status: "running",
		OwnerAgentID: "agent_a", StartedAt: now, done: make(chan struct{}),
	}
	mgr.tasks["t3"] = &BackgroundTask{
		ID: "t3", Type: backgroundTaskTypeLocalAgent, Status: "running",
		OwnerAgentID: "agent_a", StartedAt: now, done: make(chan struct{}),
	}
	mgr.tasks["t4"] = &BackgroundTask{
		ID: "t4", Type: backgroundTaskTypeLocalBash, Status: "completed",
		OwnerAgentID: "agent_a", StartedAt: now, done: make(chan struct{}),
	}
	mgr.tasks["t5"] = &BackgroundTask{
		ID: "t5", Type: backgroundTaskTypeLocalBash, Status: "running",
		OwnerAgentID: "agent_b", StartedAt: now, done: make(chan struct{}),
	}
	// Pre-close done channels so Stop() doesn't block waiting on a process.
	close(mgr.tasks["t1"].done)
	close(mgr.tasks["t2"].done)
	close(mgr.tasks["t3"].done)
	close(mgr.tasks["t4"].done)
	close(mgr.tasks["t5"].done)
	mgr.mu.Unlock()

	cancelled := mgr.CleanupShellTasksForAgent("agent_a")
	if len(cancelled) != 2 {
		t.Fatalf("expected 2 cancellations for agent_a's running shell tasks, got %d (%v)", len(cancelled), cancelled)
	}
	got := map[string]bool{}
	for _, id := range cancelled {
		got[id] = true
	}
	if !got["t1"] || !got["t2"] {
		t.Fatalf("expected t1+t2 cancelled, got %v", cancelled)
	}
}

func TestBackgroundTaskManager_TagShellTaskForAgent_RejectsEmpty(t *testing.T) {
	mgr := NewBackgroundTaskManager(t.TempDir())
	defer mgr.Shutdown()
	// Should not panic on missing task or empty ids.
	mgr.TagShellTaskForAgent("", "agent_a")
	mgr.TagShellTaskForAgent("nope", "")
	mgr.TagShellTaskForAgent("nope", "agent_a")
	if got := mgr.CleanupShellTasksForAgent(""); got != nil {
		t.Fatalf("empty agent id should return nil, got %v", got)
	}
}
