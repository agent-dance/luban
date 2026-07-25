package ui

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func renderedSession(t *testing.T, tracker *CostTracker) string {
	t.Helper()
	return FormatSessionUsage(i18n.LangEN, BuildSessionUsageProjection(tracker))
}

func TestBDDUncompactedSessionAccumulatesEveryRequest(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	recordTurn(tracker, 1_000, 40, 300, 0, time.Second)
	recordTurn(tracker, 1_500, 60, 700, 0, time.Second)

	got := renderedSession(t, tracker)
	if !strings.Contains(got, "Session: in 2.5K") {
		t.Fatalf("Then session input is cumulative: %q", got)
	}
	if strings.Contains(got, "Req:") || strings.Contains(got, "Last request") {
		t.Fatalf("Then last-request scope is absent from the session segment: %q", got)
	}
}

func TestBDDCompactedSessionAccumulatesCurrentSegment(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	recordTurn(tracker, 1_000, 120, 400, 0, time.Second)
	recordTurn(tracker, 1_500, 60, 600, 0, time.Second)
	tracker.MarkCompactionBoundary("boundary-1")
	recordTurn(tracker, 700, 60, 200, 0, time.Second)
	recordTurn(tracker, 900, 70, 450, 0, time.Second)

	got := renderedSession(t, tracker)
	if !strings.Contains(got, "in 1.6K (4.1K total)") || strings.Contains(got, "in 900") {
		t.Fatalf("Then input is the compact segment/session pair, not the last request: %q", got)
	}
	if !strings.Contains(got, "41% cached") {
		t.Fatalf("Then cache uses 650/1600 from the same segment: %q", got)
	}

	tracker.MarkCompactionBoundary("boundary-2")
	recordTurn(tracker, 600, 40, 300, 0, time.Second)
	got = renderedSession(t, tracker)
	if !strings.Contains(got, "in 600 (4.7K total)") {
		t.Fatalf("Then the second successful compact replaces the baseline: %q", got)
	}
}

func TestBDDCompactionCallBelongsToSessionButNotNewSegment(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	recordTurn(tracker, 2_500, 180, 1_000, 0, time.Second)
	tracker.RecordAuxiliaryUsageForProviderModel("", "", types.Usage{InputTokens: 500, OutputTokens: 20, CacheReadInputTokens: 200})
	tracker.MarkCompactionBoundary("boundary-after-billed-call")
	recordTurn(tracker, 600, 40, 300, 0, time.Second)

	projection := BuildSessionUsageProjection(tracker)
	if projection.InputTokens != 600 || projection.TotalInputTokens != 3_600 || projection.OutputTokens != 240 {
		t.Fatalf("Then compact usage is before the new baseline: %+v", projection)
	}
}

func TestBDDOutputAccumulatesAcrossCompaction(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	recordTurn(tracker, 2_500, 180, 1_000, 0, time.Second)
	tracker.MarkCompactionBoundary("boundary")
	recordTurn(tracker, 700, 60, 200, 0, time.Second)
	recordTurn(tracker, 900, 70, 450, 0, time.Second)

	got := renderedSession(t, tracker)
	if !strings.Contains(got, "out 310") || strings.Contains(got, "out 70") {
		t.Fatalf("Then output remains session-cumulative: %q", got)
	}
}

func TestBDDCacheRateIsUnavailableForEmptyCompactSegment(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	recordTurn(tracker, 2_500, 180, 1_000, 0, time.Second)
	tracker.MarkCompactionBoundary("boundary")

	got := renderedSession(t, tracker)
	if strings.Contains(got, "0% cached") || strings.Contains(got, "% cached") {
		t.Fatalf("Then an empty segment does not fabricate a cache rate: %q", got)
	}
}

func TestBDDCostAccumulatesAcrossCompactionAndUnknownPricingWins(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{CostCurrency: "USD", Provider: "priced", ID: "fixture", CostPer1MIn: 1, CostPer1MOut: 2})
	tracker := NewCostTracker("fixture")
	tracker.SetCatalog(catalog)
	tracker.SetProvider("priced")
	tracker.RestoreSession("fixture", 0, 0, 0, 0, 0, 0.0300, true)
	tracker.RecordAuxiliaryUsageForProviderModel("priced", "fixture", types.Usage{InputTokens: 5_000})
	tracker.MarkCompactionBoundary("boundary")
	tracker.RecordTurnUsageForProviderModel("priced", "fixture", types.Usage{InputTokens: 20_000}, time.Second)
	if got := renderedSession(t, tracker); !strings.Contains(got, "$0.0550") {
		t.Fatalf("Then compact and post-compact costs remain cumulative: %q", got)
	}

	tracker.RecordAuxiliaryUsageForProviderModel("unknown", "unpriced", types.Usage{InputTokens: 1})
	got := renderedSession(t, tracker)
	if !strings.Contains(got, "cost unknown") || strings.Contains(got, "$0.0000") {
		t.Fatalf("Then any non-zero unpriced usage makes session cost unknown: %q", got)
	}
}

func TestBDDNonzeroUsageWithMissingPriceMakesCostUnknown(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{CostCurrency: "USD", Provider: "partial", ID: "fixture", CostPer1MIn: 1})
	tracker := NewCostTracker("fixture")
	tracker.SetCatalog(catalog)
	tracker.SetProvider("partial")
	tracker.RecordTurnUsageForProviderModel("partial", "fixture", types.Usage{InputTokens: 1_000, OutputTokens: 100}, time.Second)

	projection := BuildSessionUsageProjection(tracker)
	if projection.CostKnown || !strings.Contains(renderedSession(t, tracker), "cost unknown") {
		t.Fatalf("Then a missing output price cannot masquerade as a known partial cost: %+v", projection)
	}
}

func TestBDDRetryUsageIsSettledExactlyOnce(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	first := types.Usage{InputTokens: 1_000, OutputTokens: 100}
	second := types.Usage{InputTokens: 1_500, OutputTokens: 200}
	if !tracker.RecordAuxiliaryUsageOnceForProviderModel("request:first", "", "", first) {
		t.Fatal("Given the discarded billed attempt was not recorded")
	}
	if tracker.RecordAuxiliaryUsageOnceForProviderModel("request:first", "", "", first) {
		t.Fatal("Then duplicate delivery recorded the discarded attempt twice")
	}
	if !tracker.RecordTurnUsageOnceForProviderModel("request:second", "", "", second, time.Second) {
		t.Fatal("Given the successful retry was not recorded")
	}
	snapshot := tracker.Snapshot()
	if snapshot.SessionInput != 2_500 || snapshot.SessionOutput != 300 {
		t.Fatalf("Then both actual calls are included exactly once: %+v", snapshot)
	}
}

func TestBDDCompactionBoundaryIsExactlyOnce(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	recordTurn(tracker, 2_500, 100, 1_000, 0, time.Second)
	if !tracker.MarkCompactionBoundary("boundary-B") {
		t.Fatal("Given boundary B was not applied")
	}
	recordTurn(tracker, 700, 50, 300, 0, time.Second)
	if tracker.MarkCompactionBoundary("boundary-B") {
		t.Fatal("When duplicate boundary B was applied again")
	}
	projection := BuildSessionUsageProjection(tracker)
	if projection.InputTokens != 700 {
		t.Fatalf("Then the current segment remains 700: %+v", projection)
	}
}

func TestBDDResumeKnownAndLegacyCompactionBaselines(t *testing.T) {
	t.Run("known baseline continues", func(t *testing.T) {
		tracker := NewCostTracker("claude-opus-4-5")
		tracker.RestoreSession("claude-opus-4-5", 4_100, 310, 1_650, 0, 0, 0.1, true)
		tracker.RestoreCompactionBaselineState(true, true, 2_500, 1_000)
		recordTurn(tracker, 600, 40, 300, 0, time.Second)
		if got := renderedSession(t, tracker); !strings.Contains(got, "in 2.2K (4.7K total)") {
			t.Fatalf("Then resume continues from the same baseline: %q", got)
		}
	})

	t.Run("legacy baseline is unknown", func(t *testing.T) {
		tracker := NewCostTracker("claude-opus-4-5")
		tracker.RestoreSession("claude-opus-4-5", 4_100, 310, 1_650, 0, 0, 0.1, true)
		tracker.RestoreCompactionBaselineState(true, false, 0, 0)
		got := renderedSession(t, tracker)
		if !strings.Contains(got, "Session: in 4.1K") || strings.Contains(got, "0 (4.1K total)") {
			t.Fatalf("Then legacy data shows only the reliable session total: %q", got)
		}
	})
}

func TestBDDConcurrentUsageSnapshotsNeverTear(t *testing.T) {
	tracker := NewCostTracker("claude-opus-4-5")
	var writers sync.WaitGroup
	for writer := 0; writer < 4; writer++ {
		writer := writer
		writers.Add(1)
		go func() {
			defer writers.Done()
			for index := 0; index < 400; index++ {
				tracker.RecordAuxiliaryUsageOnceForProviderModel(
					fmt.Sprintf("writer:%d:%d", writer, index), "", "",
					types.Usage{InputTokens: 10, OutputTokens: 2, CacheReadInputTokens: 4},
				)
				if index%67 == 0 {
					tracker.MarkCompactionBoundary(fmt.Sprintf("boundary:%d:%d", writer, index))
				}
			}
		}()
	}

	for index := 0; index < 2_000; index++ {
		snapshot := tracker.Snapshot()
		if snapshot.SessionCacheRead > snapshot.SessionInput {
			t.Fatalf("Then cache never exceeds its corresponding input: %+v", snapshot)
		}
		if snapshot.CompactionBaselineKnown {
			segmentInput := snapshot.SessionInput - snapshot.InputAtCompact
			segmentCache := snapshot.SessionCacheRead - snapshot.CacheReadAtCompact
			if segmentInput < 0 || segmentCache < 0 || segmentCache > segmentInput {
				t.Fatalf("Then totals and baselines come from one revision: %+v", snapshot)
			}
		}
	}
	writers.Wait()
}
