package report

import (
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestAggregateOrderStrataPreservesQualityTimeCostAndCoverage(t *testing.T) {
	passed, failed := true, false
	trial10, trial20, cost2 := 10.0, 20.0, 2.0
	runs := []RunData{
		{ExperimentID: "formal", AgentID: "luban", ExecutionPosition: "first", Disposition: string(harness.DeepSWEAttemptScored), Passed: &passed, Metrics: MetricData{TrialDurationSeconds: &trial10, ComparableCost: &cost2, ComparableCostBasis: comparableCostBasisFrozen}},
		{ExperimentID: "formal", AgentID: "luban", ExecutionPosition: "first", Disposition: string(harness.DeepSWEAttemptScored), Passed: &failed, Metrics: MetricData{TrialDurationSeconds: &trial20}},
		{ExperimentID: "formal", AgentID: "luban", ExecutionPosition: "second", Disposition: string(harness.DeepSWEAttemptExcluded), Passed: nil},
	}
	strata := aggregateOrderStrata("formal", runs)
	if len(strata) != 2 {
		t.Fatalf("strata = %d, want 2", len(strata))
	}
	var first, second *OrderStratumData
	for index := range strata {
		switch strata[index].Position {
		case "first":
			first = &strata[index]
		case "second":
			second = &strata[index]
		}
	}
	if first == nil || first.Raw != 2 || first.Scored != 2 || first.Excluded != 0 || first.Passed != 1 || first.PassRate == nil || *first.PassRate != 0.5 || first.MeanTrialSeconds == nil || *first.MeanTrialSeconds != 15 || first.TrialObserved != 2 || first.MeanComparableCost == nil || *first.MeanComparableCost != 2 || first.ComparableCostObserved != 1 {
		t.Fatalf("first stratum = %#v", first)
	}
	if second == nil || second.Raw != 1 || second.Scored != 0 || second.Excluded != 1 || second.PassRate != nil || second.MeanTrialSeconds != nil || second.MeanComparableCost != nil {
		t.Fatalf("second stratum fabricated excluded-attempt values: %#v", second)
	}
}
