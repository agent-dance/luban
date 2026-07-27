package harness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
)

// ScoreExperimentForManifest is the only formal scoring entry point. The
// legacy generic scorer remains available for non-formal diagnostics, but a
// manifest-pinned run must dispatch through its fail-closed public profile.
func ScoreExperimentForManifest(loaded LoadedManifest, state ExperimentState, plan RunPlan) (Scorecard, error) {
	if err := validateLoadedManifestIdentity(loaded); err != nil {
		return Scorecard{}, err
	}
	manifest := loaded.Manifest
	if manifest.Scoring.Profile != ScoringProfileDeepSWEV11PublicCI {
		return Scorecard{}, fmt.Errorf("unsupported formal scoring profile %q", manifest.Scoring.Profile)
	}
	input, err := DeepSWEPublicInputFromState(loaded, state, plan)
	if err != nil {
		return Scorecard{}, err
	}
	public, err := ScoreDeepSWEPublicCI(input)
	if err != nil {
		return Scorecard{}, err
	}
	return Scorecard{
		SchemaVersion: "agentic-bench/scorecard-v2",
		Profile:       manifest.Scoring.Profile,
		DeepSWEPublic: &public,
	}, nil
}

// DeepSWEPublicOutcome is the normalized scoring outcome shared by the formal
// public scorer and report projections. Passed is nil only for a typed
// infrastructure exclusion.
type DeepSWEPublicOutcome struct {
	Disposition     DeepSWEAttemptDisposition `json:"disposition"`
	Passed          *bool                     `json:"passed"`
	FailureCategory DeepSWEFailureCategory    `json:"failure_category"`
}

// NormalizeDeepSWEPublicOutcome is the single authority for converting a raw
// state-v2 record into public quality semantics. In particular, timeout and
// context exhaustion override a raw verifier pass and remain scored failures.
func NormalizeDeepSWEPublicOutcome(record RunRecord) (DeepSWEPublicOutcome, error) {
	outcome := DeepSWEPublicOutcome{Disposition: record.Disposition, FailureCategory: record.FailureCategory}
	switch record.Disposition {
	case DeepSWEAttemptScored:
		if record.Execution == nil || record.Verification == nil {
			return DeepSWEPublicOutcome{}, errors.New("scored attempt lacks execution or verifier evidence")
		}
		verification := record.Verification
		if !verification.ProtocolValid || math.IsNaN(verification.Reward) || math.IsInf(verification.Reward, 0) || (verification.Reward != 0 && verification.Reward != 1) {
			return DeepSWEPublicOutcome{}, errors.New("scored attempt has an invalid binary effective verifier result")
		}
		if verification.RawReward != nil && (math.IsNaN(*verification.RawReward) || math.IsInf(*verification.RawReward, 0) || (*verification.RawReward != 0 && *verification.RawReward != 1)) {
			return DeepSWEPublicOutcome{}, errors.New("scored attempt has an invalid binary raw verifier result")
		}
		passed := verification.Reward == 1
		expectedCategory := DeepSWEFailureNone
		switch record.Execution.ExitClass {
		case "timeout":
			if verification.Reward != 0 || verification.RawReward == nil {
				return DeepSWEPublicOutcome{}, errors.New("timeout attempt did not preserve raw and effective verifier rewards")
			}
			passed = false
			expectedCategory = DeepSWEFailureAgentTimeout
		case "context_failure":
			if verification.Reward != 0 || verification.RawReward == nil {
				return DeepSWEPublicOutcome{}, errors.New("context-failure attempt did not preserve raw and effective verifier rewards")
			}
			passed = false
			expectedCategory = DeepSWEFailureContext
		default:
			if verification.RawReward != nil {
				return DeepSWEPublicOutcome{}, errors.New("non-terminal attempt has an unexplained raw verifier override")
			}
		}
		if record.FailureCategory != expectedCategory {
			return DeepSWEPublicOutcome{}, fmt.Errorf("failure category %s disagrees with execution class %s", record.FailureCategory, record.Execution.ExitClass)
		}
		outcome.Passed = &passed
	case DeepSWEAttemptExcluded:
		if record.Verification != nil {
			return DeepSWEPublicOutcome{}, errors.New("excluded attempt unexpectedly has a scoreable verifier result")
		}
		if !validInfrastructureCategory(record.FailureCategory) {
			return DeepSWEPublicOutcome{}, errors.New("excluded attempt lacks a typed infrastructure category")
		}
	default:
		return DeepSWEPublicOutcome{}, errors.New("attempt has no valid scoring disposition")
	}
	return outcome, nil
}

// DeepSWEPublicInputFromState adapts the complete typed raw-attempt ledger. It
// never infers infrastructure exclusions from free-form Failure text and uses
// Pier trial start, rather than agent-process start, for public run ordering.
func DeepSWEPublicInputFromState(loaded LoadedManifest, state ExperimentState, plan RunPlan) (DeepSWEPublicScoringInput, error) {
	if err := validateLoadedManifestIdentity(loaded); err != nil {
		return DeepSWEPublicScoringInput{}, err
	}
	manifest := loaded.Manifest
	if err := ValidateManifest(manifest); err != nil {
		return DeepSWEPublicScoringInput{}, err
	}
	if state.Status != ExperimentComplete {
		return DeepSWEPublicScoringInput{}, fmt.Errorf("experiment status %s is not scoreable", state.Status)
	}
	if plan.SchemaVersion != "agentic-bench/plan-v1" || plan.ManifestSHA256 != loaded.SHA256 || state.ManifestSHA256 != loaded.SHA256 {
		return DeepSWEPublicScoringInput{}, errors.New("state and plan are not bound to the same immutable manifest")
	}
	planSHA256, err := HashCanonical(plan)
	if err != nil {
		return DeepSWEPublicScoringInput{}, err
	}
	if state.SchemaVersion != "agentic-bench/state-v2" || state.PlanSHA256 != planSHA256 {
		return DeepSWEPublicScoringInput{}, errors.New("state is not bound to the exact deterministic run plan")
	}
	// Formal scoring consumes the complete immutable raw-attempt ledger. An
	// unplanned key can only be a replacement/extra attempt or ledger
	// corruption; silently ignoring it would let callers choose which result to
	// present to the scorer.
	if len(state.Runs) != len(plan.Entries) {
		return DeepSWEPublicScoringInput{}, fmt.Errorf("raw-attempt ledger contains %d slots, want exactly the %d preregistered plan slots", len(state.Runs), len(plan.Entries))
	}
	input := DeepSWEPublicScoringInput{
		Profile:           manifest.Scoring.Profile,
		Repetitions:       manifest.Scheduling.Repetitions,
		BaselineAgentID:   manifest.Scoring.BaselineAgentID,
		ChallengerAgentID: manifest.Scoring.ChallengerAgentID,
	}
	for _, agent := range manifest.Agents {
		input.AgentIDs = append(input.AgentIDs, agent.ID)
	}
	taskSet := map[string]struct{}{}
	for _, entry := range plan.Entries {
		taskSet[entry.TaskID] = struct{}{}
	}
	for taskID := range taskSet {
		input.TaskIDs = append(input.TaskIDs, taskID)
	}
	slices.Sort(input.TaskIDs)
	if err := validateDeepSWEScoringTaskUniverse(manifest.Selection, input.TaskIDs); err != nil {
		return DeepSWEPublicScoringInput{}, err
	}

	for _, taskID := range input.TaskIDs {
		oracle, exists := state.Oracle[taskID]
		if !exists || !oracle.Validated || !oracle.Verification.ProtocolValid || oracle.Verification.Reward != 1 {
			return DeepSWEPublicScoringInput{}, fmt.Errorf("task %s has no passing oracle validation", taskID)
		}
	}
	for _, entry := range plan.Entries {
		record, exists := state.Runs[RunKey(entry)]
		if !exists {
			return DeepSWEPublicScoringInput{}, fmt.Errorf("raw attempt %s is absent", RunKey(entry))
		}
		if record.Entry != entry {
			return DeepSWEPublicScoringInput{}, fmt.Errorf("raw attempt %s is bound to a different plan entry", RunKey(entry))
		}
		if record.Attempts != 1 || record.Phase != RunComplete {
			return DeepSWEPublicScoringInput{}, fmt.Errorf("raw attempt %s has phase %s; a typed infrastructure disposition is required and free-form failures are never inferred", RunKey(entry), record.Phase)
		}
		if record.Execution == nil {
			return DeepSWEPublicScoringInput{}, fmt.Errorf("raw attempt %s lacks sealed trial timing", RunKey(entry))
		}
		execution := record.Execution
		controllerRecovery := record.Disposition == DeepSWEAttemptExcluded && record.FailureCategory == DeepSWEFailureControllerInfrastructure
		attemptStartedAt := execution.TrialStartedAt
		if controllerRecovery {
			if err := validateRecoveredControllerAttempt(*execution); err != nil || !record.AttemptStartedAt.Equal(execution.Lifecycle.ControllerStartedAt) {
				return DeepSWEPublicScoringInput{}, fmt.Errorf("raw attempt %s is not bound to its controller recovery lifecycle", RunKey(entry))
			}
			attemptStartedAt = execution.Lifecycle.ControllerStartedAt
		} else if execution.TrialStartedAt.IsZero() || execution.TrialFinishedAt.Before(execution.TrialStartedAt) || !record.AttemptStartedAt.Equal(execution.TrialStartedAt) {
			return DeepSWEPublicScoringInput{}, fmt.Errorf("raw attempt %s is not bound to its Pier trial timing", RunKey(entry))
		}
		if execution.StartedAt.IsZero() != execution.FinishedAt.IsZero() || (!execution.StartedAt.IsZero() && execution.FinishedAt.Before(execution.StartedAt)) {
			return DeepSWEPublicScoringInput{}, fmt.Errorf("raw attempt %s has invalid agent timing", RunKey(entry))
		}
		outcome, err := NormalizeDeepSWEPublicOutcome(record)
		if err != nil {
			return DeepSWEPublicScoringInput{}, fmt.Errorf("raw attempt %s: %w", RunKey(entry), err)
		}
		attempt := DeepSWEPublicAttempt{
			AttemptID: RunKey(entry), AgentID: entry.AgentID, TaskID: entry.TaskID, Slot: entry.Repetition + 1,
			StartedAt: attemptStartedAt, Disposition: outcome.Disposition, Passed: outcome.Passed, FailureCategory: outcome.FailureCategory,
		}
		switch record.Disposition {
		case DeepSWEAttemptScored:
			if record.Metrics == nil || execution.StartedAt.IsZero() {
				return DeepSWEPublicScoringInput{}, fmt.Errorf("scored raw attempt %s lacks execution, verifier, or usage evidence", RunKey(entry))
			}
		case DeepSWEAttemptExcluded:
		}
		if err := validateDeepSWEAttemptClassification(attempt); err != nil {
			return DeepSWEPublicScoringInput{}, fmt.Errorf("raw attempt %s: %w", RunKey(entry), err)
		}
		attempt.Efficiency = deepSWEAttemptEfficiencyFromRecord(record)
		input.Attempts = append(input.Attempts, attempt)
	}
	if len(input.TaskIDs) == 0 {
		return DeepSWEPublicScoringInput{}, errors.New("plan contains no task")
	}
	return input, nil
}

func deepSWEAttemptEfficiencyFromRecord(record RunRecord) DeepSWEAttemptEfficiency {
	result := DeepSWEAttemptEfficiency{}
	if record.Execution != nil {
		execution := record.Execution
		if !execution.StartedAt.IsZero() && !execution.FinishedAt.IsZero() {
			duration := execution.FinishedAt.Sub(execution.StartedAt).Seconds()
			result.AgentDurationSeconds = &duration
		}
		if !execution.TrialStartedAt.IsZero() && !execution.TrialFinishedAt.IsZero() {
			duration := execution.TrialFinishedAt.Sub(execution.TrialStartedAt).Seconds()
			result.TrialDurationSeconds = &duration
		}
		// A sealed no-provider lifecycle proves that zero model generations
		// started. An interrupted/unsealed provider attempt does not prove a
		// count and therefore remains unknown when its metrics are absent.
		if execution.Lifecycle.ProviderAttemptState == "no_provider_attempt" && execution.Lifecycle.ProviderAttemptCount == 0 {
			result.LLMCallsStarted = int64Pointer(0)
		}
	}
	if record.Metrics == nil {
		return result
	}
	metrics := record.Metrics
	result.CostUSD = metrics.CatalogCost
	result.LLMCallsStarted = intPointerAsInt64(metrics.LLMCallsStarted)
	result.ProviderRequests = intPointerAsInt64(metrics.ProviderRequests)
	result.ProviderRounds = intPointerAsInt64(metrics.ProviderRounds)
	result.ProviderErrors = intPointerAsInt64(metrics.ProviderErrors)
	result.ToolInvocations = intPointerAsInt64(metrics.ToolInvocations)
	if metrics.PhysicalToolObservations == metrics.ToolBearingRounds {
		result.PhysicalToolOperations = intPointerAsInt64(metrics.PhysicalToolOperations)
	}
	if metrics.UsageReceiptObservations == metrics.UsageReceiptTotal {
		result.InputTokens = &metrics.InputTokens
		result.CachedInputTokens = &metrics.CachedInputTokens
		result.OutputTokens = &metrics.OutputTokens
	}
	return result
}

func validateLoadedManifestIdentity(loaded LoadedManifest) error {
	if len(loaded.Raw) == 0 || !hex64Pattern.MatchString(loaded.SHA256) {
		return errors.New("formal scoring requires the exact loaded manifest bytes and SHA-256")
	}
	digest := sha256.Sum256(loaded.Raw)
	if hex.EncodeToString(digest[:]) != loaded.SHA256 {
		return errors.New("loaded manifest bytes differ from their declared SHA-256")
	}
	decoder := json.NewDecoder(bytes.NewReader(loaded.Raw))
	decoder.DisallowUnknownFields()
	var decoded Manifest
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decode loaded scoring manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("loaded scoring manifest has a trailing JSON value")
	}
	if !reflect.DeepEqual(decoded, loaded.Manifest) {
		return errors.New("loaded manifest struct differs from its immutable bytes")
	}
	return ValidateManifest(decoded)
}

func validateDeepSWEScoringTaskUniverse(selection SelectionSpec, taskIDs []string) error {
	switch selection.Mode {
	case "full":
		if len(taskIDs) != selection.ExpectedTaskCount {
			return fmt.Errorf("full public score contains %d tasks, want the frozen universe of %d", len(taskIDs), selection.ExpectedTaskCount)
		}
	case "tasks":
		expected := slices.Clone(selection.TaskIDs)
		slices.Sort(expected)
		if !slices.Equal(taskIDs, expected) {
			return errors.New("public score task universe differs from the explicit selection")
		}
	case "sample":
		if len(taskIDs) != selection.SampleCount {
			return fmt.Errorf("sample public score contains %d tasks, want %d", len(taskIDs), selection.SampleCount)
		}
	default:
		return fmt.Errorf("unsupported task selection mode %q", selection.Mode)
	}
	return nil
}

func intPointerAsInt64(value int) *int64 {
	converted := int64(value)
	return &converted
}
