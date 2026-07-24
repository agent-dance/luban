package ui

import (
	"math"
	"testing"
	"time"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func requireCostEqual(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", got, want)
	}
}

func TestCostTracker_AccumulateFiveTurns(t *testing.T) {
	// Use a well-known model so CalculateCost returns actual numbers.
	ct := NewCostTracker("claude-opus-4-5")

	// Record 5 turns with varying token counts.
	turns := []struct {
		in, out, cr, cm int
	}{
		{1000, 200, 0, 0},
		{800, 150, 500, 0},
		{1200, 300, 1000, 200},
		{1200, 400, 200, 100},
		{500, 100, 300, 50},
	}

	wantIn, wantOut := 0, 0
	for i, tc := range turns {
		ct.RecordTurn(tc.in, tc.out, tc.cr, tc.cm, time.Duration(i+1)*time.Second)
		wantIn += tc.in
		wantOut += tc.out
	}

	// Turn count
	if got := ct.TurnCount(); got != 5 {
		t.Fatalf("TurnCount = %d, want 5", got)
	}

	// Total tokens
	gotIn, gotOut := ct.TotalTokens()
	if gotIn != wantIn {
		t.Errorf("TotalTokens input = %d, want %d", gotIn, wantIn)
	}
	if gotOut != wantOut {
		t.Errorf("TotalTokens output = %d, want %d", gotOut, wantOut)
	}
	if gotIn, gotOut, gotRead, gotMake := ct.TotalUsage(); gotIn != wantIn || gotOut != wantOut || gotRead != 2000 || gotMake != 350 {
		t.Errorf("TotalUsage = %d/%d/%d/%d, want %d/%d/2000/350", gotIn, gotOut, gotRead, gotMake, wantIn, wantOut)
	}

	// TotalCost must be positive (model is in the pricing table)
	if got := ct.TotalCost(); got <= 0 {
		t.Errorf("TotalCost = %f, want > 0", got)
	}

	// LastTurn should reflect the final recorded turn
	last := ct.LastTurn()
	if last == nil {
		t.Fatal("LastTurn returned nil")
	}
	if last.InputTokens != 500 {
		t.Errorf("LastTurn.InputTokens = %d, want 500", last.InputTokens)
	}
	if last.Duration != 5*time.Second {
		t.Errorf("LastTurn.Duration = %v, want 5s", last.Duration)
	}
}

func TestCostTracker_EmptyTracker(t *testing.T) {
	ct := NewCostTracker("claude-sonnet-4-5")

	if ct.TurnCount() != 0 {
		t.Errorf("expected 0 turns, got %d", ct.TurnCount())
	}
	if ct.TotalCost() != 0 {
		t.Errorf("expected 0 cost, got %f", ct.TotalCost())
	}
	if ct.LastTurn() != nil {
		t.Error("expected nil LastTurn on empty tracker")
	}
	in, out := ct.TotalTokens()
	if in != 0 || out != 0 {
		t.Errorf("expected 0/0 tokens, got %d/%d", in, out)
	}
}

func TestCostTracker_UnknownModel(t *testing.T) {
	ct := NewCostTracker("unknown-model-xyz")
	ct.RecordTurn(1000, 500, 0, 0, time.Second)

	// Should still record the turn even if cost is 0 (unknown model)
	if ct.TurnCount() != 1 {
		t.Fatalf("TurnCount = %d, want 1", ct.TurnCount())
	}
	if ct.TotalCost() != 0 {
		t.Errorf("unknown model cost = %f, want zero numeric placeholder", ct.TotalCost())
	}
	if ct.CostKnown() {
		t.Fatal("unknown model usage was presented as a known zero cost")
	}
	if _, complete := ct.TotalCostBreakdown(); complete {
		t.Fatal("unknown model usage produced a supposedly complete cost breakdown")
	}
}

func TestCostTrackerRestorePreservesUnknownCostAcrossKnownUsage(t *testing.T) {
	ct := NewCostTracker("claude-opus-4-5")
	ct.RestoreSession("claude-opus-4-5", 1000, 200, 0, 0, 0, 0, false)
	ct.RecordTurn(100, 20, 0, 0, time.Second)

	if ct.CostKnown() {
		t.Fatal("known pricing after restore erased an earlier unknown session cost")
	}
}

func TestCostTracker_Reset(t *testing.T) {
	ct := NewCostTracker("claude-opus-4-5")
	ct.RecordTurn(1000, 200, 300, 50, time.Second)

	ct.Reset("claude-sonnet-4-5")

	if got := ct.Model(); got != "claude-sonnet-4-5" {
		t.Fatalf("Model = %q, want claude-sonnet-4-5", got)
	}
	if got := ct.TurnCount(); got != 0 {
		t.Fatalf("TurnCount = %d, want 0", got)
	}
	if got := ct.TotalCost(); got != 0 {
		t.Fatalf("TotalCost = %f, want 0", got)
	}
	if gotIn, gotOut := ct.TotalTokens(); gotIn != 0 || gotOut != 0 {
		t.Fatalf("TotalTokens = %d/%d, want 0/0", gotIn, gotOut)
	}
	if gotRead, gotMake := ct.TotalCacheTokens(); gotRead != 0 || gotMake != 0 {
		t.Fatalf("TotalCacheTokens = %d/%d, want 0/0", gotRead, gotMake)
	}
	if last := ct.LastTurn(); last != nil {
		t.Fatalf("LastTurn = %+v, want nil", last)
	}
}

func TestCostTrackerRestoreSessionContinuesCumulativeTotals(t *testing.T) {
	ct := NewCostTracker("claude-opus-4-5")
	ct.RestoreSession("claude-opus-4-5", 100, 20, 30, 4, 2, 1.25)
	ct.RecordTurnUsage(types.Usage{InputTokens: 7, OutputTokens: 3, CacheReadInputTokens: 2, CacheCreationInputTokens: 1}, time.Second)
	if input, output, read, make := ct.TotalUsage(); input != 107 || output != 23 || read != 32 || make != 5 {
		t.Fatalf("restored totals after turn = %d/%d/%d/%d", input, output, read, make)
	}
	if ct.TotalWebSearchRequests() != 2 {
		t.Fatalf("web search total = %d, want 2", ct.TotalWebSearchRequests())
	}
	if ct.TotalCost() < 1.25 {
		t.Fatalf("cost regressed below restored total: %f", ct.TotalCost())
	}
}

func TestCostTrackerCompactionBaselineTracksLatestSuccessfulBoundary(t *testing.T) {
	ct := NewCostTracker("claude-opus-4-5")
	ct.RecordTurnUsage(types.Usage{InputTokens: 100, CacheReadInputTokens: 30}, time.Second)
	ct.MarkCompaction()
	ct.RecordAuxiliaryUsage(types.Usage{InputTokens: 7, CacheReadInputTokens: 2})

	hasCompacted, input, cacheRead := ct.CompactionBaseline()
	if !hasCompacted || input != 100 || cacheRead != 30 {
		t.Fatalf("compaction baseline = %t/%d/%d, want true/100/30", hasCompacted, input, cacheRead)
	}

	ct.RestoreSession("claude-opus-4-5", 200, 20, 80, 0, 0, 1.25)
	ct.RestoreCompactionBaseline(true, 150, 60)
	if hasCompacted, input, cacheRead = ct.CompactionBaseline(); !hasCompacted || input != 150 || cacheRead != 60 {
		t.Fatalf("restored compaction baseline = %t/%d/%d, want true/150/60", hasCompacted, input, cacheRead)
	}

	ct.RestoreCompactionBaseline(true, 999, 999)
	if _, input, cacheRead = ct.CompactionBaseline(); input != 200 || cacheRead != 80 {
		t.Fatalf("clamped compaction baseline = %d/%d, want 200/80", input, cacheRead)
	}
}

func TestCostTrackerConversationUsageAccumulatesRoundFinalRequests(t *testing.T) {
	ct := NewCostTracker("claude-opus-4-5")
	ct.RecordTurnUsage(types.Usage{InputTokens: 1000, OutputTokens: 120, CacheReadInputTokens: 400}, time.Second)
	ct.RecordTurnUsage(types.Usage{InputTokens: 1500, OutputTokens: 80, CacheReadInputTokens: 600}, time.Second)
	ct.MarkCompaction()

	ct.RecordAuxiliaryUsage(types.Usage{InputTokens: 7000, OutputTokens: 900})
	ct.RecordTurnUsage(types.Usage{InputTokens: 700, OutputTokens: 60, CacheReadInputTokens: 200}, time.Second)
	ct.RecordTurnUsage(types.Usage{InputTokens: 900, OutputTokens: 70, CacheReadInputTokens: 450}, time.Second)
	ct.MarkCompaction()
	ct.RecordTurnUsage(types.Usage{InputTokens: 600, OutputTokens: 40, CacheReadInputTokens: 300}, time.Second)

	got := ct.ConversationUsage()
	if got.CompactionCount != 2 || got.CompletedInputTokens != 2400 || got.CompletedOutputTokens != 150 {
		t.Fatalf("completed round usage = %+v, want count/input/output 2/2400/150", got)
	}
	if got.LastInputTokens != 600 || got.LastOutputTokens != 40 || got.LastCacheReadTokens != 300 {
		t.Fatalf("current round usage = %+v, want input/output/cache 600/40/300", got)
	}
	if input, output, _, _ := ct.TotalUsage(); input != 11700 || output != 1270 {
		t.Fatalf("full usage ledger = %d/%d, want 11700/1270", input, output)
	}
}

func TestCostTrackerDoesNotClaimLegacyCompactedRoundUsageIsExact(t *testing.T) {
	ct := NewCostTracker("claude-opus-4-5")
	ct.RestoreSession("claude-opus-4-5", 2500, 200, 1000, 0, 0, 1.25)
	ct.RestoreCompactionBaseline(true, 1800, 700)
	ct.RestoreConversationUsage(ConversationUsage{LastInputTokens: 700, LastOutputTokens: 80, LastCacheReadTokens: 300})

	ct.RecordTurnUsage(types.Usage{InputTokens: 900, OutputTokens: 70, CacheReadInputTokens: 450}, time.Second)

	if usage := ct.ConversationUsage(); usage.Known {
		t.Fatalf("legacy compacted endpoint history was presented as exact: %+v", usage)
	}
}

func TestCostTrackerRecordAuxiliaryUsageOnlyChangesSessionTotals(t *testing.T) {
	ct := NewCostTracker("claude-opus-4-5")
	ct.RecordTurnUsage(types.Usage{
		InputTokens: 100, OutputTokens: 20, CacheReadInputTokens: 30, CacheCreationInputTokens: 4,
	}, 2*time.Second)
	before := ct.LastTurn()
	beforeCost := ct.TotalCost()

	ct.RecordAuxiliaryUsage(types.Usage{
		InputTokens: 7, OutputTokens: 3, CacheReadInputTokens: 2, CacheCreationInputTokens: 1,
	})

	if input, output, read, created := ct.TotalUsage(); input != 107 || output != 23 || read != 32 || created != 5 {
		t.Fatalf("TotalUsage = %d/%d/%d/%d, want 107/23/32/5", input, output, read, created)
	}
	if ct.TotalCost() <= beforeCost {
		t.Fatalf("auxiliary usage did not increase cost: before=%f after=%f", beforeCost, ct.TotalCost())
	}
	if ct.TurnCount() != 1 {
		t.Fatalf("TurnCount = %d, want 1", ct.TurnCount())
	}
	if last := ct.LastTurn(); last == nil || before == nil || *last != *before {
		t.Fatalf("LastTurn changed: before=%+v after=%+v", before, last)
	}
	entries := ct.PerModelCosts()
	if len(entries) != 1 || entries[0].InputTokens != 107 || entries[0].OutputTokens != 23 || entries[0].TurnCount != 1 {
		t.Fatalf("PerModelCosts = %+v", entries)
	}
}

func TestCostTrackerRecordAuxiliaryUsageForModelDoesNotChangeConversationModel(t *testing.T) {
	ct := NewCostTracker("conversation-model")
	ct.RecordTurnUsage(types.Usage{InputTokens: 100, OutputTokens: 20}, time.Second)

	ct.RecordAuxiliaryUsageForModel("evaluator-model", types.Usage{InputTokens: 7, OutputTokens: 3})

	if got := ct.Model(); got != "conversation-model" {
		t.Fatalf("Model = %q, want conversation model unchanged", got)
	}
	entries := ct.PerModelCosts()
	if len(entries) != 2 {
		t.Fatalf("PerModelCosts = %+v, want two model entries", entries)
	}
	byModel := make(map[string]ModelCostEntry, len(entries))
	for _, entry := range entries {
		byModel[entry.Model] = entry
	}
	if got := byModel["conversation-model"]; got.InputTokens != 100 || got.OutputTokens != 20 || got.TurnCount != 1 {
		t.Fatalf("conversation model costs = %+v", got)
	}
	if got := byModel["evaluator-model"]; got.InputTokens != 7 || got.OutputTokens != 3 || got.TurnCount != 0 {
		t.Fatalf("evaluator model costs = %+v", got)
	}
}

func TestCostTrackerUsesProviderSpecificCatalogPricing(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{ID: "shared-model", Provider: "alpha", CostPer1MIn: 1, CostPer1MOut: 2})
	catalog.Register(provider.ModelInfo{ID: "shared-model", Provider: "omega", CostPer1MIn: 9, CostPer1MOut: 11})

	ct := NewCostTracker("shared-model")
	ct.SetCatalog(catalog)
	ct.SetProvider("omega")
	ct.RecordTurn(1_000_000, 1_000_000, 0, 0, time.Second)

	requireCostEqual(t, ct.TotalCost(), 20)
}

func TestCostTrackerPricesTurnWithEventProviderAndModelWithoutChangingActiveIdentity(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{ID: "primary-model", Provider: "priced", CostPer1MIn: 1, CostPer1MOut: 2})
	catalog.Register(provider.ModelInfo{ID: "fallback-model", Provider: "priced", CostPer1MIn: 9, CostPer1MOut: 11})

	ct := NewCostTracker("primary-model")
	ct.SetCatalog(catalog)
	ct.SetProvider("priced")
	ct.RecordTurnUsageForProviderModel("priced", "fallback-model", types.Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}, time.Second)

	requireCostEqual(t, ct.TotalCost(), 20)
	if ct.Provider() != "priced" || ct.Model() != "primary-model" {
		t.Fatalf("event identity changed active provider/model to %q/%q", ct.Provider(), ct.Model())
	}
}

func TestCostTrackerResolvesGroqModelIDContainingSlash(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{
		ID: "openai/gpt-oss-120b", Provider: "groq", CostPer1MIn: 0.15, CostPer1MOut: 0.60,
	})

	ct := NewCostTracker("openai/gpt-oss-120b")
	ct.SetCatalog(catalog)
	ct.SetProvider("groq")
	ct.RecordTurn(1_000_000, 1_000_000, 0, 0, time.Second)

	requireCostEqual(t, ct.TotalCost(), 0.75)
}

func TestCostTrackerCatalogChargesCacheReadAndCreationAtTheirRates(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{
		ID: "cached-model", Provider: "priced",
		CostPer1MIn: 2, CostPer1MOut: 4, CacheReadPer1M: 0.5, CacheCreatePer1M: 3,
	})

	ct := NewCostTracker("cached-model")
	ct.SetCatalog(catalog)
	ct.SetProvider("priced")
	ct.RecordTurn(1_000_000, 250_000, 200_000, 300_000, time.Second)

	// 500K uncached input * $2/M + 200K reads * $0.50/M +
	// 300K writes * $3/M + 250K output * $4/M.
	requireCostEqual(t, ct.TotalCost(), 3)
}

func TestCostTrackerCatalogFallsBackToInputRateForMissingCacheRates(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{
		ID: "implicit-cache-model", Provider: "priced", CostPer1MIn: 2, CostPer1MOut: 4,
	})

	ct := NewCostTracker("implicit-cache-model")
	ct.SetCatalog(catalog)
	ct.SetProvider("priced")
	ct.RecordTurn(1_000_000, 0, 200_000, 300_000, time.Second)

	// Missing cache rates must not silently make cached input free. All input
	// buckets fall back to the regular $2/M input rate.
	requireCostEqual(t, ct.TotalCost(), 2)
}

func TestCostTrackerStaticPricingFallsBackToInputRateForMissingCacheRate(t *testing.T) {
	ct := NewCostTracker("gpt-5.5")
	ct.RecordTurn(1_000_000, 0, 200_000, 400_000, time.Second)

	// GPT-5.5 has a known $0.50/M cache-read rate but no separate cache-write
	// rate in the static table. Writes therefore remain normal $5/M input.
	requireCostEqual(t, ct.TotalCost(), 4.10)
}

func TestCostTrackerAccumulatesExactCatalogCostAcrossTurns(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{
		ID: "multi-turn", Provider: "priced",
		CostPer1MIn: 2, CostPer1MOut: 4, CacheReadPer1M: 0.5, CacheCreatePer1M: 3,
	})

	ct := NewCostTracker("multi-turn")
	ct.SetCatalog(catalog)
	ct.SetProvider("priced")
	ct.RecordTurn(800_000, 100_000, 200_000, 100_000, time.Second)
	ct.RecordTurn(1_200_000, 300_000, 600_000, 200_000, 2*time.Second)

	// Turn 1: 500K*$2 + 200K*$0.50 + 100K*$3 + 100K*$4 = $1.80.
	// Turn 2: 400K*$2 + 600K*$0.50 + 200K*$3 + 300K*$4 = $2.90.
	requireCostEqual(t, ct.TotalCost(), 4.70)
	breakdown, complete := ct.TotalCostBreakdown()
	if !complete {
		t.Fatal("fresh-session cost breakdown is incomplete")
	}
	requireCostEqual(t, breakdown.InputUSD, 1.80)
	requireCostEqual(t, breakdown.OutputUSD, 1.60)
	requireCostEqual(t, breakdown.CacheReadUSD, 0.40)
	requireCostEqual(t, breakdown.CacheCreationUSD, 0.90)
	requireCostEqual(t, breakdown.TotalUSD, 4.70)
}

func TestCostTrackerRestoreSessionAddsExactProviderPricedDelta(t *testing.T) {
	catalog := provider.NewModelCatalog()
	catalog.Register(provider.ModelInfo{
		ID: "resumed", Provider: "priced",
		CostPer1MIn: 2, CostPer1MOut: 4, CacheReadPer1M: 0.5, CacheCreatePer1M: 3,
	})

	ct := NewCostTracker("resumed")
	ct.SetCatalog(catalog)
	ct.SetProvider("priced")
	ct.RestoreSession("resumed", 9_000_000, 1_000_000, 2_000_000, 500_000, 0, 7.25)
	ct.RecordTurn(1_000_000, 250_000, 200_000, 300_000, time.Second)

	requireCostEqual(t, ct.TotalCost(), 10.25)
	if _, complete := ct.TotalCostBreakdown(); complete {
		t.Fatal("restored aggregate cost must not invent a historical bucket breakdown")
	}
	if input, output, read, created := ct.TotalUsage(); input != 10_000_000 || output != 1_250_000 || read != 2_200_000 || created != 800_000 {
		t.Fatalf("TotalUsage = %d/%d/%d/%d", input, output, read, created)
	}
}
