package compact

import (
	"context"
	"errors"
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
		SummaryMessages:           []types.Message{types.UserMessage("legacy summary")},
		PostCompactTokenCount:     20000,
		TruePostCompactTokenCount: 20000,
	}, nil
}

func TestAutoCompactIfNeededTriesSessionMemoryBeforeLegacyCompactor(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")
	ResetSessionMemoryCompactConfig()
	SetSessionMemoryCompactConfig(SessionMemoryCompactConfig{MinTokens: 1, MinTextBlockMessages: 1, MaxTokens: 1_000})
	defer ResetSessionMemoryCompactConfig()

	window := NewContextWindow(100)
	legacy := &autoCompactCountingCompactor{}
	messages := []types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("recent"),
	}
	provider := &fakeSessionMemoryProvider{snapshot: smSnapshot(t, messages, len(messages)-1, "session memory summary")}

	result, attempted, err := AutoCompactIfNeeded(context.Background(), messages, AutoCompactOptions{
		Window:                window,
		Compactor:             legacy,
		SessionMemoryProvider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !attempted || result == nil {
		t.Fatalf("attempted=%v result=%#v, want session-memory result", attempted, result)
	}
	if legacy.calls != 0 {
		t.Fatalf("legacy compactor calls = %d, want 0 when session memory compacts", legacy.calls)
	}
	if got := BuildPostCompactMessages(result)[1].GetText(); got == "" || got == "legacy summary" {
		t.Fatalf("unexpected session-memory post-compact summary: %q", got)
	}
}

func TestAutoCompactIfNeededFallsBackWhenSessionMemoryUnavailable(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")

	window := NewContextWindow(100)
	legacy := &autoCompactCountingCompactor{}
	provider := &fakeSessionMemoryProvider{snapshot: SessionMemorySnapshot{Available: false}}

	result, attempted, err := AutoCompactIfNeeded(context.Background(), []types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("recent"),
	}, AutoCompactOptions{
		Window:                window,
		Compactor:             legacy,
		SessionMemoryProvider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !attempted || result == nil {
		t.Fatalf("attempted=%v result=%#v, want legacy result", attempted, result)
	}
	if legacy.calls != 1 {
		t.Fatalf("legacy compactor calls = %d, want 1", legacy.calls)
	}
}

func TestAutoCompactIfNeededInvalidSessionMemoryAnchorDoesNotFallBack(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")

	window := NewContextWindow(100)
	legacy := &autoCompactCountingCompactor{}
	provider := &fakeSessionMemoryProvider{snapshot: SessionMemorySnapshot{
		Available:               true,
		Content:                 "stale session memory",
		LastSummarizedMessageID: "reused",
	}}

	result, attempted, err := AutoCompactIfNeeded(context.Background(), []types.Message{
		smMsg("reused", types.RoleAssistant, "current content cannot validate legacy memory"),
	}, AutoCompactOptions{
		Window:                window,
		Compactor:             legacy,
		SessionMemoryProvider: provider,
	})
	if result != nil || !attempted || !errors.Is(err, ErrSessionMemoryAnchorInvalid) {
		t.Fatalf("result=%#v attempted=%v err=%v, want fail-closed invalid anchor", result, attempted, err)
	}
	if legacy.calls != 0 {
		t.Fatalf("legacy compactor calls = %d, want 0 after invalid session-memory anchor", legacy.calls)
	}
}

func TestAutoCompactTelemetrySuccessAndFailureMetrics(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		window := NewContextWindow(50000)
		window.UsedInput = 40000
		legacy := &autoCompactCountingCompactor{}
		var events []CompactionTelemetryEvent

		result, attempted, err := AutoCompactIfNeeded(context.Background(), []types.Message{
			types.UserMessage("old"),
			types.AssistantMessage("recent"),
		}, AutoCompactOptions{
			Window:      window,
			Compactor:   legacy,
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
		legacy := &autoCompactCountingCompactor{err: fmt.Errorf("boom")}
		var events []CompactionTelemetryEvent

		_, attempted, err := AutoCompactIfNeeded(context.Background(), []types.Message{
			types.UserMessage("old"),
			types.AssistantMessage("recent"),
		}, AutoCompactOptions{
			Window:      window,
			Compactor:   legacy,
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
