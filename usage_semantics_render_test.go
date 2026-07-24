package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
)

func TestJSONTurnEndEmitsOneScopeSafeUsageProjection(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewJSONRenderer(&output)
	tracker := ui.NewCostTracker("claude-opus-4-5")
	handle := makeREPLEventHandlerWithCost(renderer, tracker, nil, func() (int, int) {
		return 1_000_000, 560_000
	})
	handle(loop.Event{Type: loop.EventTurnEnd, Usage: &types.Usage{
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

func TestContextEstimateEventPreservesLocalAuthorityAcrossJSONAndEngineProjection(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewJSONRenderer(&output)
	handle := makeREPLEventHandlerWithCost(renderer, nil, nil, nil)
	handle(loop.Event{Type: loop.EventContextUsage, ContextUsage: &loop.ContextUsageEvent{
		UsedTokens: 56_000, CapacityTokens: 100_000, Measurement: "local_estimate",
		EstimateComplete: false, UnknownOverheads: []string{"media"},
	}})
	if !strings.Contains(output.String(), `"type":"context_usage"`) || !strings.Contains(output.String(), `"measurement":"local_estimate"`) || !strings.Contains(output.String(), `"estimate_complete":false`) {
		t.Fatalf("JSON context event lost estimate authority: %s", output.String())
	}

	projection := effectiveContextFromEngine(&engine.ContextUsageInfo{
		TotalTokens: 100_000, UsedTokens: 56_000, Measurement: "local_estimate",
		EstimateComplete: false, UnknownOverheads: []string{"media"},
	})
	if projection.Measurement != ui.ContextMeasurementLocalEstimate || projection.EstimateComplete || projection.PercentUsed != 56 || len(projection.UnknownOverheads) != 1 {
		t.Fatalf("engine/TUI context projection = %+v", projection)
	}
}
