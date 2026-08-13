package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// conversation holds per-session state for the engine.
type conversation struct {
	ql          *loop.QueryLoop
	model       string
	projectDir  string
	projectRoot string
	cwd         string

	// queryGate serializes Query/QueryFollowUp cancel+wait+setup. It stays
	// separate from mutationGate so a replacing Query can cancel the current
	// owner without waiting for that owner's mutation lease first.
	queryGate chan struct{}

	// mutationGate serializes every QueryLoop mutation and its persistence for
	// this session. Query transfers the lease to its worker goroutine; manual
	// compaction holds the same lease through ForceCompact and Save.
	mutationGate chan struct{}

	// in-flight query control
	cancel  context.CancelFunc // nil when idle
	running chan struct{}      // closed when the current query finishes; nil when idle
	deleted bool

	// authoritativeReloadRequired is set when a failed persistence transaction
	// could not be reconciled with the exact durable session namespace. The
	// QueryLoop is restored to a known pre-image for memory safety, but no later
	// mutation may sample or persist that fallback until an authoritative reload
	// succeeds. This matters when the model-context CAS committed and only a
	// metadata sidecar failed: blindly continuing from the old pre-image would
	// overwrite an already committed generation.
	authoritativeReloadRequired bool // guarded by mu

	mu sync.Mutex
}

// acquireMutation obtains the per-session mutation lease. The returned
// release function is idempotent so error cleanup and goroutine ownership
// transfer cannot accidentally over-release the semaphore.
func (c *conversation) acquireMutation(ctx context.Context) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case c.mutationGate <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-c.mutationGate })
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// conversationKey is the durable identity of an engine conversation. Session
// IDs are project-scoped in the repository, so using the bare ID here would let
// a resume, background follow-up, or deletion in one project affect another.
type conversationKey struct {
	projectDir string
	sessionID  string
}

// queryEventStream serializes event publication with terminal closure. Hook
// observers may outlive the parent query (for example, a retained Agent's
// completion Notification), so the producer that closes the public channel
// cannot rely on goroutine ownership alone.
type queryEventStream struct {
	mu     sync.Mutex
	ch     chan Event
	closed bool
}

func newQueryEventStream(buffer int) *queryEventStream {
	return &queryEventStream{ch: make(chan Event, buffer)}
}

func (s *queryEventStream) emit(ctx context.Context, event Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *queryEventStream) finish(event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	// Commit closure before publishing Final. Late observers block on mu and
	// then deterministically drop instead of racing a send against close.
	s.closed = true
	s.ch <- event
	close(s.ch)
}

// CoreEngine is the canonical Engine implementation.
type CoreEngine struct {
	cfg         Config
	providerRef *provider.ProviderRef
	permission  *permissionHandlerRef
	sessions    SessionManager
	// maxTokensExplicit prevents provider/model switches from replacing an
	// application-supplied request budget with a model default.
	maxTokensExplicit bool

	permissionUpdateMu sync.Mutex
	// providerUpdateMu serializes provider transitions while SetProvider takes
	// an exclusive lease over every live QueryLoop mutation.
	providerUpdateMu sync.Mutex
	// skillSessionTxnMu linearizes the cross-resource transition that restores
	// one bare-session override map and publishes its project conversation.
	// It is never held across Prepare and never nests inside Manager.
	skillSessionTxnMu sync.Mutex

	convsMu sync.RWMutex
	convs   map[conversationKey]*conversation
	deleted map[conversationKey]struct{}

	shutdownOnce sync.Once
	shutdownCh   chan struct{}
}

// New creates a CoreEngine from cfg. The provider field is required.
func New(cfg Config) (*CoreEngine, error) {
	maxTokensExplicit := cfg.MaxTokens > 0
	cfg.defaults()
	if cfg.Provider == nil {
		return nil, ErrNoProvider
	}

	// Adopt an explicitly shared reference or wrap a fixed provider.
	pRef, ok := cfg.Provider.(*provider.ProviderRef)
	if !ok {
		pRef = provider.NewProviderRef(cfg.Provider)
	}

	var sm SessionManager
	if cfg.Sessions != nil {
		sm = cfg.Sessions
	} else {
		repo := session.DefaultRepository()
		projectDir := ""
		if projectRoot := strings.TrimSpace(cfg.ProjectRoot); projectRoot != "" {
			projectDir = repo.ProjectDirForCWD(projectRoot)
		}
		sm = newRepositorySessionManager(repo, func() string { return projectDir })
	}

	eng := &CoreEngine{
		cfg:               cfg,
		providerRef:       pRef,
		permission:        newPermissionHandlerRef(cfg.Permission),
		sessions:          sm,
		maxTokensExplicit: maxTokensExplicit,
		convs:             make(map[conversationKey]*conversation),
		deleted:           make(map[conversationKey]struct{}),
		shutdownCh:        make(chan struct{}),
	}
	eng.publishChildPermissionHandler()
	return eng, nil
}

// ---- Engine interface -------------------------------------------------------

// Query sends a user message and returns a channel of events.
// If a query is already running for the session it is cancelled and replaced.
func (e *CoreEngine) Query(ctx context.Context, req QueryRequest) (<-chan Event, error) {
	return e.query(ctx, req, true)
}

// QueryFollowUp queues a runtime-generated turn behind any query already
// running for the session. This preserves the parent turn that launched a
// background task while still feeding the completed result back to the model.
func (e *CoreEngine) QueryFollowUp(ctx context.Context, req QueryRequest) (<-chan Event, error) {
	return e.query(ctx, req, false)
}

func (e *CoreEngine) query(ctx context.Context, req QueryRequest, replaceRunning bool) (<-chan Event, error) {
	if e.isShutdown() {
		return nil, ErrShutdown
	}
	// Internal message kinds are runtime authority, not caller input. Ordinary
	// Query calls cannot manufacture control messages; the dedicated follow-up
	// entry point assigns its one trusted kind regardless of request fields.
	if replaceRunning {
		if req.InternalKind != types.InternalMessageKindSkillInvocation || !req.InternalControlCapability.Valid() {
			req.InternalKind = ""
		}
		req.RuntimeEventID = ""
	} else {
		if req.InternalControlCapability.Valid() {
			req.InternalKind = types.InternalMessageKindBackgroundFollowUp
			req.RuntimeEventID = strings.TrimSpace(req.RuntimeEventID)
		} else {
			req.InternalKind = ""
			req.RuntimeEventID = ""
		}
	}
	if req.RuntimeEventID != "" && strings.TrimSpace(req.SessionID) != "" {
		key := e.queryConversationKey(req)
		persisted, loadErr := e.loadSessionFromProject(req.SessionID, key.projectDir)
		if loadErr == nil && hasRuntimeFollowUpEvent(persisted, req.RuntimeEventID) {
			return completedQueryEvents(req.SessionID), nil
		}
		if loadErr != nil && !errors.Is(loadErr, ErrSessionNotFound) && !errors.Is(loadErr, ErrSessionDeleted) {
			return nil, loadErr
		}
	}

	// Resolve session ID.
	if req.SessionID == "" {
		req.SessionID = uuid.New().String()
	}
	if e.usesBareSkillSessionState() {
		e.skillSessionTxnMu.Lock()
		defer e.skillSessionTxnMu.Unlock()
	}

	conv, err := e.getOrCreateConv(req)
	if err != nil {
		return nil, err
	}

	// Serialize concurrent callers: only one may cancel-and-replace at a time.
	// This closes the TOCTOU gap in the previous unlock→wait→relock pattern.
	select {
	case conv.queryGate <- struct{}{}: // acquire
		defer func() { <-conv.queryGate }() // release after cancel+wait+setup
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// User queries interrupt and replace. Runtime follow-ups wait so a task
	// completion cannot cancel the parent turn that launched it.
	conv.mu.Lock()
	if conv.deleted {
		conv.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrSessionDeleted, req.SessionID)
	}
	if conv.cancel != nil {
		if replaceRunning {
			conv.cancel()
		}
		done := conv.running
		conv.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		conv.mu.Lock()
	}
	conv.mu.Unlock()

	// The query mutates QueryLoop state asynchronously after this method
	// returns. Acquire the session mutation lease before publishing the event
	// stream, then transfer its release to the worker goroutine.
	releaseMutation, err := conv.acquireMutation(ctx)
	if err != nil {
		return nil, err
	}
	if err := e.ensureConversationAuthoritativeLocked(req.SessionID, conv); err != nil {
		releaseMutation()
		return nil, err
	}
	if req.RuntimeEventID != "" && hasRuntimeFollowUpEvent(conv.ql.Messages(), req.RuntimeEventID) {
		releaseMutation()
		return completedQueryEvents(req.SessionID), nil
	}
	rollbackState, err := conv.ql.CapturePreparedVisibleState()
	if err != nil {
		releaseMutation()
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	contextRollbackState := conv.ql.CaptureCompactionContextState()

	// Set up a new cancellable child context.
	conv.mu.Lock()
	if conv.deleted {
		conv.mu.Unlock()
		releaseMutation()
		return nil, fmt.Errorf("%w: %s", ErrSessionDeleted, req.SessionID)
	}
	queryCtx, cancel := context.WithCancel(ctx)
	doneCh := make(chan struct{})
	conv.cancel = cancel
	conv.running = doneCh
	conv.mu.Unlock()

	events := newQueryEventStream(e.cfg.EventBufferSize)

	go func() {
		// Release last: Run, persistence, terminal publication, and in-flight
		// bookkeeping all belong to the same session mutation lifecycle.
		defer releaseMutation()
		defer cancel()
		defer func() {
			conv.mu.Lock()
			conv.cancel = nil
			conv.running = nil
			conv.mu.Unlock()
			close(doneCh)
		}()

		var runErr error
		if req.InternalKind != "" {
			message := types.UserMessage(req.Message)
			if len(req.Content) > 0 {
				message = types.Message{Role: types.RoleUser, Content: append([]types.ContentBlock(nil), req.Content...)}
			}
			message.IsMeta = true
			message.InternalKind = req.InternalKind
			if req.RuntimeEventID != "" {
				message.ID = runtimeFollowUpMessageID(req.RuntimeEventID)
			}
			var sealed bool
			message, sealed = conv.ql.SealRuntimeControlMessage(req.InternalControlCapability, message)
			if !sealed {
				runErr = i18n.NewError(i18n.KeyLoopQueryControlScopeInvalid)
			} else {
				runErr = conv.ql.RunMessage(queryCtx, message, func(ev stream.Event) {
					events.emit(queryCtx, Event{SessionID: req.SessionID, Inner: ev})
				})
			}
		} else if len(req.Content) > 0 {
			runErr = conv.ql.RunWithContent(queryCtx, req.Content, func(ev stream.Event) {
				events.emit(queryCtx, Event{SessionID: req.SessionID, Inner: ev})
			})
		} else {
			runErr = conv.ql.Run(queryCtx, req.Message, func(ev stream.Event) {
				events.emit(queryCtx, Event{SessionID: req.SessionID, Inner: ev})
			})
		}

		// Persist after each query. Provider events may already have been streamed,
		// so a persistence failure is terminalized on this same stream and the
		// live model view is reconciled before the mutation lease is released.
		if saveErr := e.saveConversationLocked(req.SessionID, conv); saveErr != nil {
			saveErr = e.reconcileConversationAfterSaveFailureLocked(req.SessionID, conv, rollbackState, contextRollbackState, saveErr)
			if runErr == nil {
				runErr = saveErr
			} else {
				runErr = errors.Join(runErr, saveErr)
			}
		}
		// Final and close are one serialized transition. Delayed execution-scoped
		// observers may still run, but they can no longer write this old stream.
		events.finish(Event{
			SessionID: req.SessionID,
			Final:     true,
			Error:     runErr,
		})
	}()

	return events.ch, nil
}

func runtimeFollowUpMessageID(eventID string) string {
	return "runtime-follow-up:" + strings.TrimSpace(eventID)
}

func completedQueryEvents(sessionID string) <-chan Event {
	completed := make(chan Event, 1)
	completed <- Event{SessionID: sessionID, Final: true}
	close(completed)
	return completed
}

func hasRuntimeFollowUpEvent(messages []types.Message, eventID string) bool {
	want := runtimeFollowUpMessageID(eventID)
	for _, message := range messages {
		if message.ID == want && message.IsMeta && message.Role == types.RoleUser &&
			message.InternalKind == types.InternalMessageKindBackgroundFollowUp && message.HasInternalControlProvenance() {
			return true
		}
	}
	return false
}

// Resume loads the message history for sessionID and returns the message count.
func (e *CoreEngine) Resume(ctx context.Context, sessionID string) (count int, err error) {
	if e.isShutdown() {
		return 0, ErrShutdown
	}
	key := e.currentConversationKey(sessionID)
	msgs, err := e.loadSessionFromProject(sessionID, key.projectDir)
	if err != nil {
		return 0, err
	}
	seenToolUseIDs, err := e.loadToolUseLedgerFromProject(sessionID, key.projectDir)
	if err != nil {
		return 0, err
	}
	loadedToolNames, err := e.loadLoadedToolNamesFromProject(sessionID, key.projectDir)
	if err != nil {
		return 0, err
	}
	skillsMeta, err := e.loadSessionSkillsMetaFromProject(sessionID, key.projectDir)
	if err != nil {
		return 0, err
	}

	prepared := e.newConvWithRuntime(sessionID, "", e.defaultRuntimeContext(), key.projectDir)
	if err := e.installConversationControlScope(sessionID, prepared); err != nil {
		return 0, err
	}
	if err := restoreVisibleSkillState(prepared.ql, msgs, seenToolUseIDs, loadedToolNames, skillsMeta); err != nil {
		return 0, err
	}
	visibleState, err := prepared.ql.CapturePreparedVisibleState()
	if err != nil {
		return 0, err
	}
	e.skillSessionTxnMu.Lock()
	defer e.skillSessionTxnMu.Unlock()
	existing, err := e.preflightSkillResumeConversation(key)
	if err != nil {
		return 0, err
	}
	rollbackOverrides, err := e.restoreSessionOverrides(key, skillsMeta)
	if err != nil {
		return 0, err
	}
	overridesCommitted := false
	defer func() {
		if !overridesCommitted {
			err = errors.Join(err, rollbackOverrides())
		}
	}()

	e.convsMu.Lock()
	if existing != nil {
		existing.mu.Lock()
	}
	if _, deleted := e.deleted[key]; deleted ||
		e.usesBareSkillSessionState() && ambiguousConversationKeyLocked(e.convs, key) ||
		existing != nil && e.convs[key] != existing {
		if existing != nil {
			existing.mu.Unlock()
		}
		e.convsMu.Unlock()
		return 0, i18n.NewError(i18n.KeyEngineSessionResumeConflict, sessionID)
	}
	if existing != nil && (existing.running != nil || existing.cancel != nil || existing.deleted) {
		existing.mu.Unlock()
		e.convsMu.Unlock()
		return 0, i18n.NewError(i18n.KeyEngineSessionResumeConflict, sessionID)
	}
	if existing == nil {
		if current := e.convs[key]; current != nil {
			e.convsMu.Unlock()
			return 0, i18n.NewError(i18n.KeyEngineSessionResumeConflict, sessionID)
		}
		e.convs[key] = prepared
	} else {
		existing.ql.InstallPreparedVisibleState(visibleState)
		existing.authoritativeReloadRequired = false
	}
	if existing != nil {
		existing.mu.Unlock()
	}
	e.convsMu.Unlock()
	overridesCommitted = true
	return len(msgs), nil
}

type preparedRuntimeContextResume struct {
	engine         *CoreEngine
	key            conversationKey
	sessionID      string
	projectDir     string
	messages       []types.Message
	seenToolUseIDs []string
	skillsMeta     *session.SessionSkillsMeta
	conv           *conversation
	visibleState   loop.PreparedVisibleState
	replace        bool

	mu        sync.Mutex
	completed bool
}

func (p *preparedRuntimeContextResume) MessageCount() int { return len(p.messages) }

func (p *preparedRuntimeContextResume) Commit() error {
	return p.CommitContext(context.Background())
}

func (p *preparedRuntimeContextResume) CommitContext(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.completed {
		return i18n.NewError(i18n.KeyEngineSessionResumeCompleted)
	}
	deletedOnDisk, deletedErr := p.engine.sessionHistoryDeleted(p.sessionID, p.projectDir)
	if deletedErr != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, deletedErr)
	}
	if deletedOnDisk {
		return fmt.Errorf("%w: %s", ErrSessionDeleted, p.sessionID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.engine.skillSessionTxnMu.Lock()
	defer p.engine.skillSessionTxnMu.Unlock()
	existing, err := p.engine.preflightSkillResumeConversation(p.key)
	if err != nil {
		return err
	}
	rollbackOverrides, err := p.engine.restoreSessionOverrides(p.key, p.skillsMeta)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, rollbackOverrides())
	}
	p.engine.convsMu.Lock()
	if existing != nil {
		existing.mu.Lock()
	}
	if _, deleted := p.engine.deleted[p.key]; deleted {
		if existing != nil {
			existing.mu.Unlock()
		}
		p.engine.convsMu.Unlock()
		return errors.Join(fmt.Errorf("%w: %s", ErrSessionDeleted, p.sessionID), rollbackOverrides())
	}
	if p.engine.usesBareSkillSessionState() && ambiguousConversationKeyLocked(p.engine.convs, p.key) {
		if existing != nil {
			existing.mu.Unlock()
		}
		p.engine.convsMu.Unlock()
		return errors.Join(i18n.NewError(i18n.KeyEngineSessionAmbiguous, p.sessionID), rollbackOverrides())
	}
	if existing != nil {
		if p.engine.convs[p.key] != existing {
			existing.mu.Unlock()
			p.engine.convsMu.Unlock()
			return errors.Join(i18n.NewError(i18n.KeyEngineSessionResumeConflict, p.sessionID), rollbackOverrides())
		}
		if existing.running != nil || existing.cancel != nil || existing.deleted {
			existing.mu.Unlock()
			p.engine.convsMu.Unlock()
			return errors.Join(i18n.NewError(i18n.KeyEngineSessionResumeConflict, p.sessionID), rollbackOverrides())
		}
	}
	if existing == nil {
		if p.engine.convs[p.key] != nil {
			p.engine.convsMu.Unlock()
			return errors.Join(i18n.NewError(i18n.KeyEngineSessionResumeConflict, p.sessionID), rollbackOverrides())
		}
		p.engine.convs[p.key] = p.conv
	} else if p.replace {
		p.engine.convs[p.key] = p.conv
	} else {
		existing.ql.InstallPreparedVisibleState(p.visibleState)
		existing.authoritativeReloadRequired = false
	}
	if existing != nil {
		existing.mu.Unlock()
	}
	p.engine.convsMu.Unlock()
	p.completed = true
	return nil
}

func (p *preparedRuntimeContextResume) Abort() {
	p.mu.Lock()
	p.completed = true
	p.mu.Unlock()
}

// PrepareRuntimeContextResume loads and constructs a detached conversation.
// The active conversation map remains untouched until Commit succeeds.
func (e *CoreEngine) PrepareRuntimeContextResume(ctx context.Context, sessionID, projectDir string, runtime RuntimeContext) (PreparedRuntimeContextResume, error) {
	if e.isShutdown() {
		return nil, ErrShutdown
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	msgs, err := e.loadSessionFromProject(sessionID, projectDir)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	seenToolUseIDs, err := e.loadToolUseLedgerFromProject(sessionID, projectDir)
	if err != nil {
		return nil, err
	}
	loadedToolNames, err := e.loadLoadedToolNamesFromProject(sessionID, projectDir)
	if err != nil {
		return nil, err
	}
	key := newConversationKey(projectDir, sessionID)
	if key.projectDir == "" {
		key = e.currentConversationKey(sessionID)
	}
	skillsMeta, err := e.loadSessionSkillsMetaFromProject(sessionID, key.projectDir)
	if err != nil {
		return nil, err
	}
	conv := e.newConvWithRuntime(sessionID, "", runtime, key.projectDir)
	if err := e.installConversationControlScope(sessionID, conv); err != nil {
		return nil, err
	}
	if err := restoreVisibleSkillState(conv.ql, msgs, seenToolUseIDs, loadedToolNames, skillsMeta); err != nil {
		return nil, err
	}
	visibleState, err := conv.ql.CapturePreparedVisibleState()
	if err != nil {
		return nil, err
	}
	return &preparedRuntimeContextResume{
		engine: e, key: key, sessionID: sessionID, projectDir: key.projectDir,
		messages: msgs, seenToolUseIDs: seenToolUseIDs, skillsMeta: skillsMeta, conv: conv, visibleState: visibleState, replace: true,
	}, nil
}

func (e *CoreEngine) PrepareResume(ctx context.Context, sessionID string) (PreparedRuntimeContextResume, error) {
	if e.isShutdown() {
		return nil, ErrShutdown
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := e.currentConversationKey(sessionID)
	msgs, err := e.loadSessionFromProject(sessionID, key.projectDir)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	seenToolUseIDs, err := e.loadToolUseLedgerFromProject(sessionID, key.projectDir)
	if err != nil {
		return nil, err
	}
	loadedToolNames, err := e.loadLoadedToolNamesFromProject(sessionID, key.projectDir)
	if err != nil {
		return nil, err
	}
	skillsMeta, err := e.loadSessionSkillsMetaFromProject(sessionID, key.projectDir)
	if err != nil {
		return nil, err
	}
	conv := e.newConvWithRuntime(sessionID, "", e.defaultRuntimeContext(), key.projectDir)
	if err := e.installConversationControlScope(sessionID, conv); err != nil {
		return nil, err
	}
	if err := restoreVisibleSkillState(conv.ql, msgs, seenToolUseIDs, loadedToolNames, skillsMeta); err != nil {
		return nil, err
	}
	visibleState, err := conv.ql.CapturePreparedVisibleState()
	if err != nil {
		return nil, err
	}
	return &preparedRuntimeContextResume{engine: e, key: key, sessionID: sessionID, projectDir: key.projectDir, messages: msgs, seenToolUseIDs: seenToolUseIDs, skillsMeta: skillsMeta, conv: conv, visibleState: visibleState}, nil
}

// Compact runs context compaction on the stored session history.
func (e *CoreEngine) Compact(ctx context.Context, sessionID string, customInstructions ...string) (CompactResult, error) {
	instructions := ""
	if len(customInstructions) > 0 {
		instructions = strings.TrimSpace(customInstructions[0])
	}
	return e.compactWithEvents(ctx, sessionID, instructions, nil)
}

// CompactWithEvents runs manual compaction and forwards its structured usage
// event so interactive clients can include the provider call in session cost.
func (e *CoreEngine) CompactWithEvents(ctx context.Context, sessionID, customInstructions string, onEvent func(stream.Event)) (CompactResult, error) {
	return e.compactWithEvents(ctx, sessionID, strings.TrimSpace(customInstructions), onEvent)
}

// manualCompactionEventBuffer is the publication half of the manual
// compaction transaction. QueryLoop owns compaction and authenticates the
// boundary, but the engine owns the durable session CAS. Holding every event
// here prevents a successful boundary or terminal lifecycle event from
// becoming observable before that CAS and its required sidecars have
// completed.
type manualCompactionEventBuffer struct {
	mu     sync.Mutex
	events []stream.Event
}

func (b *manualCompactionEventBuffer) record(event stream.Event) {
	b.mu.Lock()
	b.events = append(b.events, event)
	b.mu.Unlock()
}

func (b *manualCompactionEventBuffer) snapshot() []stream.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]stream.Event(nil), b.events...)
}

func (b *manualCompactionEventBuffer) hasCompactBoundary() bool {
	for _, event := range b.snapshot() {
		if event.Type == stream.EventCompactBoundary {
			return true
		}
	}
	return false
}

func (b *manualCompactionEventBuffer) hasSuccessfulLifecycle() bool {
	for _, event := range b.snapshot() {
		if event.Type == stream.EventCompactBoundary {
			return true
		}
		if event.Type == stream.EventProgress && event.Progress != nil {
			switch event.Progress.Stage {
			case "compact_end", "compact_success", "auto_compact_success":
				return true
			}
		}
	}
	return false
}

func (b *manualCompactionEventBuffer) publish(onEvent func(stream.Event)) {
	if onEvent == nil {
		return
	}
	for _, event := range b.snapshot() {
		onEvent(event)
	}
}

// publishPersistenceFailure preserves incurred provider usage and
// non-terminal progress, but replaces the speculative success boundary/end
// with one failure terminal. The public diagnostic is semantic; the returned
// error retains the private persistence and reload causes for errors.Is/As.
func (b *manualCompactionEventBuffer) publishPersistenceFailure(onEvent func(stream.Event), persistenceErr error) {
	if onEvent == nil {
		return
	}
	events := b.snapshot()
	var terminal stream.Event
	for _, event := range events {
		if event.Type == stream.EventCompactBoundary {
			continue
		}
		if event.Type == stream.EventProgress && event.Progress != nil {
			switch event.Progress.Stage {
			case "compact_end", "compact_failed", "compact_cancelled", "compact_success", "auto_compact_success":
				if event.Progress.Stage == "compact_end" {
					terminal = event
				}
				continue
			}
		}
		if event.Type == stream.EventProviderUsage && event.Metadata["kind"] == "compaction" {
			event.Metadata = cloneManualCompactionMetadata(event.Metadata)
			event.Metadata["status"] = "failure"
		}
		onEvent(event)
	}

	if terminal.Progress == nil {
		terminal = stream.Event{Type: stream.EventProgress}
	}
	terminal.Type = stream.EventProgress
	terminal.Progress = cloneManualCompactionProgress(terminal.Progress)
	terminal.Progress.Stage = "compact_failed"
	// Protocol status token: downstream renderers localize this stage through a semantic i18n key.
	terminal.Progress.Message = "failed"
	terminal.Progress.Metadata = cloneManualCompactionMetadata(terminal.Progress.Metadata)
	terminal.Progress.Metadata["trigger"] = "manual"
	terminal.Progress.Metadata["status"] = "failed"
	terminal.Progress.Metadata["error"] = UserFacingError(i18n.DetectOrLoadLanguage(), persistenceErr)
	onEvent(terminal)
}

func cloneManualCompactionProgress(progress *stream.ProgressEvent) *stream.ProgressEvent {
	if progress == nil {
		return &stream.ProgressEvent{}
	}
	cloned := *progress
	cloned.Metadata = cloneManualCompactionMetadata(progress.Metadata)
	return &cloned
}

func cloneManualCompactionMetadata(metadata map[string]any) map[string]any {
	cloned := make(map[string]any, len(metadata)+3)
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

type contextGenerationStateReader interface {
	contextGenerationState(sessionID, projectDir string) (ContextGenerationState, error)
}

type internalControlScopeReader interface {
	internalControlScope(sessionID, projectDir string) (messagecontrol.Scope, error)
}

var errManualCompactionGenerationNotCommitted = errors.New("manual compaction context generation was not committed")

func (e *CoreEngine) compactWithEvents(ctx context.Context, sessionID, instructions string, onEvent func(stream.Event)) (outcome CompactResult, err error) {
	if e.isShutdown() {
		return outcome, ErrShutdown
	}

	e.convsMu.RLock()
	conv, ok := e.convs[e.currentConversationKey(sessionID)]
	e.convsMu.RUnlock()
	if !ok {
		return outcome, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	releaseMutation, err := conv.acquireMutation(ctx)
	if err != nil {
		return outcome, err
	}
	defer releaseMutation()

	conv.mu.Lock()
	deleted := conv.deleted
	conv.mu.Unlock()
	if deleted {
		return outcome, fmt.Errorf("%w: %s", ErrSessionDeleted, sessionID)
	}
	if err := e.ensureConversationAuthoritativeLocked(sessionID, conv); err != nil {
		return outcome, err
	}
	outcome.BeforeMessageCount = len(conv.ql.Messages())
	outcome.AfterMessageCount = outcome.BeforeMessageCount
	defer func() {
		outcome.AfterMessageCount = len(conv.ql.Messages())
		if err != nil {
			outcome.Compacted = false
			outcome.ContextGeneration = 0
		}
	}()
	generationReader, generationAware := e.sessions.(contextGenerationStateReader)
	var generationBefore ContextGenerationState
	if generationAware {
		generationBefore, err = generationReader.contextGenerationState(sessionID, conv.projectDir)
		if err != nil {
			return outcome, i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, err)
		}
	}

	// ForceCompact installs its replacement in the live QueryLoop before the
	// session CAS can publish it. Keep an opaque, reconciled pre-image so a
	// failed persistence boundary can never leave that uncommitted replacement
	// available to the next query.
	rollbackState, err := conv.ql.CapturePreparedVisibleState()
	if err != nil {
		return outcome, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	contextRollbackState := conv.ql.CaptureCompactionContextState()
	visibleMessagesBefore := conv.ql.Messages()
	var buffered manualCompactionEventBuffer
	var eventSink func(stream.Event)
	if onEvent != nil {
		eventSink = buffered.record
	}
	loopOutcome, compactLoopErr := conv.ql.ForceCompactWithInstructionsAndEvents(ctx, instructions, eventSink)
	if compactLoopErr != nil {
		// Provider/compactor failures already carry a correct failed or cancelled
		// lifecycle. They are buffered only to keep the callback serialized after
		// QueryLoop has stopped all progress producers.
		compactErr := i18n.WrapError(i18n.KeyEngineSessionCompactFailed, compactLoopErr, sessionID)
		if buffered.hasSuccessfulLifecycle() {
			// Defensive outer transaction fence: a QueryLoop failure must never
			// let a speculative local boundary/end escape even if a future local
			// cleanup path forgets to rewrite its buffered lifecycle first.
			buffered.publishPersistenceFailure(onEvent, compactErr)
		} else {
			buffered.publish(onEvent)
		}
		return outcome, compactErr
	}
	if err := e.saveConversationLocked(sessionID, conv); err != nil {
		// The CAS may have failed before publication (including a stale
		// generation) or after publishing the model context but before a metadata
		// sidecar completed. Reloading resolves both cases to the durable
		// authority. If the authority itself cannot be read, restore the known
		// committed pre-image rather than exposing the uncommitted compact view.
		reconciledErr := e.reconcileConversationAfterSaveFailureLocked(sessionID, conv, rollbackState, contextRollbackState, err)
		buffered.publishPersistenceFailure(onEvent, reconciledErr)
		return outcome, reconciledErr
	}
	if generationAware {
		generationAfter, generationErr := generationReader.contextGenerationState(sessionID, conv.projectDir)
		requiresAdvance := buffered.hasCompactBoundary() || !reflect.DeepEqual(visibleMessagesBefore, conv.ql.Messages())
		generationInvalid := generationErr != nil || !generationAfter.Persisted || generationAfter.Generation < generationBefore.Generation ||
			requiresAdvance && generationAfter.Generation <= generationBefore.Generation
		if generationInvalid {
			if generationErr == nil {
				generationErr = fmt.Errorf("%w: before=%d persisted=%t, after=%d persisted=%t",
					errManualCompactionGenerationNotCommitted,
					generationBefore.Generation, generationBefore.Persisted,
					generationAfter.Generation, generationAfter.Persisted,
				)
			}
			generationErr = i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, generationErr)
			reconciledErr := e.reconcileConversationAfterSaveFailureLocked(sessionID, conv, rollbackState, contextRollbackState, generationErr)
			buffered.publishPersistenceFailure(onEvent, reconciledErr)
			return outcome, reconciledErr
		}
		if generationAfter.Persisted {
			outcome.ContextGeneration = generationAfter.Generation
		}
	}
	outcome.Compacted = loopOutcome.Compacted
	buffered.publish(onEvent)
	return outcome, nil
}

// reconcileConversationAfterSaveFailureLocked restores the exact durable view
// before releasing the mutation lease. A stale generation always takes this
// path, as does a post-CAS sidecar failure whose error alone cannot prove
// whether the model context was published. If durable reload is unavailable,
// the known pre-image is installed and the conversation is invalidated: later
// mutations must reload successfully before they can sample that fallback.
func (e *CoreEngine) reconcileConversationAfterSaveFailureLocked(sessionID string, conv *conversation, rollbackState loop.PreparedVisibleState, contextRollbackState compact.CompactionTrackerSnapshot, saveErr error) error {
	if reloadErr := e.reloadConversationVisibleStateLocked(sessionID, conv); reloadErr != nil {
		conv.ql.InstallPreparedVisibleState(rollbackState)
		conv.ql.RestoreCompactionContextState(contextRollbackState)
		conv.mu.Lock()
		conv.authoritativeReloadRequired = true
		conv.mu.Unlock()
		return errors.Join(saveErr, reloadErr)
	}
	conv.ql.RestoreCompactionContextState(contextRollbackState)
	return saveErr
}

// ensureConversationAuthoritativeLocked is the fail-closed barrier for a
// conversation whose previous persistence outcome could not be reconciled.
// The caller holds the mutation lease. No provider call, compaction, or later
// save may proceed while the exact durable namespace remains unreadable.
func (e *CoreEngine) ensureConversationAuthoritativeLocked(sessionID string, conv *conversation) error {
	conv.mu.Lock()
	required := conv.authoritativeReloadRequired
	conv.mu.Unlock()
	if !required {
		return nil
	}
	return e.reloadConversationVisibleStateLocked(sessionID, conv)
}

func (e *CoreEngine) installConversationControlScope(sessionID string, conv *conversation) error {
	reader, ok := e.sessions.(internalControlScopeReader)
	if !ok {
		return nil
	}
	scope, err := reader.internalControlScope(sessionID, conv.projectDir)
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, err)
	}
	if !conv.ql.SetInternalControlScope(messagecontrol.Runtime(), scope) {
		return i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, session.ErrCorruptSessionHistory)
	}
	return nil
}

func (e *CoreEngine) acknowledgeConversationControlScope(sessionID string, conv *conversation) error {
	reader, ok := e.sessions.(internalControlScopeReader)
	if !ok {
		return nil
	}
	scope, err := reader.internalControlScope(sessionID, conv.projectDir)
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
	}
	if err := conv.ql.AcknowledgeCommittedControlScope(messagecontrol.Runtime(), scope); err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
	}
	return nil
}

// reloadConversationVisibleStateLocked replaces only the QueryLoop's
// model-visible state from the exact durable session namespace. The caller
// holds conv's mutation lease, so no query can observe an uncommitted pre-CAS
// image between the failed save and this authoritative replacement.
func (e *CoreEngine) reloadConversationVisibleStateLocked(sessionID string, conv *conversation) error {
	messages, err := e.loadSessionFromProject(sessionID, conv.projectDir)
	if err != nil {
		return err
	}
	seenToolUseIDs, err := e.loadToolUseLedgerFromProject(sessionID, conv.projectDir)
	if err != nil {
		return err
	}
	loadedToolNames, err := e.loadLoadedToolNamesFromProject(sessionID, conv.projectDir)
	if err != nil {
		return err
	}
	skillsMeta, err := e.loadSessionSkillsMetaFromProject(sessionID, conv.projectDir)
	if err != nil {
		return err
	}

	// Reconcile into a detached loop first. No fallible operation is allowed
	// after the live loop starts changing.
	prepared := e.newConvWithRuntime(sessionID, conv.model, e.defaultRuntimeContext(), conv.projectDir)
	if err := e.installConversationControlScope(sessionID, prepared); err != nil {
		return err
	}
	if err := restoreVisibleSkillState(prepared.ql, messages, seenToolUseIDs, loadedToolNames, skillsMeta); err != nil {
		return err
	}
	visibleState, err := prepared.ql.CapturePreparedVisibleState()
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	conv.ql.InstallPreparedVisibleState(visibleState)
	conv.mu.Lock()
	conv.authoritativeReloadRequired = false
	conv.mu.Unlock()
	return nil
}

// Interrupt cancels any in-flight query for the session.
func (e *CoreEngine) Interrupt(sessionID string) {
	e.convsMu.RLock()
	conv, ok := e.convs[e.currentConversationKey(sessionID)]
	e.convsMu.RUnlock()
	if !ok {
		return
	}

	conv.mu.Lock()
	if conv.cancel != nil {
		conv.cancel()
	}
	conv.mu.Unlock()
}

// SetModel changes the model used for future queries in a session.
func (e *CoreEngine) SetModel(sessionID string, model string) error {
	if e.isShutdown() {
		return ErrShutdown
	}
	e.cfg.Model = model
	if !e.maxTokensExplicit {
		e.cfg.MaxTokens = engineDefaultRequestMaxOutput(e.providerRef.Name(), model)
	}

	e.convsMu.RLock()
	conv, ok := e.convs[e.currentConversationKey(sessionID)]
	e.convsMu.RUnlock()
	if !ok {
		return nil
	}

	conv.mu.Lock()
	conv.model = model
	conv.ql.SetModel(model)
	if !e.maxTokensExplicit {
		conv.ql.SetMaxTokens(e.cfg.MaxTokens)
	}
	conv.mu.Unlock()
	return nil
}

// SetReasoningEffort changes the reasoning effort used for future queries in a session.
func (e *CoreEngine) SetReasoningEffort(sessionID string, effort string) error {
	if e.isShutdown() {
		return ErrShutdown
	}
	e.cfg.ReasoningEffort = effort

	e.convsMu.RLock()
	conv, ok := e.convs[e.currentConversationKey(sessionID)]
	e.convsMu.RUnlock()
	if !ok {
		return nil
	}

	conv.mu.Lock()
	conv.ql.SetReasoningEffort(effort)
	conv.mu.Unlock()
	return nil
}

// SetThinkingConfig enables or disables extended thinking for future queries in a session.
func (e *CoreEngine) SetThinkingConfig(sessionID string, enabled bool, budgetTokens int) error {
	if e.isShutdown() {
		return ErrShutdown
	}

	e.convsMu.RLock()
	conv, ok := e.convs[e.currentConversationKey(sessionID)]
	e.convsMu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	conv.mu.Lock()
	conv.ql.SetThinkingConfig(enabled, budgetTokens)
	conv.mu.Unlock()
	return nil
}

// ContextUsage returns token usage statistics for a session.
// It reads live data from the QueryLoop's internal ContextWindow so the values
// are always up-to-date (fixes the stale engine-side copy bug).
func (e *CoreEngine) ContextUsage(sessionID string) (*ContextUsageInfo, error) {
	e.convsMu.RLock()
	conv, ok := e.convs[e.currentConversationKey(sessionID)]
	e.convsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	total, usage := conv.ql.ContextUsageDetail()
	used := usage.UsedTokens
	if total == 0 {
		return &ContextUsageInfo{}, nil
	}
	remaining := total - used
	if remaining < 0 {
		remaining = 0
	}
	return &ContextUsageInfo{
		TotalTokens: total, UsedTokens: used, RemainingTokens: remaining,
		Measurement: string(usage.Measurement),
	}, nil
}

// ContextGenerationState returns an explicit durable/unpersisted state in the
// engine's current project namespace.
func (e *CoreEngine) ContextGenerationState(sessionID string) (ContextGenerationState, error) {
	return e.ContextGenerationStateForSession(sessionID, "")
}

// ContextGenerationStateForSession resolves an exact project-scoped state.
func (e *CoreEngine) ContextGenerationStateForSession(sessionID, projectDir string) (ContextGenerationState, error) {
	provider, ok := e.sessions.(interface {
		contextGenerationState(string, string) (ContextGenerationState, error)
	})
	if !ok {
		return ContextGenerationState{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	return provider.contextGenerationState(sessionID, projectDir)
}

// Tools returns the names of tools enabled in the current runtime context.
func (e *CoreEngine) Tools() []string {
	return e.cfg.Registry.EnabledNames()
}

// ToolDefinitions returns schemas for tools enabled in the current runtime
// context, including tools deferred from the current model request.
func (e *CoreEngine) ToolDefinitions() []types.ToolDefinition {
	return e.cfg.Registry.EnabledDefinitions()
}

// ResolveSkillLoadedLedger routes one Skill invocation to the exact
// conversation whose visible context can prove that body is already loaded.
// It deliberately never calls SkillManager: SkillTool invokes this callback
// from inside Manager.ResolveLatest's read transaction.
func (e *CoreEngine) ResolveSkillLoadedLedger(ctx context.Context, sessionID string, id skills.SkillID) loop.SkillLoadedLedgerState {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if e == nil || trimmedSessionID == "" || sessionID != trimmedSessionID || id.Validate() != nil {
		return loop.SkillLoadedLedgerState{}
	}
	sessionID = trimmedSessionID

	if _, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		// Model invocations must use the capability carried by the execution
		// contract. Never reinterpret caller-visible message fields here.
		return loop.SkillLoadedLedgerState{}
	}

	// Explicit UI calls have no immutable execution snapshot. They may consult
	// q.messages only while the exact current-project conversation is idle and
	// the bare session ID is globally unambiguous inside this engine.
	key := e.currentConversationKey(sessionID)
	conv, unique := e.exactUniqueSkillConversation(key)
	if !unique || conv == nil {
		return loop.SkillLoadedLedgerState{}
	}
	conv.mu.Lock()
	defer conv.mu.Unlock()
	if conv.deleted || conv.running != nil || conv.cancel != nil || conv.authoritativeReloadRequired {
		return loop.SkillLoadedLedgerState{}
	}
	return conv.ql.ResolveSkillLoadedLedger(conv.ql.Messages(), id)
}

func (e *CoreEngine) exactUniqueSkillConversation(key conversationKey) (*conversation, bool) {
	e.convsMu.RLock()
	defer e.convsMu.RUnlock()
	conv := e.convs[key]
	if conv == nil {
		return nil, false
	}
	matches := 0
	for candidate := range e.convs {
		if candidate.sessionID == key.sessionID {
			matches++
		}
	}
	return conv, matches == 1
}

func ambiguousConversationKeyLocked(conversations map[conversationKey]*conversation, key conversationKey) bool {
	for candidate := range conversations {
		if candidate.sessionID == key.sessionID && candidate.projectDir != key.projectDir {
			return true
		}
	}
	return false
}

func (e *CoreEngine) usesBareSkillSessionState() bool {
	return e.cfg.SkillManager != nil || e.cfg.SkillSessionOverrides != nil
}

func (e *CoreEngine) preflightSkillResumeConversation(key conversationKey) (*conversation, error) {
	e.convsMu.RLock()
	defer e.convsMu.RUnlock()
	if _, deleted := e.deleted[key]; deleted {
		return nil, fmt.Errorf("%w: %s", ErrSessionDeleted, key.sessionID)
	}
	if e.usesBareSkillSessionState() && ambiguousConversationKeyLocked(e.convs, key) {
		return nil, i18n.NewError(i18n.KeyEngineSessionAmbiguous, key.sessionID)
	}
	existing := e.convs[key]
	if existing == nil {
		return nil, nil
	}
	existing.mu.Lock()
	defer existing.mu.Unlock()
	if existing.running != nil || existing.cancel != nil || existing.deleted {
		return nil, i18n.NewError(i18n.KeyEngineSessionResumeConflict, key.sessionID)
	}
	return existing, nil
}

func (e *CoreEngine) restoreSessionOverrides(key conversationKey, persisted *session.SessionSkillsMeta) (func() error, error) {
	layer := e.cfg.SkillSessionOverrides
	if layer == nil {
		return func() error { return nil }, nil
	}

	// SessionOverrideLayer is keyed by bare session ID. Until that storage
	// contract itself becomes project-qualified, restoring the same ID for two
	// live project conversations would silently share policy. Fail closed.
	e.convsMu.RLock()
	_, deleted := e.deleted[key]
	if ambiguousConversationKeyLocked(e.convs, key) {
		e.convsMu.RUnlock()
		return nil, i18n.NewError(i18n.KeyEngineSessionAmbiguous, key.sessionID)
	}
	e.convsMu.RUnlock()
	if deleted {
		return nil, fmt.Errorf("%w: %s", ErrSessionDeleted, key.sessionID)
	}

	previous, err := layer.Snapshot(key.sessionID)
	if err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	var replacements map[skills.SkillID]skills.VisibilityOverride
	if persisted != nil {
		replacements = persisted.Overrides
	}
	if err := layer.ReplaceSession(key.sessionID, replacements); err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	return func() error {
		if err := layer.ReplaceSession(key.sessionID, previous); err != nil {
			return i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
		}
		return nil
	}, nil
}

// Provider returns the underlying LLM provider.
func (e *CoreEngine) Provider() provider.Provider {
	return e.providerRef.Get()
}

// SetProvider atomically replaces the active provider for all future queries.
// Existing in-flight queries keep using the provider snapshot they started with.
// It also notifies all active QueryLoops to adapt their context window to the
// new provider's capabilities (e.g. switching from 200K to 128K context).
func (e *CoreEngine) SetProvider(p provider.Provider) {
	e.providerUpdateMu.Lock()
	defer e.providerUpdateMu.Unlock()

	// Acquire every live conversation's mutation lease before publishing the
	// new provider. This lets an in-flight query finish entirely on its original
	// protocol and prevents its continuation state from racing the invalidation.
	// Repeat because a conversation may be installed while leases are acquired;
	// the final convsMu lock closes that insertion window before Swap.
	releases := make(map[*conversation]func())
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	for {
		e.convsMu.Lock()
		missing := make([]*conversation, 0)
		for _, conv := range e.convs {
			if _, held := releases[conv]; !held {
				missing = append(missing, conv)
			}
		}
		if len(missing) == 0 {
			e.providerRef.Swap(p)
			e.cfg.Model = p.ModelID()
			if !e.maxTokensExplicit {
				e.cfg.MaxTokens = engineDefaultRequestMaxOutput(p.Name(), p.ModelID())
			}
			for _, conv := range e.convs {
				conv.mu.Lock()
				conv.ql.HandleProviderChange()
				if !e.maxTokensExplicit {
					conv.ql.SetMaxTokens(e.cfg.MaxTokens)
				}
				conv.mu.Unlock()
			}
			e.convsMu.Unlock()
			return
		}
		e.convsMu.Unlock()

		for _, conv := range missing {
			release, err := conv.acquireMutation(context.Background())
			if err != nil {
				continue
			}
			releases[conv] = release
		}
	}
}

func engineDefaultRequestMaxOutput(providerName, model string) int {
	if budget := provider.DefaultRequestMaxOutput(providerName, model); budget > 0 {
		return budget
	}
	return sharedDefaultMaxOutputTokens
}

// ProviderRef returns the shared ProviderRef.
func (e *CoreEngine) ProviderRef() *provider.ProviderRef {
	return e.providerRef
}

// Sessions returns the session manager.
func (e *CoreEngine) Sessions() SessionManager {
	return e.sessions
}

// Shutdown cancels all in-flight queries, saves sessions, and marks the engine closed.
func (e *CoreEngine) Shutdown(ctx context.Context) error {
	// Startup can fail before CoreEngine construction while a typed nil is still
	// retained behind the Engine interface. Shutdown is an idempotent lifecycle
	// boundary, so an engine that was never initialized is already shut down.
	if e == nil {
		return nil
	}

	var firstErr error
	e.shutdownOnce.Do(func() {
		close(e.shutdownCh)

		// Collect all conversations under the write lock so we can safely iterate.
		e.convsMu.Lock()
		convSnapshot := make(map[conversationKey]*conversation, len(e.convs))
		for key, c := range e.convs {
			convSnapshot[key] = c
		}
		e.convsMu.Unlock()

		// Cancel all in-flight queries and wait for them to finish.
		var wg sync.WaitGroup
		for _, conv := range convSnapshot {
			conv.mu.Lock()
			cancel := conv.cancel
			done := conv.running
			conv.mu.Unlock()

			if cancel != nil {
				cancel()
			}
			if done != nil {
				wg.Add(1)
				go func(ch chan struct{}) {
					defer wg.Done()
					select {
					case <-ch:
					case <-ctx.Done():
					}
				}(done)
			}
		}
		wg.Wait()

		// Flush sessions to disk.
		for key, conv := range convSnapshot {
			if err := e.saveConversationWithContext(ctx, key.sessionID, conv); err != nil && firstErr == nil {
				firstErr = err
			}
		}

	})
	return firstErr
}

func (e *CoreEngine) DeleteSessionHistory(ctx context.Context, sessionID, projectDir string) error {
	if strings.TrimSpace(sessionID) == "" {
		return i18n.NewError(i18n.KeyEngineSessionIDRequired)
	}
	key := newConversationKey(projectDir, sessionID)
	if key.projectDir == "" {
		key = e.currentConversationKey(sessionID)
	}
	e.convsMu.Lock()
	if _, deleted := e.deleted[key]; deleted {
		e.convsMu.Unlock()
		return e.deletePersistedSessionHistory(sessionID, key.projectDir)
	}
	conv := e.convs[key]
	if conv != nil && key.projectDir == "" {
		key.projectDir = cleanProjectDir(conv.projectDir)
	}
	projectDir = key.projectDir
	e.deleted[key] = struct{}{}
	delete(e.convs, key)
	e.convsMu.Unlock()

	if conv != nil {
		conv.mu.Lock()
		conv.deleted = true
		cancel := conv.cancel
		done := conv.running
		if cancel != nil {
			cancel()
		}
		conv.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				e.restoreDeletedConversation(key, conv)
				return ctx.Err()
			}
		}
	}

	err := e.deletePersistedSessionHistory(sessionID, projectDir)
	if err != nil {
		persisted, checkErr := e.sessionHistoryDeleted(sessionID, projectDir)
		if checkErr == nil && !persisted {
			e.restoreDeletedConversation(key, conv)
		}
		if checkErr != nil {
			return i18n.WrapInternalError(i18n.KeyEngineSessionDeleteFailed, errors.Join(err, checkErr))
		}
		if errors.Is(err, ErrSessionDeleted) || errors.Is(err, ErrSessionNotFound) {
			return err
		}
		return i18n.WrapInternalError(i18n.KeyEngineSessionDeleteFailed, err)
	}
	return nil
}

func (e *CoreEngine) deletePersistedSessionHistory(sessionID, projectDir string) error {
	if scoped, ok := e.sessions.(projectScopedSessionManager); ok && strings.TrimSpace(projectDir) != "" {
		return scoped.deleteFromProject(sessionID, projectDir)
	}
	return e.sessions.Delete(sessionID)
}

func (e *CoreEngine) restoreDeletedConversation(key conversationKey, conv *conversation) {
	e.convsMu.Lock()
	delete(e.deleted, key)
	if conv != nil {
		conv.mu.Lock()
		conv.deleted = false
		conv.mu.Unlock()
		e.convs[key] = conv
	}
	e.convsMu.Unlock()
}

// ---- internal helpers -------------------------------------------------------

func (e *CoreEngine) isShutdown() bool {
	select {
	case <-e.shutdownCh:
		return true
	default:
		return false
	}
}

// getOrCreateConv returns an existing conversation or builds a fresh one,
// applying any per-request overrides (SystemPromptOverride, MaxTurns).
func (e *CoreEngine) getOrCreateConv(req QueryRequest) (*conversation, error) {
	key := e.queryConversationKey(req)
	e.convsMu.Lock()
	defer e.convsMu.Unlock()
	if e.usesBareSkillSessionState() {
		if _, exact := e.convs[key]; !exact {
			for existing := range e.convs {
				if existing.sessionID == key.sessionID && existing.projectDir != key.projectDir {
					return nil, i18n.NewError(i18n.KeyEngineSessionAmbiguous, req.SessionID)
				}
			}
		}
	}
	if _, deleted := e.deleted[key]; deleted {
		return nil, fmt.Errorf("%w: %s", ErrSessionDeleted, req.SessionID)
	}
	deletedOnDisk, deletedErr := e.sessionHistoryDeleted(req.SessionID, key.projectDir)
	if deletedErr != nil {
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, deletedErr)
	}
	if deletedOnDisk {
		e.deleted[key] = struct{}{}
		return nil, fmt.Errorf("%w: %s", ErrSessionDeleted, req.SessionID)
	}

	if conv, ok := e.convs[key]; ok {
		return conv, nil
	}
	if preparer, ok := e.sessions.(interface {
		prepareContextGeneration(sessionID, projectDir string) error
	}); ok {
		if err := preparer.prepareContextGeneration(req.SessionID, key.projectDir); err != nil {
			return nil, i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, err)
		}
	}

	// Resolve per-request overrides.
	system := e.cfg.SystemPrompt
	systemBlocks := append([]prompt.SystemPromptBlock(nil), e.cfg.SystemPromptBlocks...)
	if req.SystemPromptOverride != "" {
		system = req.SystemPromptOverride
		systemBlocks = nil
	}
	maxTurns := e.cfg.MaxTurns
	if req.MaxTurns > 0 {
		maxTurns = req.MaxTurns
	}

	runtime := e.defaultRuntimeContext()
	if req.SystemPromptOverride != "" {
		runtime.GeneratedToolPrompt = false
	}
	if strings.TrimSpace(req.ProjectRoot) != "" {
		runtime.ProjectRoot = strings.TrimSpace(req.ProjectRoot)
	}
	if strings.TrimSpace(req.CWD) != "" {
		runtime.CWD = strings.TrimSpace(req.CWD)
	}
	conv := e.buildConvWithRuntime(req.SessionID, "", system, systemBlocks, maxTurns, runtime, key.projectDir)
	if err := e.installConversationControlScope(req.SessionID, conv); err != nil {
		return nil, err
	}
	e.convs[key] = conv
	return conv, nil
}

func cleanProjectDir(projectDir string) string {
	trimmed := strings.TrimSpace(projectDir)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

func newConversationKey(projectDir, sessionID string) conversationKey {
	return conversationKey{projectDir: cleanProjectDir(projectDir), sessionID: strings.TrimSpace(sessionID)}
}

func (e *CoreEngine) currentConversationKey(sessionID string) conversationKey {
	projectDir := ""
	if scoped, ok := e.sessions.(projectScopedSessionManager); ok {
		projectDir = scoped.projectDir()
	}
	return newConversationKey(projectDir, sessionID)
}

func (e *CoreEngine) queryConversationKey(req QueryRequest) conversationKey {
	if projectDir := strings.TrimSpace(req.SessionProjectDir); projectDir != "" {
		return newConversationKey(projectDir, req.SessionID)
	}
	projectDir := ""
	if resolver, ok := e.sessions.(interface{ projectDirForRoot(string) string }); ok {
		root := strings.TrimSpace(req.ProjectRoot)
		if root != "" {
			projectDir = resolver.projectDirForRoot(root)
		}
	}
	if strings.TrimSpace(projectDir) == "" {
		return e.currentConversationKey(req.SessionID)
	}
	return newConversationKey(projectDir, req.SessionID)
}

func sameRuntimeWorkspace(left, right string) bool {
	canonical := func(value string) string {
		value = strings.TrimSpace(value)
		if value == "" {
			return ""
		}
		if abs, err := filepath.Abs(value); err == nil {
			value = abs
		}
		value = filepath.Clean(value)
		if resolved, err := filepath.EvalSymlinks(value); err == nil {
			value = filepath.Clean(resolved)
		}
		return value
	}
	return canonical(left) != "" && canonical(left) == canonical(right)
}

func (e *CoreEngine) defaultRuntimeContext() RuntimeContext {
	return RuntimeContext{
		SystemPrompt:        e.cfg.SystemPrompt,
		SystemPromptBlocks:  append([]prompt.SystemPromptBlock(nil), e.cfg.SystemPromptBlocks...),
		UserContext:         e.cfg.UserContext,
		VisibleTools:        e.cfg.VisibleTools,
		ToolPromptConfig:    e.cfg.ToolPromptConfig,
		GeneratedToolPrompt: e.cfg.GeneratedToolPrompt,
		HookRunner:          e.cfg.HookRunner,
		ProjectRoot:         e.cfg.ProjectRoot,
		CWD:                 e.cfg.CWD,
	}
}

func (e *CoreEngine) newConvWithRuntime(sessionID, model string, runtime RuntimeContext, projectDir string) *conversation {
	return e.buildConvWithRuntime(sessionID, model, runtime.SystemPrompt, runtime.SystemPromptBlocks, e.cfg.MaxTurns, runtime, projectDir)
}

func (e *CoreEngine) buildConvWithRuntime(sessionID, model, system string, systemBlocks []prompt.SystemPromptBlock, maxTurns int, runtime RuntimeContext, projectDir string) *conversation {
	if strings.TrimSpace(projectDir) == "" {
		if scoped, ok := e.sessions.(projectScopedSessionManager); ok {
			projectDir = scoped.projectDir()
		}
	}
	if model == "" {
		model = e.cfg.Model
	}
	if model == "" {
		model = e.providerRef.Get().ModelID()
	}

	// Auto-detect context window from model lookup table when MaxContextTokens
	// is not explicitly configured (P2 fix).
	maxCtx := e.cfg.MaxContextTokens
	if maxCtx <= 0 {
		maxCtx = provider.LookupMaxContext(model)
	}

	// Pass the ProviderRef as the provider. Since ProviderRef implements
	// the Provider interface, loop.New() accepts it directly. This means
	// all CreateStream calls inside the loop go through the ref, and a
	// runtime Swap() takes effect between queries automatically.
	goalRuntime := newSessionGoalRuntime(e.sessions, sessionID, projectDir)
	var goalEvaluator loop.GoalEvaluator
	if goalRuntime != nil {
		goalEvaluator = e.cfg.GoalEvaluator
		if goalEvaluator == nil {
			goalEvaluator = loop.NewProviderGoalEvaluatorWithModelAndServiceTier(e.providerRef, model, e.cfg.ServiceTier)
		}
	}
	ql := loop.New(e.providerRef, e.cfg.Registry, loop.Config{
		Model:               model,
		System:              system,
		SystemBlocks:        append([]prompt.SystemPromptBlock(nil), systemBlocks...),
		UserContext:         runtime.UserContext,
		VisibleTools:        runtime.VisibleTools,
		ToolPromptConfig:    runtime.ToolPromptConfig,
		GeneratedToolPrompt: runtime.GeneratedToolPrompt,
		GoalRuntime:         goalRuntime,
		GoalEvaluator:       goalEvaluator,
		MaxTokens:           e.cfg.MaxTokens,
		TaskBudget:          e.cfg.TaskBudget,
		MaxTurns:            maxTurns,
		MaxContextTokens:    maxCtx,
		MaxOutputTokens:     e.cfg.MaxTokens, // pass to ContextWindow for output reservation
		ProgressiveContext:  e.cfg.ProgressiveContext,
		HookRunner:          runtime.HookRunner,
		SessionID:           sessionID,
		CacheLineageID:      e.cacheLineageIDForProject(sessionID, projectDir),
		SessionProjectDir:   projectDir,
		ProjectRoot:         runtime.ProjectRoot,
		CWD:                 runtime.CWD,
		TranscriptPath:      e.transcriptPathForProject(sessionID, projectDir),
		TranscriptPathResolver: func() string {
			return e.transcriptPathForProject(sessionID, projectDir)
		},
		ReasoningEffort:   e.cfg.ReasoningEffort,
		ServiceTier:       e.cfg.ServiceTier,
		PinnedModel:       e.cfg.PinnedModel,
		PermissionHandler: e.permission,
		SkillManager:      e.cfg.SkillManager,
		PlanState:         e.cfg.PlanState,
		BackgroundTasks:   e.cfg.BackgroundTasks,
		MCPState:          e.cfg.MCPState,
		AgentDefinitions:  e.cfg.AgentDefinitions,
	})

	// Inject ResultStore: persist oversized tool results to disk so they don't
	// bloat the context window. Keep artifacts beside the transcript when the
	// session manager can resolve a per-session artifacts directory.
	sessionDir := e.artifactsDirForProject(sessionID, projectDir)
	if sessionDir != "" {
		ql.SetResultStore(compact.NewResultStore(sessionDir))
	}

	return &conversation{
		ql:           ql,
		model:        model,
		projectDir:   strings.TrimSpace(projectDir),
		projectRoot:  strings.TrimSpace(runtime.ProjectRoot),
		cwd:          strings.TrimSpace(runtime.CWD),
		queryGate:    make(chan struct{}, 1),
		mutationGate: make(chan struct{}, 1),
	}
}

type projectScopedSessionManager interface {
	projectDir() string
	loadFromProject(sessionID, projectDir string) ([]types.Message, error)
	saveToProject(sessionID, projectDir string, messages []types.Message) error
	saveConversationMetaToProject(sessionID, projectDir string, meta session.SessionMeta) error
	loadToolUseLedgerFromProject(sessionID, projectDir string) ([]string, error)
	loadLoadedToolNamesFromProject(sessionID, projectDir string) ([]string, error)
	loadSkillsMetaFromProject(sessionID, projectDir string) (*session.SessionSkillsMeta, error)
	deleteFromProject(sessionID, projectDir string) error
	isDeletedInProject(sessionID, projectDir string) (bool, error)
	artifactsDirForProject(sessionID, projectDir string) string
	transcriptPathForProject(sessionID, projectDir string) string
}

type sessionSkillsMetaStore interface {
	saveSkillsMetaToProject(sessionID, projectDir string, skillsMeta *session.SessionSkillsMeta) error
	loadSkillsMetaFromProject(sessionID, projectDir string) (*session.SessionSkillsMeta, error)
}

type sessionCacheLineageStore interface {
	loadCacheLineageIDFromProject(sessionID, projectDir string) (string, error)
}

func (e *CoreEngine) cacheLineageIDForProject(sessionID, projectDir string) string {
	fallback := strings.TrimSpace(sessionID)
	store, ok := e.sessions.(sessionCacheLineageStore)
	if !ok {
		return fallback
	}
	lineageID, err := store.loadCacheLineageIDFromProject(sessionID, projectDir)
	if err != nil {
		return fallback
	}
	if lineageID = strings.TrimSpace(lineageID); lineageID != "" {
		return lineageID
	}
	return fallback
}

func (e *CoreEngine) sessionHistoryDeleted(sessionID, projectDir string) (bool, error) {
	if scoped, ok := e.sessions.(projectScopedSessionManager); ok && strings.TrimSpace(projectDir) != "" {
		return scoped.isDeletedInProject(sessionID, projectDir)
	}
	if checker, ok := e.sessions.(interface {
		isDeleted(string) (bool, error)
	}); ok {
		return checker.isDeleted(sessionID)
	}
	return false, nil
}

func (e *CoreEngine) saveConversationWithContext(ctx context.Context, sessionID string, conv *conversation) error {
	releaseMutation, err := conv.acquireMutation(ctx)
	if err != nil {
		return err
	}
	defer releaseMutation()
	if err := e.ensureConversationAuthoritativeLocked(sessionID, conv); err != nil {
		return err
	}
	return e.saveConversationLocked(sessionID, conv)
}

// saveConversationLocked persists one coherent QueryLoop snapshot while the
// caller holds conv's mutation lease.
func (e *CoreEngine) saveConversationLocked(sessionID string, conv *conversation) error {
	if scoped, ok := e.sessions.(projectScopedSessionManager); ok && strings.TrimSpace(conv.projectDir) != "" {
		if err := scoped.saveToProject(sessionID, conv.projectDir, conv.ql.Messages()); err != nil {
			return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
		}
		if err := e.acknowledgeConversationControlScope(sessionID, conv); err != nil {
			return err
		}
		skillsMeta, err := e.conversationSkillsMeta(sessionID, conv)
		if err != nil {
			return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
		}
		p := e.providerRef.Get()
		meta := session.SessionMeta{
			SeenToolUseIDs:  append([]string(nil), conv.ql.SeenToolUseIDs()...),
			LoadedToolNames: append([]string(nil), conv.ql.LoadedToolNames()...),
			Skills:          skillsMeta,
			CWD:             strings.TrimSpace(conv.cwd),
			GitBranch:       currentGitBranch(strings.TrimSpace(conv.cwd)),
			Provider:        p.Name(),
			Model:           conv.model,
		}
		if err := scoped.saveConversationMetaToProject(sessionID, conv.projectDir, meta); err != nil {
			return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
		}
		return nil
	}
	if err := e.sessions.Save(sessionID, conv.ql.Messages()); err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
	}
	if err := e.acknowledgeConversationControlScope(sessionID, conv); err != nil {
		return err
	}
	if store, ok := e.sessions.(SessionToolUseLedgerStore); ok {
		if err := store.SaveToolUseLedger(sessionID, conv.ql.SeenToolUseIDs()); err != nil {
			return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
		}
	}
	if store, ok := e.sessions.(SessionLoadedToolStore); ok {
		if err := store.SaveLoadedToolNames(sessionID, conv.ql.LoadedToolNames()); err != nil {
			return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
		}
	}
	if err := e.saveConversationSkills(sessionID, conv); err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
	}
	if err := e.saveConversationContext(sessionID, conv); err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
	}
	if err := e.saveConversationProviderModel(sessionID, conv); err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, err)
	}
	return nil
}

func (e *CoreEngine) saveConversationSkills(sessionID string, conv *conversation) error {
	store, ok := e.sessions.(sessionSkillsMetaStore)
	if !ok || conv == nil || conv.ql == nil {
		return nil
	}
	meta, err := e.conversationSkillsMeta(sessionID, conv)
	if err != nil {
		return err
	}
	if err := store.saveSkillsMetaToProject(sessionID, conv.projectDir, meta); err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	return nil
}

func (e *CoreEngine) conversationSkillsMeta(sessionID string, conv *conversation) (*session.SessionSkillsMeta, error) {
	if conv == nil || conv.ql == nil {
		return nil, nil
	}
	messages := conv.ql.Messages()
	visible := conv.ql.VisibleSkillCatalogState(messages)
	meta, err := sessionSkillsMetaFromRuntime(visible)
	if err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	if e.cfg.SkillSessionOverrides != nil {
		overrides, snapshotErr := e.cfg.SkillSessionOverrides.Snapshot(sessionID)
		if snapshotErr != nil {
			return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, snapshotErr)
		}
		meta.Overrides = overrides
	}
	if err := meta.Validate(); err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	return meta, nil
}

func (e *CoreEngine) loadSessionSkillsMetaFromProject(sessionID, projectDir string) (*session.SessionSkillsMeta, error) {
	store, ok := e.sessions.(sessionSkillsMetaStore)
	if !ok {
		return nil, nil
	}
	meta, err := store.loadSkillsMetaFromProject(sessionID, projectDir)
	if err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	if meta == nil {
		return nil, nil
	}
	if err := meta.Validate(); err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	cloned := meta.Clone()
	return &cloned, nil
}

func sessionSkillsMetaFromRuntime(state loop.SkillCatalogRuntimeState) (*session.SessionSkillsMeta, error) {
	if err := state.Validate(); err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	meta := &session.SessionSkillsMeta{ContextEpoch: state.ContextEpoch}
	if !state.Cursor.Empty() {
		meta.AnnouncedRevision = state.Cursor.AnnouncedRevision()
		meta.AnnouncedEntries = make(map[skills.SkillID]session.SessionCatalogEntryDigest)
		for _, row := range state.Cursor.AnnouncedSnapshot.Skills {
			if !row.ModelVisible {
				continue
			}
			encoded, err := json.Marshal(row)
			if err != nil {
				return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
			}
			digest := sha256.Sum256(encoded)
			meta.AnnouncedEntries[row.ID] = session.SessionCatalogEntryDigest(fmt.Sprintf("sha256:%x", digest))
		}
	}
	if len(state.LoadedDigests) > 0 {
		meta.LoadedDigests = make(map[skills.SkillID]session.SessionLoadedSkillDigest, len(state.LoadedDigests))
		for id, loaded := range state.LoadedDigests {
			meta.LoadedDigests[id] = session.SessionLoadedSkillDigest{
				ContentDigest: loaded.ContentDigest,
				PayloadDigest: loaded.PayloadDigest,
			}
		}
	}
	if err := meta.Validate(); err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	return meta, nil
}

func sessionVisibleStateFromRuntime(state loop.SkillCatalogRuntimeState) (session.SessionSkillsVisibleState, error) {
	meta, err := sessionSkillsMetaFromRuntime(state)
	if err != nil {
		return session.SessionSkillsVisibleState{}, err
	}
	return session.SessionSkillsVisibleState{
		ContextEpoch:      meta.ContextEpoch,
		AnnouncedRevision: meta.AnnouncedRevision,
		AnnouncedEntries:  meta.AnnouncedEntries,
		LoadedDigests:     meta.LoadedDigests,
	}, nil
}

func restoreVisibleSkillState(q *loop.QueryLoop, messages []types.Message, seenToolUseIDs, loadedToolNames []string, persisted *session.SessionSkillsMeta) error {
	q.SetMessagesWithRuntimeLedgers(messages, seenToolUseIDs, loadedToolNames)
	visibleRuntime := q.VisibleSkillCatalogState(messages)
	if persisted != nil && persisted.ContextEpoch != 0 && len(persisted.LoadedDigests) > 0 {
		// A post-compact standalone body carries its source epoch in Message.ID.
		// Seed only the persisted digests, never a cursor/revision, then require
		// the pure loop reconciler to prove every entry against exact visible
		// body, payload, canonical identity, and epoch provenance. A mismatch
		// yields an empty ledger and the next request emits a full catalog.
		seed := loop.SkillCatalogRuntimeState{
			ContextEpoch:  persisted.ContextEpoch,
			LoadedDigests: make(map[skills.SkillID]loop.SkillLoadedLedgerEntry, len(persisted.LoadedDigests)),
		}
		for id, loaded := range persisted.LoadedDigests {
			seed.LoadedDigests[id] = loop.SkillLoadedLedgerEntry{
				ContentDigest: loaded.ContentDigest,
				PayloadDigest: loaded.PayloadDigest,
			}
		}
		persistedEpochVisible := q.ReconcileVisibleSkillCatalogState(messages, seed)
		if len(persistedEpochVisible.LoadedDigests) > 0 {
			visibleRuntime = persistedEpochVisible
		}
	}
	visible, err := sessionVisibleStateFromRuntime(visibleRuntime)
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	reconciled, err := session.ReconcileSessionSkillsMeta(persisted, visible)
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}

	// The loop runtime state is the trust root here: it was reconstructed from
	// exact visible messages in the new context epoch. SessionSkillsMeta can
	// only remove evidence; it can never manufacture the cursor snapshots that
	// the loop needs to emit a safe delta.
	next := visibleRuntime.Clone()
	if reconciled.AnnouncedRevision == 0 {
		next.Cursor = loop.SkillCatalogCursor{}
	}
	if len(reconciled.LoadedDigests) == 0 {
		next.LoadedDigests = nil
	} else {
		filtered := make(map[skills.SkillID]loop.SkillLoadedLedgerEntry, len(reconciled.LoadedDigests))
		for id := range reconciled.LoadedDigests {
			if loaded, ok := visibleRuntime.LoadedDigests[id]; ok {
				filtered[id] = loaded
			}
		}
		next.LoadedDigests = filtered
	}
	if err := q.SetSkillCatalogState(next); err != nil {
		return i18n.WrapInternalError(i18n.KeyEngineSessionSkillStateFailed, err)
	}
	return nil
}

func (e *CoreEngine) loadToolUseLedgerFromProject(sessionID, projectDir string) ([]string, error) {
	if scoped, ok := e.sessions.(projectScopedSessionManager); ok && strings.TrimSpace(projectDir) != "" {
		ledger, err := scoped.loadToolUseLedgerFromProject(sessionID, projectDir)
		if err != nil {
			return nil, i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, err)
		}
		return ledger, nil
	}
	if store, ok := e.sessions.(SessionToolUseLedgerStore); ok {
		ledger, err := store.LoadToolUseLedger(sessionID)
		if err != nil {
			return nil, i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, err)
		}
		return ledger, nil
	}
	return nil, nil
}

func (e *CoreEngine) loadLoadedToolNamesFromProject(sessionID, projectDir string) ([]string, error) {
	if scoped, ok := e.sessions.(projectScopedSessionManager); ok && strings.TrimSpace(projectDir) != "" {
		loaded, err := scoped.loadLoadedToolNamesFromProject(sessionID, projectDir)
		if err != nil {
			return nil, i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, err)
		}
		return loaded, nil
	}
	if store, ok := e.sessions.(SessionLoadedToolStore); ok {
		loaded, err := store.LoadLoadedToolNames(sessionID)
		if err != nil {
			return nil, i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, err)
		}
		return loaded, nil
	}
	return nil, nil
}

func (e *CoreEngine) saveConversationContext(sessionID string, conv *conversation) error {
	cwd := strings.TrimSpace(conv.cwd)
	if saver, ok := e.sessions.(SessionContextSaver); ok {
		return saver.SaveSessionContext(sessionID, cwd, currentGitBranch(cwd))
	}
	return nil
}

func (e *CoreEngine) saveConversationProviderModel(sessionID string, conv *conversation) error {
	p := e.providerRef.Get()
	if saver, ok := e.sessions.(SessionMetaSaver); ok {
		return saver.SaveProviderModel(sessionID, p.Name(), conv.model)
	}
	return nil
}

func (e *CoreEngine) loadSessionFromProject(sessionID, projectDir string) ([]types.Message, error) {
	var (
		messages []types.Message
		err      error
	)
	if scoped, ok := e.sessions.(projectScopedSessionManager); ok && strings.TrimSpace(projectDir) != "" {
		messages, err = scoped.loadFromProject(sessionID, projectDir)
	} else {
		messages, err = e.sessions.Load(sessionID)
	}
	if err == nil || errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrSessionDeleted) {
		return messages, err
	}
	return nil, i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, err)
}

func (e *CoreEngine) transcriptPathForProject(sessionID, projectDir string) string {
	if scoped, ok := e.sessions.(projectScopedSessionManager); ok && strings.TrimSpace(projectDir) != "" {
		return scoped.transcriptPathForProject(sessionID, projectDir)
	}
	return e.transcriptPath(sessionID)
}

func (e *CoreEngine) artifactsDirForProject(sessionID, projectDir string) string {
	if scoped, ok := e.sessions.(projectScopedSessionManager); ok && strings.TrimSpace(projectDir) != "" {
		if dir := scoped.artifactsDirForProject(sessionID, projectDir); dir != "" {
			return dir
		}
	}
	if provider, ok := e.sessions.(SessionArtifactsDirProvider); ok {
		if dir := provider.ArtifactsDir(sessionID); dir != "" {
			return dir
		}
	}
	return ""
}

func (e *CoreEngine) transcriptPath(sessionID string) string {
	if provider, ok := e.sessions.(SessionTranscriptPathProvider); ok {
		return strings.TrimSpace(provider.TranscriptPath(sessionID))
	}
	return ""
}

// SetPermission atomically replaces the authority used by subsequent parent
// and child permission checks, including already-created conversations.
func (e *CoreEngine) SetPermission(h permission.PermissionHandler) {
	e.permissionUpdateMu.Lock()
	defer e.permissionUpdateMu.Unlock()
	e.convsMu.Lock()
	e.cfg.Permission = h
	e.convsMu.Unlock()
	e.permission.Set(h)
	e.publishChildPermissionHandler()
}

func (e *CoreEngine) publishChildPermissionHandler() {
	reg := e.cfg.Registry
	if reg == nil {
		return
	}
	childHandler := permission.PermissionHandler(e.permission)
	for _, tool := range reg.All() {
		if setter, ok := tool.(interface {
			SetChildPermissionHandler(permission.PermissionHandler)
		}); ok {
			setter.SetChildPermissionHandler(childHandler)
		}
	}
}

// UpdateRuntimeContext retargets future conversations to the active workspace.
// Existing conversations keep the configuration they were created with.
func (e *CoreEngine) UpdateRuntimeContext(ctx RuntimeContext) {
	e.convsMu.Lock()
	defer e.convsMu.Unlock()
	e.cfg.SystemPrompt = ctx.SystemPrompt
	e.cfg.SystemPromptBlocks = append([]prompt.SystemPromptBlock(nil), ctx.SystemPromptBlocks...)
	e.cfg.UserContext = ctx.UserContext
	e.cfg.VisibleTools = ctx.VisibleTools
	e.cfg.ToolPromptConfig = ctx.ToolPromptConfig
	e.cfg.GeneratedToolPrompt = ctx.GeneratedToolPrompt
	e.cfg.HookRunner = ctx.HookRunner
	e.cfg.ProjectRoot = ctx.ProjectRoot
	e.cfg.CWD = ctx.CWD
}

// RebindWorkspaceRuntime authorizes and queues a worktree enter/exit for the
// exact running conversation that invoked the tool. Its conversationKey and
// projectDir remain unchanged; only runtime workspace fields advance. The
// active QueryLoop keeps its per-run config/generation snapshot, while the
// queued update becomes visible before the next Run snapshot.
func (e *CoreEngine) RebindWorkspaceRuntime(ctx context.Context, sessionID string, runtime RuntimeContext) error {
	if e == nil || e.isShutdown() {
		return ErrShutdown
	}
	exec, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	boundSessionID, boundSessionProjectDir, _, _, active := exec.ActiveRuntimeOwnerIdentity()
	if !ok || !active || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(boundSessionID) != strings.TrimSpace(sessionID) {
		return ErrWorkspaceRebindUnauthorized
	}
	if strings.TrimSpace(runtime.ProjectRoot) == "" || strings.TrimSpace(runtime.CWD) == "" || !sameRuntimeWorkspace(runtime.ProjectRoot, runtime.CWD) {
		return ErrWorkspaceRebindUnauthorized
	}
	key := newConversationKey(boundSessionProjectDir, boundSessionID)

	e.convsMu.Lock()
	defer e.convsMu.Unlock()
	conv := e.convs[key]
	if conv == nil {
		return ErrWorkspaceRebindUnauthorized
	}

	conv.mu.Lock()
	defer conv.mu.Unlock()
	if e.convs[key] != conv || conv.deleted || conv.running == nil || conv.cancel == nil || !conv.ql.OwnsToolExecution(exec) {
		return ErrWorkspaceRebindUnauthorized
	}
	conv.ql.QueueWorkspaceRuntime(loop.WorkspaceRuntimeUpdate{
		System: runtime.SystemPrompt, SystemBlocks: runtime.SystemPromptBlocks, UserContext: runtime.UserContext,
		VisibleTools: runtime.VisibleTools, ToolPromptConfig: runtime.ToolPromptConfig, GeneratedToolPrompt: runtime.GeneratedToolPrompt,
		HookRunner:  runtime.HookRunner,
		ProjectRoot: runtime.ProjectRoot, CWD: runtime.CWD,
	})
	conv.projectRoot = strings.TrimSpace(runtime.ProjectRoot)
	conv.cwd = strings.TrimSpace(runtime.CWD)

	// Retarget defaults for any future conversation created after the active
	// worktree transition. The existing conversation keeps its exact store key.
	e.cfg.SystemPrompt = runtime.SystemPrompt
	e.cfg.SystemPromptBlocks = append([]prompt.SystemPromptBlock(nil), runtime.SystemPromptBlocks...)
	e.cfg.UserContext = runtime.UserContext
	e.cfg.VisibleTools = runtime.VisibleTools
	e.cfg.ToolPromptConfig = runtime.ToolPromptConfig
	e.cfg.GeneratedToolPrompt = runtime.GeneratedToolPrompt
	e.cfg.HookRunner = runtime.HookRunner
	e.cfg.ProjectRoot = runtime.ProjectRoot
	e.cfg.CWD = runtime.CWD
	return nil
}

// SetSystemPrompt updates the default prompt used to create future
// conversations. Existing conversations keep the prompt they were created with.
func (e *CoreEngine) SetSystemPrompt(systemPrompt prompt.SystemPrompt) {
	e.convsMu.Lock()
	defer e.convsMu.Unlock()
	e.cfg.SystemPrompt = systemPrompt.JoinedText()
	e.cfg.SystemPromptBlocks = append([]prompt.SystemPromptBlock(nil), systemPrompt...)
	e.cfg.GeneratedToolPrompt = false
}

func currentGitBranch(cwd string) string {
	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" {
		return ""
	}
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = trimmed
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
