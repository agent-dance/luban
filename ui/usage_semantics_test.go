package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func incidentUsageSnapshot() UsageSemanticsSnapshot {
	last := &types.Usage{
		InputTokens:          136_081,
		OutputTokens:         146,
		CacheReadInputTokens: 131_999,
	}
	tracker := NewCostTracker("fixture-model")
	tracker.RestoreSession("fixture-model", 136_081, 146, 131_999, 0, 0, 16.6019)
	return BuildUsageSemanticsSnapshot(last, tracker, 560_000, 1_000_000)
}

func TestSessionProjectionUsesCurrentCompactionSegmentAndSessionOutput(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	tracker.RecordTurnUsage(types.Usage{InputTokens: 1_000, OutputTokens: 120, CacheReadInputTokens: 400}, time.Second)
	tracker.RecordTurnUsage(types.Usage{InputTokens: 1_500, OutputTokens: 60, CacheReadInputTokens: 600}, time.Second)
	tracker.RecordAuxiliaryUsage(types.Usage{InputTokens: 100, OutputTokens: 5, CacheReadInputTokens: 50})
	tracker.MarkCompaction()

	justCompacted := BuildSessionUsageProjection(tracker)
	if justCompacted.InputTokens != 0 || justCompacted.TotalInputTokens != 2_600 || justCompacted.CacheHitKnown {
		t.Fatalf("just-compacted projection = %+v", justCompacted)
	}

	tracker.RecordTurnUsage(types.Usage{InputTokens: 700, OutputTokens: 60, CacheReadInputTokens: 200}, time.Second)
	tracker.RecordTurnUsage(types.Usage{InputTokens: 900, OutputTokens: 70, CacheReadInputTokens: 450}, time.Second)
	current := BuildSessionUsageProjection(tracker)
	if current.InputTokens != 1_600 || current.TotalInputTokens != 4_200 || current.OutputTokens != 315 || current.CacheReadTokens != 650 || current.CacheHitPercent != 41 || !current.CacheHitKnown {
		t.Fatalf("first compacted segment projection = %+v", current)
	}

	tracker.MarkCompaction()
	tracker.RecordTurnUsage(types.Usage{InputTokens: 600, OutputTokens: 40, CacheReadInputTokens: 300}, time.Second)
	second := BuildSessionUsageProjection(tracker)
	if second.InputTokens != 600 || second.TotalInputTokens != 4_800 || second.OutputTokens != 355 || second.CacheHitPercent != 50 {
		t.Fatalf("second compacted segment projection = %+v", second)
	}
}

func TestJSONUsageSemanticsKeepsAccountingScopesSeparate(t *testing.T) {
	var output bytes.Buffer
	NewJSONRenderer(&output).UsageSemantics(incidentUsageSnapshot())
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if event["type"] != "usage_summary" || event["schema_version"] != UsageSemanticsSchemaVersion {
		t.Fatalf("event identity = %#v", event)
	}
	last := event["last_request"].(map[string]any)
	session := event["cumulative_session"].(map[string]any)
	context := event["model_context"].(map[string]any)
	if last["scope"] != "last_request" || last["input_tokens"] != float64(136_081) || last["output_tokens"] != float64(146) || last["cache_hit_percent"] != float64(97) {
		t.Fatalf("last request projection = %#v", last)
	}
	if session["scope"] != "cumulative_session" || session["cost_usd"] != 16.6019 || session["cache_hit_percent"] != float64(97) {
		t.Fatalf("cumulative session projection = %#v", session)
	}
	if context["scope"] != "model_context" || context["percent_used"] != float64(56) || context["capacity_tokens"] != float64(1_000_000) {
		t.Fatalf("context projection = %#v", context)
	}
}

func TestScreenReaderUsageSemanticsUsesSameScopedLabels(t *testing.T) {
	var output bytes.Buffer
	renderer := NewScreenReaderRenderer(&output, nil)
	renderer.UsageSemantics(incidentUsageSnapshot())
	renderer.Close()
	text := output.String()
	for _, want := range []string{
		"Session: in 136.1K · 97% cached · out 146 · $16.6019",
		"Context: 56% (560.0K/1000.0K)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("screen-reader projection missing %q in %q", want, text)
		}
	}
}

func TestUsageSemanticsMakesUnknownLedgerAndContextVisible(t *testing.T) {
	snapshot := BuildUsageSemanticsSnapshot(&types.Usage{InputTokens: 7}, nil, 0, 0)
	if !snapshot.LastRequest.Known || snapshot.CumulativeSession.Known || snapshot.EffectiveModelContext.Known {
		t.Fatalf("unknown accounting scopes collapsed to zero: %+v", snapshot)
	}
}

func TestStructuredRenderersLabelLocalContextEstimate(t *testing.T) {
	projection := EffectiveContextProjection{
		Scope: UsageScopeModelContext, Known: true, UsedTokens: 56_000, CapacityTokens: 100_000, PercentUsed: 56,
		Measurement: ContextMeasurementLocalEstimate, EstimateComplete: false,
		UnknownOverheads: []string{"media"},
	}
	var jsonOutput bytes.Buffer
	NewJSONRenderer(&jsonOutput).EffectiveContext(projection)
	if !strings.Contains(jsonOutput.String(), `"measurement":"local_estimate"`) || !strings.Contains(jsonOutput.String(), `"estimate_complete":false`) || !strings.Contains(jsonOutput.String(), `"media"`) {
		t.Fatalf("JSON estimate projection lost authority/completeness: %s", jsonOutput.String())
	}

	var spoken bytes.Buffer
	renderer := NewScreenReaderRenderer(&spoken, nil)
	renderer.EffectiveContext(projection)
	renderer.Close()
	if !strings.Contains(spoken.String(), "Context: ≥56%") || !strings.Contains(spoken.String(), "at least 56.0K/100.0K") {
		t.Fatalf("screen-reader estimate was not fail-visible: %q", spoken.String())
	}
}
