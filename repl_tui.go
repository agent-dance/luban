package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/i18n"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/cost"
	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/tui"
	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
)

// TUIREPLConfig holds everything the TUI-based REPL loop needs.
type TUIREPLConfig struct {
	Engine              engine.Engine
	Repo                *session.Repository
	SessionID           *string
	SessionProjectDir   *string
	CWD                 *string
	HookRunnerRef       **hooks.Runner
	SwitchSession       func(context.Context, commands.SessionListEntry) error
	PublishSessionID    func(string)
	OpenSessionTerminal func(context.Context, string, string, string, string) error
	CurrentModel        func() string
	MCPBackend          commands.MCPBackend
	SkillManager        commands.SkillsBackend
	SkillInvoker        commands.SkillInvoker
	SkillsMenuLauncher  tui.SkillsMenuLauncher
	ReasoningEffort     string
	BuildDiagnostic     func(string) buildinfo.Diagnostic

	// Provider support (Phase 4)
	ProviderRef      *provider.ProviderRef
	ProviderRegistry *provider.ProviderRegistry
	CredentialStore  *provider.CredentialStore

	// Mode switching support (Shift+Tab)
	PermChecker         *permissions.Checker
	PlanState           *tools.PlanState
	AskUserQuestionTool *tools.AskUserQuestionTool
	RuntimeScope        *tools.RuntimeScope
	TaskCreateTool      *tools.TaskCreateTool
	AgentTool           *tools.AgentTool
	BackgroundTasks     *tools.BackgroundTaskManager
	RestoreSessionUsage func(tui.SessionUsage)
	FailClosed          func(error)

	SessionTransitionMu *sync.Mutex
	CommandMu           *sync.Mutex
}

type tuiInputAdmissionKey struct{}

func terminalTUIProviderStatus(runErr error, requestCompleted bool) (tui.ProviderStatus, bool) {
	if runErr == nil {
		return tui.StatusConnected, true
	}
	if errors.Is(runErr, context.Canceled) {
		return tui.StatusUnknown, false
	}
	if _, providerFailure := provider.AsAPIError(runErr); providerFailure {
		return tui.StatusError, true
	}
	if requestCompleted {
		// A local post-response failure (for example, session persistence)
		// does not make the provider disconnected.
		return tui.StatusConnected, true
	}
	return tui.StatusUnknown, false
}

func withTUIInputAdmission(ctx context.Context, generation uint64) context.Context {
	return context.WithValue(ctx, tuiInputAdmissionKey{}, generation)
}

func tuiInputAdmission(ctx context.Context) uint64 {
	generation, _ := ctx.Value(tuiInputAdmissionKey{}).(uint64)
	return generation
}

func hasConflictingTUIQuery(ctx context.Context, state *tui.AppState) bool {
	return state.HasActiveQueryOtherThan(tuiInputAdmission(ctx))
}

type sessionGoalRuntime struct {
	cfg        TUIREPLConfig
	sessionID  string
	projectDir string
}

func newSessionGoalRuntime(cfg TUIREPLConfig, sessionID, projectDir string) commands.GoalRuntime {
	if cfg.Repo == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return &sessionGoalRuntime{
		cfg:        cfg,
		sessionID:  strings.TrimSpace(sessionID),
		projectDir: strings.TrimSpace(projectDir),
	}
}

func (r *sessionGoalRuntime) LoadGoal() (*goal.Goal, error) {
	meta, _, err := r.cfg.Repo.GetMeta(r.sessionID, r.projectDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if meta.Goal == nil {
		return nil, nil
	}
	current := *meta.Goal
	return &current, nil
}

func (r *sessionGoalRuntime) SaveGoal(next goal.Goal) error {
	saved := next
	return saveTUISessionMeta(r.cfg, r.sessionID, r.projectDir, session.SessionMeta{Goal: &saved})
}

func (r *sessionGoalRuntime) UpdateGoal(update goal.UpdateFunc) (goal.Goal, error) {
	next, err := r.cfg.Repo.UpdateGoal(r.sessionID, r.projectDir, update)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return next, err
	}
	if err := saveTUISessionMeta(r.cfg, r.sessionID, r.projectDir, session.SessionMeta{}); err != nil {
		return goal.Goal{}, err
	}
	return r.cfg.Repo.UpdateGoal(r.sessionID, r.projectDir, update)
}

var _ goal.Updater = (*sessionGoalRuntime)(nil)

func withTUICommandLock(cfg TUIREPLConfig, fn func()) {
	if cfg.CommandMu == nil {
		fn()
		return
	}
	cfg.CommandMu.Lock()
	defer cfg.CommandMu.Unlock()
	fn()
}

type tuiPermissionRequester interface {
	PermissionRequest(toolName string, input map[string]any, riskLevel int) string
}

type tuiDecisionRequester interface {
	DecisionRequest(context.Context, permissions.PromptRequest) permissions.PromptResponse
}

type tuiActivityApp interface {
	State() *tui.AppState
	UpdateSync(func()) bool
}

func tuiContextGeneration(eng engine.Engine, sessionID, projectDir string) (engine.ContextGenerationState, error) {
	normalize := func(state engine.ContextGenerationState, err error) (engine.ContextGenerationState, error) {
		if err != nil {
			return engine.ContextGenerationState{}, err
		}
		if (state.Persisted && state.Generation == 0) || (!state.Persisted && state.Generation != 0) {
			return engine.ContextGenerationState{}, session.ErrCorruptSessionHistory
		}
		return state, nil
	}
	projectDir = strings.TrimSpace(projectDir)
	if projectDir != "" {
		if scoped, ok := eng.(engine.ScopedContextGenerationStateProvider); ok {
			return normalize(scoped.ContextGenerationStateForSession(sessionID, projectDir))
		}
		if scoped, ok := eng.(engine.ScopedContextGenerationProvider); ok {
			generation, err := scoped.ContextGenerationForSession(sessionID, projectDir)
			return normalize(engine.ContextGenerationState{Generation: generation, Persisted: generation != 0}, err)
		}
		// A provider that exposes only unscoped state cannot safely answer an
		// exact project request. Do not fall back across namespaces.
		if _, ok := eng.(engine.ContextGenerationStateProvider); ok {
			return engine.ContextGenerationState{}, engine.ErrContextGenerationUnavailable
		}
		if _, ok := eng.(engine.ContextGenerationProvider); ok {
			return engine.ContextGenerationState{}, engine.ErrContextGenerationUnavailable
		}
		return engine.ContextGenerationState{}, nil
	}
	if provider, ok := eng.(engine.ContextGenerationStateProvider); ok {
		return normalize(provider.ContextGenerationState(sessionID))
	}
	if provider, ok := eng.(engine.ContextGenerationProvider); ok {
		generation, err := provider.ContextGeneration(sessionID)
		return normalize(engine.ContextGenerationState{Generation: generation, Persisted: generation != 0}, err)
	}
	return engine.ContextGenerationState{}, nil
}

func commitTUIContextGeneration(r ui.Renderer, eng engine.Engine, base ui.ToolEventContext, projectDir string) (engine.ContextGenerationState, error) {
	committer, ok := r.(tuiContextGenerationCommitter)
	if !ok {
		return engine.ContextGenerationState{}, engine.ErrContextGenerationUnavailable
	}
	next, err := tuiContextGeneration(eng, base.SessionID, projectDir)
	if err != nil {
		// Still enqueue an old-state barrier so every already-admitted reducer
		// closure settles before the lookup failure is surfaced.
		_ = committer.CommitContextGeneration(base, base.ContextGeneration, base.ContextGenerationPersisted)
		return engine.ContextGenerationState{}, err
	}
	if !committer.CommitContextGeneration(base, next.Generation, next.Persisted) {
		return engine.ContextGenerationState{}, engine.ErrContextGenerationUnavailable
	}
	return next, nil
}

func syncTUIUsageFromTracker(state *tui.AppState, tracker *ui.CostTracker) {
	if state == nil || tracker == nil {
		return
	}
	input, output, cacheRead, cacheCreate := tracker.TotalUsage()
	state.SessionTotalInputTokens.Set(input)
	state.SessionTotalOutputTokens.Set(output)
	state.SessionTotalCacheReadTokens.Set(cacheRead)
	state.SessionTotalCacheCreateTokens.Set(cacheCreate)
	state.SessionWebSearchRequests.Set(tracker.TotalWebSearchRequests())
	state.CumulativeCost.Set(tracker.TotalCost())
	state.SessionCostKnown.Set(tracker.CostKnown())
	hasCompacted, inputAtCompact, cacheReadAtCompact := tracker.CompactionBaseline()
	state.SessionHasCompacted.Set(hasCompacted)
	state.SessionInputTokensAtCompact.Set(inputAtCompact)
	state.SessionCacheReadAtCompact.Set(cacheReadAtCompact)
	conversation := tracker.ConversationUsage()
	state.SessionRoundUsageKnown.Set(conversation.Known)
	state.SessionCompactionCount.Set(conversation.CompactionCount)
	state.SessionCompletedRoundInputTokens.Set(conversation.CompletedInputTokens)
	state.SessionCompletedRoundOutputTokens.Set(conversation.CompletedOutputTokens)
	state.SessionInputTokens.Set(conversation.LastInputTokens)
	state.SessionOutputTokens.Set(conversation.LastOutputTokens)
	state.SessionCacheReadTokens.Set(conversation.LastCacheReadTokens)
	state.SessionCacheCreateTokens.Set(conversation.LastCacheMakeTokens)
	if conversation.Known || input > 0 || output > 0 {
		state.SessionUsageKnown.Set(true)
	}
}

func restoreTrackerConversationUsage(tracker *ui.CostTracker, usage tui.SessionUsage) {
	if tracker == nil {
		return
	}
	tracker.RestoreConversationUsage(ui.ConversationUsage{
		Known: usage.RoundUsageKnown, CompactionCount: usage.CompactionCount,
		CompletedInputTokens: usage.CompletedRoundInputTokens, CompletedOutputTokens: usage.CompletedRoundOutputTokens,
		LastInputTokens: usage.LastInputTokens, LastOutputTokens: usage.LastOutputTokens,
		LastCacheReadTokens: usage.LastCacheReadTokens, LastCacheMakeTokens: usage.LastCacheCreateTokens,
	})
}

func restoreTrackerConversationUsageFromMeta(tracker *ui.CostTracker, usage *session.SessionUsageMeta) {
	if tracker == nil || usage == nil {
		return
	}
	tracker.RestoreConversationUsage(ui.ConversationUsage{
		Known: usage.RoundUsageKnown, CompactionCount: usage.CompactionCount,
		CompletedInputTokens: usage.CompletedRoundInputTokens, CompletedOutputTokens: usage.CompletedRoundOutputTokens,
		LastInputTokens: usage.LastInputTokens, LastOutputTokens: usage.LastOutputTokens,
		LastCacheReadTokens: usage.LastCacheReadTokens, LastCacheMakeTokens: usage.LastCacheCreateTokens,
	})
}

func flushTUIUsageUpdates(app tuiActivityApp, tracker *ui.CostTracker) bool {
	if app.UpdateSync(func() { syncTUIUsageFromTracker(app.State(), tracker) }) {
		return true
	}
	syncTUIUsageFromTracker(app.State(), tracker)
	return false
}

func releaseTUIQueryAfterUpdates(app tuiActivityApp, generation uint64, tracker *ui.CostTracker) {
	state := app.State()
	flushTUIUsageUpdates(app, tracker)
	state.ClearQueryCancel(generation)
}

func settleTUIQueryViewAfterUpdates(app tuiActivityApp, tracker *ui.CostTracker) {
	state := app.State()
	flushTUIUsageUpdates(app, tracker)
	if !app.UpdateSync(func() {
		state.FinalizeStream()
		state.ClearLLMCall()
	}) {
		state.FinalizeStream()
		state.ClearLLMCall()
	}
}

type tuiActivityStopper interface {
	Stop(string) (tools.BackgroundTaskSnapshot, error)
}

type tuiEpochRenderer interface {
	TextAtEpoch(uint64, string)
	ThinkingAtEpoch(uint64, string)
	ErrorAtEpoch(uint64, string)
	InfoAtEpoch(uint64, string)
	UsageAtEpoch(uint64, *types.Usage)
	CostSummaryAtEpoch(uint64, float64, float64, int, int)
	ContextBarAtEpoch(uint64, int, int)
	SetProviderStatusAtEpoch(uint64, tui.ProviderStatus)
	SpinnerStartAtEpoch(uint64, string) func()
}

// tuiContextEventRenderer is the generation-aware reducer path used by the
// interactive TUI. Every queued closure rechecks session, presentation epoch,
// and durable context generation before mutating state.
type tuiContextEventRenderer interface {
	TextAtContext(ui.ToolEventContext, string)
	ThinkingAtContext(ui.ToolEventContext, string)
	ErrorAtContext(ui.ToolEventContext, string)
	InfoAtContext(ui.ToolEventContext, string)
	UsageAtContext(ui.ToolEventContext, *types.Usage)
	CostSummaryAtContext(ui.ToolEventContext, float64, float64, int, int)
	CostKnownAtContext(ui.ToolEventContext, bool)
	ContextBarAtContext(ui.ToolEventContext, int, int)
	EffectiveContextAtContext(ui.ToolEventContext, ui.EffectiveContextProjection)
	SetProviderStatusAtContext(ui.ToolEventContext, tui.ProviderStatus)
	GoalStatusAtContext(ui.ToolEventContext, loop.GoalStatusEvent)
	LLMRequestStatusAtContext(ui.ToolEventContext, loop.EventType, loop.RequestStatusEvent)
	FreezeAggregatesAtContext(ui.ToolEventContext)
	ActivityAtContext(ui.ToolEventContext, tui.ActivityEvent)
}

type tuiContextGenerationCommitter interface {
	CommitContextGeneration(ui.ToolEventContext, uint64, bool) bool
}

type tuiRuntimeErrorRenderer interface {
	RuntimeErrorEvent(ui.ToolEventContext, string, string, *types.APIError, map[string]any)
}

type tuiCostKnownEpochRenderer interface {
	CostKnownAtEpoch(uint64, bool)
}

type tuiSessionUsageRenderer interface {
	SessionUsageAtContext(ui.ToolEventContext, ui.SessionUsageProjection)
}

type tuiEffectiveContextEpochRenderer interface {
	EffectiveContextAtEpoch(uint64, ui.EffectiveContextProjection)
}

type tuiActivityEpochRenderer interface {
	ActivityAtEpoch(uint64, tui.ActivityEvent)
}

type tuiCompactionProgressEpochRenderer interface {
	CompactionProgressAtEpoch(uint64, ui.ToolEventContext, loop.ProgressEvent)
}

type tuiCompactionBoundaryEpochRenderer interface {
	CompactionBoundaryAtEpoch(uint64, ui.ToolEventContext, loop.CompactBoundaryEvent)
}

type tuiGoalStatusEpochRenderer interface {
	GoalStatusAtEpoch(uint64, loop.GoalStatusEvent)
}

type tuiLLMRequestEpochRenderer interface {
	LLMRequestStatusAtEpoch(uint64, loop.EventType, loop.RequestStatusEvent)
}

type tuiAggregateFreezer interface {
	FreezeAggregatesAtEpoch(uint64, string, string)
}

type tuiSessionSnapshotPublisher interface {
	ApplySessionSnapshot(tui.SessionSnapshot) error
}

func publishTUISessionSnapshot(app tuiActivityApp, snapshot tui.SessionSnapshot) error {
	if publisher, ok := app.(tuiSessionSnapshotPublisher); ok {
		return publisher.ApplySessionSnapshot(snapshot)
	}
	return app.State().ApplySessionSnapshot(snapshot)
}

const (
	replWrapFormat       = "%s: %w"
	replWrapDetailFormat = "%s: %w (%s)"
)

func replError(key i18n.Key, args ...any) error {
	return replErrorInLanguage(i18n.DetectOrLoadLanguage(), key, args...)
}

func replErrorInLanguage(lang i18n.Language, key i18n.Key, args ...any) error {
	return errors.New(i18n.Format(lang, key, args...))
}

func replWrap(key i18n.Key, cause error, args ...any) error {
	return replWrapInLanguage(i18n.DetectOrLoadLanguage(), key, cause, args...)
}

func replWrapInLanguage(lang i18n.Language, key i18n.Key, cause error, args ...any) error {
	return fmt.Errorf(replWrapFormat, i18n.Format(lang, key, args...), cause)
}

func replWrapWithDetail(key i18n.Key, cause error, detailKey i18n.Key, detailArgs ...any) error {
	return replWrapWithDetailInLanguage(i18n.DetectOrLoadLanguage(), key, cause, detailKey, detailArgs...)
}

func replWrapWithDetailInLanguage(lang i18n.Language, key i18n.Key, cause error, detailKey i18n.Key, detailArgs ...any) error {
	return fmt.Errorf(replWrapDetailFormat, i18n.Text(lang, key), cause, i18n.Format(lang, detailKey, detailArgs...))
}

func replErrorWithDetailInLanguage(lang i18n.Language, key, detailKey i18n.Key, detailArgs ...any) error {
	return errors.New(i18n.Text(lang, key) + " (" + i18n.Format(lang, detailKey, detailArgs...) + ")")
}

func transitionTUISession(ctx context.Context, cfg TUIREPLConfig, app tuiActivityApp, store *sessionStoreAdapter, entry commands.SessionListEntry) error {
	lang := app.State().Language.Get()
	if cfg.SwitchSession == nil {
		return replErrorInLanguage(lang, i18n.KeyREPLErrorSessionSwitchUnavailable)
	}
	if cfg.SessionTransitionMu == nil {
		return replErrorInLanguage(lang, i18n.KeyREPLErrorSessionTransitionLockMissing)
	}
	cfg.SessionTransitionMu.Lock()
	defer cfg.SessionTransitionMu.Unlock()
	if app.State().HasActiveQuery() {
		return replErrorInLanguage(lang, i18n.KeyREPLErrorSwitchWhileQueryRunning)
	}
	if err := persistTUISessionLifecycleForApp(cfg, app); err != nil {
		return replWrapInLanguage(lang, i18n.KeyREPLErrorSaveCurrentLifecycle, err)
	}
	oldEntry := commands.SessionListEntry{ID: *cfg.SessionID, ProjectDir: currentProjectDir(cfg), CWD: currentCWD(cfg)}
	oldMode := app.State().Mode.Get()
	messages, err := store.LoadEntry(entry)
	if err != nil {
		return err
	}
	snapshot, err := prepareTUISessionSnapshot(cfg, entry.ID, entry.ProjectDir, app.State().SessionEpoch.Get()+1, messages)
	if err != nil {
		return err
	}
	if err := cfg.SwitchSession(ctx, entry); err != nil {
		return err
	}
	rollback := func(cause error) error {
		rollbackErr := cfg.SwitchSession(ctx, oldEntry)
		if rollbackErr == nil {
			modeErr := applyTUISessionPermissionMode(cfg, oldMode)
			if modeErr != nil {
				actual := currentTUISessionPermissionMode(cfg, oldMode)
				if !app.UpdateSync(func() { app.State().Mode.Set(actual) }) {
					app.State().Mode.Set(actual)
				}
				fatalErr := replWrapWithDetailInLanguage(lang, i18n.KeyREPLErrorSessionTransitionFailed, cause, i18n.KeyREPLErrorRollbackModeFailedClosed, modeErr, actual.String())
				if cfg.FailClosed != nil {
					cfg.FailClosed(fatalErr)
				}
				return fatalErr
			}
			return cause
		}

		// The engine and identity are still on the target. Complete the target
		// projection using the permission mode that actually survived.
		snapshot.PermissionMode = currentTUISessionPermissionMode(cfg, snapshot.PermissionMode)
		var publishErr error
		if !app.UpdateSync(func() { publishErr = publishTUISessionSnapshot(app, snapshot) }) {
			publishErr = publishTUISessionSnapshot(app, snapshot)
		}
		if cfg.RestoreSessionUsage != nil {
			cfg.RestoreSessionUsage(snapshot.Usage)
		}
		if cfg.PermChecker != nil {
			cfg.PermChecker.ResetSession()
		}
		if publishErr != nil {
			return replWrapWithDetailInLanguage(lang, i18n.KeyREPLErrorSessionTransitionFailed, cause, i18n.KeyREPLErrorRollbackSessionTargetPublish, rollbackErr, publishErr)
		}
		return replWrapWithDetailInLanguage(lang, i18n.KeyREPLErrorSessionTransitionFailed, cause, i18n.KeyREPLErrorRollbackSessionTargetRetained, rollbackErr, snapshot.PermissionMode.String())
	}
	if err := applyTUISessionPermissionMode(cfg, snapshot.PermissionMode); err != nil {
		return rollback(replWrapInLanguage(lang, i18n.KeyREPLErrorRestorePermissionMode, err))
	}
	var publishErr error
	if !app.UpdateSync(func() { publishErr = publishTUISessionSnapshot(app, snapshot) }) {
		return rollback(replErrorInLanguage(lang, i18n.KeyREPLErrorPublishSessionStopped))
	}
	if publishErr != nil {
		return rollback(replWrapInLanguage(lang, i18n.KeyREPLErrorPublishSession, publishErr))
	}
	if cfg.RestoreSessionUsage != nil {
		cfg.RestoreSessionUsage(snapshot.Usage)
	}
	if cfg.PermChecker != nil {
		cfg.PermChecker.ResetSession()
	}
	return nil
}

func clearTUIConversation(ctx context.Context, cfg TUIREPLConfig, app tuiActivityApp) (string, error) {
	lang := app.State().Language.Get()
	if cfg.SessionTransitionMu == nil {
		return "", replErrorInLanguage(lang, i18n.KeyREPLErrorSessionTransitionLockMissing)
	}
	cfg.SessionTransitionMu.Lock()
	defer cfg.SessionTransitionMu.Unlock()
	if app.State().HasActiveQuery() {
		return "", replErrorInLanguage(lang, i18n.KeyREPLErrorClearWhileQueryRunning)
	}
	if err := persistTUISessionLifecycleForApp(cfg, app); err != nil {
		return "", replWrapInLanguage(lang, i18n.KeyREPLErrorSaveCurrentLifecycle, err)
	}
	oldMode := app.State().Mode.Get()
	newID := uuid.NewString()
	snapshot, prepared, err := prepareEmptyTUISession(ctx, cfg, newID, app.State().SessionEpoch.Get()+1)
	if err != nil {
		return "", err
	}
	committed := false
	defer func() {
		if !committed {
			prepared.Abort()
			_ = cfg.Engine.Sessions().Delete(newID)
		}
	}()
	if err := applyTUISessionPermissionMode(cfg, tui.ModeAutoEdit); err != nil {
		return "", replWrapInLanguage(lang, i18n.KeyREPLErrorResetPermissionMode, err)
	}

	var commitErr, publishErr error
	if !app.UpdateSync(func() {
		commitErr = commitPreparedRuntimeResume(ctx, prepared)
		if commitErr != nil {
			return
		}
		committed = true
		*cfg.SessionID = newID
		if cfg.PublishSessionID != nil {
			cfg.PublishSessionID(newID)
		}
		publishErr = publishTUISessionSnapshot(app, snapshot)
	}) {
		modeErr := applyTUISessionPermissionMode(cfg, oldMode)
		if modeErr != nil {
			// Rollback cannot restore the old permission state. Commit the
			// prepared empty session and align its projection to actual state.
			if commitErr = commitPreparedRuntimeResume(context.Background(), prepared); commitErr == nil {
				committed = true
				*cfg.SessionID = newID
				if cfg.PublishSessionID != nil {
					cfg.PublishSessionID(newID)
				}
				snapshot.PermissionMode = currentTUISessionPermissionMode(cfg, tui.ModeAutoEdit)
				publishErr = publishTUISessionSnapshot(app, snapshot)
				if cfg.RestoreSessionUsage != nil {
					cfg.RestoreSessionUsage(snapshot.Usage)
				}
			}
			return newID, replErrorWithDetailInLanguage(lang, i18n.KeyREPLErrorPublishEmptySessionStopped, i18n.KeyREPLErrorPublishEmptySessionDetails, modeErr, commitErr, publishErr)
		}
		return "", replErrorInLanguage(lang, i18n.KeyREPLErrorPublishEmptySessionStopped)
	}
	if commitErr != nil {
		modeErr := applyTUISessionPermissionMode(cfg, oldMode)
		if modeErr != nil {
			actual := currentTUISessionPermissionMode(cfg, tui.ModeAutoEdit)
			if !app.UpdateSync(func() { app.State().Mode.Set(actual) }) {
				app.State().Mode.Set(actual)
			}
			fatalErr := replWrapWithDetailInLanguage(lang, i18n.KeyREPLErrorActivateEmptySession, commitErr, i18n.KeyREPLErrorRollbackModeFailedClosed, modeErr, actual.String())
			if cfg.FailClosed != nil {
				cfg.FailClosed(fatalErr)
			}
			return "", fatalErr
		}
		return "", replWrapWithDetailInLanguage(lang, i18n.KeyREPLErrorActivateEmptySession, commitErr, i18n.KeyREPLErrorRollbackMode, modeErr)
	}
	if publishErr != nil {
		// Snapshot preparation guarantees validity; keep the committed engine
		// and identity aligned even if a custom state implementation rejects it.
		publishErr = publishTUISessionSnapshot(app, snapshot)
		return newID, replWrapInLanguage(lang, i18n.KeyREPLErrorPublishSession, publishErr)
	}
	if cfg.RestoreSessionUsage != nil {
		cfg.RestoreSessionUsage(snapshot.Usage)
	}
	if cfg.PermChecker != nil {
		cfg.PermChecker.ResetSession()
	}
	return newID, nil
}

func prepareEmptyTUISession(ctx context.Context, cfg TUIREPLConfig, newID string, epoch uint64) (tui.SessionSnapshot, engine.PreparedRuntimeContextResume, error) {
	snapshot, err := prepareTUISessionSnapshot(cfg, newID, currentProjectDir(cfg), epoch, nil)
	if err != nil {
		return tui.SessionSnapshot{}, nil, err
	}
	snapshot.Usage = tui.SessionUsage{Known: true, RoundUsageKnown: true}
	snapshot.Interaction = tui.SessionInteraction{}
	snapshot.PermissionMode = tui.ModeAutoEdit
	if err := cfg.Engine.Sessions().Save(newID, nil); err != nil {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replWrap(i18n.KeyREPLErrorCreateEmptySession, err)
	}
	// The first save publishes generation 1. Refresh before the /clear snapshot
	// is committed so the new visible session never remains in legacy zero.
	generation, generationErr := tuiContextGeneration(cfg.Engine, newID, currentProjectDir(cfg))
	if generationErr != nil {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replWrap(i18n.KeyREPLErrorCreateEmptySession, generationErr)
	}
	snapshot.ContextGeneration = generation.Generation
	snapshot.ContextGenerationPersisted = generation.Persisted
	if err := saveTUISessionMeta(cfg, newID, currentProjectDir(cfg), session.SessionMeta{
		Usage: &session.SessionUsageMeta{RoundUsageKnown: true, CostKnown: true}, Presentation: &session.SessionPresentationMeta{PermissionMode: "auto"},
	}); err != nil {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replWrap(i18n.KeyREPLErrorSaveEmptySessionLifecycle, err)
	}
	preparer, ok := cfg.Engine.(engine.SessionResumePreparer)
	if !ok {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replError(i18n.KeyREPLErrorEmptySessionResumeUnsupported)
	}
	prepared, err := preparer.PrepareResume(ctx, newID)
	if err != nil {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replWrap(i18n.KeyREPLErrorPrepareEmptySession, err)
	}
	return snapshot, prepared, nil
}

func currentTUISessionPermissionMode(cfg TUIREPLConfig, fallback tui.InteractionMode) tui.InteractionMode {
	if cfg.PlanState != nil && cfg.PlanState.IsActive() {
		return tui.ModePlanEdit
	}
	if cfg.RuntimeScope != nil {
		return interactionModeFromSessionMeta(cfg.RuntimeScope.PermissionMode())
	}
	if cfg.PermChecker != nil {
		if cfg.PermChecker.Mode() == permissions.ModeAllowAll {
			return tui.ModeAutoEdit
		}
		return tui.ModeAskEdit
	}
	return fallback
}

func installTUIPermissionPrompt(checker *permissions.Checker, requester any) {
	if checker == nil || requester == nil {
		return
	}
	var promptMu sync.Mutex
	if structured, ok := requester.(tuiDecisionRequester); ok {
		checker.SetStructuredPromptFunc(func(ctx context.Context, request permissions.PromptRequest) permissions.PromptResponse {
			promptMu.Lock()
			defer promptMu.Unlock()
			return structured.DecisionRequest(ctx, request)
		})
		return
	}
	legacy, ok := requester.(tuiPermissionRequester)
	if !ok {
		return
	}
	checker.SetPromptFunc(func(toolName string, input map[string]any) permissions.Decision {
		promptMu.Lock()
		defer promptMu.Unlock()
		resp := legacy.PermissionRequest(toolName, input, permissionRiskLevelForTUI(toolName, input))
		return permissionDecisionFromTUIResponse(resp)
	})
}

func permissionDecisionFromTUIResponse(resp string) permissions.Decision {
	switch strings.ToLower(strings.TrimSpace(resp)) {
	case "y":
		return permissions.DecisionAllowOnce
	case "a":
		return permissions.DecisionAllow
	default:
		return permissions.DecisionDeny
	}
}

func permissionRiskLevelForTUI(toolName string, input map[string]any) int {
	switch permissions.ClassifyRisk(toolName, input) {
	case permissions.RiskHigh:
		return 3
	case permissions.RiskMedium:
		return 2
	default:
		return 1
	}
}

type asyncGate struct {
	mu        sync.Mutex
	accepting bool
	wg        sync.WaitGroup
}

func newAsyncGate() *asyncGate {
	return &asyncGate{accepting: true}
}

func (g *asyncGate) Go(fn func()) bool {
	if fn == nil {
		return true
	}
	g.mu.Lock()
	if !g.accepting {
		g.mu.Unlock()
		return false
	}
	g.wg.Add(1)
	g.mu.Unlock()
	go func() {
		defer g.wg.Done()
		fn()
	}()
	return true
}

func (g *asyncGate) Close() {
	g.mu.Lock()
	g.accepting = false
	g.mu.Unlock()
}

func (g *asyncGate) Wait() {
	g.wg.Wait()
}

// RunTUIREPL runs the default interactive loop using go-tui.
// The CLI now enters this full-screen TUI by default for interactive sessions.
func RunTUIREPL(ctx context.Context, cfg TUIREPLConfig, sigHandler *SignalHandler) error {
	if cfg.SessionTransitionMu == nil {
		cfg.SessionTransitionMu = &sync.Mutex{}
	}
	if cfg.CommandMu == nil {
		cfg.CommandMu = &sync.Mutex{}
	}
	ctx, cancelRuntime := context.WithCancel(ctx)
	defer cancelRuntime()

	// Cost tracker
	tracker := ui.NewCostTracker(cfg.Engine.Provider().ModelID())
	tracker.SetProvider(cfg.Engine.Provider().Name())
	if catalog := getCatalog(cfg); catalog != nil {
		tracker.SetCatalog(catalog)
	}
	var tuiAppRef *tui.App
	cfg.RestoreSessionUsage = func(usage tui.SessionUsage) {
		tracker.SetProviderAndModel(cfg.Engine.Provider().Name(), cfg.Engine.Provider().ModelID())
		costKnown := true
		if tuiAppRef != nil {
			costKnown = tuiAppRef.State().SessionCostKnown.Get()
		}
		tracker.RestoreSession(cfg.Engine.Provider().ModelID(), usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheCreateTokens, usage.WebSearchRequests, usage.CumulativeCost, costKnown)
		tracker.RestoreCompactionBaseline(usage.HasCompacted, usage.InputTokensAtCompact, usage.CacheReadAtCompact)
		restoreTrackerConversationUsage(tracker, usage)
	}

	// Command registry (reuse existing 18 built-in commands)
	cmdReg := commands.NewRegistry()
	commands.RegisterBuiltins(cmdReg)
	storeAdapter := &sessionStoreAdapter{
		repo: cfg.Repo,
		currentProjectDir: func() string {
			if cfg.SessionProjectDir == nil {
				return ""
			}
			return *cfg.SessionProjectDir
		},
	}

	// Adapter that exposes engine conversation state to slash commands.
	ql := &engineQueryLooper{
		eng:             cfg.Engine,
		sessionID:       func() string { return *cfg.SessionID },
		model:           cfg.Engine.Provider().ModelID(),
		reasoningEffort: cfg.ReasoningEffort,
	}
	cfg.CurrentModel = ql.Model

	// Use a pointer indirection to break the circular reference:
	// The closure passed to NewTUIApp references tuiAppRef, which is
	// populated after NewTUIApp returns but before Run() is called.
	var inputScheduler *tuiInputScheduler

	// WaitGroup tracks inflight handleTUIInput goroutines so we can
	// wait for them to finish after Run() returns (e.g. on Ctrl+C)
	// before calling Close(). This prevents writes to a closed app.
	inflight := newAsyncGate()
	runTracked := inflight.Go

	// Create TUI app with submit handler
	providerName := cfg.Engine.Provider().Name()
	modelID := cfg.Engine.Provider().ModelID()
	slashCommands := builtinTUISlashCommandEntries(cmdReg)

	tuiApp, err := tui.NewTUIAppWithAdmission(func(inputStr string) bool {
		if tuiAppRef == nil || inputScheduler == nil {
			return false
		}
		// Foreground input is an admission request, not an implicit queueing
		// operation. A busy turn must leave the composer's draft and attachments
		// intact; queueing remains available only through an explicit surface.
		admitted, _ := inputScheduler.TrySubmit(inputStr, tuiAppRef.State().TakePendingImages, false)
		if !admitted {
			tuiAppRef.Renderer().Warning(i18n.Text(tuiAppRef.State().Language.Get(), i18n.KeyREPLTUIQueryRunning))
		}
		return admitted
	}, providerName, modelID, getCatalog(cfg), slashCommands)
	if err != nil {
		return replWrap(i18n.KeyREPLErrorCreateTUIApp, err)
	}
	if cfg.BuildDiagnostic != nil {
		tuiApp.SetBuildDiagnostic(cfg.BuildDiagnostic(currentCWD(cfg)))
	}
	tuiAppRef = tuiApp
	inputScheduler = newTUIInputScheduler(
		runTracked,
		func(submission *tuiInputSubmission) bool {
			return prepareTUIInputAdmission(ctx, tuiApp.State(), submission)
		},
		func(submission tuiInputSubmission) {
			defer submission.abort()
			handleTUIInputSubmission(submission.ctx, cfg, tuiApp, cmdReg, storeAdapter, ql, tracker, submission, sigHandler, runTracked)
		},
		tuiApp.State().TryCancelQuery,
		func(count int) {
			tuiApp.GoTuiApp().QueueUpdateLossless(func() {
				tuiApp.State().QueuedInputCount.Set(count)
			})
		},
	)
	tuiApp.SetOnSteerQueued(func() {
		if inputScheduler.PromoteQueuedToSteering() {
			tuiApp.Renderer().Info(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyTUIInputQueuedAsGuidance))
		}
	})
	if cfg.SkillsMenuLauncher == nil {
		if launcher, ok := any(tuiApp).(tui.SkillsMenuLauncher); ok {
			cfg.SkillsMenuLauncher = launcher
		}
	}
	if cfg.FailClosed == nil {
		cfg.FailClosed = func(error) {
			cancelRuntime()
			tuiApp.Stop()
		}
	}
	initialMessages, loadErr := cfg.Engine.Sessions().Load(*cfg.SessionID)
	if loadErr != nil && !errors.Is(loadErr, engine.ErrSessionNotFound) {
		_ = tuiApp.Close()
		return replWrapInLanguage(tuiApp.State().Language.Get(), i18n.KeyREPLErrorLoadInitialTUISession, loadErr)
	}
	if errors.Is(loadErr, engine.ErrSessionNotFound) {
		initialMessages = nil
	}
	initialSnapshot, err := prepareTUISessionSnapshot(cfg, *cfg.SessionID, currentProjectDir(cfg), 1, initialMessages)
	if err != nil {
		_ = tuiApp.Close()
		return replWrapInLanguage(tuiApp.State().Language.Get(), i18n.KeyREPLErrorPrepareInitialTUISession, err)
	}
	if err := tuiApp.ApplySessionSnapshot(initialSnapshot); err != nil {
		_ = tuiApp.Close()
		return replWrapInLanguage(tuiApp.State().Language.Get(), i18n.KeyREPLErrorApplyInitialTUISession, err)
	}
	// A persisted running row is not proof that its producer survived the
	// process boundary. Reconcile against the manager's in-memory live set so
	// stale rows become historical orphans instead of current footer work.
	liveActivityRuns := make(map[string]string)
	if cfg.BackgroundTasks != nil {
		for _, snapshot := range cfg.BackgroundTasks.InMemorySnapshots() {
			if snapshot.Status != "running" {
				continue
			}
			liveActivityRuns["background:"+snapshot.ID] = snapshot.CurrentRunID
			liveActivityRuns["agent:"+snapshot.ID] = snapshot.CurrentRunID
		}
	}
	tuiApp.State().Activities.ReconcileNonTerminal(liveActivityRuns)
	if err := applyTUISessionPermissionMode(cfg, initialSnapshot.PermissionMode); err != nil {
		_ = tuiApp.Close()
		return replWrapInLanguage(tuiApp.State().Language.Get(), i18n.KeyREPLErrorRestoreInitialTUIMode, err)
	}
	trackerModel := initialSnapshot.Model
	if strings.TrimSpace(trackerModel) == "" {
		trackerModel = modelID
	}
	tracker.RestoreSession(trackerModel, initialSnapshot.Usage.InputTokens, initialSnapshot.Usage.OutputTokens,
		initialSnapshot.Usage.CacheReadTokens, initialSnapshot.Usage.CacheCreateTokens,
		initialSnapshot.Usage.WebSearchRequests, initialSnapshot.Usage.CumulativeCost,
		tuiApp.State().SessionCostKnown.Get())
	tracker.RestoreCompactionBaseline(initialSnapshot.Usage.HasCompacted, initialSnapshot.Usage.InputTokensAtCompact, initialSnapshot.Usage.CacheReadAtCompact)
	restoreTrackerConversationUsage(tracker, initialSnapshot.Usage)
	if initialSnapshot.SessionCostKnown == nil {
		tuiApp.State().SessionCostKnown.Set(tracker.CostKnown())
	}
	unbindTaskView := bindTaskCreateViewState(cfg.TaskCreateTool, tuiApp)
	defer unbindTaskView()
	unbindAgentProgress := bindTUIAgentProgress(cfg.AgentTool, cfg.BackgroundTasks, tuiApp)
	defer unbindAgentProgress()
	unbindBackgroundActivities := bindTUIBackgroundActivities(cfg.BackgroundTasks, tuiApp)
	defer unbindBackgroundActivities()
	unbindActivityPersistence := bindTUIActivityPersistence(cfg, tuiApp)
	defer unbindActivityPersistence()

	// Get the TUI renderer
	r := tuiApp.Renderer()
	if cfg.AskUserQuestionTool != nil {
		cfg.AskUserQuestionTool.SetInteractionRequester(r)
		defer cfg.AskUserQuestionTool.SetInteractionRequester(nil)
	}
	hookRenderer := tuiREPLHookRenderer{app: tuiApp}
	if hookRunner := currentHookRunner(cfg); hookRunner != nil {
		hookContext := newREPLHookContext(*cfg.SessionID, hooks.HookSessionStart, "repl", "system")
		hookContext.SessionEpoch = tuiApp.State().SessionEpoch.Get()
		runObservedREPLHooks(ctx, hookRunner, hookRenderer, hooks.HookSessionStart, hooks.HookInput{}, hookContext)
	}
	installTUIPermissionPrompt(cfg.PermChecker, r)
	unbindBackgroundNotifications := installLocalizedTUIBackgroundNotifications(cfg.BackgroundTasks, r, func() i18n.Language {
		return tuiApp.State().Language.Get()
	}, func(_ context.Context, notification tools.RuntimeNotification) error {
		err := runTUIBackgroundFollowUp(ctx, cfg, tuiApp, tracker, notification)
		if err != nil {
			r.Error(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIBackgroundFailed, err))
		}
		return err
	})
	defer unbindBackgroundNotifications()
	if cfg.RuntimeScope != nil {
		cfg.RuntimeScope.SetPermissionModeObserver(func(mode string) {
			publishTUIRuntimePermissionMode(tuiApp.State(), tuiApp.GoTuiApp().QueueUpdateLossless, mode)
		})
		defer cfg.RuntimeScope.SetPermissionModeObserver(nil)
	}
	ql.modelSaver = func(providerName, modelID string, reasoningEffort ...string) error {
		if err := saveUserModelSettings(providerName, modelID, reasoningEffort...); err != nil {
			r.Warning(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIModelSaveFailed, err))
			return err
		}
		return nil
	}

	// Register Model Picker callback: Alt+P populates picker from catalog.
	if catalog := getCatalog(cfg); catalog != nil {
		openCascadingPicker := func() {
			buildCascadingPicker(cfg, tuiApp, r, ql, tracker)
		}
		tuiApp.SetOpenModelPicker(openCascadingPicker)
	}

	// Register Mode Switch callback: Shift+Tab cycles Auto → Ask → Plan.
	// Wires mode changes to permissions.Checker and tools.PlanState.
	tuiApp.SetOnModeSwitch(func(mode tui.InteractionMode) {
		if err := applyTUIInteractionModeAtSessionBoundary(cfg, tuiApp.State(), mode); err != nil {
			r.Warning(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIModeSwitchFailed, err))
			return
		}
		switch mode {
		case tui.ModeAutoEdit:
			r.Info(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyModeSwitchedAuto))
		case tui.ModeAskEdit:
			r.Info(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyModeSwitchedAsk))
		case tui.ModePlanEdit:
			r.Info(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyModeSwitchedPlan))
		}
	})
	tuiApp.SetOnActivityAction(func(id string, action tui.ActivityAction) {
		runTracked(func() {
			lang := tuiApp.State().Language.Get()
			message, err := performTUIActivityActionInLanguage(lang, tuiApp, cfg.BackgroundTasks, id, string(action))
			if err != nil {
				r.Error(i18n.Format(lang, i18n.KeyREPLTUIActivityActionFailed, err))
				return
			}
			if message != "" {
				r.Info(message)
			}
		})
	})

	// Set initial TUI mode based on the permission checker's current mode.
	if cfg.RuntimeScope == nil && cfg.PermChecker != nil {
		switch cfg.PermChecker.Mode() {
		case permissions.ModeAllowAll:
			tuiApp.State().Mode.Set(tui.ModeAutoEdit)
		case permissions.ModeAskAlways:
			tuiApp.State().Mode.Set(tui.ModeAskEdit)
		case permissions.ModeRuleBased:
			tuiApp.State().Mode.Set(tui.ModeAskEdit)
		}
	}

	// Banner (provider/model) was already set in NewTUIApp.
	// Set session info via the renderer — also safe before Run() as
	// go-tui's QueueUpdate queues closures that are drained on Run().
	sessionID := *cfg.SessionID
	r.SessionInfo(sessionID, cfg.Engine.Tools())

	// Run the TUI event loop (blocks until exit)
	runErr := tuiApp.Run()
	inflight.Close()
	cancelRuntime()
	tuiApp.State().TryCancelQuery()
	tuiApp.State().SignalStop()

	// Stop accepting new background follow-ups before waiting. Unbinding waits
	// for any observer callback already in progress to enqueue its work.
	unbindBackgroundNotifications()

	// Wait for all inflight query goroutines to finish before closing
	// the terminal. This prevents panics from QueueUpdate on a closed app.
	inflight.Wait()
	unbindActivityPersistence()
	if hookRunner := currentHookRunner(cfg); hookRunner != nil {
		hookContext := newREPLHookContext(*cfg.SessionID, hooks.HookSessionEnd, "repl", "system")
		hookContext.SessionEpoch = tuiApp.State().SessionEpoch.Get()
		runObservedREPLHooks(context.Background(), hookRunner, hookRenderer, hooks.HookSessionEnd, hooks.HookInput{}, hookContext)
	}
	persistErr := persistTUISessionLifecycleForApp(cfg, tuiApp)
	closeErr := tuiApp.Close()
	if persistErr != nil {
		persistErr = i18n.WrapInternalError(i18n.KeyREPLTUILifecycleSaveFailed, persistErr)
	}
	if closeErr != nil {
		closeErr = i18n.WrapInternalError(i18n.KeyREPLTUICleanupFailed, closeErr)
	}
	// Return failures after the terminal owner has attempted to restore the
	// screen. Callers decide how their output transport presents the semantic
	// error; the alternate-screen lifetime never writes product text directly.
	return errors.Join(runErr, persistErr, closeErr)
}

func builtinTUISlashCommandEntries(reg *commands.Registry) []tui.SlashCommandEntry {
	if reg == nil {
		return nil
	}
	all := reg.All()
	entries := make([]tui.SlashCommandEntry, 0, len(all))
	for _, cmd := range all {
		descriptionKey, _ := commands.CommandDescriptionKey(cmd)
		entries = append(entries, tui.SlashCommandEntry{
			Name:           cmd.Name(),
			Aliases:        cmd.Aliases(),
			Description:    cmd.Description(),
			DescriptionKey: descriptionKey,
			OpensSubmenu:   cmd.Name() == "language",
		})
	}
	return entries
}

type tuiInfoRenderer interface {
	Info(string)
}

type tuiSessionAwareInfoRenderer interface {
	VisibleSessionID() string
}

type tuiSessionScopedInfoRenderer interface {
	TryInfoForVisibleSession(sessionID, message string) bool
}

type tuiSubagentObservationRenderer interface {
	HasSubagentObservation(sessionID, parentToolUseID string) bool
	AcknowledgeSubagentResult(taskID string) bool
}

type tuiBackgroundFollowUp func(context.Context, tools.RuntimeNotification) error

func installTUIBackgroundNotifications(manager *tools.BackgroundTaskManager, renderer tuiInfoRenderer, followUps ...tuiBackgroundFollowUp) func() {
	return installLocalizedTUIBackgroundNotifications(manager, renderer, i18n.DetectOrLoadLanguage, followUps...)
}

func installLocalizedTUIBackgroundNotifications(manager *tools.BackgroundTaskManager, renderer tuiInfoRenderer, language func() i18n.Language, followUps ...tuiBackgroundFollowUp) func() {
	if manager == nil || renderer == nil {
		return func() {}
	}
	observer := tools.RuntimeNotificationSinkFunc(func(_ context.Context, notification tools.RuntimeNotification) error {
		lang := i18n.DetectOrLoadLanguage()
		if language != nil {
			lang = language()
		}
		message := notification.Message
		snapshot, ok := manager.ResolveNotificationTarget(notification)
		if !ok {
			return nil
		}
		notification = tools.LocalizeRuntimeNotification(lang, notification, snapshot)
		message = notification.Message
		if snapshot.OwnerProjectRoot == "" || !sameTUIProjectRoot(snapshot.OwnerProjectRoot, manager.CurrentProjectRoot()) {
			return nil
		}
		if snapshot.Type == "local_agent" {
			if dynamic, supported := renderer.(tuiSubagentObservationRenderer); supported && snapshot.LatestProgress != nil &&
				dynamic.HasSubagentObservation(snapshot.OwnerSessionID, snapshot.LatestProgress.ParentToolUseID) {
				return nil
			}
			if group := backgroundAgentGroupSummaryInLanguage(lang, manager.Snapshots(), snapshot); group != "" {
				message = group + "\n" + backgroundAgentMemberSummaryInLanguage(lang, snapshot) + "\n" + message
			}
			if result := strings.TrimSpace(snapshot.Result); result != "" {
				bounded := tui.RedactPresentationText(result, 1200)
				message += "\n\n" + bounded
			}
			if taskErr := strings.TrimSpace(snapshot.Error); taskErr != "" {
				message += "\n\n" + i18n.Format(lang, i18n.KeyREPLTUIErrorPrefix, taskErr)
			}
		}
		if scoped, ok := renderer.(tuiSessionScopedInfoRenderer); ok && snapshot.OwnerSessionID != "" {
			if !scoped.TryInfoForVisibleSession(snapshot.OwnerSessionID, message) {
				return i18n.NewError(i18n.KeyREPLNotificationSessionChanged)
			}
			return nil
		}
		if aware, sessionAware := renderer.(tuiSessionAwareInfoRenderer); sessionAware && snapshot.OwnerSessionID != "" && snapshot.OwnerSessionID != aware.VisibleSessionID() {
			return nil
		}
		renderer.Info(message)
		return nil
	})
	var followUp tools.RuntimeNotificationSink
	if len(followUps) > 0 && followUps[0] != nil {
		followUp = tools.RuntimeNotificationSinkFunc(func(ctx context.Context, notification tools.RuntimeNotification) error {
			if err := followUps[0](ctx, notification); err != nil {
				return err
			}
			snapshot, ok := manager.ResolveNotificationTarget(notification)
			dynamic, supported := renderer.(tuiSubagentObservationRenderer)
			if ok && supported && snapshot.Type == "local_agent" && snapshot.LatestProgress != nil &&
				dynamic.HasSubagentObservation(snapshot.OwnerSessionID, snapshot.LatestProgress.ParentToolUseID) {
				dynamic.AcknowledgeSubagentResult(snapshot.ID)
			}
			return nil
		})
	}
	manager.SetNotificationConsumers(observer, followUp)
	var once sync.Once
	return func() {
		once.Do(func() { manager.SetNotificationConsumers(nil, nil) })
	}
}

func backgroundAgentGroupSummary(snapshots []tools.BackgroundTaskSnapshot, current tools.BackgroundTaskSnapshot) string {
	return backgroundAgentGroupSummaryInLanguage(i18n.DetectOrLoadLanguage(), snapshots, current)
}

func backgroundAgentGroupSummaryInLanguage(lang i18n.Language, snapshots []tools.BackgroundTaskSnapshot, current tools.BackgroundTaskSnapshot) string {
	latest := make(map[string]tools.BackgroundTaskSnapshot)
	for _, snapshot := range snapshots {
		if snapshot.Type != "local_agent" || snapshot.OwnerSessionID != current.OwnerSessionID || !sameTUIProjectRoot(snapshot.OwnerProjectRoot, current.OwnerProjectRoot) {
			continue
		}
		latest[snapshot.ID] = snapshot
	}
	if len(latest) == 0 {
		return ""
	}
	ready, failed, running, cancelled := 0, 0, 0, 0
	for _, snapshot := range latest {
		switch snapshot.Status {
		case "completed":
			ready++
		case "failed":
			failed++
		case "killed", "cancelled":
			cancelled++
		default:
			running++
		}
	}
	parts := []string{i18n.Format(lang, i18n.KeyREPLTUIAgentGroupTotal, len(latest))}
	for _, item := range []struct {
		count int
		key   i18n.Key
	}{{failed, i18n.KeyREPLTUIAgentCountFailed}, {running, i18n.KeyREPLTUIAgentCountRunning}, {ready, i18n.KeyREPLTUIAgentCountReady}, {cancelled, i18n.KeyREPLTUIAgentCountCancelled}} {
		if item.count > 0 {
			parts = append(parts, i18n.Format(lang, item.key, item.count))
		}
	}
	return strings.Join(parts, i18n.Text(lang, i18n.KeyREPLTUISummarySeparator)) + i18n.Text(lang, i18n.KeyREPLTUISummaryEnd)
}

func backgroundAgentMemberSummary(snapshot tools.BackgroundTaskSnapshot) string {
	return backgroundAgentMemberSummaryInLanguage(i18n.DetectOrLoadLanguage(), snapshot)
}

func backgroundAgentMemberSummaryInLanguage(lang i18n.Language, snapshot tools.BackgroundTaskSnapshot) string {
	name := firstSemanticString(snapshot.AgentAlias, snapshot.ID)
	status := localizedBackgroundAgentStatus(lang, snapshot.Status)
	parts := []string{i18n.Format(lang, i18n.KeyREPLTUIAgentMember, name, status)}
	if snapshot.Attempt > 0 {
		parts = append(parts, i18n.Format(lang, i18n.KeyREPLTUIAgentAttempt, snapshot.Attempt))
	}
	if path := strings.TrimSpace(snapshot.AgentPath); path != "" {
		parts = append(parts, i18n.Format(lang, i18n.KeyREPLTUIAgentPath, path))
	}
	if description := strings.TrimSpace(snapshot.Description); description != "" {
		parts = append(parts, description)
	}
	parts = append(parts, i18n.Text(lang, i18n.KeyREPLTUIAgentDetails))
	return strings.Join(parts, i18n.Text(lang, i18n.KeyREPLTUISummarySeparator)) + i18n.Text(lang, i18n.KeyREPLTUISummaryEnd)
}

func localizedBackgroundAgentStatus(lang i18n.Language, status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return i18n.Text(lang, i18n.KeyREPLTUIStatusReady)
	case "failed":
		return i18n.Text(lang, i18n.KeyREPLTUIStatusFailed)
	case "running":
		return i18n.Text(lang, i18n.KeyREPLTUIStatusRunning)
	case "cancelled":
		return i18n.Text(lang, i18n.KeyREPLTUIStatusCancelled)
	case "killed":
		return i18n.Text(lang, i18n.KeyREPLTUIStatusKilled)
	default:
		return i18n.RootAgentPhaseLabel(lang, status)
	}
}

func runTUIBackgroundFollowUp(ctx context.Context, cfg TUIREPLConfig, tuiApp *tui.App, tracker *ui.CostTracker, notification tools.RuntimeNotification) error {
	lang := i18n.DetectOrLoadLanguage()
	if tuiApp != nil {
		lang = tuiApp.State().Language.Get()
	}
	followUpEngine, ok := cfg.Engine.(engine.FollowUpEngine)
	if !ok {
		return replErrorInLanguage(lang, i18n.KeyREPLErrorFollowUpUnsupported)
	}
	if cfg.BackgroundTasks == nil {
		return replError(i18n.KeyREPLErrorFollowUpUnavailable)
	}
	target, ok := cfg.BackgroundTasks.NotificationFollowUpTarget(notification)
	if !ok {
		return replErrorInLanguage(lang, i18n.KeyREPLErrorFollowUpTaskUnresolved, notification.TaskID)
	}
	if !backgroundFollowUpProjectMatches(cfg.BackgroundTasks, target.ProjectRoot) {
		return replErrorInLanguage(lang, i18n.KeyREPLErrorFollowUpUnavailable)
	}
	if tuiApp == nil {
		return replErrorInLanguage(lang, i18n.KeyREPLErrorFollowUpUnavailable)
	}
	sessionID := target.SessionID
	ch, err := followUpEngine.QueryFollowUp(ctx, engine.QueryRequest{
		SessionID: sessionID, SessionProjectDir: target.SessionProjectDir,
		Message: target.Message, CWD: target.ProjectRoot, ProjectRoot: target.ProjectRoot,
		RuntimeEventID: notification.ID, InternalControlCapability: messagecontrol.Runtime(),
	})
	if err != nil {
		if errors.Is(err, engine.ErrSessionDeleted) {
			tuiApp.Renderer().Info(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLBackgroundDiscarded, notification.TaskID))
			return nil
		}
		return err
	}
	r := tuiApp.Renderer()
	epoch := uint64(0)
	if tuiApp.State().SessionID.Get() == sessionID && sameTUIProjectRoot(target.ProjectRoot, cfg.BackgroundTasks.CurrentProjectRoot()) {
		epoch = tuiApp.State().SessionEpoch.Get()
	}
	eventTracker := tracker
	if epoch == 0 {
		eventTracker = nil
	}
	generation, generationErr := tuiContextGeneration(cfg.Engine, sessionID, target.SessionProjectDir)
	if generationErr != nil {
		return generationErr
	}
	base := ui.ToolEventContext{SessionID: sessionID, SessionEpoch: epoch,
		ContextGeneration: generation.Generation, ContextGenerationPersisted: generation.Persisted,
		ProjectRoot: target.ProjectRoot, ActorID: "background", ActorType: "background", WorkUnitID: notification.TaskID}
	onEvent, stopToolSpinners := makeTUIEventHandler(r, eventTracker, func() (int, int) {
		info, err := cfg.Engine.ContextUsage(sessionID)
		if err != nil || info == nil {
			return 0, 0
		}
		return info.TotalTokens, info.UsedTokens
	}, base)
	defer stopToolSpinners()
	defer func() {
		if epoch == 0 {
			return
		}
		settleTUIQueryViewAfterUpdates(tuiApp, eventTracker)
		if persistErr := persistSettledTUISessionLifecycleForApp(cfg, tuiApp); persistErr != nil {
			r.Warning(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUILifecycleSaveFailed, persistErr))
		}
	}()
	var runErr error
	providerRequestCompleted := false
	terminalContext := base
	for event := range ch {
		if event.Final {
			runErr = event.Error
			continue
		}
		switch event.Inner.Type {
		case loop.EventRequestStart, loop.EventRequestFailed:
			providerRequestCompleted = false
		case loop.EventRequestEnd:
			providerRequestCompleted = true
		}
		onEvent(event.Inner)
	}
	if epoch != 0 {
		if committed, generationErr := commitTUIContextGeneration(r, cfg.Engine, base, target.SessionProjectDir); generationErr != nil && runErr == nil {
			runErr = generationErr
		} else if generationErr == nil {
			terminalContext.ContextGeneration = committed.Generation
			terminalContext.ContextGenerationPersisted = committed.Persisted
		}
	}
	stopToolSpinners()
	if tuiApp.State().AdmitRuntimeGeneration(epoch, terminalContext.ContextGeneration, terminalContext.ContextGenerationPersisted) {
		tuiApp.State().FinalizeStream()
	}
	if runErr != nil {
		if errors.Is(runErr, engine.ErrSessionDeleted) {
			r.InfoAtContext(terminalContext, i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLBackgroundDiscarded, notification.TaskID))
			return nil
		}
		if errors.Is(runErr, context.Canceled) {
			r.InfoAtContext(terminalContext, i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIBackgroundCancelled))
		} else {
			r.ErrorAtContext(terminalContext, i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIBackgroundFailed, runErr))
			if status, update := terminalTUIProviderStatus(runErr, providerRequestCompleted); update {
				r.SetProviderStatusAtContext(terminalContext, status)
			}
		}
		return runErr
	}
	if usageEvent, ok := backgroundNotificationUsageEvent(notification); ok && eventTracker != nil {
		onEvent(usageEvent)
	}
	if snapshot, ok := cfg.BackgroundTasks.ResolveNotificationTarget(notification); ok && snapshot.Type == "local_agent" {
		_ = tuiApp.UpdateSync(func() {
			_ = tuiApp.State().AcknowledgeActivity("background:" + snapshot.ID)
		})
	}
	r.SetProviderStatusAtContext(terminalContext, tui.StatusConnected)
	return nil
}

func sameTUIProjectRoot(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && right != "" && filepath.Clean(left) == filepath.Clean(right)
}

func backgroundFollowUpProjectMatches(manager *tools.BackgroundTaskManager, targetProjectRoot string) bool {
	return manager != nil && sameTUIProjectRoot(targetProjectRoot, manager.CurrentProjectRoot())
}

func bindTaskCreateViewState(tool *tools.TaskCreateTool, target any) func() {
	if tool == nil || target == nil {
		return func() {}
	}
	var state *tui.AppState
	update := func(fn func()) { fn() }
	switch typed := target.(type) {
	case *tui.App:
		state = typed.State()
		update = func(fn func()) { typed.UpdateSync(fn) }
	case *tui.AppState:
		state = typed
	default:
		return func() {}
	}
	convert := func(items []tools.TaskViewItem) []tui.TaskViewItem {
		out := make([]tui.TaskViewItem, len(items))
		for i, item := range items {
			out[i] = tui.TaskViewItem{
				ID: item.ID, Subject: item.Subject, Status: item.Status, Owner: item.Owner,
				BlockedBy: append([]string(nil), item.BlockedBy...),
			}
		}
		return out
	}
	if len(state.TaskViewItems.Get()) == 0 {
		update(func() { state.RefreshTasksView(convert(tool.TaskViewSnapshot())) })
	}
	unsubscribe := tool.Store.Subscribe(func(tools.TaskStoreEvent) error {
		update(func() { state.RefreshTasksView(convert(tool.TaskViewSnapshot())) })
		return nil
	})
	tool.SetTaskViewNotifier(func(items []tools.TaskViewItem) {
		update(func() { state.RefreshTasksView(convert(items)) })
	})
	return func() {
		tool.SetTaskViewNotifier(nil)
		unsubscribe()
	}
}

func bindTUIAgentProgress(tool *tools.AgentTool, manager *tools.BackgroundTaskManager, app *tui.App) func() {
	if tool == nil || app == nil {
		return func() {}
	}
	return tool.SubscribeProgress(func(progress tools.AgentProgressEvent) {
		if strings.TrimSpace(progress.SessionID) == "" || strings.TrimSpace(progress.ParentToolUseID) == "" {
			return
		}
		// Retained agents already publish through BackgroundTaskSnapshot, which
		// preserves run fencing and continuation attempts. The direct bridge is
		// for one-shot/direct agents that never enter that manager.
		if manager != nil {
			if snapshot, ok := manager.Snapshot(progress.AgentID); ok && snapshot.Type == "local_agent" && snapshot.OwnerSessionID == progress.SessionID {
				return
			}
		}
		state := app.State()
		epoch := state.SessionEpoch.Get()
		if state.SessionID.Get() != progress.SessionID {
			return
		}
		app.GoTuiApp().QueueUpdateLossless(func() {
			if state.SessionID.Get() != progress.SessionID || state.SessionEpoch.Get() != epoch {
				return
			}
			parent, ok := state.GetActivity("tool:" + progress.ParentToolUseID)
			if !ok || parent.SessionID != progress.SessionID || parent.Epoch != epoch {
				return
			}
			_ = state.ApplyActivity(agentProgressActivityEventInLanguage(state.Language.Get(), progress, epoch))
		})
	})
}

func agentProgressActivityEventInLanguage(lang i18n.Language, progress tools.AgentProgressEvent, epoch uint64) tui.ActivityEvent {
	state, outcome := tui.ActivityRunning, tui.OutcomeRunning
	switch progress.Phase {
	case tools.AgentPhaseCompleted, tools.AgentPhaseBackground, tools.AgentPhaseRemoteLaunch:
		state, outcome = tui.ActivityCompleted, tui.OutcomeSucceeded
	case tools.AgentPhaseError:
		state, outcome = tui.ActivityFailed, tui.OutcomeFailed
	case tools.AgentPhaseAborted:
		state, outcome = tui.ActivityCancelled, tui.OutcomeCancelled
	}
	return tui.ActivityEvent{
		ID:        "tool:" + progress.ParentToolUseID,
		SessionID: progress.SessionID, Epoch: epoch, TurnID: progress.TurnID, WorkUnitID: progress.WorkUnitID,
		Kind: tui.ActivityAgent, Name: "Agent", Phase: tui.ActivityPhaseExecuting, State: state, Outcome: outcome,
		SourceSequence: progress.SourceSequence,
		Progress: tui.ActivityProgress{
			Current: progress.MessageCount, Message: backgroundAgentProgressMessageInLanguage(lang, progress),
			AgentID: progress.AgentID, AgentType: progress.AgentType, ParentToolUseID: progress.ParentToolUseID,
			Phase: string(progress.Phase), LatestTool: progress.LatestTool,
			Output: boundedAgentLiveOutput(progress.PartialText), ElapsedMs: progress.ElapsedMs,
			TokensUsed: progress.TokensUsed, Provider: progress.Provider, Model: progress.Model,
			Usage: cloneSubagentUsage(progress.Usage), LastRequestUsage: cloneSubagentUsage(progress.LastRequestUsage), DroppedCount: progress.DroppedCount,
		},
	}
}

func cloneSubagentUsage(usage *types.Usage) *types.Usage {
	if usage == nil {
		return nil
	}
	copy := *usage
	return &copy
}

func bindTUIBackgroundActivities(manager *tools.BackgroundTaskManager, app tuiActivityApp) func() {
	if manager == nil || app == nil {
		return func() {}
	}
	state := app.State()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	updates, unsubscribeSnapshots := manager.SubscribeSnapshots()
	lastSnapshotSignature := make(map[string]string)
	includePersisted := len(state.ActivityRunHistory()) == 0
	refresh := func() {
		sessionID := state.SessionID.Get()
		epoch := state.SessionEpoch.Get()
		lang := state.Language.Get()
		snapshots := manager.InMemorySnapshots()
		if includePersisted {
			snapshots = manager.Snapshots()
			includePersisted = false
		}
		for _, snapshot := range snapshots {
			if snapshot.OwnerSessionID == "" || snapshot.OwnerSessionID != sessionID {
				continue
			}
			if snapshot.OwnerProjectRoot == "" || !sameTUIProjectRoot(snapshot.OwnerProjectRoot, manager.CurrentProjectRoot()) {
				continue
			}
			// A freshly registered retained session is persisted as an idle,
			// completed shell before its first prompt is dequeued. The Agent tool
			// observation already owns that launch frame; projecting this empty
			// attempt would create a one-frame second Agent and false review badge.
			idleRegistration := snapshot.Attempt <= 0 && snapshot.CurrentRunID == ""
			idleLegacyRegistration := strings.HasPrefix(snapshot.CurrentRunID, "legacy:") && snapshot.LatestProgress == nil
			if snapshot.Type == "local_agent" && (idleRegistration || idleLegacyRegistration) &&
				strings.TrimSpace(snapshot.Status) == "completed" && strings.TrimSpace(snapshot.Result) == "" &&
				strings.TrimSpace(snapshot.Error) == "" {
				continue
			}
			signatureKey := fmt.Sprintf("%s:%d:%s", sessionID, epoch, snapshot.ID)
			signature := backgroundActivitySignature(snapshot)
			if lastSnapshotSignature[signatureKey] == signature {
				continue
			}
			lastSnapshotSignature[signatureKey] = signature
			name := snapshot.Description
			if strings.TrimSpace(name) == "" {
				name = snapshot.Command
			}
			if strings.TrimSpace(name) == "" {
				name = snapshot.Type
			}
			kind := tui.ActivityBackground
			actorType := "background"
			actorID := snapshot.OwnerAgentID
			if snapshot.Type == "local_agent" {
				kind = tui.ActivityAgent
				actorType = "agent"
				actorID = snapshot.ID
			}
			if actorID == "" {
				actorID = "assistant"
			}
			var transcript []byte
			expectsTranscript := snapshot.Type != "local_agent" && strings.TrimSpace(snapshot.TranscriptPath) != "" && activityStateIsTerminal(snapshot.Status)
			if expectsTranscript {
				if info, statErr := os.Lstat(snapshot.TranscriptPath); statErr == nil && info.Mode().IsRegular() {
					transcript, _ = os.ReadFile(snapshot.TranscriptPath)
				}
			}
			app.UpdateSync(func() {
				var detailRefs []tui.DetailRef
				transcriptRetained := false
				jumpTarget := "task:" + snapshot.ID
				parentToolUseID := ""
				if snapshot.LatestProgress != nil {
					parentToolUseID = strings.TrimSpace(snapshot.LatestProgress.ParentToolUseID)
				}
				activityEvidenceID := "background:" + snapshot.ID
				if snapshot.CurrentRunID != "" {
					activityEvidenceID += ":" + snapshot.CurrentRunID
				}
				if evidence := strings.TrimSpace(snapshot.Result + "\n" + snapshot.Error); snapshot.Type != "local_agent" && evidence != "" {
					if ref, err := state.RetainDetailForEpoch(sessionID, epoch, activityEvidenceID+":"+snapshot.Status, []byte(evidence)); err == nil {
						detailRefs = append(detailRefs, ref)
						if parentToolUseID != "" {
							if observationID, attachErr := state.AttachToolObservationDetailForEpoch(sessionID, epoch, parentToolUseID, ref); attachErr == nil {
								jumpTarget = observationID
							}
						}
					}
				}
				if len(transcript) > 0 {
					if ref, retainErr := state.RetainDetailForEpoch(sessionID, epoch, activityEvidenceID+":transcript", transcript); retainErr == nil {
						detailRefs = append(detailRefs, ref)
						if parentToolUseID != "" {
							if observationID, attachErr := state.AttachToolObservationDetailForEpoch(sessionID, epoch, parentToolUseID, ref); attachErr == nil {
								jumpTarget = observationID
							}
						}
						transcriptRetained = true
					}
				}
				if snapshot.Type == "local_agent" && activityStateIsTerminal(snapshot.Status) && parentToolUseID != "" {
					_, conclusionOutcome := backgroundActivityTerminalState(snapshot)
					conclusion := firstSemanticString(snapshot.Result, snapshot.Error)
					if observationID, updateErr := state.UpdateToolObservationAgentResultForEpoch(sessionID, epoch, parentToolUseID, conclusion, conclusionOutcome); updateErr == nil {
						jumpTarget = observationID
					}
				}
				event := backgroundActivityEventInLanguage(lang, snapshot, sessionID, epoch, name, actorType, actorID, detailRefs, jumpTarget, transcriptRetained, expectsTranscript && !transcriptRetained)
				event.Kind = kind
				_ = state.ApplyActivity(event)
			})
		}
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		refresh()
		// The long fallback covers external durable-store edits; normal live
		// updates arrive through the manager subscription without directory polls.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-updates:
				refresh()
			case <-ticker.C:
				manager.ReconcileInterruptedAgentRecords()
				refresh()
			case <-stop:
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			wg.Wait()
			unsubscribeSnapshots()
		})
	}
}

// bindTUIActivityPersistence writes the one durable session-view checkpoint
// shortly after any semantic view transition. The name is retained for call
// site compatibility, but the revision token deliberately covers messages,
// observation disclosure, interaction state, mode, activities, and overlays;
// adding a new visible surface must extend that single token rather than add a
// resume/fork-specific persistence path.
func bindTUIActivityPersistence(cfg TUIREPLConfig, app tuiActivityApp) func() {
	if app == nil || cfg.Repo == nil || cfg.SessionID == nil {
		return func() {}
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	lastRevision := app.State().SessionLifecycleRevision()
	// Establish the current settled transcript as an exact boundary before the
	// user can start another query. This also performs the one-time migration
	// for a legacy session; older transcript prefixes remain legacy-compatible,
	// while this and every future boundary are fail-closed.
	if err := persistTUISessionLifecycleForApp(cfg, app); err != nil {
		lastRevision.Messages = -1
	} else {
		lastRevision = app.State().SessionLifecycleRevision()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		lastPersistedAt := time.Time{}
		persist := func(revision tui.SessionLifecycleRevision) {
			if app.State().HasActiveQuery() {
				return
			}
			if cfg.SessionTransitionMu != nil {
				cfg.SessionTransitionMu.Lock()
			}
			err := persistTUISessionLifecycleForApp(cfg, app)
			if cfg.SessionTransitionMu != nil {
				cfg.SessionTransitionMu.Unlock()
			}
			if err == nil {
				lastRevision = revision
				lastPersistedAt = time.Now()
			}
		}
		for {
			select {
			case <-ticker.C:
				revision := app.State().SessionLifecycleRevision()
				if revision == lastRevision {
					continue
				}
				interval := activityPersistenceInterval(app.State().ActivityRunCount())
				if !lastPersistedAt.IsZero() && time.Since(lastPersistedAt) < interval {
					continue
				}
				persist(revision)
			case <-stop:
				if revision := app.State().SessionLifecycleRevision(); revision != lastRevision {
					persist(revision)
				}
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			wg.Wait()
		})
	}
}

func activityPersistenceInterval(runCount int) time.Duration {
	switch {
	case runCount >= 50_000:
		return 5 * time.Second
	case runCount >= 10_000:
		return 2 * time.Second
	case runCount >= 1_000:
		return time.Second
	default:
		return 250 * time.Millisecond
	}
}

func activityStateIsTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "killed", "cancelled":
		return true
	default:
		return false
	}
}

func backgroundActivitySignature(snapshot tools.BackgroundTaskSnapshot) string {
	progressSequence := uint64(0)
	progressPhase := tools.AgentProgressPhase("")
	progressDetail := ""
	if snapshot.LatestProgress != nil {
		progressSequence = snapshot.LatestProgress.SourceSequence
		progressPhase = snapshot.LatestProgress.Phase
		progressDetail = snapshot.LatestProgress.Detail + "\x00" + snapshot.LatestProgress.LatestTool
	}
	transcriptFingerprint := ""
	if path := strings.TrimSpace(snapshot.TranscriptPath); path != "" {
		if info, err := os.Lstat(path); err == nil {
			transcriptFingerprint = fmt.Sprintf("%d:%d:%s", info.Size(), info.ModTime().UnixNano(), info.Mode())
		} else {
			transcriptFingerprint = "unavailable"
		}
	}
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%d\x00%s\x00%t",
		snapshot.Status, snapshot.Outcome, snapshot.TerminalReason, snapshot.CurrentRunID, snapshot.Attempt, snapshot.Result, snapshot.Error,
		progressPhase, progressSequence, progressDetail, snapshot.TranscriptPath, transcriptFingerprint, snapshot.QueuedPrompts, snapshot.QueueReason, snapshot.Detached)
}

func backgroundActivityEvent(snapshot tools.BackgroundTaskSnapshot, sessionID string, epoch uint64, name, actorType, actorID string, detailRefs []tui.DetailRef, jumpTarget string, transcriptState ...bool) tui.ActivityEvent {
	return backgroundActivityEventInLanguage(i18n.DetectOrLoadLanguage(), snapshot, sessionID, epoch, name, actorType, actorID, detailRefs, jumpTarget, transcriptState...)
}

func backgroundActivityEventInLanguage(lang i18n.Language, snapshot tools.BackgroundTaskSnapshot, sessionID string, epoch uint64, name, actorType, actorID string, detailRefs []tui.DetailRef, jumpTarget string, transcriptState ...bool) tui.ActivityEvent {
	activityState, outcome := backgroundActivityTerminalState(snapshot)
	event := tui.ActivityEvent{
		ID: "background:" + snapshot.ID, RunID: snapshot.CurrentRunID, Attempt: snapshot.Attempt,
		BatchID: snapshot.BatchID, ParentRunID: snapshot.ParentRunID, AgentPath: snapshot.AgentPath,
		SessionID: sessionID, Epoch: epoch, WorkUnitID: snapshot.ID,
		Actor: tui.ActivityActor{ID: actorID, Type: actorType}, Kind: tui.ActivityBackground,
		Name: name, Phase: backgroundActivityPhase(snapshot, name, actorType), State: activityState, Outcome: outcome,
		Control: tui.ActivityControl{Cancelable: activityState == tui.ActivityRunning, JumpTarget: jumpTarget, DetailRefs: detailRefs},
	}
	if progress := snapshot.LatestProgress; progress != nil && (snapshot.CurrentRunID == "" || progress.RunID == snapshot.CurrentRunID) {
		event.SourceSequence = progress.SourceSequence
		event.Progress.Current = progress.MessageCount
		event.Progress.Message = backgroundAgentProgressMessageInLanguage(lang, *progress)
		event.Progress.AgentID = strings.TrimSpace(progress.AgentID)
		if event.Progress.AgentID == "" {
			event.Progress.AgentID = snapshot.ID
		}
		event.Progress.AgentType = progress.AgentType
		event.Progress.ParentToolUseID = progress.ParentToolUseID
		event.Progress.Phase = string(progress.Phase)
		event.Progress.LatestTool = progress.LatestTool
		event.Progress.Output = boundedAgentLiveOutput(progress.PartialText)
		event.Progress.ElapsedMs = progress.ElapsedMs
		event.Progress.TokensUsed = progress.TokensUsed
		event.Progress.Provider = progress.Provider
		event.Progress.Model = progress.Model
		event.Progress.Usage = cloneSubagentUsage(progress.Usage)
		event.Progress.LastRequestUsage = cloneSubagentUsage(progress.LastRequestUsage)
		event.Progress.DroppedCount = progress.DroppedCount
	}
	if snapshot.Usage != nil {
		event.Progress.Usage = cloneSubagentUsage(snapshot.Usage)
	}
	metrics := make([]string, 0, 3)
	if snapshot.DurationMs != nil && *snapshot.DurationMs >= 0 {
		metrics = append(metrics, formatBackgroundDuration(*snapshot.DurationMs))
	}
	if snapshot.TotalTokens != nil {
		metrics = append(metrics, i18n.Format(lang, i18n.KeyREPLTUITokenCount, *snapshot.TotalTokens))
	}
	transcriptRetained := len(transcriptState) > 0 && transcriptState[0]
	transcriptUnavailable := len(transcriptState) > 1 && transcriptState[1]
	if transcriptRetained {
		metrics = append(metrics, i18n.Text(lang, i18n.KeyREPLTUIThreadRetained))
	} else if transcriptUnavailable {
		metrics = append(metrics, i18n.Text(lang, i18n.KeyREPLTUIThreadUnavailable))
		event.Attention = tui.ActivityAttention{Kind: tui.ActivityAttentionWarning, Severity: tui.ActivityAttentionSeverityWarning, Unread: true, Message: i18n.Text(lang, i18n.KeyREPLTUITranscriptUnretained)}
	}
	if snapshot.QueuedPrompts > 0 {
		queue := i18n.Format(lang, i18n.KeyREPLTUIQueuedCount, snapshot.QueuedPrompts)
		if reason := strings.TrimSpace(snapshot.QueueReason); reason != "" {
			queue = i18n.Format(lang, i18n.KeyREPLTUIQueuedReason, queue, i18n.RootAgentQueueReasonLabel(lang, reason))
		}
		metrics = append(metrics, queue)
	}
	if reason := strings.TrimSpace(snapshot.TerminalReason); reason != "" && activityStateIsTerminal(snapshot.Status) {
		metrics = append(metrics, i18n.Format(lang, i18n.KeyREPLTUITerminalReason, i18n.RootAgentTerminalReasonLabel(lang, reason)))
	}
	if count := len(snapshot.ArtifactRefs); count > 0 {
		metrics = append(metrics, i18n.Format(lang, i18n.KeyREPLTUIArtifactCount, count))
	}
	if count := len(snapshot.VerificationRefs); count > 0 {
		metrics = append(metrics, i18n.Format(lang, i18n.KeyREPLTUIVerificationCount, count))
	}
	if len(metrics) > 0 {
		if event.Progress.Message != "" {
			event.Progress.Message += " | "
		}
		event.Progress.Message += strings.Join(metrics, " | ")
	}
	return event
}

const maxAgentLiveOutputRunes = 2400

func boundedAgentLiveOutput(value string) string {
	value = tui.RedactPresentationText(value, 0)
	runes := []rune(value)
	if len(runes) > maxAgentLiveOutputRunes {
		value = string(runes[len(runes)-maxAgentLiveOutputRunes:])
	}
	return value
}

func backgroundAgentProgressMessage(progress tools.AgentProgressEvent) string {
	return backgroundAgentProgressMessageInLanguage(i18n.DetectOrLoadLanguage(), progress)
}

func backgroundAgentProgressMessageInLanguage(lang i18n.Language, progress tools.AgentProgressEvent) string {
	parts := make([]string, 0, 4)
	if progress.Phase != "" {
		parts = append(parts, localizedBackgroundAgentStatus(lang, string(progress.Phase)))
	}
	if tool := strings.TrimSpace(progress.LatestTool); tool != "" {
		parts = append(parts, i18n.Format(lang, i18n.KeyREPLTUIToolName, tool))
	}
	if detail := strings.TrimSpace(tui.RedactPresentationText(progress.Detail, 120)); detail != "" {
		parts = append(parts, detail)
	}
	if progress.ElapsedMs > 0 {
		parts = append(parts, formatBackgroundDuration(progress.ElapsedMs))
	}
	if progress.TokensUsed > 0 {
		parts = append(parts, i18n.Format(lang, i18n.KeyREPLTUITokenCount, progress.TokensUsed))
	}
	if !progress.Timestamp.IsZero() {
		parts = append(parts, i18n.Format(lang, i18n.KeyREPLTUIUpdatedAt, progress.Timestamp.UTC().Format("15:04:05Z")))
	}
	message := strings.Join(parts, " | ")
	const maxRunes = 180
	runes := []rune(message)
	if len(runes) > maxRunes {
		message = string(runes[:maxRunes-3]) + "..."
	}
	return message
}

func formatBackgroundDuration(milliseconds int64) string {
	if milliseconds < 1000 {
		return fmt.Sprintf("%dms", milliseconds)
	}
	return (time.Duration(milliseconds) * time.Millisecond).Round(100 * time.Millisecond).String()
}

func backgroundActivityPhase(snapshot tools.BackgroundTaskSnapshot, name, actorType string) tui.ActivityPhase {
	return tui.ActivityPhaseForTool(snapshot.Type+" "+name, map[string]any{"command": snapshot.Command}, actorType)
}

func backgroundActivityTerminalState(snapshot tools.BackgroundTaskSnapshot) (tui.ActivityState, tui.ObservationOutcome) {
	switch snapshot.Outcome {
	case tools.AgentRunOutcomeSucceeded:
		if snapshot.Type == "local_agent" && snapshot.Detached {
			return tui.ActivityReadyReview, tui.OutcomeSucceeded
		}
		return tui.ActivityCompleted, tui.OutcomeSucceeded
	case tools.AgentRunOutcomePartial:
		return tui.ActivityFailed, tui.OutcomePartial
	case tools.AgentRunOutcomeCancelled:
		return tui.ActivityCancelled, tui.OutcomeCancelled
	case tools.AgentRunOutcomeTimedOut:
		return tui.ActivityCancelled, tui.OutcomeTimedOut
	case tools.AgentRunOutcomeInterrupted:
		return tui.ActivityFailed, tui.OutcomeOrphan
	case tools.AgentRunOutcomeFailed:
		return tui.ActivityFailed, tui.OutcomeFailed
	}
	switch snapshot.Status {
	case "completed":
		if snapshot.Type == "local_agent" && snapshot.Detached {
			return tui.ActivityReadyReview, tui.OutcomeSucceeded
		}
		return tui.ActivityCompleted, tui.OutcomeSucceeded
	case "killed", "cancelled":
		return tui.ActivityCancelled, tui.OutcomeCancelled
	case "failed":
		return tui.ActivityFailed, tui.OutcomeFailed
	default:
		return tui.ActivityRunning, tui.OutcomeRunning
	}
}

// handleTUIInput processes a single user input in TUI mode.
// It runs in a separate goroutine to avoid blocking the TUI event loop.
func handleTUIInput(
	ctx context.Context,
	cfg TUIREPLConfig,
	tuiApp *tui.App,
	cmdReg *commands.Registry,
	storeAdapter *sessionStoreAdapter,
	ql *engineQueryLooper,
	tracker *ui.CostTracker,
	inputStr string,
	sigHandler *SignalHandler,
	runTracked ...func(func()) bool,
) {
	handleTUIInputSubmission(ctx, cfg, tuiApp, cmdReg, storeAdapter, ql, tracker, tuiInputSubmission{text: inputStr}, sigHandler, runTracked...)
}

func handleTUIInputSubmission(
	ctx context.Context,
	cfg TUIREPLConfig,
	tuiApp *tui.App,
	cmdReg *commands.Registry,
	storeAdapter *sessionStoreAdapter,
	ql *engineQueryLooper,
	tracker *ui.CostTracker,
	submission tuiInputSubmission,
	sigHandler *SignalHandler,
	runTracked ...func(func()) bool,
) {
	r := tuiApp.Renderer()
	deferredActionCtx := submission.lifecycleCtx
	if deferredActionCtx == nil {
		deferredActionCtx = ctx
	}

	inputStr := submission.text
	rawInput := inputStr
	inputStr = strings.TrimSpace(inputStr)
	pendingImageCount := len(tuiApp.State().PendingImages.Get())
	if submission.imagesCaptured {
		pendingImageCount = len(submission.images)
	}
	if inputStr == "" && pendingImageCount == 0 {
		return
	}

	appendUserMessage := func(images []tui.ImageAttachment) {
		userMsg := tui.Message{
			Kind:      tui.MsgUser,
			Text:      inputStr,
			Timestamp: time.Now(),
		}
		if len(images) > 0 {
			userMsg.Images = images
		}
		tuiApp.State().AppendMessage(userMsg)
	}

	if inputStr == "exit" || inputStr == "quit" {
		appendUserMessage(nil)
		r.Goodbye()
		return
	}

	transitionSessionAt := func(actionCtx context.Context, entry commands.SessionListEntry) error {
		return transitionTUISession(actionCtx, cfg, tuiApp, storeAdapter, entry)
	}
	transitionSession := func(entry commands.SessionListEntry) error {
		return transitionSessionAt(ctx, entry)
	}

	clearConversation := func() error {
		_, err := clearTUIConversation(ctx, cfg, tuiApp)
		return err
	}

	// Build command context for slash commands.
	// Command output goes via r.Info() so it renders as system info
	// rather than assistant text.
	buildCmdCtx := func() *commands.Context {
		totalIn, totalOut := tracker.TotalTokens()
		cacheRead, cacheMake := tracker.TotalCacheTokens()
		webSearchRequests := tracker.TotalWebSearchRequests()
		var sessionCostBreakdown *cost.CostBreakdown
		if breakdown, complete := tracker.TotalCostBreakdown(); complete {
			sessionCostBreakdown = &breakdown
		}
		sessionID := *cfg.SessionID
		projectDir := currentProjectDir(cfg)
		commandRunID := "command-run-" + uuid.NewString()
		var commandCrossedSessionBoundary atomic.Bool
		// Command output belongs to the session in which the command started.
		// A successful /resume or /clear changes identity mid-command; appending
		// its receipt to the target would mutate the exact restored view and make
		// repeated resumes accumulate durable noise.
		sessionScopedOutput := newSessionScopedTUICommandOutput(tuiApp.State(), sessionID, commandCrossedSessionBoundary.Load, func(s string) {
			r.Info(commands.RedactCommandPresentationText(s, 0))
		})
		return &commands.Context{
			Language:              tuiApp.State().Language.Get(),
			QueryLoop:             ql,
			OnEvent:               sessionScopedOutput,
			OnCommandPresentation: newTUICommandPresentationSink(tuiApp, sessionID, tuiApp.State().SessionEpoch.Get(), commandRunID, sessionScopedOutput),
			CWD:                   currentCWD(cfg),
			CurrentProjectDir:     projectDir,
			SessionID:             sessionID,
			SetSessionID:          func(id string) { *cfg.SessionID = id },
			SessionStore:          storeAdapter,
			GoalRuntime:           newSessionGoalRuntime(cfg, sessionID, projectDir),
			BuildDiagnostic:       currentBuildDiagnostic(cfg),
			OpenFileEditor:        tuiApp.OpenFileInEditor,
			ResumeSession: func(entry commands.SessionListEntry) error {
				err := transitionSession(entry)
				if err == nil {
					commandCrossedSessionBoundary.Store(true)
				}
				return err
			},
			ClearView: func() error {
				if !tuiApp.UpdateSync(tuiApp.State().ClearView) {
					return errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIStopped))
				}
				return nil
			},
			ClearConversation: func() error {
				err := clearConversation()
				if err == nil {
					commandCrossedSessionBoundary.Store(true)
				}
				return err
			},
			OpenForkPicker: func() error {
				if hasConflictingTUIQuery(ctx, tuiApp.State()) {
					return errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIForkRunning))
				}
				messages := ql.Messages()
				entries := availableConversationForkEntries(messages, tuiApp.State().Language.Get(), r.Warning)
				if len(entries) == 0 {
					return nil
				}
				picker := &tui.ForkPickerState{
					Visible: true, Entries: entries, Selected: 0,
					OnCancel: func() { r.Info(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIForkCancelled)) },
					OnSelect: func(entry tui.ForkEntry) {
						run := func(fn func()) bool {
							if len(runTracked) > 0 && runTracked[0] != nil {
								return runTracked[0](fn)
							}
							go fn()
							return true
						}
						run(func() {
							withTUICommandLock(cfg, func() {
								cfg.SessionTransitionMu.Lock()
								defer cfg.SessionTransitionMu.Unlock()
								if hasConflictingTUIQuery(ctx, tuiApp.State()) {
									r.Warning(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIForkRunning))
									return
								}
								if current := ql.Messages(); !reflect.DeepEqual(current, messages) {
									r.Warning(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIForkSnapshotChanged))
									return
								}
								fork, err := forkSessionFromSnapshotWithApp(deferredActionCtx, cfg, tuiApp, messages, entry.MessageEnd)
								if err != nil {
									r.Error(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIForkFailed, err))
									return
								}
								r.Success(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIForkOpened, fork.ID))
							})
						})
					},
				}
				if !tuiApp.UpdateSync(func() { tuiApp.State().ForkPicker.Set(picker) }) {
					return errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIStopped))
				}
				return nil
			},
			SearchTranscript: func(query string) (string, error) {
				cfg.SessionTransitionMu.Lock()
				defer cfg.SessionTransitionMu.Unlock()
				var match tui.TranscriptSearchMatch
				var count int
				var ok bool
				var err error
				closed := false
				if query != "--next" && query != "--previous" && query != "--close" {
					prepared, prepareErr := tuiApp.State().PrepareTranscriptSearch(query)
					if prepareErr != nil {
						return "", prepareErr
					}
					if !tuiApp.UpdateSync(func() {
						match, count, ok, err = tuiApp.State().PublishTranscriptSearch(prepared)
					}) {
						return "", errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIStopped))
					}
				} else if !tuiApp.UpdateSync(func() {
					switch query {
					case "--next":
						match, count, ok, err = tuiApp.State().MoveTranscriptSearch(1)
					case "--previous":
						match, count, ok, err = tuiApp.State().MoveTranscriptSearch(-1)
					case "--close":
						tuiApp.State().CloseTranscriptSearch()
						closed = true
					}
				}) {
					return "", errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIStopped))
				}
				if closed {
					return i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUITranscriptClosed), nil
				}
				if err != nil {
					return "", err
				}
				if !ok {
					return i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUITranscriptNoMatches), nil
				}
				return i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUITranscriptMatch, count, match.ObservationID), nil
			},
			ExportTranscript: func(path string) (string, error) {
				cfg.SessionTransitionMu.Lock()
				defer cfg.SessionTransitionMu.Unlock()
				if strings.TrimSpace(path) == "" {
					path = filepath.Join(currentCWD(cfg), "transcript-"+*cfg.SessionID+".txt")
				} else if !filepath.IsAbs(path) {
					path = filepath.Join(currentCWD(cfg), path)
				}
				observations, details, presentation := tuiApp.State().TranscriptResources()
				exporter := withTUITranscriptControlScope(cfg, tui.NewTranscriptExporter(observations, details, ql.Messages()).WithPresentation(presentation).WithDecisions(tuiApp.State().DecisionHistory.Get()).WithLanguage(tuiApp.State().Language.Get()))
				if err := exporter.Export(path, tui.TranscriptExportHumanReadable); err != nil {
					return "", err
				}
				return path, nil
			},
			OpenTranscriptEditor: func(path string) error {
				cfg.SessionTransitionMu.Lock()
				defer cfg.SessionTransitionMu.Unlock()
				if strings.TrimSpace(path) == "" {
					root := filepath.Join(os.TempDir(), "luban-code", *cfg.SessionID)
					if cfg.Repo != nil {
						root = cfg.Repo.ArtifactsDir(*cfg.SessionID, currentProjectDir(cfg))
					}
					path = filepath.Join(root, "transcript.txt")
				} else if !filepath.IsAbs(path) {
					path = filepath.Join(currentCWD(cfg), path)
				}
				observations, details, presentation := tuiApp.State().TranscriptResources()
				exporter := withTUITranscriptControlScope(cfg, tui.NewTranscriptExporter(observations, details, ql.Messages()).WithPresentation(presentation).WithDecisions(tuiApp.State().DecisionHistory.Get()).WithLanguage(tuiApp.State().Language.Get()))
				if err := exporter.Export(path, tui.TranscriptExportHumanReadable); err != nil {
					return err
				}
				return tuiApp.OpenFileInEditor(path)
			},
			OpenDetailEditor: func(observationID string) error {
				cfg.SessionTransitionMu.Lock()
				defer cfg.SessionTransitionMu.Unlock()
				evidence, err := tuiApp.State().ObservationEvidence(observationID)
				if err != nil {
					return err
				}
				root := filepath.Join(os.TempDir(), "luban-code", *cfg.SessionID)
				if cfg.Repo != nil {
					root = cfg.Repo.ArtifactsDir(*cfg.SessionID, currentProjectDir(cfg))
				}
				if err := os.MkdirAll(root, 0o700); err != nil {
					return err
				}
				path := filepath.Join(root, "detail-"+uuid.NewString()+".txt")
				if err := os.WriteFile(path, evidence, 0o600); err != nil {
					return err
				}
				return tuiApp.OpenFileInEditor(path)
			},
			OpenModelPicker: func() error {
				if getCatalog(cfg) == nil {
					return errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIModelCatalogMissing))
				}
				buildCascadingPicker(cfg, tuiApp, r, ql, tracker)
				return nil
			},
			SetMouseCapture: func(mode string) (bool, error) {
				enabled := tuiApp.MouseEnabled()
				switch mode {
				case "on":
					enabled = true
				case "off":
					enabled = false
				case "toggle":
					enabled = !enabled
				default:
					return enabled, errors.New(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIMouseModeInvalid, mode))
				}
				if !tuiApp.UpdateSync(func() { tuiApp.SetMouseEnabled(enabled) }) {
					return enabled, errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIStopped))
				}
				return enabled, nil
			},
			OpenActivityView: func() string {
				var snapshot tui.ActivitySnapshot
				if !tuiApp.UpdateSync(func() { snapshot = tuiApp.State().ExpandActivityView() }) {
					return i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIStopped)
				}
				return i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIActivityViewOpened, snapshot.Counts.Total, snapshot.Counts.Running, snapshot.Counts.NeedsInput)
			},
			CloseActivityView: func() string {
				tuiApp.UpdateSync(func() {
					tuiApp.State().SetExpandedView("")
					tuiApp.State().ActivityFocus.Set("")
				})
				return i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIActivityViewClosed)
			},
			ActivityAction: func(id, action string) (string, error) {
				return performTUIActivityActionInLanguage(tuiApp.State().Language.Get(), tuiApp, cfg.BackgroundTasks, id, action)
			},
			SetDisclosure: func(id, level string) (string, error) {
				var selected tui.DisclosureLevel
				var disclosureErr error
				switch strings.ToLower(level) {
				case "next":
					if !tuiApp.UpdateSync(func() { selected, disclosureErr = tuiApp.State().CycleObservationDisclosure(id) }) {
						return "", errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIStopped))
					}
				case "summary":
					selected = tui.DisclosureSummary
				case "detail":
					selected = tui.DisclosureDetail
				case "evidence":
					selected = tui.DisclosureEvidence
				default:
					return "", errors.New(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIDisclosureUnknown, level))
				}
				if strings.ToLower(level) != "next" {
					if !tuiApp.UpdateSync(func() { disclosureErr = tuiApp.State().RevealObservation(id, selected) }) {
						return "", errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIStopped))
					}
				}
				if disclosureErr != nil {
					return "", disclosureErr
				}
				return i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIDisclosureReceipt, id, selected), nil
			},
			DeleteHistory: func(sessionID string) error {
				deleter, _ := cfg.Engine.(engine.SessionHistoryDeleter)
				return deleteTUISessionHistory(ctx, cfg.Repo, deleter, cfg.SessionTransitionMu, r,
					func() string { return *cfg.SessionID }, currentProjectDir(cfg), sessionID)
			},
			AppVersion:               cli.Version,
			CurrentModel:             ql.Model(),
			MCPBackend:               cfg.MCPBackend,
			SkillManager:             cfg.SkillManager,
			SkillInvoker:             cfg.SkillInvoker,
			TotalInputTokens:         totalIn,
			TotalOutputTokens:        totalOut,
			TotalCacheReadTokens:     cacheRead,
			TotalCacheCreationTokens: cacheMake,
			TotalWebSearchRequests:   webSearchRequests,
			TotalCostUSD:             tracker.TotalCost(),
			CostUnknown:              !tracker.CostKnown(),
			SessionCostBreakdown:     sessionCostBreakdown,
			Confirm: func(prompt string) bool {
				// In TUI mode, use the permission dialog infrastructure
				// to ask a simple y/n question.
				r.Info(prompt)
				// For now, auto-approve clipboard paste in TUI mode
				// (the user explicitly ran /paste, so intent is clear).
				return true
			},
			CompactFunc: func(customInstructions string) error {
				if cfg.SessionTransitionMu != nil {
					cfg.SessionTransitionMu.Lock()
					defer cfg.SessionTransitionMu.Unlock()
				}
				base := ui.ToolEventContext{
					SessionID: *cfg.SessionID, SessionEpoch: tuiApp.State().SessionEpoch.Get(),
					ContextGeneration: tuiApp.State().ContextGeneration.Get(), ContextGenerationPersisted: tuiApp.State().ContextGenerationPersisted.Get(),
					ProjectRoot: currentRuntimeProjectRoot(cfg),
				}
				handle, cleanup := makeTUIEventHandler(r, tracker, func() (int, int) { return ql.ContextUsage() }, base)
				defer cleanup()
				if err := runManualCompactionEventsInLanguage(ctx, cfg.Engine, *cfg.SessionID, customInstructions, tuiApp.State().Language.Get(), handle); err != nil {
					return err
				}
				if _, err := commitTUIContextGeneration(r, cfg.Engine, base, currentProjectDir(cfg)); err != nil {
					return err
				}
				settleTUIQueryViewAfterUpdates(tuiApp, tracker)
				return persistSettledTUISessionLifecycleForApp(cfg, tuiApp)
			},
			// Provider support (Phase 4)
			CurrentProvider:  cfg.Engine.Provider().Name(),
			ProviderRef:      cfg.ProviderRef,
			ProviderRegistry: cfg.ProviderRegistry,
			CredentialStore:  cfg.CredentialStore,
			// Multi-model cost tracking (Phase 9)
			PerModelCosts: convertPerModelCosts(tracker.PerModelCosts()),
			SwitchLanguage: func(code string) string {
				return switchTUILanguage(tuiApp, code)
			},
		}
	}

	refreshActiveSessionChrome := func(resetTracker bool) {
		runtimeProvider, runtimeModel := cfg.Engine.Provider().Name(), ql.Model()
		tracker.SetProviderAndModel(runtimeProvider, runtimeModel)
		displayProvider, displayModel := tuiApp.State().Provider.Get(), tuiApp.State().Model.Get()
		if strings.TrimSpace(displayProvider) == "" {
			displayProvider = runtimeProvider
		}
		if strings.TrimSpace(displayModel) == "" {
			displayModel = runtimeModel
		}
		r.Banner(displayProvider, displayModel)
		if cfg.BuildDiagnostic != nil {
			tuiApp.SetBuildDiagnostic(cfg.BuildDiagnostic(currentCWD(cfg)))
		}
		r.SetReasoningEffort(ql.ReasoningEffort())
		if resetTracker && cfg.RestoreSessionUsage == nil {
			usage := tuiApp.State().ActiveSessionUsage()
			tracker.RestoreSession(ql.Model(), usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheCreateTokens, usage.WebSearchRequests, usage.CumulativeCost, tuiApp.State().SessionCostKnown.Get())
			tracker.RestoreCompactionBaseline(usage.HasCompacted, usage.InputTokensAtCompact, usage.CacheReadAtCompact)
			restoreTrackerConversationUsage(tracker, usage)
		}
		r.SessionInfo(*cfg.SessionID, cfg.Engine.Tools())
		info, err := cfg.Engine.ContextUsage(*cfg.SessionID)
		if err != nil || info == nil || info.TotalTokens <= 0 {
			r.ContextBar(0, 0)
			return
		}
		r.EffectiveContext(effectiveContextFromEngine(info))
	}

	menuRequest := tui.SkillsMenuOpenRequest{
		SessionID: func() string {
			return tuiApp.State().SessionID.Get()
		},
		Language: func() i18n.Language { return tuiApp.State().Language.Get() },
		Backend:  cfg.SkillManager,
	}
	if handled, openErr := tui.RouteExactSkillsMenu(inputStr, cfg.SkillsMenuLauncher, menuRequest); handled {
		appendUserMessage(nil)
		if openErr != nil {
			lang := tuiApp.State().Language.Get()
			if errors.Is(openErr, tui.ErrSkillsMenuLauncherUnavailable) {
				r.Error(i18n.Text(lang, i18n.KeyTUISkillsMenuUnavailable))
			} else {
				r.Error(i18n.Format(lang, i18n.KeyTUISkillsMenuOpenFailed, openErr))
			}
		}
		return
	}

	// Slash-command dispatch
	if cmdReg.IsCommand(inputStr) {
		cmd, args := cmdReg.Parse(inputStr)
		if cmd == nil {
			cfg.SessionTransitionMu.Lock()
			defer cfg.SessionTransitionMu.Unlock()
			if hasConflictingTUIQuery(ctx, tuiApp.State()) {
				r.Warning(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIQueryRunning))
				return
			}
			if hookRunner := currentHookRunner(cfg); hookRunner != nil {
				hookContext := newREPLHookContext(*cfg.SessionID, hooks.HookUserPromptSubmit, "user", "local")
				hookContext.SessionEpoch = tuiApp.State().SessionEpoch.Get()
				result := runObservedREPLHooks(ctx, hookRunner, tuiREPLHookRenderer{app: tuiApp}, hooks.HookUserPromptSubmit, hooks.HookInput{UserInput: inputStr}, hookContext)
				if result.Blocked {
					r.Info(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIInputBlocked, result.Reason))
					return
				}
			}

			appendUserMessage(nil)
			submission := tui.InvokeUserSkillSlash(ctx, cfg.SkillManager, cfg.SkillInvoker, *cfg.SessionID, rawInput)
			if !submission.Successful() {
				r.Error(tui.FormatTUISkillSlashFailure(tuiApp.State().Language.Get(), submission))
				return
			}
			req := engine.QueryRequest{
				SessionID:         *cfg.SessionID,
				SessionProjectDir: currentProjectDir(cfg),
				Content: []types.ContentBlock{types.TextBlock{
					Type: types.ContentTypeText,
					Text: submission.ModelContent,
				}},
				InternalKind:              types.InternalMessageKindSkillInvocation,
				InternalControlCapability: messagecontrol.Runtime(),
				CWD:                       currentCWD(cfg),
				ProjectRoot:               currentRuntimeProjectRoot(cfg),
			}
			tuiRunQuery(ctx, cfg, tuiApp, req, tracker, sigHandler)
			return
		}

		cfg.CommandMu.Lock()
		commandLocked := true
		defer func() {
			if commandLocked {
				cfg.CommandMu.Unlock()
			}
		}()
		// Session navigation is UI control, not conversation history. Keeping
		// /resume, /fork, or /clear as a durable transcript row would mutate the
		// source just before snapshotting and leak the control command into the
		// restored or forked target view.
		if name := cmd.Name(); name != "resume" && name != "fork" && name != "clear" {
			appendUserMessage(nil)
		}
		prevSessionID := *cfg.SessionID
		cmdCtx := buildCmdCtx()
		var goalActivationObjective string
		cmdCtx.OnGoalActivated = func(objective string) {
			goalActivationObjective = strings.TrimSpace(objective)
		}
		if cmdErr := cmd.Execute(cmdCtx, args); cmdErr != nil {
			if errors.Is(cmdErr, commands.ErrExit) {
				r.Goodbye()
				return
			}
			if _, presented := cmd.(commands.CommandPresentationProvider); !presented {
				r.Error(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUICommandError, cmdErr))
			}
			return
		}

		// Post-command TUI state updates
		switch cmd.Name() {
		case "model":
			// /model may have changed the active model; update banner
			// Use ql.Model() which reflects SetModel(), not provider.ModelID()
			// which may still return the initial model.
			r.Banner(cfg.Engine.Provider().Name(), ql.Model())
			r.SetReasoningEffort(ql.ReasoningEffort())
			// Update cost tracker for the new model so future turns use correct pricing
			tracker.SetProviderAndModel(cfg.Engine.Provider().Name(), ql.Model())
		case "resume":
			if strings.TrimSpace(args) == "" {
				entries, _ := storeAdapter.List()
				pickerEntries := make([]tui.SessionEntry, len(entries))
				for i, e := range entries {
					msgs, _ := storeAdapter.LoadEntry(e)
					previewMsgs := make([]tui.Message, 0, len(msgs))
					for _, m := range msgs {
						if m.IsInternalRuntimeMessage() {
							continue
						}
						previewMsgs = append(previewMsgs, tui.Message{Text: m.GetText()})
					}
					pickerEntries[i] = tui.SessionEntry{
						ID:           e.ID,
						ProjectDir:   e.ProjectDir,
						Title:        e.Title,
						UpdatedAt:    e.UpdatedAt,
						CreatedAt:    e.CreatedAt,
						MessageCount: e.MessageCount,
						CWD:          e.CWD,
						GitBranch:    e.GitBranch,
						PreviewText:  e.PreviewText,
						Messages:     previewMsgs,
					}
				}
				tuiApp.State().SessionPicker.Set(&tui.SessionPickerState{
					Visible:  true,
					Entries:  pickerEntries,
					Selected: 0,
					OnCancel: func() { r.Info(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIResumeCancelled)) },
					OnSelect: func(entry tui.SessionEntry) {
						run := func(fn func()) bool {
							if len(runTracked) > 0 && runTracked[0] != nil {
								return runTracked[0](fn)
							}
							go fn()
							return true
						}
						run(func() {
							withTUICommandLock(cfg, func() {
								previousID := *cfg.SessionID
								if err := transitionSessionAt(deferredActionCtx, commands.SessionListEntry{
									ID:           entry.ID,
									ProjectDir:   entry.ProjectDir,
									Title:        entry.Title,
									UpdatedAt:    entry.UpdatedAt,
									CreatedAt:    entry.CreatedAt,
									MessageCount: entry.MessageCount,
									CWD:          entry.CWD,
									GitBranch:    entry.GitBranch,
									PreviewText:  entry.PreviewText,
								}); err != nil {
									if *cfg.SessionID != previousID {
										refreshActiveSessionChrome(true)
									}
									r.Error(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIResumeFailed, err))
									return
								}
								refreshActiveSessionChrome(true)
								// The changed banner/transcript is the success feedback. A
								// durable receipt here would pollute the restored target view.
							})
						})
					},
				})
			} else if *cfg.SessionID != prevSessionID {
				refreshActiveSessionChrome(true)
			}
		case "clear":
			if *cfg.SessionID != prevSessionID {
				refreshActiveSessionChrome(true)
			}
		case "goal":
			current, goalErr := cmdCtx.GoalRuntime.LoadGoal()
			if goalErr != nil {
				r.Warning(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIGoalRefreshFailed, goalErr))
				break
			}
			tuiApp.UpdateSync(func() {
				tuiApp.State().SetGoalView(tui.GoalViewFromGoal(current))
			})
		}
		if goalActivationObjective != "" {
			cfg.SessionTransitionMu.Lock()
			defer cfg.SessionTransitionMu.Unlock()
			cfg.CommandMu.Unlock()
			commandLocked = false

			if hasConflictingTUIQuery(ctx, tuiApp.State()) {
				r.Warning(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIQueryRunning))
				return
			}
			req, ok := buildGoalActivationQueryRequest(
				*cfg.SessionID,
				goalActivationObjective,
				currentCWD(cfg),
				currentRuntimeProjectRoot(cfg),
			)
			if ok {
				req.SessionProjectDir = currentProjectDir(cfg)
				tuiRunQuery(ctx, cfg, tuiApp, req, tracker, sigHandler)
			}
		}
		return
	}
	cfg.SessionTransitionMu.Lock()
	defer cfg.SessionTransitionMu.Unlock()
	if hasConflictingTUIQuery(ctx, tuiApp.State()) {
		r.Warning(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIQueryRunning))
		return
	}

	// UserPromptSubmit hook — may block the input
	if hookRunner := currentHookRunner(cfg); hookRunner != nil {
		hookContext := newREPLHookContext(*cfg.SessionID, hooks.HookUserPromptSubmit, "user", "local")
		hookContext.SessionEpoch = tuiApp.State().SessionEpoch.Get()
		result := runObservedREPLHooks(ctx, hookRunner, tuiREPLHookRenderer{app: tuiApp}, hooks.HookUserPromptSubmit, hooks.HookInput{UserInput: inputStr}, hookContext)
		if result.Blocked {
			r.Info(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIInputBlocked, result.Reason))
			return
		}
	}

	// Build query request (with image parsing support)
	req, parseErr := buildQueryRequest(*cfg.SessionID, inputStr)
	if parseErr != nil {
		r.Error(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIImageLoadFailed, parseErr))
		return
	}
	req.CWD = currentCWD(cfg)
	req.ProjectRoot = currentRuntimeProjectRoot(cfg)
	req.SessionProjectDir = currentProjectDir(cfg)

	pendingImages := tuiApp.State().PendingImages.Get()
	if submission.imagesCaptured {
		pendingImages = append([]tui.ImageAttachment(nil), submission.images...)
	}
	hasImages := len(pendingImages) > 0 || queryRequestHasImage(req)
	if hasImages && !currentModelSupportsImages(cfg, ql.Model()) {
		r.Warning(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIImageUnsupported))
		return
	}
	if len(pendingImages) > 0 && !submission.imagesCaptured {
		pendingImages = tuiApp.State().TakePendingImages()
	}
	appendUserMessage(pendingImages)

	// Inject pending clipboard images into the query as additional content blocks.
	// If the request already has Content blocks (from /image or @path parsing),
	// append the clipboard images. If it's a plain text request (Message only),
	// convert it to a Content-based request so images can be included.
	if len(pendingImages) > 0 {
		req = attachPendingImagesToQuery(req, pendingImages)
	}

	tuiRunQuery(ctx, cfg, tuiApp, req, tracker, sigHandler)
}

func attachPendingImagesToQuery(req engine.QueryRequest, images []tui.ImageAttachment) engine.QueryRequest {
	if len(images) == 0 {
		return req
	}
	if len(req.Content) == 0 {
		if req.Message != "" {
			req.Content = append(req.Content, types.TextBlock{Type: types.ContentTypeText, Text: req.Message})
		}
		req.Message = ""
	}
	for _, image := range images {
		req.Content = append(req.Content, types.ImageBlock{
			Type: types.ContentTypeImage,
			Source: &types.ImageSource{
				Type:      "base64",
				MediaType: image.MediaType,
				Data:      image.Base64,
			},
		})
	}
	return req
}

func newSessionScopedTUICommandOutput(state *tui.AppState, sessionID string, crossedBoundary func() bool, output func(string)) func(string) {
	return func(message string) {
		if state == nil || output == nil || state.SessionID.Get() != sessionID || (crossedBoundary != nil && crossedBoundary()) {
			return
		}
		output(message)
	}
}

func deleteTUISessionHistory(
	ctx context.Context,
	repo *session.Repository,
	deleter engine.SessionHistoryDeleter,
	transitionMu *sync.Mutex,
	requester tuiDecisionRequester,
	currentSession func() string,
	projectDir, sessionID string,
) error {
	if repo == nil {
		return replError(i18n.KeyREPLErrorSessionRepositoryUnavailable)
	}
	if transitionMu == nil || requester == nil || currentSession == nil {
		return replError(i18n.KeyREPLErrorDeletionBoundaryUnavailable)
	}
	transitionMu.Lock()
	defer transitionMu.Unlock()
	if sessionID == currentSession() {
		return replError(i18n.KeyREPLErrorDeleteActiveSessionGuidance)
	}
	response := requester.DecisionRequest(ctx, permissions.PromptRequest{
		DecisionID: "decision:delete-history:" + sessionID,
		SessionID:  currentSession(),
		ActorID:    "user", ActorType: "local", WorkUnitID: "session-management",
		Kind:   permissions.PromptKindPermission,
		Action: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryAction), Target: sessionID,
		Impact:    i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryImpact),
		RiskLevel: 3, RiskReason: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryRisk),
		RuleSource: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryRule), ApprovalScope: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryScope),
		Choices: []string{"allow_once", "reject"},
	})
	if response.Outcome != permissions.PromptOutcomeApproved {
		return replError(i18n.KeyREPLErrorDeletionNotApproved)
	}
	if sessionID == currentSession() {
		return replError(i18n.KeyREPLErrorDeleteActiveSession)
	}
	if deleter != nil {
		return deleter.DeleteSessionHistory(ctx, sessionID, projectDir)
	}
	return repo.Delete(sessionID, projectDir)
}

func prepareTUISessionSnapshot(cfg TUIREPLConfig, sessionID, namespace string, epoch uint64, messages []types.Message) (tui.SessionSnapshot, error) {
	var details tui.DetailStore
	artifactRoot := ""
	if cfg.Repo != nil {
		artifactRoot = cfg.Repo.ArtifactsDir(sessionID, namespace)
	} else if provider, ok := cfg.Engine.Sessions().(engine.SessionArtifactsDirProvider); ok {
		artifactRoot = provider.ArtifactsDir(sessionID)
	}
	contextGeneration, generationErr := tuiContextGeneration(cfg.Engine, sessionID, namespace)
	if generationErr != nil {
		return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, generationErr)
	}
	identity := tui.SessionIdentity{Namespace: namespace, SessionID: sessionID, Epoch: epoch}
	if cfg.Repo != nil {
		controlScope, scopeErr := cfg.Repo.StoreForProjectDir(namespace).MessageControlScope(sessionID)
		if scopeErr != nil {
			return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, scopeErr)
		}
		if controlScope.Bound() {
			scopeGeneration := controlScope.ContextGeneration()
			if contextGeneration.Persisted && scopeGeneration != contextGeneration.Generation {
				return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, session.ErrCorruptSessionHistory)
			}
			// Generation zero is the pre-first-commit authority scope returned for
			// a new session. It is bound to a namespace, but it is not persisted.
			// Publishing it as persisted creates an impossible renderer state and
			// causes every response event to fail the generation fence.
			if scopeGeneration != 0 {
				// Repository metadata is the authoritative fallback when an embedder
				// has not supplied an Engine generation provider. Never downgrade a
				// durable checkpoint merely because Engine is nil.
				contextGeneration = engine.ContextGenerationState{
					Generation: scopeGeneration, Persisted: true,
				}
				identity = identity.WithInternalControlScope(messagecontrol.Runtime(), controlScope)
			}
		} else if contextGeneration.Persisted {
			return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, session.ErrCorruptSessionHistory)
		}
	}
	if artifactRoot != "" {
		exact, restored, restoreErr := tui.LoadSessionViewCheckpoint(artifactRoot, messages, identity)
		if restoreErr != nil {
			return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, restoreErr)
		}
		if restored {
			exact.ContextGeneration = contextGeneration.Generation
			exact.ContextGenerationPersisted = contextGeneration.Persisted
			return exact, nil
		}
	}
	if root := strings.TrimSpace(artifactRoot); root != "" {
		store, err := tui.NewFileDetailStore(root + string(os.PathSeparator) + "tui-details")
		if err != nil {
			return tui.SessionSnapshot{}, err
		}
		details = store
	}
	projection, err := tui.ProjectPersistedMessages(identity, messages, details)
	if err != nil {
		return tui.SessionSnapshot{}, err
	}
	providerName, modelID := tuiEngineProviderIdentity(cfg.Engine)
	if cfg.CurrentModel != nil {
		if current := strings.TrimSpace(cfg.CurrentModel()); current != "" {
			modelID = current
		}
	}
	snapshot := tui.SessionSnapshot{
		Identity: identity, Projection: projection, ContextGeneration: contextGeneration.Generation,
		ContextGenerationPersisted: contextGeneration.Persisted,
		DurableSessionView: tui.DurableSessionView{
			Provider: providerName, Model: modelID,
			Usage: tui.SessionUsage{Known: false, RoundUsageKnown: true}, PermissionMode: tui.ModeAutoEdit,
		},
	}
	if cfg.Repo != nil {
		meta, _, metaErr := cfg.Repo.GetMeta(sessionID, namespace)
		if metaErr != nil && !errors.Is(metaErr, fs.ErrNotExist) {
			return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, metaErr)
		}
		if metaErr == nil {
			if strings.TrimSpace(meta.Provider) != "" {
				snapshot.Provider = meta.Provider
			}
			if strings.TrimSpace(meta.Model) != "" {
				snapshot.Model = meta.Model
			}
			restoreTUISessionEvidence(&snapshot.Projection, meta.Evidence)
			for _, activity := range meta.Activities {
				snapshot.Activities = append(snapshot.Activities, activityFromSessionMeta(snapshot.Projection.Details, activity))
			}
			if meta.Goal != nil {
				snapshot.Goal = tui.GoalViewFromGoal(meta.Goal)
			}
			if meta.Usage != nil {
				snapshot.Usage = tui.SessionUsage{
					Known: true, RoundUsageKnown: meta.Usage.RoundUsageKnown,
					InputTokens: meta.Usage.InputTokens, OutputTokens: meta.Usage.OutputTokens,
					CacheReadTokens: meta.Usage.CacheReadTokens, CacheCreateTokens: meta.Usage.CacheCreateTokens,
					HasCompacted: meta.Usage.HasCompacted, CompactionCount: meta.Usage.CompactionCount,
					CompletedRoundInputTokens:  meta.Usage.CompletedRoundInputTokens,
					CompletedRoundOutputTokens: meta.Usage.CompletedRoundOutputTokens,
					InputTokensAtCompact:       meta.Usage.InputTokensAtCompact,
					CacheReadAtCompact:         meta.Usage.CacheReadAtCompact,
					LastInputTokens:            meta.Usage.LastInputTokens, LastOutputTokens: meta.Usage.LastOutputTokens,
					LastCacheReadTokens:   meta.Usage.LastCacheReadTokens,
					LastCacheCreateTokens: meta.Usage.LastCacheCreateTokens,
					WebSearchRequests:     meta.Usage.WebSearchRequests, CumulativeCost: meta.Usage.CumulativeCost,
					UsedTokens: meta.Usage.UsedTokens, MaxTokens: meta.Usage.MaxTokens,
				}
				costKnown := meta.Usage.CostKnown
				snapshot.SessionCostKnown = &costKnown
			}
			if meta.Presentation != nil {
				snapshot.Interaction = tui.SessionInteraction{
					FocusedObservationID: meta.Presentation.FocusedObservationID,
					ScrollAnchorID:       meta.Presentation.ScrollAnchorID,
					ScrollOffset:         meta.Presentation.ScrollOffset,
					InputDraft:           meta.Presentation.InputDraft,
					InputCursor:          meta.Presentation.InputCursor,
					InputCursorSet:       meta.Presentation.InputCursorSet,
				}
				if len(meta.Presentation.DisclosureReturns) > 0 {
					snapshot.DisclosureReturns = make(map[string]tui.SessionInteraction, len(meta.Presentation.DisclosureReturns))
					for id, restore := range meta.Presentation.DisclosureReturns {
						snapshot.DisclosureReturns[id] = tui.SessionInteraction{
							FocusedObservationID: restore.FocusedObservationID, ScrollAnchorID: restore.ScrollAnchorID,
							ScrollOffset: restore.ScrollOffset, InputDraft: restore.InputDraft,
							InputCursor: restore.InputCursor, InputCursorSet: restore.InputCursorSet,
						}
					}
				}
				snapshot.PermissionMode = interactionModeFromSessionMeta(meta.Presentation.PermissionMode)
				snapshot.ActivityFocus = meta.Presentation.ActivityFocus
				snapshot.ActivityViewOffset = meta.Presentation.ActivityViewOffset
			}
			for _, decision := range meta.Decisions {
				snapshot.Decisions = append(snapshot.Decisions, tui.DecisionRecord{
					Prompt: permissions.PromptRequest{
						DecisionID: decision.DecisionID, SessionID: sessionID, ExecutionSessionID: decision.ExecutionSessionID, TurnID: decision.TurnID, ToolUseID: decision.ToolUseID,
						ToolName: decision.ToolName, Input: decision.Input, RiskLevel: decision.RiskLevel, Message: decision.Message,
						ActorID: decision.ActorID, ActorType: decision.ActorType, WorkUnitID: decision.WorkUnitID,
						Kind: permissions.PromptKind(decision.Kind), Action: decision.Action, Target: decision.Target,
						Impact: decision.Impact, RiskReason: decision.RiskReason, RuleSource: decision.RuleSource,
						ApprovalScope: decision.ApprovalScope, Choices: append([]string(nil), decision.Choices...), Body: decision.Body,
						ReviewDetails: append([]string(nil), decision.ReviewDetails...), PostMode: decision.PostMode,
					},
					Response: permissions.PromptResponse{
						DecisionID: decision.DecisionID, Decision: restoredPermissionDecision(decision), Outcome: permissions.PromptOutcome(decision.Outcome), Choice: decision.Choice,
					},
					ResolvedAt: decision.ResolvedAt,
				})
			}
		}
	}
	if journal, ok := details.(tui.ObservationEvidenceJournal); ok {
		recovered, journalErr := journal.LoadObservationEvidence()
		if journalErr != nil {
			return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorRecoverDurableEvidence, journalErr)
		}
		journalMeta := make([]session.SessionEvidenceMeta, 0, len(recovered))
		for _, observation := range recovered {
			disclosure := observation.Disclosure.Level
			if disclosure == tui.DisclosureEvidence {
				disclosure = tui.DisclosureSummary
			}
			entry := session.SessionEvidenceMeta{
				ObservationID: observation.ID, SessionID: observation.SessionID, TurnID: observation.TurnID,
				ToolUseID: observation.ToolUseID, ToolName: observation.ToolName, WorkUnitID: observation.WorkUnitID, ActorID: observation.ActorID,
				Outcome: observation.Outcome.String(), Disclosure: int(disclosure), DisclosureSet: true,
				HasMore: observation.Disclosure.HasMore,
			}
			for _, ref := range observation.ResultRefs {
				entry.Results = append(entry.Results, sessionDetailRefMeta(ref))
			}
			for _, ref := range observation.EnvelopeRefs {
				entry.Envelopes = append(entry.Envelopes, sessionDetailRefMeta(ref))
			}
			journalMeta = append(journalMeta, entry)
		}
		restoreTUISessionEvidence(&snapshot.Projection, journalMeta)
	}
	return snapshot, nil
}

func withTUITranscriptControlScope(cfg TUIREPLConfig, exporter *tui.TranscriptExporter) *tui.TranscriptExporter {
	if exporter == nil || cfg.Repo == nil || cfg.SessionID == nil || strings.TrimSpace(*cfg.SessionID) == "" {
		return exporter
	}
	scope, err := cfg.Repo.StoreForProjectDir(currentProjectDir(cfg)).MessageControlScope(*cfg.SessionID)
	if err != nil || !scope.Bound() {
		return exporter
	}
	return exporter.WithInternalControlScope(messagecontrol.Runtime(), scope)
}

func tuiEngineProviderIdentity(eng engine.Engine) (providerName, modelID string) {
	if eng == nil {
		return "", ""
	}
	// Session preparation is also used by partial test/dry-run engines whose
	// optional embedded provider implementation may be unavailable. Provider
	// chrome is recoverable from session metadata, so it must not make an
	// otherwise valid transcript impossible to resume.
	defer func() {
		if recover() != nil {
			providerName, modelID = "", ""
		}
	}()
	if activeProvider := eng.Provider(); activeProvider != nil {
		return activeProvider.Name(), activeProvider.ModelID()
	}
	return "", ""
}

func restoredPermissionDecision(meta session.SessionDecisionMeta) permissions.Decision {
	if meta.Decision != nil {
		return permissions.Decision(*meta.Decision)
	}
	if permissions.PromptOutcome(meta.Outcome) != permissions.PromptOutcomeApproved {
		return permissions.DecisionDeny
	}
	if meta.Choice == "always_allow" {
		return permissions.DecisionAllow
	}
	return permissions.DecisionAllowOnce
}

func interactionModeFromSessionMeta(mode string) tui.InteractionMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto", "bypasspermissions", "acceptedits":
		return tui.ModeAutoEdit
	case "plan":
		return tui.ModePlanEdit
	default:
		return tui.ModeAskEdit
	}
}

func persistTUISessionLifecycle(cfg TUIREPLConfig, state *tui.AppState) error {
	return persistTUISessionLifecycleAtBoundaryWithUpdate(cfg, state, false, nil)
}

func persistTUISessionLifecycleForApp(cfg TUIREPLConfig, app tuiActivityApp) error {
	if app == nil {
		return nil
	}
	return persistTUISessionLifecycleAtBoundaryWithUpdate(cfg, app.State(), false, app.UpdateSync)
}

// persistSettledTUISessionLifecycle commits the terminal query boundary while
// QueryCancelFn is still installed. Event producers have stopped and the UI
// queue has been drained, but keeping the query marked active prevents
// /resume or /fork from observing a durable transcript without its exact view.
func persistSettledTUISessionLifecycle(cfg TUIREPLConfig, state *tui.AppState) error {
	return persistTUISessionLifecycleAtBoundaryWithUpdate(cfg, state, true, nil)
}

func persistSettledTUISessionLifecycleForApp(cfg TUIREPLConfig, app tuiActivityApp) error {
	if app == nil {
		return nil
	}
	return persistTUISessionLifecycleAtBoundaryWithUpdate(cfg, app.State(), true, app.UpdateSync)
}

func persistTUISessionLifecycleAtBoundaryWithUpdate(cfg TUIREPLConfig, state *tui.AppState, settledBoundary bool, update func(func()) bool) error {
	if cfg.Repo == nil || cfg.SessionID == nil || strings.TrimSpace(*cfg.SessionID) == "" || state == nil {
		return nil
	}
	if state.HasActiveQuery() && !settledBoundary {
		return nil
	}
	projectDir := currentProjectDir(cfg)
	messages, _, err := cfg.Repo.LoadByID(*cfg.SessionID, projectDir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return replWrap(i18n.KeyREPLErrorLoadTranscriptForLifecycle, err)
		}
		messages = nil
		if cfg.Engine != nil && cfg.Engine.Sessions() != nil {
			loaded, loadErr := cfg.Engine.Sessions().Load(*cfg.SessionID)
			if loadErr == nil {
				messages = loaded
			} else if !errors.Is(loadErr, engine.ErrSessionNotFound) && !errors.Is(loadErr, fs.ErrNotExist) {
				return replWrap(i18n.KeyREPLErrorLoadTranscriptForLifecycle, loadErr)
			}
		}
		if saveErr := cfg.Repo.Save(*cfg.SessionID, projectDir, messages); saveErr != nil {
			return replWrap(i18n.KeyREPLErrorCreateTranscriptForLifecycle, saveErr)
		}
	}
	var capture tui.SessionViewCapture
	var captureErr error
	captureView := func() {
		capture, captureErr = tui.CaptureSessionViewCheckpoint(state, messages)
	}
	if update == nil {
		captureView()
	} else if !update(captureView) {
		// A stopped app has no concurrent event producers, so a final direct
		// capture is safe and keeps shutdown persistence available.
		captureView()
	}
	if captureErr != nil {
		return captureErr
	}
	view := capture.View
	usage := view.Usage
	costKnown := true
	if view.SessionCostKnown != nil {
		costKnown = *view.SessionCostKnown
	}
	interaction := view.Interaction
	disclosureReturns := view.DisclosureReturns
	meta := session.SessionMeta{
		Presentation: &session.SessionPresentationMeta{
			Version:              3,
			FocusedObservationID: interaction.FocusedObservationID,
			ScrollAnchorID:       interaction.ScrollAnchorID,
			ScrollOffset:         interaction.ScrollOffset,
			InputDraft:           interaction.InputDraft,
			InputCursor:          interaction.InputCursor,
			InputCursorSet:       interaction.InputCursorSet,
			PermissionMode:       view.PermissionMode.Code(),
			ActivityFocus:        view.ActivityFocus,
			ActivityViewOffset:   view.ActivityViewOffset,
		},
		Activities: make([]session.SessionActivityMeta, 0),
	}
	for _, activity := range view.Activities {
		meta.Activities = append(meta.Activities, sessionActivityMetaFromActivity(activity))
	}
	if len(disclosureReturns) > 0 {
		meta.Presentation.DisclosureReturns = make(map[string]session.SessionDisclosureReturnMeta, len(disclosureReturns))
		for id, restore := range disclosureReturns {
			meta.Presentation.DisclosureReturns[id] = session.SessionDisclosureReturnMeta{
				FocusedObservationID: restore.FocusedObservationID, ScrollAnchorID: restore.ScrollAnchorID,
				ScrollOffset: restore.ScrollOffset, InputDraft: restore.InputDraft,
				InputCursor: restore.InputCursor, InputCursorSet: restore.InputCursorSet,
			}
		}
	}
	if usage.Known {
		meta.Usage = &session.SessionUsageMeta{
			InputTokens:                usage.InputTokens,
			OutputTokens:               usage.OutputTokens,
			CacheReadTokens:            usage.CacheReadTokens,
			CacheCreateTokens:          usage.CacheCreateTokens,
			HasCompacted:               usage.HasCompacted,
			RoundUsageKnown:            usage.RoundUsageKnown,
			CompactionCount:            usage.CompactionCount,
			CompletedRoundInputTokens:  usage.CompletedRoundInputTokens,
			CompletedRoundOutputTokens: usage.CompletedRoundOutputTokens,
			InputTokensAtCompact:       usage.InputTokensAtCompact,
			CacheReadAtCompact:         usage.CacheReadAtCompact,
			LastInputTokens:            usage.LastInputTokens,
			LastOutputTokens:           usage.LastOutputTokens,
			LastCacheReadTokens:        usage.LastCacheReadTokens,
			LastCacheCreateTokens:      usage.LastCacheCreateTokens,
			WebSearchRequests:          usage.WebSearchRequests,
			CumulativeCost:             usage.CumulativeCost,
			CostKnown:                  costKnown,
			UsedTokens:                 usage.UsedTokens,
			MaxTokens:                  usage.MaxTokens,
		}
	}
	for _, decision := range view.Decisions {
		executionDecision := int(decision.Response.Decision)
		meta.Decisions = append(meta.Decisions, session.SessionDecisionMeta{
			DecisionID: decision.Prompt.DecisionID, ExecutionSessionID: decision.Prompt.ExecutionSessionID, TurnID: decision.Prompt.TurnID, ToolUseID: decision.Prompt.ToolUseID,
			ToolName: decision.Prompt.ToolName, Input: decision.Prompt.Input, RiskLevel: decision.Prompt.RiskLevel, Message: decision.Prompt.Message,
			ActorID: decision.Prompt.ActorID, ActorType: decision.Prompt.ActorType, WorkUnitID: decision.Prompt.WorkUnitID,
			Kind: string(decision.Prompt.Kind), Action: decision.Prompt.Action, Target: decision.Prompt.Target,
			Impact: decision.Prompt.Impact, RiskReason: decision.Prompt.RiskReason, RuleSource: decision.Prompt.RuleSource,
			ApprovalScope: decision.Prompt.ApprovalScope, Choices: append([]string(nil), decision.Prompt.Choices...), Body: decision.Prompt.Body,
			ReviewDetails: append([]string(nil), decision.Prompt.ReviewDetails...), PostMode: decision.Prompt.PostMode,
			Outcome: string(decision.Response.Outcome), Choice: decision.Response.Choice, Decision: &executionDecision, ResolvedAt: decision.ResolvedAt,
		})
	}
	for _, observation := range capture.Observations {
		if len(observation.ResultRefs) == 0 && len(observation.EnvelopeRefs) == 0 {
			continue
		}
		disclosure := observation.Disclosure.Level
		if disclosure == tui.DisclosureEvidence {
			disclosure = tui.DisclosureSummary
		}
		evidence := session.SessionEvidenceMeta{
			ObservationID: observation.ID, SessionID: observation.SessionID, TurnID: observation.TurnID,
			ToolUseID: observation.ToolUseID, ToolName: observation.ToolName, WorkUnitID: observation.WorkUnitID,
			ActorID: observation.ActorID, Outcome: observation.Outcome.String(), Disclosure: int(disclosure), DisclosureSet: true,
			HasMore: observation.Disclosure.HasMore,
		}
		for _, ref := range observation.ResultRefs {
			evidence.Results = append(evidence.Results, sessionDetailRefMeta(ref))
		}
		for _, ref := range observation.EnvelopeRefs {
			evidence.Envelopes = append(evidence.Envelopes, sessionDetailRefMeta(ref))
		}
		meta.Evidence = append(meta.Evidence, evidence)
	}
	if err := saveTUISessionMeta(cfg, *cfg.SessionID, projectDir, meta); err != nil {
		return err
	}
	return tui.SaveSessionViewCapture(cfg.Repo.ArtifactsDir(*cfg.SessionID, projectDir), capture)
}

func saveTUISessionMeta(cfg TUIREPLConfig, sessionID, projectDir string, meta session.SessionMeta) error {
	if cfg.Repo == nil {
		return nil
	}
	err := cfg.Repo.SaveMeta(sessionID, projectDir, meta)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var messages []types.Message
	if cfg.Engine != nil && cfg.Engine.Sessions() != nil {
		loaded, loadErr := cfg.Engine.Sessions().Load(sessionID)
		if loadErr == nil {
			messages = loaded
		} else if !errors.Is(loadErr, engine.ErrSessionNotFound) && !errors.Is(loadErr, fs.ErrNotExist) {
			return replWrap(i18n.KeyREPLErrorLoadTranscriptForLifecycle, loadErr)
		}
	}
	if err := cfg.Repo.Save(sessionID, projectDir, messages); err != nil {
		return replWrap(i18n.KeyREPLErrorCreateTranscriptForLifecycle, err)
	}
	return cfg.Repo.SaveMeta(sessionID, projectDir, meta)
}

func sessionDetailRefMeta(ref tui.DetailRef) session.SessionDetailRefMeta {
	return session.SessionDetailRefMeta{Source: ref.Source, Key: ref.Key, Size: ref.Size, Digest: ref.Digest}
}

func sessionActivityMetaFromActivity(activity tui.Activity) session.SessionActivityMeta {
	meta := session.SessionActivityMeta{
		Version: session.SessionActivityMetaVersionProvisional,
		ID:      activity.ID, RunID: activity.RunID, SupersedesRunID: activity.SupersedesRunID, Attempt: activity.Attempt,
		BatchID: activity.BatchID, ParentRunID: activity.ParentRunID, AgentPath: activity.AgentPath,
		TurnID: activity.TurnID, WorkUnitID: activity.WorkUnitID,
		ActorID: activity.Actor.ID, ActorType: activity.Actor.Type,
		Kind: string(activity.Kind), Name: activity.Name, Phase: string(activity.Phase),
		State: string(activity.State), Lifecycle: string(activity.Lifecycle),
		AttentionKind: string(activity.Attention.Kind), AttentionSeverity: string(activity.Attention.Severity),
		AttentionUnread: activity.Attention.Unread, DecisionID: activity.Attention.DecisionID,
		AttentionMessage: activity.Attention.Message, Outcome: activity.Outcome.String(), Provisional: activity.Provisional,
		SourceSequence:  activity.SourceSequence,
		ProgressCurrent: activity.Progress.Current, ProgressTotal: activity.Progress.Total, ProgressMessage: activity.Progress.Message,
		Cancelable: activity.Control.Cancelable, JumpTarget: activity.Control.JumpTarget,
		OccurrenceCount: activity.OccurrenceCount, FirstSequence: activity.FirstSequence, LastSequence: activity.LastSequence,
		Acknowledged: activity.Acknowledged,
	}
	for _, ref := range activity.Control.DetailRefs {
		meta.DetailRefs = append(meta.DetailRefs, sessionDetailRefMeta(ref))
	}
	return meta
}

func activityFromSessionMeta(details tui.DetailStore, meta session.SessionActivityMeta) tui.Activity {
	outcome := tui.OutcomeUnknown
	if parsed, ok := tui.ParseObservationOutcome(meta.Outcome); ok {
		outcome = parsed
	}
	activity := tui.Activity{
		ActivityEvent: tui.ActivityEvent{
			ID: meta.ID, RunID: meta.RunID, SupersedesRunID: meta.SupersedesRunID, Attempt: meta.Attempt,
			BatchID: meta.BatchID, ParentRunID: meta.ParentRunID, AgentPath: meta.AgentPath,
			TurnID: meta.TurnID, WorkUnitID: meta.WorkUnitID,
			Actor: tui.ActivityActor{ID: meta.ActorID, Type: meta.ActorType},
			Kind:  tui.ActivityKind(meta.Kind), Name: meta.Name, Phase: tui.ActivityPhase(meta.Phase),
			State: tui.ActivityState(meta.State), Lifecycle: tui.ActivityLifecycle(meta.Lifecycle),
			Attention: tui.ActivityAttention{
				Kind: tui.ActivityAttentionKind(meta.AttentionKind), Severity: tui.ActivityAttentionSeverity(meta.AttentionSeverity),
				Unread: meta.AttentionUnread, DecisionID: meta.DecisionID, Message: meta.AttentionMessage,
			},
			Outcome: outcome, Provisional: sessionActivityMetaProvisional(meta), Sequence: meta.LastSequence, SourceSequence: meta.SourceSequence,
			Progress: tui.ActivityProgress{Current: meta.ProgressCurrent, Total: meta.ProgressTotal, Message: meta.ProgressMessage},
			Control:  tui.ActivityControl{Cancelable: meta.Cancelable, JumpTarget: meta.JumpTarget, DetailRefs: sessionDetailRefs(details, meta.DetailRefs)},
		},
		OccurrenceCount: meta.OccurrenceCount, FirstSequence: meta.FirstSequence, LastSequence: meta.LastSequence,
		Acknowledged: meta.Acknowledged,
	}
	return activity
}

func sessionActivityMetaProvisional(meta session.SessionActivityMeta) bool {
	if meta.Version >= session.SessionActivityMetaVersionProvisional {
		return meta.Provisional
	}
	// Preserve an explicit provisional bit written by any short-lived writer
	// that predates the version discriminator.
	if meta.Provisional {
		return true
	}
	return legacyRuntimeErrorActivityMeta(meta)
}

func legacyRuntimeErrorActivityMeta(meta session.SessionActivityMeta) bool {
	if !strings.HasPrefix(meta.ID, "tool:") || !strings.HasPrefix(meta.JumpTarget, "runtime-error:") || meta.Outcome != tui.OutcomeFailed.String() {
		return false
	}
	switch tui.ActivityKind(meta.Kind) {
	case "", tui.ActivityTool, tui.ActivityAgent, tui.ActivityMCP:
	default:
		return false
	}
	if meta.State != "" && meta.State != string(tui.ActivityFailed) {
		return false
	}
	if meta.Lifecycle != "" && meta.Lifecycle != string(tui.ActivityLifecycleFailed) {
		return false
	}
	if meta.State == "" && meta.Lifecycle == "" {
		return false
	}
	message := strings.TrimSpace(meta.ProgressMessage)
	if message == "" {
		return false
	}
	for _, lang := range i18n.AllLanguages() {
		if message == strings.TrimSpace(i18n.Text(lang, i18n.KeyRuntimeErrorPublicSummary)) {
			return true
		}
	}
	return false
}

func restoreTUISessionEvidence(projection *tui.SessionProjection, evidence []session.SessionEvidenceMeta) {
	if projection == nil || len(evidence) == 0 {
		return
	}
	byID := make(map[string]session.SessionEvidenceMeta, len(evidence))
	for _, entry := range evidence {
		if entry.ObservationID == "" {
			continue
		}
		byID[entry.ObservationID] = entry
	}
	restored := make(map[string]struct{}, len(projection.Observations))
	for index := range projection.Observations {
		restored[projection.Observations[index].ID] = struct{}{}
		entry, ok := byID[projection.Observations[index].ID]
		if !ok || (entry.SessionID != "" && projection.Observations[index].SessionID != "" && entry.SessionID != projection.Observations[index].SessionID) {
			continue
		}
		applySessionEvidenceIdentity(&projection.Observations[index], entry)
		projection.Observations[index].ResultRefs = mergeDetailRefs(projection.Observations[index].ResultRefs, sessionDetailRefs(projection.Details, entry.Results))
		projection.Observations[index].EnvelopeRefs = mergeDetailRefs(projection.Observations[index].EnvelopeRefs, sessionDetailRefs(projection.Details, entry.Envelopes))
	}
	for index := range projection.Messages {
		entry, ok := byID[projection.Messages[index].ObservationID]
		if ok && len(entry.Results) > 0 {
			projection.Messages[index].DetailRefs = mergeDetailRefs(projection.Messages[index].DetailRefs, sessionDetailRefs(projection.Details, entry.Results))
		}
	}
	for _, entry := range evidence {
		if _, ok := restored[entry.ObservationID]; ok || entry.ObservationID == "" || entry.SessionID == "" {
			continue
		}
		results := sessionDetailRefs(projection.Details, entry.Results)
		envelopes := sessionDetailRefs(projection.Details, entry.Envelopes)
		if len(results) == 0 && len(envelopes) == 0 {
			continue
		}
		observation := tui.Observation{ID: entry.ObservationID, SessionID: entry.SessionID, ResultRefs: results, EnvelopeRefs: envelopes}
		applySessionEvidenceIdentity(&observation, entry)
		projection.Observations = append(projection.Observations, observation)
		message := tui.Message{Kind: tui.MsgToolResult, Text: entry.ToolName, ToolName: entry.ToolName, ObservationID: entry.ObservationID,
			ToolUseID: entry.ToolUseID, WorkUnitID: entry.WorkUnitID, ActorID: entry.ActorID, Outcome: observation.Outcome,
			Disclosure: observation.Disclosure, DetailRefs: append([]tui.DetailRef(nil), results...)}
		projection.Messages = append(projection.Messages, message)
	}
}

func applySessionEvidenceIdentity(observation *tui.Observation, entry session.SessionEvidenceMeta) {
	observation.SessionID = preferNonEmpty(entry.SessionID, observation.SessionID)
	observation.TurnID = preferNonEmpty(entry.TurnID, observation.TurnID)
	observation.ToolUseID = preferNonEmpty(entry.ToolUseID, observation.ToolUseID)
	observation.ToolName = preferNonEmpty(entry.ToolName, observation.ToolName)
	observation.WorkUnitID = preferNonEmpty(entry.WorkUnitID, observation.WorkUnitID)
	observation.ActorID = preferNonEmpty(entry.ActorID, observation.ActorID)
	if outcome, ok := tui.ParseObservationOutcome(entry.Outcome); ok {
		observation.Outcome = outcome
	}
	if (entry.DisclosureSet || entry.Disclosure != int(tui.DisclosureSummary)) && entry.Disclosure >= int(tui.DisclosureSummary) && entry.Disclosure <= int(tui.DisclosureEvidence) {
		disclosure := tui.DisclosureLevel(entry.Disclosure)
		if disclosure == tui.DisclosureEvidence {
			disclosure = tui.DisclosureSummary
		}
		observation.Disclosure.Level = disclosure
	}
	observation.Disclosure.HasMore = observation.Disclosure.HasMore || entry.HasMore
	observation.Disclosure.UserPinned = false
}

func preferNonEmpty(preferred, fallback string) string {
	if strings.TrimSpace(preferred) != "" {
		return preferred
	}
	return fallback
}

func mergeDetailRefs(existing, restored []tui.DetailRef) []tui.DetailRef {
	out := append([]tui.DetailRef(nil), existing...)
	seen := make(map[string]struct{}, len(out)+len(restored))
	for _, ref := range out {
		seen[detailRefPhysicalIdentity(ref)] = struct{}{}
	}
	for _, ref := range restored {
		identity := detailRefPhysicalIdentity(ref)
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		out = append(out, ref)
	}
	return out
}

func detailRefPhysicalIdentity(ref tui.DetailRef) string {
	if ref.Digest != "" {
		return fmt.Sprintf("%s\x00%s\x00%d", ref.Source, ref.Digest, ref.Size)
	}
	return fmt.Sprintf("%s\x00%s\x00%d", ref.Source, ref.Key, ref.Size)
}

func sessionDetailRefs(store tui.DetailStore, refs []session.SessionDetailRefMeta) []tui.DetailRef {
	out := make([]tui.DetailRef, 0, len(refs))
	for _, ref := range refs {
		if ref.Source != "file" || ref.Size < 0 || len(ref.Digest) != 64 || ref.Key == "" {
			continue
		}
		detailRef := tui.DetailRef{Source: ref.Source, Key: ref.Key, Size: ref.Size, Digest: ref.Digest}
		if store != nil {
			if _, err := store.Get(detailRef); err != nil {
				continue
			}
		}
		out = append(out, detailRef)
	}
	return out
}

func publishTUIRuntimePermissionMode(state *tui.AppState, enqueue func(func()) bool, mode string) {
	if state == nil || enqueue == nil {
		return
	}
	presentationMode := tui.ModeAskEdit
	switch mode {
	case "plan":
		presentationMode = tui.ModePlanEdit
	case "auto", "acceptEdits", "bypassPermissions":
		presentationMode = tui.ModeAutoEdit
	}
	// Shift+Tab already publishes the requested presentation on the event
	// loop. Re-enqueuing that same value can deadlock a full update queue whose
	// only consumer is the current callback.
	if state.Mode.Get() == presentationMode {
		return
	}
	enqueue(func() { state.Mode.Set(presentationMode) })
}

func previousTUIInteractionMode(mode tui.InteractionMode) tui.InteractionMode {
	switch mode {
	case tui.ModeAskEdit:
		return tui.ModeAutoEdit
	case tui.ModePlanEdit:
		return tui.ModeAskEdit
	default:
		return tui.ModePlanEdit
	}
}

func applyTUIInteractionModeAtSessionBoundary(cfg TUIREPLConfig, state *tui.AppState, mode tui.InteractionMode) error {
	if cfg.SessionTransitionMu == nil {
		return applyTUIInteractionMode(cfg, state, mode)
	}
	if !cfg.SessionTransitionMu.TryLock() {
		state.Mode.Set(previousTUIInteractionMode(mode))
		return replErrorInLanguage(state.Language.Get(), i18n.KeyREPLErrorModeSwitchCommitting)
	}
	defer cfg.SessionTransitionMu.Unlock()
	return applyTUIInteractionMode(cfg, state, mode)
}

func applyTUIInteractionMode(cfg TUIREPLConfig, state *tui.AppState, mode tui.InteractionMode) error {
	if state == nil {
		return replError(i18n.KeyREPLErrorTUIStateRequired)
	}
	lang := state.Language.Get()
	runtimeMode := "default"
	rollbackMode := tui.ModeAutoEdit
	switch mode {
	case tui.ModeAutoEdit:
		runtimeMode = "bypassPermissions"
		rollbackMode = tui.ModePlanEdit
	case tui.ModePlanEdit:
		runtimeMode = "plan"
		rollbackMode = tui.ModeAskEdit
	}
	if cfg.PlanState != nil && mode != tui.ModePlanEdit && cfg.PlanState.IsActive() {
		if cfg.RuntimeScope == nil && cfg.PermChecker != nil {
			permissionMode := permissions.ModeAskAlways
			if mode == tui.ModeAutoEdit {
				permissionMode = permissions.ModeAllowAll
				if err := cfg.PermChecker.SetModeFromUser(permissionMode); err != nil {
					state.Mode.Set(rollbackMode)
					return replWrapInLanguage(lang, i18n.KeyREPLErrorExitPlanMode, err)
				}
			} else if err := cfg.PermChecker.SetMode(permissionMode); err != nil {
				state.Mode.Set(rollbackMode)
				return replWrapInLanguage(lang, i18n.KeyREPLErrorExitPlanMode, err)
			}
		}
		if err := cfg.PlanState.ExitForModeSwitch(runtimeMode); err != nil {
			if cfg.RuntimeScope == nil && cfg.PermChecker != nil {
				_ = cfg.PermChecker.SetMode(permissions.ModeAskAlways)
			}
			state.Mode.Set(rollbackMode)
			return replWrapInLanguage(lang, i18n.KeyREPLErrorExitPlanMode, err)
		}
		// ExitForModeSwitch is the transaction owner when PlanState has the
		// RuntimeScope; the no-scope branch prepared the checker before commit.
		return nil
	}
	if mode == tui.ModePlanEdit && cfg.PlanState != nil && !cfg.PlanState.IsActive() {
		if err := cfg.PlanState.Enter(""); err != nil {
			state.Mode.Set(rollbackMode)
			return replWrapInLanguage(lang, i18n.KeyREPLErrorPersistPlanMode, err)
		}
	}
	var err error
	if cfg.RuntimeScope != nil {
		err = cfg.RuntimeScope.TransitionPermissionMode(runtimeMode)
	} else if cfg.PermChecker != nil {
		permissionMode := permissions.ModeAskAlways
		if mode == tui.ModeAutoEdit {
			permissionMode = permissions.ModeAllowAll
			err = cfg.PermChecker.SetModeFromUser(permissionMode)
		} else {
			err = cfg.PermChecker.SetMode(permissionMode)
		}
	}
	if err == nil {
		return nil
	}
	if mode == tui.ModePlanEdit && cfg.PlanState != nil && cfg.PlanState.IsActive() {
		_ = cfg.PlanState.ExitForSessionRestore("default")
	}
	if mode == tui.ModeAutoEdit {
		rollbackMode = tui.ModeAskEdit
		if cfg.RuntimeScope != nil {
			_ = cfg.RuntimeScope.RestorePermissionMode("default")
		} else if cfg.PermChecker != nil {
			_ = cfg.PermChecker.SetMode(permissions.ModeAskAlways)
		}
	}
	state.Mode.Set(rollbackMode)
	return replWrapInLanguage(lang, i18n.KeyREPLErrorSwitchMode, err, mode.String())
}

func applyTUISessionPermissionMode(cfg TUIREPLConfig, mode tui.InteractionMode) error {
	runtimeMode := "default"
	switch mode {
	case tui.ModeAutoEdit:
		runtimeMode = "bypassPermissions"
	case tui.ModePlanEdit:
		runtimeMode = "plan"
	}
	if cfg.PlanState != nil && mode != tui.ModePlanEdit && cfg.PlanState.IsActive() {
		if cfg.RuntimeScope == nil && cfg.PermChecker != nil {
			permissionMode := permissions.ModeAskAlways
			if mode == tui.ModeAutoEdit {
				permissionMode = permissions.ModeAllowAll
				if err := cfg.PermChecker.SetModeFromUser(permissionMode); err != nil {
					return err
				}
			} else if err := cfg.PermChecker.SetMode(permissionMode); err != nil {
				return err
			}
		}
		if err := cfg.PlanState.ExitForSessionRestore(runtimeMode); err != nil {
			if cfg.RuntimeScope == nil && cfg.PermChecker != nil {
				_ = cfg.PermChecker.SetMode(permissions.ModeAskAlways)
			}
			return err
		}
		return nil
	}
	enteredPlan := false
	if cfg.PlanState != nil && mode == tui.ModePlanEdit && !cfg.PlanState.IsActive() {
		if err := cfg.PlanState.Enter(""); err != nil {
			return replWrap(i18n.KeyREPLErrorPersistRestoredPlanMode, err)
		}
		enteredPlan = true
	}
	var err error
	if cfg.RuntimeScope != nil {
		err = cfg.RuntimeScope.RestorePermissionMode(runtimeMode)
	} else if cfg.PermChecker != nil {
		permissionMode := permissions.ModeAskAlways
		if mode == tui.ModeAutoEdit {
			permissionMode = permissions.ModeAllowAll
		}
		err = cfg.PermChecker.SetModeFromUser(permissionMode)
	}
	if err != nil {
		if enteredPlan {
			previousRuntimeMode := "default"
			if cfg.RuntimeScope != nil {
				previousRuntimeMode = cfg.RuntimeScope.PermissionMode()
			}
			_ = cfg.PlanState.ExitForSessionRestore(previousRuntimeMode)
		}
		return err
	}
	return nil
}

// getCatalog returns the ModelCatalog from the ProviderRegistry if available.
func getCatalog(cfg TUIREPLConfig) *provider.ModelCatalog {
	if cfg.ProviderRegistry != nil {
		return cfg.ProviderRegistry.Catalog()
	}
	return nil
}

func queryRequestHasImage(req engine.QueryRequest) bool {
	for _, block := range req.Content {
		if block.GetType() == types.ContentTypeImage {
			return true
		}
	}
	return false
}

func currentModelSupportsImages(cfg TUIREPLConfig, model string) bool {
	providerName := ""
	if cfg.ProviderRef != nil {
		providerName = cfg.ProviderRef.Name()
	} else if cfg.Engine != nil && cfg.Engine.Provider() != nil {
		providerName = cfg.Engine.Provider().Name()
	}
	providerName = provider.CanonicalProviderName(providerName)
	if catalog := getCatalog(cfg); catalog != nil && providerName != "" {
		if info, ok := catalog.ResolveForProvider(providerName, model); ok {
			return info.CanSeeImages
		}
	}
	if cfg.Engine != nil && cfg.Engine.Provider() != nil {
		if cp, ok := cfg.Engine.Provider().(provider.CapabilityProvider); ok {
			return cp.Capabilities().Vision
		}
	}
	return true
}

// buildCascadingPicker creates and shows a two-phase cascading model picker.
// Phase 1: select a visible provider (connected and unconnected).
// Phase 2: select a model from the chosen provider's catalog (for connected providers).
// Phase 3: enter or refresh provider credentials (API key, OAuth, device auth).
func buildCascadingPicker(
	cfg TUIREPLConfig,
	tuiApp *tui.App,
	r *tui.TuiRenderer,
	ql *engineQueryLooper,
	tracker *ui.CostTracker,
) {
	catalog := getCatalog(cfg)
	if catalog == nil || cfg.ProviderRegistry == nil {
		return
	}

	// Show every visible provider, not just currently available providers.
	allProviders := cfg.ProviderRegistry.Visible()
	if len(allProviders) == 0 {
		r.Info(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUINoProviders))
		return
	}

	currentProvider := ""
	if cfg.ProviderRef != nil {
		currentProvider = provider.CanonicalProviderName(cfg.ProviderRef.Name())
	}

	// Build provider entries for Phase 1, including setup-required providers.
	providers := make([]tui.ProviderPickerEntry, len(allProviders))
	for i, p := range allProviders {
		models := catalog.ListByProvider(p.Name)
		connection := cfg.ProviderRegistry.ConnectionState(p.Name).Localized(tuiApp.State().Language.Get())
		baseURL := ""
		if cfg.CredentialStore != nil {
			for _, credentialName := range provider.CredentialLookupNames(p.Name) {
				if credential, ok := cfg.CredentialStore.Get(credentialName); ok {
					baseURL = credential.BaseURL
					break
				}
			}
		}
		providers[i] = tui.ProviderPickerEntry{
			Name:            p.Name,
			DisplayName:     p.DisplayName,
			ModelCount:      len(models),
			IsActive:        p.Name == currentProvider,
			IsConnected:     connection.State == provider.ConnectionStateConnected,
			ConnectionState: string(connection.State),
			ConnectionLabel: connection.Detail,
			CanSelectModels: connection.CanSelectModels,
			CanConnect:      connection.CanConnect,
			SetupHint:       connection.SetupHint,
			AuthMethods:     p.AuthMethods,
			EnvKey:          p.EnvKey,
			BaseURL:         baseURL,
			DefaultBaseURL:  p.DefaultBaseURL,
		}
	}

	// Pre-select the current active provider
	selectedIdx := 0
	for i, p := range providers {
		if p.IsActive {
			selectedIdx = i
			break
		}
	}

	picker := &tui.ModelPickerState{
		Visible:          true,
		Phase:            tui.PickerPhaseProvider,
		Providers:        providers,
		ProviderSelected: selectedIdx,
		OnSelect: func(entry tui.ModelPickerEntry) {
			withTUICommandLock(cfg, func() {
				lang := tuiApp.State().Language.Get()
				// Switch provider/model
				if cfg.ProviderRegistry != nil && cfg.ProviderRef != nil {
					providerCfg, err := provider.ResolveCredentialConfig(cfg.ProviderRegistry, entry.Provider)
					if err != nil {
						r.Error(i18n.Format(lang, i18n.KeyREPLTUIProviderCredsFailed, err))
						return
					}
					newProvider, err := cfg.ProviderRegistry.Create(
						entry.Provider,
						providerCfg,
						entry.ModelID,
					)
					if err != nil {
						r.Error(i18n.Format(lang, i18n.KeyREPLTUIProviderCreateFailed, err))
						return
					}
					ql.SetProvider(newProvider)
					ql.SetModel(entry.ModelID)
				}
				ql.SetReasoningEffort(entry.ReasoningEffort)
				r.SetReasoningEffort(entry.ReasoningEffort)
				r.Banner(entry.Provider, entry.ModelID)
				if entry.ReasoningEffort != "" {
					r.Info(i18n.Format(lang, i18n.KeyREPLTUIModelSwitchedReasoning, entry.Provider, entry.ModelID, entry.ReasoningEffort))
				} else {
					r.Info(i18n.Format(lang, i18n.KeyREPLTUIModelSwitched, entry.Provider, entry.ModelID))
				}
				tracker.SetProviderAndModel(entry.Provider, entry.ModelID)

				// Immediately refresh context bar so it reflects the new model's
				// context window size without waiting for the next turn to end.
				// Two paths:
				//   1. Session exists → read live ContextUsage from QueryLoop
				//   2. Session not yet created (no query sent yet) → derive
				//      MaxContext from the model catalog and show 0/Max
				info, err := cfg.Engine.ContextUsage(*cfg.SessionID)
				if err == nil && info != nil && info.TotalTokens > 0 {
					r.EffectiveContext(effectiveContextFromEngine(info))
				} else {
					// Fallback: session doesn't exist yet. Use model catalog.
					maxCtx := provider.LookupMaxContext(entry.ModelID)
					if modelInfo, ok := catalog.ResolveForProvider(entry.Provider, entry.ModelID); ok {
						maxCtx = modelInfo.ContextWindow
					}
					if maxCtx > 0 {
						r.EffectiveContext(ui.EffectiveContextProjection{
							Scope: ui.UsageScopeModelContext, Known: true,
							CapacityTokens: maxCtx,
							Measurement:    ui.ContextMeasurementUnknown,
						})
					}
				}
			})
		},
		OnCancel: func() {
			r.Info(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIModelPickerCancelled))
		},
		OnSaveLimits: func(entry tui.ModelPickerEntry, contextWindow *int) error {
			if contextWindow != nil && !provider.ValidOverrideContextWindow(*contextWindow) {
				return errors.New(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIContextWindowRange, provider.MinOverrideContextWindow, provider.MaxOverrideContextWindow))
			}
			if err := saveUserModelOverride(entry.Provider, entry.ModelID, contextWindow); err != nil {
				return err
			}
			settings, err := loadStartupModelSettings(currentCWD(cfg))
			if err != nil {
				return err
			}
			provider.SetRuntimeModelOverrides(settings.ModelOverrides)
			if cfg.ProviderRegistry != nil {
				cfg.ProviderRegistry.ApplyModelOverrides(settings.ModelOverrides)
			}
			return nil
		},
	}

	// OnConnect handles credential storage for connect and reconnect flows.
	// authMethod is one of: "api_key", "oauth_pkce", "device_code".
	picker.OnConnect = func(providerName, authMethod, baseURL, apiKey string) {
		providerName = provider.CanonicalProviderName(providerName)
		go func() {
			language := func() i18n.Language { return tuiApp.State().Language.Get() }
			switch authMethod {
			case "api_key":
				baseURL = strings.TrimSpace(baseURL)
				if err := validateCustomEndpointInLanguage(language(), providerName, baseURL); err != nil {
					setModelPickerConnectError(tuiApp, providerName, err.Error())
					return
				}
				if cfg.CredentialStore == nil {
					tuiApp.GoTuiApp().QueueUpdate(func() {
						p := tuiApp.State().ModelPicker.Get()
						if p != nil {
							p.ConnectError = i18n.Text(language(), i18n.KeyREPLTUICredentialStoreMissing)
							p.ConnectStatus = ""
							tuiApp.State().ModelPicker.Set(p)
						}
					})
					return
				}
				entry := provider.CredentialEntry{
					Provider:   providerName,
					AuthMethod: "api_key",
					APIKey:     apiKey,
					BaseURL:    baseURL,
					LastUsed:   time.Now(),
				}
				if err := cfg.CredentialStore.Set(entry); err != nil {
					tuiApp.GoTuiApp().QueueUpdate(func() {
						p := tuiApp.State().ModelPicker.Get()
						if p != nil {
							p.ConnectError = i18n.Format(language(), i18n.KeyREPLTUICredentialSaveFailed, err)
							p.ConnectStatus = ""
							tuiApp.State().ModelPicker.Set(p)
						}
					})
					return
				}
				showConnectedProviderModels(cfg, tuiApp, providerName, ql)
				r.Info(i18n.Format(language(), i18n.KeyREPLTUIProviderConnected, providerName))

			case "oauth_pkce":
				tuiApp.GoTuiApp().QueueUpdate(func() {
					p := tuiApp.State().ModelPicker.Get()
					if p != nil {
						p.ConnectStatus = i18n.Text(language(), i18n.KeyREPLTUIOAuthWaiting)
						p.ConnectError = ""
						tuiApp.State().ModelPicker.Set(p)
					}
				})
				r.Info(i18n.Format(language(), i18n.KeyREPLTUIOAuthStarting, providerName))
				if err := handleOAuthConnect(cfg, tuiApp, r, providerName); err != nil {
					setModelPickerConnectError(tuiApp, providerName, err.Error())
					r.Error(i18n.Format(language(), i18n.KeyREPLTUIOAuthFailed, providerName, err))
					return
				}
				showConnectedProviderModels(cfg, tuiApp, providerName, ql)
				r.Info(i18n.Format(language(), i18n.KeyREPLTUIProviderConnected, providerName))

			case "device_code":
				tuiApp.GoTuiApp().QueueUpdate(func() {
					p := tuiApp.State().ModelPicker.Get()
					if p != nil {
						p.ConnectStatus = i18n.Text(language(), i18n.KeyREPLTUIDeviceAuthWaiting)
						p.ConnectError = ""
						tuiApp.State().ModelPicker.Set(p)
					}
				})
				r.Info(i18n.Format(language(), i18n.KeyREPLTUIDeviceAuthStarting, providerName))
				if err := handleDeviceConnect(cfg, tuiApp, r, providerName); err != nil {
					setModelPickerConnectError(tuiApp, providerName, err.Error())
					r.Error(i18n.Format(language(), i18n.KeyREPLTUIDeviceAuthFailed, providerName, err))
					return
				}
				showConnectedProviderModels(cfg, tuiApp, providerName, ql)
				r.Info(i18n.Format(language(), i18n.KeyREPLTUIProviderConnected, providerName))
			}
		}()
	}

	// EnterProvider populates model entries when a provider is selected in Phase 1
	picker.EnterProvider = func(providerName string) {
		p := tuiApp.State().ModelPicker.Get()
		if p == nil {
			return
		}
		populateModelPickerEntries(p, catalog, providerName, currentPickerProvider(cfg), ql.Model(), ql.ReasoningEffort())
		tuiApp.State().ModelPicker.Set(p)
	}

	tuiApp.State().ModelPicker.Set(picker)
}

func effectiveContextFromEngine(info *engine.ContextUsageInfo) ui.EffectiveContextProjection {
	if info == nil {
		return ui.EffectiveContextProjection{Scope: ui.UsageScopeModelContext}
	}
	percent := modelContextPercent(info.UsedTokens, info.TotalTokens)
	measurement := ui.ContextMeasurement(info.Measurement)
	if measurement == "" {
		measurement = ui.ContextMeasurementUnknown
	}
	return ui.EffectiveContextProjection{
		Scope: ui.UsageScopeModelContext, Known: info.TotalTokens > 0,
		UsedTokens: info.UsedTokens, CapacityTokens: info.TotalTokens, PercentUsed: percent,
		Measurement: measurement, EstimateComplete: info.EstimateComplete,
		UnknownOverheads: append([]string(nil), info.UnknownOverheads...),
	}
}

func setModelPickerConnectError(tuiApp *tui.App, providerName, message string) {
	tuiApp.GoTuiApp().QueueUpdate(func() {
		p := tuiApp.State().ModelPicker.Get()
		if p == nil || (p.ConnectProvider != "" && p.ConnectProvider != providerName) {
			return
		}
		p.ConnectStatus = ""
		p.ConnectError = message
		tuiApp.State().ModelPicker.Set(p)
	})
}

func showConnectedProviderModels(cfg TUIREPLConfig, tuiApp *tui.App, providerName string, ql *engineQueryLooper) {
	providerName = provider.CanonicalProviderName(providerName)
	catalog := getCatalog(cfg)
	if catalog == nil || cfg.ProviderRegistry == nil {
		return
	}
	tuiApp.GoTuiApp().QueueUpdate(func() {
		p := tuiApp.State().ModelPicker.Get()
		if p == nil {
			return
		}
		connection := cfg.ProviderRegistry.ConnectionState(providerName).Localized(tuiApp.State().Language.Get())
		for i := range p.Providers {
			if p.Providers[i].Name == providerName {
				p.Providers[i].IsConnected = connection.State == provider.ConnectionStateConnected
				p.Providers[i].ConnectionState = string(connection.State)
				p.Providers[i].ConnectionLabel = connection.Detail
				p.Providers[i].CanSelectModels = connection.CanSelectModels
				p.Providers[i].CanConnect = connection.CanConnect
				if cfg.CredentialStore != nil {
					if credential, ok := cfg.CredentialStore.Get(providerName); ok {
						p.Providers[i].BaseURL = credential.BaseURL
					}
				}
				p.ProviderSelected = i
				break
			}
		}
		currentModel, currentEffort := "", ""
		if ql != nil {
			currentModel = ql.Model()
			currentEffort = ql.ReasoningEffort()
		}
		populateModelPickerEntries(p, catalog, providerName, currentPickerProvider(cfg), currentModel, currentEffort)
		p.Phase = tui.PickerPhaseModel
		p.SelectedProvider = providerName
		p.ConnectProvider = ""
		p.ConnectAuthMethods = nil
		p.ConnectSelectedAuth = 0
		p.ConnectAPIKeyInput = ""
		p.ConnectBaseURLInput = ""
		p.ConnectInputField = tui.ConnectInputAPIKey
		p.ConnectStatus = ""
		p.ConnectError = ""
		p.ConnectHint = ""
		p.IsReconnect = false
		tuiApp.State().ModelPicker.Set(p)
		tuiApp.GoTuiApp().RequestFullRedraw()
	})
}

func validateCustomEndpoint(providerName, raw string) error {
	return validateCustomEndpointInLanguage(i18n.DetectOrLoadLanguage(), providerName, raw)
}

func validateCustomEndpointInLanguage(lang i18n.Language, providerName, raw string) error {
	if provider.CanonicalProviderName(providerName) == "vertex" && raw == "" {
		return errors.New(i18n.Text(lang, i18n.KeyREPLTUIVertexBaseURLRequired))
	}
	if raw == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New(i18n.Text(lang, i18n.KeyREPLTUIBaseURLAbsolute))
	}
	if parsed.User != nil {
		return errors.New(i18n.Text(lang, i18n.KeyREPLTUIBaseURLCredentials))
	}
	return nil
}

func currentPickerProvider(cfg TUIREPLConfig) string {
	if cfg.Engine == nil || cfg.Engine.Provider() == nil {
		return ""
	}
	return provider.CanonicalProviderName(cfg.Engine.Provider().Name())
}

func populateModelPickerEntries(p *tui.ModelPickerState, catalog *provider.ModelCatalog, providerName, currentProvider, currentModel, currentEffort string) {
	providerName = provider.CanonicalProviderName(providerName)
	currentProvider = provider.CanonicalProviderName(currentProvider)
	models := catalog.ListByProvider(providerName)
	entries := make([]tui.ModelPickerEntry, len(models))
	for i, m := range models {
		ctxK := ""
		if m.ContextWindow > 0 {
			ctxK = tui.FmtContextWindow(m.ContextWindow)
		}
		reasoningEffort := provider.DefaultReasoningEffort(m.ReasoningEfforts)
		if providerName == currentProvider && modelPickerModelMatches(m, currentModel) && modelPickerEffortAvailable(m.ReasoningEfforts, currentEffort) {
			reasoningEffort = currentEffort
		}
		entries[i] = tui.ModelPickerEntry{
			Provider:          m.Provider,
			ModelID:           m.ID,
			DisplayName:       m.Name,
			ContextK:          ctxK,
			ContextTokens:     m.ContextWindow,
			ContextOverridden: m.ContextOverridden,
			CostIn:            m.CostPer1MIn,
			CostOut:           m.CostPer1MOut,
			CostCurrency:      m.BillingCurrency(),
			CanReason:         m.CanReason,
			CanSeeImages:      m.CanSeeImages,
			ReasoningEfforts:  append([]string(nil), m.ReasoningEfforts...),
			ReasoningEffort:   reasoningEffort,
			IsDefault:         m.IsDefault,
		}
	}
	p.Entries = entries
	p.Filtered = nil
	p.Selected = 0
	p.Query = ""
	p.ApplyFilter()
}

func modelPickerModelMatches(model provider.ModelInfo, current string) bool {
	if model.ID == current {
		return true
	}
	for _, alias := range model.Aliases {
		if alias == current {
			return true
		}
	}
	return false
}

func modelPickerEffortAvailable(efforts []string, current string) bool {
	for _, effort := range efforts {
		if effort == current {
			return true
		}
	}
	return false
}

// handleOAuthConnect runs an OAuth PKCE flow for the given provider.
// This is called from within the picker's OnConnect callback when the user
// selects the OAuth authentication method.
func handleOAuthConnect(cfg TUIREPLConfig, app *tui.App, r *tui.TuiRenderer, providerName string) error {
	lang := i18n.DetectOrLoadLanguage()
	if app != nil {
		lang = app.State().Language.Get()
	}
	providerName = provider.CanonicalProviderName(providerName)
	_, ok := cfg.ProviderRegistry.Get(providerName)
	if !ok {
		return errors.New(i18n.Format(lang, i18n.KeyREPLTUIUnknownProvider, providerName))
	}

	if cfg.CredentialStore == nil {
		return errors.New(i18n.Text(lang, i18n.KeyREPLTUICredentialStoreMissing))
	}

	ctx := connectCommandContext(cfg, r, lang)
	if err := commands.RunProviderOAuthConnect(ctx, providerName); err != nil {
		return err
	}
	return nil
}

// handleDeviceConnect runs a device authorization flow for the given provider.
func handleDeviceConnect(cfg TUIREPLConfig, app *tui.App, r *tui.TuiRenderer, providerName string) error {
	lang := i18n.DetectOrLoadLanguage()
	if app != nil {
		lang = app.State().Language.Get()
	}
	providerName = provider.CanonicalProviderName(providerName)
	_, ok := cfg.ProviderRegistry.Get(providerName)
	if !ok {
		return errors.New(i18n.Format(lang, i18n.KeyREPLTUIUnknownProvider, providerName))
	}

	if cfg.CredentialStore == nil {
		return errors.New(i18n.Text(lang, i18n.KeyREPLTUICredentialStoreMissing))
	}

	ctx := connectCommandContext(cfg, r, lang)
	if err := commands.RunProviderDeviceConnect(ctx, providerName); err != nil {
		return err
	}
	return nil
}

func connectCommandContext(cfg TUIREPLConfig, r *tui.TuiRenderer, languages ...i18n.Language) *commands.Context {
	lang := i18n.DetectOrLoadLanguage()
	if len(languages) > 0 {
		lang = languages[0]
	}
	return &commands.Context{
		Language:         lang,
		OnEvent:          r.Info,
		ProviderRegistry: cfg.ProviderRegistry,
		CredentialStore:  cfg.CredentialStore,
	}
}

type tuiLanguageSwitcher interface {
	State() *tui.AppState
	SwitchLanguage(i18n.Language) error
}

// switchTUILanguage handles the /language command through the same durable
// transaction used by the TUI keyboard shortcut.
func switchTUILanguage(app tuiLanguageSwitcher, code string) string {
	if app == nil {
		lang := i18n.DetectOrLoadLanguage()
		return i18n.Text(lang, i18n.KeyLanguageUnavailable)
	}
	state := app.State()
	cur := state.Language.Get()
	var target i18n.Language
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "show":
		return i18n.Format(cur, i18n.KeyLanguageCurrent, cur.String())
	case "next":
		target = cur.Next()
	case "en":
		target = i18n.LangEN
	case "zh":
		target = i18n.LangZH
	case "de":
		target = i18n.LangDE
	case "ja":
		target = i18n.LangJA
	case "ko":
		target = i18n.LangKO
	case "ru":
		target = i18n.LangRU
	default:
		return i18n.Text(cur, i18n.KeyLanguageUnsupported)
	}
	if err := app.SwitchLanguage(target); err != nil {
		return i18n.Text(cur, i18n.KeyLanguageUnavailable)
	}
	return i18n.Format(target, i18n.KeyLanguageSwitched, target.String())
}

// convertPerModelCosts converts ui.ModelCostEntry to commands.ModelCostEntry.
func convertPerModelCosts(entries []ui.ModelCostEntry) []commands.ModelCostEntry {
	if len(entries) == 0 {
		return nil
	}
	result := make([]commands.ModelCostEntry, len(entries))
	for i, e := range entries {
		result[i] = commands.ModelCostEntry{
			Model:             e.Model,
			InputTokens:       e.InputTokens,
			OutputTokens:      e.OutputTokens,
			WebSearchRequests: e.WebSearchRequests,
			CostUSD:           e.CostUSD,
			TurnCount:         e.TurnCount,
		}
	}
	return result
}

func activityHasAction(activity tui.Activity, want tui.ActivityAction) bool {
	for _, action := range activity.Actions {
		if action == want {
			return true
		}
	}
	return false
}

func performTUIActivityAction(app tuiActivityApp, stopper tuiActivityStopper, id, action string) (string, error) {
	lang := i18n.DetectOrLoadLanguage()
	if app != nil && app.State() != nil {
		lang = app.State().Language.Get()
	}
	return performTUIActivityActionInLanguage(lang, app, stopper, id, action)
}

func performTUIActivityActionInLanguage(lang i18n.Language, app tuiActivityApp, stopper tuiActivityStopper, id, action string) (string, error) {
	activity, ok := app.State().GetActivity(id)
	if !ok {
		return "", errors.New(i18n.Format(lang, i18n.KeyREPLTUIActivityNotFound, id))
	}
	switch strings.ToLower(action) {
	case "cancel":
		if !activityHasAction(activity, tui.ActivityCancel) {
			return "", errors.New(i18n.Format(lang, i18n.KeyREPLTUIActivityNotCancellable, id))
		}
		if stopper == nil || !strings.HasPrefix(id, "background:") {
			return "", errors.New(i18n.Format(lang, i18n.KeyREPLTUIActivityNoController, id))
		}
		if _, err := stopper.Stop(strings.TrimPrefix(id, "background:")); err != nil {
			return "", err
		}
		return i18n.Format(lang, i18n.KeyREPLTUIActivityCancelled, id), nil
	case "jump":
		if activity.Control.JumpTarget == "" {
			return "", errors.New(i18n.Format(lang, i18n.KeyREPLTUIActivityNoJump, id))
		}
		if _, ok := app.State().GetObservation(activity.Control.JumpTarget); ok {
			var revealErr error
			if !app.UpdateSync(func() { revealErr = app.State().RevealObservation(activity.Control.JumpTarget, tui.DisclosureDetail) }) {
				return "", errors.New(i18n.Text(lang, i18n.KeyREPLTUIStopped))
			}
			if revealErr != nil {
				return "", revealErr
			}
			app.UpdateSync(func() { app.State().SetExpandedView("") })
			return i18n.Format(lang, i18n.KeyREPLTUIActivityJumped, activity.Control.JumpTarget), nil
		}
		app.UpdateSync(func() {
			app.State().SetExpandedView("activities")
			app.State().ActivityFocus.Set(id)
		})
		return i18n.Format(lang, i18n.KeyREPLTUIActivityLocated, id), nil
	case "details":
		if activity.Kind == tui.ActivityAgent || len(activity.Control.DetailRefs) == 0 {
			return "", errors.New(i18n.Format(lang, i18n.KeyREPLTUIActivityNoDetails, id))
		}
		if _, ok := app.State().GetObservation(activity.Control.JumpTarget); ok {
			var revealErr error
			if !app.UpdateSync(func() { revealErr = app.State().RevealObservation(activity.Control.JumpTarget, tui.DisclosureEvidence) }) {
				return "", errors.New(i18n.Text(lang, i18n.KeyREPLTUIStopped))
			}
			if revealErr != nil {
				return "", revealErr
			}
			app.UpdateSync(func() { app.State().SetExpandedView("") })
			return i18n.Format(lang, i18n.KeyREPLTUIActivityEvidenceOpened, activity.Control.JumpTarget), nil
		}
		var detail strings.Builder
		for _, ref := range activity.Control.DetailRefs {
			content, err := app.State().ReadDetail(ref)
			if err != nil {
				return "", err
			}
			detail.Write(content)
			if len(content) > 0 && content[len(content)-1] != '\n' {
				detail.WriteByte('\n')
			}
		}
		return detail.String(), nil
	case "acknowledge", "ack":
		if !activityHasAction(activity, tui.ActivityAcknowledge) {
			return "", errors.New(i18n.Format(lang, i18n.KeyREPLTUIActivityNoAttention, id))
		}
		var acknowledgeErr error
		if !app.UpdateSync(func() { acknowledgeErr = app.State().AcknowledgeActivity(id) }) {
			return "", errors.New(i18n.Text(lang, i18n.KeyREPLTUIStopped))
		}
		if acknowledgeErr != nil {
			return "", acknowledgeErr
		}
		return i18n.Format(lang, i18n.KeyREPLTUIActivityAcknowledged, id), nil
	default:
		return "", errors.New(i18n.Format(lang, i18n.KeyREPLTUIActivityUnknownAction, action))
	}
}

// tuiRunQuery dispatches a single QueryRequest through the engine and
// streams events to the TUI renderer.
func tuiRunQuery(
	ctx context.Context,
	cfg TUIREPLConfig,
	tuiApp *tui.App,
	req engine.QueryRequest,
	tracker *ui.CostTracker,
	sigHandler *SignalHandler,
) {
	r := tuiApp.Renderer()

	queryCtx, queryCancel := context.WithCancel(ctx)
	sigHandler.SetQueryCancel(queryCancel)

	// Also register in AppState so the TUI Ctrl+C handler can cancel
	queryGeneration := tuiInputAdmission(ctx)
	if queryGeneration == 0 {
		queryGeneration = tuiApp.State().SetQueryCancel(func() { queryCancel() })
	} else if !tuiApp.State().SetReservedQueryCancel(queryGeneration, func() { queryCancel() }) {
		queryCancel()
		return
	}

	defer func() {
		queryCancel()
		sigHandler.ClearQueryCancel()
		// Queue ordering is the commit barrier: all text/tool/status updates are
		// applied, streaming markdown is finalized, and usage is synchronized
		// before the durable transcript is paired with its exact view. The query
		// remains active until that commit attempt finishes.
		settleTUIQueryViewAfterUpdates(tuiApp, tracker)
		persistErr := persistSettledTUISessionLifecycleForApp(cfg, tuiApp)
		tuiApp.State().ClearQueryCancel(queryGeneration)
		if persistErr != nil {
			r.Warning(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUILifecycleSaveFailed, persistErr))
		}
	}()
	if !tuiApp.UpdateSync(func() { tuiApp.State().BeginLLMWork() }) {
		tuiApp.State().BeginLLMWork()
	}

	ch, err := cfg.Engine.Query(queryCtx, req)
	if err != nil {
		lang := tuiApp.State().Language.Get()
		r.Error(i18n.Format(lang, i18n.KeyREPLTUIQueryStartFailed, engine.UserFacingError(lang, err)))
		if status, update := terminalTUIProviderStatus(err, false); update {
			r.SetProviderStatus(status)
		}
		return
	}

	// Build the event handler (same cost-tracking logic as the old REPL)
	getContextUsage := func() (int, int) {
		info, err := cfg.Engine.ContextUsage(*cfg.SessionID)
		if err != nil || info == nil {
			return 0, 0
		}
		return info.TotalTokens, info.UsedTokens
	}
	reqEpoch := tuiApp.State().SessionEpoch.Get()
	base := ui.ToolEventContext{
		SessionID: req.SessionID, SessionEpoch: reqEpoch, ContextGeneration: tuiApp.State().ContextGeneration.Get(),
		ContextGenerationPersisted: tuiApp.State().ContextGenerationPersisted.Get(),
	}
	onEvent, stopToolSpinners := makeTUIEventHandler(r, tracker, getContextUsage, base)
	defer stopToolSpinners()

	var runErr error
	providerRequestCompleted := false
	terminalContext := base
	for evt := range ch {
		if evt.Final {
			runErr = evt.Error
		} else {
			switch evt.Inner.Type {
			case loop.EventRequestStart, loop.EventRequestFailed:
				providerRequestCompleted = false
			case loop.EventRequestEnd:
				providerRequestCompleted = true
			}
			onEvent(evt.Inner)
		}
	}
	if committed, generationErr := commitTUIContextGeneration(r, cfg.Engine, base, currentProjectDir(cfg)); generationErr != nil && runErr == nil {
		runErr = generationErr
	} else if generationErr == nil {
		terminalContext.ContextGeneration = committed.Generation
		terminalContext.ContextGenerationPersisted = committed.Persisted
	}
	stopToolSpinners()

	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			r.InfoAtContext(terminalContext, i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUIQueryCancelled))
		} else {
			lang := tuiApp.State().Language.Get()
			r.ErrorAtContext(terminalContext, i18n.Format(lang, i18n.KeyREPLQueryFailed, engine.UserFacingError(lang, runErr)))
			if status, update := terminalTUIProviderStatus(runErr, providerRequestCompleted); update {
				r.SetProviderStatusAtContext(terminalContext, status)
			}
		}
	} else {
		// Query succeeded — provider is connected
		r.SetProviderStatusAtContext(terminalContext, tui.StatusConnected)
	}
}

func currentCWD(cfg TUIREPLConfig) string {
	if cfg.CWD == nil {
		return ""
	}
	return *cfg.CWD
}

func currentBuildDiagnostic(cfg TUIREPLConfig) buildinfo.Diagnostic {
	if cfg.BuildDiagnostic == nil {
		return buildinfo.Diagnostic{}
	}
	return cfg.BuildDiagnostic(currentCWD(cfg))
}

func currentProjectDir(cfg TUIREPLConfig) string {
	if cfg.SessionProjectDir == nil {
		return ""
	}
	return *cfg.SessionProjectDir
}

func currentRuntimeProjectRoot(cfg TUIREPLConfig) string {
	if cfg.RuntimeScope == nil {
		return ""
	}
	return strings.TrimSpace(cfg.RuntimeScope.ProjectRoot())
}

func currentHookRunner(cfg TUIREPLConfig) *hooks.Runner {
	if cfg.HookRunnerRef == nil {
		return nil
	}
	return *cfg.HookRunnerRef
}

// makeTUIEventHandler returns an event handler for TUI mode with cost tracking.
// getContextUsage returns (maxTokens, usedTokens) from the engine's context window.
func makeTUIEventHandler(r ui.Renderer, tracker *ui.CostTracker, getContextUsage func() (int, int), baseContexts ...ui.ToolEventContext) (func(loop.Event), func()) {
	turnStart := time.Now()
	baseContext := ui.ToolEventContext{}
	if len(baseContexts) > 0 {
		baseContext = baseContexts[0]
	}
	guarded, hasEpochRenderer := r.(tuiEpochRenderer)
	contextGuarded, hasContextRenderer := r.(tuiContextEventRenderer)
	costKnownRenderer, hasCostKnownRenderer := r.(tuiCostKnownEpochRenderer)
	sessionUsageRenderer, hasSessionUsageRenderer := r.(tuiSessionUsageRenderer)
	effectiveContextRenderer, hasEffectiveContextRenderer := r.(tuiEffectiveContextEpochRenderer)
	activityRenderer, hasActivityRenderer := r.(tuiActivityEpochRenderer)
	compactionProgressRenderer, hasCompactionProgressRenderer := r.(tuiCompactionProgressEpochRenderer)
	compactionBoundaryRenderer, hasCompactionBoundaryRenderer := r.(tuiCompactionBoundaryEpochRenderer)
	goalStatusRenderer, hasGoalStatusRenderer := r.(tuiGoalStatusEpochRenderer)
	llmRequestRenderer, hasLLMRequestRenderer := r.(tuiLLMRequestEpochRenderer)
	aggregateFreezer, hasAggregateFreezer := r.(tuiAggregateFreezer)
	activeToolSpinners := make(map[string]func())
	var anonymousToolSpinners []func()
	stopToolSpinner := func(toolUseID string) {
		if toolUseID == "" {
			// Missing identity is an orphan signal, not permission to infer a
			// positional sibling. Anonymous spinners remain until the turn-wide
			// terminal event or handler cleanup.
			return
		}
		if stop, ok := activeToolSpinners[toolUseID]; ok {
			stop()
			delete(activeToolSpinners, toolUseID)
		}
	}
	stopAllToolSpinners := func() {
		for toolUseID, stop := range activeToolSpinners {
			stop()
			delete(activeToolSpinners, toolUseID)
		}
		for _, stop := range anonymousToolSpinners {
			stop()
		}
		anonymousToolSpinners = anonymousToolSpinners[:0]
	}
	handle := func(event loop.Event) {
		if !ui.AdmitContextGeneration(r, baseContext) {
			return
		}
		ui.SetRenderTurn(r, event.TurnCount)
		eventContext := baseContext
		if event.TurnID != "" {
			eventContext.TurnID = event.TurnID
		} else if event.TurnCount > 0 {
			eventContext.TurnID = fmt.Sprintf("%s:turn-%d", eventContext.SessionID, event.TurnCount)
		}
		if event.ActorID != "" {
			eventContext.ActorID = event.ActorID
		}
		if event.ActorType != "" {
			eventContext.ActorType = event.ActorType
		}
		if event.WorkUnitID != "" {
			eventContext.WorkUnitID = event.WorkUnitID
		}
		if event.Type != loop.EventSystemWarning && event.ProjectRoot != "" {
			eventContext.ProjectRoot = event.ProjectRoot
		}
		switch event.Type {
		case loop.EventRequestStart, loop.EventRequestRetry, loop.EventRequestFirstToken, loop.EventRequestEnd, loop.EventRequestFailed:
			if hasContextRenderer && event.RequestStatus != nil {
				contextGuarded.LLMRequestStatusAtContext(eventContext, event.Type, *event.RequestStatus)
			} else if hasLLMRequestRenderer && event.RequestStatus != nil {
				llmRequestRenderer.LLMRequestStatusAtEpoch(baseContext.SessionEpoch, event.Type, *event.RequestStatus)
			}
		case loop.EventText:
			if hasContextRenderer {
				contextGuarded.TextAtContext(eventContext, event.Text)
			} else if hasEpochRenderer {
				guarded.TextAtEpoch(baseContext.SessionEpoch, event.Text)
			} else {
				r.Text(event.Text)
			}
		case loop.EventThinking:
			if hasContextRenderer {
				contextGuarded.ThinkingAtContext(eventContext, event.Text)
			} else if hasEpochRenderer {
				guarded.ThinkingAtEpoch(baseContext.SessionEpoch, event.Text)
			} else {
				r.Thinking(event.Text)
			}
		case loop.EventToolUse:
			if event.ToolUse != nil {
				ui.DispatchToolCallEvent(r, eventContext, *event.ToolUse)
				var stop func()
				if hasEpochRenderer {
					stop = guarded.SpinnerStartAtEpoch(baseContext.SessionEpoch, event.ToolUse.Name)
				} else {
					stop = r.SpinnerStart(event.ToolUse.Name)
				}
				if event.ToolUse.ID == "" {
					anonymousToolSpinners = append(anonymousToolSpinners, stop)
				} else {
					if previous, ok := activeToolSpinners[event.ToolUse.ID]; ok {
						previous()
					}
					activeToolSpinners[event.ToolUse.ID] = stop
				}
			}
		case loop.EventToolResult:
			if event.ToolResult != nil {
				stopToolSpinner(event.ToolResult.ToolUseID)
				ui.DispatchToolResultEvent(r, eventContext, *event.ToolResult)
			}
		case loop.EventGoalEvaluation, loop.EventProviderUsage:
			costDelta, cumulativeCost, recorded := recordAuxiliaryUsageEvent(tracker, event)
			if recorded && hasSessionUsageRenderer {
				sessionUsageRenderer.SessionUsageAtContext(eventContext, ui.BuildSessionUsageProjection(tracker))
			}
			if recorded && hasContextRenderer {
				contextGuarded.CostSummaryAtContext(eventContext, costDelta, cumulativeCost, event.Usage.InputTokens, event.Usage.OutputTokens)
				contextGuarded.CostKnownAtContext(eventContext, tracker.CostKnown())
			} else if recorded && hasEpochRenderer {
				// Auxiliary provider calls are part of session usage/cost but are not
				// conversation requests, so they only update cumulative cost here.
				// Full token totals are synchronized from the tracker at query release.
				guarded.CostSummaryAtEpoch(baseContext.SessionEpoch, costDelta, cumulativeCost, event.Usage.InputTokens, event.Usage.OutputTokens)
				if hasCostKnownRenderer {
					costKnownRenderer.CostKnownAtEpoch(baseContext.SessionEpoch, tracker.CostKnown())
				}
			}
		case loop.EventContextUsage:
			if hasContextRenderer {
				contextGuarded.EffectiveContextAtContext(eventContext, effectiveContextProjection(event.ContextUsage))
			} else if hasEffectiveContextRenderer {
				effectiveContextRenderer.EffectiveContextAtEpoch(baseContext.SessionEpoch, effectiveContextProjection(event.ContextUsage))
			}
		case loop.EventGoalStatus:
			if hasContextRenderer && event.GoalStatus != nil {
				contextGuarded.GoalStatusAtContext(eventContext, *event.GoalStatus)
			} else if hasGoalStatusRenderer && event.GoalStatus != nil {
				goalStatusRenderer.GoalStatusAtEpoch(baseContext.SessionEpoch, *event.GoalStatus)
			}
		case loop.EventTurnEnd:
			stopAllToolSpinners()
			if hasContextRenderer {
				contextGuarded.FreezeAggregatesAtContext(eventContext)
			} else if hasAggregateFreezer {
				aggregateFreezer.FreezeAggregatesAtEpoch(baseContext.SessionEpoch, eventContext.SessionID, eventContext.TurnID)
			}
			if hasContextRenderer {
				contextGuarded.UsageAtContext(eventContext, event.Usage)
			} else if hasEpochRenderer {
				guarded.UsageAtEpoch(baseContext.SessionEpoch, event.Usage)
			} else {
				r.Usage(event.Usage)
			}
			if recordTurnUsageEvent(tracker, event, time.Since(turnStart)) {
				if hasSessionUsageRenderer {
					sessionUsageRenderer.SessionUsageAtContext(eventContext, ui.BuildSessionUsageProjection(tracker))
				}
				if hasContextRenderer {
					contextGuarded.CostKnownAtContext(eventContext, tracker.CostKnown())
				} else if hasCostKnownRenderer {
					costKnownRenderer.CostKnownAtEpoch(baseContext.SessionEpoch, tracker.CostKnown())
				}
				last := tracker.LastTurn()
				if last != nil {
					if hasContextRenderer {
						contextGuarded.CostSummaryAtContext(eventContext, last.CostUSD, tracker.TotalCost(), last.InputTokens, last.OutputTokens)
					} else if hasEpochRenderer {
						guarded.CostSummaryAtEpoch(baseContext.SessionEpoch, last.CostUSD, tracker.TotalCost(), last.InputTokens, last.OutputTokens)
					} else {
						r.CostSummary(last.CostUSD, tracker.TotalCost(), last.InputTokens, last.OutputTokens)
					}
				}
			}
			// Update context bar with live token usage from the engine
			if getContextUsage != nil && event.Usage != nil {
				maxTok, usedTok := getContextUsage()
				if maxTok > 0 {
					if hasContextRenderer {
						contextGuarded.ContextBarAtContext(eventContext, usedTok, maxTok)
					} else if hasEpochRenderer {
						guarded.ContextBarAtEpoch(baseContext.SessionEpoch, usedTok, maxTok)
					} else {
						r.ContextBar(usedTok, maxTok)
					}
				}
			}
			turnStart = time.Now()
		case loop.EventError:
			stopAllToolSpinners()
			if structuredError, ok := r.(tuiRuntimeErrorRenderer); ok {
				structuredError.RuntimeErrorEvent(eventContext, event.ToolUseID, event.Text, event.Error, event.Metadata)
			} else {
				publicMessage := ui.RuntimeErrorPublicMessage(eventContext, event.ToolUseID, event.Text, event.Error, event.Metadata, i18n.LangEN, false)
				if hasContextRenderer {
					contextGuarded.ErrorAtContext(eventContext, publicMessage)
				} else if hasEpochRenderer {
					guarded.ErrorAtEpoch(baseContext.SessionEpoch, publicMessage)
				} else {
					r.Error(publicMessage)
				}
			}
		case loop.EventSystemWarning:
			language := i18n.DetectOrLoadLanguage()
			if languageRenderer, ok := r.(ui.RuntimeLanguageRenderer); ok {
				language = languageRenderer.RuntimeLanguage()
			}
			ui.DispatchRuntimeWarningEvent(r, event.SystemWarningRuntimeEvent(), language, true)
		case loop.EventUserInterruption:
			stopAllToolSpinners()
		case loop.EventProgress:
			if !hasActivityRenderer || event.Progress == nil {
				break
			}
			if activity, ok := compactionProgressActivityInLanguage(i18n.DetectOrLoadLanguage(), eventContext, event.Progress); ok {
				if hasCompactionProgressRenderer {
					compactionProgressRenderer.CompactionProgressAtEpoch(baseContext.SessionEpoch, eventContext, *event.Progress)
				} else {
					if hasContextRenderer {
						contextGuarded.ActivityAtContext(eventContext, activity)
					} else {
						activityRenderer.ActivityAtEpoch(baseContext.SessionEpoch, activity)
					}
				}
			}
		case loop.EventCompactBoundary:
			if event.Compact == nil {
				break
			}
			if tracker != nil {
				tracker.MarkCompaction()
			}
			if hasCompactionBoundaryRenderer {
				compactionBoundaryRenderer.CompactionBoundaryAtEpoch(baseContext.SessionEpoch, eventContext, *event.Compact)
			}
			if hasSessionUsageRenderer && tracker != nil {
				sessionUsageRenderer.SessionUsageAtContext(eventContext, ui.BuildSessionUsageProjection(tracker))
			}
		case loop.EventHookSummary:
			if event.HookSummary == nil {
				break
			}
			summary := ui.HookSummary{
				ExecutionID: event.HookSummary.HookExecutionID,
				ToolUseID:   event.HookSummary.ToolUseID,
				Name:        event.HookSummary.HookName,
				Status:      event.HookSummary.Status,
				Summary:     event.HookSummary.Summary,
				Metadata:    event.HookSummary.Metadata,
			}
			if structured, ok := r.(ui.StructuredHookRenderer); ok {
				structured.RenderHookSummary(eventContext, summary)
			} else {
				lang := i18n.DetectOrLoadLanguage()
				r.Info(i18n.Format(lang, i18n.KeyREPLTUIHookSummary, summary.Name, localizedHookStatus(lang, summary.Status), hookSummarySuffix(summary.Summary)))
			}
		}
	}
	return handle, stopAllToolSpinners
}

func compactionProgressActivity(ctx ui.ToolEventContext, progress *loop.ProgressEvent) (tui.ActivityEvent, bool) {
	return compactionProgressActivityInLanguage(i18n.DetectOrLoadLanguage(), ctx, progress)
}

func compactionProgressActivityInLanguage(lang i18n.Language, ctx ui.ToolEventContext, progress *loop.ProgressEvent) (tui.ActivityEvent, bool) {
	if progress == nil || !strings.Contains(strings.ToLower(progress.Stage), "compact") {
		return tui.ActivityEvent{}, false
	}
	turnID := ctx.TurnID
	if turnID == "" {
		turnID = ctx.SessionID + ":turn"
	}
	workUnitID := ctx.WorkUnitID
	if workUnitID == "" {
		workUnitID = "context-compaction"
	}
	actorID, actorType := ctx.ActorID, ctx.ActorType
	if actorID == "" {
		actorID = "assistant"
	}
	if actorType == "" {
		actorType = "runtime"
	}
	trigger := compactionTrigger(progress.Metadata)
	state, outcome := tui.ActivityRunning, tui.OutcomeRunning
	stage := strings.ToLower(progress.Stage)
	switch {
	case stage == "compact_cancelled":
		state, outcome = tui.ActivityCancelled, tui.OutcomeCancelled
	case strings.Contains(stage, "failure") || strings.Contains(stage, "failed"):
		state, outcome = tui.ActivityFailed, tui.OutcomeFailed
	case stage == "compact_end" || strings.Contains(stage, "success"):
		state, outcome = tui.ActivityCompleted, tui.OutcomeSucceeded
	}
	message := strings.TrimSpace(progress.Message)
	if message == "" {
		message = progress.Stage
	}
	message = localizedCompactionProgressMessage(lang, message)
	return tui.ActivityEvent{
		ID: compactionActivityID(ctx.SessionID, turnID, trigger), SessionID: ctx.SessionID, TurnID: turnID,
		WorkUnitID: workUnitID, Actor: tui.ActivityActor{ID: actorID, Type: actorType},
		Kind: tui.ActivityBackground, Name: i18n.Text(lang, i18n.KeyREPLTUIContextCompaction), Phase: tui.ActivityPhaseExecuting,
		State: state, Outcome: outcome,
		Progress: tui.ActivityProgress{Current: progress.Current, Total: progress.Total, Message: message},
		Control:  tui.ActivityControl{Cancelable: state == tui.ActivityRunning},
	}, true
}

func localizedCompactionProgressMessage(lang i18n.Language, message string) string {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "compacting", "compact_start":
		return i18n.Text(lang, i18n.KeyREPLTUICompactionCompacting)
	case "failed", "compact_failed":
		return i18n.Text(lang, i18n.KeyREPLTUICompactionFailed)
	case "cancelled", "canceled", "compact_cancelled":
		return i18n.Text(lang, i18n.KeyREPLTUICompactionCancelled)
	case "compact_end":
		return i18n.TUIOutcomeLabel(lang, "completed")
	case "idle":
		return i18n.Text(lang, i18n.KeyREPLTUICompactionIdle)
	default:
		return message
	}
}

func compactionTrigger(metadata map[string]any) string {
	if trigger, ok := metadata["trigger"].(string); ok && strings.TrimSpace(trigger) != "" {
		return strings.ToLower(strings.TrimSpace(trigger))
	}
	return "unknown"
}

func compactionActivityID(sessionID, turnID, trigger string) string {
	return "progress:compaction:" + sessionID + ":" + turnID + ":" + trigger
}

type manualCompactionEventEngine interface {
	CompactWithEvents(context.Context, string, string, func(loop.Event)) error
}

func runManualCompactionEvents(ctx context.Context, eng engine.Engine, sessionID, customInstructions string, onEvent func(loop.Event)) error {
	return runManualCompactionEventsInLanguage(ctx, eng, sessionID, customInstructions, i18n.DetectOrLoadLanguage(), onEvent)
}

func runManualCompactionEventsInLanguage(ctx context.Context, eng engine.Engine, sessionID, customInstructions string, lang i18n.Language, onEvent func(loop.Event)) error {
	before, err := eng.Sessions().Load(sessionID)
	if err != nil {
		return err
	}
	turnID := sessionID + ":manual-compact:" + uuid.NewString()
	base := loop.Event{
		TurnID: turnID, ActorID: "assistant", ActorType: "runtime", WorkUnitID: "context-compaction",
	}
	emitProgress := func(stage, status string, terminalErr error) {
		if onEvent == nil {
			return
		}
		metadata := map[string]any{"trigger": "manual", "status": status}
		if terminalErr != nil {
			metadata["error"] = terminalErr.Error()
		}
		event := base
		event.Type = loop.EventProgress
		event.Progress = &loop.ProgressEvent{Stage: stage, Message: status, Metadata: metadata}
		onEvent(event)
	}
	emitProgress("compact_start", "compacting", nil)
	var compactErr error
	var completedBoundary *loop.CompactBoundaryEvent
	if eventEngine, ok := eng.(manualCompactionEventEngine); ok {
		compactErr = eventEngine.CompactWithEvents(ctx, sessionID, customInstructions, func(event loop.Event) {
			switch event.Type {
			case loop.EventCompactBoundary:
				if event.Compact != nil {
					boundary := cloneCompactBoundaryEvent(*event.Compact)
					completedBoundary = &boundary
				}
			case loop.EventProviderUsage:
				if onEvent != nil {
					// Keep the wrapper's manual-compaction identity while retaining the
					// exact provider/model metadata emitted by QueryLoop.
					event.TurnID = base.TurnID
					event.ActorID = base.ActorID
					event.ActorType = base.ActorType
					event.WorkUnitID = base.WorkUnitID
					onEvent(event)
				}
			}
		})
	} else {
		compactErr = eng.Compact(ctx, sessionID, customInstructions)
	}
	if compactErr != nil {
		stage, status := "compact_failed", "failed"
		if errors.Is(compactErr, context.Canceled) {
			stage, status = "compact_cancelled", "cancelled"
		}
		localizedErr := compact.LocalizeUserError(lang, compactErr)
		// Progress metadata is diagnostic data consumed by tests and downstream
		// observers, so keep the original error text there. The returned error is
		// localized for the user-facing command surface while retaining the
		// original error chain.
		emitProgress(stage, status, compactErr)
		return localizedErr
	}
	after, err := eng.Sessions().Load(sessionID)
	if err != nil {
		emitProgress("compact_failed", "failed", err)
		return err
	}
	if onEvent != nil {
		event := base
		event.Type = loop.EventCompactBoundary
		boundary := manualCompactionBoundary(before, after)
		if completedBoundary != nil {
			// The event-bearing engine carries evidence from the exact authorized
			// result. Publish it only after Compact and the authoritative reload
			// have succeeded; persisted history cannot reconstruct hook display
			// output and must never cause unsigned descriptors to be trusted.
			boundary = cloneCompactBoundaryEvent(*completedBoundary)
		}
		event.Compact = &boundary
		onEvent(event)
	}
	emitProgress("compact_end", "idle", nil)
	return nil
}

func manualCompactionBoundary(before, after []types.Message) loop.CompactBoundaryEvent {
	counter := compact.NewContextWindow(0)
	boundary := loop.CompactBoundaryEvent{
		Trigger: "manual", PreCompactTokenCount: counter.EstimateMessages(before),
		PostCompactTokenCount: counter.EstimateMessages(after), TruePostCompactTokenCount: counter.EstimateMessages(after),
	}
	for i := len(after) - 1; i >= 0; i-- {
		metadata, ok := compact.ParseCompactBoundaryMessage(after[i])
		if !ok {
			continue
		}
		if metadata.Trigger != "" {
			boundary.Trigger = metadata.Trigger
		}
		if metadata.PreCompactTokenCount > 0 {
			boundary.PreCompactTokenCount = metadata.PreCompactTokenCount
		}
		boundary.PreviousTailIdentifier = metadata.PreviousTailIdentifier
		boundary.PreCompactDiscoveredTools = append([]string(nil), metadata.PreCompactDiscoveredTools...)
		if metadata.PreservedSegment != nil {
			preserved := *metadata.PreservedSegment
			boundary.PreservedSegment = &preserved
		}
		if i+1 < len(after) && compact.IsCompactSummaryMessage(after[i+1]) {
			boundary.Summary = strings.TrimSpace(after[i+1].GetText())
		}
		break
	}
	return boundary
}

func cloneCompactBoundaryEvent(boundary loop.CompactBoundaryEvent) loop.CompactBoundaryEvent {
	cloned := boundary
	cloned.PreCompactDiscoveredTools = append([]string(nil), boundary.PreCompactDiscoveredTools...)
	if boundary.PreservedSegment != nil {
		preserved := *boundary.PreservedSegment
		cloned.PreservedSegment = &preserved
	}
	return cloned
}

func hookSummarySuffix(summary string) string {
	if strings.TrimSpace(summary) == "" {
		return ""
	}
	return " - " + strings.TrimSpace(summary)
}

func localizedHookStatus(lang i18n.Language, status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "blocked":
		return i18n.Text(lang, i18n.KeyREPLTUIHookBlocked)
	case "success", "succeeded", "completed", "ok":
		return i18n.Text(lang, i18n.KeyREPLTUIHookSucceeded)
	case "failed", "error":
		return i18n.Text(lang, i18n.KeyREPLTUIHookFailed)
	case "cancelled", "canceled":
		return i18n.Text(lang, i18n.KeyREPLTUIHookCancelled)
	case "running":
		return i18n.Text(lang, i18n.KeyREPLTUIHookRunning)
	default:
		return status
	}
}
