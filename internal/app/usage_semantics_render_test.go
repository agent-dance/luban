package app

import (
	"bytes"
	"encoding/json"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"io"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/types"
)

func TestJSONTurnEndEmitsOneScopeSafeUsageProjection(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewJSONRenderer(&output)
	tracker := ui.NewCostTracker("claude-opus-4-5")
	handle, cleanup := makeTUIEventHandler(renderer, tracker, func() (int, int) {
		return 1_000_000, 560_000
	})
	t.Cleanup(cleanup)
	handle(stream.Event{Type: stream.EventTurnEnd, Usage: &types.Usage{
		InputTokens: 136_081, OutputTokens: 146, CacheReadInputTokens: 131_999,
	}})

	lines := strings.FieldsFunc(strings.TrimSpace(output.String()), func(r rune) bool { return r == '\n' })
	if len(lines) != 1 {
		t.Fatalf("turn end emitted %d usage ledgers instead of one: %q", len(lines), output.String())
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatalf("decode usage event: %v", err)
	}
	if event["type"] != "usage_summary" || event["last_request"] == nil || event["cumulative_session"] == nil || event["model_context"] == nil {
		t.Fatalf("scope-safe usage event = %#v", event)
	}
}

func TestEngineContextProjectionPreservesMeasurementSource(t *testing.T) {
	estimated := modelContextFromEngine(&engine.ContextUsageInfo{
		TotalTokens: 100_000, UsedTokens: 56_000, Measurement: "local_estimate",
	})
	if !estimated.Known || estimated.Measurement != presentation.ContextMeasurementLocalEstimate || estimated.PercentUsed != 56 {
		t.Fatalf("local estimate projection = %+v", estimated)
	}

	exact := modelContextFromEngine(&engine.ContextUsageInfo{
		TotalTokens: 100_000, UsedTokens: 56_000, Measurement: "provider_reported",
	})
	if !exact.Known || exact.Measurement != presentation.ContextMeasurementProviderReported || exact.PercentUsed != 56 {
		t.Fatalf("provider usage projection = %+v", exact)
	}
}

func TestBDDUsageEventDeliveryIsAccountedExactlyOnce(t *testing.T) {
	tracker := ui.NewCostTracker("claude-opus-4-5")
	handle, cleanup := makeTUIEventHandler(ui.NewQuietRenderer(io.Discard), tracker, nil)
	t.Cleanup(cleanup)
	discarded := stream.Event{
		Type: stream.EventProviderUsage, TurnID: "turn-1",
		Usage:    &types.Usage{InputTokens: 1_000, OutputTokens: 100},
		Metadata: map[string]any{"kind": "provider_attempt", "usage_id": "provider_request:first"},
	}
	handle(discarded)
	handle(discarded)
	handle(stream.Event{
		Type: stream.EventTurnEnd, TurnID: "turn-1",
		Usage:    &types.Usage{InputTokens: 1_500, OutputTokens: 200},
		Metadata: map[string]any{"usage_id": "provider_request:second"},
	})
	snapshot := tracker.Snapshot()
	if snapshot.SessionInput != 2_500 || snapshot.SessionOutput != 300 {
		t.Fatalf("duplicate delivery changed billed session totals: %+v", snapshot)
	}
}
