package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type ExperimentStatus string

const (
	ExperimentPending  ExperimentStatus = "pending"
	ExperimentRunning  ExperimentStatus = "running"
	ExperimentComplete ExperimentStatus = "complete"
	ExperimentInvalid  ExperimentStatus = "invalid"
)

type RunPhase string

const (
	RunPending        RunPhase = "pending"
	RunComplete       RunPhase = "complete"
	RunProtocolFailed RunPhase = "protocol_failed"
)

type OracleRecord struct {
	TaskID       string             `json:"task_id"`
	Validated    bool               `json:"validated"`
	Verification VerificationResult `json:"verification"`
	Failure      string             `json:"failure,omitempty"`
}

type RunRecord struct {
	Entry            PlanEntry                 `json:"entry"`
	Phase            RunPhase                  `json:"phase"`
	Attempts         int                       `json:"attempts"`
	SlotReservedAt   time.Time                 `json:"slot_reserved_at,omitempty"`
	StorageAdmission StorageAdmissionReceipt   `json:"storage_admission"`
	AttemptStartedAt time.Time                 `json:"attempt_started_at,omitempty"`
	Disposition      DeepSWEAttemptDisposition `json:"disposition,omitempty"`
	FailureCategory  DeepSWEFailureCategory    `json:"failure_category,omitempty"`
	ArtifactDir      string                    `json:"artifact_dir,omitempty"`
	Execution        *AgentExecution           `json:"execution,omitempty"`
	Verification     *VerificationResult       `json:"verification,omitempty"`
	Metrics          *UsageMetrics             `json:"metrics,omitempty"`
	Failure          string                    `json:"failure,omitempty"`
}

type ExperimentState struct {
	SchemaVersion  string                  `json:"schema_version"`
	ManifestSHA256 string                  `json:"manifest_sha256"`
	PlanSHA256     string                  `json:"plan_sha256"`
	Status         ExperimentStatus        `json:"status"`
	StartedAt      time.Time               `json:"started_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
	CompletedAt    *time.Time              `json:"completed_at,omitempty"`
	Backend        BackendSnapshot         `json:"backend"`
	Agents         []AgentSnapshot         `json:"agents"`
	Oracle         map[string]OracleRecord `json:"oracle"`
	Runs           map[string]RunRecord    `json:"runs"`
	InvalidReason  string                  `json:"invalid_reason,omitempty"`
}

func NewExperimentState(manifestSHA256 string, plan RunPlan, backend BackendSnapshot, agents []AgentSnapshot, now time.Time) (ExperimentState, error) {
	planSHA, err := HashCanonical(plan)
	if err != nil {
		return ExperimentState{}, err
	}
	runs := make(map[string]RunRecord, len(plan.Entries))
	for _, entry := range plan.Entries {
		key := RunKey(entry)
		runs[key] = RunRecord{Entry: entry, Phase: RunPending}
	}
	return ExperimentState{
		SchemaVersion:  "agentic-bench/state-v2",
		ManifestSHA256: manifestSHA256,
		PlanSHA256:     planSHA,
		Status:         ExperimentPending,
		StartedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
		Backend:        backend,
		Agents:         slices.Clone(agents),
		Oracle:         map[string]OracleRecord{},
		Runs:           runs,
	}, nil
}

func LoadState(path, manifestSHA256, planSHA256 string) (ExperimentState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ExperimentState{}, err
	}
	var state ExperimentState
	if err := json.Unmarshal(raw, &state); err != nil {
		return ExperimentState{}, err
	}
	if state.SchemaVersion != "agentic-bench/state-v2" || state.ManifestSHA256 != manifestSHA256 || state.PlanSHA256 != planSHA256 {
		return ExperimentState{}, errors.New("resume state does not match the immutable manifest and plan")
	}
	return state, nil
}

func RunKey(entry PlanEntry) string {
	return fmt.Sprintf("%03d/%s/%s", entry.Repetition, entry.TaskID, entry.AgentID)
}

func HashCanonical(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

type AgentScore struct {
	AgentID         string       `json:"agent_id"`
	Passed          int          `json:"passed"`
	Total           int          `json:"total"`
	Score           float64      `json:"score"`
	Usage           UsageMetrics `json:"usage"`
	WallTimeSeconds float64      `json:"wall_time_seconds"`
}

type Scorecard struct {
	SchemaVersion string                  `json:"schema_version"`
	Profile       string                  `json:"profile,omitempty"`
	Agents        []AgentScore            `json:"agents,omitempty"`
	DeepSWEPublic *DeepSWEPublicScorecard `json:"deepswe_public,omitempty"`
}

// ScoreExperiment refuses to score any oracle, infrastructure, protocol, or
// verifier gap. This prevents broken infrastructure from becoming a model
// failure and prevents a broken gold solution from legitimizing a task.
func ScoreExperiment(state ExperimentState, plan RunPlan) (Scorecard, error) {
	if state.Status != ExperimentComplete {
		return Scorecard{}, fmt.Errorf("experiment status %s is not scoreable", state.Status)
	}
	for _, entry := range plan.Entries {
		oracle, exists := state.Oracle[entry.TaskID]
		if !exists || !oracle.Validated || !oracle.Verification.ProtocolValid || oracle.Verification.Reward != 1 {
			return Scorecard{}, fmt.Errorf("task %s has no passing oracle validation", entry.TaskID)
		}
		record, exists := state.Runs[RunKey(entry)]
		if !exists || record.Phase != RunComplete || record.Verification == nil || record.Metrics == nil {
			return Scorecard{}, fmt.Errorf("run %s is incomplete", RunKey(entry))
		}
		if !record.Verification.ProtocolValid || (record.Verification.Reward != 0 && record.Verification.Reward != 1) ||
			(record.Verification.RawReward != nil && (*record.Verification.RawReward != 0 && *record.Verification.RawReward != 1)) {
			return Scorecard{}, fmt.Errorf("run %s has an invalid verifier result", RunKey(entry))
		}
	}
	byAgent := map[string][]RunRecord{}
	for _, entry := range plan.Entries {
		record := state.Runs[RunKey(entry)]
		byAgent[entry.AgentID] = append(byAgent[entry.AgentID], record)
	}
	agentIDs := make([]string, 0, len(byAgent))
	for id := range byAgent {
		agentIDs = append(agentIDs, id)
	}
	slices.Sort(agentIDs)
	card := Scorecard{SchemaVersion: "agentic-bench/scorecard-v1"}
	for _, id := range agentIDs {
		records := byAgent[id]
		metrics := make([]UsageMetrics, 0, len(records))
		score := AgentScore{AgentID: id, Total: len(records)}
		for _, record := range records {
			if record.Verification.Reward == 1 {
				score.Passed++
			}
			metrics = append(metrics, *record.Metrics)
			if record.Execution != nil {
				score.WallTimeSeconds += record.Execution.FinishedAt.Sub(record.Execution.StartedAt).Seconds()
			}
		}
		score.Score = float64(score.Passed) / float64(score.Total)
		score.Usage = MergeUsageMetrics(metrics)
		card.Agents = append(card.Agents, score)
	}
	return card, nil
}

func artifactPath(root, relative string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(root, filepath.FromSlash(relative))
	joined, err = filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if joined != root && !strings.HasPrefix(joined, root+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes its root")
	}
	return joined, nil
}
