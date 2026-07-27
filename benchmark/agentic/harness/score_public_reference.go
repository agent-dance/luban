package harness

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"time"
)

const (
	DeepSWEGPT56SolXHighSourceURL        = "https://deepswe.datacurve.ai/artifacts/v1.1/trials.json"
	DeepSWEGPT56SolXHighSourceSHA256     = "7844056bade4cee4a2c2964c9582bf7eb1344735a28695cae7d419055656417a"
	DeepSWEGPT56SolXHighProjectionSHA256 = "55a59b36d22f1406d49c11134275293bc4970c34a4dd54a03a3b89b5635986cd"
	DeepSWEGPT56SolXHighConfig           = "mini_swe_agent_gpt_5_6_sol_xhigh"
	DeepSWEGPT56SolXHighReferenceAgentID = "official-gpt-5.6-sol-xhigh"
	DeepSWEGPT56SolXHighProjectionPath   = "harness/testdata/deepswe-v1.1-gpt-5.6-sol-xhigh-public-trials.json"
)

//go:embed testdata/deepswe-v1.1-gpt-5.6-sol-xhigh-public-trials.json
var deepSWEGPT56SolXHighProjectionJSON []byte

// DeepSWEPublicReferenceArtifact is the serializable, computed public
// reference consumed by reports. Quality fields are never copied from a web
// page: the loader verifies the frozen projection and rebuilds the scorecard
// through ScoreDeepSWEPublicCI on every load.
type DeepSWEPublicReferenceArtifact struct {
	SchemaVersion            string                           `json:"schema_version"`
	Provenance               DeepSWEPublicReferenceProvenance `json:"provenance"`
	RawRows                  int                              `json:"raw_rows"`
	ScoredRows               int                              `json:"scored_rows"`
	ExcludedRows             int                              `json:"excluded_rows"`
	PassedRows               int                              `json:"passed_rows"`
	SourceExclusions         []DeepSWEPublicSourceExclusion   `json:"source_exclusions"`
	SourceExclusionsByReason map[string]int                   `json:"source_exclusions_by_reason"`
	Scorecard                DeepSWEPublicScorecard           `json:"scorecard"`
}

type DeepSWEPublicReferenceProvenance struct {
	SourceURL          string `json:"source_url"`
	SourceSHA256       string `json:"source_sha256"`
	ProjectionPath     string `json:"projection_path"`
	ProjectionSHA256   string `json:"projection_sha256"`
	ProjectionBytes    int    `json:"projection_bytes"`
	Configuration      string `json:"configuration"`
	ProjectionSchema   string `json:"projection_schema"`
	ScoringProfile     string `json:"scoring_profile"`
	ScorecardSchema    string `json:"scorecard_schema"`
	ReferenceAgentID   string `json:"reference_agent_id"`
	ScheduledRuns      int    `json:"scheduled_runs"`
	FrozenTaskUniverse int    `json:"frozen_task_universe"`
}

type DeepSWEPublicSourceExclusion struct {
	TrialName             string                 `json:"trial_name"`
	TaskID                string                 `json:"task_id"`
	StartedAt             time.Time              `json:"started_at"`
	SourceCategory        string                 `json:"source_category"`
	PublicFailureCategory DeepSWEFailureCategory `json:"public_failure_category"`
}

type deepSWEPublicTrialProjectionFile struct {
	SchemaVersion string                         `json:"schema_version"`
	SourceURL     string                         `json:"source_url"`
	SourceSHA256  string                         `json:"source_sha256"`
	Config        string                         `json:"config"`
	Rows          []deepSWEPublicTrialProjection `json:"rows"`
}

type deepSWEPublicTrialProjection struct {
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

// LoadDeepSWEGPT56SolXHighPublicReference returns a fresh computed artifact.
// Callers may serialize it directly; mutation cannot affect a later call.
func LoadDeepSWEGPT56SolXHighPublicReference() (DeepSWEPublicReferenceArtifact, error) {
	digest := sha256.Sum256(deepSWEGPT56SolXHighProjectionJSON)
	projectionSHA256 := hex.EncodeToString(digest[:])
	if projectionSHA256 != DeepSWEGPT56SolXHighProjectionSHA256 {
		return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("DeepSWE public reference projection SHA-256 %s does not match its frozen identity", projectionSHA256)
	}
	var projection deepSWEPublicTrialProjectionFile
	decoder := json.NewDecoder(bytes.NewReader(deepSWEGPT56SolXHighProjectionJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&projection); err != nil {
		return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("decode DeepSWE public reference projection: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return DeepSWEPublicReferenceArtifact{}, errors.New("DeepSWE public reference projection has a trailing JSON value")
	}
	if projection.SchemaVersion != "deepswe-public-trial-projection/v1" || projection.SourceURL != DeepSWEGPT56SolXHighSourceURL || projection.SourceSHA256 != DeepSWEGPT56SolXHighSourceSHA256 || projection.Config != DeepSWEGPT56SolXHighConfig {
		return DeepSWEPublicReferenceArtifact{}, errors.New("DeepSWE public reference provenance differs from the frozen source contract")
	}
	if len(projection.Rows) != 452 {
		return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("DeepSWE public reference has %d raw rows, want 452", len(projection.Rows))
	}

	artifact := DeepSWEPublicReferenceArtifact{
		SchemaVersion:            "agentic-bench/deepswe-public-reference-v1",
		RawRows:                  len(projection.Rows),
		SourceExclusionsByReason: map[string]int{},
		Provenance: DeepSWEPublicReferenceProvenance{
			SourceURL: DeepSWEGPT56SolXHighSourceURL, SourceSHA256: DeepSWEGPT56SolXHighSourceSHA256,
			ProjectionPath: DeepSWEGPT56SolXHighProjectionPath, ProjectionSHA256: projectionSHA256,
			ProjectionBytes: len(deepSWEGPT56SolXHighProjectionJSON), Configuration: DeepSWEGPT56SolXHighConfig,
			ProjectionSchema: projection.SchemaVersion, ScoringProfile: ScoringProfileDeepSWEV11PublicCI,
			ScorecardSchema: "agentic-bench/deepswe-public-scorecard-v2", ReferenceAgentID: DeepSWEGPT56SolXHighReferenceAgentID,
			ScheduledRuns: 4, FrozenTaskUniverse: 113,
		},
	}
	input := DeepSWEPublicScoringInput{
		Profile: ScoringProfileDeepSWEV11PublicCI, Repetitions: 4,
		AgentIDs: []string{DeepSWEGPT56SolXHighReferenceAgentID},
	}
	byTask := make(map[string][]deepSWEPublicTrialProjection, 113)
	trialNames := make(map[string]struct{}, len(projection.Rows))
	for index, row := range projection.Rows {
		if !idPattern.MatchString(row.TaskName) || row.TrialName == "" || row.StartedAt.IsZero() {
			return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("DeepSWE public reference row %d has an invalid identity or start time", index)
		}
		if _, exists := trialNames[row.TrialName]; exists {
			return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("DeepSWE public reference trial %q is duplicated", row.TrialName)
		}
		trialNames[row.TrialName] = struct{}{}
		if row.AgentDurationSeconds == nil || row.TrialDurationSeconds == nil || row.CostUSD == nil || row.AgentSteps == nil || row.InputTokens == nil || row.CacheTokens == nil || row.OutputTokens == nil {
			return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("DeepSWE public reference trial %q has incomplete projected efficiency", row.TrialName)
		}
		byTask[row.TaskName] = append(byTask[row.TaskName], row)
	}
	for taskID := range byTask {
		input.TaskIDs = append(input.TaskIDs, taskID)
	}
	slices.Sort(input.TaskIDs)
	if len(input.TaskIDs) != artifact.Provenance.FrozenTaskUniverse {
		return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("DeepSWE public reference has %d tasks, want %d", len(input.TaskIDs), artifact.Provenance.FrozenTaskUniverse)
	}
	for _, taskID := range input.TaskIDs {
		rows := byTask[taskID]
		sort.Slice(rows, func(i, j int) bool { return rows[i].StartedAt.Before(rows[j].StartedAt) })
		if len(rows) != artifact.Provenance.ScheduledRuns {
			return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("DeepSWE public reference task %s has %d raw rows, want %d", taskID, len(rows), artifact.Provenance.ScheduledRuns)
		}
		for index, row := range rows {
			attempt := DeepSWEPublicAttempt{
				AttemptID: row.TrialName, AgentID: DeepSWEGPT56SolXHighReferenceAgentID,
				TaskID: taskID, Slot: index + 1, StartedAt: row.StartedAt,
				Efficiency: DeepSWEAttemptEfficiency{
					AgentDurationSeconds: row.AgentDurationSeconds, TrialDurationSeconds: row.TrialDurationSeconds,
					CostUSD: row.CostUSD, AgentSteps: row.AgentSteps, InputTokens: row.InputTokens,
					CachedInputTokens: row.CacheTokens, OutputTokens: row.OutputTokens,
				},
			}
			if row.IncludedInScore {
				if row.ErrorCategory != nil {
					return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("included DeepSWE public reference trial %q has an error category", row.TrialName)
				}
				attempt.Disposition = DeepSWEAttemptScored
				attempt.Passed = booleanPointer(row.Passed)
				attempt.FailureCategory = DeepSWEFailureNone
				artifact.ScoredRows++
				if row.Passed {
					artifact.PassedRows++
				}
			} else {
				if row.ErrorCategory == nil || *row.ErrorCategory != "verifier_timeout" {
					return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("DeepSWE public reference trial %q has an unsupported source exclusion", row.TrialName)
				}
				attempt.Disposition = DeepSWEAttemptExcluded
				attempt.FailureCategory = DeepSWEFailureVerifierInfrastructure
				artifact.ExcludedRows++
				artifact.SourceExclusionsByReason[*row.ErrorCategory]++
				artifact.SourceExclusions = append(artifact.SourceExclusions, DeepSWEPublicSourceExclusion{
					TrialName: row.TrialName, TaskID: taskID, StartedAt: row.StartedAt,
					SourceCategory: *row.ErrorCategory, PublicFailureCategory: DeepSWEFailureVerifierInfrastructure,
				})
			}
			input.Attempts = append(input.Attempts, attempt)
		}
	}
	if artifact.ScoredRows+artifact.ExcludedRows != artifact.RawRows || artifact.ScoredRows != 451 || artifact.ExcludedRows != 1 || artifact.PassedRows != 319 || artifact.SourceExclusionsByReason["verifier_timeout"] != 1 {
		return DeepSWEPublicReferenceArtifact{}, errors.New("DeepSWE public reference row accounting differs from the frozen official artifact")
	}
	scorecard, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		return DeepSWEPublicReferenceArtifact{}, fmt.Errorf("score DeepSWE public reference: %w", err)
	}
	artifact.Scorecard = scorecard
	if err := validateDeepSWEGPT56SolXHighReferenceScore(artifact); err != nil {
		return DeepSWEPublicReferenceArtifact{}, err
	}
	return artifact, nil
}

func validateDeepSWEGPT56SolXHighReferenceScore(artifact DeepSWEPublicReferenceArtifact) error {
	if artifact.Scorecard.SchemaVersion != artifact.Provenance.ScorecardSchema || artifact.Scorecard.Profile != artifact.Provenance.ScoringProfile || artifact.Scorecard.Repetitions != artifact.Provenance.ScheduledRuns || artifact.Scorecard.TaskCount != artifact.Provenance.FrozenTaskUniverse || len(artifact.Scorecard.Agents) != 1 {
		return errors.New("computed DeepSWE public reference scorecard violates its provenance contract")
	}
	score := artifact.Scorecard.Agents[0]
	if score.AgentID != artifact.Provenance.ReferenceAgentID || score.Counts.Raw != artifact.RawRows || score.Counts.Scored != artifact.ScoredRows || score.Counts.Excluded != artifact.ExcludedRows || score.Counts.Passed != artifact.PassedRows || score.Counts.Failed != artifact.ScoredRows-artifact.PassedRows {
		return errors.New("computed DeepSWE public reference attempt counts differ from its source rows")
	}
	if score.LivePooled.Numerator != float64(artifact.PassedRows) || score.LivePooled.Denominator != artifact.ScoredRows || score.LivePooled.Rate == nil || score.PassAt4 == nil || score.FourRunStatistics == nil {
		return errors.New("computed DeepSWE public reference is missing public quality statistics")
	}
	if math.Abs(*score.LivePooled.Rate-319.0/451.0) > 1e-15 || score.TaskMacro.Numerator != 80 || score.TaskMacro.Denominator != 113 || score.TaskMacro.Rate == nil || math.Abs(*score.TaskMacro.Rate-80.0/113.0) > 1e-15 || score.PassAt4.PassedTasks != 97 || score.PassAt4.TotalTasks != 113 || score.PassAt4.UniverseTasks != 113 || score.PassAt4.Rate == nil || math.Abs(*score.PassAt4.Rate-97.0/113.0) > 1e-15 {
		return errors.New("computed DeepSWE public reference quality differs from the frozen official expected values")
	}
	sensitivity := score.ExclusionSensitivity
	if sensitivity.RawAttempts != 452 || sensitivity.ScoredAttempts != 451 || sensitivity.ExcludedAttempts != 1 || sensitivity.ExclusionRate == nil || math.Abs(*sensitivity.ExclusionRate-1.0/452.0) > 1e-15 || sensitivity.WorstCaseAllExcludedAsFailure.LivePooled.Rate == nil || math.Abs(*sensitivity.WorstCaseAllExcludedAsFailure.LivePooled.Rate-319.0/452.0) > 1e-15 || sensitivity.BestCaseAllExcludedAsPass.LivePooled.Rate == nil || math.Abs(*sensitivity.BestCaseAllExcludedAsPass.LivePooled.Rate-320.0/452.0) > 1e-15 || sensitivity.WorstCaseAllExcludedAsFailure.TaskMacro.Rate == nil || math.Abs(*sensitivity.WorstCaseAllExcludedAsFailure.TaskMacro.Rate-79.75/113.0) > 1e-15 || sensitivity.BestCaseAllExcludedAsPass.TaskMacro.Rate == nil || math.Abs(*sensitivity.BestCaseAllExcludedAsPass.TaskMacro.Rate-80.0/113.0) > 1e-15 || sensitivity.WorstCaseAllExcludedAsFailure.PassAt4 == nil || sensitivity.BestCaseAllExcludedAsPass.PassAt4 == nil || sensitivity.WorstCaseAllExcludedAsFailure.PassAt4.PassedTasks != 97 || sensitivity.BestCaseAllExcludedAsPass.PassAt4.PassedTasks != 97 {
		return errors.New("computed DeepSWE public reference exclusion sensitivity differs from the frozen official expected values")
	}
	if len(artifact.Scorecard.ExclusionAnalysis.Agents) != 1 || artifact.Scorecard.ExclusionAnalysis.Agents[0].AgentID != artifact.Provenance.ReferenceAgentID || artifact.Scorecard.ExclusionAnalysis.Agents[0].ExcludedByCategory[DeepSWEFailureVerifierInfrastructure] != 1 || artifact.Scorecard.ExclusionAnalysis.PairedImbalance != nil {
		return errors.New("computed DeepSWE public reference exclusion accounting differs from the frozen official artifact")
	}
	runPassed, runScored := []int{81, 80, 80, 78}, []int{113, 113, 113, 112}
	statistics := score.FourRunStatistics
	if len(statistics.Runs) != 4 || statistics.RunMean == nil || statistics.SampleStandardDeviation == nil || statistics.ConfidenceCenter == nil || statistics.ConfidenceLower == nil || statistics.ConfidenceUpper == nil || statistics.ConfidenceHalfWidth == nil || statistics.HalfWidthPercentagePoints == nil {
		return errors.New("computed DeepSWE public reference lacks the four-run confidence contract")
	}
	for index, run := range statistics.Runs {
		if run.Run != index+1 || run.Passed != runPassed[index] || run.Scored != runScored[index] || run.Rate == nil {
			return errors.New("computed DeepSWE public reference run samples differ from the frozen official expected values")
		}
	}
	expected := [][2]float64{
		{*statistics.RunMean, 0.7072929835651074},
		{*statistics.SampleStandardDeviation, 0.008358436463071354},
		{*statistics.ConfidenceCenter, 319.0 / 451.0},
		{*statistics.ConfidenceLower, 0.6991259559533886},
		{*statistics.ConfidenceUpper, 0.7155081903880748},
		{*statistics.ConfidenceHalfWidth, 0.008191117217343103},
		{*statistics.HalfWidthPercentagePoints, 0.8191117217343103},
	}
	for _, pair := range expected {
		if math.Abs(pair[0]-pair[1]) > 1e-13 {
			return errors.New("computed DeepSWE public reference confidence interval differs from the frozen official expected values")
		}
	}
	return nil
}

func booleanPointer(value bool) *bool { return &value }
