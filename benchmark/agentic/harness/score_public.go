package harness

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

const normal95Z = 1.959963984540054

type DeepSWEAttemptDisposition string

const (
	DeepSWEAttemptScored   DeepSWEAttemptDisposition = "scored"
	DeepSWEAttemptExcluded DeepSWEAttemptDisposition = "excluded"
)

type DeepSWEFailureCategory string

const (
	DeepSWEFailureNone                     DeepSWEFailureCategory = "none"
	DeepSWEFailureAgentTimeout             DeepSWEFailureCategory = "agent_timeout"
	DeepSWEFailureContext                  DeepSWEFailureCategory = "context_failure"
	DeepSWEFailureProviderInfrastructure   DeepSWEFailureCategory = "provider_infrastructure"
	DeepSWEFailureVerifierInfrastructure   DeepSWEFailureCategory = "verifier_infrastructure"
	DeepSWEFailureNetworkInfrastructure    DeepSWEFailureCategory = "network_infrastructure"
	DeepSWEFailureControllerInfrastructure DeepSWEFailureCategory = "controller_infrastructure"
)

// DeepSWEPublicScoringInput is the content-complete, normalized boundary of
// the public scorer. Adapters must classify failures before this boundary; the
// scorer deliberately never guesses from an error string or an exit code.
type DeepSWEPublicScoringInput struct {
	Profile           string                 `json:"profile"`
	Repetitions       int                    `json:"repetitions"`
	TaskIDs           []string               `json:"task_ids"`
	AgentIDs          []string               `json:"agent_ids"`
	BaselineAgentID   string                 `json:"baseline_agent_id,omitempty"`
	ChallengerAgentID string                 `json:"challenger_agent_id,omitempty"`
	Attempts          []DeepSWEPublicAttempt `json:"attempts"`
}

type DeepSWEPublicAttempt struct {
	AttemptID       string                    `json:"attempt_id"`
	AgentID         string                    `json:"agent_id"`
	TaskID          string                    `json:"task_id"`
	Slot            int                       `json:"slot"`
	StartedAt       time.Time                 `json:"started_at"`
	Disposition     DeepSWEAttemptDisposition `json:"disposition"`
	Passed          *bool                     `json:"passed"`
	FailureCategory DeepSWEFailureCategory    `json:"failure_category"`
	Efficiency      DeepSWEAttemptEfficiency  `json:"efficiency"`
}

// Every efficiency field is optional independently. Nil means unobserved and
// is preserved as null in aggregates; absence is never coerced to zero.
type DeepSWEAttemptEfficiency struct {
	AgentDurationSeconds   *float64 `json:"agent_duration_seconds"`
	TrialDurationSeconds   *float64 `json:"trial_duration_seconds"`
	CostUSD                *float64 `json:"cost_usd"`
	AgentSteps             *int64   `json:"agent_steps"`
	LLMCallsStarted        *int64   `json:"llm_calls_started"`
	ProviderRequests       *int64   `json:"provider_requests"`
	ProviderRounds         *int64   `json:"provider_rounds"`
	ProviderErrors         *int64   `json:"provider_errors"`
	ToolInvocations        *int64   `json:"tool_invocations"`
	PhysicalToolOperations *int64   `json:"physical_tool_operations"`
	InputTokens            *int64   `json:"input_tokens"`
	CachedInputTokens      *int64   `json:"cached_input_tokens"`
	OutputTokens           *int64   `json:"output_tokens"`
}

type DeepSWEPublicScorecard struct {
	SchemaVersion     string                         `json:"schema_version"`
	Profile           string                         `json:"profile"`
	Repetitions       int                            `json:"repetitions"`
	TaskCount         int                            `json:"task_count"`
	Agents            []DeepSWEPublicAgentScore      `json:"agents"`
	ExclusionAnalysis DeepSWEPublicExclusionAnalysis `json:"exclusion_analysis"`
	Paired            *DeepSWEPublicPairedComparison `json:"paired"`
}

type DeepSWEPublicAgentScore struct {
	AgentID                       string                         `json:"agent_id"`
	Counts                        DeepSWEAttemptCounts           `json:"counts"`
	ExcludedByCategory            map[DeepSWEFailureCategory]int `json:"excluded_by_category"`
	ScoredFailuresByCategory      map[DeepSWEFailureCategory]int `json:"scored_failures_by_category"`
	LivePooled                    DeepSWERate                    `json:"live_pooled"`
	TaskMacro                     DeepSWERate                    `json:"task_macro"`
	PassAt4                       *DeepSWEPassAtK                `json:"pass_at_4"`
	FourRunStatistics             *DeepSWEFourRunStatistics      `json:"four_run_statistics"`
	ExclusionSensitivity          DeepSWEExclusionSensitivity    `json:"exclusion_sensitivity"`
	ScoredCountDistributionByTask map[int]int                    `json:"scored_count_distribution_by_task"`
	AllExecutedEfficiency         DeepSWEAllExecutedEfficiency   `json:"all_executed_efficiency"`
	Tasks                         []DeepSWETaskScore             `json:"tasks"`
}

type DeepSWEAttemptCounts struct {
	Raw      int `json:"raw"`
	Scored   int `json:"scored"`
	Excluded int `json:"excluded"`
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
}

type DeepSWERate struct {
	Numerator   float64  `json:"numerator"`
	Denominator int      `json:"denominator"`
	Rate        *float64 `json:"rate"`
}

type DeepSWEPassAtK struct {
	K             int      `json:"k"`
	PassedTasks   int      `json:"passed_tasks"`
	TotalTasks    int      `json:"total_tasks"`
	UniverseTasks int      `json:"universe_tasks"`
	Rate          *float64 `json:"rate"`
	Method        string   `json:"method"`
}

// DeepSWEExclusionSensitivity gives the identified quality interval when the
// unobserved infrastructure-excluded outcomes are assigned their two extreme
// values. The conservative scenario is also the requested all-excluded-as-
// failure score. PassAt4 remains nil for a one-run experiment.
type DeepSWEExclusionSensitivity struct {
	RawAttempts                   int                    `json:"raw_attempts"`
	ScoredAttempts                int                    `json:"scored_attempts"`
	ExcludedAttempts              int                    `json:"excluded_attempts"`
	ExclusionRate                 *float64               `json:"exclusion_rate"`
	WorstCaseAllExcludedAsFailure DeepSWEQualityScenario `json:"worst_case_all_excluded_as_failure"`
	BestCaseAllExcludedAsPass     DeepSWEQualityScenario `json:"best_case_all_excluded_as_pass"`
}

type DeepSWEQualityScenario struct {
	LivePooled DeepSWERate     `json:"live_pooled"`
	TaskMacro  DeepSWERate     `json:"task_macro"`
	PassAt4    *DeepSWEPassAtK `json:"pass_at_4"`
}

// DeepSWEPublicExclusionAnalysis exposes counts and paired imbalance without
// requiring report code to reconstruct the raw matrix.
type DeepSWEPublicExclusionAnalysis struct {
	Agents          []DeepSWEAgentExclusionSummary `json:"agents"`
	PairedImbalance *DeepSWEExclusionImbalance     `json:"paired_imbalance"`
}

type DeepSWEAgentExclusionSummary struct {
	AgentID            string                         `json:"agent_id"`
	Raw                int                            `json:"raw"`
	Scored             int                            `json:"scored"`
	Excluded           int                            `json:"excluded"`
	ExclusionRate      *float64                       `json:"exclusion_rate"`
	ExcludedByCategory map[DeepSWEFailureCategory]int `json:"excluded_by_category"`
}

type DeepSWEExclusionImbalance struct {
	ChallengerAgentID        string                               `json:"challenger_agent_id"`
	BaselineAgentID          string                               `json:"baseline_agent_id"`
	RawPairs                 int                                  `json:"raw_pairs"`
	CommonScored             int                                  `json:"common_scored"`
	AnyExcluded              int                                  `json:"any_excluded"`
	BothExcluded             int                                  `json:"both_excluded"`
	ChallengerOnlyExcluded   int                                  `json:"challenger_only_excluded"`
	BaselineOnlyExcluded     int                                  `json:"baseline_only_excluded"`
	DiscordantExclusionSlots int                                  `json:"discordant_exclusion_slots"`
	ChallengerExcluded       int                                  `json:"challenger_excluded"`
	BaselineExcluded         int                                  `json:"baseline_excluded"`
	ChallengerMinusBaseline  int                                  `json:"challenger_minus_baseline"`
	AbsoluteCountDifference  int                                  `json:"absolute_count_difference"`
	ChallengerExclusionRate  *float64                             `json:"challenger_exclusion_rate"`
	BaselineExclusionRate    *float64                             `json:"baseline_exclusion_rate"`
	ExclusionRateDelta       *float64                             `json:"exclusion_rate_delta_challenger_minus_baseline"`
	Categories               []DeepSWEExclusionCategoryComparison `json:"categories"`
}

type DeepSWEExclusionCategoryComparison struct {
	Category                DeepSWEFailureCategory `json:"category"`
	ChallengerExcluded      int                    `json:"challenger_excluded"`
	BaselineExcluded        int                    `json:"baseline_excluded"`
	ChallengerMinusBaseline int                    `json:"challenger_minus_baseline"`
	AbsoluteCountDifference int                    `json:"absolute_count_difference"`
}

type DeepSWEFourRunStatistics struct {
	Runs                      []DeepSWERunSample `json:"runs"`
	RunMean                   *float64           `json:"run_mean"`
	SampleStandardDeviation   *float64           `json:"sample_standard_deviation"`
	Z                         float64            `json:"z"`
	ConfidenceCenter          *float64           `json:"confidence_center"`
	ConfidenceLower           *float64           `json:"confidence_lower"`
	ConfidenceUpper           *float64           `json:"confidence_upper"`
	ConfidenceHalfWidth       *float64           `json:"confidence_half_width"`
	HalfWidthPercentagePoints *float64           `json:"half_width_percentage_points"`
}

type DeepSWERunSample struct {
	Run    int      `json:"run"`
	Passed int      `json:"passed"`
	Scored int      `json:"scored"`
	Rate   *float64 `json:"rate"`
}

type DeepSWETaskScore struct {
	TaskID           string                   `json:"task_id"`
	Raw              int                      `json:"raw"`
	Scored           int                      `json:"scored"`
	Excluded         int                      `json:"excluded"`
	Passed           int                      `json:"passed"`
	Rate             *float64                 `json:"rate"`
	ScoredAttempts   []DeepSWEScoredAttempt   `json:"scored_attempts"`
	ExcludedAttempts []DeepSWEExcludedAttempt `json:"excluded_attempts"`
}

type DeepSWEScoredAttempt struct {
	AttemptID       string                 `json:"attempt_id"`
	Slot            int                    `json:"slot"`
	Run             int                    `json:"run"`
	StartedAt       time.Time              `json:"started_at"`
	Passed          bool                   `json:"passed"`
	FailureCategory DeepSWEFailureCategory `json:"failure_category"`
}

type DeepSWEExcludedAttempt struct {
	AttemptID       string                 `json:"attempt_id"`
	Slot            int                    `json:"slot"`
	StartedAt       time.Time              `json:"started_at"`
	FailureCategory DeepSWEFailureCategory `json:"failure_category"`
}

type DeepSWEAllExecutedEfficiency struct {
	Attempts               int                   `json:"attempts"`
	AgentDurationSeconds   DeepSWEFloatAggregate `json:"agent_duration_seconds"`
	TrialDurationSeconds   DeepSWEFloatAggregate `json:"trial_duration_seconds"`
	CostUSD                DeepSWEFloatAggregate `json:"cost_usd"`
	AgentSteps             DeepSWEIntAggregate   `json:"agent_steps"`
	LLMCallsStarted        DeepSWEIntAggregate   `json:"llm_calls_started"`
	ProviderRequests       DeepSWEIntAggregate   `json:"provider_requests"`
	ProviderRounds         DeepSWEIntAggregate   `json:"provider_rounds"`
	ProviderErrors         DeepSWEIntAggregate   `json:"provider_errors"`
	ToolInvocations        DeepSWEIntAggregate   `json:"tool_invocations"`
	PhysicalToolOperations DeepSWEIntAggregate   `json:"physical_tool_operations"`
	InputTokens            DeepSWEIntAggregate   `json:"input_tokens"`
	CachedInputTokens      DeepSWEIntAggregate   `json:"cached_input_tokens"`
	OutputTokens           DeepSWEIntAggregate   `json:"output_tokens"`
	TokenWeightedCacheRate *float64              `json:"token_weighted_cache_rate"`
}

type DeepSWEFloatAggregate struct {
	Observed int      `json:"observed"`
	Total    int      `json:"total"`
	Sum      *float64 `json:"sum"`
	Mean     *float64 `json:"mean"`
}

type DeepSWEIntAggregate struct {
	Observed int      `json:"observed"`
	Total    int      `json:"total"`
	Sum      *int64   `json:"sum"`
	Mean     *float64 `json:"mean"`
}

type DeepSWEPublicPairedComparison struct {
	ChallengerAgentID string                   `json:"challenger_agent_id"`
	BaselineAgentID   string                   `json:"baseline_agent_id"`
	PairingMethod     string                   `json:"pairing_method"`
	CommonScored      int                      `json:"common_scored"`
	ChallengerPassed  int                      `json:"challenger_passed"`
	BaselinePassed    int                      `json:"baseline_passed"`
	PassRateDelta     *float64                 `json:"pass_rate_delta_challenger_minus_baseline"`
	BothPass          int                      `json:"both_pass"`
	ChallengerOnly    int                      `json:"challenger_only"`
	BaselineOnly      int                      `json:"baseline_only"`
	BothFail          int                      `json:"both_fail"`
	McNemar           DeepSWEMcNemarInputs     `json:"mcnemar"`
	Rows              []DeepSWEPublicPairedRow `json:"rows"`
}

type DeepSWEMcNemarInputs struct {
	BChallengerOnly int      `json:"b_challenger_only"`
	CBaselineOnly   int      `json:"c_baseline_only"`
	Discordant      int      `json:"discordant"`
	ExactTwoSidedP  *float64 `json:"exact_two_sided_p"`
}

type DeepSWEPublicPairedRow struct {
	TaskID           string `json:"task_id"`
	Slot             int    `json:"slot"`
	ChallengerPassed bool   `json:"challenger_passed"`
	BaselinePassed   bool   `json:"baseline_passed"`
}

// ScoreDeepSWEPublicCI implements the DeepSWE v1.1 public live-score rules.
// Infrastructure exclusions remain in raw/all-executed efficiency and are not
// replaced. Agent timeout and context failures remain scored failures.
func ScoreDeepSWEPublicCI(input DeepSWEPublicScoringInput) (DeepSWEPublicScorecard, error) {
	if err := validateDeepSWEPublicInput(input); err != nil {
		return DeepSWEPublicScorecard{}, err
	}

	card := DeepSWEPublicScorecard{
		SchemaVersion: "agentic-bench/deepswe-public-scorecard-v2",
		Profile:       input.Profile,
		Repetitions:   input.Repetitions,
		TaskCount:     len(input.TaskIDs),
	}
	tasks := slices.Clone(input.TaskIDs)
	slices.Sort(tasks)

	byAgentTask := make(map[string][]DeepSWEPublicAttempt, len(input.AgentIDs)*len(tasks))
	byAgentSlot := make(map[string]DeepSWEPublicAttempt, len(input.Attempts))
	for _, attempt := range input.Attempts {
		byAgentTask[agentTaskKey(attempt.AgentID, attempt.TaskID)] = append(byAgentTask[agentTaskKey(attempt.AgentID, attempt.TaskID)], attempt)
		byAgentSlot[agentTaskSlotKey(attempt.AgentID, attempt.TaskID, attempt.Slot)] = attempt
	}

	for _, agentID := range input.AgentIDs {
		score, err := scoreDeepSWEAgent(agentID, tasks, input.Repetitions, byAgentTask)
		if err != nil {
			return DeepSWEPublicScorecard{}, err
		}
		card.Agents = append(card.Agents, score)
		card.ExclusionAnalysis.Agents = append(card.ExclusionAnalysis.Agents, exclusionSummaryFromScore(score))
	}
	if input.BaselineAgentID != "" || input.ChallengerAgentID != "" {
		paired := scoreDeepSWEPairs(input, tasks, byAgentSlot)
		card.Paired = &paired
		imbalance := scoreDeepSWEExclusionImbalance(input, tasks, byAgentSlot)
		card.ExclusionAnalysis.PairedImbalance = &imbalance
	}
	return card, nil
}

func validateDeepSWEPublicInput(input DeepSWEPublicScoringInput) error {
	if input.Profile != ScoringProfileDeepSWEV11PublicCI {
		return fmt.Errorf("unsupported public scoring profile %q", input.Profile)
	}
	if input.Repetitions != 1 && input.Repetitions != 4 {
		return errors.New("deepswe-v1.1-public-ci requires exactly one or four scheduled repetitions")
	}
	if len(input.TaskIDs) == 0 || len(input.AgentIDs) == 0 {
		return errors.New("public scorer requires explicit task and agent universes")
	}
	tasks, err := validatedIDSet("task", input.TaskIDs)
	if err != nil {
		return err
	}
	agents, err := validatedIDSet("agent", input.AgentIDs)
	if err != nil {
		return err
	}
	if (input.BaselineAgentID == "") != (input.ChallengerAgentID == "") {
		return errors.New("baseline and challenger must either both be set or both be absent")
	}
	if input.BaselineAgentID != "" {
		if input.BaselineAgentID == input.ChallengerAgentID {
			return errors.New("baseline and challenger must differ")
		}
		if _, ok := agents[input.BaselineAgentID]; !ok {
			return errors.New("baseline agent is outside the scoring universe")
		}
		if _, ok := agents[input.ChallengerAgentID]; !ok {
			return errors.New("challenger agent is outside the scoring universe")
		}
	}
	wantAttempts := len(tasks) * len(agents) * input.Repetitions
	if len(input.Attempts) != wantAttempts {
		return fmt.Errorf("raw attempt count is %d, want the preregistered matrix size %d", len(input.Attempts), wantAttempts)
	}
	attemptIDs := map[string]struct{}{}
	matrix := map[string]struct{}{}
	starts := map[string]map[time.Time]struct{}{}
	for index, attempt := range input.Attempts {
		prefix := fmt.Sprintf("attempt[%d]", index)
		if attempt.AttemptID == "" {
			return fmt.Errorf("%s has no immutable attempt ID", prefix)
		}
		if _, exists := attemptIDs[attempt.AttemptID]; exists {
			return fmt.Errorf("attempt ID %q is duplicated", attempt.AttemptID)
		}
		attemptIDs[attempt.AttemptID] = struct{}{}
		if _, ok := agents[attempt.AgentID]; !ok {
			return fmt.Errorf("%s names an agent outside the scoring universe", prefix)
		}
		if _, ok := tasks[attempt.TaskID]; !ok {
			return fmt.Errorf("%s names a task outside the scoring universe", prefix)
		}
		if attempt.Slot < 1 || attempt.Slot > input.Repetitions {
			return fmt.Errorf("%s slot is outside the preregistered repetitions", prefix)
		}
		key := agentTaskSlotKey(attempt.AgentID, attempt.TaskID, attempt.Slot)
		if _, exists := matrix[key]; exists {
			return fmt.Errorf("attempt matrix slot %q is duplicated", key)
		}
		matrix[key] = struct{}{}
		if attempt.StartedAt.IsZero() {
			return fmt.Errorf("%s has no started_at", prefix)
		}
		groupKey := agentTaskKey(attempt.AgentID, attempt.TaskID)
		if starts[groupKey] == nil {
			starts[groupKey] = map[time.Time]struct{}{}
		}
		if _, exists := starts[groupKey][attempt.StartedAt]; exists {
			return fmt.Errorf("%s has an ambiguous duplicate started_at within its task", prefix)
		}
		starts[groupKey][attempt.StartedAt] = struct{}{}
		if err := validateDeepSWEAttemptClassification(attempt); err != nil {
			return fmt.Errorf("%s: %w", prefix, err)
		}
		if err := validateDeepSWEEfficiency(attempt.Efficiency); err != nil {
			return fmt.Errorf("%s efficiency: %w", prefix, err)
		}
	}
	return nil
}

func validatedIDSet(kind string, ids []string) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !idPattern.MatchString(id) {
			return nil, fmt.Errorf("%s ID %q is invalid", kind, id)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("%s ID %q is duplicated", kind, id)
		}
		seen[id] = struct{}{}
	}
	return seen, nil
}

func validateDeepSWEAttemptClassification(attempt DeepSWEPublicAttempt) error {
	switch attempt.Disposition {
	case DeepSWEAttemptScored:
		if attempt.Passed == nil {
			return errors.New("scored attempt has null passed")
		}
		if *attempt.Passed {
			if attempt.FailureCategory != DeepSWEFailureNone {
				return errors.New("passing attempt has a failure category")
			}
			return nil
		}
		switch attempt.FailureCategory {
		case DeepSWEFailureNone, DeepSWEFailureAgentTimeout, DeepSWEFailureContext:
			return nil
		default:
			return errors.New("scored failure uses an infrastructure exclusion category")
		}
	case DeepSWEAttemptExcluded:
		if attempt.Passed != nil {
			return errors.New("excluded infrastructure attempt must have null passed")
		}
		switch attempt.FailureCategory {
		case DeepSWEFailureProviderInfrastructure, DeepSWEFailureVerifierInfrastructure, DeepSWEFailureNetworkInfrastructure, DeepSWEFailureControllerInfrastructure:
			return nil
		default:
			return errors.New("only a typed infrastructure category may be excluded")
		}
	default:
		return errors.New("attempt disposition is invalid")
	}
}

func validateDeepSWEEfficiency(e DeepSWEAttemptEfficiency) error {
	for name, value := range map[string]*float64{
		"agent_duration_seconds": e.AgentDurationSeconds,
		"trial_duration_seconds": e.TrialDurationSeconds,
		"cost_usd":               e.CostUSD,
	} {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	for name, value := range map[string]*int64{
		"agent_steps":              e.AgentSteps,
		"llm_calls_started":        e.LLMCallsStarted,
		"provider_requests":        e.ProviderRequests,
		"provider_rounds":          e.ProviderRounds,
		"provider_errors":          e.ProviderErrors,
		"tool_invocations":         e.ToolInvocations,
		"physical_tool_operations": e.PhysicalToolOperations,
		"input_tokens":             e.InputTokens,
		"cached_input_tokens":      e.CachedInputTokens,
		"output_tokens":            e.OutputTokens,
	} {
		if value != nil && *value < 0 {
			return fmt.Errorf("%s is negative", name)
		}
	}
	if e.InputTokens != nil && e.CachedInputTokens != nil && *e.CachedInputTokens > *e.InputTokens {
		return errors.New("cached input tokens exceed input tokens")
	}
	if e.ProviderRequests != nil && e.ProviderRounds != nil && *e.ProviderRounds > *e.ProviderRequests {
		return errors.New("provider rounds exceed provider requests")
	}
	if e.ProviderRequests != nil && e.ProviderErrors != nil && *e.ProviderErrors > *e.ProviderRequests {
		return errors.New("provider errors exceed provider requests")
	}
	return nil
}

func scoreDeepSWEAgent(agentID string, tasks []string, repetitions int, grouped map[string][]DeepSWEPublicAttempt) (DeepSWEPublicAgentScore, error) {
	score := DeepSWEPublicAgentScore{
		AgentID:                       agentID,
		ExcludedByCategory:            map[DeepSWEFailureCategory]int{},
		ScoredFailuresByCategory:      map[DeepSWEFailureCategory]int{},
		ScoredCountDistributionByTask: map[int]int{},
	}
	runPassed := make([]int, repetitions)
	runScored := make([]int, repetitions)
	macroNumerator := 0.0
	macroTasks := 0
	passedTasks := 0
	worstCaseMacroNumerator := 0.0
	bestCaseMacroNumerator := 0.0
	worstCasePassedTasks := 0
	bestCasePassedTasks := 0
	allAttempts := make([]DeepSWEPublicAttempt, 0, len(tasks)*repetitions)

	for _, taskID := range tasks {
		attempts := slices.Clone(grouped[agentTaskKey(agentID, taskID)])
		sort.Slice(attempts, func(i, j int) bool { return attempts[i].StartedAt.Before(attempts[j].StartedAt) })
		task := DeepSWETaskScore{TaskID: taskID, Raw: len(attempts)}
		for _, attempt := range attempts {
			allAttempts = append(allAttempts, attempt)
			score.Counts.Raw++
			if attempt.Disposition == DeepSWEAttemptExcluded {
				task.Excluded++
				score.Counts.Excluded++
				score.ExcludedByCategory[attempt.FailureCategory]++
				task.ExcludedAttempts = append(task.ExcludedAttempts, DeepSWEExcludedAttempt{
					AttemptID: attempt.AttemptID, Slot: attempt.Slot, StartedAt: attempt.StartedAt,
					FailureCategory: attempt.FailureCategory,
				})
				continue
			}
			run := task.Scored + 1
			passed := *attempt.Passed
			task.Scored++
			score.Counts.Scored++
			runScored[run-1]++
			if passed {
				task.Passed++
				score.Counts.Passed++
				runPassed[run-1]++
			} else {
				score.Counts.Failed++
				score.ScoredFailuresByCategory[attempt.FailureCategory]++
			}
			task.ScoredAttempts = append(task.ScoredAttempts, DeepSWEScoredAttempt{
				AttemptID: attempt.AttemptID, Slot: attempt.Slot, Run: run, StartedAt: attempt.StartedAt,
				Passed: passed, FailureCategory: attempt.FailureCategory,
			})
		}
		if task.Scored > 0 {
			rate := float64(task.Passed) / float64(task.Scored)
			task.Rate = float64Pointer(rate)
			macroNumerator += rate
			macroTasks++
			if task.Passed > 0 {
				passedTasks++
			}
		}
		worstCaseMacroNumerator += float64(task.Passed) / float64(task.Raw)
		bestCaseMacroNumerator += float64(task.Passed+task.Excluded) / float64(task.Raw)
		if task.Passed > 0 {
			worstCasePassedTasks++
		}
		if task.Passed > 0 || task.Excluded > 0 {
			bestCasePassedTasks++
		}
		score.ScoredCountDistributionByTask[task.Scored]++
		score.Tasks = append(score.Tasks, task)
	}
	if score.Counts.Raw != len(tasks)*repetitions || score.Counts.Scored+score.Counts.Excluded != score.Counts.Raw {
		return DeepSWEPublicAgentScore{}, fmt.Errorf("agent %s attempt accounting is inconsistent", agentID)
	}
	score.LivePooled = DeepSWERate{Numerator: float64(score.Counts.Passed), Denominator: score.Counts.Scored}
	if score.Counts.Scored > 0 {
		rate := float64(score.Counts.Passed) / float64(score.Counts.Scored)
		score.LivePooled.Rate = float64Pointer(rate)
	}
	score.TaskMacro = DeepSWERate{
		Numerator: macroNumerator, Denominator: macroTasks,
	}
	if macroTasks > 0 {
		rate := macroNumerator / float64(macroTasks)
		score.TaskMacro.Rate = float64Pointer(rate)
	}
	if repetitions == 4 {
		score.PassAt4 = &DeepSWEPassAtK{
			K: 4, PassedTasks: passedTasks, TotalTasks: macroTasks, UniverseTasks: len(tasks),
			Method: "any-pass-among-remaining-scored-attempts",
		}
		if macroTasks > 0 {
			rate := float64(passedTasks) / float64(macroTasks)
			score.PassAt4.Rate = float64Pointer(rate)
		}
		score.FourRunStatistics = buildFourRunStatistics(runPassed, runScored, score.LivePooled.Rate)
	}
	score.ExclusionSensitivity = buildDeepSWEExclusionSensitivity(
		score.Counts, len(tasks), repetitions,
		worstCaseMacroNumerator, bestCaseMacroNumerator,
		worstCasePassedTasks, bestCasePassedTasks,
	)
	score.AllExecutedEfficiency = aggregateDeepSWEEfficiency(allAttempts)
	return score, nil
}

func buildDeepSWEExclusionSensitivity(counts DeepSWEAttemptCounts, taskCount, repetitions int, worstMacroNumerator, bestMacroNumerator float64, worstPassedTasks, bestPassedTasks int) DeepSWEExclusionSensitivity {
	result := DeepSWEExclusionSensitivity{
		RawAttempts: counts.Raw, ScoredAttempts: counts.Scored, ExcludedAttempts: counts.Excluded,
		WorstCaseAllExcludedAsFailure: DeepSWEQualityScenario{
			LivePooled: DeepSWERate{Numerator: float64(counts.Passed), Denominator: counts.Raw},
			TaskMacro:  DeepSWERate{Numerator: worstMacroNumerator, Denominator: taskCount},
		},
		BestCaseAllExcludedAsPass: DeepSWEQualityScenario{
			LivePooled: DeepSWERate{Numerator: float64(counts.Passed + counts.Excluded), Denominator: counts.Raw},
			TaskMacro:  DeepSWERate{Numerator: bestMacroNumerator, Denominator: taskCount},
		},
	}
	if counts.Raw > 0 {
		exclusionRate := float64(counts.Excluded) / float64(counts.Raw)
		worstPooledRate := float64(counts.Passed) / float64(counts.Raw)
		bestPooledRate := float64(counts.Passed+counts.Excluded) / float64(counts.Raw)
		result.ExclusionRate = float64Pointer(exclusionRate)
		result.WorstCaseAllExcludedAsFailure.LivePooled.Rate = float64Pointer(worstPooledRate)
		result.BestCaseAllExcludedAsPass.LivePooled.Rate = float64Pointer(bestPooledRate)
	}
	if taskCount > 0 {
		worstMacroRate := worstMacroNumerator / float64(taskCount)
		bestMacroRate := bestMacroNumerator / float64(taskCount)
		result.WorstCaseAllExcludedAsFailure.TaskMacro.Rate = float64Pointer(worstMacroRate)
		result.BestCaseAllExcludedAsPass.TaskMacro.Rate = float64Pointer(bestMacroRate)
	}
	if repetitions == 4 {
		result.WorstCaseAllExcludedAsFailure.PassAt4 = &DeepSWEPassAtK{
			K: 4, PassedTasks: worstPassedTasks, TotalTasks: taskCount, UniverseTasks: taskCount,
			Method: "any-pass-with-infrastructure-exclusions-as-failures",
		}
		result.BestCaseAllExcludedAsPass.PassAt4 = &DeepSWEPassAtK{
			K: 4, PassedTasks: bestPassedTasks, TotalTasks: taskCount, UniverseTasks: taskCount,
			Method: "any-pass-with-infrastructure-exclusions-as-passes",
		}
		if taskCount > 0 {
			worstRate := float64(worstPassedTasks) / float64(taskCount)
			bestRate := float64(bestPassedTasks) / float64(taskCount)
			result.WorstCaseAllExcludedAsFailure.PassAt4.Rate = float64Pointer(worstRate)
			result.BestCaseAllExcludedAsPass.PassAt4.Rate = float64Pointer(bestRate)
		}
	}
	return result
}

func buildFourRunStatistics(passed, scored []int, confidenceCenter *float64) *DeepSWEFourRunStatistics {
	statistics := &DeepSWEFourRunStatistics{Z: normal95Z}
	rates := make([]float64, 0, 4)
	for index := 0; index < 4; index++ {
		sample := DeepSWERunSample{Run: index + 1, Passed: passed[index], Scored: scored[index]}
		if scored[index] > 0 {
			rate := float64(passed[index]) / float64(scored[index])
			sample.Rate = float64Pointer(rate)
			rates = append(rates, rate)
		}
		statistics.Runs = append(statistics.Runs, sample)
	}
	if len(rates) != 4 || confidenceCenter == nil {
		return statistics
	}
	mean := 0.0
	for _, rate := range rates {
		mean += rate
	}
	mean /= 4
	variance := 0.0
	for _, rate := range rates {
		delta := rate - mean
		variance += delta * delta
	}
	variance /= 3
	sd := math.Sqrt(variance)
	halfWidth := normal95Z * sd / math.Sqrt(4)
	lower, upper := *confidenceCenter-halfWidth, *confidenceCenter+halfWidth
	halfWidthPP := halfWidth * 100
	statistics.RunMean = float64Pointer(mean)
	statistics.SampleStandardDeviation = float64Pointer(sd)
	statistics.ConfidenceCenter = float64Pointer(*confidenceCenter)
	statistics.ConfidenceLower = float64Pointer(lower)
	statistics.ConfidenceUpper = float64Pointer(upper)
	statistics.ConfidenceHalfWidth = float64Pointer(halfWidth)
	statistics.HalfWidthPercentagePoints = float64Pointer(halfWidthPP)
	return statistics
}

func aggregateDeepSWEEfficiency(attempts []DeepSWEPublicAttempt) DeepSWEAllExecutedEfficiency {
	efficiencies := make([]DeepSWEAttemptEfficiency, 0, len(attempts))
	for _, attempt := range attempts {
		efficiencies = append(efficiencies, attempt.Efficiency)
	}
	result := DeepSWEAllExecutedEfficiency{Attempts: len(attempts)}
	result.AgentDurationSeconds = aggregateFloat(efficiencies, func(e DeepSWEAttemptEfficiency) *float64 { return e.AgentDurationSeconds })
	result.TrialDurationSeconds = aggregateFloat(efficiencies, func(e DeepSWEAttemptEfficiency) *float64 { return e.TrialDurationSeconds })
	result.CostUSD = aggregateFloat(efficiencies, func(e DeepSWEAttemptEfficiency) *float64 { return e.CostUSD })
	result.AgentSteps = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.AgentSteps })
	result.LLMCallsStarted = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.LLMCallsStarted })
	result.ProviderRequests = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.ProviderRequests })
	result.ProviderRounds = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.ProviderRounds })
	result.ProviderErrors = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.ProviderErrors })
	result.ToolInvocations = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.ToolInvocations })
	result.PhysicalToolOperations = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.PhysicalToolOperations })
	result.InputTokens = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.InputTokens })
	result.CachedInputTokens = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.CachedInputTokens })
	result.OutputTokens = aggregateInt(efficiencies, func(e DeepSWEAttemptEfficiency) *int64 { return e.OutputTokens })
	if result.InputTokens.Sum != nil && result.CachedInputTokens.Sum != nil && *result.InputTokens.Sum > 0 {
		rate := float64(*result.CachedInputTokens.Sum) / float64(*result.InputTokens.Sum)
		result.TokenWeightedCacheRate = float64Pointer(rate)
	}
	return result
}

func aggregateFloat(values []DeepSWEAttemptEfficiency, selectValue func(DeepSWEAttemptEfficiency) *float64) DeepSWEFloatAggregate {
	result := DeepSWEFloatAggregate{Total: len(values)}
	sum := 0.0
	for _, value := range values {
		selected := selectValue(value)
		if selected == nil {
			continue
		}
		result.Observed++
		sum += *selected
	}
	if result.Observed == result.Total && result.Total > 0 {
		mean := sum / float64(result.Total)
		result.Sum, result.Mean = float64Pointer(sum), float64Pointer(mean)
	}
	return result
}

func aggregateInt(values []DeepSWEAttemptEfficiency, selectValue func(DeepSWEAttemptEfficiency) *int64) DeepSWEIntAggregate {
	result := DeepSWEIntAggregate{Total: len(values)}
	var sum int64
	for _, value := range values {
		selected := selectValue(value)
		if selected == nil {
			continue
		}
		result.Observed++
		sum += *selected
	}
	if result.Observed == result.Total && result.Total > 0 {
		mean := float64(sum) / float64(result.Total)
		result.Sum, result.Mean = int64Pointer(sum), float64Pointer(mean)
	}
	return result
}

func scoreDeepSWEPairs(input DeepSWEPublicScoringInput, tasks []string, indexed map[string]DeepSWEPublicAttempt) DeepSWEPublicPairedComparison {
	result := DeepSWEPublicPairedComparison{
		ChallengerAgentID: input.ChallengerAgentID,
		BaselineAgentID:   input.BaselineAgentID,
		PairingMethod:     "preregistered-task-slot-intersection",
	}
	for _, taskID := range tasks {
		for slot := 1; slot <= input.Repetitions; slot++ {
			challenger := indexed[agentTaskSlotKey(input.ChallengerAgentID, taskID, slot)]
			baseline := indexed[agentTaskSlotKey(input.BaselineAgentID, taskID, slot)]
			if challenger.Disposition != DeepSWEAttemptScored || baseline.Disposition != DeepSWEAttemptScored {
				continue
			}
			challengerPassed, baselinePassed := *challenger.Passed, *baseline.Passed
			result.CommonScored++
			if challengerPassed {
				result.ChallengerPassed++
			}
			if baselinePassed {
				result.BaselinePassed++
			}
			switch {
			case challengerPassed && baselinePassed:
				result.BothPass++
			case challengerPassed:
				result.ChallengerOnly++
			case baselinePassed:
				result.BaselineOnly++
			default:
				result.BothFail++
			}
			result.Rows = append(result.Rows, DeepSWEPublicPairedRow{
				TaskID: taskID, Slot: slot, ChallengerPassed: challengerPassed, BaselinePassed: baselinePassed,
			})
		}
	}
	if result.CommonScored > 0 {
		delta := float64(result.ChallengerPassed-result.BaselinePassed) / float64(result.CommonScored)
		result.PassRateDelta = float64Pointer(delta)
	}
	discordant := result.ChallengerOnly + result.BaselineOnly
	result.McNemar = DeepSWEMcNemarInputs{
		BChallengerOnly: result.ChallengerOnly,
		CBaselineOnly:   result.BaselineOnly,
		Discordant:      discordant,
	}
	if result.CommonScored > 0 {
		p := exactMcNemarP(result.ChallengerOnly, result.BaselineOnly)
		result.McNemar.ExactTwoSidedP = float64Pointer(p)
	}
	return result
}

var deepSWEInfrastructureCategories = []DeepSWEFailureCategory{
	DeepSWEFailureProviderInfrastructure,
	DeepSWEFailureVerifierInfrastructure,
	DeepSWEFailureNetworkInfrastructure,
	DeepSWEFailureControllerInfrastructure,
}

func exclusionSummaryFromScore(score DeepSWEPublicAgentScore) DeepSWEAgentExclusionSummary {
	byCategory := make(map[DeepSWEFailureCategory]int, len(deepSWEInfrastructureCategories))
	for _, category := range deepSWEInfrastructureCategories {
		byCategory[category] = score.ExcludedByCategory[category]
	}
	return DeepSWEAgentExclusionSummary{
		AgentID: score.AgentID, Raw: score.Counts.Raw, Scored: score.Counts.Scored,
		Excluded: score.Counts.Excluded, ExclusionRate: score.ExclusionSensitivity.ExclusionRate,
		ExcludedByCategory: byCategory,
	}
}

func scoreDeepSWEExclusionImbalance(input DeepSWEPublicScoringInput, tasks []string, indexed map[string]DeepSWEPublicAttempt) DeepSWEExclusionImbalance {
	result := DeepSWEExclusionImbalance{
		ChallengerAgentID: input.ChallengerAgentID,
		BaselineAgentID:   input.BaselineAgentID,
	}
	challengerByCategory := make(map[DeepSWEFailureCategory]int, len(deepSWEInfrastructureCategories))
	baselineByCategory := make(map[DeepSWEFailureCategory]int, len(deepSWEInfrastructureCategories))
	for _, taskID := range tasks {
		for slot := 1; slot <= input.Repetitions; slot++ {
			challenger := indexed[agentTaskSlotKey(input.ChallengerAgentID, taskID, slot)]
			baseline := indexed[agentTaskSlotKey(input.BaselineAgentID, taskID, slot)]
			challengerExcluded := challenger.Disposition == DeepSWEAttemptExcluded
			baselineExcluded := baseline.Disposition == DeepSWEAttemptExcluded
			result.RawPairs++
			if challengerExcluded {
				result.ChallengerExcluded++
				challengerByCategory[challenger.FailureCategory]++
			}
			if baselineExcluded {
				result.BaselineExcluded++
				baselineByCategory[baseline.FailureCategory]++
			}
			switch {
			case challengerExcluded && baselineExcluded:
				result.BothExcluded++
				result.AnyExcluded++
			case challengerExcluded:
				result.ChallengerOnlyExcluded++
				result.AnyExcluded++
			case baselineExcluded:
				result.BaselineOnlyExcluded++
				result.AnyExcluded++
			default:
				result.CommonScored++
			}
		}
	}
	result.DiscordantExclusionSlots = result.ChallengerOnlyExcluded + result.BaselineOnlyExcluded
	result.ChallengerMinusBaseline = result.ChallengerExcluded - result.BaselineExcluded
	result.AbsoluteCountDifference = absoluteInt(result.ChallengerMinusBaseline)
	if result.RawPairs > 0 {
		challengerRate := float64(result.ChallengerExcluded) / float64(result.RawPairs)
		baselineRate := float64(result.BaselineExcluded) / float64(result.RawPairs)
		delta := challengerRate - baselineRate
		result.ChallengerExclusionRate = float64Pointer(challengerRate)
		result.BaselineExclusionRate = float64Pointer(baselineRate)
		result.ExclusionRateDelta = float64Pointer(delta)
	}
	for _, category := range deepSWEInfrastructureCategories {
		delta := challengerByCategory[category] - baselineByCategory[category]
		result.Categories = append(result.Categories, DeepSWEExclusionCategoryComparison{
			Category: category, ChallengerExcluded: challengerByCategory[category], BaselineExcluded: baselineByCategory[category],
			ChallengerMinusBaseline: delta, AbsoluteCountDifference: absoluteInt(delta),
		})
	}
	return result
}

func absoluteInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func exactMcNemarP(b, c int) float64 {
	n := b + c
	if n == 0 {
		return 1
	}
	k := min(b, c)
	term := math.Pow(0.5, float64(n))
	cumulative := term
	for i := 0; i < k; i++ {
		term *= float64(n-i) / float64(i+1)
		cumulative += term
	}
	return math.Min(1, 2*cumulative)
}

func agentTaskKey(agentID, taskID string) string {
	return agentID + "\x00" + taskID
}

func agentTaskSlotKey(agentID, taskID string, slot int) string {
	return fmt.Sprintf("%s\x00%s\x00%d", agentID, taskID, slot)
}

func float64Pointer(value float64) *float64 { return &value }
func int64Pointer(value int64) *int64       { return &value }
