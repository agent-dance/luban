package report

import (
	"math"
	"testing"
)

func TestTokenCacheComparisonUsesRatioOfSums(t *testing.T) {
	pairs := [][2]RunData{
		{
			{TaskID: "small", Metrics: cacheMetrics(0, 1)},
			{TaskID: "small", Metrics: cacheMetrics(1, 1)},
		},
		{
			{TaskID: "large", Metrics: cacheMetrics(900, 1000)},
			{TaskID: "large", Metrics: cacheMetrics(200, 1000)},
		},
	}
	statistics := StatisticsSpec{ConfidenceLevel: 0.95, Method: "paired-task-cluster-bootstrap-v1", Resamples: 1000, Seed: 7}
	comparison := compareMetric(pairs, MetricTokenCacheHit, statistics, "ratio-of-sums-test")
	wantBaseline := 900.0 / 1001.0
	wantContender := 201.0 / 1001.0
	if comparison.Baseline == nil || comparison.Contender == nil || comparison.Difference == nil {
		t.Fatalf("comparison is incomplete: %+v", comparison)
	}
	if math.Abs(*comparison.Baseline-wantBaseline) > 1e-12 || math.Abs(*comparison.Contender-wantContender) > 1e-12 {
		t.Fatalf("baseline=%g contender=%g, want %g and %g", *comparison.Baseline, *comparison.Contender, wantBaseline, wantContender)
	}
	if *comparison.Difference >= 0 {
		t.Fatalf("ratio-of-sums difference=%g, want contender lower; a mean of run-level rates would reverse this result", *comparison.Difference)
	}
	if comparison.CI == nil {
		t.Fatal("two task clusters did not produce a deterministic bootstrap interval")
	}
}

func cacheMetrics(cached, input int64) MetricData {
	rate := float64(cached) / float64(input)
	return MetricData{CachedInputTokens: &cached, InputTokens: &input, TokenWeightedCacheHit: &rate}
}
