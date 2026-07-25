package agent

import (
	"context"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"testing"

	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/registry"
)

func TestAgentProgressEmitterStampsRunIdentityAndSequence(t *testing.T) {
	emitter := newAgentProgressEmitter("agent-1", "explore")
	emitter.ConfigureRun("run-2", 2, "batch-7")
	var observed []agentcontract.ProgressEvent
	emitter.SetObserver(func(event agentcontract.ProgressEvent) {
		observed = append(observed, event)
	})

	if !emitter.EmitPhase(agentcontract.ProgressStart, 0, "") {
		t.Fatal("start event was not delivered")
	}
	if !emitter.EmitPhase(agentcontract.ProgressRunning, 1, "Read") {
		t.Fatal("running event was not delivered")
	}
	if !emitter.EmitPhase(agentcontract.ProgressAssistant, 2, "") {
		t.Fatal("assistant event was not delivered")
	}
	emitter.Finish(agentcontract.ProgressCompleted, "done")

	if len(observed) != 4 {
		t.Fatalf("observer events=%d, want 4", len(observed))
	}
	for index, event := range observed {
		if event.RunID != "run-2" || event.Attempt != 2 || event.BatchID != "batch-7" {
			t.Fatalf("event %d identity=%+v", index, event)
		}
		if event.SourceSequence != uint64(index+1) {
			t.Fatalf("event %d source sequence=%d", index, event.SourceSequence)
		}
	}
	for index, event := range observed {
		if event.DroppedCount != 0 {
			t.Fatalf("observer event %d reported a synthetic drop: %+v", index, event)
		}
	}
}

func TestRetainedAgentPersistsDistinctRunHistory(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	provider := &retainedSessionTestProvider{}
	agentID := "history-agent"
	queryLoop := loop.New(provider, registry.New(), loop.Config{
		Model: provider.ModelID(), MaxTokens: 1024, SessionID: agentID,
	})
	session, _, err := manager.RegisterAgentSession(
		agentID, "helper", "initial", "history test",
		agentcontract.Input{Prompt: "initial", Description: "history test"},
		queryLoop,
		agentcontract.SessionMetadata{AgentType: "general-purpose", Model: provider.ModelID(), CWD: root},
		nil, newAgentProgressEmitter(agentID, "general-purpose"), context.Background(),
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := session.runSync(context.Background(), "round one"); err != nil {
		t.Fatalf("round one: %v", err)
	}
	first, ok := manager.Snapshot(agentID)
	if !ok || first.Attempt != 1 || len(first.Runs) != 1 || first.CurrentRunID == "" {
		t.Fatalf("first snapshot=%+v ok=%v", first, ok)
	}
	if first.Runs[0].Status != "completed" || first.Runs[0].FinishedAt == nil {
		t.Fatalf("first run not terminal: %+v", first.Runs[0])
	}

	if _, err := session.runSync(context.Background(), "round two"); err != nil {
		t.Fatalf("round two: %v", err)
	}
	second, ok := manager.Snapshot(agentID)
	if !ok || second.Attempt != 2 || len(second.Runs) != 2 {
		t.Fatalf("second snapshot=%+v ok=%v", second, ok)
	}
	if second.CurrentRunID == first.CurrentRunID {
		t.Fatalf("run id reused across attempts: %q", second.CurrentRunID)
	}
	if second.Runs[0].RunID != first.CurrentRunID || second.Runs[1].RunID != second.CurrentRunID {
		t.Fatalf("run history order=%+v", second.Runs)
	}
	if second.LatestProgress == nil || second.LatestProgress.RunID != second.CurrentRunID || second.LatestProgress.Phase != agentcontract.ProgressCompleted {
		t.Fatalf("latest progress=%+v", second.LatestProgress)
	}

	persisted, ok := manager.store.Get(agentID)
	if !ok || persisted.Attempt != 2 || len(persisted.Runs) != 2 {
		t.Fatalf("persisted run history=%+v ok=%v", persisted, ok)
	}
}
