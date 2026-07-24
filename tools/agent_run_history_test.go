package tools

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type retainedHistoryBarrierProvider struct {
	calls    atomic.Int32
	started  chan int
	releases []chan struct{}
}

func (p *retainedHistoryBarrierProvider) Name() string    { return "retained-history-barrier" }
func (p *retainedHistoryBarrierProvider) ModelID() string { return "retained-history-barrier-model" }

func (p *retainedHistoryBarrierProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	call := int(p.calls.Add(1))
	select {
	case p.started <- call:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if call <= 0 || call > len(p.releases) {
		return nil, context.Canceled
	}
	select {
	case <-p.releases[call-1]:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	stream := make(chan types.StreamEvent, 4)
	stream <- types.StreamEvent{
		Type:         types.EventContentBlockStart,
		Index:        0,
		ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
	}
	stream <- types.StreamEvent{
		Type:  types.EventContentBlockDelta,
		Index: 0,
		Delta: &types.ContentDelta{Type: "text_delta", Text: "round complete"},
	}
	stream <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
	stream <- types.StreamEvent{Type: types.EventMessageStop}
	close(stream)
	return stream, nil
}

func TestAgentProgressEmitterStampsRunIdentitySequenceAndDrops(t *testing.T) {
	emitter := NewAgentProgressEmitter("agent-1", "explore", 1)
	emitter.ConfigureRun("run-2", 2, "batch-7")
	var observed []AgentProgressEvent
	emitter.SetObserver(func(event AgentProgressEvent) {
		observed = append(observed, event)
	})

	if !emitter.EmitPhase(AgentPhaseStart, 0, "") {
		t.Fatal("start event was not delivered")
	}
	if !emitter.EmitPhase(AgentPhaseRunning, 1, "Read") {
		t.Fatal("running event was not delivered")
	}
	<-emitter.Channel()
	if !emitter.EmitPhase(AgentPhaseAssistant, 2, "") {
		t.Fatal("assistant event was not delivered")
	}
	emitter.Finish(AgentPhaseCompleted, "done")

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
	if observed[1].DroppedCount != 1 {
		t.Fatalf("second event dropped count=%d, want 1", observed[1].DroppedCount)
	}
	if observed[2].DroppedCount != observed[1].DroppedCount || observed[3].DroppedCount < observed[2].DroppedCount {
		t.Fatalf("terminal dropped count regressed: %+v", observed)
	}
}

func TestDecodeLegacyAgentRecordCreatesFirstRun(t *testing.T) {
	record, err := decodeRuntimeTaskRecord([]byte(`{
		"id":"legacy-agent",
		"type":"local_agent",
		"status":"completed",
		"prompt":"inspect",
		"started_at":"2026-07-15T01:00:00Z",
		"finished_at":"2026-07-15T01:00:01Z"
	}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if record.Attempt != 1 || record.CurrentRunID == "" || len(record.Runs) != 1 {
		t.Fatalf("legacy run migration=%+v", record)
	}
	if record.Runs[0].RunID != record.CurrentRunID || record.Runs[0].Attempt != 1 || record.Runs[0].Status != "completed" {
		t.Fatalf("legacy first run=%+v", record.Runs[0])
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal migrated record: %v", err)
	}
	var roundTrip RuntimeTaskRecord
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if len(roundTrip.Runs) != 1 || roundTrip.CurrentRunID == "" {
		t.Fatalf("round trip lost runs: %+v", roundTrip)
	}
}

func TestRetainedAgentPersistsDistinctRunHistory(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	provider := &retainedSessionTestProvider{}
	agentID := "history-agent"
	queryLoop := loop.New(provider, registry.New(), loop.Config{
		Model: provider.ModelID(), MaxTokens: 1024, SessionID: agentID,
	})
	session, _, err := manager.RegisterAgentSession(
		agentID, "helper", "initial", "history test",
		AgentInput{Prompt: "initial", Description: "history test"},
		queryLoop,
		agentSessionMetadata{AgentType: "general-purpose", Model: provider.ModelID(), CWD: root},
		nil, NewAgentProgressEmitter(agentID, "general-purpose", 16),
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
	if second.LatestProgress == nil || second.LatestProgress.RunID != second.CurrentRunID || second.LatestProgress.Phase != AgentPhaseCompleted {
		t.Fatalf("latest progress=%+v", second.LatestProgress)
	}

	persisted, ok := manager.store.Get(agentID)
	if !ok || persisted.Attempt != 2 || len(persisted.Runs) != 2 {
		t.Fatalf("persisted run history=%+v ok=%v", persisted, ok)
	}
}

func TestRetainedAgentQueueCommitsLifecycleBeforePublishingCompletion(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	provider := &retainedHistoryBarrierProvider{
		started:  make(chan int, 2),
		releases: []chan struct{}{make(chan struct{}), make(chan struct{})},
	}
	agentID := "history-barrier-agent"
	queryLoop := loop.New(provider, registry.New(), loop.Config{
		Model: provider.ModelID(), MaxTokens: 1024, SessionID: agentID,
	})
	session, _, err := manager.RegisterAgentSession(
		agentID, "helper", "initial", "history lifecycle test",
		AgentInput{Prompt: "initial", Description: "history lifecycle test"},
		queryLoop,
		agentSessionMetadata{AgentType: "general-purpose", Model: provider.ModelID(), CWD: root},
		nil, NewAgentProgressEmitter(agentID, "general-purpose", 16),
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	lifecycleEntered := make(chan struct{})
	releaseLifecycle := make(chan struct{})
	var firstLifecycle sync.Once
	var releaseLifecycleOnce sync.Once
	releaseTerminalLifecycle := func() {
		releaseLifecycleOnce.Do(func() { close(releaseLifecycle) })
	}
	t.Cleanup(releaseTerminalLifecycle)
	SetAsyncAgentNotificationSink(AsyncAgentNotificationSinkFunc(func(notification AsyncAgentNotification) {
		if notification.AgentID != agentID {
			return
		}
		firstLifecycle.Do(func() {
			close(lifecycleEntered)
			<-releaseLifecycle
		})
	}))
	t.Cleanup(func() { SetAsyncAgentNotificationSink(nil) })

	firstResponse := make(chan agentRunResponse, 1)
	if err := session.enqueue("round one", firstResponse); err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	assertBarrierCall(t, provider.started, 1)
	close(provider.releases[0])
	select {
	case <-lifecycleEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("first run did not enter terminal lifecycle")
	}
	select {
	case result := <-firstResponse:
		t.Fatalf("first response escaped before terminal lifecycle committed: %+v", result)
	default:
	}

	secondResponse := make(chan agentRunResponse, 1)
	if err := session.enqueue("round two", secondResponse); err != nil {
		t.Fatalf("enqueue second: %v", err)
	}
	queued, ok := manager.Snapshot(agentID)
	if !ok || queued.QueuedPrompts != 1 {
		t.Fatalf("queued snapshot=%+v ok=%v", queued, ok)
	}

	releaseTerminalLifecycle()
	assertAgentRunResponse(t, firstResponse, "first")
	assertBarrierCall(t, provider.started, 2)
	close(provider.releases[1])
	assertAgentRunResponse(t, secondResponse, "second")

	snapshot, ok := manager.Snapshot(agentID)
	if !ok || snapshot.Attempt != 2 || snapshot.QueuedPrompts != 0 || len(snapshot.Runs) != 2 {
		t.Fatalf("final snapshot=%+v ok=%v", snapshot, ok)
	}
	if snapshot.Runs[0].RunID == snapshot.Runs[1].RunID ||
		snapshot.Runs[0].Attempt != 1 || snapshot.Runs[1].Attempt != 2 ||
		snapshot.Runs[0].Status != "completed" || snapshot.Runs[1].Status != "completed" {
		t.Fatalf("run history lost, duplicated, or reordered: %+v", snapshot.Runs)
	}
	persisted, ok := manager.store.Get(agentID)
	if !ok || persisted.Attempt != 2 || persisted.CurrentRunID != snapshot.CurrentRunID || len(persisted.Runs) != 2 {
		t.Fatalf("persisted run history=%+v ok=%v", persisted, ok)
	}
	if persisted.Runs[0].RunID != snapshot.Runs[0].RunID || persisted.Runs[1].RunID != snapshot.Runs[1].RunID {
		t.Fatalf("durable run history diverged: memory=%+v persisted=%+v", snapshot.Runs, persisted.Runs)
	}
}

func assertBarrierCall(t *testing.T, started <-chan int, want int) {
	t.Helper()
	select {
	case got := <-started:
		if got != want {
			t.Fatalf("provider call=%d, want %d", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("provider call %d did not start", want)
	}
}

func assertAgentRunResponse(t *testing.T, response <-chan agentRunResponse, label string) {
	t.Helper()
	select {
	case result := <-response:
		if result.err != nil {
			t.Fatalf("%s response: %v", label, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s response was not published", label)
	}
}
