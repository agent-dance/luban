package report

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const computedDeepSWEGPT56SolXHighReference = "deepswe-gpt-5.6-sol-xhigh-public-v1"

func loadPublicReferences(references []PublicReference) ([]PublicReference, error) {
	result := slices.Clone(references)
	for index := range result {
		if result[index].ComputedArtifact == "" {
			continue
		}
		if result[index].ComputedArtifact != computedDeepSWEGPT56SolXHighReference {
			return nil, fmt.Errorf("public reference %s requests an unknown computed artifact", result[index].ID)
		}
		artifact, err := harness.LoadDeepSWEGPT56SolXHighPublicReference()
		if err != nil {
			return nil, fmt.Errorf("load public reference %s: %w", result[index].ID, err)
		}
		if err := validateComputedDeepSWEReference(result[index], artifact); err != nil {
			return nil, fmt.Errorf("public reference %s: %w", result[index].ID, err)
		}
		populateComputedDeepSWEReference(&result[index], artifact)
	}
	return result, nil
}

func validateComputedDeepSWEReference(reference PublicReference, artifact harness.DeepSWEPublicReferenceArtifact) error {
	provenance := artifact.Provenance
	if artifact.SchemaVersion != "agentic-bench/deepswe-public-reference-v1" ||
		provenance.SourceURL != harness.DeepSWEGPT56SolXHighSourceURL ||
		provenance.SourceSHA256 != harness.DeepSWEGPT56SolXHighSourceSHA256 ||
		provenance.ProjectionSHA256 != harness.DeepSWEGPT56SolXHighProjectionSHA256 ||
		provenance.Configuration != harness.DeepSWEGPT56SolXHighConfig ||
		provenance.ProjectionPath != harness.DeepSWEGPT56SolXHighProjectionPath ||
		provenance.ProjectionSchema != "deepswe-public-trial-projection/v1" ||
		provenance.ScoringProfile != harness.ScoringProfileDeepSWEV11PublicCI ||
		provenance.ScorecardSchema != "agentic-bench/deepswe-public-scorecard-v2" ||
		provenance.ReferenceAgentID != harness.DeepSWEGPT56SolXHighReferenceAgentID ||
		provenance.ScheduledRuns != 4 || provenance.FrozenTaskUniverse != 113 || provenance.ProjectionBytes < 1 {
		return errors.New("computed artifact provenance differs from the registered double-SHA contract")
	}
	if reference.SourceURL != provenance.SourceURL || reference.Version != "v1.1" || reference.Agent != "mini-swe-agent" || reference.Model != "gpt-5.6-sol" || reference.ReasoningEffort != "xhigh" || reference.Benchmark != "DeepSWE" {
		return errors.New("reference metadata differs from the computed artifact")
	}
	if artifact.RawRows != 452 || artifact.ScoredRows != 451 || artifact.ExcludedRows != 1 || artifact.PassedRows != 319 ||
		len(artifact.SourceExclusions) != 1 || artifact.SourceExclusionsByReason["verifier_timeout"] != 1 {
		return errors.New("computed artifact row accounting differs from 452/451/1/319")
	}
	exclusion := artifact.SourceExclusions[0]
	if exclusion.SourceCategory != "verifier_timeout" || exclusion.PublicFailureCategory != harness.DeepSWEFailureVerifierInfrastructure || exclusion.TrialName == "" || exclusion.TaskID == "" || exclusion.StartedAt.IsZero() {
		return errors.New("computed artifact does not preserve the sole verifier_timeout exclusion")
	}
	card := artifact.Scorecard
	if card.SchemaVersion != provenance.ScorecardSchema || card.Profile != provenance.ScoringProfile || card.Repetitions != 4 || card.TaskCount != 113 || len(card.Agents) != 1 || card.Paired != nil {
		return errors.New("computed public scorecard identity differs from its provenance")
	}
	score := card.Agents[0]
	if score.AgentID != provenance.ReferenceAgentID || score.Counts.Raw != 452 || score.Counts.Scored != 451 || score.Counts.Excluded != 1 || score.Counts.Passed != 319 || score.Counts.Failed != 132 ||
		score.LivePooled.Numerator != 319 || score.LivePooled.Denominator != 451 || !referenceFloatNear(score.LivePooled.Rate, 319.0/451.0, 1e-15) ||
		score.TaskMacro.Numerator != 80 || score.TaskMacro.Denominator != 113 || !referenceFloatNear(score.TaskMacro.Rate, 80.0/113.0, 1e-15) {
		return errors.New("computed public score differs from the frozen pooled and task-macro contract")
	}
	if score.PassAt4 == nil || score.PassAt4.K != 4 || score.PassAt4.PassedTasks != 97 || score.PassAt4.TotalTasks != 113 || score.PassAt4.UniverseTasks != 113 || !referenceFloatNear(score.PassAt4.Rate, 97.0/113.0, 1e-15) {
		return errors.New("computed public pass@4 differs from 97/113")
	}
	statistics := score.FourRunStatistics
	if statistics == nil || len(statistics.Runs) != 4 || !referenceFloatNear(statistics.RunMean, 0.7072929835651074, 1e-15) ||
		!referenceFloatNear(statistics.SampleStandardDeviation, 0.008358436463071354, 1e-15) || statistics.Z != 1.959963984540054 ||
		!referenceFloatNear(statistics.ConfidenceCenter, 319.0/451.0, 1e-15) ||
		!referenceFloatNear(statistics.ConfidenceLower, 0.6991259559533886, 1e-15) ||
		!referenceFloatNear(statistics.ConfidenceUpper, 0.7155081903880748, 1e-15) ||
		!referenceFloatNear(statistics.ConfidenceHalfWidth, 0.008191117217343103, 1e-15) ||
		!referenceFloatNear(statistics.HalfWidthPercentagePoints, 0.8191117217343103, 1e-13) {
		return errors.New("computed public four-run confidence interval differs from the frozen official projection")
	}
	wantPassed, wantScored := []int{81, 80, 80, 78}, []int{113, 113, 113, 112}
	for index, sample := range statistics.Runs {
		if sample.Run != index+1 || sample.Passed != wantPassed[index] || sample.Scored != wantScored[index] {
			return errors.New("computed public run samples differ from the frozen official projection")
		}
	}
	efficiency := score.AllExecutedEfficiency
	if efficiency.Attempts != 452 || efficiency.AgentDurationSeconds.Observed != 452 || efficiency.TrialDurationSeconds.Observed != 452 || efficiency.CostUSD.Observed != 452 ||
		efficiency.AgentSteps.Observed != 452 || efficiency.InputTokens.Observed != 452 || efficiency.CachedInputTokens.Observed != 452 || efficiency.OutputTokens.Observed != 452 ||
		efficiency.AgentSteps.Sum == nil || *efficiency.AgentSteps.Sum != 19910 ||
		efficiency.InputTokens.Sum == nil || *efficiency.InputTokens.Sum != 1927681373 ||
		efficiency.CachedInputTokens.Sum == nil || *efficiency.CachedInputTokens.Sum != 1795639296 ||
		efficiency.OutputTokens.Sum == nil || *efficiency.OutputTokens.Sum != 18428674 ||
		!referenceFloatNear(efficiency.CostUSD.Sum, 2127.8190749999994, 1e-10) ||
		!referenceFloatNear(efficiency.TokenWeightedCacheRate, 1795639296.0/1927681373.0, 1e-15) {
		return errors.New("computed public all-executed efficiency differs from the frozen official projection")
	}
	return nil
}

func populateComputedDeepSWEReference(reference *PublicReference, artifact harness.DeepSWEPublicReferenceArtifact) {
	score := artifact.Scorecard.Agents[0]
	reference.Score = cloneFloat(score.LivePooled.Rate)
	passed, total := score.Counts.Passed, score.Counts.Scored
	reference.Passed, reference.Total = &passed, &total
	reference.CostPerTask = cloneFloat(score.AllExecutedEfficiency.CostUSD.Mean)
	if score.AllExecutedEfficiency.TrialDurationSeconds.Mean != nil {
		minutes := *score.AllExecutedEfficiency.TrialDurationSeconds.Mean / 60
		reference.MinutesPerTask = &minutes
	}
	reference.TurnsPerTask = cloneFloat(score.AllExecutedEfficiency.AgentSteps.Mean)
	if score.AllExecutedEfficiency.InputTokens.Sum != nil && score.AllExecutedEfficiency.OutputTokens.Sum != nil && score.AllExecutedEfficiency.Attempts > 0 {
		tokens := float64(*score.AllExecutedEfficiency.InputTokens.Sum+*score.AllExecutedEfficiency.OutputTokens.Sum) / float64(score.AllExecutedEfficiency.Attempts)
		reference.TokensPerTask = &tokens
	}
	reference.TokenWeightedCacheHit = cloneFloat(score.AllExecutedEfficiency.TokenWeightedCacheRate)
	reference.Components = []ReferenceComponent{
		{Name: "task_macro", Score: cloneFloat(score.TaskMacro.Rate)},
		{Name: "pass_at_4", Score: cloneFloat(score.PassAt4.Rate)},
		{Name: "public_run_ci_center", Score: cloneFloat(score.FourRunStatistics.ConfidenceCenter)},
	}
	reference.Computed = &artifact
}

func referenceFloatNear(value *float64, want, tolerance float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && math.Abs(*value-want) <= tolerance
}
