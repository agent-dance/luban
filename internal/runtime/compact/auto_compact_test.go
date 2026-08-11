package compact

import (
	"context"
	"fmt"
	"testing"

	"github.com/agent-dance/luban/types"
)

type autoCompactCountingCompactor struct {
	calls      int
	keepRecent int
	err        error
}

func (c *autoCompactCountingCompactor) Compact(_ context.Context, _ []types.Message, keepRecent int) (*CompactionResult, error) {
	c.calls++
	c.keepRecent = keepRecent
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

func TestAutoCompactPassesScopedKeepRecentOverride(t *testing.T) {
	window := NewContextWindow(50_000)
	window.UsedInput = 40_000
	compactor := &autoCompactCountingCompactor{}
	_, attempted, err := AutoCompactIfNeeded(context.Background(), []types.Message{types.UserMessage("old")}, AutoCompactOptions{
		Window: window, Compactor: compactor, KeepRecent: 8,
	})
	if err != nil || !attempted || compactor.keepRecent != 8 {
		t.Fatalf("attempted=%v err=%v keepRecent=%d, want 8", attempted, err, compactor.keepRecent)
	}
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

func TestShouldProgressiveProjectionUsesPreCompactHeadroom(t *testing.T) {
	window := NewContextWindow(80_000)
	window.MaxOutputTokens = 16_384
	// Auto-compact threshold is 50,616; progressive projection opens 8,000
	// tokens earlier so it has roughly two coding turns to release pressure.
	if window.ShouldProgressiveProjection(ModelContextTokenEstimate{KnownTotalTokens: 42_615}) {
		t.Fatal("progressive gate opened before headroom threshold")
	}
	if !window.ShouldProgressiveProjection(ModelContextTokenEstimate{KnownTotalTokens: 42_616}) {
		t.Fatal("progressive gate did not open at headroom threshold")
	}
	window.UpdateUsage(&types.Usage{InputTokens: 50_000})
	if !window.ShouldProgressiveProjection(ModelContextTokenEstimate{KnownTotalTokens: 10_000}) {
		t.Fatal("progressive gate ignored the larger provider-reported context")
	}
	window.UpdatePostCompactUsage(10_000)
	if window.ProviderReportedInputTokens() != 0 || window.ShouldProgressiveProjection(ModelContextTokenEstimate{KnownTotalTokens: 10_000}) {
		t.Fatal("progressive gate retained stale pre-compact provider usage")
	}
	for index := 0; index < MaxConsecutiveAutocompactFailures; index++ {
		window.RecordCompactFailure()
	}
	if window.ShouldProgressiveProjection(ModelContextTokenEstimate{KnownTotalTokens: 60_000}) {
		t.Fatal("progressive gate bypassed compaction circuit breaker")
	}
}

func TestProviderAdjustedInputTokensUsesProviderBaselineAndLocalDelta(t *testing.T) {
	window := NewContextWindow(60_000)
	previous := ModelContextTokenEstimate{KnownTotalTokens: 68_000, Complete: true}
	window.UpdateLocalEstimate(previous)
	window.UpdateUsage(&types.Usage{InputTokens: 34_000, CacheReadInputTokens: 25_000})

	current := ModelContextTokenEstimate{KnownTotalTokens: 70_000, Complete: true}
	if got := window.ProviderAdjustedInputTokens(current); got != 36_000 {
		t.Fatalf("provider-adjusted growth = %d, want 36,000", got)
	}
	projected := ModelContextTokenEstimate{KnownTotalTokens: 55_000, Complete: true}
	if got := window.ProviderAdjustedInputTokens(projected); got != 21_000 {
		t.Fatalf("provider-adjusted projection = %d, want 21,000", got)
	}
	if window.ShouldSnipEstimate(projected) {
		t.Fatal("stale absolute local estimate triggered semantic compaction after projection")
	}
	if !window.ShouldProgressiveProjection(current) {
		t.Fatal("provider-adjusted current request did not open the progressive pressure gate")
	}
}

func TestAutoCompactDecisionCapsOnlyUnverifiedLocalGrowth(t *testing.T) {
	window := NewContextWindow(50_000)
	window.MaxOutputTokens = 16_384
	previous := ModelContextTokenEstimate{KnownTotalTokens: 8_000, Complete: true}
	window.UpdateLocalEstimate(previous)
	window.UpdateUsage(&types.Usage{InputTokens: 8_000, CacheReadInputTokens: 6_000})

	inflated := ModelContextTokenEstimate{KnownTotalTokens: 28_000, Complete: true}
	if got := window.AutoCompactDecisionTokens(inflated, 0); got != 28_000 {
		t.Fatalf("default decision tokens = %d, want unchanged 28,000", got)
	}
	if got := window.AutoCompactDecisionTokens(inflated, 8_000); got != 16_000 {
		t.Fatalf("bounded decision tokens = %d, want provider baseline + 8,000", got)
	}
	if window.ShouldSnipEstimateWithPolicy(inflated, 20_616, 8_000) {
		t.Fatal("uncalibrated local growth triggered provider-scoped compaction")
	}

	window.UpdateUsage(&types.Usage{InputTokens: 25_000, CacheReadInputTokens: 20_000})
	if !window.ShouldSnipEstimateWithPolicy(inflated, 20_616, 8_000) {
		t.Fatal("growth cap hid authoritative provider pressure")
	}
	projected := ModelContextTokenEstimate{KnownTotalTokens: 4_000, Complete: true}
	if got := window.AutoCompactDecisionTokens(projected, 8_000); got >= 25_000 {
		t.Fatalf("projection was raised to provider baseline: %d", got)
	}
}

func TestAutoCompactMinimumThresholdPercentOnlyPostpones(t *testing.T) {
	stress := NewContextWindow(50_000)
	stress.MaxOutputTokens = 16_384
	if got := stress.AutoCompactThreshold(); got != 20_616 {
		t.Fatalf("legacy stress threshold = %d, want 20,616", got)
	}
	if got := stress.AutoCompactThresholdWithMinPercent(90); got != 30_254 {
		t.Fatalf("90%% stress threshold = %d, want 30,254", got)
	}

	production := NewContextWindow(1_048_576)
	production.MaxOutputTokens = 20_000
	if got, want := production.AutoCompactThresholdWithMinPercent(90), production.AutoCompactThreshold(); got != want {
		t.Fatalf("large-window threshold changed from %d to %d", want, got)
	}
}

func TestProviderUsageKnownFailsClosedAcrossFreshAndPostCompactWindows(t *testing.T) {
	window := NewContextWindow(60_000)
	if window.ProviderUsageKnown() {
		t.Fatal("fresh context window claimed a provider cache baseline")
	}
	window.UpdateUsage(&types.Usage{InputTokens: 20_000, CacheReadInputTokens: 15_000})
	if !window.ProviderUsageKnown() {
		t.Fatal("provider usage did not establish a cache baseline")
	}
	window.UpdatePostCompactUsage(5_000)
	if window.ProviderUsageKnown() {
		t.Fatal("post-compact window retained a stale cache baseline")
	}
}

func TestProgressiveThresholdTracksDynamicContextWindow(t *testing.T) {
	window := NewContextWindow(200_000)
	window.MaxOutputTokens = 20_000
	first := window.AutoCompactThreshold()
	window.MaxTokens = 80_000
	second := window.AutoCompactThreshold()
	if first != 167_000 || second != 47_000 || second >= first {
		t.Fatalf("dynamic thresholds = %d then %d", first, second)
	}
}

func findCompactTelemetry(events []CompactionTelemetryEvent, kind CompactionTelemetryKind) *CompactionTelemetryEvent {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}
