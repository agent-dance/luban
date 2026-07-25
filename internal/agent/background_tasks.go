package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/types"
	"github.com/google/uuid"
)

// BackgroundOutputCapBytes caps each task output file at 50 MB before rotation.
const BackgroundOutputCapBytes int64 = 50 * 1024 * 1024

// rotatingFileWriter is an io.WriteCloser that rotates the underlying file to
// "<path>.1" once its size exceeds capBytes. Rotation is best-effort: a failure
// to rotate falls back to writing to the existing file so the command keeps
// running.
type rotatingFileWriter struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	written  int64
	capBytes int64
}

func newRotatingFileWriter(path string, capBytes int64) (*rotatingFileWriter, error) {
	if capBytes <= 0 {
		capBytes = BackgroundOutputCapBytes
	}
	if err := validateBackgroundOutputPath(path); err != nil {
		return nil, err
	}
	f, err := secureio.OpenPrivateRuntimeAppendFile(path)
	if err != nil {
		return nil, err
	}
	w := &rotatingFileWriter{
		path:     path,
		file:     f,
		capBytes: capBytes,
	}
	if info, err := f.Stat(); err == nil {
		w.written = info.Size()
	}
	return w, nil
}

// rotateLocked rotates the current file to "<path>.1" and re-opens the live
// file. Caller must hold w.mu.
func (w *rotatingFileWriter) rotateLocked() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	rotated := w.path + ".1"
	if _, err := secureio.TightenPrivateRuntimeRegularFile(rotated, true); err != nil {
		if reopened, openErr := secureio.OpenPrivateRuntimeAppendFile(w.path); openErr == nil {
			w.file = reopened
			if info, statErr := reopened.Stat(); statErr == nil {
				w.written = info.Size()
			}
		}
		return err
	}
	if err := os.Remove(rotated); err != nil && !errors.Is(err, fs.ErrNotExist) {
		w.file, _ = secureio.OpenPrivateRuntimeAppendFile(w.path)
		return err
	}
	if err := os.Rename(w.path, rotated); err != nil {
		// Best-effort: if rename fails, keep writing to current file.
		if f, openErr := secureio.OpenPrivateRuntimeAppendFile(w.path); openErr == nil {
			w.file = f
			if info, statErr := f.Stat(); statErr == nil {
				w.written = info.Size()
			}
		}
		return err
	}
	if err := secureio.SyncRuntimeDirectory(filepath.Dir(w.path)); err != nil {
		w.file, _ = secureio.OpenPrivateRuntimeAppendFile(w.path)
		return err
	}
	f, err := secureio.OpenPrivateRuntimeAppendFile(w.path)
	if err != nil {
		return err
	}
	w.file = f
	w.written = 0
	return nil
}

// Write implements io.Writer; rotates when the cap is exceeded.
func (w *rotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, fmt.Errorf("rotating writer is closed")
	}
	total := 0
	for len(p) > 0 {
		remaining := w.capBytes - w.written
		if remaining <= 0 {
			if err := w.rotateLocked(); err != nil {
				// Rotation is best-effort. Preserve command output even when the
				// filesystem refuses rename; the cap cannot be guaranteed in this
				// exceptional path, but the writer remains usable.
				if w.file == nil {
					return total, err
				}
				n, writeErr := w.file.Write(p)
				w.written += int64(n)
				return total + n, writeErr
			}
			remaining = w.capBytes
		}
		chunk := len(p)
		if int64(chunk) > remaining {
			chunk = int(remaining)
		}
		n, err := w.file.Write(p[:chunk])
		w.written += int64(n)
		total += n
		p = p[n:]
		if err != nil {
			return total, err
		}
		if n != chunk {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}

// WriteString lets callers append a synchronous status line at the end of the
// log without going through io.Writer interface plumbing.
func (w *rotatingFileWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// Close flushes and closes the underlying file.
func (w *rotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	syncErr := w.file.Sync()
	err := w.file.Close()
	w.file = nil
	if syncErr != nil {
		return syncErr
	}
	return err
}

type BackgroundTaskOutput struct {
	Content      string
	WasTruncated bool
}

type BackgroundTask struct {
	mu                      sync.RWMutex
	persistMu               sync.Mutex
	ID                      string
	Type                    string
	Status                  string
	Description             string
	Command                 string
	Prompt                  string
	OutputPath              string
	ExitCode                *int
	Error                   string
	Result                  string
	StartedAt               time.Time
	FinishedAt              *time.Time
	cancel                  context.CancelFunc
	process                 *os.Process
	done                    chan struct{}
	AgentAlias              string
	Detached                bool
	AgentInput              *agentcontract.Input
	AgentMetadata           *agentcontract.SessionMetadata
	AgentMessages           []types.Message
	OwnerSessionID          string
	OwnerSessionProjectDir  string
	OwnerProjectRoot        string
	OwnerPID                int
	origin                  *backgroundTaskOrigin
	CurrentRunID            string
	Attempt                 int
	BatchID                 string
	ParentRunID             string
	AgentPath               string
	QueuedPrompts           int
	QueueReason             string
	Runs                    []agentcontract.RunRecord
	LatestProgress          *agentcontract.ProgressEvent
	TranscriptPath          string
	DurationMs              *int64
	TotalTokens             *int
	Usage                   *types.Usage
	Outcome                 agentcontract.RunOutcome
	TerminalReason          string
	Timeout                 time.Duration
	ArtifactRefs            []string
	VerificationRefs        []string
	pendingTerminalProgress *agentcontract.ProgressEvent

	// OwnerAgentID tags shell background tasks with the sub-agent that
	// spawned them so the parent can call CleanupTasksForAgent on agent
	// finish and reap any leftover dev servers, watchers, etc. Empty for
	// tasks spawned by the top-level session.
	OwnerAgentID string
}

// backgroundTaskOrigin is captured when a task is created. It is immutable so
// a later session/project switch cannot redirect an in-flight task's output,
// durable record, or lifecycle events into the new project root.
type backgroundTaskOrigin struct {
	projectRoot      string
	outputDir        string
	store            *runtimestore.RuntimeTaskStore
	lifecycle        *runtimestore.RuntimeLifecycle
	notificationSink RuntimeNotificationSink
	hookObserver     hooks.ExecutionObserver
	hookCorrelation  hooks.HookInput
	hookContextSet   bool
}

func (o *backgroundTaskOrigin) taskOutputPath(id string) string {
	if o == nil {
		return ""
	}
	return filepath.Join(o.outputDir, runtimestore.OutputPathComponent(id)+".output")
}

func pinBackgroundTaskOriginHookContext(origin *backgroundTaskOrigin, ctx context.Context) {
	if origin == nil || ctx == nil {
		return
	}
	origin.hookObserver = hooks.ExecutionObserverFromContext(ctx)
	origin.hookCorrelation = hooks.CorrelateInput(ctx, hooks.HookInput{})
	origin.hookContextSet = true
}

func (o *backgroundTaskOrigin) hookContext(fallback context.Context) context.Context {
	if fallback == nil {
		fallback = context.Background()
	}
	if o == nil {
		return fallback
	}
	ctx := fallback
	if o.hookObserver != nil {
		ctx = hooks.WithExecutionObserver(ctx, o.hookObserver)
	}
	if o.hookContextSet {
		ctx = hooks.WithCorrelation(ctx, o.hookCorrelation)
	}
	return ctx
}

func backgroundTaskOwnerSessionID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if execCtx, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		return strings.TrimSpace(execCtx.SessionID)
	}
	return ""
}

func backgroundTaskOwnerSessionProjectDir(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if execCtx, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		return strings.TrimSpace(execCtx.SessionProjectDir)
	}
	return ""
}

func (t *BackgroundTask) recordLocked() runtimestore.RuntimeTaskRecord {
	record := runtimestore.RuntimeTaskRecord{
		ID:                     t.ID,
		Type:                   t.Type,
		Status:                 t.Status,
		Description:            t.Description,
		Command:                t.Command,
		Prompt:                 t.Prompt,
		OutputPath:             t.OutputPath,
		Error:                  durableBackgroundTaskError(t.Error, t.TerminalReason),
		Result:                 t.Result,
		StartedAt:              t.StartedAt,
		FinishedAt:             t.FinishedAt,
		OwnerSessionID:         t.OwnerSessionID,
		OwnerSessionProjectDir: t.OwnerSessionProjectDir,
		OwnerProjectRoot:       t.OwnerProjectRoot,
		OwnerAgentID:           t.OwnerAgentID,
		OwnerPID:               t.OwnerPID,
		Detached:               t.Detached,
		CurrentRunID:           t.CurrentRunID,
		Attempt:                t.Attempt,
		BatchID:                t.BatchID,
		ParentRunID:            t.ParentRunID,
		AgentPath:              t.AgentPath,
		QueuedPrompts:          t.QueuedPrompts,
		QueueReason:            t.QueueReason,
		Runs:                   cloneRuntimeTaskRunRecords(t.Runs),
		LatestProgress:         cloneAgentProgressEvent(t.LatestProgress),
		TranscriptPath:         t.TranscriptPath,
		DurationMs:             cloneInt64Pointer(t.DurationMs),
		TotalTokens:            cloneIntPointer(t.TotalTokens),
		Usage:                  cloneUsagePointer(t.Usage),
		Outcome:                t.Outcome,
		TerminalReason:         t.TerminalReason,
		TimeoutNanos:           int64(t.Timeout),
		ArtifactRefs:           append([]string(nil), t.ArtifactRefs...),
		VerificationRefs:       append([]string(nil), t.VerificationRefs...),
	}
	if strings.TrimSpace(t.AgentAlias) != "" {
		record.AgentAlias = t.AgentAlias
	}
	if t.AgentInput != nil {
		input := *t.AgentInput
		record.AgentInput = &input
	}
	if t.AgentMetadata != nil {
		metadata := cloneAgentSessionMetadata(*t.AgentMetadata)
		record.AgentMetadata = &metadata
	}
	if len(t.AgentMessages) > 0 {
		record.AgentMessages = append([]types.Message(nil), t.AgentMessages...)
	}
	if t.ExitCode != nil {
		code := *t.ExitCode
		record.ExitCode = &code
	}
	return record
}

func (t *BackgroundTask) snapshot() agentcontract.TaskSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	snap := agentcontract.TaskSnapshot{
		ID:                     t.ID,
		Type:                   t.Type,
		Status:                 t.Status,
		Description:            t.Description,
		Command:                t.Command,
		Prompt:                 t.Prompt,
		OutputPath:             t.OutputPath,
		Error:                  localizedBackgroundTaskError(t.Error, t.TerminalReason, t.Timeout),
		Result:                 t.Result,
		OwnerSessionID:         t.OwnerSessionID,
		OwnerSessionProjectDir: t.OwnerSessionProjectDir,
		OwnerProjectRoot:       t.OwnerProjectRoot,
		OwnerAgentID:           t.OwnerAgentID,
		OwnerPID:               t.OwnerPID,
		AgentAlias:             t.AgentAlias,
		Detached:               t.Detached,
		CurrentRunID:           t.CurrentRunID,
		Attempt:                t.Attempt,
		BatchID:                t.BatchID,
		ParentRunID:            t.ParentRunID,
		AgentPath:              t.AgentPath,
		QueuedPrompts:          t.QueuedPrompts,
		QueueReason:            t.QueueReason,
		Runs:                   cloneRuntimeTaskRunRecords(t.Runs),
		LatestProgress:         cloneAgentProgressEvent(t.LatestProgress),
		TranscriptPath:         t.TranscriptPath,
		DurationMs:             cloneInt64Pointer(t.DurationMs),
		TotalTokens:            cloneIntPointer(t.TotalTokens),
		Usage:                  cloneUsagePointer(t.Usage),
		Outcome:                t.Outcome,
		TerminalReason:         t.TerminalReason,
		Timeout:                t.Timeout,
		ArtifactRefs:           append([]string(nil), t.ArtifactRefs...),
		VerificationRefs:       append([]string(nil), t.VerificationRefs...),
	}
	if t.ExitCode != nil {
		code := *t.ExitCode
		snap.ExitCode = &code
	}
	return snap
}

type BackgroundTaskManager struct {
	mu                     sync.Mutex
	scheduledMu            sync.Mutex
	snapshotMu             sync.Mutex
	reconcileMu            sync.Mutex
	notificationMu         sync.Mutex
	asyncWG                sync.WaitGroup
	followUpWG             sync.WaitGroup
	shuttingDown           bool
	shutdownDone           chan struct{}
	lifecycleCtx           context.Context
	lifecycleCancel        context.CancelFunc
	tasks                  map[string]*BackgroundTask
	sessions               map[string]*backgroundAgentSession
	aliases                map[string]string
	nextID                 int
	outputDir              string
	store                  *runtimestore.RuntimeTaskStore
	lifecycle              *runtimestore.RuntimeLifecycle
	notificationSink       RuntimeNotificationSink
	notificationObserver   RuntimeNotificationSink
	notificationFollowUp   RuntimeNotificationSink
	followUpQueues         map[string]*runtimeNotificationFollowUpQueue
	followUpQueued         map[string]string
	agentSessionRestorer   func(agentID string, record runtimestore.RuntimeTaskRecord) error
	hookRunner             *hooks.Runner
	trustedAgentResumes    map[string]trustedAgentResumeContext
	snapshotSubscribers    map[uint64]chan struct{}
	nextSnapshotSubscriber uint64
	// children maps a parent task ID to the IDs of subordinate tasks it
	// spawned. TK-02: when Stop is called on a parent, children must be
	// cancelled too so a halted coordinator doesn't leave its workers
	// running. Updates happen under m.mu.
	children map[string]map[string]struct{}
}

type trustedAgentResumeContext struct {
	Input    agentcontract.Input
	Metadata agentcontract.SessionMetadata
}

type RuntimeNotificationSink interface {
	DeliverRuntimeNotification(context.Context, agentcontract.RuntimeNotification) error
}

type RuntimeNotificationSinkFunc func(context.Context, agentcontract.RuntimeNotification) error

func (f RuntimeNotificationSinkFunc) DeliverRuntimeNotification(ctx context.Context, notification agentcontract.RuntimeNotification) error {
	return f(ctx, notification)
}

// runtimeNotificationFollowUpQueue is the in-memory execution projection of
// the durable notification records. notificationMu owns every field. The head
// remains present until its consumer succeeds and the acknowledgement is
// durably stored, so a failed item blocks only its owning conversation.
type runtimeNotificationFollowUpQueue struct {
	items   []runtimeNotificationFollowUpItem
	worker  bool
	blocked bool
}

type runtimeNotificationFollowUpItem struct {
	origin       *backgroundTaskOrigin
	notification agentcontract.RuntimeNotification
}

func NewBackgroundTaskManager(cwd string) *BackgroundTaskManager {
	root := filepath.Clean(strings.TrimSpace(cwd))
	if root == "" {
		root = "."
	}
	outputDir := filepath.Join(root, ".luban-code", "task-output")
	_ = secureio.EnsurePrivateRuntimeDirectory(outputDir)
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	manager := &BackgroundTaskManager{
		tasks:               make(map[string]*BackgroundTask),
		sessions:            make(map[string]*backgroundAgentSession),
		aliases:             make(map[string]string),
		outputDir:           outputDir,
		store:               runtimestore.NewRuntimeTaskStore(root),
		lifecycle:           runtimestore.NewRuntimeLifecycle(root),
		children:            make(map[string]map[string]struct{}),
		followUpQueues:      make(map[string]*runtimeNotificationFollowUpQueue),
		followUpQueued:      make(map[string]string),
		shutdownDone:        make(chan struct{}),
		lifecycleCtx:        lifecycleCtx,
		lifecycleCancel:     lifecycleCancel,
		trustedAgentResumes: make(map[string]trustedAgentResumeContext),
		snapshotSubscribers: make(map[uint64]chan struct{}),
	}
	manager.ReconcileInterruptedAgentRecords()
	return manager
}

const (
	interruptedAgentTerminalReason   = "process_restart"
	backgroundTerminalReasonTimeout  = "timeout"
	backgroundTerminalReasonCanceled = "cancelled"
)

// ReconcileInterruptedAgentRecords marks durable agent runs whose owning
// process has exited as interrupted. It is safe to call periodically after
// manager startup; concurrent calls are serialized and only active records are
// rewritten. The return value is the number of records transitioned.
func (m *BackgroundTaskManager) ReconcileInterruptedAgentRecords() int {
	if m == nil || m.store == nil {
		return 0
	}
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	reconciled := 0
	now := time.Now().UTC()
	for _, record := range m.store.List() {
		if record.Type != agentcontract.TaskTypeLocalAgent || !runtimeAgentRecordIsActive(record) || runtimeTaskOwnerAlive(record.OwnerPID) {
			continue
		}
		record.Status = "failed"
		record.Outcome = agentcontract.RunOutcomeInterrupted
		record.TerminalReason = interruptedAgentTerminalReason
		record.QueuedPrompts = 0
		record.QueueReason = ""
		record.FinishedAt = cloneTimePointer(&now)
		code := -1
		record.ExitCode = &code
		// Persist only the stable reason. Snapshot projection localizes it using
		// the active language during replay.
		record.Error = ""
		for index := range record.Runs {
			run := &record.Runs[index]
			if run.Outcome != agentcontract.RunOutcomeRunning && !strings.EqualFold(strings.TrimSpace(run.Status), "running") {
				continue
			}
			run.Status = "failed"
			run.Outcome = agentcontract.RunOutcomeInterrupted
			run.TerminalReason = interruptedAgentTerminalReason
			run.Error = ""
			run.FinishedAt = cloneTimePointer(&now)
			run.UpdatedAt = now
			if run.DurationMs == nil && !run.StartedAt.IsZero() {
				duration := now.Sub(run.StartedAt).Milliseconds()
				run.DurationMs = &duration
			}
		}
		if err := m.store.Save(record); err == nil {
			reconciled++
		}
	}
	if reconciled > 0 {
		m.notifySnapshotSubscribers()
	}
	return reconciled
}

func runtimeAgentRecordIsActive(record runtimestore.RuntimeTaskRecord) bool {
	if strings.EqualFold(strings.TrimSpace(record.Status), "running") || record.QueuedPrompts > 0 {
		return true
	}
	for _, run := range record.Runs {
		if run.Outcome == agentcontract.RunOutcomeRunning || strings.EqualFold(strings.TrimSpace(run.Status), "running") {
			return true
		}
	}
	return false
}

func runtimeTaskOwnerAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if pid == os.Getpid() {
		return true
	}
	if runtimeIsWindows() {
		// A different PID cannot identify this freshly-started runtime. Windows
		// does not provide the zero-signal probe used on Unix, so fail closed and
		// reconcile the stale record rather than presenting an immortal task.
		return false
	}
	return backgroundProcessAlive(pid)
}

func (m *BackgroundTaskManager) trustedAgentResume(agentID string) (trustedAgentResumeContext, bool) {
	if m == nil {
		return trustedAgentResumeContext{}, false
	}
	m.mu.Lock()
	trusted, ok := m.trustedAgentResumes[strings.TrimSpace(agentID)]
	m.mu.Unlock()
	trusted.Metadata = cloneAgentSessionMetadata(trusted.Metadata)
	return trusted, ok
}

func (m *BackgroundTaskManager) updateTrustedAgentMetadata(agentID string, metadata agentcontract.SessionMetadata) {
	if m == nil {
		return
	}
	m.mu.Lock()
	trusted, ok := m.trustedAgentResumes[strings.TrimSpace(agentID)]
	if ok {
		trusted.Metadata = cloneAgentSessionMetadata(metadata)
		m.trustedAgentResumes[strings.TrimSpace(agentID)] = trusted
	}
	m.mu.Unlock()
}

func trustedAgentResumeMatchesRecord(trusted trustedAgentResumeContext, input agentcontract.Input, metadata agentcontract.SessionMetadata) bool {
	if trusted.Input != input {
		return false
	}
	want := trusted.Metadata
	return want.AgentType == metadata.AgentType &&
		want.CWD == metadata.CWD &&
		want.Mode == metadata.Mode &&
		want.Isolation == metadata.Isolation &&
		want.WorktreeRepoRoot == metadata.WorktreeRepoRoot &&
		want.WorktreePath == metadata.WorktreePath &&
		want.WorktreeBranch == metadata.WorktreeBranch &&
		want.WorktreeHeadCommit == metadata.WorktreeHeadCommit &&
		want.TeamMember == metadata.TeamMember &&
		want.SkillProjectGeneration == metadata.SkillProjectGeneration &&
		want.ApprovalRouting == metadata.ApprovalRouting &&
		want.PresentationSessionID == metadata.PresentationSessionID &&
		reflect.DeepEqual(want.PermissionSnapshot, metadata.PermissionSnapshot)
}

// RegisterChildTask records that childID was spawned by parentID. TK-02: Stop
// uses this to cascade cancellation down the parent-child task tree so a
// halted coordinator's workers don't keep running. The relation is one-shot:
// callers should invoke this immediately after creating the child task.
func (m *BackgroundTaskManager) RegisterChildTask(parentID, childID string) {
	parentID = strings.TrimSpace(parentID)
	childID = strings.TrimSpace(childID)
	if parentID == "" || childID == "" || parentID == childID {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.children == nil {
		m.children = make(map[string]map[string]struct{})
	}
	if m.children[parentID] == nil {
		m.children[parentID] = map[string]struct{}{}
	}
	m.children[parentID][childID] = struct{}{}
}

func (m *BackgroundTaskManager) takeChildrenLocked(parentID string) []string {
	if len(m.children) == 0 {
		return nil
	}
	set, ok := m.children[parentID]
	if !ok {
		return nil
	}
	delete(m.children, parentID)
	out := make([]string, 0, len(set))
	for child := range set {
		out = append(out, child)
	}
	return out
}

func (m *BackgroundTaskManager) SetProjectRoot(root string) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	trimmed = filepath.Clean(trimmed)
	m.outputDir = filepath.Join(trimmed, ".luban-code", "task-output")
	_ = secureio.EnsurePrivateRuntimeDirectory(m.outputDir)
	m.store = runtimestore.NewRuntimeTaskStore(trimmed)
	m.lifecycle = runtimestore.NewRuntimeLifecycle(trimmed)
}

// SetTaskOwnerSession binds a retained task to the parent conversation that
// launched it. The binding is persisted so completion cannot be routed to a
// different session after a project/session switch.
func (m *BackgroundTaskManager) SetTaskOwnerSession(taskID, sessionID string) bool {
	taskID = strings.TrimSpace(taskID)
	sessionID = strings.TrimSpace(sessionID)
	if m == nil || taskID == "" || sessionID == "" {
		return false
	}
	m.mu.Lock()
	task := m.tasks[taskID]
	m.mu.Unlock()
	if task == nil {
		return false
	}
	task.mu.Lock()
	task.OwnerSessionID = sessionID
	record := task.recordLocked()
	task.mu.Unlock()
	m.persistRecordForTask(task, record)
	return true
}

// MarkAgentDetached records that a retained Agent run has left the foreground
// call path. The flag is durable because completion presentation may be
// reconstructed after a restart.
func (m *BackgroundTaskManager) MarkAgentDetached(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if m == nil || taskID == "" {
		return false
	}
	m.mu.Lock()
	task := m.tasks[taskID]
	m.mu.Unlock()
	if task == nil || task.Type != agentcontract.TaskTypeLocalAgent {
		return false
	}
	task.mu.Lock()
	changed := !task.Detached
	task.Detached = true
	record := task.recordLocked()
	task.mu.Unlock()
	if changed {
		m.persistRecordForTask(task, record)
		m.notifySnapshotSubscribers()
	}
	return true
}

func (m *BackgroundTaskManager) currentTaskOriginLocked() *backgroundTaskOrigin {
	return &backgroundTaskOrigin{
		projectRoot:      filepath.Clean(strings.TrimSpace(filepath.Dir(filepath.Dir(m.outputDir)))),
		outputDir:        m.outputDir,
		store:            m.store,
		lifecycle:        m.lifecycle,
		notificationSink: m.notificationSink,
	}
}

func (m *BackgroundTaskManager) currentTaskOrigin() *backgroundTaskOrigin {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentTaskOriginLocked()
}

// CurrentProjectRoot returns the project namespace currently selected by the
// manager. Task origins remain pinned separately when a task is launched.
func (m *BackgroundTaskManager) CurrentProjectRoot() string {
	origin := m.currentTaskOrigin()
	if origin == nil {
		return ""
	}
	return origin.projectRoot
}

func (m *BackgroundTaskManager) originForTask(task *BackgroundTask) *backgroundTaskOrigin {
	if task != nil {
		task.mu.RLock()
		origin := task.origin
		task.mu.RUnlock()
		if origin != nil {
			return origin
		}
	}
	return m.currentTaskOrigin()
}

// completionContextForTask rehydrates the immutable parent hook scope for a
// retained task after its asynchronous request context has ended.
func (m *BackgroundTaskManager) completionContextForTask(task *BackgroundTask) context.Context {
	return m.originForTask(task).hookContext(context.Background())
}

func (m *BackgroundTaskManager) SetAgentSessionRestorer(restorer func(agentID string, record runtimestore.RuntimeTaskRecord) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agentSessionRestorer = restorer
}

// SetHookRunner registers a hooks.Runner that will receive a Notification hook
// (with task-completion message) when a background task finishes. Passing nil
// disables hook emission.
func (m *BackgroundTaskManager) SetHookRunner(runner *hooks.Runner) {
	m.mu.Lock()
	if m.shuttingDown {
		m.mu.Unlock()
		return
	}
	m.hookRunner = runner
	if runner != nil && runner.HasHooks(hooks.HookNotification) {
		m.notificationSink = hookRuntimeNotificationSink{runner: runner}
	} else if _, ok := m.notificationSink.(hookRuntimeNotificationSink); ok {
		m.notificationSink = nil
	}
	m.mu.Unlock()
	_ = m.ReplayPendingNotifications(context.Background())
}

func (m *BackgroundTaskManager) SetNotificationSink(sink RuntimeNotificationSink) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.shuttingDown {
		m.mu.Unlock()
		return
	}
	m.notificationSink = sink
	m.mu.Unlock()
	_ = m.ReplayPendingNotifications(context.Background())
}

// SetNotificationConsumers updates the TUI observer and model follow-up as one
// registration step, so replay cannot acknowledge one before the other is
// known to be required.
func (m *BackgroundTaskManager) SetNotificationConsumers(observer, followUp RuntimeNotificationSink) {
	if m == nil {
		return
	}
	m.notificationMu.Lock()
	m.mu.Lock()
	if m.shuttingDown {
		m.mu.Unlock()
		m.notificationMu.Unlock()
		return
	}
	m.notificationObserver = observer
	m.notificationFollowUp = followUp
	m.mu.Unlock()
	m.notificationMu.Unlock()
	if observer != nil || followUp != nil {
		_ = m.ReplayPendingNotifications(context.Background())
	}
	if followUp == nil {
		m.followUpWG.Wait()
	}
}

func (m *BackgroundTaskManager) nextTaskIDLocked(origin *backgroundTaskOrigin) string {
	for {
		m.nextID++
		id := strconv.Itoa(m.nextID)
		if _, exists := m.tasks[id]; exists {
			continue
		}
		if _, exists := m.sessions[id]; exists {
			continue
		}
		if origin != nil && origin.store != nil {
			if origin.store.Exists(id) {
				continue
			}
		}
		if origin != nil {
			if _, err := secureio.TightenPrivateRuntimeRegularFile(origin.taskOutputPath(id), false); err == nil {
				continue
			}
		}
		return id
	}
}

func (m *BackgroundTaskManager) runManagedAsync(fn func()) bool {
	if m == nil || fn == nil {
		return false
	}
	release, ok := m.beginManagedWork()
	if !ok {
		return false
	}
	go func() {
		defer release()
		fn()
	}()
	return true
}

func (m *BackgroundTaskManager) beginManagedWork() (func(), bool) {
	if m == nil {
		return func() {}, false
	}
	m.mu.Lock()
	if m.shuttingDown {
		m.mu.Unlock()
		return func() {}, false
	}
	m.asyncWG.Add(1)
	m.mu.Unlock()
	var once sync.Once
	return func() { once.Do(m.asyncWG.Done) }, true
}

// Shutdown initiates manager shutdown exactly once and waits only for
// the supplied context. Cancellation of that wait does not abandon cleanup:
// the one background finalizer keeps draining admitted work and eventually
// closes shutdownDone, so a later caller can still observe full completion.
func (m *BackgroundTaskManager) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	if m.shutdownDone == nil {
		m.shutdownDone = make(chan struct{})
	}
	done := m.shutdownDone
	if m.shuttingDown {
		m.mu.Unlock()
		return waitForShutdownContext(ctx, done)
	}
	m.shuttingDown = true
	lifecycleCancel := m.lifecycleCancel
	sessions := make([]*backgroundAgentSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	tasks := make([]*BackgroundTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	m.mu.Unlock()

	if lifecycleCancel != nil {
		lifecycleCancel()
	}
	for _, session := range sessions {
		if session != nil && session.cancel != nil {
			session.cancel()
		}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		task.mu.RLock()
		cancel := task.cancel
		process := task.process
		task.mu.RUnlock()
		if cancel != nil {
			cancel()
		}
		if process != nil {
			_ = forceKillBackgroundProcess(process)
		}
	}

	go m.finishShutdown(sessions, tasks, done)
	return waitForShutdownContext(ctx, done)
}

func (m *BackgroundTaskManager) finishShutdown(sessions []*backgroundAgentSession, tasks []*BackgroundTask, done chan struct{}) {
	for _, session := range sessions {
		if session != nil && session.done != nil {
			<-session.done
		}
	}
	for _, session := range sessions {
		if session != nil {
			_, _ = cleanupAgentWorktreeIfClean(session.metadataSnapshot())
		}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		task.mu.RLock()
		taskDone := task.done
		task.mu.RUnlock()
		if taskDone != nil {
			<-taskDone
		}
	}
	m.asyncWG.Wait()
	m.notificationMu.Lock()
	m.mu.Lock()
	m.notificationFollowUp = nil
	m.mu.Unlock()
	m.notificationMu.Unlock()
	m.followUpWG.Wait()
	close(done)
}

func waitForShutdownContext(ctx context.Context, done <-chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// managedContext binds admitted work to the manager lifetime in addition to
// the caller lifetime. It closes the admission/snapshot race where shutdown
// starts after beginManagedWork but before the task is registered in m.tasks.
func (m *BackgroundTaskManager) managedContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	m.mu.Lock()
	lifecycleCtx := m.lifecycleCtx
	m.mu.Unlock()
	if lifecycleCtx == nil {
		return context.WithCancel(parent)
	}
	ctx, cancel := context.WithCancel(parent)
	stopLifecycleCancel := context.AfterFunc(lifecycleCtx, cancel)
	return ctx, func() {
		stopLifecycleCancel()
		cancel()
	}
}

func (m *BackgroundTaskManager) registerTask(task *BackgroundTask) {
	if task == nil {
		return
	}
	currentOrigin := m.currentTaskOrigin()
	task.mu.Lock()
	if task.OwnerPID == 0 {
		task.OwnerPID = os.Getpid()
	}
	if task.origin == nil {
		task.origin = currentOrigin
	}
	origin := task.origin
	task.mu.Unlock()

	m.mu.Lock()
	m.tasks[task.ID] = task
	m.mu.Unlock()

	m.persistTask(task)
	m.publishTaskStarted(origin, task)
}

func (m *BackgroundTaskManager) publishTaskStarted(origin *backgroundTaskOrigin, task *BackgroundTask) {
	if task == nil {
		return
	}
	if origin != nil && origin.lifecycle != nil {
		base := runtimestore.RuntimeLifecycleEvent{
			Type:     runtimestore.LifecycleTaskCreated,
			EntityID: task.ID,
			ToolName: lifecycleToolNameForTask(task.Type),
			Status:   task.Status,
			Payload: map[string]any{
				"type":        task.Type,
				"description": task.Description,
			},
		}
		_ = origin.lifecycle.Publish(context.Background(), base)
		base.Type = runtimestore.LifecycleToolStart
		_ = origin.lifecycle.Publish(context.Background(), base)
	}
}

func (m *BackgroundTaskManager) persistTask(task *BackgroundTask) {
	m.persistCurrentTask(task)
}

func (m *BackgroundTaskManager) persistRecordForTask(task *BackgroundTask, _ runtimestore.RuntimeTaskRecord) {
	m.persistCurrentTask(task)
}

// persistCurrentTask serializes durable state publication for one task and
// captures the record only after acquiring that publication gate. Callers
// intentionally release task.mu before doing I/O; without this second gate an
// older captured record can acquire the file lock after a newer record and
// erase a retained agent attempt from durable history.
func (m *BackgroundTaskManager) persistCurrentTask(task *BackgroundTask) {
	if task == nil {
		return
	}
	task.persistMu.Lock()
	defer task.persistMu.Unlock()

	task.mu.RLock()
	record := task.recordLocked()
	origin := task.origin
	task.mu.RUnlock()
	if origin == nil {
		origin = m.currentTaskOrigin()
	}
	m.persistRecordTo(origin, record)
}

func (m *BackgroundTaskManager) persistRecordTo(origin *backgroundTaskOrigin, record runtimestore.RuntimeTaskRecord) {
	_ = m.persistRecordToErr(origin, record)
}

func (m *BackgroundTaskManager) persistRecordToErr(origin *backgroundTaskOrigin, record runtimestore.RuntimeTaskRecord) error {
	if origin == nil || origin.store == nil || strings.TrimSpace(record.ID) == "" {
		return fs.ErrInvalid
	}
	if record.Notification == nil {
		if current, ok := origin.store.Get(record.ID); ok {
			record.Notification = current.Notification
			record.Notifications = append([]agentcontract.RuntimeNotification(nil), current.Notifications...)
		}
	}
	if strings.TrimSpace(record.OwnerProjectRoot) == "" {
		record.OwnerProjectRoot = origin.projectRoot
	}
	if record.Notification != nil && strings.TrimSpace(record.Notification.ProjectRoot) == "" {
		record.Notification.ProjectRoot = record.OwnerProjectRoot
	}
	if record.Notification != nil && strings.TrimSpace(record.Notification.SessionProjectDir) == "" {
		record.Notification.SessionProjectDir = record.OwnerSessionProjectDir
	}
	if err := origin.store.Save(record); err != nil {
		return err
	}
	m.notifySnapshotSubscribers()
	return nil
}

// SubscribeSnapshots publishes an edge-triggered notification after an
// in-memory task snapshot is durably updated. Consumers fetch
// InMemorySnapshots on receipt; the buffered edge coalesces bursts without
// blocking task execution.
func (m *BackgroundTaskManager) SubscribeSnapshots() (<-chan struct{}, func()) {
	if m == nil {
		closed := make(chan struct{})
		close(closed)
		return closed, func() {}
	}
	m.snapshotMu.Lock()
	m.nextSnapshotSubscriber++
	id := m.nextSnapshotSubscriber
	updates := make(chan struct{}, 1)
	if m.snapshotSubscribers == nil {
		m.snapshotSubscribers = make(map[uint64]chan struct{})
	}
	m.snapshotSubscribers[id] = updates
	m.snapshotMu.Unlock()
	var once sync.Once
	return updates, func() {
		once.Do(func() {
			m.snapshotMu.Lock()
			delete(m.snapshotSubscribers, id)
			m.snapshotMu.Unlock()
		})
	}
}

func (m *BackgroundTaskManager) notifySnapshotSubscribers() {
	if m == nil {
		return
	}
	m.snapshotMu.Lock()
	for _, updates := range m.snapshotSubscribers {
		select {
		case updates <- struct{}{}:
		default:
		}
	}
	m.snapshotMu.Unlock()
}

func snapshotFromRecord(record runtimestore.RuntimeTaskRecord) agentcontract.TaskSnapshot {
	runs := cloneRuntimeTaskRunRecords(record.Runs)
	for index := range runs {
		runs[index].Error = localizedBackgroundTaskError(runs[index].Error, runs[index].TerminalReason, 0)
	}
	return agentcontract.TaskSnapshot{
		ID:                     record.ID,
		Type:                   record.Type,
		Status:                 record.Status,
		Description:            record.Description,
		Command:                record.Command,
		Prompt:                 record.Prompt,
		OutputPath:             record.OutputPath,
		ExitCode:               record.ExitCode,
		Error:                  localizedBackgroundTaskError(record.Error, record.TerminalReason, time.Duration(record.TimeoutNanos)),
		Result:                 record.Result,
		OwnerSessionID:         record.OwnerSessionID,
		OwnerSessionProjectDir: record.OwnerSessionProjectDir,
		OwnerProjectRoot:       record.OwnerProjectRoot,
		OwnerAgentID:           record.OwnerAgentID,
		OwnerPID:               record.OwnerPID,
		AgentAlias:             record.AgentAlias,
		Detached:               runtimeTaskRecordDetached(record),
		CurrentRunID:           record.CurrentRunID,
		Attempt:                record.Attempt,
		BatchID:                record.BatchID,
		ParentRunID:            record.ParentRunID,
		AgentPath:              record.AgentPath,
		QueuedPrompts:          record.QueuedPrompts,
		QueueReason:            record.QueueReason,
		Runs:                   runs,
		LatestProgress:         cloneAgentProgressEvent(record.LatestProgress),
		TranscriptPath:         record.TranscriptPath,
		DurationMs:             cloneInt64Pointer(record.DurationMs),
		TotalTokens:            cloneIntPointer(record.TotalTokens),
		Usage:                  cloneUsagePointer(record.Usage),
		Outcome:                record.Outcome,
		TerminalReason:         record.TerminalReason,
		Timeout:                time.Duration(record.TimeoutNanos),
		ArtifactRefs:           append([]string(nil), record.ArtifactRefs...),
		VerificationRefs:       append([]string(nil), record.VerificationRefs...),
	}
}

func snapshotFromRecordForNotification(record runtimestore.RuntimeTaskRecord, notification agentcontract.RuntimeNotification) agentcontract.TaskSnapshot {
	snapshot := snapshotFromRecord(record)
	runID := strings.TrimSpace(notification.RunID)
	if runID == "" {
		return snapshot
	}
	for _, run := range record.Runs {
		if run.RunID != runID {
			continue
		}
		snapshot.Status = run.Status
		snapshot.Prompt = run.Prompt
		snapshot.Error = localizedBackgroundTaskError(run.Error, run.TerminalReason, 0)
		snapshot.Result = run.Result
		snapshot.CurrentRunID = run.RunID
		snapshot.Attempt = run.Attempt
		snapshot.BatchID = run.BatchID
		snapshot.ParentRunID = run.ParentRunID
		snapshot.AgentPath = run.AgentPath
		snapshot.LatestProgress = cloneAgentProgressEvent(run.LatestProgress)
		snapshot.TranscriptPath = run.TranscriptPath
		snapshot.DurationMs = cloneInt64Pointer(run.DurationMs)
		snapshot.TotalTokens = cloneIntPointer(run.TotalTokens)
		snapshot.Usage = cloneUsagePointer(run.Usage)
		snapshot.Outcome = run.Outcome
		snapshot.TerminalReason = run.TerminalReason
		snapshot.ArtifactRefs = append([]string(nil), run.ArtifactRefs...)
		snapshot.VerificationRefs = append([]string(nil), run.VerificationRefs...)
		if notification.ExitCode != nil {
			code := *notification.ExitCode
			snapshot.ExitCode = &code
		}
		return snapshot
	}
	return snapshot
}

func runtimeTaskRecordDetached(record runtimestore.RuntimeTaskRecord) bool {
	if record.Detached {
		return true
	}
	return record.AgentInput != nil && record.AgentInput.RunInBackground
}

func localizedBackgroundTaskError(raw, terminalReason string, timeout time.Duration) string {
	switch strings.TrimSpace(terminalReason) {
	case interruptedAgentTerminalReason:
		return toolRuntimeText(i18n.KeyToolBackgroundRuntimeInterrupted)
	case backgroundTerminalReasonTimeout:
		if timeout > 0 {
			return toolRuntimeFormat(i18n.KeyToolBackgroundCommandTimedOutAfter, timeout)
		}
		return toolRuntimeText(i18n.KeyToolBackgroundCommandTimedOut)
	case backgroundTerminalReasonCanceled:
		return toolRuntimeText(i18n.KeyToolBackgroundTaskCanceled)
	default:
		return raw
	}
}

func durableBackgroundTaskError(raw, terminalReason string) string {
	switch strings.TrimSpace(terminalReason) {
	case interruptedAgentTerminalReason, backgroundTerminalReasonTimeout, backgroundTerminalReasonCanceled:
		return ""
	default:
		return raw
	}
}

func lifecycleToolNameForTask(taskType string) string {
	switch taskType {
	case agentcontract.TaskTypeLocalAgent:
		return "Agent"
	case agentcontract.TaskTypeLocalBash:
		return "Bash"
	default:
		return "Task"
	}
}

func (m *BackgroundTaskManager) Snapshot(id string) (agentcontract.TaskSnapshot, bool) {
	m.mu.Lock()
	task, ok := m.tasks[id]
	store := m.store
	m.mu.Unlock()
	if !ok {
		if store != nil {
			if record, ok := store.Get(id); ok {
				return snapshotFromRecord(record), true
			}
		}
		return agentcontract.TaskSnapshot{}, false
	}
	return task.snapshot(), true
}

// Snapshots returns a race-safe copy of all known background activities.
func (m *BackgroundTaskManager) Snapshots() []agentcontract.TaskSnapshot {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	tasks := make([]*BackgroundTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	store := m.store
	m.mu.Unlock()
	byID := make(map[string]agentcontract.TaskSnapshot, len(tasks))
	for _, task := range tasks {
		snapshot := task.snapshot()
		byID[snapshot.ID] = snapshot
	}
	if store != nil {
		for _, record := range store.List() {
			snapshot := snapshotFromRecord(record)
			if _, exists := byID[snapshot.ID]; !exists {
				byID[snapshot.ID] = snapshot
			}
		}
	}
	out := make([]agentcontract.TaskSnapshot, 0, len(byID))
	for _, snapshot := range byID {
		out = append(out, snapshot)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// InMemorySnapshots returns race-safe clones of tasks owned by this manager
// without scanning or decoding the durable runtime-task directory. Live UI
// refresh loops use this API after their initial startup reconciliation.
func (m *BackgroundTaskManager) InMemorySnapshots() []agentcontract.TaskSnapshot {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	tasks := make([]*BackgroundTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	m.mu.Unlock()
	out := make([]agentcontract.TaskSnapshot, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// stopGracePeriod is how long Stop waits for a process to exit cleanly after
// SIGTERM/cancel before escalating to SIGKILL/Process.Kill. TK-01: TS does not
// have a grace period either but the rubustness audit calls it out as a
// standard pattern; 2s is enough to flush stdout buffers and run shutdown
// handlers without making "stop" feel sluggish.
var stopGracePeriod = 2 * time.Second

func (m *BackgroundTaskManager) Stop(id string) (agentcontract.TaskSnapshot, error) {
	m.mu.Lock()
	task, ok := m.tasks[id]
	childIDs := m.takeChildrenLocked(id)
	store := m.store
	m.mu.Unlock()
	if !ok {
		if store != nil {
			if record, ok := store.Get(id); ok {
				if record.Status == "running" {
					return agentcontract.TaskSnapshot{}, i18n.NewError(i18n.KeyToolRuntimeBackgroundTaskRunningOtherProcess, "Task", id)
				}
				return agentcontract.TaskSnapshot{}, i18n.NewError(i18n.KeyToolRuntimeBackgroundTaskNotRunning, "Task", id, record.Status)
			}
		}
		return agentcontract.TaskSnapshot{}, i18n.NewError(i18n.KeyToolRuntimeBackgroundTaskNotFound, "Task", "ID", id)
	}

	task.mu.Lock()
	if task.Status != "running" {
		status := task.Status
		task.mu.Unlock()
		return agentcontract.TaskSnapshot{}, i18n.NewError(i18n.KeyToolRuntimeBackgroundTaskNotRunning, "Task", id, status)
	}
	task.Status = "killed"
	code := -1
	task.ExitCode = &code
	finishedAt := time.Now().UTC()
	task.FinishedAt = &finishedAt
	cancel := task.cancel
	process := task.process
	done := task.done
	task.mu.Unlock()

	// TK-01: cancel the context first (cooperative shutdown via SIGTERM-like
	// signal on Unix when the cmd was started in its own process group), then
	// wait briefly for the process to exit before escalating to Kill().
	if cancel != nil {
		cancel()
	}
	if process != nil {
		_ = terminateBackgroundProcess(process)
		if !waitForProcessExit(done, stopGracePeriod) {
			_ = forceKillBackgroundProcess(process)
		}
	}

	// TK-02: cascade cancellation to any registered child tasks so a halted
	// parent doesn't leave its sub-agents running.
	for _, childID := range childIDs {
		go func(child string) {
			_, _ = m.Stop(child)
		}(childID)
	}

	m.persistTask(task)
	return task.snapshot(), nil
}

// waitForProcessExit returns true when the task's done channel closes within
// the grace period, false on timeout. A nil done channel means we have no
// signal — return false so the caller escalates to Kill.
func waitForProcessExit(done chan struct{}, grace time.Duration) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	case <-time.After(grace):
		return false
	}
}

func (m *BackgroundTaskManager) StartShellTask(parent context.Context, command, description string, cmd *exec.Cmd) (*agentcontract.TaskSnapshot, error) {
	return m.startShellTask(parent, command, description, cmd, 0, nil)
}

// StartShellCommand starts a managed shell command and returns the stable task
// receipt needed by shell-tool consumers. A positive timeout is a hard runtime
// limit owned by the manager; zero or a negative duration disables that limit,
// while parent cancellation still stops the command. After a successful start,
// the manager invokes completion exactly once when the process waiter returns.
func (m *BackgroundTaskManager) StartShellCommand(parent context.Context, command, description string, cmd *exec.Cmd, timeout time.Duration, completion func(error, int)) (taskID, outputPath string, err error) {
	snapshot, err := m.startShellTaskWithCompletion(parent, command, description, cmd, timeout, completion)
	if err != nil {
		return "", "", err
	}
	return snapshot.ID, snapshot.OutputPath, nil
}

// startShellTaskWithCompletion transfers ownership of completion to the task
// waiter after a successful start. It is used by Bash sed transactions to hold
// cooperative file locks through process exit and evidence refresh.
func (m *BackgroundTaskManager) startShellTaskWithCompletion(parent context.Context, command, description string, cmd *exec.Cmd, timeout time.Duration, completion func(error, int)) (*agentcontract.TaskSnapshot, error) {
	return m.startShellTask(parent, command, description, cmd, timeout, completion)
}

func (m *BackgroundTaskManager) startShellTask(parent context.Context, command, description string, cmd *exec.Cmd, timeout time.Duration, completion func(error, int)) (*agentcontract.TaskSnapshot, error) {
	releaseWaiter, admitted := m.beginManagedWork()
	if !admitted {
		return nil, i18n.NewError(i18n.KeyToolNotificationManagerShuttingDown)
	}
	releaseWatcher, admitted := m.beginManagedWork()
	if !admitted {
		releaseWaiter()
		return nil, i18n.NewError(i18n.KeyToolNotificationManagerShuttingDown)
	}
	workersStarted := false
	defer func() {
		if !workersStarted {
			releaseWatcher()
			releaseWaiter()
		}
	}()
	m.mu.Lock()
	origin := m.currentTaskOriginLocked()
	taskID := m.nextTaskIDLocked(origin)
	m.mu.Unlock()

	if err := secureio.EnsurePrivateRuntimeDirectory(origin.outputDir); err != nil {
		return nil, i18n.WrapError(i18n.KeyToolRuntimeBackgroundOutputDirCreateFailed, err)
	}

	outputPath := origin.taskOutputPath(taskID)
	outFile, err := newRotatingFileWriter(outputPath, BackgroundOutputCapBytes)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyToolBackgroundOutputOpenFailed, err)
	}

	if parent == nil {
		parent = context.Background()
	}
	pinBackgroundTaskOriginHookContext(origin, parent)
	taskCtx, cancel := m.managedContext(parent)
	cmd.Stdout = outFile
	cmd.Stderr = outFile
	prepareBackgroundCommand(cmd)

	task := &BackgroundTask{
		ID:                     taskID,
		Type:                   agentcontract.TaskTypeLocalBash,
		Status:                 "running",
		Description:            description,
		Command:                command,
		OutputPath:             outputPath,
		StartedAt:              time.Now().UTC(),
		cancel:                 cancel,
		done:                   make(chan struct{}),
		origin:                 origin,
		OwnerSessionID:         backgroundTaskOwnerSessionID(parent),
		OwnerSessionProjectDir: backgroundTaskOwnerSessionProjectDir(parent),
		OwnerProjectRoot:       origin.projectRoot,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		_ = outFile.Close()
		return nil, i18n.WrapError(i18n.KeyToolRuntimeBackgroundCommandStartFailed, err)
	}

	task.mu.Lock()
	task.process = cmd.Process
	task.mu.Unlock()
	m.registerTask(task)
	workersStarted = true
	go func() {
		defer releaseWatcher()
		m.watchShellTask(task, taskCtx, timeout)
	}()

	go func() {
		defer releaseWaiter()
		defer close(task.done)
		defer cancel()

		err := cmd.Wait()
		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		} else if err != nil {
			exitCode = -1
		}

		// Emit TASK_FINISHED line BEFORE closing the writer so callers tailing
		// the file see the final state.
		duration := time.Since(task.StartedAt).Milliseconds()
		_, _ = outFile.WriteString(fmt.Sprintf("\nTASK_FINISHED exit=%d duration=%d\n", exitCode, duration))
		_ = outFile.Close()
		if completion != nil {
			// Completion is a bookkeeping boundary, not part of process success.
			// Isolate a faulty observer so task terminal state and done closure are
			// still published; file-lock release callbacks use their own defer.
			func() {
				defer func() { _ = recover() }()
				completion(err, exitCode)
			}()
		}

		task.mu.Lock()
		if task.Status == "killed" {
			code := -1
			task.ExitCode = &code
			finishedAt := time.Now().UTC()
			task.FinishedAt = &finishedAt
			record := task.recordLocked()
			task.mu.Unlock()
			m.persistRecordForTask(task, record)
			m.emitTaskCompletionNotification(context.Background(), task, "killed", -1)
			return
		}
		if task.Status == "failed" && task.TerminalReason == backgroundTerminalReasonTimeout {
			code := -1
			task.ExitCode = &code
			finishedAt := time.Now().UTC()
			task.FinishedAt = &finishedAt
			record := task.recordLocked()
			task.mu.Unlock()
			m.persistRecordForTask(task, record)
			m.emitTaskCompletionNotification(context.Background(), task, "failed", -1)
			return
		}

		code := exitCode
		task.ExitCode = &code
		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
			finishedAt := time.Now().UTC()
			task.FinishedAt = &finishedAt
			record := task.recordLocked()
			task.mu.Unlock()
			m.persistRecordForTask(task, record)
			m.emitTaskCompletionNotification(context.Background(), task, "failed", exitCode)
			return
		}
		task.Status = "completed"
		finishedAt := time.Now().UTC()
		task.FinishedAt = &finishedAt
		record := task.recordLocked()
		task.mu.Unlock()
		m.persistRecordForTask(task, record)
		m.emitTaskCompletionNotification(context.Background(), task, "completed", exitCode)
	}()

	snap := task.snapshot()
	return &snap, nil
}

func (m *BackgroundTaskManager) watchShellTask(task *BackgroundTask, taskCtx context.Context, timeout time.Duration) {
	if task == nil {
		return
	}
	var (
		timer    *time.Timer
		timeoutC <-chan time.Time
	)
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		timeoutC = timer.C
		defer timer.Stop()
	}

	status := "killed"
	terminalReason := backgroundTerminalReasonCanceled
	select {
	case <-task.done:
		return
	case <-taskCtx.Done():
		if errors.Is(taskCtx.Err(), context.DeadlineExceeded) {
			status = "failed"
			terminalReason = backgroundTerminalReasonTimeout
		}
	case <-timeoutC:
		status = "failed"
		terminalReason = backgroundTerminalReasonTimeout
	}

	task.mu.Lock()
	if task.Status != "running" {
		task.mu.Unlock()
		return
	}
	task.Status = status
	task.Timeout = timeout
	task.Error = localizedBackgroundTaskError("", terminalReason, timeout)
	task.TerminalReason = terminalReason
	process := task.process
	task.mu.Unlock()

	if process == nil {
		return
	}
	_ = terminateBackgroundProcess(process)
	if !waitForProcessExit(task.done, stopGracePeriod) {
		_ = forceKillBackgroundProcess(process)
	}
}

func (m *BackgroundTaskManager) emitTaskCompletionNotification(ctx context.Context, task *BackgroundTask, status string, exitCode int) {
	if task == nil {
		return
	}
	notification := runtimeNotificationForTask(task, status, exitCode)
	m.emitCompletionNotification(ctx, task, notification, status, exitCode)
}

func (m *BackgroundTaskManager) emitAgentCompletionNotification(ctx context.Context, task *BackgroundTask, status string, exitCode int, summary agentRunSummary) {
	if task == nil {
		return
	}
	notification := runtimeNotificationForTask(task, status, exitCode)
	notification.TranscriptPath = summary.TranscriptPath
	durationMs := summary.TotalDuration
	totalTokens := summary.TotalTokens
	notification.DurationMs = &durationMs
	notification.TotalTokens = &totalTokens
	notification.Provider = summary.Provider
	notification.Model = summary.Model
	if summary.Usage != nil {
		usage := *summary.Usage
		notification.Usage = &usage
	}
	m.emitCompletionNotification(ctx, task, notification, status, exitCode)
}

func (m *BackgroundTaskManager) emitCompletionNotification(ctx context.Context, task *BackgroundTask, notification agentcontract.RuntimeNotification, status string, exitCode int) {
	_ = m.deliverRuntimeNotification(ctx, task, notification)
	origin := m.originForTask(task)
	if origin != nil && origin.lifecycle != nil {
		base := runtimestore.RuntimeLifecycleEvent{
			Type:     runtimestore.LifecycleTaskCompleted,
			EntityID: task.ID,
			ToolName: lifecycleToolNameForTask(task.Type),
			Status:   status,
			Payload: map[string]any{
				"type":      task.Type,
				"exit_code": exitCode,
			},
		}
		_ = origin.lifecycle.Publish(ctx, base)
		base.Type = runtimestore.LifecycleToolComplete
		_ = origin.lifecycle.Publish(ctx, base)
	}
}

func runtimeNotificationForTask(task *BackgroundTask, status string, exitCode int) agentcontract.RuntimeNotification {
	task.mu.RLock()
	id := task.ID
	taskType := task.Type
	runID := task.CurrentRunID
	attempt := task.Attempt
	description := task.Description
	command := task.Command
	sessionID := task.OwnerSessionID
	sessionProjectDir := task.OwnerSessionProjectDir
	projectRoot := task.OwnerProjectRoot
	if strings.TrimSpace(projectRoot) == "" && task.origin != nil {
		projectRoot = task.origin.projectRoot
	}
	task.mu.RUnlock()

	label := description
	if strings.TrimSpace(label) == "" {
		label = command
	}
	if strings.TrimSpace(label) == "" {
		label = taskType
	}
	title := toolRuntimeFormat(i18n.KeyToolRuntimeBackgroundTaskNotificationTitle, status)
	message := toolRuntimeFormat(i18n.KeyToolRuntimeBackgroundTaskNotification, id, taskType, status, exitCode)
	if strings.TrimSpace(label) != "" {
		message = toolRuntimeFormat(i18n.KeyToolRuntimeBackgroundTaskNotificationWithLabel, label, id, taskType, status, exitCode)
	}
	code := exitCode
	return agentcontract.RuntimeNotification{
		ID:                uuid.NewString(),
		Kind:              "task-notification",
		TaskID:            id,
		RunID:             runID,
		Attempt:           attempt,
		SessionID:         sessionID,
		ProjectRoot:       projectRoot,
		SessionProjectDir: sessionProjectDir,
		Title:             title,
		Message:           message,
		Status:            status,
		ExitCode:          &code,
		CreatedAt:         time.Now().UTC(),
	}
}

// LocalizeRuntimeNotification renders the product-owned notification shell in
// lang from structured task facts, so replay uses the active runtime language.
func LocalizeRuntimeNotification(lang i18n.Language, notification agentcontract.RuntimeNotification, snapshot agentcontract.TaskSnapshot) agentcontract.RuntimeNotification {
	if notification.Kind != "task-notification" {
		return notification
	}
	status := firstNonEmpty(notification.Status, snapshot.Status)
	taskType := firstNonEmpty(snapshot.Type, "task")
	exitCode := 0
	if notification.ExitCode != nil {
		exitCode = *notification.ExitCode
	} else if snapshot.ExitCode != nil {
		exitCode = *snapshot.ExitCode
	}
	label := firstNonEmpty(snapshot.Description, snapshot.Command, taskType)
	notification.Title = i18n.Format(lang, i18n.KeyToolRuntimeBackgroundTaskNotificationTitle, status)
	if strings.TrimSpace(label) == "" {
		notification.Message = i18n.Format(lang, i18n.KeyToolRuntimeBackgroundTaskNotification, notification.TaskID, taskType, status, exitCode)
	} else {
		notification.Message = i18n.Format(lang, i18n.KeyToolRuntimeBackgroundTaskNotificationWithLabel, label, notification.TaskID, taskType, status, exitCode)
	}
	return notification
}

type RuntimeNotificationFollowUpTarget struct {
	SessionID         string
	SessionProjectDir string
	ProjectRoot       string
	Message           string
}

// NotificationFollowUpTarget returns the full owning conversation identity and
// a structured user turn. The task record is loaded from the notification's
// immutable origin rather than the manager's mutable foreground task map.
func (m *BackgroundTaskManager) NotificationFollowUpTarget(notification agentcontract.RuntimeNotification) (RuntimeNotificationFollowUpTarget, bool) {
	if m == nil || strings.TrimSpace(notification.TaskID) == "" {
		return RuntimeNotificationFollowUpTarget{}, false
	}
	record, projectRoot, ok := m.notificationRecord(notification)
	if !ok {
		return RuntimeNotificationFollowUpTarget{}, false
	}
	snapshot := snapshotFromRecordForNotification(record, notification)
	sessionID := firstNonEmpty(notification.SessionID, snapshot.OwnerSessionID)
	if strings.TrimSpace(sessionID) == "" {
		return RuntimeNotificationFollowUpTarget{}, false
	}
	output := snapshot.Result
	truncated := false
	if strings.TrimSpace(output) == "" && strings.TrimSpace(snapshot.OutputPath) != "" {
		if taskOutput, err := ReadTaskOutput(snapshot.OutputPath, 64*1024); err == nil {
			output = taskOutput.Content
			truncated = taskOutput.WasTruncated
		}
	}
	payload := struct {
		NotificationID  string `json:"notification_id"`
		TaskID          string `json:"task_id"`
		RunID           string `json:"run_id,omitempty"`
		Attempt         int    `json:"attempt,omitempty"`
		Type            string `json:"type"`
		Status          string `json:"status"`
		Description     string `json:"description,omitempty"`
		Result          string `json:"result,omitempty"`
		Error           string `json:"error,omitempty"`
		OutputFile      string `json:"output_file,omitempty"`
		OutputTruncated bool   `json:"output_truncated,omitempty"`
		TranscriptPath  string `json:"transcript_path,omitempty"`
	}{
		NotificationID:  notification.ID,
		TaskID:          snapshot.ID,
		RunID:           notification.RunID,
		Attempt:         notification.Attempt,
		Type:            snapshot.Type,
		Status:          snapshot.Status,
		Description:     snapshot.Description,
		Result:          output,
		Error:           snapshot.Error,
		OutputFile:      snapshot.OutputPath,
		OutputTruncated: truncated,
		TranscriptPath:  notification.TranscriptPath,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return RuntimeNotificationFollowUpTarget{}, false
	}
	message := "<task-notification>\n" + string(data) + "\n</task-notification>\n" +
		toolRuntimeText(i18n.KeyToolBackgroundFollowUpInstruction)
	sessionProjectDir := firstNonEmpty(notification.SessionProjectDir, snapshot.OwnerSessionProjectDir)
	return RuntimeNotificationFollowUpTarget{
		SessionID: sessionID, SessionProjectDir: sessionProjectDir,
		ProjectRoot: projectRoot, Message: message,
	}, true
}

// ResolveNotificationTarget resolves a task snapshot in the same origin scope
// used by durable notification delivery.
func (m *BackgroundTaskManager) ResolveNotificationTarget(notification agentcontract.RuntimeNotification) (agentcontract.TaskSnapshot, bool) {
	record, _, ok := m.notificationRecord(notification)
	if !ok {
		return agentcontract.TaskSnapshot{}, false
	}
	return snapshotFromRecordForNotification(record, notification), true
}

func (m *BackgroundTaskManager) notificationRecord(notification agentcontract.RuntimeNotification) (runtimestore.RuntimeTaskRecord, string, bool) {
	root := strings.TrimSpace(notification.ProjectRoot)
	if root == "" {
		return runtimestore.RuntimeTaskRecord{}, "", false
	}
	root = filepath.Clean(root)
	record, ok := runtimestore.NewRuntimeTaskStore(root).Get(notification.TaskID)
	if !ok {
		return runtimestore.RuntimeTaskRecord{}, "", false
	}
	return record, firstNonEmpty(record.OwnerProjectRoot, root), true
}

func (m *BackgroundTaskManager) deliverRuntimeNotification(ctx context.Context, task *BackgroundTask, notification agentcontract.RuntimeNotification) error {
	return m.deliverRuntimeNotificationAtOrigin(ctx, task, notification, m.originForTask(task))
}

func (m *BackgroundTaskManager) deliverRuntimeNotificationAtOrigin(ctx context.Context, task *BackgroundTask, notification agentcontract.RuntimeNotification, origin *backgroundTaskOrigin) error {
	if m == nil || strings.TrimSpace(notification.TaskID) == "" {
		return nil
	}
	ctx = origin.hookContext(ctx)
	ctx, cancel := m.managedContext(ctx)
	defer cancel()
	m.notificationMu.Lock()
	defer m.notificationMu.Unlock()

	record := runtimestore.RuntimeTaskRecord{ID: notification.TaskID}
	if origin != nil && origin.store != nil {
		if stored, ok := origin.store.Get(notification.TaskID); ok {
			record = stored
		}
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = notification.TaskID
	}
	if strings.TrimSpace(record.OwnerProjectRoot) == "" && origin != nil {
		record.OwnerProjectRoot = origin.projectRoot
	}
	if strings.TrimSpace(notification.SessionID) == "" {
		notification.SessionID = strings.TrimSpace(record.OwnerSessionID)
	}
	if strings.TrimSpace(notification.ProjectRoot) == "" {
		notification.ProjectRoot = strings.TrimSpace(record.OwnerProjectRoot)
	}
	if strings.TrimSpace(notification.SessionProjectDir) == "" {
		notification.SessionProjectDir = strings.TrimSpace(record.OwnerSessionProjectDir)
	}
	if task != nil && (strings.TrimSpace(record.Type) == "" || strings.TrimSpace(record.Status) == "") {
		task.mu.RLock()
		record = task.recordLocked()
		task.mu.RUnlock()
	}
	storedNotification, notificationSlot, notificationExists := runtimeTaskNotification(record, notification)
	if notificationExists {
		if strings.TrimSpace(storedNotification.ID) != "" {
			notification.ID = storedNotification.ID
		}
		if !storedNotification.CreatedAt.IsZero() {
			notification.CreatedAt = storedNotification.CreatedAt
		}
		if strings.TrimSpace(notification.SessionID) == "" {
			notification.SessionID = strings.TrimSpace(storedNotification.SessionID)
		}
		if strings.TrimSpace(notification.ProjectRoot) == "" {
			notification.ProjectRoot = strings.TrimSpace(storedNotification.ProjectRoot)
		}
		if strings.TrimSpace(notification.SessionProjectDir) == "" {
			notification.SessionProjectDir = strings.TrimSpace(storedNotification.SessionProjectDir)
		}
		notification.HookExecutions = append([]agentcontract.HookExecutionReceipt(nil), storedNotification.HookExecutions...)
		notification.Attempts = storedNotification.Attempts
		notification.LastError = storedNotification.LastError
		notification.SinkRequired = storedNotification.SinkRequired
		notification.ObserverRequired = storedNotification.ObserverRequired
		notification.FollowUpRequired = storedNotification.FollowUpRequired
		notification.SinkDeliveredAt = storedNotification.SinkDeliveredAt
		notification.ObserverDeliveredAt = storedNotification.ObserverDeliveredAt
		notification.FollowUpDeliveredAt = storedNotification.FollowUpDeliveredAt
		notification.DeliveredAt = storedNotification.DeliveredAt
	} else {
		notificationSlot = runtimeNotificationCurrentSlot
		archiveCurrentRuntimeNotification(&record)
	}
	if strings.TrimSpace(notification.ID) == "" {
		notification.ID = uuid.NewString()
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now().UTC()
	}
	m.mu.Lock()
	sink := m.notificationSink
	if origin != nil && origin.notificationSink != nil {
		sink = origin.notificationSink
	}
	observer := m.notificationObserver
	followUp := m.notificationFollowUp
	m.mu.Unlock()
	if notification.DeliveredAt != nil {
		return nil
	}

	if sink == nil && observer == nil && followUp == nil {
		setRuntimeTaskNotification(&record, notificationSlot, notification)
		m.persistRecordTo(origin, record)
		return nil
	}
	if sink != nil {
		notification.SinkRequired = true
	}
	if observer != nil {
		notification.ObserverRequired = true
	}
	if followUp != nil {
		notification.FollowUpRequired = true
	}

	notification.Attempts++
	notification.LastError = ""
	setRuntimeTaskNotification(&record, notificationSlot, notification)
	if err := m.persistRecordToErr(origin, record); err != nil {
		return i18n.WrapInternalError(i18n.KeyToolNotificationPersistenceFailed, err)
	}
	deliveryNotification := LocalizeRuntimeNotification(i18n.DetectOrLoadLanguage(), notification, snapshotFromRecordForNotification(record, notification))

	var deliveryErrors []error
	if notification.SinkRequired && sink != nil && notification.SinkDeliveredAt == nil {
		var err error
		if detailed, ok := sink.(interface {
			deliverRuntimeNotificationDetailed(context.Context, agentcontract.RuntimeNotification) ([]agentcontract.HookExecutionReceipt, error)
		}); ok {
			var receipts []agentcontract.HookExecutionReceipt
			receipts, err = detailed.deliverRuntimeNotificationDetailed(ctx, deliveryNotification)
			notification.HookExecutions = append(notification.HookExecutions, receipts...)
		} else {
			err = sink.DeliverRuntimeNotification(ctx, deliveryNotification)
		}
		if err != nil {
			deliveryErrors = append(deliveryErrors, err)
		} else {
			deliveredAt := time.Now().UTC()
			notification.SinkDeliveredAt = &deliveredAt
		}
	}
	if notification.ObserverRequired && observer != nil && notification.ObserverDeliveredAt == nil {
		if err := observer.DeliverRuntimeNotification(ctx, deliveryNotification); err != nil {
			deliveryErrors = append(deliveryErrors, err)
		} else {
			deliveredAt := time.Now().UTC()
			notification.ObserverDeliveredAt = &deliveredAt
		}
	}
	followUpWorkerStarted := false
	if notification.FollowUpRequired && followUp != nil && notification.FollowUpDeliveredAt == nil {
		queueKey, startWorker := m.enqueueRuntimeNotificationFollowUpLocked(origin, deliveryNotification)
		if startWorker {
			followUpWorkerStarted = m.startRuntimeNotificationFollowUpWorkerLocked(queueKey)
		}
		if startWorker && !followUpWorkerStarted {
			deliveryErrors = append(deliveryErrors, i18n.NewError(i18n.KeyToolNotificationManagerShuttingDown))
		}
	}
	if len(deliveryErrors) > 0 {
		err := errors.Join(deliveryErrors...)
		notification.LastError = err.Error()
		setRuntimeTaskNotification(&record, notificationSlot, notification)
		m.persistRecordTo(origin, record)
		return err
	}

	if !runtimeNotificationFullyDelivered(notification) {
		setRuntimeTaskNotification(&record, notificationSlot, notification)
		m.persistRecordTo(origin, record)
		return nil
	}
	deliveredAt := time.Now().UTC()
	notification.DeliveredAt = &deliveredAt
	notification.LastError = ""
	setRuntimeTaskNotification(&record, notificationSlot, notification)
	m.persistRecordTo(origin, record)
	return nil
}

func runtimeNotificationFullyDelivered(notification agentcontract.RuntimeNotification) bool {
	if !notification.SinkRequired && !notification.ObserverRequired && !notification.FollowUpRequired {
		return false
	}
	return (!notification.SinkRequired || notification.SinkDeliveredAt != nil) &&
		(!notification.ObserverRequired || notification.ObserverDeliveredAt != nil) &&
		(!notification.FollowUpRequired || notification.FollowUpDeliveredAt != nil)
}

// runtimeNotificationsShareRun distinguishes a duplicate terminal emission
// from a later attempt of a retained agent that reuses the same task ID. New
// agent notifications always carry RunID; shell tasks are single-run and use
// their TaskID identity.
func runtimeNotificationsShareRun(stored, incoming agentcontract.RuntimeNotification) bool {
	storedRunID := strings.TrimSpace(stored.RunID)
	incomingRunID := strings.TrimSpace(incoming.RunID)
	if storedRunID != "" || incomingRunID != "" {
		return storedRunID != "" && storedRunID == incomingRunID
	}
	return strings.TrimSpace(stored.TaskID) != "" && strings.TrimSpace(stored.TaskID) == strings.TrimSpace(incoming.TaskID)
}

const runtimeNotificationCurrentSlot = -1

func runtimeTaskNotification(record runtimestore.RuntimeTaskRecord, incoming agentcontract.RuntimeNotification) (agentcontract.RuntimeNotification, int, bool) {
	incomingID := strings.TrimSpace(incoming.ID)
	if record.Notification != nil && ((incomingID != "" && record.Notification.ID == incomingID) || runtimeNotificationsShareRun(*record.Notification, incoming)) {
		return *record.Notification, runtimeNotificationCurrentSlot, true
	}
	for index := range record.Notifications {
		stored := record.Notifications[index]
		if (incomingID != "" && stored.ID == incomingID) || runtimeNotificationsShareRun(stored, incoming) {
			return stored, index, true
		}
	}
	return agentcontract.RuntimeNotification{}, runtimeNotificationCurrentSlot, false
}

func archiveCurrentRuntimeNotification(record *runtimestore.RuntimeTaskRecord) {
	if record == nil || record.Notification == nil || strings.TrimSpace(record.Notification.ID) == "" {
		return
	}
	for index := range record.Notifications {
		if record.Notifications[index].ID == record.Notification.ID {
			record.Notifications[index] = *record.Notification
			record.Notification = nil
			return
		}
	}
	record.Notifications = append(record.Notifications, *record.Notification)
	record.Notification = nil
}

func setRuntimeTaskNotification(record *runtimestore.RuntimeTaskRecord, slot int, notification agentcontract.RuntimeNotification) {
	if record == nil {
		return
	}
	if slot == runtimeNotificationCurrentSlot {
		copy := notification
		record.Notification = &copy
		return
	}
	if slot >= 0 && slot < len(record.Notifications) {
		record.Notifications[slot] = notification
	}
}

func runtimeTaskNotifications(record runtimestore.RuntimeTaskRecord) []agentcontract.RuntimeNotification {
	out := append([]agentcontract.RuntimeNotification(nil), record.Notifications...)
	if record.Notification != nil {
		out = append(out, *record.Notification)
	}
	return out
}

func runtimeNotificationFollowUpQueueKey(notification agentcontract.RuntimeNotification) string {
	root := filepath.Clean(strings.TrimSpace(notification.ProjectRoot))
	sessionProjectDir := filepath.Clean(strings.TrimSpace(notification.SessionProjectDir))
	sessionID := strings.TrimSpace(notification.SessionID)
	if sessionID == "" {
		// An unresolved notification must not block unrelated ownerless work.
		sessionID = "task:" + strings.TrimSpace(notification.TaskID)
	}
	return strings.Join([]string{root, sessionProjectDir, sessionID}, "\x00")
}

func runtimeNotificationFollowUpIdentity(notification agentcontract.RuntimeNotification) string {
	return filepath.Clean(strings.TrimSpace(notification.ProjectRoot)) + "\x00" + strings.TrimSpace(notification.ID)
}

// enqueueRuntimeNotificationFollowUpLocked appends once to the owning
// conversation queue. notificationMu must be held by the caller.
func (m *BackgroundTaskManager) enqueueRuntimeNotificationFollowUpLocked(origin *backgroundTaskOrigin, notification agentcontract.RuntimeNotification) (string, bool) {
	if m.followUpQueues == nil {
		m.followUpQueues = make(map[string]*runtimeNotificationFollowUpQueue)
	}
	if m.followUpQueued == nil {
		m.followUpQueued = make(map[string]string)
	}
	queueKey := runtimeNotificationFollowUpQueueKey(notification)
	identity := runtimeNotificationFollowUpIdentity(notification)
	if existingQueue, exists := m.followUpQueued[identity]; exists {
		queue := m.followUpQueues[existingQueue]
		if queue == nil {
			delete(m.followUpQueued, identity)
		} else {
			if queue.blocked && len(queue.items) > 0 && runtimeNotificationFollowUpIdentity(queue.items[0].notification) == identity {
				// Re-admitting the durable head through ReplayPendingNotifications
				// is the explicit retry signal. Merely appending newer work does not
				// bypass or spin a failed head.
				queue.blocked = false
			}
			return existingQueue, !queue.worker && !queue.blocked
		}
	}
	queue := m.followUpQueues[queueKey]
	if queue == nil {
		queue = &runtimeNotificationFollowUpQueue{}
		m.followUpQueues[queueKey] = queue
	}
	queue.items = append(queue.items, runtimeNotificationFollowUpItem{origin: origin, notification: notification})
	m.followUpQueued[identity] = queueKey
	return queueKey, !queue.worker && !queue.blocked
}

// startRuntimeNotificationFollowUpWorkerLocked starts at most one worker for
// one owning session. notificationMu must be held by the caller.
func (m *BackgroundTaskManager) startRuntimeNotificationFollowUpWorkerLocked(queueKey string) bool {
	queue := m.followUpQueues[queueKey]
	if queue == nil || queue.worker || len(queue.items) == 0 {
		return true
	}
	queue.worker = true
	m.followUpWG.Add(1)
	if m.runManagedAsync(func() {
		defer m.followUpWG.Done()
		m.runRuntimeNotificationFollowUpQueue(queueKey)
	}) {
		return true
	}
	queue.worker = false
	m.followUpWG.Done()
	return false
}

func (m *BackgroundTaskManager) runRuntimeNotificationFollowUpQueue(queueKey string) {
	for {
		m.notificationMu.Lock()
		queue := m.followUpQueues[queueKey]
		if queue == nil || len(queue.items) == 0 {
			delete(m.followUpQueues, queueKey)
			m.notificationMu.Unlock()
			return
		}
		m.mu.Lock()
		followUp := m.notificationFollowUp
		shuttingDown := m.shuttingDown
		m.mu.Unlock()
		if followUp == nil || shuttingDown {
			queue.worker = false
			m.notificationMu.Unlock()
			return
		}
		item := queue.items[0]
		m.notificationMu.Unlock()

		deliveryCtx, cancel := m.managedContext(context.Background())
		deliveryErr := followUp.DeliverRuntimeNotification(deliveryCtx, item.notification)
		cancel()

		m.notificationMu.Lock()
		queue = m.followUpQueues[queueKey]
		if queue == nil || len(queue.items) == 0 || runtimeNotificationFollowUpIdentity(queue.items[0].notification) != runtimeNotificationFollowUpIdentity(item.notification) {
			m.notificationMu.Unlock()
			return
		}
		m.mu.Lock()
		shuttingDown = m.shuttingDown
		m.mu.Unlock()
		if shuttingDown && errors.Is(deliveryErr, context.Canceled) {
			// Manager cancellation is a process-lifecycle boundary, not a failed
			// delivery attempt. Leave the durable head pending for the next owner.
			queue.worker = false
			m.notificationMu.Unlock()
			return
		}
		if err := m.finishRuntimeNotificationFollowUpLocked(item.origin, item.notification, deliveryErr); err != nil {
			// Keep the failed head in place. Re-admitting that durable head via
			// ReplayPendingNotifications explicitly restarts the worker.
			queue.worker = false
			queue.blocked = true
			m.notificationMu.Unlock()
			return
		}
		delete(m.followUpQueued, runtimeNotificationFollowUpIdentity(item.notification))
		queue.items = queue.items[1:]
		if len(queue.items) == 0 {
			delete(m.followUpQueues, queueKey)
			m.notificationMu.Unlock()
			return
		}
		m.notificationMu.Unlock()
	}
}

// finishRuntimeNotificationFollowUpLocked writes the acknowledgement before
// the queue head is removed. notificationMu must be held by the caller.
func (m *BackgroundTaskManager) finishRuntimeNotificationFollowUpLocked(origin *backgroundTaskOrigin, delivered agentcontract.RuntimeNotification, deliveryErr error) error {
	if origin == nil || origin.store == nil {
		return fs.ErrInvalid
	}
	record, ok := origin.store.Get(delivered.TaskID)
	if !ok {
		return fs.ErrNotExist
	}
	notification, notificationSlot, found := runtimeTaskNotification(record, delivered)
	if !found || notification.ID != delivered.ID {
		return fs.ErrNotExist
	}
	if deliveryErr != nil {
		notification.LastError = deliveryErr.Error()
		setRuntimeTaskNotification(&record, notificationSlot, notification)
		if persistErr := m.persistRecordToErr(origin, record); persistErr != nil {
			return errors.Join(deliveryErr, persistErr)
		}
		return deliveryErr
	}
	deliveredAt := time.Now().UTC()
	notification.FollowUpDeliveredAt = &deliveredAt
	if runtimeNotificationFullyDelivered(notification) {
		notification.DeliveredAt = &deliveredAt
		notification.LastError = ""
	}
	setRuntimeTaskNotification(&record, notificationSlot, notification)
	return m.persistRecordToErr(origin, record)
}

func (m *BackgroundTaskManager) ReplayPendingNotifications(ctx context.Context) error {
	if m == nil {
		return nil
	}
	releaseWork, admitted := m.beginManagedWork()
	if !admitted {
		return nil
	}
	defer releaseWork()
	origin := m.currentTaskOrigin()
	if origin == nil || origin.store == nil {
		return nil
	}
	var errs []error
	var notifications []agentcontract.RuntimeNotification
	for _, record := range origin.store.List() {
		notifications = append(notifications, runtimeTaskNotifications(record)...)
	}
	// Task start time is not notification order: long-running tasks may finish
	// after shorter tasks from the same conversation. Rebuild each FIFO from
	// the durable notification creation order after a restart.
	sort.SliceStable(notifications, func(i, j int) bool {
		left, right := notifications[i], notifications[j]
		if left.CreatedAt.Equal(right.CreatedAt) {
			return left.ID < right.ID
		}
		if left.CreatedAt.IsZero() {
			return false
		}
		if right.CreatedAt.IsZero() {
			return true
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
	for _, notification := range notifications {
		if notification.DeliveredAt != nil {
			continue
		}
		if err := m.deliverRuntimeNotificationAtOrigin(ctx, nil, notification, origin); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type hookRuntimeNotificationSink struct {
	runner *hooks.Runner
}

func (s hookRuntimeNotificationSink) DeliverRuntimeNotification(ctx context.Context, notification agentcontract.RuntimeNotification) error {
	_, err := s.deliverRuntimeNotificationDetailed(ctx, notification)
	return err
}

func (s hookRuntimeNotificationSink) deliverRuntimeNotificationDetailed(ctx context.Context, notification agentcontract.RuntimeNotification) ([]agentcontract.HookExecutionReceipt, error) {
	if s.runner == nil || !s.runner.HasHooks(hooks.HookNotification) {
		return nil, i18n.NewError(i18n.KeyToolNotificationHookUnavailable)
	}
	input := hooks.CorrelateInput(ctx, hooks.HookInput{
		ToolName:    lifecycleToolNameForNotification(notification),
		SessionID:   notification.SessionID,
		ProjectRoot: notification.ProjectRoot,
		Title:       notification.Title,
		Message:     notification.Message,
		TaskID:      notification.TaskID,
		Trigger:     notification.Kind,
	})
	if strings.TrimSpace(input.WorkUnitID) == "" {
		input.WorkUnitID = notification.TaskID
	}
	executions := s.runner.RunDetailedObserved(ctx, hooks.HookNotification, input)
	recordedAt := time.Now().UTC()
	receipts := make([]agentcontract.HookExecutionReceipt, 0, len(executions))
	for _, execution := range executions {
		execution = execution.Snapshot()
		receipts = append(receipts, agentcontract.HookExecutionReceipt{
			HookType: string(hooks.HookNotification), ExecutionID: execution.ExecutionID,
			ConfigID: execution.ConfigID, ConfigIndex: execution.ConfigIndex,
			Hook: contractHook(execution.Hook), Input: contractHookInput(execution.Input), Output: contractHookOutput(execution.Output),
			RecordedAt: recordedAt,
		})
		output := execution.Output
		stderr := strings.TrimSpace(output.Stderr)
		executionError := strings.TrimSpace(output.ExecutionError)
		if stderr != "" || executionError != "" || output.ExitCode != 0 {
			reason := stderr
			if reason == "" {
				reason = executionError
			}
			return receipts, i18n.NewError(i18n.KeyToolNotificationHookFailed, execution.ExecutionID, output.ExitCode, reason)
		}
	}
	return receipts, nil
}

func contractHook(hook hooks.Hook) agentcontract.Hook {
	headers := make(map[string]string, len(hook.Headers))
	for key, value := range hook.Headers {
		headers[key] = value
	}
	return agentcontract.Hook{
		Type: string(hook.Type), Kind: string(hook.Kind), Command: hook.Command,
		Timeout: hook.Timeout, URL: hook.URL, Headers: headers,
		RetryCount: hook.RetryCount, Matcher: hook.Matcher,
	}
}

func contractHookInput(input hooks.HookInput) agentcontract.HookInput {
	toolInput := make(map[string]any, len(input.ToolInput))
	for key, value := range input.ToolInput {
		toolInput[key] = value
	}
	return agentcontract.HookInput{
		Type: string(input.Type), HookEventName: string(input.HookEventName), ToolName: input.ToolName,
		ToolUseID: input.ToolUseID, SessionID: input.SessionID, ProjectRoot: input.ProjectRoot,
		TurnID: input.TurnID, WorkUnitID: input.WorkUnitID, HookConfigID: input.HookConfigID,
		HookExecutionID: input.HookExecutionID, ToolInput: toolInput, Result: input.Result,
		Messages: append([]any(nil), input.Messages...), UserInput: input.UserInput, AgentID: input.AgentID,
		AgentType: input.AgentType, AgentTranscriptPath: input.AgentTranscriptPath,
		LastAssistantMessage: input.LastAssistantMessage, StopHookActive: input.StopHookActive,
		TeammateName: input.TeammateName, TeamName: input.TeamName, TaskID: input.TaskID,
		TaskSubject: input.TaskSubject, TaskDescription: input.TaskDescription, TaskOwner: input.TaskOwner,
		Owner: input.Owner, Trigger: input.Trigger, CustomInstructions: cloneBackgroundStringPointer(input.CustomInstructions),
		CompactSummary: input.CompactSummary, Message: input.Message, Title: input.Title,
	}
}

func cloneBackgroundStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func contractHookOutput(output hooks.HookOutput) agentcontract.HookOutput {
	modifiedInput := make(map[string]any, len(output.ModifiedInput))
	for key, value := range output.ModifiedInput {
		modifiedInput[key] = value
	}
	return agentcontract.HookOutput{
		SystemReminder: output.SystemReminder, Block: output.Block, ModifiedInput: modifiedInput,
		PreventContinuation: output.PreventContinuation, StopReason: output.StopReason,
		AdditionalContext: output.AdditionalContext, AdditionalContexts: append([]string(nil), output.AdditionalContexts...),
		PermissionBehavior: output.PermissionBehavior, NewCustomInstructions: output.NewCustomInstructions,
		UserDisplayMessage: output.UserDisplayMessage, ExitCode: output.ExitCode, ExecutionError: output.ExecutionError,
		Stdout: output.Stdout, StdoutBytes: output.StdoutBytes, StdoutTruncated: output.StdoutTruncated,
		Stderr: output.Stderr, StderrBytes: output.StderrBytes, StderrTruncated: output.StderrTruncated,
	}
}

func lifecycleToolNameForNotification(notification agentcontract.RuntimeNotification) string {
	message := strings.ToLower(notification.Message)
	if strings.Contains(message, "local_agent") || strings.Contains(message, "agent") {
		return "Agent"
	}
	return "Bash"
}

type AgentBackgroundRunner func(context.Context, io.Writer) (string, error)

// ScheduledAgentRunner executes one durably accepted scheduled prompt. The
// delivery ID and agent input are persisted before this callback starts, so a
// process restart can resume an interrupted delivery without inventing a new
// identity.
type ScheduledAgentRunner func(context.Context, string, agentcontract.Input, io.Writer) (string, error)

const scheduledDeliveryBatchPrefix = "schedule-delivery:"

// StartScheduledAgentTask durably claims a scheduled delivery before starting
// its worker. Calls with the same project root and delivery ID are idempotent:
// an active or terminal record is returned without starting another worker.
// Records interrupted by a process restart are resumed with the same task,
// delivery, and agent identities.
func (m *BackgroundTaskManager) StartScheduledAgentTask(parent context.Context, deliveryID, agentID string, input agentcontract.Input, runner ScheduledAgentRunner) (*agentcontract.TaskSnapshot, error) {
	if m == nil || runner == nil {
		return nil, fs.ErrInvalid
	}
	if parent == nil {
		parent = context.Background()
	}
	deliveryID = strings.TrimSpace(deliveryID)
	agentID = strings.TrimSpace(agentID)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if deliveryID == "" || agentID == "" || input.Prompt == "" {
		return nil, fs.ErrInvalid
	}

	m.scheduledMu.Lock()
	defer m.scheduledMu.Unlock()
	return m.startScheduledAgentTaskLocked(parent, deliveryID, agentID, input, runner)
}

// ResumeScheduledAgentTasks restarts only scheduled deliveries that the
// startup reconciler marked as interrupted. Completed work and ordinary model
// failures remain terminal and therefore cannot be replayed accidentally.
func (m *BackgroundTaskManager) ResumeScheduledAgentTasks(parent context.Context, runner ScheduledAgentRunner) error {
	if m == nil || runner == nil {
		return fs.ErrInvalid
	}
	if parent == nil {
		parent = context.Background()
	}
	origin := m.currentTaskOrigin()
	if origin == nil || origin.store == nil {
		return fs.ErrInvalid
	}
	var resumeErr error
	for _, record := range origin.store.List() {
		if record.Type != agentcontract.TaskTypeLocalAgent || record.AgentInput == nil ||
			record.TerminalReason != interruptedAgentTerminalReason ||
			!strings.HasPrefix(record.BatchID, scheduledDeliveryBatchPrefix) {
			continue
		}
		deliveryID := strings.TrimPrefix(record.BatchID, scheduledDeliveryBatchPrefix)
		if deliveryID == "" || strings.TrimSpace(record.AgentAlias) == "" {
			resumeErr = errors.Join(resumeErr, fs.ErrInvalid)
			continue
		}
		input := *record.AgentInput
		if strings.TrimSpace(input.CWD) == "" {
			input.CWD = origin.projectRoot
		}
		if _, err := m.StartScheduledAgentTask(parent, deliveryID, record.AgentAlias, input, runner); err != nil {
			resumeErr = errors.Join(resumeErr, err)
		}
	}
	return resumeErr
}

func (m *BackgroundTaskManager) startScheduledAgentTaskLocked(parent context.Context, deliveryID, agentID string, input agentcontract.Input, runner ScheduledAgentRunner) (*agentcontract.TaskSnapshot, error) {
	origin := m.scheduledTaskOrigin(input.CWD)
	if origin == nil || origin.store == nil {
		return nil, fs.ErrInvalid
	}
	input.CWD = origin.projectRoot
	batchID := scheduledDeliveryBatchPrefix + deliveryID
	taskID := scheduledTaskID(origin.projectRoot, deliveryID)

	m.mu.Lock()
	if existing := m.tasks[taskID]; existing != nil {
		snapshot := existing.snapshot()
		m.mu.Unlock()
		if snapshot.BatchID != batchID {
			return nil, fs.ErrExist
		}
		return &snapshot, nil
	}
	m.mu.Unlock()

	attempt := 1
	if record, ok := origin.store.Get(taskID); ok {
		if record.BatchID != batchID || record.AgentInput == nil {
			return nil, fs.ErrExist
		}
		if record.TerminalReason != interruptedAgentTerminalReason && runtimeAgentRecordIsActive(record) && !runtimeTaskOwnerAlive(record.OwnerPID) {
			finishedAt := time.Now().UTC()
			code := -1
			record.Status = "failed"
			record.Outcome = agentcontract.RunOutcomeInterrupted
			record.TerminalReason = interruptedAgentTerminalReason
			record.FinishedAt = &finishedAt
			record.ExitCode = &code
			record.Error = ""
			if err := origin.store.Save(record); err != nil {
				return nil, err
			}
		}
		if record.TerminalReason != interruptedAgentTerminalReason {
			snapshot := snapshotFromRecord(record)
			return &snapshot, nil
		}
		attempt = record.Attempt + 1
	}

	releaseWork, admitted := m.beginManagedWork()
	if !admitted {
		return nil, i18n.NewError(i18n.KeyToolNotificationManagerShuttingDown)
	}
	workerStarted := false
	defer func() {
		if !workerStarted {
			releaseWork()
		}
	}()
	pinBackgroundTaskOriginHookContext(origin, parent)
	if err := secureio.EnsurePrivateRuntimeDirectory(origin.outputDir); err != nil {
		return nil, i18n.WrapError(i18n.KeyToolRuntimeBackgroundOutputDirCreateFailed, err)
	}
	outputPath := origin.taskOutputPath(taskID)
	outFile, err := newRotatingFileWriter(outputPath, BackgroundOutputCapBytes)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyToolBackgroundOutputOpenFailed, err)
	}

	// Acceptance transfers ownership from the scheduler call to this manager.
	// Preserve correlation values, but do not let a scheduler rebind cancel an
	// already-durable task; BackgroundTaskManager.Shutdown still cancels it.
	taskCtx, cancel := m.managedContext(context.WithoutCancel(parent))
	startedAt := time.Now().UTC()
	inputCopy := input
	task := &BackgroundTask{
		ID: taskID, Type: agentcontract.TaskTypeLocalAgent, Status: "running",
		Description: firstNonEmpty(input.Description, input.Prompt), Command: firstNonEmpty(input.Description, input.Prompt),
		Prompt: input.Prompt, OutputPath: outputPath, StartedAt: startedAt,
		cancel: cancel, done: make(chan struct{}), Detached: true, origin: origin,
		OwnerSessionID:         backgroundTaskOwnerSessionID(parent),
		OwnerSessionProjectDir: backgroundTaskOwnerSessionProjectDir(parent),
		OwnerProjectRoot:       origin.projectRoot, OwnerPID: os.Getpid(), AgentAlias: agentID,
		AgentInput: &inputCopy, BatchID: batchID, Attempt: attempt, Outcome: agentcontract.RunOutcomeRunning,
	}
	task.mu.RLock()
	record := task.recordLocked()
	task.mu.RUnlock()
	if err := m.persistRecordToErr(origin, record); err != nil {
		cancel()
		_ = outFile.Close()
		return nil, err
	}
	m.mu.Lock()
	if existing := m.tasks[taskID]; existing != nil {
		m.mu.Unlock()
		cancel()
		_ = outFile.Close()
		snapshot := existing.snapshot()
		return &snapshot, nil
	}
	m.tasks[taskID] = task
	m.mu.Unlock()
	m.notifySnapshotSubscribers()
	m.publishTaskStarted(origin, task)

	workerStarted = true
	go func() {
		defer releaseWork()
		m.runAgentTaskWorker(taskCtx, task, outFile, func(ctx context.Context, writer io.Writer) (string, error) {
			return runner(ctx, agentID, input, writer)
		})
	}()
	snapshot := task.snapshot()
	return &snapshot, nil
}

func (m *BackgroundTaskManager) scheduledTaskOrigin(projectRoot string) *backgroundTaskOrigin {
	root := strings.TrimSpace(projectRoot)
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.currentTaskOriginLocked()
	if root == "" {
		return current
	}
	root = filepath.Clean(root)
	if current != nil && root == current.projectRoot {
		return current
	}
	return &backgroundTaskOrigin{
		projectRoot:      root,
		outputDir:        filepath.Join(root, ".luban-code", "task-output"),
		store:            runtimestore.NewRuntimeTaskStore(root),
		lifecycle:        runtimestore.NewRuntimeLifecycle(root),
		notificationSink: m.notificationSink,
	}
}

func scheduledTaskID(projectRoot, deliveryID string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(projectRoot) + "\x00" + deliveryID))
	return fmt.Sprintf("scheduled-%x", digest[:])
}

func (m *BackgroundTaskManager) StartAgentTask(parent context.Context, prompt, description string, runner AgentBackgroundRunner) (*agentcontract.TaskSnapshot, error) {
	if parent == nil {
		parent = context.Background()
	}
	releaseWork, admitted := m.beginManagedWork()
	if !admitted {
		return nil, i18n.NewError(i18n.KeyToolNotificationManagerShuttingDown)
	}
	workerStarted := false
	defer func() {
		if !workerStarted {
			releaseWork()
		}
	}()
	m.mu.Lock()
	origin := m.currentTaskOriginLocked()
	taskID := m.nextTaskIDLocked(origin)
	m.mu.Unlock()
	pinBackgroundTaskOriginHookContext(origin, parent)

	if err := secureio.EnsurePrivateRuntimeDirectory(origin.outputDir); err != nil {
		return nil, i18n.WrapError(i18n.KeyToolRuntimeBackgroundOutputDirCreateFailed, err)
	}

	outputPath := origin.taskOutputPath(taskID)
	outFile, err := newRotatingFileWriter(outputPath, BackgroundOutputCapBytes)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyToolBackgroundOutputOpenFailed, err)
	}

	taskCtx, cancel := m.managedContext(parent)
	task := &BackgroundTask{
		ID:                     taskID,
		Type:                   agentcontract.TaskTypeLocalAgent,
		Status:                 "running",
		Description:            description,
		Command:                description,
		Prompt:                 prompt,
		OutputPath:             outputPath,
		StartedAt:              time.Now().UTC(),
		cancel:                 cancel,
		done:                   make(chan struct{}),
		Detached:               true,
		origin:                 origin,
		OwnerSessionID:         backgroundTaskOwnerSessionID(parent),
		OwnerSessionProjectDir: backgroundTaskOwnerSessionProjectDir(parent),
		OwnerProjectRoot:       origin.projectRoot,
	}
	m.registerTask(task)

	workerStarted = true
	go func() {
		defer releaseWork()
		m.runAgentTaskWorker(taskCtx, task, outFile, runner)
	}()

	snap := task.snapshot()
	return &snap, nil
}

func (m *BackgroundTaskManager) runAgentTaskWorker(taskCtx context.Context, task *BackgroundTask, outFile *rotatingFileWriter, runner AgentBackgroundRunner) {
	defer close(task.done)
	result, err := runner(taskCtx, outFile)
	if strings.TrimSpace(result) != "" {
		// Best effort: if the runner produced no streaming output, append the
		// final result so callers can read something useful.
		if info, statErr := os.Stat(task.OutputPath); statErr == nil && info.Size() == 0 {
			_, _ = outFile.WriteString(result)
		}
	}

	exitCode := 0
	if err != nil {
		exitCode = -1
	}
	duration := time.Since(task.StartedAt).Milliseconds()
	_, _ = outFile.WriteString(fmt.Sprintf("\nTASK_FINISHED exit=%d duration=%d\n", exitCode, duration))
	_ = outFile.Close()

	task.mu.Lock()
	task.Result = result
	if task.Status == "killed" {
		code := -1
		task.ExitCode = &code
		finishedAt := time.Now().UTC()
		task.FinishedAt = &finishedAt
		record := task.recordLocked()
		task.mu.Unlock()
		m.persistRecordForTask(task, record)
		m.emitTaskCompletionNotification(context.Background(), task, "killed", -1)
		return
	}
	if err != nil {
		task.Status = "failed"
		if strings.HasPrefix(task.BatchID, scheduledDeliveryBatchPrefix) && errors.Is(err, context.Canceled) {
			task.Error = ""
			task.Outcome = agentcontract.RunOutcomeInterrupted
			task.TerminalReason = interruptedAgentTerminalReason
		} else {
			task.Error = err.Error()
		}
		code := -1
		task.ExitCode = &code
		finishedAt := time.Now().UTC()
		task.FinishedAt = &finishedAt
		record := task.recordLocked()
		task.mu.Unlock()
		m.persistRecordForTask(task, record)
		m.emitTaskCompletionNotification(context.Background(), task, "failed", -1)
		return
	}

	task.Status = "completed"
	code := 0
	task.ExitCode = &code
	finishedAt := time.Now().UTC()
	task.FinishedAt = &finishedAt
	record := task.recordLocked()
	task.mu.Unlock()
	m.persistRecordForTask(task, record)
	m.emitTaskCompletionNotification(context.Background(), task, "completed", 0)
}

func (m *BackgroundTaskManager) Wait(id string, timeout time.Duration) (agentcontract.TaskSnapshot, string) {
	m.mu.Lock()
	task, ok := m.tasks[id]
	store := m.store
	m.mu.Unlock()
	if !ok {
		if store != nil {
			deadline := time.Now().Add(timeout)
			for {
				record, ok := store.Get(id)
				if !ok {
					return agentcontract.TaskSnapshot{}, "missing"
				}
				snap := snapshotFromRecord(record)
				if snap.Status != "running" {
					return snap, "success"
				}
				if timeout <= 0 {
					return snap, "not_ready"
				}
				if time.Now().After(deadline) {
					return snap, "timeout"
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		return agentcontract.TaskSnapshot{}, "missing"
	}

	task.mu.RLock()
	if task.Status != "running" {
		task.mu.RUnlock()
		return task.snapshot(), "success"
	}
	done := task.done
	task.mu.RUnlock()

	if timeout <= 0 {
		return task.snapshot(), "not_ready"
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return task.snapshot(), "success"
	case <-timer.C:
		return task.snapshot(), "timeout"
	}
}

func ReadTaskOutput(path string, maxBytes int64) (BackgroundTaskOutput, error) {
	if maxBytes <= 0 {
		maxBytes = 64 * 1024
	}

	if err := validateBackgroundOutputPath(path); err != nil {
		return BackgroundTaskOutput{}, err
	}
	f, err := secureio.OpenPrivateRuntimeRegularFile(path)
	if err != nil {
		return BackgroundTaskOutput{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return BackgroundTaskOutput{}, err
	}
	if info.Size() <= maxBytes {
		data, err := io.ReadAll(f)
		if err != nil {
			return BackgroundTaskOutput{}, err
		}
		return BackgroundTaskOutput{Content: string(data)}, nil
	}

	if _, err := f.Seek(-maxBytes, io.SeekEnd); err != nil {
		return BackgroundTaskOutput{}, err
	}

	buf := make([]byte, maxBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return BackgroundTaskOutput{}, err
	}

	return BackgroundTaskOutput{
		Content:      strings.TrimLeft(string(buf[:n]), "\n"),
		WasTruncated: true,
	}, nil
}

func validateBackgroundOutputPath(path string) error {
	if strings.TrimSpace(path) == "" || filepath.Clean(path) != path {
		return fs.ErrInvalid
	}
	dir := filepath.Dir(path)
	if filepath.Ext(path) != ".output" {
		return fs.ErrInvalid
	}
	stem := strings.TrimSuffix(filepath.Base(path), ".output")
	if runtimestore.ValidateTaskID(stem) != nil || runtimestore.SafeTaskPathComponent(stem) != stem {
		return fs.ErrInvalid
	}
	return secureio.EnsurePrivateRuntimeDirectory(dir)
}
