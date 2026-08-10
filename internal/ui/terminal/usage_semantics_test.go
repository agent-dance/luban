package ui

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/presentation"
	"github.com/agent-dance/luban/provider"

	"github.com/agent-dance/luban/types"
)

func incidentUsageSnapshot() presentation.UsageSemanticsSnapshot {
	last := &types.Usage{
		InputTokens:          136_081,
		OutputTokens:         146,
		CacheReadInputTokens: 131_999,
	}
	tracker := NewCostTracker("fixture-model")
	tracker.RestoreSession("fixture-model", 136_081, 146, 131_999, 0, 0, 16.6019, true)
	return BuildUsageSemanticsSnapshot(last, tracker, 560_000, 1_000_000)
}

func TestSessionProjectionUsesCurrentCompactionSegmentAndSessionOutput(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{InputTokens: 1_000, OutputTokens: 120, CacheReadInputTokens: 400}, time.Second)
	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{InputTokens: 1_500, OutputTokens: 60, CacheReadInputTokens: 600}, time.Second)
	tracker.RecordAuxiliaryUsageForProviderModel("", "", types.Usage{InputTokens: 100, OutputTokens: 5, CacheReadInputTokens: 50})
	tracker.MarkCompaction()

	justCompacted := BuildSessionUsageProjection(tracker)
	if justCompacted.InputTokens != 0 || justCompacted.TotalInputTokens != 2_600 || justCompacted.CacheHitKnown {
		t.Fatalf("just-compacted projection = %+v", justCompacted)
	}

	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{InputTokens: 700, OutputTokens: 60, CacheReadInputTokens: 200}, time.Second)
	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{InputTokens: 900, OutputTokens: 70, CacheReadInputTokens: 450}, time.Second)
	current := BuildSessionUsageProjection(tracker)
	if current.InputTokens != 1_600 || current.TotalInputTokens != 4_200 || current.OutputTokens != 315 || current.CacheReadTokens != 650 || current.CacheHitPercent != 41 || !current.CacheHitKnown {
		t.Fatalf("first compacted segment projection = %+v", current)
	}

	tracker.MarkCompaction()
	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{InputTokens: 600, OutputTokens: 40, CacheReadInputTokens: 300}, time.Second)
	second := BuildSessionUsageProjection(tracker)
	if second.InputTokens != 600 || second.TotalInputTokens != 4_800 || second.OutputTokens != 355 || second.CacheHitPercent != 50 {
		t.Fatalf("second compacted segment projection = %+v", second)
	}
}

func TestSessionProjectionFormatsCatalogBillingCurrency(t *testing.T) {
	tracker := NewCostTracker("deepseek-v4-flash")
	tracker.SetProvider("deepseek")
	tracker.SetCatalog(provider.DefaultCatalog())
	tracker.RecordTurnUsageForProviderModel("deepseek", "deepseek-v4-flash", types.Usage{
		InputTokens: 1_000_000, OutputTokens: 1_000_000,
	}, time.Second)

	projection := BuildSessionUsageProjection(tracker)
	if projection.CostCurrency != "USD" || math.Abs(projection.CostUSD-0.42) > 1e-9 {
		t.Fatalf("projection cost = %.2f %s, want 0.42 USD", projection.CostUSD, projection.CostCurrency)
	}
	if got := FormatSessionUsage(i18n.LangZH, projection); !strings.Contains(got, "$0.4200") || strings.Contains(got, "¥") {
		t.Fatalf("formatted native-currency usage = %q", got)
	}
}

func TestJSONUsageSemanticsKeepsAccountingScopesSeparate(t *testing.T) {
	var output bytes.Buffer
	NewJSONRenderer(&output).UsageSemantics(incidentUsageSnapshot())
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if event["type"] != "usage_summary" || event["schema_version"] != presentation.UsageSemanticsSchemaVersion {
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
		"Session total: in 136.1K · 97% cached · out 146 · $16.6019",
		"Context: 56% (560.0K/1000.0K)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("screen-reader projection missing %q in %q", want, text)
		}
	}
}

func TestUsageSemanticsMakesUnknownLedgerAndContextVisible(t *testing.T) {
	snapshot := BuildUsageSemanticsSnapshot(&types.Usage{InputTokens: 7}, nil, 0, 0)
	if !snapshot.LastRequest.Known || snapshot.CumulativeSession.Known || snapshot.ModelContext.Known {
		t.Fatalf("unknown accounting scopes collapsed to zero: %+v", snapshot)
	}
}

func TestBDDScreenReaderPreservesContextMeasurementMarkers(t *testing.T) {
	var output bytes.Buffer
	renderer := NewScreenReaderRenderer(&output, nil)
	renderer.ModelContext(presentation.ModelContextProjection{
		Scope: presentation.UsageScopeModelContext, Known: true,
		UsedTokens: 90_000, CapacityTokens: 200_000, PercentUsed: 45,
		Measurement: presentation.ContextMeasurementLocalEstimate,
	})
	renderer.ModelContext(presentation.ModelContextProjection{
		Scope: presentation.UsageScopeModelContext, Known: true,
		UsedTokens: 80_000, CapacityTokens: 200_000, PercentUsed: 40,
		Measurement: presentation.ContextMeasurementLocalLowerBound,
	})
	renderer.Close()
	text := output.String()
	if !strings.Contains(text, "Context: ≈45% (90.0K/200.0K)") || !strings.Contains(text, "Context: ≥40% (80.0K/200.0K)") {
		t.Fatalf("screen reader lost context measurement source: %q", text)
	}
}
