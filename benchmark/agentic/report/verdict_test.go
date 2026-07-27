package report

import (
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestVerdictRejectsPartialPairedCoverage(t *testing.T) {
	baselinePass, contenderPass := 0.5, 0.75
	difference := contenderPass - baselinePass
	ci := &ConfidenceInterval{Lower: 0.1, Upper: 0.4, Tasks: 2, Pairs: 2}
	comparison := PairedComparison{
		Baseline: "codex", Contender: "luban", Tasks: 2, Pairs: 2,
		Metrics: []MetricComparison{
			{Metric: MetricPassRate, Baseline: &baselinePass, Contender: &contenderPass, Difference: &difference, CI: ci, Tasks: 2, Pairs: 2},
			// Every performance metric is intentionally absent. A subset with only
			// favorable observations must never be enough to declare a winner.
		},
	}
	verdict := buildVerdict([]ExperimentData{{Class: ClassFormal, Gates: allPassingVerdictGates(), Comparisons: []PairedComparison{comparison}}})
	if verdict.Status != "insufficient" {
		t.Fatalf("verdict = %q, want insufficient", verdict.Status)
	}
}

func TestVerdictRequiresEveryFormalGate(t *testing.T) {
	gates := allPassingVerdictGates()
	gates[len(gates)-1].Status = GateFail
	verdict := buildVerdict([]ExperimentData{{Class: ClassFormal, Gates: gates, Comparisons: []PairedComparison{{Baseline: "codex", Contender: "luban"}}}})
	if verdict.Status != "insufficient" {
		t.Fatalf("verdict = %q, want insufficient", verdict.Status)
	}
}

func allPassingVerdictGates() []GateData {
	return []GateData{{Name: "classification", Status: GatePass}, {Name: "formal_score", Status: GatePass}, {Name: "complete_spend", Status: GatePass}}
}

func TestFormalExclusionSymmetryGateUsesScorerPairedAnalysisAndFrozenThresholds(t *testing.T) {
	valid := exclusionGateScorecardFixture()
	if status, _ := formalExclusionSymmetryGate(&valid); status != GatePass {
		t.Fatalf("valid exclusion analysis gate = %s, want pass", status)
	}

	for _, test := range []struct {
		name   string
		mutate func(*harness.DeepSWEPublicScorecard)
	}{
		{name: "per-agent threshold", mutate: func(card *harness.DeepSWEPublicScorecard) {
			card.Agents[0].Counts.Scored--
			card.Agents[0].Counts.Excluded++
			card.ExclusionAnalysis.Agents[0].Scored--
			card.ExclusionAnalysis.Agents[0].Excluded++
			imbalance := card.ExclusionAnalysis.PairedImbalance
			imbalance.CommonScored--
			imbalance.AnyExcluded++
			imbalance.ChallengerOnlyExcluded++
			imbalance.ChallengerExcluded++
			imbalance.AbsoluteCountDifference++
		}},
		{name: "imbalance threshold", mutate: func(card *harness.DeepSWEPublicScorecard) {
			imbalance := card.ExclusionAnalysis.PairedImbalance
			imbalance.BothExcluded--
			imbalance.ChallengerOnlyExcluded++
			imbalance.BaselineExcluded--
			imbalance.ChallengerMinusBaseline++
			imbalance.AbsoluteCountDifference++
			card.Agents[1].Counts.Scored++
			card.Agents[1].Counts.Excluded--
			card.ExclusionAnalysis.Agents[1].Scored++
			card.ExclusionAnalysis.Agents[1].Excluded--
		}},
		{name: "paired coverage", mutate: func(card *harness.DeepSWEPublicScorecard) {
			card.ExclusionAnalysis.PairedImbalance.RawPairs--
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			card := exclusionGateScorecardFixture()
			test.mutate(&card)
			if status, _ := formalExclusionSymmetryGate(&card); status != GateFail {
				t.Fatalf("mutated exclusion analysis gate = %s, want fail", status)
			}
		})
	}
}

func exclusionGateScorecardFixture() harness.DeepSWEPublicScorecard {
	const raw = 452
	card := harness.DeepSWEPublicScorecard{
		TaskCount: 113, Repetitions: 4,
		Agents: []harness.DeepSWEPublicAgentScore{
			{AgentID: "luban", Counts: harness.DeepSWEAttemptCounts{Raw: raw, Scored: 448, Excluded: 4}},
			{AgentID: "codex", Counts: harness.DeepSWEAttemptCounts{Raw: raw, Scored: 450, Excluded: 2}},
		},
		ExclusionAnalysis: harness.DeepSWEPublicExclusionAnalysis{
			Agents: []harness.DeepSWEAgentExclusionSummary{
				{AgentID: "luban", Raw: raw, Scored: 448, Excluded: 4},
				{AgentID: "codex", Raw: raw, Scored: 450, Excluded: 2},
			},
			PairedImbalance: &harness.DeepSWEExclusionImbalance{
				ChallengerAgentID: "luban", BaselineAgentID: "codex", RawPairs: raw,
				CommonScored: 448, AnyExcluded: 4, BothExcluded: 2,
				ChallengerOnlyExcluded: 2, BaselineOnlyExcluded: 0, DiscordantExclusionSlots: 2,
				ChallengerExcluded: 4, BaselineExcluded: 2, ChallengerMinusBaseline: 2, AbsoluteCountDifference: 2,
			},
		},
	}
	return card
}
