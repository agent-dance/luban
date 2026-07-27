package report

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestApplySymmetricComparableCostBasisUsesVisibleTokensWithoutInvoiceOrCacheWriteGate(t *testing.T) {
	complete := func(cost float64) RunData {
		surcharge := 0.25
		return RunData{Metrics: MetricData{
			TransportAttempts: pointerInt(2), PrewarmAttempts: pointerInt(1),
			AllExecutedUsageObserved: 2, AllExecutedUsageTotal: 2,
			AllExecutedCacheWriteObserved: 2, AllExecutedCacheWriteTotal: 2,
			CostReceiptObserved: 2, CostReceiptTotal: 2,
			UnknownCostAttempts: pointerInt(0), CostIdentityUnknownAttempts: pointerInt(0),
			CatalogCost: pointerFloat(cost), KnownCatalogCostLowerBound: pointerFloat(cost), KnownCacheWriteSurcharge: pointerFloat(surcharge),
		}}
	}

	experiment := ExperimentData{Runs: []RunData{complete(1.25), complete(2.50)}}
	applySymmetricComparableCostBasis(&experiment)
	wantCosts := []float64{1.00, 2.25}
	for index, run := range experiment.Runs {
		if run.Metrics.ComparableCostBasis != comparableCostBasisFrozen || run.Metrics.ComparableCost == nil {
			t.Fatalf("complete run %d basis/cost = %q/%v", index, run.Metrics.ComparableCostBasis, run.Metrics.ComparableCost)
		}
		if *run.Metrics.ComparableCost != wantCosts[index] {
			t.Fatalf("complete run %d visible-token cost = %v, want %v", index, *run.Metrics.ComparableCost, wantCosts[index])
		}
	}

	experiment = ExperimentData{Runs: []RunData{complete(1.25), complete(2.50)}}
	experiment.Runs[1].Metrics.AllExecutedCacheWriteObserved = 1
	experiment.Runs[1].Metrics.CostReceiptObserved = 1
	experiment.Runs[1].Metrics.UnknownCostAttempts = pointerInt(1)
	applySymmetricComparableCostBasis(&experiment)
	for index, run := range experiment.Runs {
		if run.Metrics.ComparableCostBasis != comparableCostBasisFrozen || run.Metrics.ComparableCost == nil {
			t.Fatalf("cache-write/invoice-incomplete run %d basis/cost = %q/%v, want visible-token estimate", index, run.Metrics.ComparableCostBasis, run.Metrics.ComparableCost)
		}
	}

	experiment = ExperimentData{Runs: []RunData{complete(1.25), complete(2.50)}}
	experiment.Runs[1].Metrics.AllExecutedUsageObserved = 1
	applySymmetricComparableCostBasis(&experiment)
	if experiment.Runs[0].Metrics.ComparableCost == nil || experiment.Runs[1].Metrics.ComparableCost != nil || experiment.Runs[1].Metrics.ComparableCostBasis != comparableCostBasisUnknown {
		t.Fatalf("per-run missing usage was not preserved as unknown: %+v", experiment.Runs)
	}
}

func TestHeadlineCostNeverFallsBackToAuditCatalogCost(t *testing.T) {
	metrics := MetricData{
		CatalogCost: pointerFloat(9.99), KnownCatalogCostLowerBound: pointerFloat(8.88),
		ComparableCostBasis: comparableCostBasisUnknown,
	}
	got := headlineCostValue(i18n.LangEN, metrics)
	if want := i18n.Text(i18n.LangEN, i18n.KeyAgenticReportStatusUnknown); got != want {
		t.Fatalf("headline cost = %q, want %q", got, want)
	}
	if strings.Contains(got, "9.99") || strings.Contains(got, "8.88") {
		t.Fatalf("headline leaked audit cost: %q", got)
	}
}

func TestProviderCostWithoutReceiptsUsesSemanticNotAvailableCopy(t *testing.T) {
	got := providerCostValue(i18n.LangEN, MetricData{ProviderCostTotal: 4})
	want := i18n.Text(i18n.LangEN, i18n.KeyAgenticReportCostProviderNotAvailable)
	if got != want {
		t.Fatalf("provider cost = %q, want %q", got, want)
	}
}

func TestPublicScorecardCostIsAlwaysRemovedFromFormalReport(t *testing.T) {
	cost := 3.5
	card := exclusionGateScorecardFixture()
	for index := range card.Agents {
		card.Agents[index].AllExecutedEfficiency.Attempts = 4
		card.Agents[index].AllExecutedEfficiency.CostUSD.Observed = 4
		card.Agents[index].AllExecutedEfficiency.CostUSD.Total = 4
		card.Agents[index].AllExecutedEfficiency.CostUSD.Sum = &cost
		card.Agents[index].AllExecutedEfficiency.CostUSD.Mean = &cost
	}

	got := publicScorecardWithoutCost(&card)
	if got == nil {
		t.Fatal("sanitized scorecard is nil")
	}
	for _, agent := range got.Agents {
		costAggregate := agent.AllExecutedEfficiency.CostUSD
		if costAggregate.Observed != 0 || costAggregate.Total != 4 || costAggregate.Sum != nil || costAggregate.Mean != nil {
			t.Fatalf("public cost aggregate = %+v, want N/A with total coverage denominator", costAggregate)
		}
	}
	if card.Agents[0].AllExecutedEfficiency.CostUSD.Sum == nil {
		t.Fatal("sanitization mutated the archived public scorecard")
	}
}
