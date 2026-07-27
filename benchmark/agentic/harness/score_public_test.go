package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

type publicTrialProjection struct {
	SchemaVersion string              `json:"schema_version"`
	SourceURL     string              `json:"source_url"`
	SourceSHA256  string              `json:"source_sha256"`
	Config        string              `json:"config"`
	Rows          []publicTrialRecord `json:"rows"`
}

type publicTrialRecord struct {
	TrialName            string    `json:"trial_name"`
	TaskName             string    `json:"task_name"`
	StartedAt            time.Time `json:"started_at"`
	Passed               bool      `json:"passed"`
	IncludedInScore      bool      `json:"included_in_score"`
	ErrorCategory        *string   `json:"error_category"`
	AgentDurationSeconds *float64  `json:"agent_duration_seconds"`
	TrialDurationSeconds *float64  `json:"trial_duration_seconds"`
	CostUSD              *float64  `json:"cost_usd"`
	AgentSteps           *int64    `json:"n_agent_steps"`
	InputTokens          *int64    `json:"n_input_tokens"`
	CacheTokens          *int64    `json:"n_cache_tokens"`
	OutputTokens         *int64    `json:"n_output_tokens"`
}

func TestDeepSWEPublicScorerMatchesOfficialGPT56SolXHighArtifact(t *testing.T) {
	const projectionSHA256 = "55a59b36d22f1406d49c11134275293bc4970c34a4dd54a03a3b89b5635986cd"
	const sourceSHA256 = "7844056bade4cee4a2c2964c9582bf7eb1344735a28695cae7d419055656417a"
	raw, err := os.ReadFile("testdata/deepswe-v1.1-gpt-5.6-sol-xhigh-public-trials.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if actual := hex.EncodeToString(digest[:]); actual != projectionSHA256 {
		t.Fatalf("public trial projection SHA-256 = %s, want %s", actual, projectionSHA256)
	}
	var fixture publicTrialProjection
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != "deepswe-public-trial-projection/v1" || fixture.SourceURL != "https://deepswe.datacurve.ai/artifacts/v1.1/trials.json" || fixture.SourceSHA256 != sourceSHA256 || fixture.Config != "mini_swe_agent_gpt_5_6_sol_xhigh" {
		t.Fatalf("public trial projection provenance is invalid: %#v", fixture)
	}
	if len(fixture.Rows) != 452 {
		t.Fatalf("raw public trial rows = %d, want 452", len(fixture.Rows))
	}
	input := DeepSWEPublicScoringInput{
		Profile: ScoringProfileDeepSWEV11PublicCI, Repetitions: 4,
		AgentIDs: []string{"public-gpt56-xhigh"},
	}
	byTask := map[string][]publicTrialRecord{}
	for _, row := range fixture.Rows {
		byTask[row.TaskName] = append(byTask[row.TaskName], row)
	}
	for taskID := range byTask {
		input.TaskIDs = append(input.TaskIDs, taskID)
	}
	slices.Sort(input.TaskIDs)
	for _, taskID := range input.TaskIDs {
		rows := byTask[taskID]
		sort.Slice(rows, func(i, j int) bool { return rows[i].StartedAt.Before(rows[j].StartedAt) })
		if len(rows) != 4 {
			t.Fatalf("official task %s has %d raw rows, want 4", taskID, len(rows))
		}
		for index, row := range rows {
			if row.AgentDurationSeconds == nil || row.TrialDurationSeconds == nil || row.CostUSD == nil || row.AgentSteps == nil || row.InputTokens == nil || row.CacheTokens == nil || row.OutputTokens == nil {
				t.Fatalf("official trial %s has incomplete projected efficiency", row.TrialName)
			}
			attempt := DeepSWEPublicAttempt{
				AttemptID: row.TrialName, AgentID: input.AgentIDs[0], TaskID: taskID, Slot: index + 1, StartedAt: row.StartedAt,
				Efficiency: DeepSWEAttemptEfficiency{
					AgentDurationSeconds: row.AgentDurationSeconds, TrialDurationSeconds: row.TrialDurationSeconds, CostUSD: row.CostUSD,
					AgentSteps:  row.AgentSteps,
					InputTokens: row.InputTokens, CachedInputTokens: row.CacheTokens, OutputTokens: row.OutputTokens,
				},
			}
			if row.IncludedInScore {
				if row.ErrorCategory != nil {
					t.Fatalf("included official trial %s has error category %q", row.TrialName, *row.ErrorCategory)
				}
				attempt.Disposition, attempt.Passed, attempt.FailureCategory = DeepSWEAttemptScored, boolPointer(row.Passed), DeepSWEFailureNone
			} else {
				if row.ErrorCategory == nil || *row.ErrorCategory != "verifier_timeout" {
					t.Fatalf("unsupported official exclusion on %s: %v", row.TrialName, row.ErrorCategory)
				}
				attempt.Disposition, attempt.Passed, attempt.FailureCategory = DeepSWEAttemptExcluded, nil, DeepSWEFailureVerifierInfrastructure
			}
			input.Attempts = append(input.Attempts, attempt)
		}
	}
	if len(input.TaskIDs) != 113 {
		t.Fatalf("official task count = %d, want 113", len(input.TaskIDs))
	}

	card, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		t.Fatal(err)
	}
	score := card.Agents[0]
	if score.Counts.Raw != 452 || score.Counts.Scored != 451 || score.Counts.Excluded != 1 || score.Counts.Passed != 319 {
		t.Fatalf("attempt counts = %#v, want raw/scored/excluded/passed 452/451/1/319", score.Counts)
	}
	assertRateNear(t, score.LivePooled.Rate, 319.0/451.0, 1e-15)
	if score.TaskMacro.Numerator != 80 || score.TaskMacro.Denominator != 113 {
		t.Fatalf("task macro fraction = %#v, want 80/113", score.TaskMacro)
	}
	assertRateNear(t, score.TaskMacro.Rate, 80.0/113.0, 1e-15)
	if score.PassAt4 == nil || score.PassAt4.PassedTasks != 97 || score.PassAt4.TotalTasks != 113 || score.PassAt4.UniverseTasks != 113 {
		t.Fatalf("pass@4 = %#v, want 97/113", score.PassAt4)
	}
	assertRateNear(t, score.PassAt4.Rate, 97.0/113.0, 1e-15)
	if score.FourRunStatistics == nil || len(score.FourRunStatistics.Runs) != 4 {
		t.Fatalf("four-run statistics missing: %#v", score.FourRunStatistics)
	}
	wantRunPassed, wantRunScored := []int{81, 80, 80, 78}, []int{113, 113, 113, 112}
	for index, run := range score.FourRunStatistics.Runs {
		if run.Passed != wantRunPassed[index] || run.Scored != wantRunScored[index] {
			t.Errorf("run %d = %d/%d, want %d/%d", index+1, run.Passed, run.Scored, wantRunPassed[index], wantRunScored[index])
		}
	}
	if score.FourRunStatistics.RunMean == nil || score.FourRunStatistics.SampleStandardDeviation == nil || score.FourRunStatistics.ConfidenceCenter == nil || score.FourRunStatistics.ConfidenceLower == nil || score.FourRunStatistics.ConfidenceUpper == nil || score.FourRunStatistics.ConfidenceHalfWidth == nil || score.FourRunStatistics.HalfWidthPercentagePoints == nil {
		t.Fatal("four-run z confidence interval is null")
	}
	assertNear(t, *score.FourRunStatistics.RunMean, 0.7072929835651074, 1e-15)
	assertNear(t, *score.FourRunStatistics.SampleStandardDeviation, 0.008358436463071354, 1e-15)
	assertNear(t, *score.FourRunStatistics.ConfidenceCenter, 319.0/451.0, 1e-15)
	assertNear(t, *score.FourRunStatistics.ConfidenceLower, 0.6991259559533886, 1e-15)
	assertNear(t, *score.FourRunStatistics.ConfidenceUpper, 0.7155081903880748, 1e-15)
	assertNear(t, *score.FourRunStatistics.ConfidenceHalfWidth, 0.008191117217343103, 1e-15)
	assertNear(t, *score.FourRunStatistics.HalfWidthPercentagePoints, 0.8191117217343103, 1e-13)
	if score.ScoredCountDistributionByTask[4] != 112 || score.ScoredCountDistributionByTask[3] != 1 {
		t.Fatalf("scored-count distribution = %#v", score.ScoredCountDistributionByTask)
	}
	sensitivity := score.ExclusionSensitivity
	if sensitivity.RawAttempts != 452 || sensitivity.ScoredAttempts != 451 || sensitivity.ExcludedAttempts != 1 || sensitivity.ExclusionRate == nil || sensitivity.WorstCaseAllExcludedAsFailure.PassAt4 == nil || sensitivity.BestCaseAllExcludedAsPass.PassAt4 == nil {
		t.Fatalf("official exclusion sensitivity is incomplete: %#v", sensitivity)
	}
	assertRateNear(t, sensitivity.ExclusionRate, 1.0/452.0, 1e-15)
	assertRateNear(t, sensitivity.WorstCaseAllExcludedAsFailure.LivePooled.Rate, 319.0/452.0, 1e-15)
	assertRateNear(t, sensitivity.BestCaseAllExcludedAsPass.LivePooled.Rate, 320.0/452.0, 1e-15)
	assertRateNear(t, sensitivity.WorstCaseAllExcludedAsFailure.TaskMacro.Rate, 79.75/113.0, 1e-15)
	assertRateNear(t, sensitivity.BestCaseAllExcludedAsPass.TaskMacro.Rate, 80.0/113.0, 1e-15)
	if sensitivity.WorstCaseAllExcludedAsFailure.PassAt4.PassedTasks != 97 || sensitivity.BestCaseAllExcludedAsPass.PassAt4.PassedTasks != 97 {
		t.Fatalf("official pass@4 sensitivity = worst %#v best %#v", sensitivity.WorstCaseAllExcludedAsFailure.PassAt4, sensitivity.BestCaseAllExcludedAsPass.PassAt4)
	}
	if len(card.ExclusionAnalysis.Agents) != 1 || card.ExclusionAnalysis.Agents[0].ExcludedByCategory[DeepSWEFailureVerifierInfrastructure] != 1 || card.ExclusionAnalysis.PairedImbalance != nil {
		t.Fatalf("official exclusion analysis = %#v", card.ExclusionAnalysis)
	}
	efficiency := score.AllExecutedEfficiency
	if efficiency.Attempts != 452 || efficiency.CostUSD.Observed != 452 || efficiency.AgentDurationSeconds.Observed != 452 || efficiency.TrialDurationSeconds.Observed != 452 {
		t.Fatalf("excluded trial was omitted from all-executed efficiency: %#v", efficiency)
	}
	if efficiency.AgentSteps.Sum == nil || *efficiency.AgentSteps.Sum != 19910 || efficiency.InputTokens.Sum == nil || *efficiency.InputTokens.Sum != 1927681373 || efficiency.CachedInputTokens.Sum == nil || *efficiency.CachedInputTokens.Sum != 1795639296 || efficiency.OutputTokens.Sum == nil || *efficiency.OutputTokens.Sum != 18428674 {
		t.Fatalf("all-executed token totals differ from the official projection: %#v", efficiency)
	}
	if efficiency.CostUSD.Sum == nil {
		t.Fatal("all-executed official cost is null")
	}
	assertNear(t, *efficiency.CostUSD.Sum, 2127.8190749999994, 1e-10)
	if efficiency.TokenWeightedCacheRate == nil {
		t.Fatal("all-executed official cache rate is null")
	}
	assertNear(t, *efficiency.TokenWeightedCacheRate, 1795639296.0/1927681373.0, 1e-15)
}

func TestDeepSWEPublicReferenceArtifactIsComputedSerializableAndProvenanced(t *testing.T) {
	artifact, err := LoadDeepSWEGPT56SolXHighPublicReference()
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SchemaVersion != "agentic-bench/deepswe-public-reference-v1" || artifact.Provenance.SourceURL != DeepSWEGPT56SolXHighSourceURL || artifact.Provenance.SourceSHA256 != DeepSWEGPT56SolXHighSourceSHA256 || artifact.Provenance.ProjectionSHA256 != DeepSWEGPT56SolXHighProjectionSHA256 || artifact.Provenance.ProjectionBytes != 177062 || artifact.Provenance.Configuration != DeepSWEGPT56SolXHighConfig {
		t.Fatalf("reference provenance = %#v", artifact.Provenance)
	}
	if artifact.RawRows != 452 || artifact.ScoredRows != 451 || artifact.ExcludedRows != 1 || artifact.PassedRows != 319 || len(artifact.SourceExclusions) != 1 || artifact.SourceExclusions[0].SourceCategory != "verifier_timeout" || artifact.SourceExclusions[0].PublicFailureCategory != DeepSWEFailureVerifierInfrastructure {
		t.Fatalf("reference source accounting = %#v", artifact)
	}
	score := artifact.Scorecard.Agents[0]
	if score.LivePooled.Rate == nil || score.PassAt4 == nil || score.PassAt4.PassedTasks != 97 || score.PassAt4.TotalTasks != 113 || score.FourRunStatistics == nil || score.FourRunStatistics.HalfWidthPercentagePoints == nil {
		t.Fatalf("computed reference score = %#v", score)
	}
	assertRateNear(t, score.LivePooled.Rate, 319.0/451.0, 1e-15)
	assertNear(t, *score.FourRunStatistics.HalfWidthPercentagePoints, 0.8191117217343103, 1e-13)

	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DeepSWEPublicReferenceArtifact
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Provenance != artifact.Provenance || decoded.RawRows != artifact.RawRows || decoded.Scorecard.Agents[0].Counts != artifact.Scorecard.Agents[0].Counts {
		t.Fatalf("serialized reference artifact lost its contract: %s", encoded)
	}

	second, err := LoadDeepSWEGPT56SolXHighPublicReference()
	if err != nil {
		t.Fatal(err)
	}
	artifact.Scorecard.Agents[0].Counts.Passed = 0
	if second.Scorecard.Agents[0].Counts.Passed != 319 {
		t.Fatal("reference loader returned mutable shared score state")
	}
}

func TestDeepSWEPublicScorerOneRunDoesNotClaimPassAt4OrRunCI(t *testing.T) {
	input := oneRunScoringFixture()
	card, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, score := range card.Agents {
		if score.PassAt4 != nil || score.FourRunStatistics != nil {
			t.Fatalf("one-run pilot claimed a four-run statistic: %#v", score)
		}
	}
}

func TestDeepSWEPublicScorerKeepsAllExcludedQualityNull(t *testing.T) {
	startedAt := time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC)
	input := DeepSWEPublicScoringInput{
		Profile: ScoringProfileDeepSWEV11PublicCI, Repetitions: 1,
		TaskIDs: []string{"task-a"}, AgentIDs: []string{"luban"},
		Attempts: []DeepSWEPublicAttempt{{
			AttemptID: "excluded", AgentID: "luban", TaskID: "task-a", Slot: 1, StartedAt: startedAt,
			Disposition: DeepSWEAttemptExcluded, FailureCategory: DeepSWEFailureVerifierInfrastructure,
		}},
	}
	card, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		t.Fatal(err)
	}
	score := card.Agents[0]
	if score.Counts.Raw != 1 || score.Counts.Scored != 0 || score.Counts.Excluded != 1 || score.LivePooled.Denominator != 0 || score.LivePooled.Rate != nil || score.TaskMacro.Denominator != 0 || score.TaskMacro.Rate != nil || score.Tasks[0].Rate != nil {
		t.Fatalf("all-excluded quality was imputed as zero: %#v", score)
	}
}

func TestDeepSWEPublicScorerExcludesOnlyTypedInfrastructureCategories(t *testing.T) {
	startedAt := time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC)
	categories := []DeepSWEFailureCategory{
		DeepSWEFailureProviderInfrastructure,
		DeepSWEFailureVerifierInfrastructure,
		DeepSWEFailureNetworkInfrastructure,
		DeepSWEFailureControllerInfrastructure,
	}
	input := DeepSWEPublicScoringInput{
		Profile: ScoringProfileDeepSWEV11PublicCI, Repetitions: 1,
		AgentIDs: []string{"luban"},
	}
	for index, category := range categories {
		taskID := "task-" + string(rune('a'+index))
		input.TaskIDs = append(input.TaskIDs, taskID)
		input.Attempts = append(input.Attempts, DeepSWEPublicAttempt{
			AttemptID: "excluded-" + taskID, AgentID: "luban", TaskID: taskID, Slot: 1,
			StartedAt:   startedAt.Add(time.Duration(index) * time.Second),
			Disposition: DeepSWEAttemptExcluded, FailureCategory: category,
		})
	}
	card, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		t.Fatal(err)
	}
	score := card.Agents[0]
	if score.Counts.Raw != 4 || score.Counts.Scored != 0 || score.Counts.Excluded != 4 {
		t.Fatalf("typed infrastructure counts = %#v", score.Counts)
	}
	for _, category := range categories {
		if score.ExcludedByCategory[category] != 1 {
			t.Errorf("typed exclusion %s count = %d, want 1", category, score.ExcludedByCategory[category])
		}
	}
}

func TestDeepSWEPublicScorerClassifiesInfrastructureAndAgentFailures(t *testing.T) {
	input := DeepSWEPublicScoringInput{
		Profile: ScoringProfileDeepSWEV11PublicCI, Repetitions: 4,
		TaskIDs: []string{"task-a"}, AgentIDs: []string{"luban"},
	}
	base := time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC)
	input.Attempts = []DeepSWEPublicAttempt{
		{AttemptID: "a1", AgentID: "luban", TaskID: "task-a", Slot: 1, StartedAt: base, Disposition: DeepSWEAttemptScored, Passed: boolPointer(false), FailureCategory: DeepSWEFailureAgentTimeout},
		{AttemptID: "a2", AgentID: "luban", TaskID: "task-a", Slot: 2, StartedAt: base.Add(time.Second), Disposition: DeepSWEAttemptExcluded, FailureCategory: DeepSWEFailureProviderInfrastructure},
		{AttemptID: "a3", AgentID: "luban", TaskID: "task-a", Slot: 3, StartedAt: base.Add(2 * time.Second), Disposition: DeepSWEAttemptScored, Passed: boolPointer(false), FailureCategory: DeepSWEFailureContext},
		{AttemptID: "a4", AgentID: "luban", TaskID: "task-a", Slot: 4, StartedAt: base.Add(3 * time.Second), Disposition: DeepSWEAttemptScored, Passed: boolPointer(true), FailureCategory: DeepSWEFailureNone},
	}
	card, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		t.Fatal(err)
	}
	score := card.Agents[0]
	if score.Counts.Raw != 4 || score.Counts.Scored != 3 || score.Counts.Excluded != 1 || score.Counts.Passed != 1 || score.Counts.Failed != 2 {
		t.Fatalf("classification counts = %#v", score.Counts)
	}
	if score.Tasks[0].ScoredAttempts[1].Run != 2 || score.Tasks[0].ScoredAttempts[1].Slot != 3 {
		t.Fatalf("remaining scored attempts were not renumbered by started_at: %#v", score.Tasks[0].ScoredAttempts)
	}
	if score.ExcludedByCategory[DeepSWEFailureProviderInfrastructure] != 1 || score.ScoredFailuresByCategory[DeepSWEFailureAgentTimeout] != 1 || score.ScoredFailuresByCategory[DeepSWEFailureContext] != 1 {
		t.Fatalf("failure categories = excluded %#v scored %#v", score.ExcludedByCategory, score.ScoredFailuresByCategory)
	}

	invalid := input
	invalid.Attempts = slices.Clone(input.Attempts)
	invalid.Attempts[1].FailureCategory = DeepSWEFailureAgentTimeout
	if _, err := ScoreDeepSWEPublicCI(invalid); err == nil {
		t.Fatal("agent timeout was incorrectly accepted as an exclusion")
	}
	invalid = input
	invalid.Attempts = append(slices.Clone(input.Attempts), input.Attempts[0])
	invalid.Attempts[4].AttemptID = "replacement"
	if _, err := ScoreDeepSWEPublicCI(invalid); err == nil {
		t.Fatal("replacement attempt beyond the preregistered matrix was accepted")
	}
}

func TestDeepSWEPublicScorerPreservesMissingEfficiencyAsNull(t *testing.T) {
	input := oneRunScoringFixture()
	input.Attempts[0].Efficiency = DeepSWEAttemptEfficiency{}
	card, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		t.Fatal(err)
	}
	efficiency := card.Agents[0].AllExecutedEfficiency
	if efficiency.CostUSD.Observed != 0 || efficiency.CostUSD.Sum != nil || efficiency.CostUSD.Mean != nil || efficiency.LLMCallsStarted.Observed != 0 || efficiency.LLMCallsStarted.Sum != nil || efficiency.LLMCallsStarted.Mean != nil || efficiency.TokenWeightedCacheRate != nil {
		t.Fatalf("missing efficiency became zero: %#v", efficiency)
	}
	encoded, err := json.Marshal(efficiency)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	cost := decoded["cost_usd"].(map[string]any)
	llmCallsStarted := decoded["llm_calls_started"].(map[string]any)
	if cost["sum"] != nil || cost["mean"] != nil || llmCallsStarted["sum"] != nil || llmCallsStarted["mean"] != nil || decoded["token_weighted_cache_rate"] != nil {
		t.Fatalf("JSON did not preserve null telemetry: %s", encoded)
	}
}

func TestDeepSWEPublicScorerAggregatesLLMCallsStartedIndependently(t *testing.T) {
	base := time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC)
	input := DeepSWEPublicScoringInput{
		Profile: ScoringProfileDeepSWEV11PublicCI, Repetitions: 1,
		TaskIDs: []string{"task-a", "task-b"}, AgentIDs: []string{"luban"},
		Attempts: []DeepSWEPublicAttempt{
			{
				AttemptID: "a", AgentID: "luban", TaskID: "task-a", Slot: 1, StartedAt: base,
				Disposition: DeepSWEAttemptScored, Passed: boolPointer(true), FailureCategory: DeepSWEFailureNone,
				Efficiency: DeepSWEAttemptEfficiency{LLMCallsStarted: int64Pointer(2), ProviderRequests: int64Pointer(7)},
			},
			{
				AttemptID: "b", AgentID: "luban", TaskID: "task-b", Slot: 1, StartedAt: base.Add(time.Second),
				Disposition: DeepSWEAttemptScored, Passed: boolPointer(true), FailureCategory: DeepSWEFailureNone,
				Efficiency: DeepSWEAttemptEfficiency{LLMCallsStarted: int64Pointer(3), ProviderRequests: int64Pointer(11)},
			},
		},
	}
	card, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		t.Fatal(err)
	}
	efficiency := card.Agents[0].AllExecutedEfficiency
	if efficiency.LLMCallsStarted.Observed != 2 || efficiency.LLMCallsStarted.Sum == nil || *efficiency.LLMCallsStarted.Sum != 5 || efficiency.LLMCallsStarted.Mean == nil || *efficiency.LLMCallsStarted.Mean != 2.5 {
		t.Fatalf("LLM calls aggregate = %#v", efficiency.LLMCallsStarted)
	}
	if efficiency.ProviderRequests.Sum == nil || *efficiency.ProviderRequests.Sum != 18 {
		t.Fatalf("provider requests were aliased to LLM calls: %#v", efficiency.ProviderRequests)
	}
}

func TestDeepSWEAttemptEfficiencyMapsLLMCallsStartedWithLifecycleSemantics(t *testing.T) {
	sealed := deepSWEAttemptEfficiencyFromRecord(RunRecord{
		Execution: &AgentExecution{Lifecycle: AttemptLifecycle{ProviderAttemptState: "provider_attempt_sealed", ProviderAttemptCount: 4}},
		Metrics:   &UsageMetrics{LLMCallsStarted: 3, ProviderRequests: 9},
	})
	if sealed.LLMCallsStarted == nil || *sealed.LLMCallsStarted != 3 || sealed.ProviderRequests == nil || *sealed.ProviderRequests != 9 {
		t.Fatalf("sealed metrics projection = %#v", sealed)
	}

	noProviderAttempt := deepSWEAttemptEfficiencyFromRecord(RunRecord{
		Execution: &AgentExecution{Lifecycle: AttemptLifecycle{ProviderAttemptState: "no_provider_attempt", ProviderAttemptCount: 0}},
	})
	if noProviderAttempt.LLMCallsStarted == nil || *noProviderAttempt.LLMCallsStarted != 0 {
		t.Fatalf("proven no-provider attempt did not project zero: %#v", noProviderAttempt)
	}

	unsealed := deepSWEAttemptEfficiencyFromRecord(RunRecord{
		Execution: &AgentExecution{Lifecycle: AttemptLifecycle{ProviderAttemptState: "provider_attempt_started_unsealed", ProviderAttemptCount: 1}},
	})
	if unsealed.LLMCallsStarted != nil {
		t.Fatalf("unsealed provider attempt fabricated a count: %#v", unsealed)
	}
	if unknown := deepSWEAttemptEfficiencyFromRecord(RunRecord{}); unknown.LLMCallsStarted != nil {
		t.Fatalf("unknown lifecycle fabricated a count: %#v", unknown)
	}
}

func TestDeepSWEPublicScorerBuildsCommonPairedTableAndMcNemarInputs(t *testing.T) {
	input := oneRunScoringFixture()
	input.TaskIDs = []string{"task-a", "task-b"}
	input.BaselineAgentID, input.ChallengerAgentID = "codex", "luban"
	base := input.Attempts[0].StartedAt
	input.Attempts = []DeepSWEPublicAttempt{
		pairedAttempt("c-a", "codex", "task-a", base, false),
		pairedAttempt("l-a", "luban", "task-a", base.Add(time.Millisecond), true),
		pairedAttempt("c-b", "codex", "task-b", base.Add(time.Second), true),
		pairedAttempt("l-b", "luban", "task-b", base.Add(time.Second+time.Millisecond), false),
	}
	card, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		t.Fatal(err)
	}
	paired := card.Paired
	if paired == nil || paired.CommonScored != 2 || len(paired.Rows) != 2 || paired.ChallengerOnly != 1 || paired.BaselineOnly != 1 || paired.McNemar.Discordant != 2 {
		t.Fatalf("paired comparison = %#v", paired)
	}
	if paired.PassRateDelta == nil || *paired.PassRateDelta != 0 || paired.McNemar.ExactTwoSidedP == nil || *paired.McNemar.ExactTwoSidedP != 1 {
		t.Fatalf("paired delta/McNemar = %#v", paired)
	}
}

func TestDeepSWEPublicScorerComputesPairedExclusionImbalanceBySlotAndCategory(t *testing.T) {
	base := time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC)
	scored := func(id, agent string, slot int, passed bool) DeepSWEPublicAttempt {
		return DeepSWEPublicAttempt{
			AttemptID: id, AgentID: agent, TaskID: "task-a", Slot: slot,
			StartedAt: base.Add(time.Duration(slot) * time.Second), Disposition: DeepSWEAttemptScored,
			Passed: boolPointer(passed), FailureCategory: DeepSWEFailureNone, Efficiency: completeEfficiencyFixture(),
		}
	}
	excluded := func(id, agent string, slot int, category DeepSWEFailureCategory) DeepSWEPublicAttempt {
		return DeepSWEPublicAttempt{
			AttemptID: id, AgentID: agent, TaskID: "task-a", Slot: slot,
			StartedAt: base.Add(time.Duration(slot) * time.Second), Disposition: DeepSWEAttemptExcluded,
			FailureCategory: category, Efficiency: completeEfficiencyFixture(),
		}
	}
	input := DeepSWEPublicScoringInput{
		Profile: ScoringProfileDeepSWEV11PublicCI, Repetitions: 4,
		TaskIDs: []string{"task-a"}, AgentIDs: []string{"codex", "luban"},
		BaselineAgentID: "codex", ChallengerAgentID: "luban",
		Attempts: []DeepSWEPublicAttempt{
			scored("c1", "codex", 1, true),
			excluded("c2", "codex", 2, DeepSWEFailureProviderInfrastructure),
			scored("c3", "codex", 3, false),
			excluded("c4", "codex", 4, DeepSWEFailureVerifierInfrastructure),
			scored("l1", "luban", 1, false),
			scored("l2", "luban", 2, true),
			excluded("l3", "luban", 3, DeepSWEFailureVerifierInfrastructure),
			excluded("l4", "luban", 4, DeepSWEFailureNetworkInfrastructure),
		},
	}
	card, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		t.Fatal(err)
	}
	if card.Paired == nil || card.Paired.CommonScored != 1 || len(card.Paired.Rows) != 1 {
		t.Fatalf("quality pairing did not retain only common-scored slots: %#v", card.Paired)
	}
	imbalance := card.ExclusionAnalysis.PairedImbalance
	if imbalance == nil || imbalance.RawPairs != 4 || imbalance.CommonScored != 1 || imbalance.AnyExcluded != 3 || imbalance.BothExcluded != 1 || imbalance.ChallengerOnlyExcluded != 1 || imbalance.BaselineOnlyExcluded != 1 || imbalance.DiscordantExclusionSlots != 2 || imbalance.ChallengerExcluded != 2 || imbalance.BaselineExcluded != 2 || imbalance.ChallengerMinusBaseline != 0 || imbalance.AbsoluteCountDifference != 0 {
		t.Fatalf("paired exclusion imbalance = %#v", imbalance)
	}
	if len(imbalance.Categories) != 4 || imbalance.Categories[0].Category != DeepSWEFailureProviderInfrastructure || imbalance.Categories[0].ChallengerMinusBaseline != -1 || imbalance.Categories[1].Category != DeepSWEFailureVerifierInfrastructure || imbalance.Categories[1].ChallengerMinusBaseline != 0 || imbalance.Categories[2].Category != DeepSWEFailureNetworkInfrastructure || imbalance.Categories[2].ChallengerMinusBaseline != 1 || imbalance.Categories[3].Category != DeepSWEFailureControllerInfrastructure || imbalance.Categories[3].ChallengerMinusBaseline != 0 {
		t.Fatalf("category exclusion imbalance = %#v", imbalance.Categories)
	}
	for _, score := range card.Agents {
		assertRateNear(t, score.ExclusionSensitivity.WorstCaseAllExcludedAsFailure.LivePooled.Rate, 1.0/4.0, 0)
		assertRateNear(t, score.ExclusionSensitivity.BestCaseAllExcludedAsPass.LivePooled.Rate, 3.0/4.0, 0)
	}
}

func TestFormalPublicScorerBindsExactManifestPlanAndCompleteMatrix(t *testing.T) {
	manifest := fixtureManifest(t)
	loaded := fixtureLoaded(t, manifest)
	plan, err := BuildPlan(loaded.SHA256, manifest, fixtureTasks(1))
	if err != nil {
		t.Fatal(err)
	}
	planSHA256, err := HashCanonical(plan)
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	cost := 1.25
	state := ExperimentState{
		SchemaVersion: "agentic-bench/state-v2", ManifestSHA256: loaded.SHA256, PlanSHA256: planSHA256,
		Status: ExperimentComplete,
		Oracle: map[string]OracleRecord{
			"task-a": {TaskID: "task-a", Validated: true, Verification: VerificationResult{ProtocolValid: true, Reward: 1}},
		},
		Runs: map[string]RunRecord{},
	}
	for index, entry := range plan.Entries {
		trialStartedAt := startedAt.Add(time.Duration(index) * time.Second)
		execution := &AgentExecution{
			ExitClass: "completed", StartedAt: trialStartedAt, FinishedAt: trialStartedAt.Add(time.Second),
			TrialStartedAt: trialStartedAt, TrialFinishedAt: trialStartedAt.Add(2 * time.Second),
		}
		verification := &VerificationResult{ProtocolValid: true, Reward: 1}
		metrics := &UsageMetrics{
			ProviderRequests: 1, ProviderRounds: 1, UsageReceiptObservations: 1, UsageReceiptTotal: 1,
			InputTokens: 100, CachedInputTokens: 80, OutputTokens: 20, CatalogCost: &cost,
		}
		state.Runs[RunKey(entry)] = RunRecord{
			Entry: entry, Phase: RunComplete, Attempts: 1, AttemptStartedAt: trialStartedAt,
			Disposition: DeepSWEAttemptScored, FailureCategory: DeepSWEFailureNone,
			Execution: execution, Verification: verification, Metrics: metrics,
		}
	}
	card, err := ScoreExperimentForManifest(loaded, state, plan)
	if err != nil {
		t.Fatal(err)
	}
	if card.DeepSWEPublic == nil || len(card.DeepSWEPublic.Agents) != 2 {
		t.Fatalf("formal public scorecard is incomplete: %#v", card)
	}

	excludedState := state
	excludedState.Runs = make(map[string]RunRecord, len(state.Runs))
	for key, record := range state.Runs {
		excludedState.Runs[key] = record
	}
	excludedEntry := plan.Entries[0]
	excludedRecord := excludedState.Runs[RunKey(excludedEntry)]
	excludedRecord.Disposition = DeepSWEAttemptExcluded
	excludedRecord.FailureCategory = DeepSWEFailureVerifierInfrastructure
	excludedRecord.Verification = nil
	excludedState.Runs[RunKey(excludedEntry)] = excludedRecord
	excludedCard, err := ScoreExperimentForManifest(loaded, excludedState, plan)
	if err != nil {
		t.Fatal(err)
	}
	var excludedScore *DeepSWEPublicAgentScore
	for index := range excludedCard.DeepSWEPublic.Agents {
		if excludedCard.DeepSWEPublic.Agents[index].AgentID == excludedEntry.AgentID {
			excludedScore = &excludedCard.DeepSWEPublic.Agents[index]
		}
	}
	if excludedScore == nil || excludedScore.Counts.Raw != 1 || excludedScore.Counts.Scored != 0 || excludedScore.Counts.Excluded != 1 || excludedScore.LivePooled.Rate != nil || excludedScore.AllExecutedEfficiency.TrialDurationSeconds.Observed != 1 || excludedScore.AllExecutedEfficiency.CostUSD.Observed != 1 {
		t.Fatalf("typed exclusion did not reach public scoring/all-executed efficiency: %#v", excludedScore)
	}

	timeoutState := state
	timeoutState.Runs = make(map[string]RunRecord, len(state.Runs))
	for key, record := range state.Runs {
		timeoutState.Runs[key] = record
	}
	timeoutEntry := plan.Entries[0]
	timeoutRecord := timeoutState.Runs[RunKey(timeoutEntry)]
	timeoutExecution := *timeoutRecord.Execution
	timeoutRecord.Execution = &timeoutExecution
	timeoutRecord.Execution.ExitClass = "timeout"
	timeoutState.Runs[RunKey(timeoutEntry)] = timeoutRecord
	if _, err := ScoreExperimentForManifest(loaded, timeoutState, plan); err == nil {
		t.Fatal("timeout with an inconsistent passing classification was accepted")
	}
	rawReward := timeoutRecord.Verification.Reward
	timeoutVerification := *timeoutRecord.Verification
	timeoutRecord.Verification = &timeoutVerification
	timeoutRecord.Verification.RawReward = &rawReward
	timeoutRecord.Verification.Reward = 0
	timeoutRecord.FailureCategory = DeepSWEFailureAgentTimeout
	timeoutState.Runs[RunKey(timeoutEntry)] = timeoutRecord
	timeoutCard, err := ScoreExperimentForManifest(loaded, timeoutState, plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, agentScore := range timeoutCard.DeepSWEPublic.Agents {
		if agentScore.AgentID == timeoutEntry.AgentID && (agentScore.Counts.Passed != 0 || agentScore.Counts.Failed != 1 || agentScore.ScoredFailuresByCategory[DeepSWEFailureAgentTimeout] != 1) {
			t.Fatalf("agent timeout was not scored as a failure despite a raw verifier pass: %#v", agentScore)
		}
	}

	forgedManifest := loaded
	forgedManifest.Manifest.Scoring.ChallengerAgentID = "codex"
	if _, err := ScoreExperimentForManifest(forgedManifest, state, plan); err == nil {
		t.Fatal("manifest struct differing from the hashed bytes was accepted")
	}

	omittedPlan := plan
	omittedPlan.Entries = slices.Clone(plan.Entries[:1])
	omittedState := state
	omittedState.Runs = map[string]RunRecord{RunKey(omittedPlan.Entries[0]): state.Runs[RunKey(omittedPlan.Entries[0])]}
	omittedState.PlanSHA256, err = HashCanonical(omittedPlan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ScoreExperimentForManifest(loaded, omittedState, omittedPlan); err == nil {
		t.Fatal("agent/task/repetition matrix omission was accepted")
	}

	extraState := state
	extraState.Runs = make(map[string]RunRecord, len(state.Runs)+1)
	for key, record := range state.Runs {
		extraState.Runs[key] = record
	}
	extraState.Runs["replacement/task-a/luban"] = state.Runs[RunKey(plan.Entries[0])]
	if _, err := ScoreExperimentForManifest(loaded, extraState, plan); err == nil {
		t.Fatal("replacement attempt outside the preregistered raw ledger was accepted")
	}
}

func TestNormalizeDeepSWEPublicOutcomeOverridesRawPassForAgentTerminalFailure(t *testing.T) {
	rawPass := 1.0
	for _, test := range []struct {
		name       string
		exitClass  string
		category   DeepSWEFailureCategory
		wantPassed bool
	}{
		{name: "timeout", exitClass: "timeout", category: DeepSWEFailureAgentTimeout},
		{name: "context", exitClass: "context_failure", category: DeepSWEFailureContext},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := RunRecord{
				Disposition: DeepSWEAttemptScored, FailureCategory: test.category,
				Execution:    &AgentExecution{ExitClass: test.exitClass},
				Verification: &VerificationResult{ProtocolValid: true, Reward: 0, RawReward: &rawPass},
			}
			outcome, err := NormalizeDeepSWEPublicOutcome(record)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Passed == nil || *outcome.Passed != test.wantPassed || outcome.FailureCategory != test.category || outcome.Disposition != DeepSWEAttemptScored {
				t.Fatalf("normalized terminal outcome = %#v", outcome)
			}
			record.Verification.Reward = 1
			if _, err := NormalizeDeepSWEPublicOutcome(record); err == nil {
				t.Fatal("terminal failure with non-overridden effective pass was accepted")
			}
		})
	}
	excluded, err := NormalizeDeepSWEPublicOutcome(RunRecord{
		Disposition: DeepSWEAttemptExcluded, FailureCategory: DeepSWEFailureNetworkInfrastructure,
	})
	if err != nil {
		t.Fatal(err)
	}
	if excluded.Passed != nil || excluded.Disposition != DeepSWEAttemptExcluded {
		t.Fatalf("normalized infrastructure exclusion = %#v", excluded)
	}
}

func TestFormalPublicScorerFull904AndOneRun113Contracts(t *testing.T) {
	for _, repetitions := range []int{4, 1} {
		t.Run(fmt.Sprintf("repetitions-%d", repetitions), func(t *testing.T) {
			loaded, plan, state := fullPublicScoringFixture(t, repetitions)
			wantRaw := 113 * 2 * repetitions
			if len(plan.Entries) != wantRaw || len(state.Runs) != wantRaw {
				t.Fatalf("formal raw matrix = plan %d/state %d, want %d", len(plan.Entries), len(state.Runs), wantRaw)
			}
			card, err := ScoreExperimentForManifest(loaded, state, plan)
			if err != nil {
				t.Fatal(err)
			}
			if card.DeepSWEPublic == nil || card.DeepSWEPublic.TaskCount != 113 || card.DeepSWEPublic.Repetitions != repetitions || len(card.DeepSWEPublic.Agents) != 2 || card.DeepSWEPublic.Paired == nil || card.DeepSWEPublic.Paired.CommonScored != wantRaw/2 {
				t.Fatalf("formal public matrix scorecard = %#v", card.DeepSWEPublic)
			}
			imbalance := card.DeepSWEPublic.ExclusionAnalysis.PairedImbalance
			if len(card.DeepSWEPublic.ExclusionAnalysis.Agents) != 2 || imbalance == nil || imbalance.RawPairs != 113*repetitions || imbalance.CommonScored != imbalance.RawPairs || imbalance.AnyExcluded != 0 || imbalance.DiscordantExclusionSlots != 0 || imbalance.ChallengerExcluded != 0 || imbalance.BaselineExcluded != 0 {
				t.Fatalf("no-exclusion formal matrix has exclusion imbalance: %#v", card.DeepSWEPublic.ExclusionAnalysis)
			}
			for _, score := range card.DeepSWEPublic.Agents {
				if score.Counts.Raw != 113*repetitions || score.Counts.Scored != score.Counts.Raw || score.Counts.Excluded != 0 || score.LivePooled.Rate == nil || score.TaskMacro.Rate == nil {
					t.Fatalf("formal agent score accounting = %#v", score)
				}
				sensitivity := score.ExclusionSensitivity
				if sensitivity.ExcludedAttempts != 0 || sensitivity.ExclusionRate == nil || *sensitivity.ExclusionRate != 0 || sensitivity.WorstCaseAllExcludedAsFailure.LivePooled.Rate == nil || sensitivity.BestCaseAllExcludedAsPass.LivePooled.Rate == nil || *sensitivity.WorstCaseAllExcludedAsFailure.LivePooled.Rate != *score.LivePooled.Rate || *sensitivity.BestCaseAllExcludedAsPass.LivePooled.Rate != *score.LivePooled.Rate || sensitivity.WorstCaseAllExcludedAsFailure.TaskMacro.Rate == nil || sensitivity.BestCaseAllExcludedAsPass.TaskMacro.Rate == nil || *sensitivity.WorstCaseAllExcludedAsFailure.TaskMacro.Rate != *score.TaskMacro.Rate || *sensitivity.BestCaseAllExcludedAsPass.TaskMacro.Rate != *score.TaskMacro.Rate {
					t.Fatalf("zero-exclusion bounds differ from observed quality: %#v", sensitivity)
				}
				if repetitions == 4 {
					if score.PassAt4 == nil || score.PassAt4.TotalTasks != 113 || score.PassAt4.UniverseTasks != 113 || score.PassAt4.Rate == nil || score.FourRunStatistics == nil || len(score.FourRunStatistics.Runs) != 4 || score.FourRunStatistics.SampleStandardDeviation == nil || score.FourRunStatistics.ConfidenceLower == nil || score.FourRunStatistics.ConfidenceUpper == nil {
						t.Fatalf("904-slot formal score lacks live/macro/pass@4/four-run CI: %#v", score)
					}
					if sensitivity.WorstCaseAllExcludedAsFailure.PassAt4 == nil || sensitivity.BestCaseAllExcludedAsPass.PassAt4 == nil || *sensitivity.WorstCaseAllExcludedAsFailure.PassAt4.Rate != *score.PassAt4.Rate || *sensitivity.BestCaseAllExcludedAsPass.PassAt4.Rate != *score.PassAt4.Rate {
						t.Fatalf("zero-exclusion pass@4 bounds differ from observed quality: %#v", sensitivity)
					}
				} else if score.PassAt4 != nil || score.FourRunStatistics != nil {
					t.Fatalf("1×113 formal score was mislabeled pass@4/run-CI: %#v", score)
				} else if sensitivity.WorstCaseAllExcludedAsFailure.PassAt4 != nil || sensitivity.BestCaseAllExcludedAsPass.PassAt4 != nil {
					t.Fatalf("1×113 exclusion sensitivity was mislabeled pass@4: %#v", sensitivity)
				}
			}
		})
	}
}

func fullPublicScoringFixture(t testing.TB, repetitions int) (LoadedManifest, RunPlan, ExperimentState) {
	t.Helper()
	tasks := make([]Task, 0, 113)
	for index := 0; index < 113; index++ {
		id := fmt.Sprintf("task-%03d", index)
		tasks = append(tasks, Task{
			ID: id, BaseCommit: strings.Repeat("d", 40), ManifestSHA256: strings.Repeat("a", 64),
			InstructionSHA256: strings.Repeat("b", 64), Image: "registry.example/task:" + id,
			ImageDigest: "sha256:" + strings.Repeat("c", 64),
		})
	}
	manifest := fixtureManifest(t)
	inventorySHA256, err := HashTaskInventory(tasks)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Dataset.ManifestSHA256 = inventorySHA256
	manifest.Selection = SelectionSpec{Mode: "full", ExpectedTaskCount: len(tasks)}
	manifest.Scheduling.Repetitions = repetitions
	loaded := fixtureLoaded(t, manifest)
	plan, err := BuildPlan(loaded.SHA256, manifest, tasks)
	if err != nil {
		t.Fatal(err)
	}
	planSHA256, err := HashCanonical(plan)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 26, 2, 0, 0, 0, time.UTC)
	completed := base.Add(24 * time.Hour)
	state := ExperimentState{
		SchemaVersion: "agentic-bench/state-v2", ManifestSHA256: loaded.SHA256, PlanSHA256: planSHA256,
		Status: ExperimentComplete, StartedAt: base, UpdatedAt: completed, CompletedAt: &completed,
		Oracle: map[string]OracleRecord{}, Runs: map[string]RunRecord{},
	}
	for _, task := range tasks {
		state.Oracle[task.ID] = OracleRecord{TaskID: task.ID, Validated: true, Verification: VerificationResult{ProtocolValid: true, Reward: 1}}
	}
	for _, entry := range plan.Entries {
		trialStartedAt := base.Add(time.Duration(entry.Ordinal) * time.Second)
		execution := &AgentExecution{
			ExitClass: "completed", StartedAt: trialStartedAt.Add(100 * time.Millisecond), FinishedAt: trialStartedAt.Add(900 * time.Millisecond),
			TrialStartedAt: trialStartedAt, TrialFinishedAt: trialStartedAt.Add(time.Second),
		}
		passed := entry.Repetition%2 == 0
		if entry.AgentID == "luban" {
			passed = entry.Repetition != 3
		}
		reward := 0.0
		if passed {
			reward = 1
		}
		cost := 0.01
		state.Runs[RunKey(entry)] = RunRecord{
			Entry: entry, Phase: RunComplete, Attempts: 1, AttemptStartedAt: trialStartedAt,
			Disposition: DeepSWEAttemptScored, FailureCategory: DeepSWEFailureNone, Execution: execution,
			Verification: &VerificationResult{ProtocolValid: true, Reward: reward},
			Metrics: &UsageMetrics{
				ProviderRequests: 1, ProviderRounds: 1, UsageReceiptObservations: 1, UsageReceiptTotal: 1,
				InputTokens: 100, CachedInputTokens: 80, OutputTokens: 20, CatalogCost: &cost,
			},
		}
	}
	return loaded, plan, state
}

func oneRunScoringFixture() DeepSWEPublicScoringInput {
	base := time.Date(2026, 7, 6, 18, 0, 0, 0, time.UTC)
	return DeepSWEPublicScoringInput{
		Profile: ScoringProfileDeepSWEV11PublicCI, Repetitions: 1,
		TaskIDs: []string{"task-a"}, AgentIDs: []string{"codex", "luban"},
		BaselineAgentID: "codex", ChallengerAgentID: "luban",
		Attempts: []DeepSWEPublicAttempt{
			pairedAttempt("codex-a", "codex", "task-a", base, true),
			pairedAttempt("luban-a", "luban", "task-a", base.Add(time.Millisecond), true),
		},
	}
}

func pairedAttempt(id, agentID, taskID string, startedAt time.Time, passed bool) DeepSWEPublicAttempt {
	return DeepSWEPublicAttempt{
		AttemptID: id, AgentID: agentID, TaskID: taskID, Slot: 1, StartedAt: startedAt,
		Disposition: DeepSWEAttemptScored, Passed: boolPointer(passed), FailureCategory: DeepSWEFailureNone,
		Efficiency: completeEfficiencyFixture(),
	}
}

func completeEfficiencyFixture() DeepSWEAttemptEfficiency {
	return DeepSWEAttemptEfficiency{
		AgentDurationSeconds: float64Pointer(10), TrialDurationSeconds: float64Pointer(12), CostUSD: float64Pointer(1.5),
		AgentSteps:       int64Pointer(3),
		LLMCallsStarted:  int64Pointer(3),
		ProviderRequests: int64Pointer(3), ProviderRounds: int64Pointer(3), ProviderErrors: int64Pointer(0),
		ToolInvocations: int64Pointer(4), PhysicalToolOperations: int64Pointer(5),
		InputTokens: int64Pointer(100), CachedInputTokens: int64Pointer(80), OutputTokens: int64Pointer(20),
	}
}

func boolPointer(value bool) *bool { return &value }

func assertNear(t *testing.T, actual, expected, tolerance float64) {
	t.Helper()
	if math.Abs(actual-expected) > tolerance {
		t.Fatalf("value %.17g, want %.17g (tolerance %.3g)", actual, expected, tolerance)
	}
}

func assertRateNear(t *testing.T, actual *float64, expected, tolerance float64) {
	t.Helper()
	if actual == nil {
		t.Fatal("rate is null")
	}
	assertNear(t, *actual, expected, tolerance)
}
