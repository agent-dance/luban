package report

import (
	"math"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestAggregateMetricDataKeepsTransportAndInferenceUniversesSeparate(t *testing.T) {
	runs := []RunData{
		{Metrics: MetricData{
			TransportAttempts: pointerInt(2), PrewarmAttempts: pointerInt(1), PrewarmErrors: pointerInt(0),
			LLMCallsStarted: pointerInt(1), HTTPInferenceRequests: pointerInt(1), WebSocketInferenceRequests: pointerInt(0), WebSocketConnections: pointerInt(1),
			PrewarmUsageObservations: pointerInt(1), PrewarmInputTokens: pointerInt64(20), PrewarmCachedInputTokens: pointerInt64(10), PrewarmOutputTokens: pointerInt64(2), PrewarmUnknownCostAttempts: pointerInt(0),
			AllExecutedInputTokens: pointerInt64(100), AllExecutedCachedTokens: pointerInt64(40), AllExecutedUncachedTokens: pointerInt64(55), AllExecutedNonCachedBaseTokens: pointerInt64(60), AllExecutedOutputTokens: pointerInt64(20), AllExecutedCacheWriteInputTokens: pointerInt64(5),
			AllExecutedUsageObserved: 2, AllExecutedUsageTotal: 2, AllExecutedCacheWriteObserved: 2, AllExecutedCacheWriteTotal: 2,
			ProviderRequests: pointerInt(1), ProviderRounds: pointerInt(1), ProviderErrors: pointerInt(0),
			RequestCacheHits: pointerInt(1), RequestCacheObserved: pointerInt(2),
			ProviderReportedCost: pointerFloat(0.20), ProviderCostObserved: 2, ProviderCostTotal: 2,
			CatalogCost: pointerFloat(0.30), CostReceiptObserved: 2, CostReceiptTotal: 2,
			UnknownCostAttempts: pointerInt(0), CostIdentityUnknownAttempts: pointerInt(0), KnownCatalogCostLowerBound: pointerFloat(0.30),
		}},
		{Metrics: MetricData{
			TransportAttempts: pointerInt(1), PrewarmAttempts: pointerInt(0), PrewarmErrors: pointerInt(0),
			LLMCallsStarted: pointerInt(1), HTTPInferenceRequests: pointerInt(0), WebSocketInferenceRequests: pointerInt(1), WebSocketConnections: pointerInt(1),
			PrewarmUsageObservations: pointerInt(0), PrewarmInputTokens: pointerInt64(0), PrewarmCachedInputTokens: pointerInt64(0), PrewarmOutputTokens: pointerInt64(0), PrewarmUnknownCostAttempts: pointerInt(0),
			AllExecutedInputTokens: pointerInt64(50), AllExecutedCachedTokens: pointerInt64(10), AllExecutedUncachedTokens: pointerInt64(37), AllExecutedNonCachedBaseTokens: pointerInt64(40), AllExecutedOutputTokens: pointerInt64(8), AllExecutedCacheWriteInputTokens: pointerInt64(3),
			AllExecutedUsageObserved: 1, AllExecutedUsageTotal: 1, AllExecutedCacheWriteObserved: 1, AllExecutedCacheWriteTotal: 1,
			ProviderRequests: pointerInt(1), ProviderRounds: pointerInt(1), ProviderErrors: pointerInt(0),
			RequestCacheHits: pointerInt(0), RequestCacheObserved: pointerInt(1),
			ProviderReportedCost: pointerFloat(0.10), ProviderCostObserved: 1, ProviderCostTotal: 1,
			CatalogCost: pointerFloat(0.15), CostReceiptObserved: 1, CostReceiptTotal: 1,
			UnknownCostAttempts: pointerInt(0), CostIdentityUnknownAttempts: pointerInt(0), KnownCatalogCostLowerBound: pointerFloat(0.15),
		}},
	}

	got := aggregateMetricData(runs)
	assertIntMetric(t, "transport attempts", got.TransportAttempts, 3)
	assertIntMetric(t, "inference requests", got.ProviderRequests, 2)
	assertIntMetric(t, "all-started generate=true inference LLM calls", got.LLMCallsStarted, 2)
	assertIntMetric(t, "prewarm attempts", got.PrewarmAttempts, 1)
	assertIntMetric(t, "HTTP inference requests", got.HTTPInferenceRequests, 1)
	assertIntMetric(t, "WebSocket inference requests", got.WebSocketInferenceRequests, 1)
	assertIntMetric(t, "per-run WebSocket connections", got.WebSocketConnections, 2)
	assertInt64Metric(t, "all-executed cache-write tokens", got.AllExecutedCacheWriteInputTokens, 8)
	assertInt64Metric(t, "all-executed ordinary uncached tokens", got.AllExecutedUncachedTokens, 92)
	assertInt64Metric(t, "all-executed non-cached base tokens", got.AllExecutedNonCachedBaseTokens, 100)
	if got.AllExecutedUsageObserved != 3 || got.AllExecutedUsageTotal != 3 || got.AllExecutedCacheWriteObserved != 3 || got.AllExecutedCacheWriteTotal != 3 {
		t.Fatalf("all-executed coverage = usage %d/%d cache-write %d/%d, want 3/3 and 3/3", got.AllExecutedUsageObserved, got.AllExecutedUsageTotal, got.AllExecutedCacheWriteObserved, got.AllExecutedCacheWriteTotal)
	}
	assertFloatMetric(t, "request cache hit", got.RequestCacheHit, 1.0/3.0)
	assertFloatMetric(t, "all-executed token cache hit", got.AllExecutedCacheHit, 1.0/3.0)
	assertFloatMetric(t, "provider-reported cost", got.ProviderReportedCost, 0.30)
	if got.ProviderCostPartial != nil {
		t.Fatalf("provider partial cost = %v, want nil for complete all-transport receipts", *got.ProviderCostPartial)
	}
}

func TestApplyAllExecutedTokenPartitionSeparatesCacheWriteFromOrdinaryUncached(t *testing.T) {
	complete := harness.UsageMetrics{
		AllExecutedUsageObservations:      2,
		AllExecutedInputTokens:            100,
		AllExecutedCachedInputTokens:      40,
		AllExecutedCacheWriteInputTokens:  5,
		AllExecutedCacheWriteObservations: 2,
		AllExecutedOutputTokens:           20,
	}
	var got MetricData
	if err := applyAllExecutedTokenPartition(&got, complete); err != nil {
		t.Fatalf("apply complete partition: %v", err)
	}
	assertInt64Metric(t, "input", got.AllExecutedInputTokens, 100)
	assertInt64Metric(t, "cached", got.AllExecutedCachedTokens, 40)
	assertInt64Metric(t, "cache write", got.AllExecutedCacheWriteInputTokens, 5)
	assertInt64Metric(t, "ordinary uncached", got.AllExecutedUncachedTokens, 55)
	assertInt64Metric(t, "non-cached base", got.AllExecutedNonCachedBaseTokens, 60)
	if *got.AllExecutedInputTokens != *got.AllExecutedCachedTokens+*got.AllExecutedCacheWriteInputTokens+*got.AllExecutedUncachedTokens {
		t.Fatal("complete token partition does not satisfy I=C+W+U")
	}

	incomplete := complete
	incomplete.AllExecutedCacheWriteObservations = 1
	got = MetricData{}
	if err := applyAllExecutedTokenPartition(&got, incomplete); err != nil {
		t.Fatalf("apply incomplete partition: %v", err)
	}
	if got.AllExecutedUncachedTokens != nil {
		t.Fatalf("ordinary uncached = %d, want unknown without complete cache-write coverage", *got.AllExecutedUncachedTokens)
	}
	assertInt64Metric(t, "incomplete non-cached base", got.AllExecutedNonCachedBaseTokens, 60)

	invalid := complete
	invalid.AllExecutedCacheWriteInputTokens = 61
	if err := applyAllExecutedTokenPartition(&MetricData{}, invalid); err == nil {
		t.Fatal("invalid I=C+W+U partition was accepted")
	}
}

func TestRunMetricUsesAllTransportTokenUniverse(t *testing.T) {
	run := RunData{Metrics: MetricData{
		InputTokens: pointerInt64(80), CachedInputTokens: pointerInt64(30), CacheWriteInputTokens: pointerInt64(2), OutputTokens: pointerInt64(10),
		AllExecutedInputTokens: pointerInt64(100), AllExecutedCachedTokens: pointerInt64(40), AllExecutedCacheWriteInputTokens: pointerInt64(5), AllExecutedUncachedTokens: pointerInt64(55), AllExecutedOutputTokens: pointerInt64(20),
	}}
	checks := map[ComparisonMetric]float64{
		MetricInputTokens: 100, MetricCachedInputTokens: 40, MetricCacheWriteInputTokens: 5,
		MetricUncachedInputTokens: 55, MetricOutputTokens: 20,
	}
	for metric, want := range checks {
		got, observed := runMetric(run, metric)
		if !observed || got != want {
			t.Errorf("runMetric(%s) = %v, %t; want %v, true", metric, got, observed, want)
		}
	}
}

func TestEligibleRequestCacheObservationUsesDocumentedThreshold(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    int64
		cached   int64
		usage    bool
		hit      bool
		eligible bool
	}{
		{name: "below threshold", input: 1023, cached: 512, usage: true},
		{name: "eligible miss", input: 1024, cached: 0, usage: true, eligible: true},
		{name: "eligible hit", input: 1024, cached: 128, usage: true, hit: true, eligible: true},
		{name: "no usage receipt", input: 2048, cached: 128},
	} {
		t.Run(test.name, func(t *testing.T) {
			round := harness.ProviderRoundEvidence{UsagePresent: test.usage, InputTokens: &test.input, CachedInputTokens: &test.cached}
			hit, eligible := eligibleRequestCacheObservation(round)
			if hit != test.hit || eligible != test.eligible {
				t.Fatalf("observation = hit %t eligible %t, want %t/%t", hit, eligible, test.hit, test.eligible)
			}
		})
	}
}

func TestAggregateMetricDataKeepsEligibleRequestHitDiagnosticSeparateFromAllTransport(t *testing.T) {
	run := RunData{Metrics: MetricData{
		TransportAttempts: pointerInt(3), ProviderRequests: pointerInt(2),
		RequestCacheHits: pointerInt(1), RequestCacheObserved: pointerInt(2),
		AllExecutedInputTokens: pointerInt64(100), AllExecutedCachedTokens: pointerInt64(50),
		AllExecutedUsageObserved: 2, AllExecutedUsageTotal: 3,
		ProviderCostPartial: pointerFloat(0.40), ProviderCostObserved: 2, ProviderCostTotal: 3,
		CostReceiptObserved: 2, CostReceiptTotal: 3, UnknownCostAttempts: pointerInt(1), CostIdentityUnknownAttempts: pointerInt(0),
	}}

	got := aggregateMetricData([]RunData{run})
	assertFloatMetric(t, "eligible request cache hit", got.RequestCacheHit, 0.5)
	if got.AllExecutedCacheHit != nil {
		t.Fatalf("all-executed cache hit = %v, want unknown when a transport attempt lacks usage", *got.AllExecutedCacheHit)
	}
	if got.ProviderReportedCost != nil {
		t.Fatalf("provider-reported cost = %v, want partial when one transport attempt lacks a receipt", *got.ProviderReportedCost)
	}
	assertFloatMetric(t, "partial provider-reported cost", got.ProviderCostPartial, 0.40)
	assertIntMetric(t, "unknown-cost attempts", got.UnknownCostAttempts, 1)
}

func assertIntMetric(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %d", name, got, want)
	}
}

func assertInt64Metric(t *testing.T, name string, got *int64, want int64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %d", name, got, want)
	}
}

func assertFloatMetric(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil || math.Abs(*got-want) > 1e-12 {
		t.Fatalf("%s = %v, want %.12f", name, got, want)
	}
}
