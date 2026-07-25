package tui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestActivityStoreUpdatesOneStableActivityInPlace(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 7})
	base := ActivityEvent{
		ID:         "tool:tool-use-42",
		SessionID:  "session-a",
		Epoch:      7,
		TurnID:     "session-a:3",
		WorkUnitID: "turn-3",
		Actor:      ActivityActor{ID: "assistant", Type: "main"},
		Kind:       ActivityTool,
		Name:       "go test ./...",
		Phase:      ActivityPhaseVerifying,
	}

	started := base
	started.Sequence = 10
	started.Lifecycle = ActivityLifecycleRunning
	started.Progress = ActivityProgress{Message: "starting"}
	progress := base
	progress.Sequence = 20
	progress.Lifecycle = ActivityLifecycleRunning
	progress.Progress = ActivityProgress{Current: 47, Total: 100, Message: "running tests"}
	completed := base
	completed.Sequence = 30
	completed.Lifecycle = ActivityLifecycleCompleted
	completed.Progress = ActivityProgress{Current: 100, Total: 100, Message: "128 passed"}

	for _, event := range []ActivityEvent{started, progress, completed} {
		if err := store.Apply(event); err != nil {
			t.Fatalf("Apply(%s): %v", event.Progress.Message, err)
		}
	}

	snapshot := store.Snapshot()
	if len(snapshot.Activities) != 1 {
		t.Fatalf("activities = %d, want one stable row", len(snapshot.Activities))
	}
	got := snapshot.Activities[0]
	if got.ID != base.ID || got.State != ActivityCompleted || got.Sequence != 30 {
		t.Fatalf("activity identity/state = %#v", got)
	}
	if got.FirstSequence != 10 || got.LastSequence != 30 {
		t.Fatalf("activity timeline = %d..%d, want 10..30", got.FirstSequence, got.LastSequence)
	}
	if got.Progress.Current != 100 || got.Progress.Total != 100 || got.Progress.Message != "128 passed" {
		t.Fatalf("final progress = %#v", got.Progress)
	}
	if snapshot.Counts.Total != 1 || snapshot.Counts.Completed != 1 || snapshot.Counts.Running != 0 {
		t.Fatalf("counts count activities, not events: %#v", snapshot.Counts)
	}
}

func TestActivityStoreKeepsSameNameParallelActivitiesIndependent(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	events := []ActivityEvent{
		activityTestEvent("tool:read-1", "Read", "worker-a", "work-a", 1, ActivityRunning),
		activityTestEvent("tool:read-2", "Read", "worker-b", "work-b", 1, ActivityRunning),
	}
	for _, event := range events {
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := store.Snapshot()
	if len(snapshot.Activities) != 2 {
		t.Fatalf("same-name parallel activities were merged: %#v", snapshot.Activities)
	}
	byID := activityTestIndex(snapshot.Activities)
	if byID["tool:read-1"].Actor.ID != "worker-a" || byID["tool:read-2"].Actor.ID != "worker-b" {
		t.Fatalf("actor attribution crossed parallel rows: %#v", snapshot.Activities)
	}
	if byID["tool:read-1"].WorkUnitID != "work-a" || byID["tool:read-2"].WorkUnitID != "work-b" {
		t.Fatalf("work-unit attribution crossed parallel rows: %#v", snapshot.Activities)
	}
}

func TestActivityStorePreservesCorrelationScopeAndKinds(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 9})
	events := []ActivityEvent{
		activityTestEvent("agent:researcher-1", "researcher", "researcher-1", "agent-work", 1, ActivityRunning),
		activityTestEvent("background:task-27", "lint", "assistant", "task-27", 2, ActivityRunning),
		activityTestEvent("mcp:tool-use-91", "github.search", "assistant", "turn-5", 3, ActivityRunning),
	}
	events[0].Kind = ActivityAgent
	events[0].Actor.Type = "subagent"
	events[1].Kind = ActivityBackground
	events[1].Actor.Type = "main"
	events[2].Kind = ActivityMCP
	events[2].Actor.Type = "main"
	for i := range events {
		events[i].TurnID = "session-a:5"
		events[i].Epoch = 9
		if err := store.Apply(events[i]); err != nil {
			t.Fatal(err)
		}
	}

	byID := activityTestIndex(store.Snapshot().Activities)
	for _, event := range events {
		got := byID[event.ID]
		if got.SessionID != event.SessionID || got.Epoch != event.Epoch || got.TurnID != event.TurnID {
			t.Fatalf("scope lost for %s: %#v", event.ID, got)
		}
		if got.Actor != event.Actor || got.WorkUnitID != event.WorkUnitID || got.Kind != event.Kind {
			t.Fatalf("correlation lost for %s: %#v", event.ID, got)
		}
	}
}

func TestActivityStoreSnapshotAndCountsAreDeterministic(t *testing.T) {
	events := []ActivityEvent{
		activityTestEvent("tool:z", "Bash", "assistant", "verify", 10, ActivityRunning),
		activityTestEvent("agent:a", "reviewer", "reviewer", "review", 20, ActivityNeedsInput),
		activityTestEvent("mcp:m", "docs.read", "assistant", "research", 30, ActivityFailed),
		activityTestEvent("background:b", "build", "assistant", "build", 40, ActivityCompleted),
	}
	events[1].Kind = ActivityAgent
	events[2].Kind = ActivityMCP
	events[3].Kind = ActivityBackground

	forward := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	reverse := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	for _, event := range events {
		if err := forward.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	for i := len(events) - 1; i >= 0; i-- {
		if err := reverse.Apply(events[i]); err != nil {
			t.Fatal(err)
		}
	}

	gotForward := forward.Snapshot()
	gotReverse := reverse.Snapshot()
	if !reflect.DeepEqual(gotForward, gotReverse) {
		t.Fatalf("arrival order changed snapshot:\nforward=%#v\nreverse=%#v", gotForward, gotReverse)
	}
	wantCounts := ActivityCounts{Total: 4, Running: 1, Blocked: 1, NeedsInput: 1, Failed: 1, Completed: 1, Unread: 1}
	if gotForward.Counts != wantCounts {
		t.Fatalf("counts = %#v, want %#v", gotForward.Counts, wantCounts)
	}
	wantOrder := []string{"agent:a", "mcp:m", "tool:z", "background:b"}
	for i, want := range wantOrder {
		if gotForward.Activities[i].ID != want {
			t.Fatalf("deterministic actionability/outcome order[%d] = %q, want %q", i, gotForward.Activities[i].ID, want)
		}
	}
}

func TestActivitySortOrderPlacesOrdinaryCancellationLastButTimeoutWithFailures(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	events := []ActivityEvent{
		activityTestEvent("needs", "needs", "a", "", 1, ActivityNeedsInput),
		activityTestEvent("ready", "ready", "b", "", 2, ActivityReadyReview),
		activityTestEvent("failed", "failed", "c", "", 3, ActivityFailed),
		activityTestEvent("timed", "timed", "d", "", 4, ActivityCancelled),
		activityTestEvent("running", "running", "e", "", 5, ActivityRunning),
		activityTestEvent("completed", "completed", "f", "", 6, ActivityCompleted),
		activityTestEvent("cancelled", "cancelled", "g", "", 7, ActivityCancelled),
	}
	events[3].Outcome = OutcomeTimedOut
	events[6].Outcome = OutcomeCancelled
	for _, event := range events {
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	got := store.Snapshot().Activities
	want := []string{"needs", "ready", "failed", "timed", "running", "completed", "cancelled"}
	for index := range want {
		if got[index].ID != want[index] {
			t.Fatalf("order[%d]=%q, want %q; all=%+v", index, got[index].ID, want[index], got)
		}
	}
}

func TestActivityStoreRestoreReepochsRunsAndPreservesAcknowledgement(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session", Epoch: 9})
	runs := []Activity{
		{ActivityEvent: ActivityEvent{ID: "agent", RunID: "run-1", Attempt: 1, SessionID: "old", Epoch: 1, Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded,
			Attention: ActivityAttention{Kind: ActivityAttentionReadyForReview, Unread: false}, Sequence: 3}, Actionability: ActivityActionTransition, OccurrenceCount: 1, FirstSequence: 2, LastSequence: 3, Acknowledged: true},
		{ActivityEvent: ActivityEvent{ID: "agent", RunID: "run-2", Attempt: 2, SessionID: "old", Epoch: 1, Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning, Sequence: 5}, Actionability: ActivityActionProgress, OccurrenceCount: 1, FirstSequence: 4, LastSequence: 5},
	}
	sequence, err := store.Restore(runs)
	if err != nil {
		t.Fatal(err)
	}
	if sequence != 5 {
		t.Fatalf("restored sequence=%d", sequence)
	}
	latest := store.Snapshot().Activities
	if len(latest) != 1 || latest[0].RunID != "run-2" || latest[0].Epoch != 9 || latest[0].SessionID != "session" {
		t.Fatalf("latest restored projection=%+v", latest)
	}
	old, ok := store.GetRun("agent", "run-1")
	if !ok || !old.Acknowledged || old.Attention.Unread || old.Epoch != 9 {
		t.Fatalf("old run acknowledgement was not restored: %+v ok=%v", old, ok)
	}
}

func TestActivityStoreRestoreRejectsIncompletePersistedRuns(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session", Epoch: 2})
	_, err := store.Restore([]Activity{{ActivityEvent: ActivityEvent{
		ID: "agent", SessionID: "old", Epoch: 1, Lifecycle: ActivityLifecycleRunning,
	}}})
	if err == nil {
		t.Fatal("incomplete persisted run was accepted")
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyTUISessionViewInvalidCheckpoint {
		t.Fatalf("restore error = %v (%+v, %v)", err, info, ok)
	}
}

func TestIncidentActivityFixtureRestoresHistoricalCounts(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "incident", Epoch: 2})
	activities := make([]Activity, 0, 247)
	sequence := uint64(1)
	appendRuns := func(prefix string, count int, lifecycle ActivityLifecycle, outcome ObservationOutcome) {
		for index := 0; index < count; index++ {
			activities = append(activities, Activity{ActivityEvent: ActivityEvent{
				ID: fmt.Sprintf("%s:%03d", prefix, index), SessionID: "old", Epoch: 1,
				Lifecycle: lifecycle, Outcome: outcome, Sequence: sequence,
			}, OccurrenceCount: 1, FirstSequence: sequence, LastSequence: sequence})
			sequence++
		}
	}
	appendRuns("completed", 192, ActivityLifecycleCompleted, OutcomeSucceeded)
	appendRuns("failed", 50, ActivityLifecycleFailed, OutcomeFailed)
	appendRuns("stale", 5, ActivityLifecycleRunning, OutcomeRunning)
	if _, err := store.Restore(activities); err != nil {
		t.Fatal(err)
	}
	store.ReconcileNonTerminal(nil)
	snapshot := store.Snapshot()
	if snapshot.Counts.Total != 247 || snapshot.Counts.Completed != 192 || snapshot.Counts.Failed != 50 || snapshot.Counts.Orphan != 5 || snapshot.Counts.Running != 0 {
		t.Fatalf("incident lifecycle counts = %+v", snapshot.Counts)
	}
}

func TestActivityStoreSnapshotKeepsWorkUnitAndActorGroupsContiguous(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	events := []ActivityEvent{
		activityTestEvent("work-a:needs", "needs", "actor-a", "work-a", 1, ActivityNeedsInput),
		activityTestEvent("work-b:failed", "failed", "actor-b", "work-b", 2, ActivityFailed),
		activityTestEvent("work-a:completed", "completed", "actor-a", "work-a", 3, ActivityCompleted),
	}
	for _, event := range events {
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	got := store.Snapshot().Activities
	want := []string{"work-a:needs", "work-a:completed", "work-b:failed"}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("grouped snapshot[%d] = %q, want %q; all=%+v", i, got[i].ID, want[i], got)
		}
	}
}

func TestActivityStoreDerivesActionabilityAndAvailableActions(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	running := activityTestEvent("background:task-4", "integration tests", "assistant", "task-4", 1, ActivityRunning)
	running.Kind = ActivityBackground
	running.Control = ActivityControl{
		Cancelable: true,
		JumpTarget: "work:task-4",
		DetailRefs: []DetailRef{{Source: "memory", Key: "task-4/output", Size: 4096}},
	}
	needsInput := activityTestEvent("agent:reviewer", "reviewer", "reviewer", "agent-review", 1, ActivityNeedsInput)
	needsInput.Kind = ActivityAgent
	needsInput.Control.DetailRefs = []DetailRef{{Source: "memory", Key: "agent/private-run", Size: 4096}}
	completed := activityTestEvent("tool:test", "go test", "assistant", "verify", 1, ActivityCompleted)

	for _, event := range []ActivityEvent{running, needsInput, completed} {
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}

	byID := activityTestIndex(store.Snapshot().Activities)
	if byID[running.ID].Actionability != ActivityActionProgress {
		t.Fatalf("running actionability = %v", byID[running.ID].Actionability)
	}
	for _, action := range []ActivityAction{ActivityCancel, ActivityJump, ActivityDetails} {
		if !activityTestHasAction(byID[running.ID].Actions, action) {
			t.Errorf("running activity missing %v action: %#v", action, byID[running.ID].Actions)
		}
	}
	if byID[needsInput.ID].Actionability != ActivityActionDecision {
		t.Fatalf("needs-input actionability = %v", byID[needsInput.ID].Actionability)
	}
	if activityTestHasAction(byID[needsInput.ID].Actions, ActivityDetails) {
		t.Fatalf("Agent activity exposed complete run details: %#v", byID[needsInput.ID].Actions)
	}
	if byID[completed.ID].Actionability != ActivityActionTransition {
		t.Fatalf("completed actionability = %v", byID[completed.ID].Actionability)
	}
	if activityTestHasAction(byID[completed.ID].Actions, ActivityCancel) {
		t.Fatalf("terminal activity remained cancelable: %#v", byID[completed.ID].Actions)
	}
}

func TestActivityStoreSparseProgressUpdatePreservesActorAndCancelCapability(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	initial := activityTestEvent("background:task", "build", "agent-1", "work", 1, ActivityRunning)
	initial.Actor.Type = "reviewer"
	initial.Progress = ActivityProgress{Current: 1, Total: 10, Message: "building"}
	initial.Control.Cancelable = true
	if err := store.Apply(initial); err != nil {
		t.Fatal(err)
	}
	if err := store.Apply(ActivityEvent{
		ID: "background:task", SessionID: "session-a", Epoch: 1, Sequence: 2,
		Lifecycle: ActivityLifecycleRunning,
		Actor:     ActivityActor{Type: "reviewer"}, Progress: ActivityProgress{Current: 1, Total: 2},
	}); err != nil {
		t.Fatal(err)
	}
	activity, ok := store.Get("background:task")
	if !ok {
		t.Fatal("activity missing")
	}
	if activity.Actor.ID != "agent-1" || activity.Actor.Type != "reviewer" || !activity.Control.Cancelable || !activityTestHasAction(activity.Actions, ActivityCancel) {
		t.Fatalf("sparse update erased stable control/actor: %+v", activity)
	}
	if activity.Progress != (ActivityProgress{Current: 1, Total: 2, Message: "building"}) {
		t.Fatalf("sparse progress update erased stable fields: %+v", activity.Progress)
	}
	if activity.FirstSequence != 1 || activity.LastSequence != 2 {
		t.Fatalf("sparse progress update changed timeline: %d..%d", activity.FirstSequence, activity.LastSequence)
	}
}

func TestActivityStoreRejectsEventsWithoutLifecycle(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	err := store.Apply(ActivityEvent{
		ID: "tool:done", SessionID: "session-a", Epoch: 1, Sequence: 1,
		Outcome: OutcomeSucceeded,
	})
	if err == nil || !errors.Is(err, ErrActivityStateOutcomeMismatch) {
		t.Fatalf("missing lifecycle error = %v", err)
	}
}

func TestActivityStoreRejectsContradictoryTerminalLifecycleAndOutcome(t *testing.T) {
	tests := []struct {
		lifecycle ActivityLifecycle
		outcome   ObservationOutcome
	}{
		{ActivityLifecycleCompleted, OutcomeDenied},
		{ActivityLifecycleCompleted, OutcomeFailed},
		{ActivityLifecycleCompleted, OutcomeCancelled},
		{ActivityLifecycleFailed, OutcomeSucceeded},
		{ActivityLifecycleFailed, OutcomeTimedOut},
		{ActivityLifecycleCancelled, OutcomePartial},
		{ActivityLifecycleRunning, OutcomeSucceeded},
	}
	for index, test := range tests {
		store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
		err := store.Apply(ActivityEvent{
			ID: fmt.Sprintf("tool:contradiction-%d", index), SessionID: "session-a", Epoch: 1, Sequence: 1,
			Lifecycle: test.lifecycle, Outcome: test.outcome,
		})
		if err == nil || !errors.Is(err, ErrActivityStateOutcomeMismatch) || !strings.Contains(err.Error(), "state/outcome") {
			t.Errorf("Apply(%s, %s) error = %v, want ErrActivityStateOutcomeMismatch", test.lifecycle, test.outcome, err)
		}
		if snapshot := store.Snapshot(); len(snapshot.Activities) != 0 {
			t.Errorf("contradictory event mutated store: %+v", snapshot.Activities)
		}
	}
}

func TestActivityStoreAllowsAttentionStatesWithCompatibleOutcomes(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	for _, event := range []ActivityEvent{
		{ID: "decision", SessionID: "session-a", Epoch: 1, Sequence: 1, Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning,
			Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning, Unread: true}},
		{ID: "review", SessionID: "session-a", Epoch: 1, Sequence: 2, Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded,
			Attention: ActivityAttention{Kind: ActivityAttentionReadyForReview, Severity: ActivityAttentionSeverityInfo, Unread: true}},
	} {
		if err := store.Apply(event); err != nil {
			t.Errorf("Apply(%s, %s): %v", event.Lifecycle, event.Outcome, err)
		}
	}
	snapshot := store.Snapshot()
	if snapshot.Counts.NeedsInput != 1 || snapshot.Counts.ReadyReview != 1 || snapshot.Counts.Blocked != 1 || snapshot.Counts.Completed != 1 {
		t.Fatalf("attention/lifecycle counts = %+v", snapshot.Counts)
	}
	byID := activityTestIndex(snapshot.Activities)
	if got := byID["decision"]; got.Lifecycle != ActivityLifecycleBlocked || got.Attention.Kind != ActivityAttentionNeedsInput || !got.Attention.Unread {
		t.Fatalf("needs-input projection = %+v", got)
	}
	if got := byID["review"]; got.Lifecycle != ActivityLifecycleCompleted || got.Attention.Kind != ActivityAttentionReadyForReview || !got.Attention.Unread {
		t.Fatalf("ready-review projection = %+v", got)
	}
}

func TestPendingDecisionProjectsCorrelatedWorkAsBlockedAndRestoresSourceLifecycle(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	apply := func(event ActivityEvent) {
		t.Helper()
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	tool := ActivityEvent{
		ID: "tool:write", SessionID: "session-a", Epoch: 1, Sequence: 1, TurnID: "turn-1", WorkUnitID: "work-1",
		Actor: ActivityActor{ID: "agent-1", Type: "agent"}, Kind: ActivityTool, Name: "Write",
		Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
	}
	agent := ActivityEvent{
		ID: "background:agent-1", RunID: "run-1", Attempt: 1,
		SessionID: "session-a", Epoch: 1, Sequence: 2, TurnID: "turn-1", WorkUnitID: "work-1",
		Actor: ActivityActor{ID: "agent-1", Type: "agent"}, Kind: ActivityAgent, Name: "worker",
		Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
	}
	unrelated := ActivityEvent{
		ID: "tool:read", SessionID: "session-a", Epoch: 1, Sequence: 3, TurnID: "turn-1", WorkUnitID: "work-2",
		Actor: ActivityActor{ID: "agent-2", Type: "agent"}, Kind: ActivityTool, Name: "Read",
		Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
	}
	decision := ActivityEvent{
		ID: "decision:permission-1", SessionID: "session-a", Epoch: 1, Sequence: 4, TurnID: "turn-1", WorkUnitID: "work-1",
		Actor: ActivityActor{ID: "agent-1", Type: "agent"}, Kind: ActivityDecision, Name: "Write",
		Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning,
		Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning, Unread: true, DecisionID: "permission-1"},
	}
	for _, event := range []ActivityEvent{tool, agent, unrelated, decision} {
		apply(event)
	}

	snapshot := store.Snapshot()
	byID := activityTestIndex(snapshot.Activities)
	if got := byID[tool.ID]; got.Lifecycle != ActivityLifecycleBlocked || got.State != ActivityBlocked || got.Outcome != OutcomeUnknown || got.Attention.Kind == ActivityAttentionNeedsInput {
		t.Fatalf("permission-wait tool projection = %+v, want non-actionable blocked", got)
	}
	if got := byID[agent.ID]; got.Lifecycle != ActivityLifecycleBlocked || got.State != ActivityNeedsInput || got.Outcome != OutcomeUnknown || got.Attention.DecisionID != "permission-1" {
		t.Fatalf("permission-wait agent projection = %+v, want actionable blocked", got)
	}
	if got := byID[decision.ID]; got.State != ActivityNeedsInput || got.Outcome != OutcomeUnknown {
		t.Fatalf("pending decision still claimed a running outcome: %+v", got)
	}
	if got := byID[unrelated.ID]; got.Lifecycle != ActivityLifecycleRunning || got.State != ActivityRunning {
		t.Fatalf("parallel activity was incorrectly blocked: %+v", got)
	}
	if snapshot.Counts.Running != 1 || snapshot.Counts.Blocked != 3 || snapshot.Counts.NeedsInput != 2 {
		t.Fatalf("permission-wait counts = %+v", snapshot.Counts)
	}
	if raw, ok := store.GetRun(tool.ID, ""); !ok || raw.Lifecycle != ActivityLifecycleRunning || raw.Outcome != OutcomeRunning {
		t.Fatalf("snapshot projection mutated durable tool lifecycle: %+v ok=%t", raw, ok)
	}

	decision.Sequence = 5
	decision.Lifecycle = ActivityLifecycleCompleted
	decision.Outcome = OutcomeSucceeded
	decision.Attention = ActivityAttention{}
	apply(decision)
	snapshot = store.Snapshot()
	byID = activityTestIndex(snapshot.Activities)
	if got := byID[tool.ID]; got.Lifecycle != ActivityLifecycleRunning || got.State != ActivityRunning {
		t.Fatalf("resolved permission did not restore tool lifecycle: %+v", got)
	}
	if got := byID[agent.ID]; got.Lifecycle != ActivityLifecycleRunning || got.State != ActivityRunning {
		t.Fatalf("resolved permission did not restore agent lifecycle: %+v", got)
	}
}

func TestPendingDecisionNeverMasksCorrelatedTerminalReceipt(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	for _, event := range []ActivityEvent{
		{
			ID: "tool:write", SessionID: "session-a", Epoch: 1, Sequence: 1, TurnID: "turn-1", WorkUnitID: "work-1",
			Actor: ActivityActor{ID: "agent-1"}, Kind: ActivityTool, Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded,
		},
		{
			ID: "decision:permission-1", SessionID: "session-a", Epoch: 1, Sequence: 2, TurnID: "turn-1", WorkUnitID: "work-1",
			Actor: ActivityActor{ID: "agent-1"}, Kind: ActivityDecision, Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning,
			Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Unread: true, DecisionID: "permission-1"},
		},
	} {
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	tool := activityTestIndex(store.Snapshot().Activities)["tool:write"]
	if tool.Lifecycle != ActivityLifecycleCompleted || tool.Outcome != OutcomeSucceeded {
		t.Fatalf("pending decision masked terminal receipt: %+v", tool)
	}
}

func TestActivityStoreSeparatesExplicitAttentionFromLifecycle(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	events := []ActivityEvent{
		{
			ID: "agent:writer", RunID: "run-writer-1", Attempt: 1,
			SessionID: "session-a", Epoch: 1, Sequence: 1,
			Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning,
			Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning, Unread: true, DecisionID: "decision-1"},
		},
		{
			ID: "agent:reviewer", RunID: "run-reviewer-1", Attempt: 1,
			SessionID: "session-a", Epoch: 1, Sequence: 2,
			Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded,
			Attention: ActivityAttention{Kind: ActivityAttentionReadyForReview, Severity: ActivityAttentionSeverityInfo, Unread: true},
		},
	}
	for _, event := range events {
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := store.Snapshot()
	if snapshot.Counts.Blocked != 1 || snapshot.Counts.Completed != 1 || snapshot.Counts.NeedsInput != 1 || snapshot.Counts.ReadyReview != 1 {
		t.Fatalf("split lifecycle/attention counts = %+v", snapshot.Counts)
	}
	writer, ok := store.GetRun("agent:writer", "run-writer-1")
	if !ok || writer.State != ActivityNeedsInput || writer.Lifecycle != ActivityLifecycleBlocked || writer.Actionability != ActivityActionDecision {
		t.Fatalf("writer projection = %+v, ok=%t", writer, ok)
	}
	reviewer, ok := store.GetRun("agent:reviewer", "run-reviewer-1")
	if !ok || reviewer.State != ActivityReadyReview || reviewer.Lifecycle != ActivityLifecycleCompleted {
		t.Fatalf("reviewer projection = %+v, ok=%t", reviewer, ok)
	}
	if err := store.Apply(ActivityEvent{
		ID: "agent:writer", RunID: "run-writer-1", Attempt: 1,
		SessionID: "session-a", Epoch: 1, Sequence: 3,
		Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
		Attention: ActivityAttention{Kind: ActivityAttentionNone},
	}); err != nil {
		t.Fatal(err)
	}
	writer, _ = store.GetRun("agent:writer", "run-writer-1")
	if writer.State != ActivityRunning || writer.Lifecycle != ActivityLifecycleRunning || writer.Attention.Kind != ActivityAttentionNone || writer.Actionability != ActivityActionProgress {
		t.Fatalf("resolved writer projection = %+v", writer)
	}
	if counts := store.Snapshot().Counts; counts.NeedsInput != 0 || counts.Blocked != 0 || counts.Running != 1 || counts.ReadyReview != 1 {
		t.Fatalf("resolved attention counts = %+v", counts)
	}
}

func TestActivityStoreTerminalMonotonicityIsScopedToRun(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	run1 := ActivityEvent{
		ID: "agent:security", RunID: "run-1", Attempt: 1,
		SessionID: "session-a", Epoch: 1, Sequence: 1, SourceSequence: 20,
		Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded,
		Progress: ActivityProgress{Message: "first result"},
	}
	run2 := ActivityEvent{
		ID: "agent:security", RunID: "run-2", Attempt: 2,
		SessionID: "session-a", Epoch: 1, Sequence: 2, SourceSequence: 1,
		Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
		Progress: ActivityProgress{Message: "resumed"},
	}
	lateRun1 := run1
	lateRun1.Sequence = 3
	lateRun1.SourceSequence = 21
	lateRun1.Lifecycle = ActivityLifecycleRunning
	lateRun1.Outcome = OutcomeRunning
	lateRun1.Progress.Message = "late progress"
	for _, event := range []ActivityEvent{run1, run2, lateRun1} {
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := store.Snapshot()
	if snapshot.Counts.Total != 1 || snapshot.Counts.Completed != 0 || snapshot.Counts.Running != 1 || snapshot.Activities[0].RunID != run2.RunID {
		t.Fatalf("run-aware counts = %+v activities=%+v", snapshot.Counts, snapshot.Activities)
	}
	first, ok := store.GetRun(run1.ID, run1.RunID)
	if !ok || first.Lifecycle != ActivityLifecycleCompleted || first.Progress.Message != "first result" {
		t.Fatalf("run 1 regressed = %+v, ok=%t", first, ok)
	}
	current, ok := store.Get(run1.ID)
	if !ok || current.RunID != run2.RunID || current.Attempt != 2 || current.Lifecycle != ActivityLifecycleRunning {
		t.Fatalf("latest projection lookup = %+v, ok=%t", current, ok)
	}
}

func TestActivityStoreRejectsLateSourceSequenceWithinRun(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	current := ActivityEvent{
		ID: "agent:tester", RunID: "run-1", Attempt: 1,
		SessionID: "session-a", Epoch: 1, Sequence: 1, SourceSequence: 10,
		Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
		Progress: ActivityProgress{Current: 50, Total: 100, Message: "halfway"},
	}
	late := current
	late.Sequence = 2
	late.SourceSequence = 9
	late.Progress = ActivityProgress{Current: 10, Total: 100, Message: "late"}
	if err := store.Apply(current); err != nil {
		t.Fatal(err)
	}
	if err := store.Apply(late); err != nil {
		t.Fatal(err)
	}

	got, ok := store.GetRun(current.ID, current.RunID)
	if !ok || got.SourceSequence != 10 || got.Sequence != 1 || got.Progress.Message != "halfway" || got.Progress.Current != 50 {
		t.Fatalf("late source event changed projection = %+v, ok=%t", got, ok)
	}
}

func TestActivityStoreRunAwareOrderingIsStableAcrossArrivalOrder(t *testing.T) {
	events := []ActivityEvent{
		{ID: "agent:atlas", RunID: "atlas-1", Attempt: 1, BatchID: "auth-review", AgentPath: "main/atlas", SessionID: "session-a", Epoch: 1, Sequence: 10, Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded},
		{ID: "agent:boreal", RunID: "boreal-1", Attempt: 1, BatchID: "auth-review", AgentPath: "main/boreal", SessionID: "session-a", Epoch: 1, Sequence: 20, Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning},
		{ID: "agent:delta", RunID: "delta-1", Attempt: 1, BatchID: "auth-review", AgentPath: "main/delta", SessionID: "session-a", Epoch: 1, Sequence: 30, Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning, Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Unread: true}},
	}
	forward := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	reverse := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	for _, event := range events {
		if err := forward.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	for i := len(events) - 1; i >= 0; i-- {
		if err := reverse.Apply(events[i]); err != nil {
			t.Fatal(err)
		}
	}
	gotForward, gotReverse := forward.Snapshot(), reverse.Snapshot()
	if !reflect.DeepEqual(gotForward, gotReverse) {
		t.Fatalf("run-aware order changed with arrival order:\nforward=%+v\nreverse=%+v", gotForward, gotReverse)
	}
	want := []string{"delta-1", "boreal-1", "atlas-1"}
	for i, runID := range want {
		if gotForward.Activities[i].RunID != runID {
			t.Fatalf("order[%d] = %q, want %q; all=%+v", i, gotForward.Activities[i].RunID, runID, gotForward.Activities)
		}
	}
}

func TestActivityStoreKeepsPartialDeniedAndSuccessOutcomesIndependent(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	events := []ActivityEvent{
		activityTestEvent("tool:partial", "Read", "worker-a", "work-a", 1, ActivityFailed),
		activityTestEvent("tool:denied", "Write", "worker-b", "work-b", 2, ActivityFailed),
		activityTestEvent("tool:success", "Bash", "worker-c", "work-c", 3, ActivityCompleted),
	}
	events[0].Outcome = OutcomePartial
	events[1].Outcome = OutcomeDenied
	events[2].Outcome = OutcomeSucceeded
	for _, event := range events {
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := store.Snapshot()
	if snapshot.Counts.Total != 3 || snapshot.Counts.Partial != 1 || snapshot.Counts.Denied != 1 || snapshot.Counts.Failed != 0 || snapshot.Counts.Completed != 1 {
		t.Fatalf("mixed outcome counts = %+v", snapshot.Counts)
	}
	byID := activityTestIndex(snapshot.Activities)
	if byID["tool:partial"].Outcome != OutcomePartial || byID["tool:denied"].Outcome != OutcomeDenied || byID["tool:success"].Outcome != OutcomeSucceeded {
		t.Fatalf("mixed outcomes were merged or rewritten: %+v", snapshot.Activities)
	}
}

func TestActivityStoreRejectsBackgroundEventsForAnotherSessionOrEpoch(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "visible-session", Epoch: 12})
	wrongSession := activityTestEvent("background:old-session", "build", "assistant", "task-old", 1, ActivityCompleted)
	wrongSession.SessionID = "old-session"
	wrongSession.Epoch = 12
	staleEpoch := activityTestEvent("background:stale-epoch", "lint", "assistant", "task-stale", 2, ActivityFailed)
	staleEpoch.Epoch = 11

	for _, event := range []ActivityEvent{wrongSession, staleEpoch} {
		if err := store.Apply(event); !errors.Is(err, ErrActivityScopeMismatch) {
			t.Fatalf("Apply(%s) error = %v, want ErrActivityScopeMismatch", event.ID, err)
		}
	}
	if snapshot := store.Snapshot(); len(snapshot.Activities) != 0 || snapshot.Counts.Total != 0 {
		t.Fatalf("foreign background activity polluted visible session: %#v", snapshot)
	}
}

func TestActivityStoreTerminalStateDoesNotRegressOnOutOfOrderProgress(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	completed := activityTestEvent("tool:test", "go test", "assistant", "verify", 30, ActivityCompleted)
	completed.Progress = ActivityProgress{Current: 128, Total: 128, Message: "passed"}
	lateOlderProgress := activityTestEvent(completed.ID, completed.Name, "assistant", "verify", 20, ActivityRunning)
	lateOlderProgress.Progress = ActivityProgress{Current: 64, Total: 128, Message: "still running"}
	lateNewerProgress := activityTestEvent(completed.ID, completed.Name, "assistant", "verify", 40, ActivityRunning)
	lateNewerProgress.Progress = ActivityProgress{Current: 127, Total: 128, Message: "late transport delivery"}

	for _, event := range []ActivityEvent{completed, lateOlderProgress, lateNewerProgress} {
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	got := store.Snapshot().Activities[0]
	if got.State != ActivityCompleted || got.Sequence != 30 || got.Progress.Message != "passed" {
		t.Fatalf("terminal activity regressed after progress delivery: %#v", got)
	}
}

func TestActivityStoreAllowsTerminalLifecycleAtSameProducerSequence(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	running := activityTestEvent("background:agent", "agent", "agent", "work", 1, ActivityRunning)
	running.RunID = "run-1"
	running.Attempt = 1
	running.SourceSequence = 9
	terminal := running
	terminal.Sequence = 2
	terminal.Lifecycle = ActivityLifecycleCompleted
	terminal.Outcome = OutcomeSucceeded
	terminal.Progress.Message = "formal result retained"
	if err := store.Apply(running); err != nil {
		t.Fatal(err)
	}
	if err := store.Apply(terminal); err != nil {
		t.Fatal(err)
	}
	got, ok := store.GetRun(running.ID, running.RunID)
	if !ok || got.Lifecycle != ActivityLifecycleCompleted || got.Outcome != OutcomeSucceeded || got.Progress.Message != "formal result retained" {
		t.Fatalf("same-source terminal transition was dropped: %+v ok=%t", got, ok)
	}
}

func TestActivityStorePublicPathRetainsHundredThousandRunsWithoutQuadraticPruning(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session-a", Epoch: 1})
	const count = 100000
	started := time.Now()
	for index := 0; index < count; index++ {
		event := ActivityEvent{
			ID: fmt.Sprintf("activity-%06d", index), SessionID: "session-a", Epoch: 1,
			Kind: ActivityTool, Lifecycle: ActivityLifecycleCompleted,
			Outcome: OutcomeSucceeded, Sequence: uint64(index + 1),
		}
		if index == 0 {
			event.Lifecycle = ActivityLifecycleFailed
			event.Outcome = OutcomeFailed
			event.Attention = ActivityAttention{Kind: ActivityAttentionCritical, Severity: ActivityAttentionSeverityError, Unread: true}
		}
		if err := store.Apply(event); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := store.Snapshot()
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("100k public ActivityStore apply+snapshot took %s", elapsed)
	}
	if snapshot.Counts.Total != count || snapshot.Counts.Failed != 1 || len(store.RunHistory()) != count {
		t.Fatalf("100k retention counts=%+v history=%d", snapshot.Counts, len(store.RunHistory()))
	}
	failed, ok := store.Get("activity-000000")
	if !ok || failed.Acknowledged || !failed.Attention.Unread {
		t.Fatalf("unread terminal failure was pruned or acknowledged: %+v ok=%t", failed, ok)
	}
}

func TestActivityStoreMergesRepeatedFailuresWithOccurrenceTimeline(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	for _, sequence := range []uint64{10, 14, 18} {
		if err := store.Apply(ActivityEvent{ID: "work:failed", SessionID: "session", Epoch: 1, Lifecycle: ActivityLifecycleFailed, Sequence: sequence}); err != nil {
			t.Fatal(err)
		}
	}
	activity, ok := store.Get("work:failed")
	if !ok {
		t.Fatal("missing merged failed activity")
	}
	if activity.OccurrenceCount != 3 || activity.FirstSequence != 10 || activity.LastSequence != 18 {
		t.Fatalf("failure timeline = count %d, %d..%d", activity.OccurrenceCount, activity.FirstSequence, activity.LastSequence)
	}
}

func TestActivityStoreTerminalUpdateAddsEvidenceAndRenewsAttention(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	first := ActivityEvent{ID: "work:failed", SessionID: "session", Epoch: 1, Lifecycle: ActivityLifecycleFailed, Outcome: OutcomeFailed, Sequence: 10}
	if err := store.Apply(first); err != nil {
		t.Fatal(err)
	}
	store.AcknowledgeTerminal()
	ref := DetailRef{Source: "memory", Key: "evidence", Size: 4, Digest: strings.Repeat("a", 64)}
	update := first
	update.Sequence = 20
	update.Control = ActivityControl{JumpTarget: "observation", DetailRefs: []DetailRef{ref}}
	if err := store.Apply(update); err != nil {
		t.Fatal(err)
	}
	activity, ok := store.Get(first.ID)
	if !ok {
		t.Fatal("missing terminal activity")
	}
	if activity.Acknowledged || activity.OccurrenceCount != 2 || !activityTestHasAction(activity.Actions, ActivityDetails) || activity.Control.JumpTarget != "observation" {
		t.Fatalf("terminal evidence update was not merged: %+v", activity)
	}
	if counts := store.Snapshot().Counts; counts.Failed != 1 {
		t.Fatalf("renewed failure attention counts = %+v", counts)
	}
}

func TestActivityStoreAcknowledgedReadyReviewStaysReadAcrossSameRunRefresh(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	first := ActivityEvent{
		ID: "background:agent", RunID: "run-1", Attempt: 1,
		SessionID: "session", Epoch: 1, Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded,
		Attention: ActivityAttention{Kind: ActivityAttentionReadyForReview, Severity: ActivityAttentionSeverityInfo, Unread: true},
		Sequence:  10, SourceSequence: 10,
	}
	if err := store.Apply(first); err != nil {
		t.Fatal(err)
	}
	if !store.AcknowledgeRun(first.ID, first.RunID) {
		t.Fatal("ready-review run was not acknowledged")
	}

	refresh := first
	refresh.Sequence = 20
	refresh.SourceSequence = 20
	refresh.Progress = ActivityProgress{Message: "terminal evidence retained"}
	if err := store.Apply(refresh); err != nil {
		t.Fatal(err)
	}
	acknowledged, ok := store.GetRun(first.ID, first.RunID)
	if !ok || acknowledged.Lifecycle != ActivityLifecycleCompleted || !acknowledged.Acknowledged || acknowledged.Attention.Unread {
		t.Fatalf("same-run terminal refresh reopened acknowledged result: %+v ok=%t", acknowledged, ok)
	}

	newRun := first
	newRun.RunID = "run-2"
	newRun.Attempt = 2
	newRun.Sequence = 30
	newRun.SourceSequence = 1
	if err := store.Apply(newRun); err != nil {
		t.Fatal(err)
	}
	latest, ok := store.GetRun(newRun.ID, newRun.RunID)
	if !ok || latest.Lifecycle != ActivityLifecycleCompleted || latest.Acknowledged || !latest.Attention.Unread {
		t.Fatalf("new ready-review run did not retain fresh unread attention: %+v ok=%t", latest, ok)
	}
	previous, ok := store.GetRun(first.ID, first.RunID)
	if !ok || !previous.Acknowledged || previous.Attention.Unread {
		t.Fatalf("new run changed prior acknowledgement: %+v ok=%t", previous, ok)
	}
}

func TestAppStateActivitySequenceFollowsReceiptOrderNotCallerClock(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	start := ActivityEvent{
		ID: "background:job", SessionID: "session", Epoch: 1,
		Kind: ActivityBackground, Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
		Sequence: 1_000_000,
	}
	finish := start
	finish.Lifecycle = ActivityLifecycleCompleted
	finish.Outcome = OutcomeSucceeded
	finish.Sequence = 1
	if err := state.ApplyActivity(start); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyActivity(finish); err != nil {
		t.Fatal(err)
	}

	snapshot := state.Activities.Snapshot()
	if len(snapshot.Activities) != 1 {
		t.Fatalf("activities = %#v", snapshot.Activities)
	}
	activity := snapshot.Activities[0]
	if activity.State != ActivityCompleted || activity.Outcome != OutcomeSucceeded {
		t.Fatalf("later terminal event was discarded by caller clock: %#v", activity)
	}
	if activity.Sequence != 2 || activity.LastSequence != 2 {
		t.Fatalf("app-owned terminal sequence = %d (last %d), want 2", activity.Sequence, activity.LastSequence)
	}
}

func TestApplyRuntimeErrorTerminalizesMatchingRunningActivity(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	ctx := ToolEventContext{
		SessionID: "session", TurnID: "session:turn-1", WorkUnitID: "verify", ActorID: "agent-1", ActorType: "executor",
	}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool-1", Name: "Bash", Input: map[string]any{"command": "go test ./..."}}); err != nil {
		t.Fatal(err)
	}
	before, ok := state.GetActivity("tool:tool-1")
	if !ok || before.State != ActivityRunning {
		t.Fatalf("running activity missing before runtime error: %+v", before)
	}
	if err := state.ApplyRuntimeError(ctx, "tool-1", "transport disconnected", nil, map[string]any{"attempt": 3}); err != nil {
		t.Fatal(err)
	}
	after, ok := state.GetActivity("tool:tool-1")
	if !ok {
		t.Fatal("activity missing after runtime error")
	}
	if after.State != ActivityFailed || after.Outcome != OutcomeFailed || !after.Provisional || after.Progress.Message != i18n.Text(state.Language.Get(), i18n.KeyRuntimeErrorPublicSummary) {
		t.Fatalf("runtime error activity = %+v", after)
	}
	if after.FirstSequence != before.FirstSequence || after.LastSequence <= before.LastSequence {
		t.Fatalf("runtime error timeline = %d..%d, before %d..%d", after.FirstSequence, after.LastSequence, before.FirstSequence, before.LastSequence)
	}
	if after.Control.JumpTarget == "" || len(after.Control.DetailRefs) == 0 || activityTestHasAction(after.Actions, ActivityCancel) {
		t.Fatalf("runtime error activity controls = %+v actions=%v", after.Control, after.Actions)
	}
	successCtx := ctx
	successCtx.Outcome = OutcomeSucceeded
	if err := state.ApplyToolResult(successCtx, types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tool-1", Content: "ok", Outcome: types.ToolOutcomeSucceeded}); err != nil {
		t.Fatal(err)
	}
	corrected, ok := state.GetActivity("tool:tool-1")
	if !ok || corrected.State != ActivityCompleted || corrected.Outcome != OutcomeSucceeded || corrected.Provisional {
		t.Fatalf("authoritative tool result did not correct provisional failure: %+v", corrected)
	}
}

func TestGlobalRuntimeErrorDoesNotTerminalizeSiblingTools(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	ctx := ToolEventContext{
		SessionID: "session", TurnID: "session:turn-7", WorkUnitID: "verify", ActorID: "agent-1", ActorType: "executor",
	}
	activities := []ActivityEvent{
		{ID: "tool:first", SessionID: "session", Epoch: 1, TurnID: ctx.TurnID, WorkUnitID: ctx.WorkUnitID, Actor: ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType}, Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning},
		{ID: "tool:second", SessionID: "session", Epoch: 1, TurnID: ctx.TurnID, WorkUnitID: ctx.WorkUnitID, Actor: ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType}, Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning},
		{ID: "tool:other-actor", SessionID: "session", Epoch: 1, TurnID: ctx.TurnID, WorkUnitID: ctx.WorkUnitID, Actor: ActivityActor{ID: "agent-2", Type: ctx.ActorType}, Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning},
		{ID: "tool:other-turn", SessionID: "session", Epoch: 1, TurnID: "session:turn-8", WorkUnitID: ctx.WorkUnitID, Actor: ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType}, Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning},
		{ID: "tool:already-done", SessionID: "session", Epoch: 1, TurnID: ctx.TurnID, WorkUnitID: ctx.WorkUnitID, Actor: ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType}, Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded},
	}
	for _, activity := range activities {
		if err := state.ApplyActivity(activity); err != nil {
			t.Fatal(err)
		}
	}

	if err := state.ApplyRuntimeError(ctx, "", "provider stream failed", nil, map[string]any{"retryable": false}); err != nil {
		t.Fatal(err)
	}
	for id, wantState := range map[string]ActivityState{
		"tool:first":        ActivityRunning,
		"tool:second":       ActivityRunning,
		"tool:other-actor":  ActivityRunning,
		"tool:other-turn":   ActivityRunning,
		"tool:already-done": ActivityCompleted,
	} {
		activity, ok := state.GetActivity(id)
		if !ok || activity.State != wantState {
			t.Fatalf("global transport error changed sibling %s = %+v, want %s", id, activity, wantState)
		}
	}
}

func activityTestEvent(id, name, actorID, workUnitID string, sequence uint64, state ActivityState) ActivityEvent {
	lifecycle, attention := activityTestLifecycleAndAttention(state)
	return ActivityEvent{
		ID:         id,
		SessionID:  "session-a",
		Epoch:      1,
		TurnID:     "session-a:1",
		WorkUnitID: workUnitID,
		Actor:      ActivityActor{ID: actorID, Type: "main"},
		Kind:       ActivityTool,
		Name:       name,
		Phase:      ActivityPhaseExecuting,
		Lifecycle:  lifecycle,
		Attention:  attention,
		Sequence:   sequence,
	}
}

func activityTestLifecycleAndAttention(state ActivityState) (ActivityLifecycle, ActivityAttention) {
	switch state {
	case ActivitySpawning:
		return ActivityLifecycleSpawning, ActivityAttention{}
	case ActivityQueued:
		return ActivityLifecycleQueued, ActivityAttention{}
	case ActivityRunning:
		return ActivityLifecycleRunning, ActivityAttention{}
	case ActivityWaiting:
		return ActivityLifecycleWaiting, ActivityAttention{}
	case ActivityBlocked:
		return ActivityLifecycleBlocked, ActivityAttention{}
	case ActivityNeedsInput:
		return ActivityLifecycleBlocked, ActivityAttention{Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning, Unread: true}
	case ActivityCompleted:
		return ActivityLifecycleCompleted, ActivityAttention{}
	case ActivityFailed:
		return ActivityLifecycleFailed, ActivityAttention{}
	case ActivityCancelled:
		return ActivityLifecycleCancelled, ActivityAttention{}
	case ActivityReadyReview:
		return ActivityLifecycleCompleted, ActivityAttention{Kind: ActivityAttentionReadyForReview, Severity: ActivityAttentionSeverityInfo, Unread: true}
	default:
		return "", ActivityAttention{}
	}
}

func activityTestIndex(activities []Activity) map[string]Activity {
	byID := make(map[string]Activity, len(activities))
	for _, activity := range activities {
		byID[activity.ID] = activity
	}
	return byID
}

func activityTestHasAction(actions []ActivityAction, want ActivityAction) bool {
	for _, action := range actions {
		if action == want {
			return true
		}
	}
	return false
}
