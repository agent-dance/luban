package tools

import "testing"

func TestRetainedAgentDetachedModeProjectsAndPersists(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(manager.Shutdown)

	_, foreground, err := manager.RegisterAgentSession(
		"foreground-agent", "", "prompt", "foreground",
		AgentInput{Prompt: "prompt", Description: "foreground"}, nil, agentSessionMetadata{}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if foreground.Detached {
		t.Fatalf("foreground retained Agent projected as detached: %+v", foreground)
	}

	_, background, err := manager.RegisterAgentSession(
		"background-agent", "", "prompt", "background",
		AgentInput{Prompt: "prompt", Description: "background", RunInBackground: true}, nil, agentSessionMetadata{}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !background.Detached {
		t.Fatalf("explicit background Agent lost detached mode: %+v", background)
	}
	if record, ok := manager.store.Get("background-agent"); !ok || !record.Detached {
		t.Fatalf("detached mode was not persisted: ok=%v record=%+v", ok, record)
	}
}

func TestMarkAgentDetachedUpdatesSnapshotAndRecord(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(manager.Shutdown)
	_, _, err := manager.RegisterAgentSession(
		"auto-background-agent", "", "prompt", "auto background",
		AgentInput{Prompt: "prompt", Description: "auto background"}, nil, agentSessionMetadata{}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manager.MarkAgentDetached("auto-background-agent") {
		t.Fatal("MarkAgentDetached returned false")
	}
	if snapshot, ok := manager.Snapshot("auto-background-agent"); !ok || !snapshot.Detached {
		t.Fatalf("snapshot detached mode = ok=%v snapshot=%+v", ok, snapshot)
	}
	if record, ok := manager.store.Get("auto-background-agent"); !ok || !record.Detached {
		t.Fatalf("record detached mode = ok=%v record=%+v", ok, record)
	}
}
