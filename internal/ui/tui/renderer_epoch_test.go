package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/types"
)

func TestRendererEpochFenceRunsInsideQueuedClosure(t *testing.T) {
	state := NewAppState()
	state.SessionEpoch.Set(1)
	var queued func()
	renderer := &TuiRenderer{
		state: state,
		enqueue: func(fn func()) bool {
			queued = fn
			return true
		},
	}
	renderer.TextAtEpoch(1, "stale text")
	if queued == nil {
		t.Fatal("text event was not queued")
	}
	state.SessionEpoch.Set(2)
	queued()
	if messages := state.Messages.Get(); len(messages) != 0 {
		t.Fatalf("stale queued text crossed the epoch boundary: %+v", messages)
	}

	renderer.TextAtEpoch(2, "current text")
	queued()
	if messages := state.Messages.Get(); len(messages) != 1 || messages[0].Text != "current text" {
		t.Fatalf("current queued text was not committed: %+v", messages)
	}
}

func TestRendererCoalescesThinkingDeltasWithinOneTurn(t *testing.T) {
	state := NewAppState()
	state.SessionEpoch.Set(1)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	renderer.SetRenderTurn(1)
	for _, delta := range []string{"The", " user", " said", " hello."} {
		renderer.ThinkingAtEpoch(1, delta)
	}
	messages := state.Messages.Get()
	if len(messages) != 1 || messages[0].Kind != MsgAssistantThinking || messages[0].Text != "The user said hello." || messages[0].TurnCount != 1 {
		t.Fatalf("thinking deltas were not coalesced: %+v", messages)
	}

	renderer.SetRenderTurn(2)
	renderer.ThinkingAtEpoch(1, "A separate turn")
	messages = state.Messages.Get()
	if len(messages) != 2 || messages[1].Text != "A separate turn" || messages[1].TurnCount != 2 {
		t.Fatalf("different turns were merged: %+v", messages)
	}
}

func TestRendererDropsOldContextGenerationWithinSameSessionEpoch(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(3)
	state.ContextGeneration.Set(8)
	state.ContextGenerationPersisted.Set(true)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	stale := presentation.ToolEventContext{SessionID: "session", SessionEpoch: 3, ContextGeneration: 7, ContextGenerationPersisted: true, TurnID: "old-turn"}
	renderer.RenderToolCall(stale, types.ToolUseBlock{ID: "old-tool", Name: "Read", Input: map[string]any{"file_path": "old"}})
	if got := state.Observations.Snapshot(); len(got) != 0 {
		t.Fatalf("old context generation entered current projection: %+v", got)
	}

	current := stale
	current.ContextGeneration = 8
	current.TurnID = "current-turn"
	renderer.RenderToolCall(current, types.ToolUseBlock{ID: "current-tool", Name: "Read", Input: map[string]any{"file_path": "current"}})
	if got := state.Observations.Snapshot(); len(got) != 1 || got[0].ToolUseID != "current-tool" {
		t.Fatalf("current context generation was not admitted: %+v", got)
	}
}

func TestRendererRejectsUnpinnedGenerationAgainstPersistedGeneration(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(3)
	state.ContextGeneration.Set(8)
	state.ContextGenerationPersisted.Set(true)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	unpinned := presentation.ToolEventContext{SessionID: "session", SessionEpoch: 3, ContextGeneration: 0, ContextGenerationPersisted: false, TurnID: "unpinned-turn"}
	renderer.RenderToolCall(unpinned, types.ToolUseBlock{ID: "unpinned-tool", Name: "Read"})
	if got := state.Observations.Snapshot(); len(got) != 0 {
		t.Fatalf("unpinned generation entered persisted projection: %+v", got)
	}
}

func TestRendererContextGenerationCommitIsOrderedReducerBarrier(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(5)
	state.ContextGeneration.Set(8)
	state.ContextGenerationPersisted.Set(true)
	queue := make(chan func(), 8)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { queue <- fn; return true }}
	base := presentation.ToolEventContext{SessionID: "session", SessionEpoch: 5, ContextGeneration: 8, ContextGenerationPersisted: true}

	renderer.TextAtContext(base, "old-generation text")
	committed := make(chan bool, 1)
	go func() { committed <- renderer.CommitContextGeneration(base, 9, true) }()

	(<-queue)()
	if got := state.Messages.Get(); len(got) != 1 || got[0].Text != "old-generation text" {
		t.Fatalf("pre-commit reducer event = %+v", got)
	}
	(<-queue)()
	if !<-committed || state.ContextGeneration.Get() != 9 {
		t.Fatalf("generation barrier did not commit: generation=%d", state.ContextGeneration.Get())
	}

	renderer.TextAtContext(base, "late stale text")
	(<-queue)()
	if got := state.Messages.Get(); len(got) != 1 {
		t.Fatalf("stale queued event crossed committed generation: %+v", got)
	}
	next := base
	next.ContextGeneration = 9
	renderer.TextAtContext(next, "new-generation text")
	(<-queue)()
	if got := state.Messages.Get(); len(got) != 1 || !strings.Contains(got[0].Text, "new-generation text") || strings.Contains(got[0].Text, "late stale text") {
		t.Fatalf("new generation reducer event = %+v", got)
	}
}

func TestQueuedTextThinkingUsageBriefAndCompactionRecheckGeneration(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(6)
	state.ContextGeneration.Set(10)
	state.ContextGenerationPersisted.Set(true)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 6})
	queue := make(chan func(), 8)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { queue <- fn; return true }}
	ctx := presentation.ToolEventContext{
		SessionID: "session", SessionEpoch: 6, ContextGeneration: 10, ContextGenerationPersisted: true,
		TurnID: "turn-old", ActorID: "assistant", WorkUnitID: "context",
	}

	renderer.TextAtContext(ctx, "stale text")
	renderer.ThinkingAtContext(ctx, "stale thinking")
	renderer.UsageAtContext(ctx, &types.Usage{InputTokens: 44})
	renderer.RenderSendUserMessageEvent(ctx, interaction.SendUserMessageOutput{Message: "stale brief"}, presentation.SendUserMessageRenderOptions{})
	renderer.CompactionBoundaryAtEpoch(ctx.SessionEpoch, ctx, stream.CompactBoundaryEvent{Trigger: "manual", PreCompactTokenCount: 100, PostCompactTokenCount: 20})

	if !state.PublishContextGeneration("session", 6, 11) {
		t.Fatal("failed to advance test generation")
	}
	for index := 0; index < 5; index++ {
		(<-queue)()
	}
	if got := state.Messages.Get(); len(got) != 0 {
		t.Fatalf("stale text/thinking/brief entered projection: %+v", got)
	}
	if usage := state.ActiveSessionUsage(); usage.InputTokens != 0 || usage.HasCompacted {
		t.Fatalf("stale usage/compaction mutated state: %+v", usage)
	}
	if activities := state.ActivitySnapshot().Activities; len(activities) != 0 {
		t.Fatalf("stale compaction activity entered projection: %+v", activities)
	}
}

func TestCompactionBoundaryRetainsSearchableExportableDisclosureEvidence(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(2)
	state.AccumulateSessionUsage(&types.Usage{InputTokens: 1200, CacheReadInputTokens: 900})
	state.UsedTokens.Set(160_000)
	state.MaxTokens.Set(200_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 2})
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	ctx := presentation.ToolEventContext{SessionID: "session", SessionEpoch: 2, TurnID: "turn-3", WorkUnitID: "context", ActorID: "assistant", ActorType: "runtime", ProjectRoot: "/workspace"}
	renderer.CompactionBoundaryAtEpoch(2, ctx, stream.CompactBoundaryEvent{
		Trigger: "reactive", PreCompactTokenCount: 1200, PostCompactTokenCount: 300, TruePostCompactTokenCount: 280,
		PreviousTailIdentifier: "assistant:tail", PreCompactDiscoveredTools: []string{"Read", "Bash"},
		PreservedSegment: &stream.PreservedSegmentMetadata{StartIndex: 8, Count: 4, Anchor: "assistant:tail", Direction: "tail"},
		Summary:          "complete compact summary evidence", UserDisplayMessage: "hook display evidence",
	})
	usage := state.ActiveSessionUsage()
	if !usage.HasCompacted || usage.InputTokensAtCompact != 1200 || usage.CacheReadAtCompact != 900 {
		t.Fatalf("compaction usage baseline = %+v, want input/cache 1200/900", usage)
	}
	if usage.CompactionCount != 1 || usage.CompletedRoundInputTokens != 1200 || usage.LastInputTokens != 0 {
		t.Fatalf("compaction round endpoint = %+v, want count/completed/current 1/1200/0", usage)
	}
	if state.UsedTokens.Get() != 280 || state.MaxTokens.Get() != 200_000 || state.ContextMeasurement.Get() != presentation.ContextMeasurementLocalEstimate {
		t.Fatalf("compaction boundary did not publish the post-compact estimate: used=%d max=%d measurement=%q",
			state.UsedTokens.Get(), state.MaxTokens.Get(), state.ContextMeasurement.Get())
	}

	snapshot := state.Activities.Snapshot()
	if len(snapshot.Activities) != 1 {
		t.Fatalf("compaction activities = %+v, want one", snapshot.Activities)
	}
	activity := snapshot.Activities[0]
	if activity.State != ActivityCompleted || activity.Outcome != OutcomeSucceeded || activity.Control.JumpTarget == "" || len(activity.Control.DetailRefs) != 1 {
		t.Fatalf("compaction activity lacks completed evidence controls: %+v", activity)
	}
	if !containsActivityAction(activity.Actions, ActivityJump) || !containsActivityAction(activity.Actions, ActivityDetails) {
		t.Fatalf("compaction activity actions = %v, want jump+details", activity.Actions)
	}
	observation, ok := state.GetObservation(activity.Control.JumpTarget)
	if !ok || observation.Disclosure.Level != DisclosureSummary || !observation.Disclosure.HasMore || len(observation.ResultRefs) != 1 {
		t.Fatalf("compaction disclosure observation = %+v, ok=%v", observation, ok)
	}
	evidence, err := state.ObservationEvidence(observation.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"reactive", "1200", "280", "920", "assistant:tail", "Read", "start_index", "/workspace", "complete compact summary evidence", "hook display evidence"} {
		if !bytes.Contains(evidence, []byte(want)) {
			t.Fatalf("compaction evidence omitted %q: %s", want, evidence)
		}
	}
	prepared, err := state.PrepareTranscriptSearch("assistant:tail")
	if err != nil || prepared.count == 0 {
		t.Fatalf("compaction evidence search = count %d, err %v", prepared.count, err)
	}
	target := filepath.Join(t.TempDir(), "compaction.txt")
	observations, details, presentation := state.TranscriptResources()
	if err := NewTranscriptExporter(observations, details).WithPresentation(presentation).Export(target, TranscriptExportHumanReadable); err != nil {
		t.Fatal(err)
	}
	exported, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Context compacted", "discarded 920", "assistant:tail"} {
		if !strings.Contains(string(exported), want) {
			t.Fatalf("compaction export omitted %q: %s", want, exported)
		}
	}
}

func containsActivityAction(actions []ActivityAction, want ActivityAction) bool {
	for _, action := range actions {
		if action == want {
			return true
		}
	}
	return false
}
