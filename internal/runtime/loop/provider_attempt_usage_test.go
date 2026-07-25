package loop

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func providerUsageTextEvents(text string, usage types.Usage, reason types.StopReason) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Usage: &types.Usage{InputTokens: usage.InputTokens, CacheCreationInputTokens: usage.CacheCreationInputTokens, CacheReadInputTokens: usage.CacheReadInputTokens}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, Usage: &types.Usage{OutputTokens: usage.OutputTokens, ServerToolUse: usage.ServerToolUse}, StopReason: &reason},
		{Type: types.EventMessageStop},
	}
}

func providerUsageEmptyEvents(usage types.Usage) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Usage: &types.Usage{InputTokens: usage.InputTokens, CacheCreationInputTokens: usage.CacheCreationInputTokens, CacheReadInputTokens: usage.CacheReadInputTokens}},
		{Type: types.EventMessageDelta, Usage: &types.Usage{OutputTokens: usage.OutputTokens, ServerToolUse: usage.ServerToolUse}},
		{Type: types.EventMessageStop},
	}
}

func collectProviderAccountingEvents(t *testing.T, q *QueryLoop) (attempts []stream.Event, turnEnds []stream.Event) {
	t.Helper()
	if err := q.Run(context.Background(), "hello", func(event stream.Event) {
		switch event.Type {
		case stream.EventProviderUsage:
			attempts = append(attempts, event)
		case stream.EventTurnEnd:
			turnEnds = append(turnEnds, event)
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return attempts, turnEnds
}

func assertProviderUsageMetadata(t *testing.T, event stream.Event, providerName, model string) {
	t.Helper()
	if event.Metadata["provider"] != providerName || event.Metadata["model"] != model {
		t.Fatalf("provider usage metadata = %+v, want provider=%q model=%q", event.Metadata, providerName, model)
	}
	if event.Metadata["request_id"] == "" || event.Metadata["usage_id"] == "" {
		t.Fatalf("provider usage lacks stable accounting identity: %+v", event.Metadata)
	}
}

func TestMaxTokensRecoveryAccountsDiscardedAndFinalAttemptsOnce(t *testing.T) {
	first := types.Usage{InputTokens: 100, OutputTokens: 10, CacheReadInputTokens: 80}
	final := types.Usage{InputTokens: 120, OutputTokens: 20, CacheCreationInputTokens: 25}
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: providerUsageTextEvents("partial", first, types.StopReasonMaxTokens)},
		{Events: providerUsageTextEvents("complete", final, types.StopReasonEndTurn)},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3, Model: "primary-model", MaxTokens: 1024})

	attempts, turnEnds := collectProviderAccountingEvents(t, q)
	if len(attempts) != 1 || attempts[0].Usage == nil || *attempts[0].Usage != first {
		t.Fatalf("discarded attempt events = %+v, want exactly first usage %+v", attempts, first)
	}
	assertProviderUsageMetadata(t, attempts[0], "parity-fake", "primary-model")
	if len(turnEnds) != 1 || turnEnds[0].Usage == nil || *turnEnds[0].Usage != final {
		t.Fatalf("turn_end events = %+v, want exactly final usage %+v", turnEnds, final)
	}
	assertProviderUsageMetadata(t, turnEnds[0], "parity-fake", "primary-model")
}

func TestEmptyResponseRetryAccountsBothAttemptsOnce(t *testing.T) {
	first := types.Usage{InputTokens: 200, OutputTokens: 1, CacheReadInputTokens: 150}
	final := types.Usage{InputTokens: 210, OutputTokens: 15, CacheReadInputTokens: 160}
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: providerUsageEmptyEvents(first)},
		{Events: providerUsageTextEvents("recovered", final, types.StopReasonEndTurn)},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 1, Model: "primary-model", MaxTokens: 1024})

	attempts, turnEnds := collectProviderAccountingEvents(t, q)
	if len(attempts) != 1 || attempts[0].Usage == nil || *attempts[0].Usage != first {
		t.Fatalf("discarded attempt events = %+v, want exactly first usage %+v", attempts, first)
	}
	if len(turnEnds) != 1 || turnEnds[0].Usage == nil || *turnEnds[0].Usage != final {
		t.Fatalf("turn_end events = %+v, want exactly final usage %+v", turnEnds, final)
	}
}

func TestStreamRetryPreservesUsageReportedBeforeError(t *testing.T) {
	first := types.Usage{InputTokens: 300, OutputTokens: 2, CacheCreationInputTokens: 40}
	final := types.Usage{InputTokens: 310, OutputTokens: 18, CacheReadInputTokens: 250}
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: []types.StreamEvent{
			{Type: types.EventMessageStart, Usage: &types.Usage{InputTokens: first.InputTokens, CacheCreationInputTokens: first.CacheCreationInputTokens}},
			{Type: types.EventError, Usage: &types.Usage{OutputTokens: first.OutputTokens}, Error: &types.APIError{Type: "api_error", Message: "upstream disconnected"}},
		}},
		{Events: providerUsageTextEvents("recovered", final, types.StopReasonEndTurn)},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 1, Model: "primary-model", MaxTokens: 1024})

	attempts, turnEnds := collectProviderAccountingEvents(t, q)
	if len(attempts) != 1 || attempts[0].Usage == nil || *attempts[0].Usage != first {
		t.Fatalf("discarded attempt events = %+v, want exactly first usage %+v", attempts, first)
	}
	if len(turnEnds) != 1 || turnEnds[0].Usage == nil || *turnEnds[0].Usage != final {
		t.Fatalf("turn_end events = %+v, want exactly final usage %+v", turnEnds, final)
	}
}

func TestFallbackAccountsPrimaryAndPricesFinalWithFallbackModel(t *testing.T) {
	primary := types.Usage{InputTokens: 400, OutputTokens: 3, CacheReadInputTokens: 320}
	fallback := types.Usage{InputTokens: 410, OutputTokens: 25, CacheReadInputTokens: 330}
	fallbackEvents := providerUsageTextEvents("orphan", primary, types.StopReasonEndTurn)
	fallbackEvents[len(fallbackEvents)-1] = types.StreamEvent{Type: types.EventError, Error: &types.APIError{
		Type: "fallback_triggered", Message: "busy", OriginalModel: "primary-model", FallbackModel: "fallback-model",
	}}
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: fallbackEvents},
		{Events: providerUsageTextEvents("fallback answer", fallback, types.StopReasonEndTurn)},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 1, Model: "primary-model", MaxTokens: 1024})

	attempts, turnEnds := collectProviderAccountingEvents(t, q)
	if len(attempts) != 1 || attempts[0].Usage == nil || *attempts[0].Usage != primary {
		t.Fatalf("primary attempt events = %+v, want exactly primary usage %+v", attempts, primary)
	}
	assertProviderUsageMetadata(t, attempts[0], "parity-fake", "primary-model")
	if len(turnEnds) != 1 || turnEnds[0].Usage == nil || *turnEnds[0].Usage != fallback {
		t.Fatalf("turn_end events = %+v, want exactly fallback usage %+v", turnEnds, fallback)
	}
	assertProviderUsageMetadata(t, turnEnds[0], "parity-fake", "fallback-model")
}
