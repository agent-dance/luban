package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	store := NewRuntimeLifecycle(root)

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

	resumed := NewRuntimeLifecycle(root)
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
	active, err = NewRuntimeLifecycle(root).ActiveState()
	if err != nil {
		t.Fatalf("ActiveState after completion: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("completed lifecycle entries remained active: %#v", active)
	}
}

func TestRuntimeLifecycleReadsLegacyArrayAndCamelCase(t *testing.T) {
	root := t.TempDir()
	path := runtimeLifecyclePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`[
  {"id":"legacy-1","type":"tool_start","entityId":"tool-1","toolName":"Read","sessionId":"s-1","createdAt":"2026-01-02T03:04:05Z"}
]`)
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	events, err := NewRuntimeLifecycle(root).Events()
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 1 || events[0].EntityID != "tool-1" || events[0].ToolName != "Read" || events[0].SessionID != "s-1" {
		t.Fatalf("legacy lifecycle event not migrated: %#v", events)
	}
	if events[0].CreatedAt.IsZero() {
		t.Fatalf("legacy createdAt was not decoded: %#v", events[0])
	}
}

func TestRuntimeLifecyclePersistsBeforeUnifiedDispatch(t *testing.T) {
	root := t.TempDir()
	store := NewRuntimeLifecycle(root)
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

func TestRuntimeLifecycleCompatibilityEnvelopeRoundTrip(t *testing.T) {
	root := t.TempDir()
	path := runtimeLifecyclePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	legacyEnvelope := map[string]any{
		"version": 0,
		"lifecycleEvents": []map[string]any{{
			"id": "legacy-envelope", "type": "mcp_resources_changed", "entityId": "docs", "createdAt": created,
		}},
	}
	body, _ := json.Marshal(legacyEnvelope)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewRuntimeLifecycle(root)
	if err := store.Publish(context.Background(), RuntimeLifecycleEvent{Type: LifecycleToolStart, EntityID: "next"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Type != LifecycleMCPResourcesChanged || events[1].EntityID != "next" {
		t.Fatalf("compatibility envelope was stranded on write: %#v", events)
	}
}
