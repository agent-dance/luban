package tui

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestDecisionProjectsCorrelatedToolAndTranscriptAsBlockedThenRestoresRunning(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	ctx := ToolEventContext{
		SessionID: "session", TurnID: "turn-1", WorkUnitID: "work-1",
		ActorID: "agent-1", ActorType: "agent", Outcome: OutcomeRunning,
	}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "write-1", Name: "Write", Input: map[string]any{"file_path": "/workspace/a"}}); err != nil {
		t.Fatal(err)
	}
	assertLifecycle := func(wantActivity ActivityLifecycle, wantPresentation PresentationLifecycleState) {
		t.Helper()
		activity := activityTestIndex(state.ActivitySnapshot().Activities)["tool:write-1"]
		if activity.Lifecycle != wantActivity {
			t.Fatalf("tool lifecycle = %s, want %s; activity=%+v", activity.Lifecycle, wantActivity, activity)
		}
		observation, ok := state.GetObservation(toolObservationID("session", "write-1"))
		if !ok || observation.Presentation.Lifecycle != wantPresentation {
			t.Fatalf("tool presentation lifecycle = %s, want %s; observation=%+v ok=%t", observation.Presentation.Lifecycle, wantPresentation, observation, ok)
		}
	}
	assertLifecycle(ActivityLifecycleRunning, PresentationLifecycleRunning)

	decision := ActivityEvent{
		ID: "decision:permission-1", SessionID: "session", Epoch: 1, TurnID: "turn-1", WorkUnitID: "work-1",
		Actor: ActivityActor{ID: "agent-1", Type: "agent"}, Kind: ActivityDecision, Name: "Write",
		Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning,
		Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning, Unread: true, DecisionID: "permission-1"},
	}
	if err := state.ApplyActivity(decision); err != nil {
		t.Fatal(err)
	}
	assertLifecycle(ActivityLifecycleBlocked, PresentationLifecycleBlocked)
	if activity := activityTestIndex(state.ActivitySnapshot().Activities)["tool:write-1"]; activity.Outcome != OutcomeUnknown {
		t.Fatalf("blocked tool still exposed running outcome: %+v", activity)
	}

	decision.Lifecycle = ActivityLifecycleCompleted
	decision.Outcome = OutcomeSucceeded
	decision.Attention = ActivityAttention{}
	if err := state.ApplyActivity(decision); err != nil {
		t.Fatal(err)
	}
	assertLifecycle(ActivityLifecycleRunning, PresentationLifecycleRunning)
}
