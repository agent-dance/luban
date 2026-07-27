package pierbackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

type pierTiming struct {
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type pierTrialResult struct {
	TaskName     string    `json:"task_name"`
	TrialName    string    `json:"trial_name"`
	TaskChecksum string    `json:"task_checksum"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	AgentInfo    struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		ModelInfo *struct {
			Name     string  `json:"name"`
			Provider *string `json:"provider"`
		} `json:"model_info"`
	} `json:"agent_info"`
	AgentExecution *pierTiming `json:"agent_execution"`
	Verifier       *pierTiming `json:"verifier"`
	VerifierResult *struct {
		Rewards map[string]float64 `json:"rewards"`
	} `json:"verifier_result"`
	ExceptionInfo *struct {
		ExceptionType string `json:"exception_type"`
	} `json:"exception_info"`
}

func parseTrialResult(path string) (sanitizedTrialResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return sanitizedTrialResult{}, err
	}
	var source pierTrialResult
	if err := json.Unmarshal(raw, &source); err != nil {
		return sanitizedTrialResult{}, fmt.Errorf("decode Pier trial result: %w", err)
	}
	result := sanitizedTrialResult{
		SchemaVersion: "agentic-bench/pier-trial-result-v1", TaskName: source.TaskName,
		TrialName: source.TrialName, TaskChecksum: source.TaskChecksum,
		AgentName: source.AgentInfo.Name, AgentVersion: source.AgentInfo.Version,
		StartedAt: source.StartedAt, FinishedAt: source.FinishedAt,
	}
	if source.AgentInfo.ModelInfo != nil {
		result.Model = source.AgentInfo.ModelInfo.Name
		if source.AgentInfo.ModelInfo.Provider != nil {
			result.Provider = *source.AgentInfo.ModelInfo.Provider
		}
	}
	if source.AgentExecution != nil {
		if source.AgentExecution.StartedAt != nil {
			result.AgentStartedAt = source.AgentExecution.StartedAt.UTC()
		}
		if source.AgentExecution.FinishedAt != nil {
			result.AgentFinishedAt = source.AgentExecution.FinishedAt.UTC()
		}
	}
	if source.Verifier != nil {
		if source.Verifier.StartedAt != nil {
			result.VerifierStarted = source.Verifier.StartedAt.UTC()
		}
		if source.Verifier.FinishedAt != nil {
			result.VerifierEnded = source.Verifier.FinishedAt.UTC()
		}
	}
	if source.VerifierResult != nil {
		result.Rewards = source.VerifierResult.Rewards
	}
	if source.ExceptionInfo != nil {
		result.ExceptionType = source.ExceptionInfo.ExceptionType
	}
	if result.TaskName == "" || result.TrialName == "" || result.StartedAt.IsZero() || result.FinishedAt.IsZero() {
		return sanitizedTrialResult{}, errors.New("Pier trial result lacks stable identity or timing")
	}
	return result, nil
}

func findSingleTrial(jobDir string) (string, error) {
	entries, err := os.ReadDir(jobDir)
	if err != nil {
		return "", err
	}
	var trials []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(jobDir, entry.Name())
		if info, err := os.Stat(filepath.Join(candidate, "result.json")); err == nil && info.Mode().IsRegular() {
			trials = append(trials, candidate)
		}
	}
	if len(trials) != 1 {
		return "", fmt.Errorf("Pier job has %d completed trials, expected one", len(trials))
	}
	return trials[0], nil
}

func exportVerification(trialDir, destination string, parsed sanitizedTrialResult, redact func([]byte) []byte) (harness.VerificationResult, error) {
	if len(parsed.Rewards) == 0 {
		return harness.VerificationResult{}, fmt.Errorf("Pier verifier produced no rewards (exception=%s)", parsed.ExceptionType)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return harness.VerificationResult{}, err
	}
	verifierSource := filepath.Join(trialDir, "verifier")
	paths, err := copyAuditFiles(verifierSource, destination, redact)
	if err != nil {
		return harness.VerificationResult{}, err
	}
	resultPath := filepath.Join(destination, "pier-trial-result.json")
	if err := harness.WriteJSONAtomic(resultPath, parsed, 0o644); err != nil {
		return harness.VerificationResult{}, err
	}
	paths = append(paths, resultPath)
	reward, ok := parsed.Rewards["reward"]
	if !ok && len(parsed.Rewards) == 1 {
		for _, value := range parsed.Rewards {
			reward = value
		}
	}
	return harness.VerificationResult{ProtocolValid: true, Reward: reward, Scores: parsed.Rewards, ArtifactPaths: paths}, nil
}

func copyAuditFiles(source, destination string, redact func([]byte) []byte) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Pier audit output contains unsupported file type: %s", relative)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if redact != nil {
			raw = redact(raw)
		}
		if err := harness.WriteBytesAtomic(target, raw, info.Mode().Perm()); err != nil {
			return err
		}
		paths = append(paths, target)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	return paths, nil
}

func readSanitizedTrial(path string) (sanitizedTrialResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return sanitizedTrialResult{}, err
	}
	var result sanitizedTrialResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return sanitizedTrialResult{}, err
	}
	if result.SchemaVersion != "agentic-bench/pier-trial-result-v1" {
		return sanitizedTrialResult{}, errors.New("unsupported sanitized Pier result")
	}
	return result, nil
}

func (backend *Backend) RecoverAgent(_ context.Context, invocation harness.AgentInvocation) (harness.AgentExecution, error) {
	sealedPath := filepath.Join(invocation.ArtifactDir, "sealed-attempt.json")
	raw, err := os.ReadFile(sealedPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return harness.AgentExecution{}, fmt.Errorf("sealed raw attempt is unavailable: %w", err)
		}
		return recoverInterruptedAttempt(invocation)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var sealed sealedAgentAttempt
	if err := decoder.Decode(&sealed); err != nil {
		return harness.AgentExecution{}, fmt.Errorf("decode sealed raw attempt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return harness.AgentExecution{}, errors.New("sealed raw attempt contains trailing JSON")
		}
		return harness.AgentExecution{}, fmt.Errorf("decode sealed raw attempt trailer: %w", err)
	}
	if sealed.SchemaVersion != "agentic-bench/sealed-attempt-v1" || sealed.PairID != invocation.PlanEntry.PairID || sealed.TaskID != invocation.Task.ID || sealed.AgentID != invocation.Agent.ID || sealed.Repetition != invocation.PlanEntry.Repetition {
		return harness.AgentExecution{}, errors.New("sealed raw attempt belongs to a different plan slot")
	}
	if sealed.FailureCategory == harness.DeepSWEFailureNone {
		if sealed.Failure != "" {
			return harness.AgentExecution{}, errors.New("successful sealed attempt contains a failure")
		}
		return sealed.Execution, nil
	}
	if sealed.Failure == "" {
		return harness.AgentExecution{}, errors.New("excluded sealed attempt lacks a failure receipt")
	}
	return sealed.Execution, harness.AttemptInfrastructureError{Category: sealed.FailureCategory, Err: errors.New(sealed.Failure)}
}

func recoverInterruptedAttempt(invocation harness.AgentInvocation) (harness.AgentExecution, error) {
	rawEvidence := filepath.Join(invocation.ArtifactDir, "metrics", "provider-http.jsonl")
	journalPath := evidenceproxy.AttemptJournalPath(rawEvidence)
	journal, journalErr := harness.ReadJSONLines[evidenceproxy.AttemptStartJournalEntry](journalPath)
	if journalErr != nil && !errors.Is(journalErr, os.ErrNotExist) {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: fmt.Errorf("read provider attempt WAL: %w", journalErr)}
	}
	if len(journal) == 0 {
		if info, statErr := os.Stat(rawEvidence); statErr == nil && info.Size() > 0 {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("provider evidence exists without a durable provider-attempt WAL entry")}
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return harness.AgentExecution{}, statErr
		}
		return harness.AgentExecution{}, harness.SafeRestartAttemptError{Err: errors.New("no provider attempt crossed the durable WAL boundary")}
	}

	lifecyclePath := filepath.Join(invocation.ArtifactDir, "attempt-lifecycle.json")
	lifecycleRaw, err := os.ReadFile(lifecyclePath)
	if err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: fmt.Errorf("durable provider attempt lacks controller lifecycle: %w", err)}
	}
	decoder := json.NewDecoder(strings.NewReader(string(lifecycleRaw)))
	decoder.DisallowUnknownFields()
	var lifecycle harness.AttemptLifecycle
	if err := decoder.Decode(&lifecycle); err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: fmt.Errorf("decode controller lifecycle: %w", err)}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("controller lifecycle contains trailing JSON")}
	}
	if lifecycle.SchemaVersion != "agentic-bench/attempt-lifecycle-v1" || lifecycle.RunIdentity == "" || lifecycle.ControllerStartedAt.IsZero() || !lifecycle.ControllerFinishedAt.IsZero() || lifecycle.ProviderAttemptState != "no_provider_attempt" || lifecycle.ProviderAttemptCount != 0 || lifecycle.Recovered {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("controller lifecycle is inconsistent with an interrupted attempt")}
	}
	rounds := make(map[int]struct{}, len(journal))
	for _, entry := range journal {
		if entry.SchemaVersion != "agentic-bench/provider-attempt-start-v1" || entry.RunIdentity != lifecycle.RunIdentity || entry.Round < 0 || entry.StartedAt.IsZero() {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("provider attempt WAL identity is invalid")}
		}
		if _, duplicate := rounds[entry.Round]; duplicate {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("provider attempt WAL contains a duplicate round")}
		}
		rounds[entry.Round] = struct{}{}
		state, inspectErr := evidenceproxy.InspectAttemptRecoveryState(rawEvidence, lifecycle.RunIdentity, entry.Round)
		if inspectErr != nil || state == evidenceproxy.AttemptRecoveryZeroEvidence {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("provider attempt WAL cannot be reconciled with recovery state")}
		}
	}
	if _, statErr := os.Stat(evidenceproxy.EvidenceSealPath(rawEvidence)); statErr == nil {
		if _, sealErr := evidenceproxy.ValidateEvidenceSeal(rawEvidence, lifecycle.RunIdentity); sealErr != nil {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: fmt.Errorf("interrupted attempt has an invalid provider evidence seal: %w", sealErr)}
		}
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("controller terminated after provider evidence was sealed; start a new formal experiment instead of discarding auditable spend")}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return harness.AgentExecution{}, statErr
	}
	lifecycle.ProviderAttemptState = "provider_attempt_started_unsealed"
	lifecycle.ProviderAttemptCount = uint64(len(journal))
	lifecycle.Recovered = true
	execution := harness.AgentExecution{Lifecycle: lifecycle}
	const failure = "controller terminated after a durable provider attempt start"
	if err := sealAgentAttempt(invocation, execution, harness.DeepSWEFailureControllerInfrastructure, failure); err != nil {
		return harness.AgentExecution{}, err
	}
	return execution, harness.AttemptInfrastructureError{Category: harness.DeepSWEFailureControllerInfrastructure, Err: errors.New(failure)}
}
