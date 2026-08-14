package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/runtimeevent"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/runtime/goal"
	runtimescope "github.com/agent-dance/luban/internal/runtime/scope"
	"github.com/agent-dance/luban/internal/store/session"
	toolinteraction "github.com/agent-dance/luban/internal/tools/interaction"
	tooltasks "github.com/agent-dance/luban/internal/tools/tasks"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
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
	ProviderRef              *provider.ProviderRef
	ProviderRegistry         *provider.ProviderRegistry
	CredentialStore          *provider.CredentialStore
	ProviderRuntimeOverrides provider.RuntimeOverrides

	// Mode switching support (Shift+Tab)
	PermChecker         *permissions.Checker
	PlanState           *toolinteraction.PlanState
	AskUserQuestionTool *toolinteraction.AskUserQuestionTool
	RuntimeScope        *runtimescope.RuntimeScope
	TaskCreateTool      *tooltasks.TaskCreateTool
	AgentTool           tuiAgentProgressSource
	BackgroundTasks     tuiBackgroundTaskPresentation
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
		// A provider that exposes only unscoped state cannot safely answer an
		// exact project request. Do not fall back across namespaces.
		if _, ok := eng.(engine.ContextGenerationStateProvider); ok {
			return engine.ContextGenerationState{}, engine.ErrContextGenerationUnavailable
		}
		return engine.ContextGenerationState{}, nil
	}
	if provider, ok := eng.(engine.ContextGenerationStateProvider); ok {
		return normalize(provider.ContextGenerationState(sessionID))
	}
	return engine.ContextGenerationState{}, nil
}

func commitTUIContextGeneration(r presentation.Renderer, eng engine.Engine, base presentation.ToolEventContext, projectDir string) (engine.ContextGenerationState, error) {
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
	snapshot := tracker.Snapshot()
	state.ApplySessionUsageProjection(ui.BuildSessionUsageProjectionFromSnapshot(snapshot))
	conversation := snapshot.Conversation
	state.SessionRoundUsageKnown.Set(conversation.Known)
	state.SessionCompactionCount.Set(conversation.CompactionCount)
	state.SessionCompletedRoundInputTokens.Set(conversation.CompletedInputTokens)
	state.SessionCompletedRoundOutputTokens.Set(conversation.CompletedOutputTokens)
	state.SessionInputTokens.Set(conversation.LastInputTokens)
	state.SessionOutputTokens.Set(conversation.LastOutputTokens)
	state.SessionCacheReadTokens.Set(conversation.LastCacheReadTokens)
	state.SessionCacheCreateTokens.Set(conversation.LastCacheMakeTokens)
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

func sessionUsageMetaFromTUIView(view tui.DurableSessionView) *session.SessionUsageMeta {
	usage := view.Usage
	return &session.SessionUsageMeta{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheCreateTokens: usage.CacheCreateTokens,
		HasCompacted: usage.HasCompacted, CompactionBaselineKnown: usage.CompactionBaselineKnown,
		RoundUsageKnown: usage.RoundUsageKnown, CompactionCount: usage.CompactionCount,
		ProgressiveProjectionCount: usage.ProgressiveProjectionCount, ProgressiveProjectedTools: usage.ProgressiveProjectedTools,
		ProgressiveTokensSaved: usage.ProgressiveTokensSaved, ProgressiveSavingsUSD: usage.ProgressiveSavingsUSD,
		CompletedRoundInputTokens: usage.CompletedRoundInputTokens, CompletedRoundOutputTokens: usage.CompletedRoundOutputTokens,
		InputTokensAtCompact: usage.InputTokensAtCompact, CacheReadAtCompact: usage.CacheReadAtCompact,
		LastInputTokens: usage.LastInputTokens, LastOutputTokens: usage.LastOutputTokens,
		LastCacheReadTokens: usage.LastCacheReadTokens, LastCacheCreateTokens: usage.LastCacheCreateTokens,
		WebSearchRequests: usage.WebSearchRequests, CumulativeCost: usage.CumulativeCost, CostKnown: view.SessionCostKnown,
		UsedTokens: usage.UsedTokens, MaxTokens: usage.MaxTokens,
	}
}

func flushTUIUsageUpdates(app tuiActivityApp, tracker *ui.CostTracker) bool {
	if app.UpdateSync(func() { syncTUIUsageFromTracker(app.State(), tracker) }) {
		return true
	}
	syncTUIUsageFromTracker(app.State(), tracker)
	return false
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
	Stop(string) (agentcontract.TaskSnapshot, error)
}

// tuiAgentProgressSource is the presentation-only slice of the Agent runtime.
// Keeping the TUI on this port prevents the rendering layer from depending on
// launch, permission, profile, or session-mutation APIs.
type tuiAgentProgressSource interface {
	SubscribeProgress(func(agentcontract.ProgressEvent)) func()
}

type tuiRuntimeNotificationSink interface {
	DeliverRuntimeNotification(context.Context, agentcontract.RuntimeNotification) error
}

type tuiRuntimeNotificationSinkFunc func(context.Context, agentcontract.RuntimeNotification) error

func (f tuiRuntimeNotificationSinkFunc) DeliverRuntimeNotification(ctx context.Context, notification agentcontract.RuntimeNotification) error {
	return f(ctx, notification)
}

type tuiBackgroundFollowUpTarget struct {
	SessionID         string
	SessionProjectDir string
	ProjectRoot       string
	Message           string
}

// tuiBackgroundTaskPresentation is the read/notification surface consumed by
// the TUI and screen-reader paths. The application composition root supplies
// an adapter over the concrete Agent manager.
type tuiBackgroundTaskPresentation interface {
	tuiActivityStopper
	CurrentProjectRoot() string
	InMemorySnapshots() []agentcontract.TaskSnapshot
	Snapshots() []agentcontract.TaskSnapshot
	Snapshot(string) (agentcontract.TaskSnapshot, bool)
	SubscribeSnapshots() (<-chan struct{}, func())
	ReconcileInterruptedAgentRecords() int
	ResolveNotificationTarget(agentcontract.RuntimeNotification) (agentcontract.TaskSnapshot, bool)
	NotificationFollowUpTarget(agentcontract.RuntimeNotification) (tuiBackgroundFollowUpTarget, bool)
	LocalizeRuntimeNotification(i18n.Language, agentcontract.RuntimeNotification, agentcontract.TaskSnapshot) agentcontract.RuntimeNotification
	SetNotificationConsumers(tuiRuntimeNotificationSink, tuiRuntimeNotificationSink)
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
	TextAtContext(presentation.ToolEventContext, string)
	ThinkingAtContext(presentation.ToolEventContext, string)
	ErrorAtContext(presentation.ToolEventContext, string)
	InfoAtContext(presentation.ToolEventContext, string)
	UsageAtContext(presentation.ToolEventContext, *types.Usage)
	CostSummaryAtContext(presentation.ToolEventContext, float64, float64, int, int)
	CostKnownAtContext(presentation.ToolEventContext, bool)
	ContextBarAtContext(presentation.ToolEventContext, int, int)
	ModelContextAtContext(presentation.ToolEventContext, presentation.ModelContextProjection)
	SetProviderStatusAtContext(presentation.ToolEventContext, tui.ProviderStatus)
	GoalStatusAtContext(presentation.ToolEventContext, stream.GoalStatusEvent)
	LLMRequestStatusAtContext(presentation.ToolEventContext, stream.EventType, stream.RequestStatusEvent)
	FreezeAggregatesAtContext(presentation.ToolEventContext)
	ActivityAtContext(presentation.ToolEventContext, tui.ActivityEvent)
}

type tuiContextGenerationCommitter interface {
	CommitContextGeneration(presentation.ToolEventContext, uint64, bool) bool
}

type tuiRuntimeErrorRenderer interface {
	RuntimeErrorEvent(presentation.ToolEventContext, string, string, *types.APIError, map[string]any)
}

type tuiCostKnownEpochRenderer interface {
	CostKnownAtEpoch(uint64, bool)
}

type tuiSessionUsageRenderer interface {
	SessionUsageAtContext(presentation.ToolEventContext, presentation.SessionUsageProjection)
}

type tuiActivityEpochRenderer interface {
	ActivityAtEpoch(uint64, tui.ActivityEvent)
}

type tuiCompactionProgressEpochRenderer interface {
	CompactionProgressAtEpoch(uint64, presentation.ToolEventContext, stream.ProgressEvent)
}

type tuiProgressiveContextMetricsEpochRenderer interface {
	ProgressiveContextMetricsAtEpoch(uint64, presentation.ToolEventContext, stream.ProgressEvent)
}

type tuiCompactionBoundaryEpochRenderer interface {
	CompactionBoundaryAtEpoch(uint64, presentation.ToolEventContext, stream.CompactBoundaryEvent)
}

type tuiGoalStatusEpochRenderer interface {
	GoalStatusAtEpoch(uint64, stream.GoalStatusEvent)
}

type tuiLLMRequestEpochRenderer interface {
	LLMRequestStatusAtEpoch(uint64, stream.EventType, stream.RequestStatusEvent)
}

type tuiLLMActivityContextRenderer interface {
	LLMActivityAtContext(presentation.ToolEventContext, tui.LLMCallStage, string, ...int)
}

type tuiLLMActivityEpochRenderer interface {
	LLMActivityAtEpoch(uint64, tui.LLMCallStage, string, ...int)
}

type tuiAggregateFreezer interface {
	FreezeAggregatesAtEpoch(uint64, string, string)
}

func progressMetadataInt(metadata map[string]any, key string) int {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
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
	return prepareNewTUISession(ctx, cfg, newID, epoch, tui.ModeAutoEdit)
}

// prepareNewTUISession publishes the first durable transcript and view
// checkpoint for a session that is known not to exist. Persisted sessions must
// continue through prepareTUISessionSnapshot so missing checkpoints fail
// closed instead of being silently reconstructed.
func prepareNewTUISession(ctx context.Context, cfg TUIREPLConfig, newID string, epoch uint64, permissionMode tui.InteractionMode) (tui.SessionSnapshot, engine.PreparedRuntimeContextResume, error) {
	namespace := currentProjectDir(cfg)
	identity := tui.SessionIdentity{Namespace: namespace, SessionID: newID, Epoch: epoch}
	projection, err := tui.ProjectPersistedMessages(identity, nil, nil)
	if err != nil {
		return tui.SessionSnapshot{}, nil, err
	}
	snapshot := tui.SessionSnapshot{
		Identity:   identity,
		Projection: projection,
		DurableSessionView: tui.DurableSessionView{
			Usage:            tui.SessionUsage{Known: true, RoundUsageKnown: true},
			SessionCostKnown: true,
			PermissionMode:   permissionMode,
		},
	}
	if err := cfg.Engine.Sessions().Save(newID, nil); err != nil {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replWrap(i18n.KeyREPLErrorCreateEmptySession, err)
	}
	// The first save publishes generation 1. Refresh before the /clear snapshot
	// is committed so the visible session starts with authoritative lineage.
	generation, generationErr := tuiContextGeneration(cfg.Engine, newID, currentProjectDir(cfg))
	if generationErr != nil {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replWrap(i18n.KeyREPLErrorCreateEmptySession, generationErr)
	}
	snapshot.ContextGeneration = generation.Generation
	snapshot.ContextGenerationPersisted = generation.Persisted
	checkpointState := tui.NewAppState()
	if err := checkpointState.ApplySessionSnapshot(snapshot); err != nil {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replWrap(i18n.KeyREPLErrorSaveEmptySessionLifecycle, err)
	}
	artifactRoot := tuiSessionArtifactRoot(cfg, newID, namespace)
	if strings.TrimSpace(artifactRoot) == "" {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replWrap(i18n.KeyREPLErrorSaveEmptySessionLifecycle, i18n.NewError(i18n.KeyTUISessionViewMissingCheckpoint))
	}
	if err := tui.SaveSessionViewCheckpoint(artifactRoot, checkpointState, nil); err != nil {
		_ = cfg.Engine.Sessions().Delete(newID)
		return tui.SessionSnapshot{}, nil, replWrap(i18n.KeyREPLErrorSaveEmptySessionLifecycle, err)
	}
	if err := saveTUISessionMeta(cfg, newID, currentProjectDir(cfg), session.SessionMeta{
		Usage: &session.SessionUsageMeta{RoundUsageKnown: true, CostKnown: true}, Presentation: &session.SessionPresentationMeta{PermissionMode: permissionMode.Code()},
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

func prepareInitialTUISessionSnapshot(ctx context.Context, cfg TUIREPLConfig, sessionID, namespace string, epoch uint64, messages []types.Message, newSession bool) (tui.SessionSnapshot, error) {
	if !newSession {
		return prepareTUISessionSnapshot(cfg, sessionID, namespace, epoch, messages)
	}

	permissionMode := currentTUISessionPermissionMode(cfg, tui.ModeAskEdit)
	snapshot, prepared, err := prepareNewTUISession(ctx, cfg, sessionID, epoch, permissionMode)
	if err != nil {
		return tui.SessionSnapshot{}, err
	}
	if err := commitPreparedRuntimeResume(ctx, prepared); err != nil {
		prepared.Abort()
		_ = cfg.Engine.Sessions().Delete(sessionID)
		return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorActivateEmptySession, err)
	}
	return snapshot, nil
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

func installTUIPermissionPrompt(checker *permissions.Checker, requester tuiDecisionRequester) {
	if checker == nil || requester == nil {
		return
	}
	var promptMu sync.Mutex
	checker.SetStructuredPromptFunc(func(ctx context.Context, request permissions.PromptRequest) permissions.PromptResponse {
		promptMu.Lock()
		defer promptMu.Unlock()
		return requester.DecisionRequest(ctx, request)
	})
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

// synchronizeInitialTUIChrome restores process-owned runtime values after the
// session snapshot has been applied. New-session checkpoints do not yet have a
// provider or model, and reasoning effort is deliberately not session-owned;
// without this boundary the snapshot can erase the values set by TUI
// construction before the first frame is rendered.
func synchronizeInitialTUIChrome(state *tui.AppState, providerName, modelID, reasoningEffort string) {
	if state == nil {
		return
	}
	if strings.TrimSpace(state.Provider.Get()) == "" {
		state.Provider.Set(strings.TrimSpace(providerName))
	}
	if strings.TrimSpace(state.Model.Get()) == "" {
		state.Model.Set(strings.TrimSpace(modelID))
	}
	state.ReasoningEffort.Set(strings.TrimSpace(reasoningEffort))
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
		tracker.RestoreCompactionBaselineState(usage.HasCompacted, usage.CompactionBaselineKnown, usage.InputTokensAtCompact, usage.CacheReadAtCompact)
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
	// The admission closure references tuiAppRef, which is populated after
	// construction returns but before Run is called.
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
		// A busy foreground admits the composer submission into the scheduler's
		// FIFO. Escape can then interrupt the active turn and promote its oldest
		// queued successor; idle submissions still start immediately.
		admitted, _ := inputScheduler.TrySubmit(inputStr, tuiAppRef.State().TakePendingImages, true)
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
		func(queued []string) {
			tuiApp.GoTuiApp().QueueUpdateLossless(func() {
				tuiApp.State().QueuedInputTexts.Set(queued)
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
	newSession := errors.Is(loadErr, engine.ErrSessionNotFound)
	if newSession {
		initialMessages = nil
	}
	initialSnapshot, err := prepareInitialTUISessionSnapshot(ctx, cfg, *cfg.SessionID, currentProjectDir(cfg), 1, initialMessages, newSession)
	if err != nil {
		_ = tuiApp.Close()
		return replWrapInLanguage(tuiApp.State().Language.Get(), i18n.KeyREPLErrorPrepareInitialTUISession, err)
	}
	if err := tuiApp.ApplySessionSnapshot(initialSnapshot); err != nil {
		_ = tuiApp.Close()
		return replWrapInLanguage(tuiApp.State().Language.Get(), i18n.KeyREPLErrorApplyInitialTUISession, err)
	}
	synchronizeInitialTUIChrome(tuiApp.State(), providerName, modelID, ql.ReasoningEffort())
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
	tracker.RestoreCompactionBaselineState(initialSnapshot.Usage.HasCompacted, initialSnapshot.Usage.CompactionBaselineKnown, initialSnapshot.Usage.InputTokensAtCompact, initialSnapshot.Usage.CacheReadAtCompact)
	restoreTrackerConversationUsage(tracker, initialSnapshot.Usage)
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
	}, func(_ context.Context, notification agentcontract.RuntimeNotification) error {
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
	// Wires mode changes to permissions.Checker and interaction.PlanState.
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

	// Banner (provider/model) was already set during TUI construction.
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
		persistErr = i18n.WrapError(i18n.KeyREPLTUILifecycleSaveFailed, persistErr)
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

type tuiBackgroundFollowUp func(context.Context, agentcontract.RuntimeNotification) error

func installTUIBackgroundNotifications(manager tuiBackgroundTaskPresentation, renderer tuiInfoRenderer, followUps ...tuiBackgroundFollowUp) func() {
	return installLocalizedTUIBackgroundNotifications(manager, renderer, i18n.DetectOrLoadLanguage, followUps...)
}

func installLocalizedTUIBackgroundNotifications(manager tuiBackgroundTaskPresentation, renderer tuiInfoRenderer, language func() i18n.Language, followUps ...tuiBackgroundFollowUp) func() {
	if manager == nil || renderer == nil {
		return func() {}
	}
	observer := tuiRuntimeNotificationSinkFunc(func(_ context.Context, notification agentcontract.RuntimeNotification) error {
		lang := i18n.DetectOrLoadLanguage()
		if language != nil {
			lang = language()
		}
		snapshot, ok := manager.ResolveNotificationTarget(notification)
		if !ok {
			return nil
		}
		notification = manager.LocalizeRuntimeNotification(lang, notification, snapshot)
		message := notification.Message
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
	var followUp tuiRuntimeNotificationSink
	if len(followUps) > 0 && followUps[0] != nil {
		followUp = tuiRuntimeNotificationSinkFunc(func(ctx context.Context, notification agentcontract.RuntimeNotification) error {
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

func backgroundAgentGroupSummaryInLanguage(lang i18n.Language, snapshots []agentcontract.TaskSnapshot, current agentcontract.TaskSnapshot) string {
	latest := make(map[string]agentcontract.TaskSnapshot)
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

func backgroundAgentMemberSummaryInLanguage(lang i18n.Language, snapshot agentcontract.TaskSnapshot) string {
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

func runTUIBackgroundFollowUp(ctx context.Context, cfg TUIREPLConfig, tuiApp *tui.App, tracker *ui.CostTracker, notification agentcontract.RuntimeNotification) error {
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
	base := presentation.ToolEventContext{SessionID: sessionID, SessionEpoch: epoch,
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
		case stream.EventRequestStart, stream.EventRequestFailed:
			providerRequestCompleted = false
		case stream.EventRequestEnd:
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

func backgroundFollowUpProjectMatches(manager tuiBackgroundTaskPresentation, targetProjectRoot string) bool {
	return manager != nil && sameTUIProjectRoot(targetProjectRoot, manager.CurrentProjectRoot())
}

func bindTaskCreateViewState(tool *tooltasks.TaskCreateTool, target any) func() {
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
	convert := func(items []tooltasks.TaskViewItem) []tui.TaskViewItem {
		out := make([]tui.TaskViewItem, len(items))
		for i, item := range items {
			out[i] = tui.TaskViewItem{
				ID: item.ID, Subject: item.Subject, Status: item.Status, Owner: item.Owner,
				BlockedBy: append([]string(nil), item.BlockedBy...),
			}
		}
		return out
	}
	refresh := func() {
		update(func() { state.RefreshTasksView(convert(tool.TaskViewSnapshot())) })
	}
	if len(state.TaskViewItems.Get()) == 0 {
		refresh()
	}
	return tool.SubscribeChanges(refresh)
}

func bindTUIAgentProgress(tool tuiAgentProgressSource, manager tuiBackgroundTaskPresentation, app *tui.App) func() {
	if tool == nil || app == nil {
		return func() {}
	}
	return tool.SubscribeProgress(func(progress agentcontract.ProgressEvent) {
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

func agentProgressActivityEventInLanguage(lang i18n.Language, progress agentcontract.ProgressEvent, epoch uint64) tui.ActivityEvent {
	lifecycle, outcome := tui.ActivityLifecycleRunning, tui.OutcomeRunning
	switch progress.Phase {
	case agentcontract.ProgressCompleted, agentcontract.ProgressBackground:
		lifecycle, outcome = tui.ActivityLifecycleCompleted, tui.OutcomeSucceeded
	case agentcontract.ProgressError:
		lifecycle, outcome = tui.ActivityLifecycleFailed, tui.OutcomeFailed
	case agentcontract.ProgressAborted:
		lifecycle, outcome = tui.ActivityLifecycleCancelled, tui.OutcomeCancelled
	}
	return tui.ActivityEvent{
		ID:        "tool:" + progress.ParentToolUseID,
		SessionID: progress.SessionID, Epoch: epoch, TurnID: progress.TurnID, WorkUnitID: progress.WorkUnitID,
		Kind: tui.ActivityAgent, Name: "Agent", Phase: tui.ActivityPhaseExecuting, Lifecycle: lifecycle, Outcome: outcome,
		Attention:      tui.ActivityAttention{Kind: tui.ActivityAttentionNone},
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

func bindTUIBackgroundActivities(manager tuiBackgroundTaskPresentation, app tuiActivityApp) func() {
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
			if snapshot.Type == "local_agent" && idleRegistration &&
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
					_, conclusionOutcome, _ := backgroundActivityLifecycle(snapshot)
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
// shortly after any semantic view transition. The revision token covers messages,
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
	// user can start another query.
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

func backgroundActivitySignature(snapshot agentcontract.TaskSnapshot) string {
	progressSequence := uint64(0)
	progressPhase := agentcontract.ProgressPhase("")
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

func backgroundActivityEventInLanguage(lang i18n.Language, snapshot agentcontract.TaskSnapshot, sessionID string, epoch uint64, name, actorType, actorID string, detailRefs []tui.DetailRef, jumpTarget string, transcriptState ...bool) tui.ActivityEvent {
	lifecycle, outcome, attention := backgroundActivityLifecycle(snapshot)
	event := tui.ActivityEvent{
		ID: "background:" + snapshot.ID, RunID: snapshot.CurrentRunID, Attempt: snapshot.Attempt,
		BatchID: snapshot.BatchID, ParentRunID: snapshot.ParentRunID, AgentPath: snapshot.AgentPath,
		SessionID: sessionID, Epoch: epoch, WorkUnitID: snapshot.ID,
		Actor: tui.ActivityActor{ID: actorID, Type: actorType}, Kind: tui.ActivityBackground,
		Name: name, Phase: backgroundActivityPhase(snapshot, name, actorType), Lifecycle: lifecycle, Outcome: outcome,
		Attention: attention,
		Control:   tui.ActivityControl{Cancelable: lifecycle == tui.ActivityLifecycleRunning, JumpTarget: jumpTarget, DetailRefs: detailRefs},
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

func backgroundAgentProgressMessageInLanguage(lang i18n.Language, progress agentcontract.ProgressEvent) string {
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

func backgroundActivityPhase(snapshot agentcontract.TaskSnapshot, name, actorType string) tui.ActivityPhase {
	return tui.ActivityPhaseForTool(snapshot.Type+" "+name, map[string]any{"command": snapshot.Command}, actorType)
}

func backgroundActivityLifecycle(snapshot agentcontract.TaskSnapshot) (tui.ActivityLifecycle, tui.ObservationOutcome, tui.ActivityAttention) {
	noAttention := tui.ActivityAttention{Kind: tui.ActivityAttentionNone}
	readyForReview := tui.ActivityAttention{Kind: tui.ActivityAttentionReadyForReview, Severity: tui.ActivityAttentionSeverityInfo, Unread: true}
	switch snapshot.Outcome {
	case agentcontract.RunOutcomeSucceeded:
		if snapshot.Type == "local_agent" && snapshot.Detached {
			return tui.ActivityLifecycleCompleted, tui.OutcomeSucceeded, readyForReview
		}
		return tui.ActivityLifecycleCompleted, tui.OutcomeSucceeded, noAttention
	case agentcontract.RunOutcomePartial:
		return tui.ActivityLifecycleFailed, tui.OutcomePartial, noAttention
	case agentcontract.RunOutcomeCancelled:
		return tui.ActivityLifecycleCancelled, tui.OutcomeCancelled, noAttention
	case agentcontract.RunOutcomeTimedOut:
		return tui.ActivityLifecycleCancelled, tui.OutcomeTimedOut, noAttention
	case agentcontract.RunOutcomeInterrupted:
		return tui.ActivityLifecycleFailed, tui.OutcomeOrphan, noAttention
	case agentcontract.RunOutcomeFailed:
		return tui.ActivityLifecycleFailed, tui.OutcomeFailed, noAttention
	}
	switch snapshot.Status {
	case "completed":
		if snapshot.Type == "local_agent" && snapshot.Detached {
			return tui.ActivityLifecycleCompleted, tui.OutcomeSucceeded, readyForReview
		}
		return tui.ActivityLifecycleCompleted, tui.OutcomeSucceeded, noAttention
	case "killed", "cancelled":
		return tui.ActivityLifecycleCancelled, tui.OutcomeCancelled, noAttention
	case "failed":
		return tui.ActivityLifecycleFailed, tui.OutcomeFailed, noAttention
	default:
		return tui.ActivityLifecycleRunning, tui.OutcomeRunning, noAttention
	}
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

	pendingImages := tuiApp.State().PendingImages.Get()
	if submission.imagesCaptured {
		pendingImages = append([]tui.ImageAttachment(nil), submission.images...)
	}
	rawInput, imagePositions := extractPendingImagePositions(submission.text, pendingImages)
	inputStr := rawInput
	inputStr = strings.TrimSpace(inputStr)
	imagePositions = adjustPendingImagePositionsForTrim(rawInput, inputStr, imagePositions)
	pendingImageCount := len(pendingImages)
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
	buildCmdCtx := func(suppressTerminalOutput bool) *commands.Context {
		totalIn, totalOut := tracker.TotalTokens()
		cacheRead, cacheMake := tracker.TotalCacheTokens()
		webSearchRequests := tracker.TotalWebSearchRequests()
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
		presentationSink := newTUICommandPresentationSink(tuiApp, sessionID, tuiApp.State().SessionEpoch.Get(), commandRunID, sessionScopedOutput)
		if suppressTerminalOutput {
			// /compact owns a live, in-place progress component and terminal
			// receipt. Replaying the captured command prose after completion
			// would put its present-tense "running" text after the success event.
			presentationSink = newTUICommandPresentationSink(tuiApp, sessionID, tuiApp.State().SessionEpoch.Get(), commandRunID)
		}
		return &commands.Context{
			Language:              tuiApp.State().Language.Get(),
			QueryLoop:             ql,
			OnEvent:               sessionScopedOutput,
			OnCommandPresentation: presentationSink,
			CWD:                   currentCWD(cfg),
			CurrentProjectDir:     projectDir,
			SessionID:             sessionID,
			SessionStore:          storeAdapter,
			GoalRuntime:           newSessionGoalRuntime(cfg, sessionID, projectDir),
			BuildDiagnostic:       currentBuildDiagnostic(cfg),
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
			CostCurrency:             tracker.Currency(),
			CostUnknown:              !tracker.CostKnown(),
			CompactFunc: func(customInstructions string) error {
				if cfg.SessionTransitionMu != nil {
					cfg.SessionTransitionMu.Lock()
					defer cfg.SessionTransitionMu.Unlock()
				}
				base := presentation.ToolEventContext{
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
			CurrentProvider:          cfg.Engine.Provider().Name(),
			ProviderRegistry:         cfg.ProviderRegistry,
			CredentialStore:          cfg.CredentialStore,
			ProviderRuntimeOverrides: cfg.ProviderRuntimeOverrides,
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
			tracker.RestoreCompactionBaselineState(usage.HasCompacted, usage.CompactionBaselineKnown, usage.InputTokensAtCompact, usage.CacheReadAtCompact)
			restoreTrackerConversationUsage(tracker, usage)
		}
		r.SessionInfo(*cfg.SessionID, cfg.Engine.Tools())
		info, err := cfg.Engine.ContextUsage(*cfg.SessionID)
		if err != nil || info == nil || info.TotalTokens <= 0 {
			r.ContextBar(0, 0)
			return
		}
		r.ModelContext(modelContextFromEngine(info))
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
		cmdCtx := buildCmdCtx(cmd.Name() == "compact")
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
		req = attachPendingImagesToQuery(req, pendingImages, imagePositions)
	}

	tuiRunQuery(ctx, cfg, tuiApp, req, tracker, sigHandler)
}

func attachPendingImagesToQuery(req engine.QueryRequest, images []tui.ImageAttachment, positions ...map[int]pendingImagePosition) engine.QueryRequest {
	if len(images) == 0 {
		return req
	}
	if len(req.Content) != 0 || len(positions) == 0 {
		if len(req.Content) == 0 {
			if req.Message != "" {
				req.Content = append(req.Content, types.TextBlock{Type: types.ContentTypeText, Text: req.Message})
			}
			req.Message = ""
		}
		for _, image := range images {
			req.Content = append(req.Content, pendingImageBlock(image))
		}
		return req
	}

	ordered := append([]tui.ImageAttachment(nil), images...)
	textRunes := []rune(req.Message)
	positionByID := positions[0]
	attachmentOrder := make(map[int]int, len(images))
	for index, image := range images {
		attachmentOrder[image.ID] = index
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, leftSet := positionByID[ordered[i].ID]
		right, rightSet := positionByID[ordered[j].ID]
		if !leftSet {
			left = pendingImagePosition{offset: len(textRunes), order: len(positionByID) + attachmentOrder[ordered[i].ID]}
		}
		if !rightSet {
			right = pendingImagePosition{offset: len(textRunes), order: len(positionByID) + attachmentOrder[ordered[j].ID]}
		}
		if left.offset != right.offset {
			return left.offset < right.offset
		}
		return left.order < right.order
	})
	cursor := 0
	for _, image := range ordered {
		placement, ok := positionByID[image.ID]
		if !ok {
			placement.offset = len(textRunes)
		}
		position := min(max(placement.offset, cursor), len(textRunes))
		if position > cursor {
			req.Content = append(req.Content, types.TextBlock{Type: types.ContentTypeText, Text: string(textRunes[cursor:position])})
		}
		req.Content = append(req.Content, pendingImageBlock(image))
		cursor = position
	}
	if cursor < len(textRunes) {
		req.Content = append(req.Content, types.TextBlock{Type: types.ContentTypeText, Text: string(textRunes[cursor:])})
	}
	req.Message = ""
	return req
}

func pendingImageBlock(image tui.ImageAttachment) types.ImageBlock {
	return types.ImageBlock{
		Type: types.ContentTypeImage,
		Source: &types.ImageSource{
			Type:      "base64",
			MediaType: image.MediaType,
			Data:      image.Base64,
		},
	}
}

type pendingImagePlaceholderMatch struct {
	image tui.ImageAttachment
	start int
	end   int
}

type pendingImagePosition struct {
	offset int
	order  int
}

func extractPendingImagePositions(text string, images []tui.ImageAttachment) (string, map[int]pendingImagePosition) {
	matches := make([]pendingImagePlaceholderMatch, 0, len(images))
	for _, image := range images {
		token := " " + image.Placeholder + " "
		start := strings.Index(text, token)
		if start < 0 {
			token = image.Placeholder
			start = strings.Index(text, token)
		}
		if start >= 0 && token != "" {
			matches = append(matches, pendingImagePlaceholderMatch{image: image, start: start, end: start + len(token)})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].start < matches[j].start })

	positions := make(map[int]pendingImagePosition, len(matches))
	var output strings.Builder
	runeCount := 0
	cursor := 0
	appendSegment := func(segment string) {
		if segment == "" {
			return
		}
		if output.Len() > 0 {
			last, _ := utf8.DecodeLastRuneInString(output.String())
			first, _ := utf8.DecodeRuneInString(segment)
			if !unicode.IsSpace(last) && !unicode.IsSpace(first) {
				output.WriteByte(' ')
				runeCount++
			}
		}
		output.WriteString(segment)
		runeCount += utf8.RuneCountInString(segment)
	}
	for order, match := range matches {
		if match.start < cursor {
			continue
		}
		appendSegment(text[cursor:match.start])
		positions[match.image.ID] = pendingImagePosition{offset: runeCount, order: order}
		cursor = match.end
	}
	appendSegment(text[cursor:])
	return output.String(), positions
}

func adjustPendingImagePositionsForTrim(raw, trimmed string, positions map[int]pendingImagePosition) map[int]pendingImagePosition {
	leadingRunes := utf8.RuneCountInString(raw) - utf8.RuneCountInString(strings.TrimLeftFunc(raw, unicode.IsSpace))
	maximum := utf8.RuneCountInString(trimmed)
	for id, position := range positions {
		position.offset = min(max(position.offset-leadingRunes, 0), maximum)
		positions[id] = position
	}
	return positions
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
	artifactRoot := tuiSessionArtifactRoot(cfg, sessionID, namespace)
	if strings.TrimSpace(artifactRoot) == "" {
		return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, i18n.NewError(i18n.KeyTUISessionViewMissingCheckpoint))
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
			if scopeGeneration != 0 {
				contextGeneration = engine.ContextGenerationState{Generation: scopeGeneration, Persisted: true}
				identity = identity.WithInternalControlScope(messagecontrol.Runtime(), controlScope)
			}
		} else if contextGeneration.Persisted {
			return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, session.ErrCorruptSessionHistory)
		}
	}

	exact, restored, restoreErr := tui.LoadSessionViewCheckpoint(artifactRoot, messages, identity)
	if restoreErr != nil {
		return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, restoreErr)
	}
	if !restored {
		return tui.SessionSnapshot{}, replWrap(i18n.KeyREPLErrorLoadLifecycleMetadata, i18n.NewError(i18n.KeyTUISessionViewMissingCheckpoint))
	}
	exact.ContextGeneration = contextGeneration.Generation
	exact.ContextGenerationPersisted = contextGeneration.Persisted
	return exact, nil
}

func tuiSessionArtifactRoot(cfg TUIREPLConfig, sessionID, namespace string) string {
	if cfg.Repo != nil {
		return cfg.Repo.ArtifactsDir(sessionID, namespace)
	}
	if cfg.Engine != nil && cfg.Engine.Sessions() != nil {
		if provider, ok := cfg.Engine.Sessions().(engine.SessionArtifactsDirProvider); ok {
			return provider.ArtifactsDir(sessionID)
		}
	}
	return ""
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
	// Publish the exact restorable view before its metadata projection. If the
	// checkpoint fails, metadata must remain at the last restorable boundary.
	// If metadata fails, surface the error while retaining the checkpoint so a
	// later lifecycle save can reconcile the summary from a fresh capture.
	if err := tui.SaveSessionViewCapture(cfg.Repo.ArtifactsDir(*cfg.SessionID, projectDir), capture); err != nil {
		return err
	}
	return saveTUISessionMeta(cfg, *cfg.SessionID, projectDir, session.SessionMeta{
		Usage: sessionUsageMetaFromTUIView(capture.View),
	})
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
		if rollbackErr := cfg.PlanState.ExitForSessionRestore("default"); rollbackErr != nil {
			err = errors.Join(err, rollbackErr)
		}
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
			if rollbackErr := cfg.PlanState.ExitForSessionRestore(previousRuntimeMode); rollbackErr != nil {
				err = errors.Join(err, rollbackErr)
			}
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
	providers := make([]tui.ProviderPickerEntry, 0, len(allProviders)+1)
	for _, p := range allProviders {
		models := catalog.ListByProvider(p.Name)
		connection := cfg.ProviderRegistry.ConnectionState(p.Name)
		language := tuiApp.State().Language.Get()
		baseURL := ""
		apiStyle := provider.APIStyleOpenAI
		if cfg.CredentialStore != nil {
			for _, credentialName := range provider.CredentialLookupNames(p.Name) {
				if credential, ok := cfg.CredentialStore.Get(credentialName); ok {
					baseURL = credential.BaseURL
					apiStyle = provider.ParseAPIStyle(string(credential.APIStyle))
					break
				}
			}
		}
		providers = append(providers, tui.ProviderPickerEntry{
			Name:            p.Name,
			DisplayName:     p.DisplayName,
			ModelCount:      len(models),
			IsActive:        p.Name == currentProvider,
			IsConnected:     connection.State == provider.ConnectionStateConnected,
			ConnectionState: string(connection.State),
			ConnectionLabel: connection.DetailText(language),
			CanSelectModels: connection.CanSelectModels,
			CanConnect:      connection.CanConnect,
			SetupHint:       connection.SetupHintText(language),
			AuthMethods:     p.AuthMethods,
			EnvKey:          p.EnvKey,
			BaseURL:         baseURL,
			DefaultBaseURL:  p.DefaultBaseURL,
			APIStyles:       append([]provider.APIStyle(nil), p.APIStyles...),
			APIStyle:        apiStyle,
			DefaultBaseURLs: cloneProviderBaseURLs(p.DefaultBaseURLs),
			DynamicModels:   p.DynamicModels,
			UserDefined:     p.UserDefined,
		})
	}
	providers = append(providers, tui.ProviderPickerEntry{
		Name:            "__other__",
		DisplayName:     i18n.Text(tuiApp.State().Language.Get(), i18n.KeyProviderPickerOther),
		ConnectionState: string(provider.ConnectionStateUnknown),
		CanConnect:      true,
		AuthMethods:     []string{"api_key"},
		APIStyles:       []provider.APIStyle{provider.APIStyleOpenAI, provider.APIStyleAnthropic},
		APIStyle:        provider.APIStyleOpenAI,
		DynamicModels:   true,
		IsCreate:        true,
	})

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
					if strings.TrimSpace(cfg.ProviderRuntimeOverrides.APIFormat) != "" {
						providerCfg.APIFormat = cfg.ProviderRuntimeOverrides.APIFormat
					}
					providerCfg.ResponsesWebSocket = cfg.ProviderRuntimeOverrides.ResponsesWebSocket
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
					r.ModelContext(modelContextFromEngine(info))
				} else {
					// Fallback: session doesn't exist yet. Use model catalog.
					maxCtx := provider.LookupMaxContext(entry.ModelID)
					if modelInfo, ok := catalog.ResolveForProvider(entry.Provider, entry.ModelID); ok {
						maxCtx = modelInfo.ContextWindow
					}
					if maxCtx > 0 {
						r.ModelContext(presentation.ModelContextProjection{
							Scope: presentation.UsageScopeModelContext, Known: true,
							CapacityTokens: maxCtx,
							Measurement:    presentation.ContextMeasurementUnknown,
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
	picker.OnConnect = func(request tui.ProviderConnectRequest) {
		providerName := provider.CanonicalProviderName(request.ProviderName)
		go func() {
			language := func() i18n.Language { return tuiApp.State().Language.Get() }
			switch request.AuthMethod {
			case "api_key":
				baseURL := strings.TrimSpace(request.BaseURL)
				apiKey := strings.TrimSpace(request.APIKey)
				apiStyle := provider.ParseAPIStyle(string(request.APIStyle))
				info, exists := cfg.ProviderRegistry.Get(providerName)
				creating := request.UserDefined && (!exists || providerName == "__other__")
				displayName := strings.TrimSpace(request.DisplayName)
				if creating {
					displayName = provider.CompatibleProviderDisplayName(displayName, baseURL)
					providerName = cfg.ProviderRegistry.NextUserProviderName(displayName, baseURL)
					info = provider.ProviderInfo{
						Name: providerName, DisplayName: displayName, DynamicModels: true, UserDefined: true,
						APIStyles: []provider.APIStyle{provider.APIStyleOpenAI, provider.APIStyleAnthropic},
					}
				}
				effectiveBaseURL := baseURL
				if effectiveBaseURL == "" {
					effectiveBaseURL = info.BaseURLForStyle(apiStyle)
				}
				if effectiveBaseURL == "" {
					setModelPickerConnectError(tuiApp, request.ProviderName, i18n.Text(language(), i18n.KeyProviderCompatibleBaseURLRequired))
					return
				}
				if err := validateCustomEndpointInLanguage(language(), providerName, effectiveBaseURL); err != nil {
					setModelPickerConnectError(tuiApp, request.ProviderName, err.Error())
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
				var models []provider.ModelInfo
				if info.DynamicModels || request.UserDefined {
					setModelPickerConnectStatus(tuiApp, request.ProviderName, i18n.Text(language(), i18n.KeyREPLTUIFetchingModels))
					discovered, err := cfg.ProviderRegistry.DiscoverCompatibleModels(context.Background(), provider.CompatibleModelRequest{
						Provider: providerName, APIStyle: apiStyle, BaseURL: effectiveBaseURL, APIKey: apiKey,
					})
					if err != nil {
						setModelPickerConnectError(tuiApp, request.ProviderName, localizedProviderConnectError(language(), err))
						return
					}
					models = discovered
				}
				if request.UserDefined && !creating && displayName == "" {
					if previous, ok := cfg.CredentialStore.Get(providerName); ok {
						displayName = previous.DisplayName
					}
				}
				entry := provider.CredentialEntry{
					Provider:    providerName,
					AuthMethod:  "api_key",
					APIKey:      apiKey,
					BaseURL:     baseURL,
					APIStyle:    apiStyle,
					DisplayName: displayName,
					UserDefined: request.UserDefined,
					Models:      models,
					LastUsed:    time.Now(),
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
				if request.UserDefined {
					cfg.ProviderRegistry.RegisterCompatibleProvider(provider.CompatibleProviderDefinition{
						Name: providerName, DisplayName: displayName, UserDefined: true,
					})
				}
				if len(models) > 0 {
					cfg.ProviderRegistry.ReplaceProviderModels(providerName, models)
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
	picker.OnDelete = func(providerName string) error {
		if cfg.CredentialStore == nil {
			return errors.New(i18n.Text(tuiApp.State().Language.Get(), i18n.KeyREPLTUICredentialStoreMissing))
		}
		if err := cfg.CredentialStore.Delete(providerName); err != nil {
			return err
		}
		cfg.ProviderRegistry.UnregisterUserProvider(providerName)
		r.Info(i18n.Format(tuiApp.State().Language.Get(), i18n.KeyREPLTUIProviderDeleted, providerName))
		return nil
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

func modelContextFromEngine(info *engine.ContextUsageInfo) presentation.ModelContextProjection {
	if info == nil {
		return presentation.ModelContextProjection{Scope: presentation.UsageScopeModelContext}
	}
	measurement := presentation.ContextMeasurementUnknown
	switch compact.ContextUsageMeasurement(info.Measurement) {
	case compact.ContextUsageProviderReported:
		measurement = presentation.ContextMeasurementProviderReported
	case compact.ContextUsageLocalEstimate:
		measurement = presentation.ContextMeasurementLocalEstimate
	case compact.ContextUsageLocalLowerBound:
		measurement = presentation.ContextMeasurementLocalLowerBound
	}
	if measurement == presentation.ContextMeasurementUnknown {
		return presentation.ModelContextProjection{Scope: presentation.UsageScopeModelContext}
	}
	percent := modelContextPercent(info.UsedTokens, info.TotalTokens)
	return presentation.ModelContextProjection{
		Scope: presentation.UsageScopeModelContext, Known: info.TotalTokens > 0,
		UsedTokens: info.UsedTokens, CapacityTokens: info.TotalTokens, PercentUsed: percent,
		Measurement: measurement,
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

func localizedProviderConnectError(lang i18n.Language, err error) string {
	if err == nil {
		return ""
	}
	if localizer, ok := err.(interface {
		Localized(i18n.Language) string
	}); ok {
		return localizer.Localized(lang)
	}
	return err.Error()
}

func setModelPickerConnectStatus(tuiApp *tui.App, providerName, message string) {
	tuiApp.GoTuiApp().QueueUpdate(func() {
		p := tuiApp.State().ModelPicker.Get()
		if p == nil || (p.ConnectProvider != "" && p.ConnectProvider != providerName) {
			return
		}
		p.ConnectStatus = message
		p.ConnectError = ""
		tuiApp.State().ModelPicker.Set(p)
	})
}

func cloneProviderBaseURLs(values map[provider.APIStyle]string) map[provider.APIStyle]string {
	result := make(map[provider.APIStyle]string, len(values))
	for style, value := range values {
		result[style] = value
	}
	return result
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
		connection := cfg.ProviderRegistry.ConnectionState(providerName)
		language := tuiApp.State().Language.Get()
		info, infoFound := cfg.ProviderRegistry.Get(providerName)
		credential := provider.CredentialEntry{}
		if cfg.CredentialStore != nil {
			credential, _ = cfg.CredentialStore.Get(providerName)
		}
		providerIndex := -1
		for i := range p.Providers {
			if p.Providers[i].Name == providerName {
				providerIndex = i
				if infoFound {
					p.Providers[i].DisplayName = info.DisplayName
				}
				p.Providers[i].IsConnected = connection.State == provider.ConnectionStateConnected
				p.Providers[i].ConnectionState = string(connection.State)
				p.Providers[i].ConnectionLabel = connection.DetailText(language)
				p.Providers[i].CanSelectModels = connection.CanSelectModels
				p.Providers[i].CanConnect = connection.CanConnect
				p.Providers[i].ModelCount = len(catalog.ListByProvider(providerName))
				p.Providers[i].APIStyle = provider.ParseAPIStyle(string(credential.APIStyle))
				if cfg.CredentialStore != nil {
					p.Providers[i].BaseURL = credential.BaseURL
				}
				p.ProviderSelected = i
				break
			}
		}
		if providerIndex < 0 && infoFound {
			entry := tui.ProviderPickerEntry{
				Name: providerName, DisplayName: info.DisplayName,
				ModelCount:      len(catalog.ListByProvider(providerName)),
				IsConnected:     connection.State == provider.ConnectionStateConnected,
				ConnectionState: string(connection.State), ConnectionLabel: connection.DetailText(language),
				CanSelectModels: connection.CanSelectModels, CanConnect: connection.CanConnect,
				SetupHint: connection.SetupHintText(language), AuthMethods: info.AuthMethods,
				BaseURL: credential.BaseURL, DefaultBaseURL: info.DefaultBaseURL,
				APIStyles:       append([]provider.APIStyle(nil), info.APIStyles...),
				APIStyle:        provider.ParseAPIStyle(string(credential.APIStyle)),
				DefaultBaseURLs: cloneProviderBaseURLs(info.DefaultBaseURLs),
				DynamicModels:   info.DynamicModels, UserDefined: info.UserDefined,
			}
			insertAt := len(p.Providers)
			if insertAt > 0 && p.Providers[insertAt-1].IsCreate {
				insertAt--
			}
			p.Providers = append(p.Providers, tui.ProviderPickerEntry{})
			copy(p.Providers[insertAt+1:], p.Providers[insertAt:])
			p.Providers[insertAt] = entry
			p.ProviderSelected = insertAt
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
		p.ConnectDefaultBaseURL = ""
		p.ConnectProviderNameInput = ""
		p.ConnectAPIStyles = nil
		p.ConnectDefaultBaseURLs = nil
		p.ConnectSelectedStyle = 0
		p.ConnectDynamicModels = false
		p.ConnectUserDefined = false
		p.ConnectInputField = tui.ConnectInputAPIKey
		p.ConnectStatus = ""
		p.ConnectError = ""
		p.ConnectHint = ""
		p.IsReconnect = false
		tuiApp.State().ModelPicker.Set(p)
		tuiApp.GoTuiApp().RequestFullRedraw()
	})
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
		reasoningEffort := provider.DefaultReasoningEffortForModel(m)
		if providerName == currentProvider && modelPickerModelMatches(m, currentModel) && modelPickerEffortAvailable(m.ReasoningEfforts, currentEffort) {
			reasoningEffort = currentEffort
		}
		entries[i] = tui.ModelPickerEntry{
			Provider:               m.Provider,
			ModelID:                m.ID,
			DisplayName:            m.Name,
			ContextK:               ctxK,
			ContextTokens:          m.ContextWindow,
			ContextOverridden:      m.ContextOverridden,
			CostIn:                 m.CostPer1MIn,
			CostOut:                m.CostPer1MOut,
			CostCurrency:           m.BillingCurrency(),
			CanReason:              m.CanReason,
			CanSeeImages:           m.CanSeeImages,
			ReasoningEfforts:       append([]string(nil), m.ReasoningEfforts...),
			DefaultReasoningEffort: m.DefaultReasoningEffort,
			ReasoningEffort:        reasoningEffort,
			IsDefault:              m.IsDefault,
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
		Language:                 lang,
		OnEvent:                  r.Info,
		ProviderRegistry:         cfg.ProviderRegistry,
		CredentialStore:          cfg.CredentialStore,
		ProviderRuntimeOverrides: cfg.ProviderRuntimeOverrides,
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

func activityHasAction(activity tui.Activity, want tui.ActivityAction) bool {
	for _, action := range activity.Actions {
		if action == want {
			return true
		}
	}
	return false
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
	base := presentation.ToolEventContext{
		SessionID: req.SessionID, SessionEpoch: reqEpoch, ContextGeneration: tuiApp.State().ContextGeneration.Get(),
		ContextGenerationPersisted: tuiApp.State().ContextGenerationPersisted.Get(),
	}
	onEvent, stopToolSpinners := makeTUIEventHandler(r, tracker, getContextUsage, base)
	defer stopToolSpinners()

	var runErr error
	providerRequestCompleted := false
	var renderedRuntimeErrors []*types.APIError
	terminalContext := base
	for evt := range ch {
		if evt.Final {
			runErr = evt.Error
		} else {
			if evt.Inner.Type == stream.EventError && evt.Inner.Error != nil {
				renderedRuntimeErrors = append(renderedRuntimeErrors, evt.Inner.Error)
			}
			switch evt.Inner.Type {
			case stream.EventRequestStart, stream.EventRequestFailed:
				providerRequestCompleted = false
			case stream.EventRequestEnd:
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
			if !runtimeErrorCauseAlreadyRendered(runErr, renderedRuntimeErrors) {
				r.ErrorAtContext(terminalContext, i18n.Format(lang, i18n.KeyREPLQueryFailed, engine.UserFacingError(lang, runErr)))
			}
			if status, update := terminalTUIProviderStatus(runErr, providerRequestCompleted); update {
				r.SetProviderStatusAtContext(terminalContext, status)
			}
		}
	} else {
		// Query succeeded — provider is connected
		r.SetProviderStatusAtContext(terminalContext, tui.StatusConnected)
	}
}

func runtimeErrorCauseAlreadyRendered(runErr error, rendered []*types.APIError) bool {
	if runErr == nil {
		return false
	}
	terminal, terminalIsAPIError := provider.AsAPIError(runErr)
	for _, apiErr := range rendered {
		if apiErr == nil {
			continue
		}
		if errors.Is(runErr, apiErr) {
			return true
		}
		// Provider/runtime adapters may preserve a failure by value while
		// wrapping it with localized context. errors.Is only recognizes the
		// original pointer because APIError intentionally has no broad Is
		// method. Within one query, an exact structured copy is still the same
		// already-rendered failure; comparing the complete value avoids both a
		// duplicate main error and accidental suppression of another request
		// whose status, request ID, message, or protocol evidence differs.
		if terminalIsAPIError && terminal != nil && reflect.DeepEqual(*terminal, *apiErr) {
			return true
		}
	}
	return false
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
func makeTUIEventHandler(r presentation.Renderer, tracker *ui.CostTracker, getContextUsage func() (int, int), baseContexts ...presentation.ToolEventContext) (func(stream.Event), func()) {
	turnStart := time.Now()
	baseContext := presentation.ToolEventContext{}
	if len(baseContexts) > 0 {
		baseContext = baseContexts[0]
	}
	guarded, hasEpochRenderer := r.(tuiEpochRenderer)
	contextGuarded, hasContextRenderer := r.(tuiContextEventRenderer)
	costKnownRenderer, hasCostKnownRenderer := r.(tuiCostKnownEpochRenderer)
	sessionUsageRenderer, hasSessionUsageRenderer := r.(tuiSessionUsageRenderer)
	activityRenderer, hasActivityRenderer := r.(tuiActivityEpochRenderer)
	compactionProgressRenderer, hasCompactionProgressRenderer := r.(tuiCompactionProgressEpochRenderer)
	progressiveMetricsRenderer, hasProgressiveMetricsRenderer := r.(tuiProgressiveContextMetricsEpochRenderer)
	compactionBoundaryRenderer, hasCompactionBoundaryRenderer := r.(tuiCompactionBoundaryEpochRenderer)
	goalStatusRenderer, hasGoalStatusRenderer := r.(tuiGoalStatusEpochRenderer)
	llmRequestRenderer, hasLLMRequestRenderer := r.(tuiLLMRequestEpochRenderer)
	llmActivityContextRenderer, hasLLMActivityContextRenderer := r.(tuiLLMActivityContextRenderer)
	llmActivityEpochRenderer, hasLLMActivityEpochRenderer := r.(tuiLLMActivityEpochRenderer)
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
	handle := func(event stream.Event) {
		if !presentation.AdmitContextGeneration(r, baseContext) {
			return
		}
		presentation.SetRenderTurn(r, event.TurnCount)
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
		if event.Type != stream.EventSystemWarning && event.ProjectRoot != "" {
			eventContext.ProjectRoot = event.ProjectRoot
		}
		switch event.Type {
		case stream.EventRequestStart, stream.EventRequestRetry, stream.EventRequestFirstToken, stream.EventRequestEnd, stream.EventRequestFailed:
			if hasContextRenderer && event.RequestStatus != nil {
				contextGuarded.LLMRequestStatusAtContext(eventContext, event.Type, *event.RequestStatus)
			} else if hasLLMRequestRenderer && event.RequestStatus != nil {
				llmRequestRenderer.LLMRequestStatusAtEpoch(baseContext.SessionEpoch, event.Type, *event.RequestStatus)
			}
		case stream.EventText:
			if hasContextRenderer {
				contextGuarded.TextAtContext(eventContext, event.Text)
			} else if hasEpochRenderer {
				guarded.TextAtEpoch(baseContext.SessionEpoch, event.Text)
			} else {
				r.Text(event.Text)
			}
		case stream.EventThinking:
			if hasContextRenderer {
				contextGuarded.ThinkingAtContext(eventContext, event.Text)
			} else if hasEpochRenderer {
				guarded.ThinkingAtEpoch(baseContext.SessionEpoch, event.Text)
			} else {
				r.Thinking(event.Text)
			}
		case stream.EventToolUse:
			if event.ToolUse != nil {
				presentation.DispatchToolCallEvent(r, eventContext, *event.ToolUse)
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
		case stream.EventToolResult:
			if event.ToolResult != nil {
				stopToolSpinner(event.ToolResult.ToolUseID)
				presentation.DispatchToolResultEvent(r, eventContext, *event.ToolResult)
			}
		case stream.EventGoalEvaluation, stream.EventProviderUsage:
			costDelta, cumulativeCost, recorded := recordAuxiliaryUsageEvent(tracker, event)
			if recorded && hasSessionUsageRenderer {
				sessionUsageRenderer.SessionUsageAtContext(eventContext, ui.BuildSessionUsageProjection(tracker))
			}
			if recorded && hasContextRenderer && !hasSessionUsageRenderer {
				contextGuarded.CostSummaryAtContext(eventContext, costDelta, cumulativeCost, event.Usage.InputTokens, event.Usage.OutputTokens)
				contextGuarded.CostKnownAtContext(eventContext, tracker.CostKnown())
			} else if recorded && hasEpochRenderer && !hasSessionUsageRenderer {
				// Auxiliary provider calls are part of session usage/cost but are not
				// conversation requests, so they only update cumulative cost here.
				// Full token totals are synchronized from the tracker at query release.
				guarded.CostSummaryAtEpoch(baseContext.SessionEpoch, costDelta, cumulativeCost, event.Usage.InputTokens, event.Usage.OutputTokens)
				if hasCostKnownRenderer {
					costKnownRenderer.CostKnownAtEpoch(baseContext.SessionEpoch, tracker.CostKnown())
				}
			}
		case stream.EventGoalStatus:
			if hasContextRenderer && event.GoalStatus != nil {
				contextGuarded.GoalStatusAtContext(eventContext, *event.GoalStatus)
			} else if hasGoalStatusRenderer && event.GoalStatus != nil {
				goalStatusRenderer.GoalStatusAtEpoch(baseContext.SessionEpoch, *event.GoalStatus)
			}
		case stream.EventTurnEnd:
			stopAllToolSpinners()
			if hasContextRenderer {
				contextGuarded.FreezeAggregatesAtContext(eventContext)
			} else if hasAggregateFreezer {
				aggregateFreezer.FreezeAggregatesAtEpoch(baseContext.SessionEpoch, eventContext.SessionID, eventContext.TurnID)
			}
			if structured, ok := r.(presentation.StructuredUsageRenderer); ok {
				recordTurnUsageEvent(tracker, event, time.Since(turnStart))
				maxTokens, usedTokens := 0, 0
				if getContextUsage != nil {
					maxTokens, usedTokens = getContextUsage()
				}
				structured.UsageSemantics(ui.BuildUsageSemanticsSnapshot(event.Usage, tracker, usedTokens, maxTokens))
				turnStart = time.Now()
				break
			}
			if hasContextRenderer && !hasSessionUsageRenderer {
				contextGuarded.UsageAtContext(eventContext, event.Usage)
			} else if hasEpochRenderer && !hasSessionUsageRenderer {
				guarded.UsageAtEpoch(baseContext.SessionEpoch, event.Usage)
			} else if !hasSessionUsageRenderer {
				r.Usage(event.Usage)
			}
			if recordTurnUsageEvent(tracker, event, time.Since(turnStart)) {
				if hasSessionUsageRenderer {
					sessionUsageRenderer.SessionUsageAtContext(eventContext, ui.BuildSessionUsageProjection(tracker))
				}
				if hasContextRenderer && !hasSessionUsageRenderer {
					contextGuarded.CostKnownAtContext(eventContext, tracker.CostKnown())
				} else if hasCostKnownRenderer && !hasSessionUsageRenderer {
					costKnownRenderer.CostKnownAtEpoch(baseContext.SessionEpoch, tracker.CostKnown())
				}
				last := tracker.LastTurn()
				if last != nil && !hasSessionUsageRenderer {
					if hasContextRenderer {
						contextGuarded.CostSummaryAtContext(eventContext, last.CostUSD, tracker.TotalCost(), last.InputTokens, last.OutputTokens)
					} else if hasEpochRenderer {
						guarded.CostSummaryAtEpoch(baseContext.SessionEpoch, last.CostUSD, tracker.TotalCost(), last.InputTokens, last.OutputTokens)
					} else {
						if currencyRenderer, ok := r.(presentation.CurrencyCostRenderer); ok {
							currencyRenderer.CostSummaryInCurrency(last.CostUSD, tracker.TotalCost(), tracker.Currency(), last.InputTokens, last.OutputTokens)
						} else {
							r.CostSummary(last.CostUSD, tracker.TotalCost(), last.InputTokens, last.OutputTokens)
						}
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
		case stream.EventError:
			stopAllToolSpinners()
			if structuredError, ok := r.(tuiRuntimeErrorRenderer); ok {
				structuredError.RuntimeErrorEvent(eventContext, event.ToolUseID, event.Text, event.Error, event.Metadata)
			} else {
				language := i18n.DetectOrLoadLanguage()
				if languageRenderer, ok := r.(presentation.RuntimeLanguageRenderer); ok {
					language = languageRenderer.RuntimeLanguage()
				}
				publicMessage := presentation.RuntimeErrorPublicMessage(eventContext, event.ToolUseID, event.Text, event.Error, event.Metadata, language, false)
				if hasContextRenderer {
					contextGuarded.ErrorAtContext(eventContext, publicMessage)
				} else if hasEpochRenderer {
					guarded.ErrorAtEpoch(baseContext.SessionEpoch, publicMessage)
				} else {
					r.Error(publicMessage)
				}
			}
		case stream.EventSystemWarning:
			language := i18n.DetectOrLoadLanguage()
			if languageRenderer, ok := r.(presentation.RuntimeLanguageRenderer); ok {
				language = languageRenderer.RuntimeLanguage()
			}
			presentation.DispatchRuntimeWarningEvent(r, runtimeevent.SystemWarningRuntimeEvent(event), language, true)
		case stream.EventUserInterruption:
			stopAllToolSpinners()
		case stream.EventProgress:
			if event.Progress != nil && (event.Progress.Stage == stream.ProgressStageLLMToolInput || event.Progress.Stage == stream.ProgressStageLLMWaitingAfterTools) {
				stage, toolName := tui.LLMStageWaitingAfterTools, ""
				toolInputBytes := 0
				if event.Progress.Stage == stream.ProgressStageLLMToolInput {
					stage = tui.LLMStageToolInput
					toolName, _ = event.Progress.Metadata["tool_name"].(string)
					toolInputBytes = progressMetadataInt(event.Progress.Metadata, "tool_input_bytes")
				}
				if hasLLMActivityContextRenderer {
					llmActivityContextRenderer.LLMActivityAtContext(eventContext, stage, toolName, toolInputBytes)
				} else if hasLLMActivityEpochRenderer {
					llmActivityEpochRenderer.LLMActivityAtEpoch(baseContext.SessionEpoch, stage, toolName, toolInputBytes)
				}
				break
			}
			if event.Progress != nil && event.Progress.Stage == "progressive_context_projection" {
				if hasProgressiveMetricsRenderer {
					progressiveMetricsRenderer.ProgressiveContextMetricsAtEpoch(baseContext.SessionEpoch, eventContext, *event.Progress)
				}
				break
			}
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
		case stream.EventCompactBoundary:
			if event.Compact == nil {
				break
			}
			applied := true
			if tracker != nil {
				applied = tracker.MarkCompactionBoundary(presentation.CompactionBoundaryIdentity(eventContext, *event.Compact))
			}
			if !applied {
				break
			}
			if hasCompactionBoundaryRenderer {
				compactionBoundaryRenderer.CompactionBoundaryAtEpoch(baseContext.SessionEpoch, eventContext, *event.Compact)
			}
			if hasSessionUsageRenderer && tracker != nil {
				sessionUsageRenderer.SessionUsageAtContext(eventContext, ui.BuildSessionUsageProjection(tracker))
			}
		case stream.EventHookSummary:
			if event.HookSummary == nil {
				break
			}
			summary := presentation.HookSummary{
				ExecutionID: event.HookSummary.HookExecutionID,
				ToolUseID:   event.HookSummary.ToolUseID,
				Name:        event.HookSummary.HookName,
				Status:      event.HookSummary.Status,
				Summary:     event.HookSummary.Summary,
				Metadata:    event.HookSummary.Metadata,
			}
			if structured, ok := r.(presentation.StructuredHookRenderer); ok {
				structured.RenderHookSummary(eventContext, summary)
			} else {
				lang := i18n.DetectOrLoadLanguage()
				r.Info(i18n.Format(lang, i18n.KeyREPLTUIHookSummary, summary.Name, localizedHookStatus(lang, summary.Status), hookSummarySuffix(summary.Summary)))
			}
		}
	}
	return handle, stopAllToolSpinners
}

func compactionProgressActivityInLanguage(lang i18n.Language, ctx presentation.ToolEventContext, progress *stream.ProgressEvent) (tui.ActivityEvent, bool) {
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
	lifecycle, outcome := tui.ActivityLifecycleRunning, tui.OutcomeRunning
	stage := strings.ToLower(progress.Stage)
	switch {
	case stage == "compact_cancelled":
		lifecycle, outcome = tui.ActivityLifecycleCancelled, tui.OutcomeCancelled
	case strings.Contains(stage, "failure") || strings.Contains(stage, "failed"):
		lifecycle, outcome = tui.ActivityLifecycleFailed, tui.OutcomeFailed
	case stage == "compact_end" || strings.Contains(stage, "success"):
		lifecycle, outcome = tui.ActivityLifecycleCompleted, tui.OutcomeSucceeded
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
		Lifecycle: lifecycle, Outcome: outcome,
		Attention: tui.ActivityAttention{Kind: tui.ActivityAttentionNone},
		Progress:  tui.ActivityProgress{Current: progress.Current, Total: progress.Total, Message: message},
		Control:   tui.ActivityControl{Cancelable: lifecycle == tui.ActivityLifecycleRunning},
	}, true
}

func localizedCompactionProgressMessage(lang i18n.Language, message string) string {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "accepted", "compacting", "compact_start", "compact_accepted", "auto_compact_attempt":
		return i18n.Text(lang, i18n.KeyREPLTUICompactionCompacting)
	case "preparing", "compact_preparing":
		return i18n.Text(lang, i18n.KeyTUICompactProgressPreparing)
	case "summarizing", "compact_summarizing":
		return i18n.Text(lang, i18n.KeyTUICompactProgressSummarizing)
	case "installing", "compact_installing":
		return i18n.Text(lang, i18n.KeyTUICompactProgressInstalling)
	case "persisting", "compact_persisting":
		return i18n.Text(lang, i18n.KeyTUICompactProgressPersisting)
	case "failed", "compact_failed", "compact_failure", "auto_compact_failure":
		return i18n.Text(lang, i18n.KeyREPLTUICompactionFailed)
	case "cancelled", "canceled", "compact_cancelled":
		return i18n.Text(lang, i18n.KeyREPLTUICompactionCancelled)
	case "compact_end", "compact_success", "auto_compact_success":
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
	CompactWithEvents(context.Context, string, string, func(stream.Event)) (engine.CompactResult, error)
}

func runManualCompactionEventsInLanguage(ctx context.Context, eng engine.Engine, sessionID, customInstructions string, lang i18n.Language, onEvent func(stream.Event)) error {
	turnID := sessionID + ":manual-compact:" + uuid.NewString()
	base := stream.Event{
		TurnID: turnID, ActorID: "assistant", ActorType: "runtime", WorkUnitID: "context-compaction",
	}
	var progressMu sync.Mutex
	progressRank := 0
	emitProgress := func(stage, status string, rank int, terminalErr error, details map[string]any) {
		if onEvent == nil {
			return
		}
		progressMu.Lock()
		if rank <= progressRank {
			progressMu.Unlock()
			return
		}
		progressRank = rank
		progressMu.Unlock()
		metadata := map[string]any{"trigger": "manual", "status": status}
		for key, value := range details {
			metadata[key] = value
		}
		if terminalErr != nil {
			metadata["error"] = terminalErr.Error()
		}
		event := base
		event.Type = stream.EventProgress
		event.Progress = &stream.ProgressEvent{Stage: stage, Message: status, Metadata: metadata}
		onEvent(event)
	}
	emitProgress("compact_accepted", "accepted", 1, nil, nil)
	emitProgress("compact_preparing", "preparing", 2, nil, nil)
	before, err := eng.Sessions().Load(sessionID)
	if err != nil {
		emitProgress("compact_failed", "failed", 6, err, nil)
		return err
	}
	emitProgress("compact_summarizing", "summarizing", 3, nil, nil)
	var compactErr error
	var completedBoundary *stream.CompactBoundaryEvent
	if eventEngine, ok := eng.(manualCompactionEventEngine); ok {
		_, compactErr = eventEngine.CompactWithEvents(ctx, sessionID, customInstructions, func(event stream.Event) {
			switch event.Type {
			case stream.EventProgress:
				if event.Progress == nil {
					return
				}
				stage, status, rank, ok := canonicalManualCompactionProgress(event.Progress.Stage)
				if !ok || rank >= 6 {
					// The wrapper owns the authoritative terminal transition after
					// reloading the committed session. Publishing the engine's
					// buffered terminal here would complete the component too early.
					return
				}
				emitProgress(stage, status, rank, nil, event.Progress.Metadata)
			case stream.EventCompactBoundary:
				if event.Compact != nil {
					boundary := cloneCompactBoundaryEvent(*event.Compact)
					completedBoundary = &boundary
				}
			case stream.EventProviderUsage:
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
		_, compactErr = eng.Compact(ctx, sessionID, customInstructions)
	}
	if cancelErr := ctx.Err(); cancelErr != nil {
		// User cancellation owns the terminal projection. A provider can race
		// Esc with an invalid/empty response, but that diagnostic must not turn
		// the explicitly cancelled command into a failed command.
		compactErr = errors.Join(cancelErr, compactErr)
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
		emitProgress(stage, status, 6, compactErr, nil)
		return localizedErr
	}
	// Engines with structured lifecycle events publish these phases at their
	// real boundaries. The fallback keeps older Engine implementations
	// coherent without inventing a percentage; monotonic ranking deduplicates
	// phases already observed from an event-bearing engine.
	emitProgress("compact_installing", "installing", 4, nil, nil)
	emitProgress("compact_persisting", "persisting", 5, nil, nil)
	after, err := eng.Sessions().Load(sessionID)
	if err != nil {
		emitProgress("compact_failed", "failed", 6, err, nil)
		return err
	}
	if onEvent != nil {
		event := base
		event.Type = stream.EventCompactBoundary
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
	details := map[string]any{"before_messages": len(before), "after_messages": len(after), "measurement": "local_estimate"}
	if completedBoundary != nil {
		details["pre_compact_token_count"] = completedBoundary.PreCompactTokenCount
		retained := completedBoundary.TruePostCompactTokenCount
		if retained == 0 {
			retained = completedBoundary.PostCompactTokenCount
		}
		details["post_compact_token_count"] = retained
	}
	emitProgress("compact_end", "completed", 6, nil, details)
	return nil
}

func canonicalManualCompactionProgress(stage string) (canonical, status string, rank int, ok bool) {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "compact_start", "compact_accepted":
		return "compact_accepted", "accepted", 1, true
	case "compact_preparing":
		return "compact_preparing", "preparing", 2, true
	case "compact_summarizing":
		return "compact_summarizing", "summarizing", 3, true
	case "compact_installing":
		return "compact_installing", "installing", 4, true
	case "compact_persisting":
		return "compact_persisting", "persisting", 5, true
	case "compact_end", "compact_success", "auto_compact_success":
		return "compact_end", "completed", 6, true
	case "compact_failed", "compact_failure", "auto_compact_failure":
		return "compact_failed", "failed", 6, true
	case "compact_cancelled":
		return "compact_cancelled", "cancelled", 6, true
	default:
		return "", "", 0, false
	}
}

func manualCompactionBoundary(before, after []types.Message) stream.CompactBoundaryEvent {
	counter := compact.NewContextWindow(0)
	boundary := stream.CompactBoundaryEvent{
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
			boundary.PreservedSegment = &stream.PreservedSegmentMetadata{
				StartIndex: metadata.PreservedSegment.StartIndex,
				Count:      metadata.PreservedSegment.Count,
				Anchor:     metadata.PreservedSegment.Anchor,
				Direction:  metadata.PreservedSegment.Direction,
			}
		}
		if i+1 < len(after) && compact.IsCompactSummaryMessage(after[i+1]) {
			boundary.Summary = strings.TrimSpace(after[i+1].GetText())
		}
		break
	}
	return boundary
}

func cloneCompactBoundaryEvent(boundary stream.CompactBoundaryEvent) stream.CompactBoundaryEvent {
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
