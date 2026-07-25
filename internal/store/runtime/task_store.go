package runtime

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
	"unicode/utf16"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/types"
)

type RuntimeTaskRecord struct {
	ID                     string                              `json:"id"`
	Type                   string                              `json:"type"`
	Status                 string                              `json:"status"`
	Description            string                              `json:"description,omitempty"`
	Command                string                              `json:"command,omitempty"`
	Prompt                 string                              `json:"prompt,omitempty"`
	OutputPath             string                              `json:"output_path,omitempty"`
	ExitCode               *int                                `json:"exit_code,omitempty"`
	Error                  string                              `json:"error,omitempty"`
	Result                 string                              `json:"result,omitempty"`
	StartedAt              time.Time                           `json:"started_at"`
	FinishedAt             *time.Time                          `json:"finished_at,omitempty"`
	UpdatedAt              time.Time                           `json:"updated_at"`
	OutputOffset           int64                               `json:"output_offset,omitempty"`
	AgentAlias             string                              `json:"agent_alias,omitempty"`
	AgentInput             *agentcontract.Input                `json:"agent_input,omitempty"`
	Detached               bool                                `json:"detached,omitempty"`
	AgentMetadata          *agentcontract.SessionMetadata      `json:"agent_metadata,omitempty"`
	AgentMessages          []types.Message                     `json:"agent_messages,omitempty"`
	OwnerSessionID         string                              `json:"owner_session_id,omitempty"`
	OwnerSessionProjectDir string                              `json:"owner_session_project_dir,omitempty"`
	OwnerProjectRoot       string                              `json:"owner_project_root,omitempty"`
	OwnerAgentID           string                              `json:"owner_agent_id,omitempty"`
	OwnerPID               int                                 `json:"owner_pid,omitempty"`
	CurrentRunID           string                              `json:"current_run_id,omitempty"`
	Attempt                int                                 `json:"attempt,omitempty"`
	BatchID                string                              `json:"batch_id,omitempty"`
	ParentRunID            string                              `json:"parent_run_id,omitempty"`
	AgentPath              string                              `json:"agent_path,omitempty"`
	QueuedPrompts          int                                 `json:"queued_prompts,omitempty"`
	QueueReason            string                              `json:"queue_reason,omitempty"`
	Runs                   []agentcontract.RunRecord           `json:"runs,omitempty"`
	LatestProgress         *agentcontract.ProgressEvent        `json:"latest_progress,omitempty"`
	TranscriptPath         string                              `json:"transcript_path,omitempty"`
	DurationMs             *int64                              `json:"duration_ms,omitempty"`
	TotalTokens            *int                                `json:"total_tokens,omitempty"`
	Usage                  *types.Usage                        `json:"usage,omitempty"`
	Outcome                agentcontract.RunOutcome            `json:"outcome,omitempty"`
	TerminalReason         string                              `json:"terminal_reason,omitempty"`
	TimeoutNanos           int64                               `json:"timeout_nanos,omitempty"`
	ArtifactRefs           []string                            `json:"artifact_refs,omitempty"`
	VerificationRefs       []string                            `json:"verification_refs,omitempty"`
	Notification           *agentcontract.RuntimeNotification  `json:"notification,omitempty"`
	Notifications          []agentcontract.RuntimeNotification `json:"notifications,omitempty"`
}

type RuntimeTaskStore struct {
	baseDir string
}

const maxRuntimeStorageIDBytes = 200

func sanitizeTaskPathComponent(input string) string {
	var builder strings.Builder
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			for range utf16.RuneLen(r) {
				builder.WriteByte('-')
			}
		}
	}
	return builder.String()
}

// SafeTaskPathComponent projects a runtime task identity onto the canonical
// single-component storage form shared by task records and their artifacts.
func SafeTaskPathComponent(input string) string {
	return sanitizeTaskPathComponent(input)
}

func NewRuntimeTaskStore(projectRoot string) *RuntimeTaskStore {
	root := filepath.Clean(strings.TrimSpace(projectRoot))
	if root == "" {
		root = "."
	}
	store := &RuntimeTaskStore{baseDir: filepath.Join(root, ".luban-code", "runtime-tasks")}
	// Prepare the private directory eagerly. Every operation revalidates it and
	// fails closed before touching a managed path.
	_ = secureio.EnsurePrivateRuntimeDirectory(store.baseDir)
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

// OutputPathComponent returns the canonical collision-resistant artifact name
// component for a runtime task identity.
func OutputPathComponent(id string) string {
	return runtimeOutputPathComponent(id)
}

func (s *RuntimeTaskStore) normalizeManagedPaths(record *RuntimeTaskRecord) {
	if s == nil || record == nil {
		return
	}
	if record.Type == agentcontract.TaskTypeLocalAgent || record.Type == agentcontract.TaskTypeLocalBash {
		record.OutputPath = s.outputPath(record.ID)
	}
}

func (s *RuntimeTaskStore) Exists(id string) bool {
	if s == nil || validateRuntimeStorageID(id) != nil || secureio.EnsurePrivateRuntimeDirectory(s.baseDir) != nil {
		return false
	}
	_, err := secureio.TightenPrivateRuntimeRegularFile(s.path(id), false)
	return err == nil
}

func (s *RuntimeTaskStore) Save(record RuntimeTaskRecord) error {
	if s == nil {
		return fs.ErrInvalid
	}
	if err := validateRuntimeStorageID(record.ID); err != nil {
		return err
	}
	if err := secureio.EnsurePrivateRuntimeDirectory(s.baseDir); err != nil {
		return err
	}
	if err := secureio.PreparePrivateRuntimeLock(s.lockPath(record.ID)); err != nil {
		return err
	}
	s.normalizeManagedPaths(&record)
	record.UpdatedAt = time.Now().UTC()
	return secureio.WithRuntimeFileLock(s.lockPath(record.ID), func() error {
		path := s.path(record.ID)
		if current, err := secureio.ReadPrivateRuntimeRegularFile(path); err == nil {
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
		return secureio.AtomicWritePrivateRuntimeFile(path, data)
	})
}

func (s *RuntimeTaskStore) Get(id string) (RuntimeTaskRecord, bool) {
	if s == nil || validateRuntimeStorageID(id) != nil || secureio.EnsurePrivateRuntimeDirectory(s.baseDir) != nil ||
		secureio.PreparePrivateRuntimeLock(s.lockPath(id)) != nil {
		return RuntimeTaskRecord{}, false
	}
	value, err := secureio.WithRuntimeFileLockResult(s.lockPath(id), func() (any, error) {
		data, err := secureio.ReadPrivateRuntimeRegularFile(s.path(id))
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
	if s == nil || secureio.EnsurePrivateRuntimeDirectory(s.baseDir) != nil {
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

func decodeRuntimeTaskRecord(data []byte) (RuntimeTaskRecord, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return RuntimeTaskRecord{}, fmt.Errorf("empty runtime task record")
	}
	var record RuntimeTaskRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return RuntimeTaskRecord{}, err
	}
	return record, nil
}

func (s *RuntimeTaskStore) FindLocalAgentByAlias(alias string) (RuntimeTaskRecord, bool) {
	trimmed := strings.TrimSpace(alias)
	if s == nil || trimmed == "" || secureio.EnsurePrivateRuntimeDirectory(s.baseDir) != nil {
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
			record.Type != agentcontract.TaskTypeLocalAgent {
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
	if err := secureio.PreparePrivateRuntimeLock(lockPath); err != nil {
		return RuntimeTaskRecord{}, err
	}
	value, err := secureio.WithRuntimeFileLockResult(lockPath, func() (any, error) {
		data, err := secureio.ReadPrivateRuntimeRegularFile(path)
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

// ValidateTaskID rejects identities that cannot safely own one runtime-task
// record and its adjacent artifacts.
func ValidateTaskID(id string) error {
	return validateRuntimeStorageID(id)
}
