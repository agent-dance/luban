package compact

import (
	"context"
	"fmt"
	"testing"

	"github.com/agent-dance/luban/types"
)

type autoCompactCountingCompactor struct {
	calls int
	err   error
}

func (c *autoCompactCountingCompactor) Compact(context.Context, []types.Message, int) (*CompactionResult, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	boundary := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "auto"})
	return &CompactionResult{
		BoundaryMarker:            &boundary,
		SummaryMessages:           []types.Message{types.UserMessage("summary")},
		PostCompactTokenCount:     20000,
		TruePostCompactTokenCount: 20000,
	}, nil
}

func TestAutoCompactTelemetrySuccessAndFailureMetrics(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		window := NewContextWindow(50000)
		window.UsedInput = 40000
		compactor := &autoCompactCountingCompactor{}
		var events []CompactionTelemetryEvent

		result, attempted, err := AutoCompactIfNeeded(context.Background(), []types.Message{
			types.UserMessage("old"),
			types.AssistantMessage("recent"),
		}, AutoCompactOptions{
			Window:      window,
			Compactor:   compactor,
			OnTelemetry: func(event CompactionTelemetryEvent) { events = append(events, event) },
		})
		if err != nil {
			t.Fatal(err)
		}
		if !attempted || result == nil {
			t.Fatalf("attempted=%v result=%#v, want success", attempted, result)
		}
		success := findCompactTelemetry(events, CompactionTelemetryAutoSuccess)
		if success == nil {
			t.Fatalf("missing auto success telemetry in %+v", events)
		}
		if success.AutoCompactThreshold == 0 || !success.PostCompactWouldRetrigger {
			t.Fatalf("success telemetry missing threshold/retrigger likelihood: %+v", *success)
		}
		if success.OriginalMessageCount != 2 || success.CompactedMessageCount != len(BuildPostCompactMessages(result)) {
			t.Fatalf("success telemetry missing message counts: %+v", *success)
		}
		if success.CompactionUsage != nil {
			t.Fatalf("nil provider usage should remain nil, got %+v", success.CompactionUsage)
		}
	})

	t.Run("failure", func(t *testing.T) {
		window := NewContextWindow(50000)
		window.UsedInput = 40000
		compactor := &autoCompactCountingCompactor{err: fmt.Errorf("boom")}
		var events []CompactionTelemetryEvent

		_, attempted, err := AutoCompactIfNeeded(context.Background(), []types.Message{
			types.UserMessage("old"),
			types.AssistantMessage("recent"),
		}, AutoCompactOptions{
			Window:      window,
			Compactor:   compactor,
			OnTelemetry: func(event CompactionTelemetryEvent) { events = append(events, event) },
		})
		if err == nil || !attempted {
			t.Fatalf("attempted=%v err=%v, want attempted failure", attempted, err)
		}
		failure := findCompactTelemetry(events, CompactionTelemetryAutoFailure)
		if failure == nil {
			t.Fatalf("missing auto failure telemetry in %+v", events)
		}
		if failure.ConsecutiveFailureCount != 1 || failure.MaxConsecutiveFailureCount != MaxConsecutiveAutocompactFailures {
			t.Fatalf("failure telemetry missing consecutive counts: %+v", *failure)
		}
		if failure.ErrorType == "" || failure.CompactionUsage != nil {
			t.Fatalf("failure telemetry should include error type and nil usage: %+v", *failure)
		}
	})
}

func findCompactTelemetry(events []CompactionTelemetryEvent, kind CompactionTelemetryKind) *CompactionTelemetryEvent {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}
