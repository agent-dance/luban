package tools

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/types"
)

// RuntimeHookExecutionReceipt is the durable, identity-bearing evidence for a
// notification hook that actually ran. It remains available after the parent
// query event stream closes and can be consumed by completion/follow-up paths.
type RuntimeHookExecutionReceipt struct {
	HookType    hooks.HookType   `json:"hook_type"`
	ExecutionID string           `json:"execution_id"`
	ConfigID    string           `json:"config_id"`
	ConfigIndex int              `json:"config_index"`
	Hook        hooks.Hook       `json:"hook"`
	Input       hooks.HookInput  `json:"input"`
	Output      hooks.HookOutput `json:"output"`
	RecordedAt  time.Time        `json:"recorded_at"`
}

// RuntimeNotification is the durable delivery envelope shared by background
// task notifications and future mailbox/task-update consumers.
type RuntimeNotification struct {
	ID                  string                        `json:"id"`
	Kind                string                        `json:"kind"`
	TaskID              string                        `json:"task_id"`
	RunID               string                        `json:"run_id,omitempty"`
	Attempt             int                           `json:"attempt,omitempty"`
	SessionID           string                        `json:"session_id,omitempty"`
	ProjectRoot         string                        `json:"project_root,omitempty"`
	SessionProjectDir   string                        `json:"session_project_dir,omitempty"`
	Title               string                        `json:"title"`
	Message             string                        `json:"message"`
	Status              string                        `json:"status,omitempty"`
	ExitCode            *int                          `json:"exit_code,omitempty"`
	TranscriptPath      string                        `json:"transcriptPath,omitempty"`
	DurationMs          *int64                        `json:"durationMs,omitempty"`
	TotalTokens         *int                          `json:"totalTokens,omitempty"`
	Provider            string                        `json:"provider,omitempty"`
	Model               string                        `json:"model,omitempty"`
	Usage               *types.Usage                  `json:"usage,omitempty"`
	CreatedAt           time.Time                     `json:"created_at"`
	Attempts            int                           `json:"attempts,omitempty"`
	LastError           string                        `json:"last_error,omitempty"`
	SinkRequired        bool                          `json:"sink_required,omitempty"`
	ObserverRequired    bool                          `json:"observer_required,omitempty"`
	FollowUpRequired    bool                          `json:"follow_up_required,omitempty"`
	SinkDeliveredAt     *time.Time                    `json:"sink_delivered_at,omitempty"`
	ObserverDeliveredAt *time.Time                    `json:"observer_delivered_at,omitempty"`
	FollowUpDeliveredAt *time.Time                    `json:"follow_up_delivered_at,omitempty"`
	DeliveredAt         *time.Time                    `json:"delivered_at,omitempty"`
	HookExecutions      []RuntimeHookExecutionReceipt `json:"hook_executions,omitempty"`
}

// AgentRunOutcome is the deterministic terminal classification for one agent
// attempt. Status remains the legacy task-state projection used by existing
// task APIs; Outcome carries the more precise fact needed by presentation and
// recovery code.
type AgentRunOutcome string

const (
	AgentRunOutcomeRunning     AgentRunOutcome = "running"
	AgentRunOutcomeSucceeded   AgentRunOutcome = "succeeded"
	AgentRunOutcomePartial     AgentRunOutcome = "partial"
	AgentRunOutcomeFailed      AgentRunOutcome = "failed"
	AgentRunOutcomeCancelled   AgentRunOutcome = "cancelled"
	AgentRunOutcomeTimedOut    AgentRunOutcome = "timed_out"
	AgentRunOutcomeInterrupted AgentRunOutcome = "interrupted"
)

// RuntimeTaskRunRecord preserves one execution attempt of a retained agent.
// A retained task ID identifies the logical agent; RunID identifies one
// immutable attempt so a resumed agent cannot overwrite prior evidence.
type RuntimeTaskRunRecord struct {
	RunID            string              `json:"run_id"`
	Attempt          int                 `json:"attempt"`
	BatchID          string              `json:"batch_id,omitempty"`
	ParentRunID      string              `json:"parent_run_id,omitempty"`
	AgentPath        string              `json:"agent_path,omitempty"`
	Status           string              `json:"status"`
	Prompt           string              `json:"prompt,omitempty"`
	StartedAt        time.Time           `json:"started_at"`
	FinishedAt       *time.Time          `json:"finished_at,omitempty"`
	UpdatedAt        time.Time           `json:"updated_at"`
	TranscriptPath   string              `json:"transcript_path,omitempty"`
	DurationMs       *int64              `json:"duration_ms,omitempty"`
	TotalTokens      *int                `json:"total_tokens,omitempty"`
	Usage            *types.Usage        `json:"usage,omitempty"`
	Error            string              `json:"error,omitempty"`
	Result           string              `json:"result,omitempty"`
	Outcome          AgentRunOutcome     `json:"outcome,omitempty"`
	TerminalReason   string              `json:"terminal_reason,omitempty"`
	ToolUseCount     int                 `json:"tool_use_count,omitempty"`
	LatestToolUse    string              `json:"latest_tool_use,omitempty"`
	ArtifactRefs     []string            `json:"artifact_refs,omitempty"`
	VerificationRefs []string            `json:"verification_refs,omitempty"`
	LatestProgress   *AgentProgressEvent `json:"latest_progress,omitempty"`
}

type RuntimeTaskRecord struct {
	ID                     string                 `json:"id"`
	Type                   string                 `json:"type"`
	Status                 string                 `json:"status"`
	Description            string                 `json:"description,omitempty"`
	Command                string                 `json:"command,omitempty"`
	Prompt                 string                 `json:"prompt,omitempty"`
	OutputPath             string                 `json:"output_path,omitempty"`
	ExitCode               *int                   `json:"exit_code,omitempty"`
	Error                  string                 `json:"error,omitempty"`
	Result                 string                 `json:"result,omitempty"`
	StartedAt              time.Time              `json:"started_at"`
	FinishedAt             *time.Time             `json:"finished_at,omitempty"`
	UpdatedAt              time.Time              `json:"updated_at"`
	Notified               bool                   `json:"notified,omitempty"`
	OutputOffset           int64                  `json:"output_offset,omitempty"`
	AgentAlias             string                 `json:"agent_alias,omitempty"`
	AgentInput             *AgentInput            `json:"agent_input,omitempty"`
	Detached               bool                   `json:"detached,omitempty"`
	AgentMetadata          *agentSessionMetadata  `json:"agent_metadata,omitempty"`
	AgentMessages          []types.Message        `json:"agent_messages,omitempty"`
	OwnerSessionID         string                 `json:"owner_session_id,omitempty"`
	OwnerSessionProjectDir string                 `json:"owner_session_project_dir,omitempty"`
	OwnerProjectRoot       string                 `json:"owner_project_root,omitempty"`
	OwnerAgentID           string                 `json:"owner_agent_id,omitempty"`
	OwnerPID               int                    `json:"owner_pid,omitempty"`
	CurrentRunID           string                 `json:"current_run_id,omitempty"`
	Attempt                int                    `json:"attempt,omitempty"`
	BatchID                string                 `json:"batch_id,omitempty"`
	ParentRunID            string                 `json:"parent_run_id,omitempty"`
	AgentPath              string                 `json:"agent_path,omitempty"`
	QueuedPrompts          int                    `json:"queued_prompts,omitempty"`
	QueueReason            string                 `json:"queue_reason,omitempty"`
	Runs                   []RuntimeTaskRunRecord `json:"runs,omitempty"`
	LatestProgress         *AgentProgressEvent    `json:"latest_progress,omitempty"`
	TranscriptPath         string                 `json:"transcript_path,omitempty"`
	DurationMs             *int64                 `json:"duration_ms,omitempty"`
	TotalTokens            *int                   `json:"total_tokens,omitempty"`
	Usage                  *types.Usage           `json:"usage,omitempty"`
	Outcome                AgentRunOutcome        `json:"outcome,omitempty"`
	TerminalReason         string                 `json:"terminal_reason,omitempty"`
	TimeoutNanos           int64                  `json:"timeout_nanos,omitempty"`
	ArtifactRefs           []string               `json:"artifact_refs,omitempty"`
	VerificationRefs       []string               `json:"verification_refs,omitempty"`
	Notification           *RuntimeNotification   `json:"notification,omitempty"`
	Notifications          []RuntimeNotification  `json:"notifications,omitempty"`
}

type RuntimeTaskStore struct {
	baseDir string
}

const maxRuntimeStorageIDBytes = 200

func NewRuntimeTaskStore(projectRoot string) *RuntimeTaskStore {
	root := filepath.Clean(strings.TrimSpace(projectRoot))
	if root == "" {
		root = "."
	}
	store := &RuntimeTaskStore{baseDir: filepath.Join(root, ".claude", "runtime-tasks")}
	// The constructor remains error-free for compatibility. Every operation
	// repeats this check and fails closed before touching a managed path.
	_ = ensurePrivateRuntimeDirectory(store.baseDir)
	return store
}

func (s *RuntimeTaskStore) path(id string) string {
	return filepath.Join(s.baseDir, sanitizeTaskPathComponent(id)+".json")
}

func (s *RuntimeTaskStore) lockPath(id string) string {
	return s.path(id) + ".lock"
}

func (s *RuntimeTaskStore) outputPath(id string) string {
	return filepath.Join(filepath.Dir(s.baseDir), "task-output", runtimeOutputPathComponent(id)+".output")
}

func runtimeOutputPathComponent(id string) string {
	component := sanitizeTaskPathComponent(id)
	if component == id {
		return component
	}
	digest := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%s-%x", component, digest[:6])
}

func (s *RuntimeTaskStore) normalizeManagedPaths(record *RuntimeTaskRecord) {
	if s == nil || record == nil {
		return
	}
	if record.Type == backgroundTaskTypeLocalAgent || record.Type == backgroundTaskTypeLocalBash {
		record.OutputPath = s.outputPath(record.ID)
	}
}

func (s *RuntimeTaskStore) Exists(id string) bool {
	if s == nil || validateRuntimeStorageID(id) != nil || ensurePrivateRuntimeDirectory(s.baseDir) != nil {
		return false
	}
	_, err := tightenPrivateRuntimeRegularFile(s.path(id), false)
	return err == nil
}

func (s *RuntimeTaskStore) Save(record RuntimeTaskRecord) error {
	if s == nil {
		return fs.ErrInvalid
	}
	if err := validateRuntimeStorageID(record.ID); err != nil {
		return err
	}
	if err := ensurePrivateRuntimeDirectory(s.baseDir); err != nil {
		return err
	}
	if err := preparePrivateRuntimeLock(s.lockPath(record.ID)); err != nil {
		return err
	}
	s.normalizeManagedPaths(&record)
	record.UpdatedAt = time.Now().UTC()
	return withRuntimeFileLock(s.lockPath(record.ID), func() error {
		path := s.path(record.ID)
		if current, err := readPrivateRuntimeRegularFile(path); err == nil {
			decoded, decodeErr := decodeRuntimeTaskRecord(current)
			if decodeErr == nil && decoded.ID != "" && decoded.ID != record.ID {
				return fs.ErrInvalid
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		data, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		return atomicWritePrivateRuntimeFile(path, data)
	})
}

func (s *RuntimeTaskStore) Get(id string) (RuntimeTaskRecord, bool) {
	if s == nil || validateRuntimeStorageID(id) != nil || ensurePrivateRuntimeDirectory(s.baseDir) != nil ||
		preparePrivateRuntimeLock(s.lockPath(id)) != nil {
		return RuntimeTaskRecord{}, false
	}
	value, err := withRuntimeFileLockResult(s.lockPath(id), func() (any, error) {
		data, err := readPrivateRuntimeRegularFile(s.path(id))
		if err != nil {
			return nil, err
		}
		record, err := decodeRuntimeTaskRecord(data)
		if err != nil {
			return nil, err
		}
		if record.ID != id || validateRuntimeStorageID(record.ID) != nil {
			return nil, fs.ErrInvalid
		}
		s.normalizeManagedPaths(&record)
		return record, nil
	})
	if err != nil {
		return RuntimeTaskRecord{}, false
	}
	record, ok := value.(RuntimeTaskRecord)
	if !ok {
		return RuntimeTaskRecord{}, false
	}
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		return RuntimeTaskRecord{}, false
	}
	return record, true
}

func (s *RuntimeTaskStore) List() []RuntimeTaskRecord {
	if s == nil || ensurePrivateRuntimeDirectory(s.baseDir) != nil {
		return nil
	}
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil
	}
	out := make([]RuntimeTaskRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, readErr := s.readEntryRecord(entry.Name())
		if readErr != nil || validateRuntimeStorageID(record.ID) != nil ||
			sanitizeTaskPathComponent(record.ID) != strings.TrimSuffix(entry.Name(), ".json") {
			continue
		}
		s.normalizeManagedPaths(&record)
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		if out[i].StartedAt.IsZero() {
			return false
		}
		if out[j].StartedAt.IsZero() {
			return true
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out
}

// decodeRuntimeTaskRecord preserves the current snake_case format while
// accepting TS/camelCase fields and the early {"task": {...}} wrapper used by
// experimental Go builds. Saving the record migrates it to the current shape.
func decodeRuntimeTaskRecord(data []byte) (RuntimeTaskRecord, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return RuntimeTaskRecord{}, fmt.Errorf("empty runtime task record")
	}

	var wrapped struct {
		Task   json.RawMessage `json:"task"`
		Record json.RawMessage `json:"record"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return RuntimeTaskRecord{}, err
	}
	if len(wrapped.Task) > 0 && string(wrapped.Task) != "null" {
		data = wrapped.Task
	} else if len(wrapped.Record) > 0 && string(wrapped.Record) != "null" {
		data = wrapped.Record
	}

	var record RuntimeTaskRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return RuntimeTaskRecord{}, err
	}
	var legacy struct {
		OutputPath             string                `json:"outputPath"`
		ExitCode               *int                  `json:"exitCode"`
		StartedAt              time.Time             `json:"startedAt"`
		FinishedAt             *time.Time            `json:"finishedAt"`
		UpdatedAt              time.Time             `json:"updatedAt"`
		OutputOffset           int64                 `json:"outputOffset"`
		AgentAlias             string                `json:"agentAlias"`
		AgentInput             *AgentInput           `json:"agentInput"`
		AgentMetadata          *agentSessionMetadata `json:"agentMetadata"`
		AgentMessages          []types.Message       `json:"agentMessages"`
		OwnerSessionID         string                `json:"ownerSessionId"`
		OwnerSessionProjectDir string                `json:"ownerSessionProjectDir"`
		OwnerProjectRoot       string                `json:"ownerProjectRoot"`
		OwnerAgentID           string                `json:"ownerAgentId"`
		OwnerPID               int                   `json:"ownerPid"`
		CurrentRunID           string                `json:"currentRunId"`
		Attempt                int                   `json:"attempt"`
		BatchID                string                `json:"batchId"`
		ParentRunID            string                `json:"parentRunId"`
		AgentPath              string                `json:"agentPath"`
		QueuedPrompts          int                   `json:"queuedPrompts"`
		QueueReason            string                `json:"queueReason"`
		LatestProgress         *AgentProgressEvent   `json:"latestProgress"`
		TranscriptPath         string                `json:"transcriptPath"`
		DurationMs             *int64                `json:"durationMs"`
		TotalTokens            *int                  `json:"totalTokens"`
		Outcome                AgentRunOutcome       `json:"outcome"`
		TerminalReason         string                `json:"terminalReason"`
		ArtifactRefs           []string              `json:"artifactRefs"`
		VerificationRefs       []string              `json:"verificationRefs"`
		Notification           *RuntimeNotification  `json:"runtimeNotification"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return RuntimeTaskRecord{}, err
	}
	if record.OutputPath == "" {
		record.OutputPath = legacy.OutputPath
	}
	if record.ExitCode == nil {
		record.ExitCode = legacy.ExitCode
	}
	if record.StartedAt.IsZero() {
		record.StartedAt = legacy.StartedAt
	}
	if record.FinishedAt == nil {
		record.FinishedAt = legacy.FinishedAt
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = legacy.UpdatedAt
	}
	if record.OutputOffset == 0 {
		record.OutputOffset = legacy.OutputOffset
	}
	if record.AgentAlias == "" {
		record.AgentAlias = legacy.AgentAlias
	}
	if record.AgentInput == nil {
		record.AgentInput = legacy.AgentInput
	}
	if record.AgentMetadata == nil {
		record.AgentMetadata = legacy.AgentMetadata
	}
	if record.AgentMessages == nil {
		record.AgentMessages = legacy.AgentMessages
	}
	if record.OwnerSessionID == "" {
		record.OwnerSessionID = legacy.OwnerSessionID
	}
	if record.OwnerSessionProjectDir == "" {
		record.OwnerSessionProjectDir = legacy.OwnerSessionProjectDir
	}
	if record.OwnerProjectRoot == "" {
		record.OwnerProjectRoot = legacy.OwnerProjectRoot
	}
	if record.OwnerAgentID == "" {
		record.OwnerAgentID = legacy.OwnerAgentID
	}
	if record.OwnerPID == 0 {
		record.OwnerPID = legacy.OwnerPID
	}
	if record.CurrentRunID == "" {
		record.CurrentRunID = legacy.CurrentRunID
	}
	if record.Attempt == 0 {
		record.Attempt = legacy.Attempt
	}
	if record.BatchID == "" {
		record.BatchID = legacy.BatchID
	}
	if record.ParentRunID == "" {
		record.ParentRunID = legacy.ParentRunID
	}
	if record.AgentPath == "" {
		record.AgentPath = legacy.AgentPath
	}
	if record.QueuedPrompts == 0 {
		record.QueuedPrompts = legacy.QueuedPrompts
	}
	if record.QueueReason == "" {
		record.QueueReason = legacy.QueueReason
	}
	if record.LatestProgress == nil {
		record.LatestProgress = cloneAgentProgressEvent(legacy.LatestProgress)
	}
	if record.TranscriptPath == "" {
		record.TranscriptPath = legacy.TranscriptPath
	}
	if record.DurationMs == nil {
		record.DurationMs = cloneInt64Pointer(legacy.DurationMs)
	}
	if record.TotalTokens == nil {
		record.TotalTokens = cloneIntPointer(legacy.TotalTokens)
	}
	if record.Outcome == "" {
		record.Outcome = legacy.Outcome
	}
	if record.TerminalReason == "" {
		record.TerminalReason = legacy.TerminalReason
	}
	if record.ArtifactRefs == nil {
		record.ArtifactRefs = append([]string(nil), legacy.ArtifactRefs...)
	}
	if record.VerificationRefs == nil {
		record.VerificationRefs = append([]string(nil), legacy.VerificationRefs...)
	}
	if record.Notification == nil {
		record.Notification = legacy.Notification
	}
	normalizeRuntimeTaskRunHistory(&record)
	return record, nil
}

func normalizeRuntimeTaskRunHistory(record *RuntimeTaskRecord) {
	if record == nil || record.Type != backgroundTaskTypeLocalAgent {
		return
	}
	if record.Attempt <= 0 && len(record.Runs) > 0 {
		record.Attempt = record.Runs[len(record.Runs)-1].Attempt
	}
	if record.CurrentRunID == "" && len(record.Runs) > 0 {
		record.CurrentRunID = record.Runs[len(record.Runs)-1].RunID
	}
	if len(record.Runs) == 0 && !record.StartedAt.IsZero() {
		if record.Attempt <= 0 {
			record.Attempt = 1
		}
		if record.CurrentRunID == "" {
			record.CurrentRunID = legacyAgentRunID(record.ID, record.Attempt)
		}
		record.Runs = []RuntimeTaskRunRecord{{
			RunID: record.CurrentRunID, Attempt: record.Attempt,
			BatchID: record.BatchID, ParentRunID: record.ParentRunID, AgentPath: record.AgentPath,
			Status: record.Status, Prompt: record.Prompt, StartedAt: record.StartedAt,
			FinishedAt: cloneTimePointer(record.FinishedAt), UpdatedAt: record.UpdatedAt,
			TranscriptPath: record.TranscriptPath, DurationMs: cloneInt64Pointer(record.DurationMs),
			TotalTokens: cloneIntPointer(record.TotalTokens), Usage: cloneUsagePointer(record.Usage),
			Error: record.Error, Result: record.Result, Outcome: record.Outcome,
			TerminalReason: record.TerminalReason, ArtifactRefs: append([]string(nil), record.ArtifactRefs...),
			VerificationRefs: append([]string(nil), record.VerificationRefs...), LatestProgress: cloneAgentProgressEvent(record.LatestProgress),
		}}
	}
	if record.Outcome == "" {
		record.Outcome = inferAgentRunOutcome(record.Status, record.Error)
	}
	for index := range record.Runs {
		if record.Runs[index].Outcome == "" {
			record.Runs[index].Outcome = inferAgentRunOutcome(record.Runs[index].Status, record.Runs[index].Error)
		}
	}
}

func inferAgentRunOutcome(status, runError string) AgentRunOutcome {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "queued", "spawning", "waiting", "blocked":
		return AgentRunOutcomeRunning
	case "completed", "succeeded", "success":
		return AgentRunOutcomeSucceeded
	case "partial":
		return AgentRunOutcomePartial
	case "cancelled", "canceled", "killed", "aborted":
		return AgentRunOutcomeCancelled
	case "timed_out", "timeout":
		return AgentRunOutcomeTimedOut
	case "interrupted", "orphan", "orphaned":
		return AgentRunOutcomeInterrupted
	case "failed", "error":
		return AgentRunOutcomeFailed
	default:
		if strings.TrimSpace(runError) != "" {
			return AgentRunOutcomeFailed
		}
		return ""
	}
}

func legacyAgentRunID(agentID string, attempt int) string {
	if attempt <= 0 {
		attempt = 1
	}
	return fmt.Sprintf("legacy:%s:%d", sanitizeTaskPathComponent(strings.TrimSpace(agentID)), attempt)
}

func cloneRuntimeTaskRunRecords(runs []RuntimeTaskRunRecord) []RuntimeTaskRunRecord {
	if len(runs) == 0 {
		return nil
	}
	out := make([]RuntimeTaskRunRecord, len(runs))
	for index, run := range runs {
		out[index] = run
		out[index].FinishedAt = cloneTimePointer(run.FinishedAt)
		out[index].DurationMs = cloneInt64Pointer(run.DurationMs)
		out[index].TotalTokens = cloneIntPointer(run.TotalTokens)
		out[index].Usage = cloneUsagePointer(run.Usage)
		out[index].ArtifactRefs = append([]string(nil), run.ArtifactRefs...)
		out[index].VerificationRefs = append([]string(nil), run.VerificationRefs...)
		out[index].LatestProgress = cloneAgentProgressEvent(run.LatestProgress)
	}
	return out
}

func cloneAgentProgressEvent(event *AgentProgressEvent) *AgentProgressEvent {
	if event == nil {
		return nil
	}
	copy := *event
	copy.Usage = cloneUsagePointer(event.Usage)
	copy.LastRequestUsage = cloneUsagePointer(event.LastRequestUsage)
	return &copy
}

func cloneUsagePointer(usage *types.Usage) *types.Usage {
	if usage == nil {
		return nil
	}
	copy := *usage
	return &copy
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (s *RuntimeTaskStore) FindLocalAgentByAlias(alias string) (RuntimeTaskRecord, bool) {
	trimmed := strings.TrimSpace(alias)
	if s == nil || trimmed == "" || ensurePrivateRuntimeDirectory(s.baseDir) != nil {
		return RuntimeTaskRecord{}, false
	}
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return RuntimeTaskRecord{}, false
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, readErr := s.readEntryRecord(entry.Name())
		if readErr != nil || validateRuntimeStorageID(record.ID) != nil ||
			sanitizeTaskPathComponent(record.ID) != strings.TrimSuffix(entry.Name(), ".json") ||
			record.Type != backgroundTaskTypeLocalAgent {
			continue
		}
		s.normalizeManagedPaths(&record)
		if strings.EqualFold(record.AgentAlias, trimmed) {
			return record, true
		}
		if record.AgentInput != nil && strings.EqualFold(record.AgentInput.Name, trimmed) {
			return record, true
		}
	}
	return RuntimeTaskRecord{}, false
}

func (s *RuntimeTaskStore) readEntryRecord(name string) (RuntimeTaskRecord, error) {
	if s == nil || filepath.Base(name) != name || !strings.HasSuffix(name, ".json") {
		return RuntimeTaskRecord{}, fs.ErrInvalid
	}
	path := filepath.Join(s.baseDir, name)
	lockPath := path + ".lock"
	if err := preparePrivateRuntimeLock(lockPath); err != nil {
		return RuntimeTaskRecord{}, err
	}
	value, err := withRuntimeFileLockResult(lockPath, func() (any, error) {
		data, err := readPrivateRuntimeRegularFile(path)
		if err != nil {
			return nil, err
		}
		return decodeRuntimeTaskRecord(data)
	})
	if err != nil {
		return RuntimeTaskRecord{}, err
	}
	record, ok := value.(RuntimeTaskRecord)
	if !ok {
		return RuntimeTaskRecord{}, fs.ErrInvalid
	}
	return record, nil
}

func validateRuntimeStorageID(id string) error {
	if id == "" || id != strings.TrimSpace(id) || len(id) > maxRuntimeStorageIDBytes ||
		id == "." || id == ".." || filepath.IsAbs(id) || filepath.VolumeName(id) != "" {
		return fs.ErrInvalid
	}
	for _, r := range id {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return fs.ErrInvalid
		}
	}
	return nil
}
