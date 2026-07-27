package stream

import (
	"encoding/json"
	"testing"
)

func TestEventMarshalJSONPreservesSurfaceNeutralPayload(t *testing.T) {
	event := Event{
		Type:      EventProgress,
		TurnCount: 3,
		Progress: &ProgressEvent{
			Stage:         "agentic_flight",
			Message:       "compacting",
			Disposition:   "completed_verified",
			Blocker:       "ready",
			MutationEpoch: 3,
			VerifiedEpoch: 3,
		},
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Event
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Type != event.Type || decoded.TurnCount != event.TurnCount || decoded.Progress == nil {
		t.Fatalf("decoded event = %#v", decoded)
	}
	if decoded.Progress.Stage != event.Progress.Stage || decoded.Progress.Message != event.Progress.Message ||
		decoded.Progress.Disposition != event.Progress.Disposition || decoded.Progress.Blocker != event.Progress.Blocker ||
		decoded.Progress.MutationEpoch != 3 || decoded.Progress.VerifiedEpoch != 3 {
		t.Fatalf("decoded progress = %#v", decoded.Progress)
	}
}

func TestSystemWarningMarshalJSONFailsClosed(t *testing.T) {
	if _, err := json.Marshal(Event{Type: EventSystemWarning}); err == nil {
		t.Fatal("system warning marshaled without an audience projection")
	}
}

func TestToolRoundMetricsMarshalWithoutToolContent(t *testing.T) {
	event := Event{
		Type: EventToolRoundMetrics,
		ToolRound: &ToolRoundMetricsEvent{
			RoundID: "turn-7", LogicalModelVisibleCalls: 3,
			PhysicalChildOperations: 3, Fanout: 2, BatchCount: 2,
			QueueMilliseconds: 12, CriticalPathMilliseconds: 30,
			TotalChildLatencyMilliseconds: 44, ErrorCount: 1,
			RevisionFusionCount: 1, RevisionBarrierSkips: 1, RevisionMismatchCount: 1,
		},
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Event
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ToolRound == nil || decoded.ToolRound.RoundID != "turn-7" || decoded.ToolRound.LogicalModelVisibleCalls != 3 || decoded.ToolRound.ErrorCount != 1 || decoded.ToolRound.RevisionFusionCount != 1 || decoded.ToolRound.RevisionBarrierSkips != 1 || decoded.ToolRound.RevisionMismatchCount != 1 {
		t.Fatalf("decoded metrics = %+v", decoded.ToolRound)
	}
}
