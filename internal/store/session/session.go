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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/types"
)

// SessionInfo holds metadata about a stored session.
type SessionInfo struct {
	ID           string    `json:"id"`
	ProjectDir   string    `json:"project_dir,omitempty"`
	Title        string    `json:"title,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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

// FileStore persists content-addressed session history and metadata sidecars.
type FileStore struct {
	dir                  string
	mu                   sync.Mutex // protects concurrent Save/Load operations
	rootMu               sync.Mutex
	root                 *secureio.Root
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

// ErrCorruptSessionMetadata identifies a sidecar whose declared schema cannot
// be trusted. Listing isolates the affected session, while precise lookup
// keeps the error visible to the caller.
var ErrCorruptSessionMetadata = errors.New("corrupt session metadata")

// ErrIncompatibleSessionMetadata identifies a well-formed sidecar whose
// schema or fields are not understood by this build.
var ErrIncompatibleSessionMetadata = errors.New("incompatible session metadata")

// SessionMeta is the persisted sidecar metadata for a session.
type SessionMeta struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	// CacheLineageID is the stable prompt-cache routing identity. New sessions
	// use their own ID; forked sessions inherit the source lineage.
	CacheLineageID string                   `json:"cache_lineage_id"`
	Title          string                   `json:"title,omitempty"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
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
	FirstWriterBuild *buildinfo.Fingerprint `json:"first_writer_build"`
	LastWriterBuild  *buildinfo.Fingerprint `json:"last_writer_build"`
}

type SessionUsageMeta struct {
	InputTokens                int     `json:"input_tokens,omitempty"`
	OutputTokens               int     `json:"output_tokens,omitempty"`
	CacheReadTokens            int     `json:"cache_read_tokens,omitempty"`
	CacheCreateTokens          int     `json:"cache_create_tokens,omitempty"`
	HasCompacted               bool    `json:"has_compacted,omitempty"`
	CompactionBaselineKnown    bool    `json:"compaction_baseline_known,omitempty"`
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
	PermissionMode string `json:"permission_mode,omitempty"`
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

const sessionMetaSchemaV1 = "session-meta/v1"

var legacySessionMetaFields = map[string]struct{}{
	"id": {}, "cache_lineage_id": {}, "title": {}, "created_at": {},
	"updated_at": {}, "message_count": {}, "cwd": {}, "git_branch": {},
	"preview_text": {}, "provider": {}, "model": {}, "goal": {}, "usage": {},
	"presentation": {}, "skills": {}, "decisions": {}, "seen_tool_use_ids": {},
	"loaded_tool_names": {}, "first_writer_build": {}, "last_writer_build": {},
	// These projections were persisted by pre-v1 TUI builds. They are derived
	// from durable artifacts now and are intentionally discarded on migration.
	"activities": {}, "evidence": {},
}

var legacySessionPresentationFields = map[string]struct{}{
	"permission_mode": {},
	// Pre-v1 presentation-only fields now live in the TUI checkpoint.
	"version": {}, "input_cursor_set": {},
}

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

// NewFileStore creates a file-based session store.
func NewFileStore(dir string) *FileStore {
	root, rootErr := secureio.Open(dir, true)
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

func (s *FileStore) storageRoot() (*secureio.Root, error) {
	s.rootMu.Lock()
	defer s.rootMu.Unlock()
	if s.root == nil {
		s.root, s.rootErr = secureio.Open(s.dir, true)
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
	for _, suffix := range []string{".context-v2.json", ".meta.json", ".deleted"} {
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
	manifest, exists, err := s.loadManifestLocked(sessionID)
	if err != nil || !exists {
		return ""
	}
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
	f, openErr := s.openPrivateRegularFile(path)
	if openErr != nil {
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

// SaveModelContextCAS persists session metadata and atomically publishes a
// model view only if expectedGeneration is still current.
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
	}
	expectedGeneration := uint64(0)
	if exists {
		expectedGeneration = current.ContextGeneration
	}
	if expected != nil && *expected != expectedGeneration {
		return CompactionManifestV2{}, fmt.Errorf("%w: expected %d, current %d", ErrStaleContextGeneration, *expected, expectedGeneration)
	}
	auditDelta, err := inferAuditDelta(previous, messages)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	committed, err := s.commitModelContextLocked(sessionID, expectedGeneration, messages, auditDelta)
	if err != nil {
		return committed, err
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
	messages, _, err := s.loadModelContextLocked(sessionID)
	return messages, err
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
	if meta.SeenToolUseIDs != nil {
		merged.SeenToolUseIDs = normalizeSeenToolUseIDs(meta.SeenToolUseIDs)
	}
	if meta.LoadedToolNames != nil {
		merged.LoadedToolNames = normalizeLoadedToolNames(meta.LoadedToolNames)
	}

	return s.saveMetaLocked(sessionID, merged)
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

// GetMeta returns the metadata sidecar for a session, deriving it from the
// current model context when no sidecar exists yet.
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
	if stat, statErr := s.tightenPrivateRegularFile(s.manifestPath(sessionID), false); statErr == nil {
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
	return s.list(false)
}

func (s *FileStore) list(strictMetadata bool) ([]SessionInfo, error) {
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
		if !strings.HasSuffix(entry.Name(), ".context-v2.json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".context-v2.json")
		if validateStorageID(id) != nil {
			continue
		}
		info, err := s.tightenPrivateRegularFile(s.manifestPath(id), false)
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
			if !strictMetadata && (errors.Is(metaErr, ErrCorruptSessionMetadata) || errors.Is(metaErr, ErrIncompatibleSessionMetadata)) {
				continue
			}
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
		if _, err := s.tightenPrivateRegularFile(s.manifestPath(sessionID), false); err != nil {
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
	return errors.Join(cleanupErrs...)
}

func (s *FileStore) writeDeleteMarkerAtomic(sessionID string) error {
	return s.writePrivateFileAtomic(s.dir, s.tombstonePath(sessionID), ".session-delete-*", []byte("{\"version\":1}\n"))
}

func (s *FileStore) Latest() (string, error) {
	sessions, err := s.list(true)
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
	meta, legacy, err := decodeSessionMetadata(data)
	if err != nil {
		return SessionMeta{}, err
	}
	if meta.ID != sessionID {
		return SessionMeta{}, fmt.Errorf("%w: session identity mismatch", ErrCorruptSessionMetadata)
	}
	if strings.TrimSpace(meta.CacheLineageID) == "" {
		return SessionMeta{}, fmt.Errorf("%w: cache lineage is missing", ErrCorruptSessionMetadata)
	}
	if meta.CreatedAt.IsZero() || meta.UpdatedAt.IsZero() {
		return SessionMeta{}, fmt.Errorf("%w: required timestamps are missing", ErrCorruptSessionMetadata)
	}
	if !legacy && (meta.FirstWriterBuild == nil || meta.LastWriterBuild == nil) {
		return SessionMeta{}, fmt.Errorf("%w: writer fingerprints are missing", ErrCorruptSessionMetadata)
	}
	if meta.FirstWriterBuild != nil && meta.FirstWriterBuild.ProcessStart.IsZero() {
		return SessionMeta{}, fmt.Errorf("%w: first writer fingerprint is incomplete", ErrCorruptSessionMetadata)
	}
	if meta.LastWriterBuild != nil && meta.LastWriterBuild.ProcessStart.IsZero() {
		return SessionMeta{}, fmt.Errorf("%w: last writer fingerprint is incomplete", ErrCorruptSessionMetadata)
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

func decodeSessionMetadata(data []byte) (SessionMeta, bool, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&fields); err != nil {
		return SessionMeta{}, false, fmt.Errorf("%w: %v", ErrCorruptSessionMetadata, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return SessionMeta{}, false, fmt.Errorf("%w: %v", ErrCorruptSessionMetadata, err)
	}
	if fields == nil {
		return SessionMeta{}, false, ErrCorruptSessionMetadata
	}

	if rawSchema, exists := fields["schema_version"]; exists {
		var schema string
		if err := json.Unmarshal(rawSchema, &schema); err != nil {
			return SessionMeta{}, false, fmt.Errorf("%w: invalid schema version", ErrCorruptSessionMetadata)
		}
		if schema != sessionMetaSchemaV1 {
			return SessionMeta{}, false, fmt.Errorf("%w: %s", ErrIncompatibleSessionMetadata, schema)
		}
		meta, err := decodeStrictSessionMetadata(data)
		return meta, false, err
	}

	normalized, err := normalizeLegacySessionMetadata(fields)
	if err != nil {
		return SessionMeta{}, true, err
	}
	meta, err := decodeStrictSessionMetadata(normalized)
	return meta, true, err
}

func decodeStrictSessionMetadata(data []byte) (SessionMeta, error) {
	var meta SessionMeta
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&meta); err != nil {
		return SessionMeta{}, fmt.Errorf("%w: %v", ErrCorruptSessionMetadata, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return SessionMeta{}, fmt.Errorf("%w: %v", ErrCorruptSessionMetadata, err)
	}
	return meta, nil
}

func normalizeLegacySessionMetadata(fields map[string]json.RawMessage) ([]byte, error) {
	for name := range fields {
		if _, allowed := legacySessionMetaFields[name]; !allowed {
			return nil, fmt.Errorf("%w: unsupported legacy field %q", ErrIncompatibleSessionMetadata, name)
		}
	}
	delete(fields, "activities")
	delete(fields, "evidence")

	if rawPresentation, exists := fields["presentation"]; exists {
		var presentation map[string]json.RawMessage
		if err := json.Unmarshal(rawPresentation, &presentation); err != nil {
			return nil, fmt.Errorf("%w: invalid legacy presentation", ErrCorruptSessionMetadata)
		}
		for name := range presentation {
			if _, allowed := legacySessionPresentationFields[name]; !allowed {
				return nil, fmt.Errorf("%w: unsupported legacy presentation field %q", ErrIncompatibleSessionMetadata, name)
			}
		}
		if _, exists := presentation["permission_mode"]; exists {
			fields["presentation"] = []byte(`{"permission_mode":` + string(presentation["permission_mode"]) + `}`)
		} else {
			delete(fields, "presentation")
		}
	}
	fields["schema_version"] = []byte(`"` + sessionMetaSchemaV1 + `"`)
	normalized, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptSessionMetadata, err)
	}
	return normalized, nil
}

func (s *FileStore) saveMetaLocked(sessionID string, meta SessionMeta) error {
	if s.metaWriteFault != nil {
		if err := s.metaWriteFault(); err != nil {
			return err
		}
	}
	meta.SchemaVersion = sessionMetaSchemaV1
	meta.ID = sessionID
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
	meta.SchemaVersion = sessionMetaSchemaV1
	meta.ID = sessionID
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

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fs.ErrInvalid
		}
		return err
	}
	return nil
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
		if len(messages[i].GetInvalidToolUses()) > 0 {
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
