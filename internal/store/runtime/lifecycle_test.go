package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type recordingLifecycleSink struct {
	store  *RuntimeLifecycle
	events []RuntimeLifecycleEvent
	err    error
}

func (s *recordingLifecycleSink) HandleLifecycleEvent(_ context.Context, event RuntimeLifecycleEvent) error {
	// Publish must durably append before invoking side effects. This is what
	// makes notifications/mailbox consumers replayable after a crash.
	if s.store != nil {
		persisted, err := s.store.Events()
		if err != nil {
			return err
		}
		found := false
		for _, item := range persisted {
			if item.ID == event.ID {
				found = true
				break
			}
		}
		if !found {
			return os.ErrNotExist
		}
	}
	s.events = append(s.events, event)
	return s.err
}

func TestRuntimeLifecyclePersistsResumableActiveState(t *testing.T) {
	root := t.TempDir()
	storageRoot := filepath.Join(t.TempDir(), "runtime")
	store := NewRuntimeLifecycleAt(root, storageRoot)

	started := []RuntimeLifecycleEvent{
		{Type: LifecycleToolStart, EntityID: "tool-1", ToolName: "Bash"},
		{Type: LifecycleTaskCreated, EntityID: "task-1", ToolName: "TaskCreate"},
		{Type: LifecycleWorktreeEnter, EntityID: "wt-1", ToolName: "EnterWorktree"},
		{Type: LifecycleTeamCreate, EntityID: "team-1", ToolName: "TeamCreate"},
	}
	for _, event := range started {
		if err := store.Publish(context.Background(), event); err != nil {
			t.Fatalf("Publish(%s): %v", event.Type, err)
		}
	}

	resumed := NewRuntimeLifecycleAt(root, storageRoot)
	active, err := resumed.ActiveState()
	if err != nil {
		t.Fatalf("ActiveState: %v", err)
	}
	if got := len(active); got != len(started) {
		t.Fatalf("active state count = %d, want %d: %#v", got, len(started), active)
	}

	finished := []RuntimeLifecycleEvent{
		{Type: LifecycleToolComplete, EntityID: "tool-1", ToolName: "Bash"},
		{Type: LifecycleTaskCompleted, EntityID: "task-1", ToolName: "TaskUpdate"},
		{Type: LifecycleWorktreeExit, EntityID: "wt-1", ToolName: "ExitWorktree"},
		{Type: LifecycleTeamDelete, EntityID: "team-1", ToolName: "TeamDelete"},
	}
	for _, event := range finished {
		if err := resumed.Publish(context.Background(), event); err != nil {
			t.Fatalf("Publish(%s): %v", event.Type, err)
		}
	}
	active, err = NewRuntimeLifecycleAt(root, storageRoot).ActiveState()
	if err != nil {
		t.Fatalf("ActiveState after completion: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("completed lifecycle entries remained active: %#v", active)
	}
}

func TestRuntimeLifecyclePersistsBeforeUnifiedDispatch(t *testing.T) {
	root := t.TempDir()
	store := NewRuntimeLifecycleAt(root, filepath.Join(t.TempDir(), "runtime"))
	sinkA := &recordingLifecycleSink{store: store}
	sinkB := &recordingLifecycleSink{store: store}
	store.Subscribe(sinkA)
	store.Subscribe(sinkB)

	if err := store.Publish(context.Background(), RuntimeLifecycleEvent{
		Type:     LifecycleCronFire,
		EntityID: "cron-1",
		ToolName: "CronCreate",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(sinkA.events) != 1 || len(sinkB.events) != 1 {
		t.Fatalf("unified dispatch did not reach all sinks: A=%d B=%d", len(sinkA.events), len(sinkB.events))
	}
	if sinkA.events[0].ID == "" || sinkA.events[0].ID != sinkB.events[0].ID {
		t.Fatalf("sinks observed different lifecycle identities: A=%#v B=%#v", sinkA.events, sinkB.events)
	}
}

func TestRuntimeLifecyclePublishIsIdempotentByEventID(t *testing.T) {
	store := NewRuntimeLifecycleAt(t.TempDir(), filepath.Join(t.TempDir(), "runtime"))
	sink := &recordingLifecycleSink{store: store}
	store.Subscribe(sink)
	event := RuntimeLifecycleEvent{
		ID: "schedule-delivery-1", Type: LifecycleCronFire,
		EntityID: "cron-1", ToolName: "CronCreate",
	}
	if err := store.Publish(context.Background(), event); err != nil {
		t.Fatalf("first Publish: %v", err)
	}
	if err := store.Publish(context.Background(), event); err != nil {
		t.Fatalf("duplicate Publish: %v", err)
	}
	events, err := store.Events()
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 1 || len(sink.events) != 1 {
		t.Fatalf("duplicate event persisted or dispatched: events=%d sink=%d", len(events), len(sink.events))
	}
}
