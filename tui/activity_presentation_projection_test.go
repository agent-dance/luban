package tui

import (
	"reflect"
	"testing"
)

func TestLogicalActivitiesForPresentationMergesRetainedAgentWithToolWrapper(t *testing.T) {
	wrapper := projectionTestAgent("tool:agent-call", "", "", 1, 10)
	wrapper.Lifecycle = ActivityLifecycleCompleted
	retained := projectionTestAgent("background:agent-1", "agent-1", "agent-call", 1, 20)
	retained.Lifecycle = ActivityLifecycleCompleted
	retained.Attention = ActivityAttention{Kind: ActivityAttentionReadyForReview, Unread: true}
	tool := Activity{ActivityEvent: ActivityEvent{
		ID: "tool:read", SessionID: "session", Epoch: 1, Kind: ActivityTool,
		Lifecycle: ActivityLifecycleRunning, Sequence: 5,
	}}

	got := logicalActivitiesForPresentation([]Activity{wrapper, tool, retained})
	if len(got) != 2 {
		t.Fatalf("logical activities = %#v, want retained agent plus non-agent tool", got)
	}
	if _, found := projectionActivityByID(got, wrapper.ID); found {
		t.Fatalf("Agent tool wrapper remained in logical projection: %#v", got)
	}
	if agent, found := projectionActivityByID(got, retained.ID); !found || agent.Attention.Kind != ActivityAttentionReadyForReview {
		t.Fatalf("retained Agent was not the representative: %#v", got)
	}
	if _, found := projectionActivityByID(got, tool.ID); !found {
		t.Fatalf("non-Agent activity was removed: %#v", got)
	}
}

func TestLogicalActivitiesForPresentationIsStableAcrossArrivalOrder(t *testing.T) {
	activities := []Activity{
		projectionTestAgent("tool:agent-call", "", "", 1, 10),
		projectionTestAgent("background:agent-1", "agent-1", "agent-call", 1, 20),
		{ActivityEvent: ActivityEvent{ID: "tool:verify", SessionID: "session", Epoch: 1, Kind: ActivityTool, Lifecycle: ActivityLifecycleRunning, Sequence: 30}},
	}
	reversed := append([]Activity(nil), activities...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}

	forward := logicalActivitiesForPresentation(activities)
	backward := logicalActivitiesForPresentation(reversed)
	if !reflect.DeepEqual(forward, backward) {
		t.Fatalf("arrival order changed logical projection:\nforward=%#v\nbackward=%#v", forward, backward)
	}
}

func TestLogicalActivitiesForPresentationKeepsDirectOnlyAgent(t *testing.T) {
	direct := projectionTestAgent("tool:agent-call", "agent-direct", "agent-call", 1, 10)
	got := logicalActivitiesForPresentation([]Activity{direct})
	if len(got) != 1 || got[0].ID != direct.ID {
		t.Fatalf("direct-only Agent projection = %#v, want %#v", got, direct)
	}
}

func TestLogicalActivitiesForPresentationDoesNotMergeAgentsByName(t *testing.T) {
	first := projectionTestAgent("background:agent-a", "agent-a", "call-a", 1, 10)
	second := projectionTestAgent("background:agent-b", "agent-b", "call-b", 1, 20)
	first.Name, second.Name = "reviewer", "reviewer"

	got := logicalActivitiesForPresentation([]Activity{second, first})
	if len(got) != 2 {
		t.Fatalf("same-name independent Agents were merged: %#v", got)
	}
	if _, found := projectionActivityByID(got, first.ID); !found {
		t.Fatalf("first same-name Agent missing: %#v", got)
	}
	if _, found := projectionActivityByID(got, second.ID); !found {
		t.Fatalf("second same-name Agent missing: %#v", got)
	}
}

func TestLogicalActivitiesForPresentationUsesLatestAttempt(t *testing.T) {
	oldAttempt := projectionTestAgent("background:agent-1", "agent-1", "agent-call", 1, 30)
	oldAttempt.RunID = "run-1"
	oldAttempt.Lifecycle = ActivityLifecycleCompleted
	latestAttempt := projectionTestAgent("background:agent-1", "agent-1", "agent-call", 2, 10)
	latestAttempt.RunID = "run-2"
	latestAttempt.Lifecycle = ActivityLifecycleRunning

	got := logicalActivitiesForPresentation([]Activity{latestAttempt, oldAttempt})
	if len(got) != 1 || got[0].Attempt != 2 || got[0].RunID != "run-2" || got[0].Lifecycle != ActivityLifecycleRunning {
		t.Fatalf("latest Agent attempt was not selected: %#v", got)
	}
}

func TestSuccessfulRetryExplicitlySupersedesHistoricalFailure(t *testing.T) {
	store := NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	if err := store.Apply(ActivityEvent{
		ID: "tool:edit", RunID: "run-1", Attempt: 1, SessionID: "session", Epoch: 1,
		Kind: ActivityTool, Lifecycle: ActivityLifecycleFailed, Outcome: OutcomeFailed, Sequence: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Apply(ActivityEvent{
		ID: "tool:edit", RunID: "run-2", Attempt: 2, SessionID: "session", Epoch: 1,
		Kind: ActivityTool, Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded, Sequence: 2,
	}); err != nil {
		t.Fatal(err)
	}

	current := store.Snapshot()
	if len(current.Activities) != 1 || current.Counts.Completed != 1 || current.Counts.Failed != 0 {
		t.Fatalf("current work projection = %+v", current)
	}
	if current.Activities[0].RunID != "run-2" || current.Activities[0].SupersedesRunID != "run-1" {
		t.Fatalf("retry lineage = %+v", current.Activities[0])
	}
	history := store.RunHistory()
	if len(history) != 2 || history[0].RunID != "run-1" || history[1].RunID != "run-2" {
		t.Fatalf("append-only attempt history = %+v", history)
	}
}

func TestLogicalActivitiesForPresentationFailsClosedOnMismatchedRetainedIdentity(t *testing.T) {
	wrapper := projectionTestAgent("tool:agent-call", "", "", 1, 10)
	retained := projectionTestAgent("background:agent-1", "different-agent", "agent-call", 1, 20)

	got := logicalActivitiesForPresentation([]Activity{wrapper, retained})
	if len(got) != 2 {
		t.Fatalf("mismatched retained identity suppressed an Agent wrapper: %#v", got)
	}
}

func TestLogicalActivitiesForPresentationKeepsDistinctRetainedAgentsSharingParent(t *testing.T) {
	wrapper := projectionTestAgent("tool:agent-call", "", "", 1, 10)
	first := projectionTestAgent("background:agent-1", "agent-1", "agent-call", 1, 20)
	second := projectionTestAgent("background:agent-2", "agent-2", "agent-call", 1, 30)

	got := logicalActivitiesForPresentation([]Activity{wrapper, first, second})
	if len(got) != 2 {
		t.Fatalf("distinct retained Agents sharing a parent were merged: %#v", got)
	}
	if _, found := projectionActivityByID(got, first.ID); !found {
		t.Fatalf("first retained Agent missing: %#v", got)
	}
	if _, found := projectionActivityByID(got, second.ID); !found {
		t.Fatalf("second retained Agent missing: %#v", got)
	}
}

func projectionTestAgent(id, agentID, parentToolUseID string, attempt int, sequence uint64) Activity {
	return Activity{ActivityEvent: ActivityEvent{
		ID: id, RunID: id + "-run", Attempt: attempt,
		SessionID: "session", Epoch: 1, Kind: ActivityAgent,
		Lifecycle: ActivityLifecycleRunning, Sequence: sequence,
		Progress: ActivityProgress{AgentID: agentID, ParentToolUseID: parentToolUseID},
	}, FirstSequence: sequence, LastSequence: sequence}
}

func projectionActivityByID(activities []Activity, id string) (Activity, bool) {
	for _, activity := range activities {
		if activity.ID == id {
			return activity, true
		}
	}
	return Activity{}, false
}
