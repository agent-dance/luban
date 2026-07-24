package tui

import (
	"encoding/json"
	"testing"

	"github.com/agent-dance/luban/ui"
)

func TestApplyHookSummaryRetainsCausalEvidenceAndActivity(t *testing.T) {
	state := NewAppState()
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   SessionIdentity{SessionID: "session-hook", Epoch: 7},
		Projection: SessionProjection{Details: NewMemoryDetailStore()},
	}); err != nil {
		t.Fatalf("ApplySessionSnapshot: %v", err)
	}
	ctx := ToolEventContext{SessionID: "session-hook", TurnID: "session-hook:turn-2", WorkUnitID: "review", ActorID: "agent-reviewer", ActorType: "reviewer"}
	summary := ui.HookSummary{ExecutionID: "hook:session-hook:turn-2:PostSampling", ToolUseID: "tool-source", Name: "PostSampling", Status: "blocked", Summary: "policy rejected output", Metadata: map[string]any{"outputs": 1}}
	if err := state.ApplyHookSummary(ctx, 7, summary); err != nil {
		t.Fatalf("ApplyHookSummary: %v", err)
	}

	activity, ok := state.GetActivity(summary.ExecutionID)
	if !ok {
		t.Fatal("hook activity missing")
	}
	if activity.Kind != ActivityHook || activity.Phase != ActivityPhaseVerifying || activity.State != ActivityFailed || activity.Outcome != OutcomeDenied {
		t.Fatalf("hook activity state = %#v", activity)
	}
	if activity.TurnID != ctx.TurnID || activity.WorkUnitID != ctx.WorkUnitID || activity.Actor.ID != ctx.ActorID || len(activity.Control.DetailRefs) != 1 {
		t.Fatalf("hook activity causality/evidence = %#v", activity)
	}
	observation, ok := state.Observations.Get(activity.Control.JumpTarget)
	if !ok || observation.TurnID != ctx.TurnID || observation.WorkUnitID != ctx.WorkUnitID || observation.ActorID != ctx.ActorID || observation.ToolUseID != "tool-source" {
		t.Fatalf("hook observation causality = %#v, ok=%v", observation, ok)
	}
	payload, err := state.Details.Get(activity.Control.DetailRefs[0])
	if err != nil {
		t.Fatalf("read hook evidence: %v", err)
	}
	var evidence map[string]any
	if err := json.Unmarshal(payload, &evidence); err != nil {
		t.Fatalf("decode hook evidence: %v", err)
	}
	if evidence["hook_execution_id"] != summary.ExecutionID || evidence["status"] != "blocked" {
		t.Fatalf("hook evidence = %#v", evidence)
	}
}

func TestPreventedHookRemainsActionable(t *testing.T) {
	state := NewAppState()
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity: SessionIdentity{SessionID: "session-hook", Epoch: 2}, Projection: SessionProjection{Details: NewMemoryDetailStore()},
	}); err != nil {
		t.Fatal(err)
	}
	ctx := ToolEventContext{SessionID: "session-hook", TurnID: "session-hook:turn-4", WorkUnitID: "stop", ActorID: "assistant"}
	summary := ui.HookSummary{ExecutionID: "hook:stop", Name: "Stop", Status: "prevented", Summary: "continuation required"}
	if err := state.ApplyHookSummary(ctx, 2, summary); err != nil {
		t.Fatal(err)
	}
	activity, ok := state.GetActivity(summary.ExecutionID)
	if !ok || activity.State != ActivityFailed || activity.Outcome != OutcomeDenied {
		t.Fatalf("prevented hook was hidden as success: activity=%+v ok=%v", activity, ok)
	}
	if snapshot := state.ActivitySnapshot(); snapshot.Counts.Denied != 1 {
		t.Fatalf("prevented hook attention counts = %+v", snapshot.Counts)
	}
}
