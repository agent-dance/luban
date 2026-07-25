package agent

import (
	"context"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type retainedSnapshotStartProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p *retainedSnapshotStartProvider) Name() string    { return "retained-snapshot-start" }
func (p *retainedSnapshotStartProvider) ModelID() string { return "retained-snapshot-start-model" }

func (p *retainedSnapshotStartProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	stream := make(chan types.StreamEvent, 4)
	go func() {
		defer close(stream)
		select {
		case <-ctx.Done():
			return
		case <-p.release:
		}
		stream <- types.StreamEvent{
			Type:         types.EventContentBlockStart,
			Index:        0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
		}
		stream <- types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "done"},
		}
		stream <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
		stream <- types.StreamEvent{Type: types.EventMessageStop}
	}()
	return stream, nil
}

func TestRetainedAgentFirstRunningSnapshotStartsCorrelated(t *testing.T) {
	const (
		agentID        = "snapshot-correlation-agent"
		parentSession  = "parent-session"
		parentTurn     = "parent-turn"
		parentWorkUnit = "parent-work"
		parentToolUse  = "toolu-agent"
	)

	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	updates, unsubscribe := manager.SubscribeSnapshots()
	defer unsubscribe()
	captured := make(chan agentcontract.TaskSnapshot, 64)
	stopCapture := make(chan struct{})
	defer close(stopCapture)
	go func() {
		for {
			select {
			case <-stopCapture:
				return
			case _, ok := <-updates:
				if !ok {
					return
				}
				if snapshot, found := manager.Snapshot(agentID); found {
					captured <- snapshot
				}
			}
		}
	}()

	childProvider := &retainedSnapshotStartProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	childLoop := loop.New(childProvider, registry.New(), loop.Config{
		Model:     childProvider.ModelID(),
		MaxTokens: 1024,
		SessionID: agentID,
	})
	progress := newAgentProgressEmitter(agentID, "explore")
	progress.ConfigureCorrelation(parentSession, parentTurn, parentWorkUnit, parentToolUse)
	terminalObserved := make(chan struct{})
	releaseTerminal := make(chan struct{})
	var terminalOnce sync.Once
	progress.AddObserver(func(event agentcontract.ProgressEvent) {
		if event.Phase != agentcontract.ProgressError {
			return
		}
		terminalOnce.Do(func() { close(terminalObserved) })
		<-releaseTerminal
	})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseTerminal) }) }
	defer release()

	session, initial, err := manager.RegisterAgentSession(
		agentID,
		"",
		"inspect",
		"inspect snapshot ordering",
		agentcontract.Input{Prompt: "inspect", Description: "inspect snapshot ordering", SubagentType: "explore", RunInBackground: true},
		childLoop,
		agentcontract.SessionMetadata{AgentType: "explore", Model: childProvider.ModelID()},
		nil,
		progress,
		context.Background(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Attempt != 0 || initial.CurrentRunID != "" || initial.LatestProgress != nil {
		t.Fatalf("registration snapshot must remain a non-run placeholder: %+v", initial)
	}
	select {
	case registered := <-captured:
		if registered.Attempt != 0 || registered.CurrentRunID != "" || registered.LatestProgress != nil {
			t.Fatalf("attempt-zero registration notification must be ignored as a UI run: %+v", registered)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not observe the attempt-zero registration notification")
	}
	// Make output setup fail after the first running record is persisted. The
	// terminal observer holds the worker before it can replace that record, so
	// the subscriber observes the exact first attempt snapshot without relying
	// on scheduler timing or an eventually-consistent final state.
	if err := os.Remove(initial.OutputPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(initial.OutputPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// The attempt-zero registration record is intentionally not a UI run. Only
	// snapshots with a real attempt are checked below for presentation safety.
	if err := session.enqueue("inspect", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-terminalObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("retained Agent did not reach the held output-open failure")
	}

	deadline := time.After(2 * time.Second)
	sawCorrelatedRunning := false
	for !sawCorrelatedRunning {
		select {
		case snapshot := <-captured:
			if snapshot.Attempt == 0 {
				continue
			}
			if snapshot.Attempt != 1 || snapshot.Status != "running" {
				continue
			}
			assertRetainedRunningSnapshotCorrelated(t, snapshot, agentID, parentToolUse)
			sawCorrelatedRunning = true
		case <-deadline:
			t.Fatal("did not observe the first correlated running snapshot")
		}
	}

	record, ok := manager.store.Get(agentID)
	if !ok {
		t.Fatal("running retained Agent record was not persisted")
	}
	assertRetainedRunningRecordCorrelated(t, record, agentID, parentToolUse)

	release()
}

func assertRetainedRunningSnapshotCorrelated(t *testing.T, snapshot agentcontract.TaskSnapshot, agentID, parentToolUseID string) {
	t.Helper()
	progress := snapshot.LatestProgress
	if snapshot.CurrentRunID == "" || progress == nil {
		t.Fatalf("attempt-%d running snapshot was published without start correlation: %+v", snapshot.Attempt, snapshot)
	}
	if progress.AgentID != agentID || progress.ParentToolUseID != parentToolUseID || progress.RunID != snapshot.CurrentRunID || progress.Phase != agentcontract.ProgressStart {
		t.Fatalf("first running snapshot correlation = %+v, snapshot run=%q", progress, snapshot.CurrentRunID)
	}
}

func assertRetainedRunningRecordCorrelated(t *testing.T, record runtimestore.RuntimeTaskRecord, agentID, parentToolUseID string) {
	t.Helper()
	progress := record.LatestProgress
	if record.Attempt != 1 || record.Status != "running" || record.CurrentRunID == "" || progress == nil {
		t.Fatalf("persisted first running record was not presentation-ready: %+v", record)
	}
	if progress.AgentID != agentID || progress.ParentToolUseID != parentToolUseID || progress.RunID != record.CurrentRunID || progress.Phase != agentcontract.ProgressStart {
		t.Fatalf("persisted first running correlation = %+v, record run=%q", progress, record.CurrentRunID)
	}
}
