package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/internal/securestore"
	"github.com/agent-dance/luban/types"
)

// Store persists conversation sessions.
type Store interface {
	Save(sessionID string, messages []types.Message) error
	Load(sessionID string) ([]types.Message, error)
	List() ([]SessionInfo, error)
	Latest() (string, error)
}

// SessionInfo holds metadata about a stored session.
type SessionInfo struct {
	ID           string    `json:"id"`
	ProjectDir   string    `json:"project_dir,omitempty"`
	Title        string    `json:"title,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Turns        int       `json:"turns"`
	MessageCount int       `json:"message_count,omitempty"`
	CWD          string    `json:"cwd,omitempty"`
	GitBranch    string    `json:"git_branch,omitempty"`
	PreviewText  string    `json:"preview_text,omitempty"`
	Provider     string    `json:"provider,omitempty"` // Phase 5: provider used in session
	Model        string    `json:"model,omitempty"`    // Phase 5: model used in session
}

// SearchOptions configures session listing and filtering.
type SearchOptions struct {
	Query             string
	CurrentCWD        string
	CurrentProjectDir string
	AllProjects       bool
}

// FileStore implements Store using JSONL files on disk plus a metadata sidecar.
type FileStore struct {
	dir                  string
	mu                   sync.Mutex // protects concurrent Save/Load operations
	rootMu               sync.Mutex
	root                 *securestore.Root
	rootErr              error
	historyLockMu        sync.Mutex
	historyLocks         map[string]*os.File
	writerFingerprint    func() buildinfo.Fingerprint
	historyCommitFault   func(HistoryCommitStage) error
	metaWriteFault       func() error
	storageBeforePublish func()
	removeFile           func(string) error
	removeTree           func(string) error
	writeDeleteMarker    func(string) error
}

// ErrSessionDeleted means a session ID has a durable delete-history marker.
// The marker is intentionally retained after physical cleanup so delayed work
// cannot recreate the transcript after a process restart.
var ErrSessionDeleted = errors.New("session history permanently deleted")

// ErrNoSessions distinguishes an empty store from an unreadable/corrupt one.
// Startup may safely create a fresh session only for this condition.
var ErrNoSessions = errors.New("no sessions found")

// SessionMeta is the persisted sidecar metadata for a session.
type SessionMeta struct {
	ID string `json:"id"`
	// CacheLineageID is the stable prompt-cache routing identity. New sessions
	// use their own ID; forked sessions inherit the source lineage.
	CacheLineageID string                   `json:"cache_lineage_id,omitempty"`
	Title          string                   `json:"title,omitempty"`
	CreatedAt      time.Time                `json:"created_at,omitempty"`
	UpdatedAt      time.Time                `json:"updated_at,omitempty"`
	MessageCount   int                      `json:"message_count,omitempty"`
	CWD            string                   `json:"cwd,omitempty"`
	GitBranch      string                   `json:"git_branch,omitempty"`
	PreviewText    string                   `json:"preview_text,omitempty"`
	Provider       string                   `json:"provider,omitempty"` // Phase 5: provider used in session
	Model          string                   `json:"model,omitempty"`    // Phase 5: model used in session
	Goal           *goal.Goal               `json:"goal,omitempty"`
	Usage          *SessionUsageMeta        `json:"usage,omitempty"`
	Presentation   *SessionPresentationMeta `json:"presentation,omitempty"`
	Skills         *SessionSkillsMeta       `json:"skills,omitempty"`
	Decisions      []SessionDecisionMeta    `json:"decisions,omitempty"`
	Evidence       []SessionEvidenceMeta    `json:"evidence,omitempty"`
	Activities     []SessionActivityMeta    `json:"activities,omitempty"`
	// SeenToolUseIDs is the session-lifetime identity ledger. It intentionally
	// outlives model-context compaction so a resumed provider response cannot
	// reuse an ID whose original tool message has left the visible transcript.
	SeenToolUseIDs []string `json:"seen_tool_use_ids,omitempty"`
	// LoadedToolNames preserves deferred tool schemas already exposed to the
	// model. Forks derive this list from their selected visible prefix so tools
	// discovered later in the source conversation cannot leak backward.
	LoadedToolNames []string `json:"loaded_tool_names,omitempty"`
	// FirstWriterBuild identifies the first process recorded by this schema;
	// LastWriterBuild identifies the process responsible for its latest write.
	// Keeping both prevents a resume from erasing the earliest available build
	// evidence while still making later mutations attributable.
	FirstWriterBuild *buildinfo.Fingerprint `json:"first_writer_build,omitempty"`
	LastWriterBuild  *buildinfo.Fingerprint `json:"last_writer_build,omitempty"`
}

type SessionDetailRefMeta struct {
	Source string `json:"source"`
	Key    string `json:"key"`
	Size   int    `json:"size"`
	Digest string `json:"sha256"`
}

type SessionEvidenceMeta struct {
	ObservationID string                 `json:"observation_id"`
	SessionID     string                 `json:"session_id,omitempty"`
	TurnID        string                 `json:"turn_id,omitempty"`
	ToolUseID     string                 `json:"tool_use_id,omitempty"`
	ToolName      string                 `json:"tool_name,omitempty"`
	WorkUnitID    string                 `json:"work_unit_id,omitempty"`
	ActorID       string                 `json:"actor_id,omitempty"`
	Outcome       string                 `json:"outcome,omitempty"`
	Disclosure    int                    `json:"disclosure,omitempty"`
	DisclosureSet bool                   `json:"disclosure_set,omitempty"`
	HasMore       bool                   `json:"has_more,omitempty"`
	UserPinned    bool                   `json:"user_pinned,omitempty"`
	Results       []SessionDetailRefMeta `json:"results,omitempty"`
	Envelopes     []SessionDetailRefMeta `json:"envelopes,omitempty"`
}

type SessionUsageMeta struct {
	InputTokens                int     `json:"input_tokens,omitempty"`
	OutputTokens               int     `json:"output_tokens,omitempty"`
	CacheReadTokens            int     `json:"cache_read_tokens,omitempty"`
	CacheCreateTokens          int     `json:"cache_create_tokens,omitempty"`
	HasCompacted               bool    `json:"has_compacted,omitempty"`
	RoundUsageKnown            bool    `json:"round_usage_known,omitempty"`
	CompactionCount            int     `json:"compaction_count,omitempty"`
	CompletedRoundInputTokens  int     `json:"completed_round_input_tokens,omitempty"`
	CompletedRoundOutputTokens int     `json:"completed_round_output_tokens,omitempty"`
	InputTokensAtCompact       int     `json:"input_tokens_at_compact,omitempty"`
	CacheReadAtCompact         int     `json:"cache_read_at_compact,omitempty"`
	LastInputTokens            int     `json:"last_input_tokens,omitempty"`
	LastOutputTokens           int     `json:"last_output_tokens,omitempty"`
	LastCacheReadTokens        int     `json:"last_cache_read_tokens,omitempty"`
	LastCacheCreateTokens      int     `json:"last_cache_create_tokens,omitempty"`
	WebSearchRequests          int     `json:"web_search_requests,omitempty"`
	CumulativeCost             float64 `json:"cumulative_cost,omitempty"`
	CostKnown                  bool    `json:"cost_known,omitempty"`
	UsedTokens                 int     `json:"used_tokens,omitempty"`
	MaxTokens                  int     `json:"max_tokens,omitempty"`
}

type SessionPresentationMeta struct {
	Version              int                                    `json:"version,omitempty"`
	FocusedObservationID string                                 `json:"focused_observation_id,omitempty"`
	ScrollAnchorID       string                                 `json:"scroll_anchor_id,omitempty"`
	ScrollOffset         int                                    `json:"scroll_offset,omitempty"`
	InputDraft           string                                 `json:"input_draft,omitempty"`
	InputCursor          int                                    `json:"input_cursor,omitempty"`
	InputCursorSet       bool                                   `json:"input_cursor_set,omitempty"`
	PermissionMode       string                                 `json:"permission_mode,omitempty"`
	ActivityFocus        string                                 `json:"activity_focus,omitempty"`
	ActivityViewOffset   int                                    `json:"activity_view_offset,omitempty"`
	DisclosureReturns    map[string]SessionDisclosureReturnMeta `json:"disclosure_returns,omitempty"`
}

// SessionActivityMetaVersion identifies which activity-only fields were
// written deliberately instead of being absent because an older writer did
// not know about them.
type SessionActivityMetaVersion uint8

const (
	SessionActivityMetaVersionLegacy      SessionActivityMetaVersion = iota
	SessionActivityMetaVersionProvisional                            // v1 persists provisional runtime-error state.
)

type SessionActivityMeta struct {
	Version           SessionActivityMetaVersion `json:"version,omitempty"`
	ID                string                     `json:"id"`
	RunID             string                     `json:"run_id,omitempty"`
	SupersedesRunID   string                     `json:"supersedes_run_id,omitempty"`
	Attempt           int                        `json:"attempt,omitempty"`
	BatchID           string                     `json:"batch_id,omitempty"`
	ParentRunID       string                     `json:"parent_run_id,omitempty"`
	AgentPath         string                     `json:"agent_path,omitempty"`
	TurnID            string                     `json:"turn_id,omitempty"`
	WorkUnitID        string                     `json:"work_unit_id,omitempty"`
	ActorID           string                     `json:"actor_id,omitempty"`
	ActorType         string                     `json:"actor_type,omitempty"`
	Kind              string                     `json:"kind,omitempty"`
	Name              string                     `json:"name,omitempty"`
	Phase             string                     `json:"phase,omitempty"`
	State             string                     `json:"state,omitempty"`
	Lifecycle         string                     `json:"lifecycle,omitempty"`
	AttentionKind     string                     `json:"attention_kind,omitempty"`
	AttentionSeverity string                     `json:"attention_severity,omitempty"`
	AttentionUnread   bool                       `json:"attention_unread,omitempty"`
	DecisionID        string                     `json:"decision_id,omitempty"`
	AttentionMessage  string                     `json:"attention_message,omitempty"`
	Outcome           string                     `json:"outcome,omitempty"`
	Provisional       bool                       `json:"provisional,omitempty"`
	SourceSequence    uint64                     `json:"source_sequence,omitempty"`
	ProgressCurrent   int                        `json:"progress_current,omitempty"`
	ProgressTotal     int                        `json:"progress_total,omitempty"`
	ProgressMessage   string                     `json:"progress_message,omitempty"`
	Cancelable        bool                       `json:"cancelable,omitempty"`
	JumpTarget        string                     `json:"jump_target,omitempty"`
	DetailRefs        []SessionDetailRefMeta     `json:"detail_refs,omitempty"`
	OccurrenceCount   int                        `json:"occurrence_count,omitempty"`
	FirstSequence     uint64                     `json:"first_sequence,omitempty"`
	LastSequence      uint64                     `json:"last_sequence,omitempty"`
	Acknowledged      bool                       `json:"acknowledged,omitempty"`
}

type SessionDisclosureReturnMeta struct {
	FocusedObservationID string `json:"focused_observation_id,omitempty"`
	ScrollAnchorID       string `json:"scroll_anchor_id,omitempty"`
	ScrollOffset         int    `json:"scroll_offset,omitempty"`
	InputDraft           string `json:"input_draft,omitempty"`
	InputCursor          int    `json:"input_cursor,omitempty"`
	InputCursorSet       bool   `json:"input_cursor_set,omitempty"`
}

type SessionDecisionMeta struct {
	DecisionID         string         `json:"decision_id"`
	ExecutionSessionID string         `json:"execution_session_id,omitempty"`
	TurnID             string         `json:"turn_id,omitempty"`
	ToolUseID          string         `json:"tool_use_id,omitempty"`
	ToolName           string         `json:"tool_name,omitempty"`
	Input              map[string]any `json:"input,omitempty"`
	ActorID            string         `json:"actor_id,omitempty"`
	ActorType          string         `json:"actor_type,omitempty"`
	WorkUnitID         string         `json:"work_unit_id,omitempty"`
	Kind               string         `json:"kind,omitempty"`
	Action             string         `json:"action,omitempty"`
	Target             string         `json:"target,omitempty"`
	Impact             string         `json:"impact,omitempty"`
	RiskLevel          int            `json:"risk_level,omitempty"`
	RiskReason         string         `json:"risk_reason,omitempty"`
	Message            string         `json:"message,omitempty"`
	RuleSource         string         `json:"rule_source,omitempty"`
	ApprovalScope      string         `json:"approval_scope,omitempty"`
	Choices            []string       `json:"choices,omitempty"`
	Body               string         `json:"body,omitempty"`
	ReviewDetails      []string       `json:"review_details,omitempty"`
	PostMode           string         `json:"post_mode,omitempty"`
	Outcome            string         `json:"outcome,omitempty"`
	Choice             string         `json:"choice,omitempty"`
	Decision           *int           `json:"decision,omitempty"`
	ResolvedAt         time.Time      `json:"resolved_at,omitempty"`
}

const maxStorageIDBytes = 200

func validateStorageID(id string) error {
	if id == "" || len(id) > maxStorageIDBytes {
		return fs.ErrInvalid
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fs.ErrInvalid
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	if isVolumeRoot(path) {
		return fs.ErrInvalid
	}
	f, err := openPrivateDirectory(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		f, err = openPrivateDirectory(path)
	}
	if err != nil {
		return err
	}
	return f.Close()
}

func openPrivateDirectory(path string) (*os.File, error) {
	f, err := openPathNoFollow(path, os.O_RDONLY, 0, true)
	if err != nil {
		return nil, err
	}
	before, err := f.Stat()
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	if err := f.Chmod(0o700); err != nil {
		_ = f.Close()
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !after.IsDir() || !os.SameFile(before, after) {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return f, nil
}

func isVolumeRoot(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	volume := filepath.VolumeName(abs)
	return filepath.Clean(abs) == filepath.Clean(volume+string(os.PathSeparator))
}

func tightenPrivateRegularFile(path string, missingOK bool) (fs.FileInfo, error) {
	f, err := openPrivateRegularFile(path)
	if errors.Is(err, fs.ErrNotExist) && missingOK {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func openPrivateRegularFile(path string) (*os.File, error) {
	return openPrivateRegularFileWithFlags(path, os.O_RDONLY, 0)
}

func openPrivateRegularFileWithFlags(path string, flag int, perm fs.FileMode) (*os.File, error) {
	f, err := openPathNoFollow(path, flag, perm, false)
	if err != nil {
		return nil, err
	}
	if _, err := validateAndTightenPrivateRegularFile(f, path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func openOrCreatePrivateRegularFile(path string) (*os.File, error) {
	for range 32 {
		f, err := openPrivateRegularFileWithFlags(path, os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		f, err = openPrivateRegularFileWithFlags(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
}

func validateAndTightenPrivateRegularFile(f *os.File, path string) (fs.FileInfo, error) {
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if err := validatePrivateRegularFileInfo(path, "open", before); err != nil {
		return nil, err
	}
	if err := validatePrivateRegularFileLinkCount(path, "open", before); err != nil {
		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if err := validatePrivateRegularFileInfo(path, "open", after); err != nil {
		return nil, err
	}
	if err := validatePrivateRegularFileLinkCount(path, "open", after); err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) {
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return after, nil
}

func validatePrivateRegularFileInfo(path, op string, info fs.FileInfo) error {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return &os.PathError{Op: op, Path: path, Err: fs.ErrInvalid}
	}
	return nil
}

func readPrivateRegularFile(path string) ([]byte, error) {
	f, err := openPrivateRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if _, err := validateAndTightenPrivateRegularFile(f, path); err != nil {
		return nil, err
	}
	return data, nil
}

func writePrivateFileAtomic(dir, path, pattern string, data []byte) error {
	if err := ensurePrivateDirectory(dir); err != nil {
		return err
	}
	if _, err := tightenPrivateRegularFile(path, true); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if n, err := tmp.Write(data); err != nil {
		return err
	} else if n != len(data) {
		return io.ErrShortWrite
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncPrivateDirectory(dir)
}

func syncPrivateDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// NewFileStore creates a file-based session store.
func NewFileStore(dir string) *FileStore {
	root, rootErr := securestore.Open(dir, true)
	store := &FileStore{
		dir:     dir,
		root:    root,
		rootErr: rootErr,
		writerFingerprint: func() buildinfo.Fingerprint {
			return buildinfo.Current("").Fingerprint
		},
	}
	store.removeFile = store.removePrivateFile
	store.removeTree = store.removePrivateTree
	store.writeDeleteMarker = store.writeDeleteMarkerAtomic
	return store
}

func (s *FileStore) ensureReadyLocked(sessionID string) error {
	if _, err := s.storageRoot(); err != nil {
		return err
	}
	if sessionID != "" {
		return validateStorageID(sessionID)
	}
	return nil
}

func (s *FileStore) storageRoot() (*securestore.Root, error) {
	s.rootMu.Lock()
	defer s.rootMu.Unlock()
	if s.root == nil {
		s.root, s.rootErr = securestore.Open(s.dir, true)
	}
	if s.rootErr != nil {
		return nil, s.rootErr
	}
	if err := s.root.Validate(); err != nil {
		return nil, err
	}
	return s.root, nil
}

func (s *FileStore) storageRelative(path string) (string, error) {
	absRoot, err := filepath.Abs(s.dir)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(filepath.Clean(absRoot), filepath.Clean(absPath))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fs.ErrInvalid
	}
	return rel, nil
}

func (s *FileStore) openPrivateRegularFile(path string) (*os.File, error) {
	return s.openPrivateRegularFileWithFlags(path, os.O_RDONLY, 0)
}

func (s *FileStore) openPrivateRegularFileWithFlags(path string, flag int, perm fs.FileMode) (*os.File, error) {
	root, err := s.storageRoot()
	if err != nil {
		return nil, err
	}
	rel, err := s.storageRelative(path)
	if err != nil {
		return nil, err
	}
	file, err := root.OpenFile(rel, flag, perm)
	if err != nil {
		return nil, err
	}
	if _, err := validateAndTightenPrivateRegularFile(file, path); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func (s *FileStore) openOrCreatePrivateRegularFile(path string) (*os.File, error) {
	for range 32 {
		file, err := s.openPrivateRegularFileWithFlags(path, os.O_RDWR, 0)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		file, err = s.openPrivateRegularFileWithFlags(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
}

func (s *FileStore) tightenPrivateRegularFile(path string, missingOK bool) (fs.FileInfo, error) {
	file, err := s.openPrivateRegularFile(path)
	if errors.Is(err, fs.ErrNotExist) && missingOK {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

func (s *FileStore) readPrivateRegularFile(path string) ([]byte, error) {
	file, err := s.openPrivateRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if _, err := validateAndTightenPrivateRegularFile(file, path); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *FileStore) ensurePrivateDirectory(path string) error {
	root, err := s.storageRoot()
	if err != nil {
		return err
	}
	rel, err := s.storageRelative(path)
	if err != nil {
		return err
	}
	return root.MkdirAll(rel)
}

func (s *FileStore) lstatPrivate(path string) (fs.FileInfo, error) {
	root, err := s.storageRoot()
	if err != nil {
		return nil, err
	}
	rel, err := s.storageRelative(path)
	if err != nil {
		return nil, err
	}
	return root.Lstat(rel)
}

func (s *FileStore) readPrivateDirectory(path string) ([]fs.DirEntry, error) {
	root, err := s.storageRoot()
	if err != nil {
		return nil, err
	}
	if filepath.Clean(path) == filepath.Clean(s.dir) {
		return root.ReadDir(".")
	}
	rel, err := s.storageRelative(path)
	if err != nil {
		return nil, err
	}
	return root.ReadDir(rel)
}

func (s *FileStore) writePrivateFileAtomic(dir, path, pattern string, data []byte) error {
	root, err := s.storageRoot()
	if err != nil {
		return err
	}
	rel, err := s.storageRelative(path)
	if err != nil {
		return err
	}
	parentRel := filepath.Dir(rel)
	expected := "."
	var relErr error
	if filepath.Clean(dir) != filepath.Clean(s.dir) {
		expected, relErr = s.storageRelative(dir)
	}
	if relErr != nil || filepath.Clean(expected) != filepath.Clean(parentRel) {
		return fs.ErrInvalid
	}
	parent, err := root.OpenRoot(parentRel, true)
	if err != nil {
		return err
	}
	defer parent.Close()
	base := filepath.Base(rel)
	if existing, openErr := parent.OpenFile(base, os.O_RDONLY, 0); openErr == nil {
		_, validateErr := validateAndTightenPrivateRegularFile(existing, path)
		closeErr := existing.Close()
		if validateErr != nil {
			return validateErr
		}
		if closeErr != nil {
			return closeErr
		}
	} else if !errors.Is(openErr, fs.ErrNotExist) {
		return openErr
	}
	tmp, tmpName, err := parent.CreateTemp(".", pattern)
	if err != nil {
		return err
	}
	tmpName = filepath.Base(tmpName)
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = parent.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if n, err := tmp.Write(data); err != nil {
		return err
	} else if n != len(data) {
		return io.ErrShortWrite
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	tmpInfo, err := validateAndTightenPrivateRegularFile(tmp, parent.Path(tmpName))
	if err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if s.storageBeforePublish != nil {
		s.storageBeforePublish()
	}
	if err := root.Validate(); err != nil {
		return err
	}
	if err := parent.Validate(); err != nil {
		return err
	}
	if err := s.validateActiveHistoryLockForPath(path); err != nil {
		return err
	}
	if err := parent.Rename(tmpName, base); err != nil {
		return err
	}
	published, err := parent.OpenFile(base, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	publishedInfo, validateErr := validateAndTightenPrivateRegularFile(published, path)
	if validateErr == nil && !os.SameFile(tmpInfo, publishedInfo) {
		validateErr = fs.ErrInvalid
	}
	closeErr := published.Close()
	if validateErr != nil {
		return validateErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := parent.Validate(); err != nil {
		return err
	}
	return parent.Sync(".")
}

func (s *FileStore) registerHistoryLock(sessionID string, file *os.File) {
	s.historyLockMu.Lock()
	defer s.historyLockMu.Unlock()
	if s.historyLocks == nil {
		s.historyLocks = make(map[string]*os.File)
	}
	s.historyLocks[sessionID] = file
}

func (s *FileStore) unregisterHistoryLock(sessionID string, file *os.File) {
	s.historyLockMu.Lock()
	defer s.historyLockMu.Unlock()
	if s.historyLocks[sessionID] == file {
		delete(s.historyLocks, sessionID)
	}
}

func (s *FileStore) validateActiveHistoryLockForPath(path string) error {
	sessionID := s.sessionIDForManagedPath(path)
	if sessionID == "" {
		return nil
	}
	s.historyLockMu.Lock()
	held := s.historyLocks[sessionID]
	s.historyLockMu.Unlock()
	if held == nil {
		return fs.ErrInvalid
	}
	heldInfo, err := held.Stat()
	if err != nil {
		return err
	}
	current, err := s.openPrivateRegularFile(filepath.Join(s.dir, "."+sessionID+".history.lock"))
	if err != nil {
		return err
	}
	currentInfo, statErr := current.Stat()
	closeErr := current.Close()
	if statErr != nil {
		return statErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !os.SameFile(heldInfo, currentInfo) {
		return fs.ErrInvalid
	}
	return nil
}

func (s *FileStore) sessionIDForManagedPath(path string) string {
	rel, err := s.storageRelative(path)
	if err != nil {
		return ""
	}
	first := strings.Split(rel, string(os.PathSeparator))[0]
	if validateStorageID(first) == nil {
		return first
	}
	for _, suffix := range []string{".context-v2.json", ".meta.json", ".jsonl", ".deleted"} {
		if strings.HasSuffix(first, suffix) {
			id := strings.TrimSuffix(first, suffix)
			if validateStorageID(id) == nil {
				return id
			}
		}
	}
	return ""
}

func (s *FileStore) removePrivateFile(path string) error {
	root, err := s.storageRoot()
	if err != nil {
		return err
	}
	rel, err := s.storageRelative(path)
	if err != nil {
		return err
	}
	return root.Remove(rel)
}

func (s *FileStore) removePrivateTree(path string) error {
	root, err := s.storageRoot()
	if err != nil {
		return err
	}
	rel, err := s.storageRelative(path)
	if err != nil {
		return err
	}
	return root.RemoveAll(rel)
}

func (s *FileStore) sessionPath(id string) string {
	if validateStorageID(id) != nil {
		return filepath.Join(s.dir, ".invalid-session-id")
	}
	return filepath.Join(s.dir, id+".jsonl")
}

// TranscriptPath returns the path to the JSONL transcript for sessionID.
func (s *FileStore) TranscriptPath(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted, err := s.isDeletedLocked(sessionID)
	if err != nil || deleted {
		return ""
	}
	unlock, err := s.lockSessionHistory(sessionID, false)
	if err != nil {
		return ""
	}
	defer unlock()
	// The compatibility transcript remains sensitive even when the v2 audit
	// transcript is authoritative. Refuse to hand out any transcript path while
	// that managed file has an external hard-link alias.
	if _, err := s.tightenPrivateRegularFile(s.sessionPath(sessionID), true); err != nil {
		return ""
	}
	manifest, exists, err := s.loadManifestLocked(sessionID)
	if err == nil && exists {
		if _, loadErr := s.loadAuditFromManifestLocked(manifest); loadErr != nil {
			return ""
		}
		if _, loadErr := s.loadViewFromManifestLocked(manifest); loadErr != nil {
			return ""
		}
		name, nameErr := digestFileName(manifest.AuditTranscript.Digest, ".jsonl")
		if nameErr != nil {
			return ""
		}
		path := filepath.Join(s.auditTranscriptDir(sessionID), name)
		if f, openErr := s.openPrivateRegularFile(path); openErr == nil {
			_ = f.Close()
			return path
		}
	}
	path := s.sessionPath(sessionID)
	f, err := s.openPrivateRegularFile(path)
	if err != nil {
		return ""
	}
	_ = f.Close()
	return path
}

func (s *FileStore) metaPath(id string) string {
	if validateStorageID(id) != nil {
		return filepath.Join(s.dir, ".invalid-session-id")
	}
	return filepath.Join(s.dir, id+".meta.json")
}

func (s *FileStore) tombstonePath(id string) string {
	if validateStorageID(id) != nil {
		return filepath.Join(s.dir, ".invalid-session-id")
	}
	return filepath.Join(s.dir, id+".deleted")
}

// ArtifactsDir returns the per-session artifacts directory used for result
// stores and other session-local runtime data.
func (s *FileStore) ArtifactsDir(id string) string {
	if validateStorageID(id) != nil {
		return filepath.Join(s.dir, ".invalid-session-id")
	}
	return filepath.Join(s.dir, id)
}

func (s *FileStore) Save(sessionID string, messages []types.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(sessionID); err != nil {
		return err
	}
	unlock, err := s.lockSessionHistory(sessionID, true)
	if err != nil {
		return err
	}
	defer unlock()
	return s.saveMessagesLocked(sessionID, messages)
}

func (s *FileStore) saveMessagesLocked(sessionID string, messages []types.Message) error {
	_, err := s.saveMessagesLockedCAS(sessionID, messages, nil)
	return err
}

// SaveModelContextCAS persists the compatibility metadata and atomically
// publishes a model view only if expectedGeneration is still current.
func (s *FileStore) SaveModelContextCAS(sessionID string, expectedGeneration uint64, messages []types.Message) (CompactionManifestV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(sessionID); err != nil {
		return CompactionManifestV2{}, err
	}
	unlock, err := s.lockSessionHistory(sessionID, true)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	defer unlock()
	return s.saveMessagesLockedCAS(sessionID, messages, &expectedGeneration)
}

func (s *FileStore) saveMessagesLockedCAS(sessionID string, messages []types.Message, expected *uint64) (CompactionManifestV2, error) {
	if err := s.ensureNotDeletedLocked(sessionID); err != nil {
		return CompactionManifestV2{}, err
	}
	meta, metaErr := s.loadMetaLocked(sessionID)
	if metaErr != nil && !errors.Is(metaErr, fs.ErrNotExist) {
		return CompactionManifestV2{}, fmt.Errorf("load existing session metadata before save: %w", metaErr)
	}
	if metaErr != nil {
		meta = SessionMeta{}
	}

	current, exists, err := s.loadManifestLocked(sessionID)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	var previous []types.Message
	if exists {
		previous, err = s.loadViewFromManifestLocked(current)
		if err != nil {
			return CompactionManifestV2{}, err
		}
		if _, err := s.loadAuditFromManifestLocked(current); err != nil {
			return CompactionManifestV2{}, err
		}
	} else if _, statErr := s.lstatPrivate(s.sessionPath(sessionID)); statErr == nil {
		previous, err = s.loadLegacyMessagesLocked(sessionID)
		if err != nil {
			return CompactionManifestV2{}, err
		}
		current, err = s.commitModelContextLocked(sessionID, 0, previous, previous)
		if err != nil {
			return current, err
		}
		exists = true
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return CompactionManifestV2{}, statErr
	}
	expectedGeneration := uint64(0)
	if exists {
		expectedGeneration = current.ContextGeneration
	}
	if expected != nil && *expected != expectedGeneration {
		return CompactionManifestV2{}, fmt.Errorf("%w: expected %d, current %d", ErrStaleContextGeneration, *expected, expectedGeneration)
	}
	committed, err := s.commitModelContextLocked(sessionID, expectedGeneration, messages, inferAuditDelta(previous, messages))
	if err != nil {
		return committed, err
	}

	// Keep the legacy discovery transcript as a compatibility snapshot. It is
	// no longer authoritative and is never rewritten after creation.
	if info, err := s.tightenPrivateRegularFile(s.sessionPath(sessionID), true); err != nil {
		return committed, &ContextCommitError{Manifest: committed, Cause: err}
	} else if info == nil {
		payload, encodeErr := encodeMessagesJSONL(messages)
		if encodeErr != nil {
			return committed, &ContextCommitError{Manifest: committed, Cause: encodeErr}
		}
		if writeErr := s.writePrivateFileAtomic(s.dir, s.sessionPath(sessionID), ".session-transcript-*", payload); writeErr != nil {
			return committed, &ContextCommitError{Manifest: committed, Cause: fmt.Errorf("write session compatibility file: %w", writeErr)}
		}
	}

	meta = s.mergeDerivedMeta(sessionID, meta, messages)
	if err := s.saveMetaLocked(sessionID, meta); err != nil {
		return committed, &ContextCommitError{Manifest: committed, Cause: err}
	}

	return committed, nil
}

func (s *FileStore) Load(sessionID string) ([]types.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureNotDeletedLocked(sessionID); err != nil {
		return nil, err
	}
	unlock, err := s.lockSessionHistory(sessionID, true)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return s.loadMessagesLocked(sessionID)
}

func (s *FileStore) loadMessagesLocked(sessionID string) ([]types.Message, error) {
	// A published v2 manifest is authoritative. The legacy discovery JSONL is
	// only a compatibility snapshot and may legitimately be absent after a
	// crash immediately following the first manifest CAS.
	_, manifestExists, err := s.loadManifestLocked(sessionID)
	if err != nil {
		return nil, err
	}
	// Tighten an existing compatibility snapshot even when v2 is authoritative.
	// It can still contain the complete conversation and must not remain world
	// readable merely because recovery no longer depends on it. A first v2
	// commit may legitimately have no compatibility file yet.
	if _, err := s.tightenPrivateRegularFile(s.sessionPath(sessionID), manifestExists); err != nil {
		return nil, err
	}
	messages, _, err := s.loadModelContextLocked(sessionID)
	return messages, err
}

func (s *FileStore) loadLegacyMessagesLocked(sessionID string) ([]types.Message, error) {
	if err := s.ensureReadyLocked(sessionID); err != nil {
		return nil, err
	}
	data, err := s.readPrivateRegularFile(s.sessionPath(sessionID))
	if err != nil {
		return nil, fmt.Errorf("open session: %w", err)
	}
	messages, err := decodeMessagesJSONL(data)
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// SaveMeta updates the persisted metadata for a session while preserving
// derived fields computed from the message history.
func (s *FileStore) SaveMeta(sessionID string, meta SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureNotDeletedLocked(sessionID); err != nil {
		return err
	}
	unlock, err := s.lockSessionHistory(sessionID, true)
	if err != nil {
		return err
	}
	defer unlock()

	messages, err := s.loadMessagesLocked(sessionID)
	if err != nil {
		return err
	}
	current, currentErr := s.loadMetaLocked(sessionID)
	if currentErr != nil && !errors.Is(currentErr, fs.ErrNotExist) {
		return fmt.Errorf("load existing session metadata before update: %w", currentErr)
	}
	merged := s.mergeDerivedMeta(sessionID, current, messages)

	if lineageID := strings.TrimSpace(meta.CacheLineageID); lineageID != "" {
		merged.CacheLineageID = lineageID
	}
	if meta.Title != "" {
		merged.Title = meta.Title
	}
	if !meta.CreatedAt.IsZero() {
		merged.CreatedAt = meta.CreatedAt
	}
	if meta.CWD != "" {
		merged.CWD = meta.CWD
	}
	if meta.GitBranch != "" {
		merged.GitBranch = meta.GitBranch
	}
	if meta.PreviewText != "" {
		merged.PreviewText = meta.PreviewText
	}
	if !meta.UpdatedAt.IsZero() {
		merged.UpdatedAt = meta.UpdatedAt
	}
	if meta.MessageCount > 0 {
		merged.MessageCount = meta.MessageCount
	}
	if meta.Provider != "" {
		merged.Provider = meta.Provider
	}
	if meta.Model != "" {
		merged.Model = meta.Model
	}
	if meta.Goal != nil {
		goalState := *meta.Goal
		merged.Goal = &goalState
	}
	if meta.Usage != nil {
		usage := *meta.Usage
		merged.Usage = &usage
	}
	if meta.Presentation != nil {
		presentation := *meta.Presentation
		if meta.Presentation.DisclosureReturns != nil {
			presentation.DisclosureReturns = make(map[string]SessionDisclosureReturnMeta, len(meta.Presentation.DisclosureReturns))
			for id, restore := range meta.Presentation.DisclosureReturns {
				presentation.DisclosureReturns[id] = restore
			}
		}
		merged.Presentation = &presentation
	}
	if meta.Skills != nil {
		skillsMeta, err := normalizeSessionSkillsMeta(meta.Skills)
		if err != nil {
			return fmt.Errorf("invalid session skills metadata: %w", err)
		}
		merged.Skills = skillsMeta
	}
	if meta.Decisions != nil {
		merged.Decisions = append([]SessionDecisionMeta(nil), meta.Decisions...)
		for i := range merged.Decisions {
			merged.Decisions[i].Choices = append([]string(nil), merged.Decisions[i].Choices...)
		}
	}
	if meta.Evidence != nil {
		merged.Evidence = cloneSessionEvidenceMeta(meta.Evidence)
	}
	if meta.Activities != nil {
		merged.Activities = cloneSessionActivityMeta(meta.Activities)
	}
	if meta.SeenToolUseIDs != nil {
		merged.SeenToolUseIDs = normalizeSeenToolUseIDs(meta.SeenToolUseIDs)
	}
	if meta.LoadedToolNames != nil {
		merged.LoadedToolNames = normalizeLoadedToolNames(meta.LoadedToolNames)
	}

	return s.saveMetaLocked(sessionID, merged)
}

func cloneSessionActivityMeta(activities []SessionActivityMeta) []SessionActivityMeta {
	if activities == nil {
		return nil
	}
	out := make([]SessionActivityMeta, len(activities))
	for index, activity := range activities {
		out[index] = activity
		out[index].DetailRefs = append([]SessionDetailRefMeta(nil), activity.DetailRefs...)
	}
	return out
}

// UpdateGoal atomically loads, transforms, and persists a session goal while
// preserving every unrelated metadata field. The callback runs while the same
// lock used by SaveMeta is held, so a concurrent partial metadata write cannot
// be lost between the goal read and write.
func (s *FileStore) UpdateGoal(sessionID string, update goal.UpdateFunc) (goal.Goal, error) {
	if update == nil {
		return goal.Goal{}, fmt.Errorf("goal update callback is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureNotDeletedLocked(sessionID); err != nil {
		return goal.Goal{}, err
	}
	unlock, err := s.lockSessionHistory(sessionID, true)
	if err != nil {
		return goal.Goal{}, err
	}
	defer unlock()

	messages, err := s.loadMessagesLocked(sessionID)
	if err != nil {
		return goal.Goal{}, err
	}
	currentMeta, currentErr := s.loadMetaLocked(sessionID)
	if currentErr != nil && !errors.Is(currentErr, fs.ErrNotExist) {
		return goal.Goal{}, fmt.Errorf("load existing session metadata before goal update: %w", currentErr)
	}
	merged := s.mergeDerivedMeta(sessionID, currentMeta, messages)

	current := cloneSessionGoal(merged.Goal)
	next, err := update(current)
	if err != nil {
		return goal.Goal{}, err
	}
	merged.Goal = cloneSessionGoal(&next)
	if err := s.saveMetaLocked(sessionID, merged); err != nil {
		return goal.Goal{}, err
	}
	return *cloneSessionGoal(&next), nil
}

func cloneSessionGoal(current *goal.Goal) *goal.Goal {
	if current == nil {
		return nil
	}
	cloned := *current
	if current.AchievedAt != nil {
		value := *current.AchievedAt
		cloned.AchievedAt = &value
	}
	if current.BlockedAt != nil {
		value := *current.BlockedAt
		cloned.BlockedAt = &value
	}
	return &cloned
}

func cloneSessionEvidenceMeta(evidence []SessionEvidenceMeta) []SessionEvidenceMeta {
	cloned := append([]SessionEvidenceMeta(nil), evidence...)
	for i := range cloned {
		cloned[i].Results = append([]SessionDetailRefMeta(nil), cloned[i].Results...)
		cloned[i].Envelopes = append([]SessionDetailRefMeta(nil), cloned[i].Envelopes...)
	}
	return cloned
}

// GetMeta returns the metadata sidecar for a session, deriving a fallback
// view from the message file if no sidecar exists yet.
func (s *FileStore) GetMeta(sessionID string) (SessionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureNotDeletedLocked(sessionID); err != nil {
		return SessionMeta{}, err
	}
	unlock, err := s.lockSessionHistory(sessionID, true)
	if err != nil {
		return SessionMeta{}, err
	}
	defer unlock()

	meta, err := s.loadMetaLocked(sessionID)
	if err == nil {
		return meta, nil
	}
	if !os.IsNotExist(err) {
		return SessionMeta{}, err
	}

	messages, loadErr := s.loadMessagesLocked(sessionID)
	if loadErr != nil {
		return SessionMeta{}, loadErr
	}
	derived := s.mergeDerivedMeta(sessionID, SessionMeta{}, messages)
	if stat, statErr := s.tightenPrivateRegularFile(s.sessionPath(sessionID), false); statErr == nil {
		if derived.CreatedAt.IsZero() {
			derived.CreatedAt = stat.ModTime()
		}
		if derived.UpdatedAt.IsZero() {
			derived.UpdatedAt = stat.ModTime()
		}
	}
	return derived, nil
}

// Rename updates the user-visible title for a session.
func (s *FileStore) Rename(sessionID, title string) error {
	return s.SaveMeta(sessionID, SessionMeta{Title: strings.TrimSpace(title)})
}

// Search returns sessions filtered by query and project scope.
func (s *FileStore) Search(opts SearchOptions) ([]SessionInfo, error) {
	sessions, err := s.List()
	if err != nil {
		return nil, err
	}

	query := strings.ToLower(strings.TrimSpace(opts.Query))
	current := cleanPath(opts.CurrentCWD)
	currentProjectDir := cleanPath(opts.CurrentProjectDir)
	out := make([]SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		if !opts.AllProjects && currentProjectDir != "" {
			if cleanPath(sess.ProjectDir) != currentProjectDir {
				continue
			}
		} else if !opts.AllProjects && current != "" {
			sessCWD := cleanPath(sess.CWD)
			if sessCWD != "" && sessCWD != current {
				continue
			}
		}
		if query != "" && !matchesSessionQuery(sess, query) {
			continue
		}
		out = append(out, sess)
	}
	return out, nil
}

func matchesSessionQuery(sess SessionInfo, query string) bool {
	candidates := []string{sess.ID, sess.Title, sess.PreviewText, sess.GitBranch, sess.CWD}
	for _, c := range candidates {
		if strings.Contains(strings.ToLower(c), query) {
			return true
		}
	}
	return false
}

func cleanPath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

func (s *FileStore) List() ([]SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(""); err != nil {
		return nil, err
	}

	entries, err := s.readPrivateDirectory(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		if validateStorageID(id) != nil {
			continue
		}
		info, err := s.tightenPrivateRegularFile(s.sessionPath(id), false)
		if err != nil {
			continue
		}
		deleted, deletedErr := s.isDeletedLocked(id)
		if deletedErr != nil {
			return nil, deletedErr
		}
		if deleted {
			continue
		}
		meta, metaErr := s.loadMetaLocked(id)
		if metaErr != nil && !os.IsNotExist(metaErr) {
			return nil, metaErr
		}
		if metaErr != nil {
			messages, loadErr := s.loadMessagesLocked(id)
			if loadErr != nil {
				continue
			}
			meta = s.mergeDerivedMeta(id, SessionMeta{}, messages)
		}
		if meta.CreatedAt.IsZero() {
			meta.CreatedAt = info.ModTime()
		}
		if meta.UpdatedAt.IsZero() {
			meta.UpdatedAt = info.ModTime()
		}
		sessions = append(sessions, SessionInfo{
			ID:           id,
			ProjectDir:   s.dir,
			Title:        meta.Title,
			CreatedAt:    meta.CreatedAt,
			UpdatedAt:    meta.UpdatedAt,
			Turns:        meta.MessageCount / 2,
			MessageCount: meta.MessageCount,
			CWD:          meta.CWD,
			GitBranch:    meta.GitBranch,
			PreviewText:  meta.PreviewText,
			Provider:     meta.Provider,
			Model:        meta.Model,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// IsDeleted reports whether delete-history has committed for sessionID.
func (s *FileStore) IsDeleted(sessionID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isDeletedLocked(sessionID)
}

func (s *FileStore) isDeletedLocked(sessionID string) (bool, error) {
	if err := s.ensureReadyLocked(sessionID); err != nil {
		return false, err
	}
	_, err := s.tightenPrivateRegularFile(s.tombstonePath(sessionID), false)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat session deletion marker: %w", err)
}

func (s *FileStore) ensureNotDeletedLocked(sessionID string) error {
	deleted, err := s.isDeletedLocked(sessionID)
	if err != nil {
		return err
	}
	if deleted {
		return fmt.Errorf("%w: %s", ErrSessionDeleted, sessionID)
	}
	return nil
}

// Delete atomically commits logical deletion by publishing a durable marker,
// then best-effort removes every physical history component. Cleanup failures
// are joined and returned, while the marker remains so a retry can finish the
// cleanup without allowing a late writer to resurrect the session.
func (s *FileStore) Delete(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(sessionID); err != nil {
		return err
	}
	unlock, err := s.lockSessionHistory(sessionID, true)
	if err != nil {
		return err
	}
	defer unlock()

	deleted, err := s.isDeletedLocked(sessionID)
	if err != nil {
		return err
	}
	if !deleted {
		if _, err := s.tightenPrivateRegularFile(s.sessionPath(sessionID), false); err != nil {
			return err
		}
		if err := s.writeDeleteMarker(sessionID); err != nil {
			return fmt.Errorf("commit session deletion marker: %w", err)
		}
	}

	var cleanupErrs []error
	if err := s.removeFile(s.metaPath(sessionID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("remove session metadata: %w", err))
	}
	if err := s.removeFile(s.manifestPath(sessionID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("remove session context manifest: %w", err))
	}
	if err := s.removeTree(s.ArtifactsDir(sessionID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("remove session artifacts: %w", err))
	}
	if err := s.removeFile(s.sessionPath(sessionID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("remove session transcript: %w", err))
	}
	return errors.Join(cleanupErrs...)
}

func (s *FileStore) writeDeleteMarkerAtomic(sessionID string) error {
	return s.writePrivateFileAtomic(s.dir, s.tombstonePath(sessionID), ".session-delete-*", []byte("{\"version\":1}\n"))
}

func (s *FileStore) Latest() (string, error) {
	sessions, err := s.List()
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", ErrNoSessions
	}
	return sessions[0].ID, nil
}

func (s *FileStore) loadMetaLocked(sessionID string) (SessionMeta, error) {
	if err := s.ensureReadyLocked(sessionID); err != nil {
		return SessionMeta{}, err
	}
	data, err := s.readPrivateRegularFile(s.metaPath(sessionID))
	if err != nil {
		return SessionMeta{}, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return SessionMeta{}, fmt.Errorf("parse session metadata: %w", err)
	}
	if meta.ID == "" {
		meta.ID = sessionID
	}
	if manifest, exists, manifestErr := s.loadManifestLocked(sessionID); manifestErr != nil {
		return SessionMeta{}, manifestErr
	} else if exists {
		meta.MessageCount = int(manifest.ModelContextView.MessageCount)
		meta.PreviewText = manifest.ModelContextPreview
		if meta.UpdatedAt.Before(manifest.CommittedAt) {
			meta.UpdatedAt = manifest.CommittedAt
		}
	}
	meta.CacheLineageID = normalizeCacheLineageID(sessionID, meta.CacheLineageID)
	if meta.SeenToolUseIDs != nil {
		meta.SeenToolUseIDs = normalizeSeenToolUseIDs(meta.SeenToolUseIDs)
	}
	if meta.LoadedToolNames != nil {
		meta.LoadedToolNames = normalizeLoadedToolNames(meta.LoadedToolNames)
	}
	if meta.Skills != nil {
		skillsMeta, normalizeErr := normalizeSessionSkillsMeta(meta.Skills)
		if normalizeErr != nil {
			return SessionMeta{}, fmt.Errorf("parse session skills metadata: %w", normalizeErr)
		}
		meta.Skills = skillsMeta
	}
	return meta, nil
}

func (s *FileStore) saveMetaLocked(sessionID string, meta SessionMeta) error {
	if s.metaWriteFault != nil {
		if err := s.metaWriteFault(); err != nil {
			return err
		}
	}
	if meta.ID == "" {
		meta.ID = sessionID
	}
	meta.CacheLineageID = normalizeCacheLineageID(sessionID, meta.CacheLineageID)
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = time.Now()
	}
	if s.writerFingerprint != nil {
		writer := cloneBuildFingerprint(s.writerFingerprint())
		if meta.FirstWriterBuild == nil {
			first := cloneBuildFingerprint(writer)
			meta.FirstWriterBuild = &first
		}
		last := cloneBuildFingerprint(writer)
		meta.LastWriterBuild = &last
	}
	if meta.SeenToolUseIDs != nil {
		meta.SeenToolUseIDs = normalizeSeenToolUseIDs(meta.SeenToolUseIDs)
	}
	if meta.LoadedToolNames != nil {
		meta.LoadedToolNames = normalizeLoadedToolNames(meta.LoadedToolNames)
	}
	if meta.Skills != nil {
		skillsMeta, normalizeErr := normalizeSessionSkillsMeta(meta.Skills)
		if normalizeErr != nil {
			return fmt.Errorf("normalize session skills metadata: %w", normalizeErr)
		}
		meta.Skills = skillsMeta
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session metadata: %w", err)
	}
	payload := append(data, '\n')
	if err := s.writePrivateFileAtomic(s.dir, s.metaPath(sessionID), ".session-meta-*", payload); err != nil {
		return fmt.Errorf("write session metadata: %w", err)
	}
	return nil
}

func cloneBuildFingerprint(source buildinfo.Fingerprint) buildinfo.Fingerprint {
	cloned := source
	if source.Dirty != nil {
		value := *source.Dirty
		cloned.Dirty = &value
	}
	if source.BuildTime != nil {
		value := *source.BuildTime
		cloned.BuildTime = &value
	}
	return cloned
}

func normalizeSeenToolUseIDs(ids []string) []string {
	return normalizeStableStrings(ids)
}

func normalizeLoadedToolNames(names []string) []string {
	return normalizeStableStrings(names)
}

func normalizeStableStrings(values []string) []string {
	if values == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (s *FileStore) mergeDerivedMeta(sessionID string, meta SessionMeta, messages []types.Message) SessionMeta {
	if meta.ID == "" {
		meta.ID = sessionID
	}
	meta.CacheLineageID = normalizeCacheLineageID(sessionID, meta.CacheLineageID)
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}
	meta.UpdatedAt = time.Now()
	meta.MessageCount = len(messages)
	if preview := derivePreviewText(messages); preview != "" {
		meta.PreviewText = preview
	}
	return meta
}

func normalizeCacheLineageID(sessionID, lineageID string) string {
	if lineageID = strings.TrimSpace(lineageID); lineageID != "" {
		return lineageID
	}
	return strings.TrimSpace(sessionID)
}

func derivePreviewText(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].IsInternalRuntimeMessage() {
			continue
		}
		if text := strings.TrimSpace(messages[i].GetText()); text != "" {
			text = strings.Join(strings.Fields(text), " ")
			if len([]rune(text)) > 120 {
				return string([]rune(text)[:120]) + "…"
			}
			return text
		}
	}
	return ""
}

// MemoryStore extracts and persists key facts from conversations.
type MemoryStore struct {
	mu                   sync.Mutex
	path                 string
	memories             []Memory
	storageErr           error
	storageBeforePublish func()
}

// MaxPromptMemories is the maximum number of memories injected into system prompt.
const MaxPromptMemories = 50

// Memory represents a single extracted memory.
type Memory struct {
	Fact      string    `json:"fact"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	Category  string    `json:"category"`
}

// NewMemoryStore creates or loads a memory store.
func NewMemoryStore(path string) *MemoryStore {
	if path == "" {
		path = brand.MemoryPath()
	}
	normalized, err := normalizeMemoryStorePath(path)
	ms := &MemoryStore{path: normalized, storageErr: err}
	ms.load()
	return ms
}

// Memories returns a copy of all stored memories.
func (ms *MemoryStore) Memories() []Memory {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	result := make([]Memory, len(ms.memories))
	copy(result, ms.memories)
	return result
}

func (ms *MemoryStore) load() {
	if ms.storageErr != nil {
		return
	}
	dirPath := filepath.Dir(ms.path)
	dir, err := openPrivateDirectory(dirPath)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		ms.storageErr = err
		return
	}
	defer dir.Close()
	root, err := openMemoryStoreRoot(dirPath, dir)
	if err != nil {
		ms.storageErr = err
		return
	}
	defer root.Close()

	data, err := readMemoryStoreFile(root, ms.path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := validateMemoryStoreDirectory(dirPath, dir); err != nil {
			ms.storageErr = err
		}
		return
	}
	if err != nil {
		ms.storageErr = err
		return
	}
	var wrapper struct {
		Memories []Memory `json:"memories"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		ms.storageErr = fs.ErrInvalid
		return
	}
	if err := validateMemoryStoreDirectory(dirPath, dir); err != nil {
		ms.storageErr = err
		return
	}
	ms.memories = wrapper.Memories
}

// Add stores a new memory.
func (ms *MemoryStore) Add(fact, source, category string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.storageErr != nil {
		return ms.storageErr
	}
	ms.memories = append(ms.memories, Memory{
		Fact:      fact,
		Source:    source,
		CreatedAt: time.Now(),
		Category:  category,
	})
	return ms.saveLocked()
}

// saveLocked writes memories to disk atomically. Caller must hold mu.
func (ms *MemoryStore) saveLocked() error {
	dir := filepath.Dir(ms.path)
	if err := ensurePrivateDirectory(dir); err != nil {
		return err
	}
	heldDir, err := openPrivateDirectory(dir)
	if err != nil {
		return err
	}
	defer heldDir.Close()
	root, err := openMemoryStoreRoot(dir, heldDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if ms.storageBeforePublish != nil {
		ms.storageBeforePublish()
	}

	wrapper := struct {
		Memories []Memory `json:"memories"`
	}{Memories: ms.memories}

	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fs.ErrInvalid
	}

	if err := writeMemoryStoreFileAtomic(root, heldDir, ms.path, data); err != nil {
		return err
	}
	published, err := readMemoryStoreFile(root, ms.path)
	if err != nil {
		return err
	}
	if !bytes.Equal(published, data) {
		return fs.ErrInvalid
	}
	if err := validateMemoryStoreDirectory(dir, heldDir); err != nil {
		return err
	}

	return nil
}

func normalizeMemoryStorePath(path string) (string, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 || hasMemoryPathTraversal(path) {
		return "", fs.ErrInvalid
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	base := filepath.Base(abs)
	dir := filepath.Dir(abs)
	if base == "." || base == string(os.PathSeparator) || dir == abs || isVolumeRoot(dir) {
		return "", fs.ErrInvalid
	}
	return abs, nil
}

func hasMemoryPathTraversal(path string) bool {
	for _, component := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if component == ".." {
			return true
		}
	}
	return false
}

func validateMemoryStoreDirectory(path string, held *os.File) error {
	heldInfo, err := held.Stat()
	if err != nil {
		return err
	}
	current, err := openPrivateDirectory(path)
	if err != nil {
		return err
	}
	defer current.Close()
	currentInfo, err := current.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(heldInfo, currentInfo) {
		return &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return nil
}

// ForPrompt returns memories formatted for system prompt injection.
func (ms *MemoryStore) ForPrompt() string {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if len(ms.memories) == 0 {
		return ""
	}

	memories := ms.memories
	if len(memories) > MaxPromptMemories {
		memories = memories[len(memories)-MaxPromptMemories:]
	}

	var sb strings.Builder
	sb.WriteString("# Remembered Context\n\n")
	for _, m := range memories {
		fmt.Fprintf(&sb, "- [%s] %s\n", m.Category, m.Fact)
	}
	return sb.String()
}
