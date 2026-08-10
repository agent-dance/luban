package loop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/permission"
	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

const (
	defaultMaxTurns              = 100
	escalatedMaxTokens           = 64000
	maxOutputTokensRecoveryLimit = 3
	toolInputRecoveryLimit       = 1
	invalidToolInputPreviewBytes = 4096
)

func queryTurnIdentity(snapshot QueryConfigSnapshot, queryID string, turnCount int) (turnID, actorID, workUnitID string) {
	turnID = fmt.Sprintf("%s:query-%s:turn-%d", snapshot.SessionID, queryID, turnCount)
	actorID = snapshot.AgentID
	if actorID == "" {
		actorID = "assistant"
	}
	workUnitID = snapshot.AgentID
	if workUnitID == "" {
		workUnitID = turnID
	}
	return turnID, actorID, workUnitID
}

// TeammateTask is the loop-local task shape needed for teammate lifecycle hooks.
type TeammateTask struct {
	ID          string
	Subject     string
	Description string
	Owner       string
	Status      string
}

// TeammateContext describes the current teammate and its visible task state.
type TeammateContext struct {
	TeammateName string
	TeamName     string
	Tasks        []TeammateTask
}

// TeammateContextProvider supplies teammate state without coupling loop to a
// particular swarm/task persistence backend.
type TeammateContextProvider interface {
	CurrentTeammateContext(ctx context.Context) (TeammateContext, bool, error)
}

// GoalRuntime supplies the current session goal through a live reference. The
// loop snapshots this interface, not a Goal value, so in-run updates stay visible.
type GoalRuntime interface {
	LoadGoal() (*goal.Goal, error)
	SaveGoal(goal.Goal) error
	UpdateGoal(goal.UpdateFunc) (goal.Goal, error)
}

// Config holds query loop configuration
type Config struct {
	MaxTurns            int
	DisableMaxTurns     bool
	System              string
	SystemBlocks        []prompt.SystemPromptBlock
	UserContext         prompt.UserContext
	SystemContext       prompt.SystemContext
	VisibleTools        registry.VisibleToolSnapshot
	ToolPromptConfig    prompt.Config
	GeneratedToolPrompt bool
	GoalRuntime         GoalRuntime
	GoalEvaluator       GoalEvaluator
	Model               string
	MaxTokens           int
	MaxContextTokens    int           // max context window size for compaction (0 = no compaction)
	MaxOutputTokens     int           // max output tokens per response; used for output reservation in compaction threshold
	TokenBudget         int           // target output tokens for token-budget continuation; 0 disables
	TaskBudget          int           // API-side task output budget; 0 disables
	HookRunner          *hooks.Runner // optional hook runner (nil = no hooks)
	PostSamplingRunner  PostSamplingRunner
	TurnSideEffects     TurnSideEffects
	BareMode            bool
	SessionID           string // runtime conversation identity
	CacheLineageID      string // stable PromptCacheKey inherited by forked sessions; defaults to SessionID
	SessionProjectDir   string // durable session namespace; remains stable while a worktree changes ProjectRoot
	ProjectRoot         string // immutable workspace identity; distinct from an execution CWD
	TranscriptPath      string // readable persisted transcript path for compact summaries
	// TranscriptPathResolver refreshes content-addressed transcript paths when
	// compaction starts; it prevents a long-lived loop from exposing a genesis
	// artifact after the session manifest advances.
	TranscriptPathResolver func() string
	AgentID                string // non-empty marks subagent runs and disables token-budget continuation
	AgentType              string // subagent type/name for SubagentStop hooks
	AgentTranscriptPath    string // readable persisted transcript path for SubagentStop hooks
	ReasoningEffort        string // "low", "medium", "high" for reasoning models
	ServiceTier            provider.ServiceTier
	PinnedModel            bool // reject provider-directed model fallback
	// StreamingToolExecution starts tool execution when a tool_use content block
	// closes. Default false preserves the previous message-stop path.
	StreamingToolExecution bool
	// PermissionHandler gates tool execution. nil = allow all.
	PermissionHandler permission.PermissionHandler
	// SkillManager provides access to discovered skills for listing injection.
	// If nil, no skill listing is injected into the conversation.
	// Aligns with TS getSkillListingAttachments() in src/utils/attachments.ts.
	SkillManager *skills.Manager
	// SkillProjectGeneration optionally pins this loop to a workspace authority
	// captured by its parent. Zero lets a top-level loop bind the Manager's
	// current generation at Run start. Child/background loops must inherit the
	// non-zero parent capability so a later retarget cannot silently rebind them.
	SkillProjectGeneration skills.ProjectSourceGeneration
	CWD                    string
	PlanState              compact.PlanStateProvider
	BackgroundTasks        compact.BackgroundTaskProvider
	MCPState               compact.MCPStateProvider
	AgentDefinitions       compact.AgentDefinitionProvider
	// Optional tool-post attachment pipeline integrations. Nil preserves the
	// baseline loop behavior.
	CommandQueue       CommandQueue
	MemoryPrefetcher   MemoryPrefetcher
	SkillPrefetcher    SkillPrefetcher
	ToolRefresher      ToolRefresher
	QueryScope         QueryScope
	TeammateContext    TeammateContextProvider
	PostCompactCleanup func(context.Context) error
	QuerySource        QuerySource
}

// QueryLoop implements the agentic tool-use loop
type QueryLoop struct {
	provider        provider.Provider
	registry        *registry.Registry
	config          Config
	messages        []types.Message
	loadedToolNames map[string]struct{}
	seenToolUseIDs  map[string]struct{}
	lastResponseID  string // captured from EventMessageStop.ResponseID for Responses API chaining
	// Safety: read/written only in Run()'s goroutine (processStream is synchronous)
	disableResponseChain       bool                   // set true after previous_response_id fallback; stops retrying chain for this session
	lastEnvelopeFingerprint    string                 // fingerprint of non-input request fields; previous_response_id reused only when this matches (aligned with Codex CLI get_incremental_items check)
	currentEnvelopeFingerprint string                 // request fingerprint for the in-flight stream
	ctxWindow                  *compact.ContextWindow // nil if compaction disabled
	compactor                  compact.Compactor
	toolBudget                 *compact.ToolResultBudget
	microcompactCfg            compact.MicrocompactConfig
	cachedMicrocompactState    *compact.CachedMicrocompactState
	resultStore                *compact.ResultStore
	contentReplacementState    *compact.ContentReplacementState
	internalControlScope       messagecontrol.Scope
	calibratedCounter          *compact.CalibratedCounter // nil if compaction disabled
	thinkingConfig             *provider.ThinkingConfig   // nil = thinking disabled
	cacheBreakDetector         *CacheBreakDetector        // monitors prompt cache breaks
	compactStatus              string

	// skillCatalogMu protects the context-epoch-bound catalog cursor and loaded
	// body ledger. SkillTool resolvers may read this state from concurrent tool
	// executions; only the query loop commits receipts after visible append.
	skillCatalogMu     sync.RWMutex
	skillCatalogEpoch  uint64
	skillCatalogCursor SkillCatalogCursor
	loadedSkillDigests map[skills.SkillID]SkillLoadedLedgerEntry
	// readEvidenceOwnerID is a private per-QueryLoop capability namespace.
	// Together with skillCatalogEpoch and actor identity it prevents a shared
	// registry from leaking Read evidence across sessions, agents, or compacted
	// context generations.
	readEvidenceOwnerID string
	continuationLineage string
	continuationEpoch   uint64
	continuationSentAt  uint64
	executionOwner      *executioncontract.Owner
	// skillRunGeneration is the project authority pinned for the active Run.
	// It is a short capability value, not a held Manager lock, so retarget
	// writers never deadlock behind provider or tool execution.
	skillRunGenerationMu sync.RWMutex
	skillRunGeneration   skills.ProjectSourceGeneration

	workspaceRuntimeMu      sync.Mutex
	pendingWorkspaceRuntime *WorkspaceRuntimeUpdate
	// mcpInstructionAnnouncements tracks connected MCP servers whose
	// instructions have already been announced through delta attachments.
	mcpInstructionAnnouncements map[string]struct{}
}

// WorkspaceRuntimeUpdate is queued by the engine when an existing
// conversation enters or exits a worktree. The active run keeps its immutable
// config snapshot; the next Run applies this update before taking its snapshot.
// SessionProjectDir remains unchanged because worktrees do not move the
// durable conversation namespace.
type WorkspaceRuntimeUpdate struct {
	System              string
	SystemBlocks        []prompt.SystemPromptBlock
	UserContext         prompt.UserContext
	VisibleTools        registry.VisibleToolSnapshot
	ToolPromptConfig    prompt.Config
	GeneratedToolPrompt bool
	HookRunner          *hooks.Runner
	ProjectRoot         string
	CWD                 string
}

// QueueWorkspaceRuntime schedules an infallible next-run runtime rebind.
func (q *QueryLoop) QueueWorkspaceRuntime(update WorkspaceRuntimeUpdate) {
	if q == nil {
		return
	}
	clone := update
	clone.SystemBlocks = append([]prompt.SystemPromptBlock(nil), update.SystemBlocks...)
	q.workspaceRuntimeMu.Lock()
	q.pendingWorkspaceRuntime = &clone
	q.workspaceRuntimeMu.Unlock()
}

func (q *QueryLoop) hasPendingWorkspaceRuntime() bool {
	if q == nil {
		return false
	}
	q.workspaceRuntimeMu.Lock()
	defer q.workspaceRuntimeMu.Unlock()
	return q.pendingWorkspaceRuntime != nil
}

func (q *QueryLoop) applyPendingWorkspaceRuntime() {
	if q == nil {
		return
	}
	q.workspaceRuntimeMu.Lock()
	pending := q.pendingWorkspaceRuntime
	q.pendingWorkspaceRuntime = nil
	q.workspaceRuntimeMu.Unlock()
	if pending == nil {
		return
	}
	q.config.System = pending.System
	q.config.SystemBlocks = append([]prompt.SystemPromptBlock(nil), pending.SystemBlocks...)
	q.config.UserContext = pending.UserContext
	q.config.VisibleTools = pending.VisibleTools
	q.config.ToolPromptConfig = pending.ToolPromptConfig
	q.config.GeneratedToolPrompt = pending.GeneratedToolPrompt
	q.config.HookRunner = pending.HookRunner
	q.config.ProjectRoot = pending.ProjectRoot
	q.config.CWD = pending.CWD
}

// SetResultStore sets the result store for persisting oversized tool results.
func (q *QueryLoop) SetResultStore(rs *compact.ResultStore) {
	q.resultStore = rs
}

// HookRunner returns the hook runner configured for this loop.
func (q *QueryLoop) HookRunner() *hooks.Runner {
	return q.config.HookRunner
}

func (q *QueryLoop) postSamplingRunner(snapshot QueryConfigSnapshot, onEvent func(streamevent.Event)) PostSamplingRunner {
	if snapshot.PostSamplingRunner != nil {
		return snapshot.PostSamplingRunner
	}
	if snapshot.HookRunner == nil || (!snapshot.HookRunner.HasHooks(hooks.HookPostSampling) && !snapshot.HookRunner.HasHooks(hooks.HookStopFailure)) {
		return nil
	}
	return newHookPostSamplingRunner(snapshot.HookRunner, onEvent)
}

func (q *QueryLoop) runStopFailure(ctx context.Context, snapshot QueryConfigSnapshot, onEvent func(streamevent.Event), queryID string, turnCount int, message types.Message) {
	runner := q.postSamplingRunner(snapshot, onEvent)
	if runner == nil {
		return
	}
	turnID, actorID, workUnitID := queryTurnIdentity(snapshot, queryID, turnCount)
	runner.RunStopFailure(ctx, message, StopFailureOptions{
		SessionID:  snapshot.SessionID,
		TurnID:     turnID,
		AgentID:    actorID,
		AgentType:  snapshot.AgentType,
		WorkUnitID: workUnitID,
	})
}

func (q *QueryLoop) startTurnSideEffects(ctx context.Context, snapshot QueryConfigSnapshot, messages []types.Message) {
	if snapshot.TurnSideEffects == nil || snapshot.BareMode || isSimpleMode() {
		return
	}
	snapshot.TurnSideEffects.StartTurnSideEffects(ctx, append([]types.Message(nil), messages...), TurnSideEffectOptions{
		SessionID: snapshot.SessionID,
		AgentID:   snapshot.AgentID,
		AgentType: snapshot.AgentType,
	})
}

func isSimpleMode() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("LUBAN_CODE_SIMPLE")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// previousResponseIDForRequest returns the response ID to use for chaining,
// or empty string if chaining has been disabled (e.g. after a fallback due to
// account switching in codex-lb) or if the request envelope has changed enough
// that the prior response is no longer a safe parent for the next turn.
//
// Aligned with Codex CLI (codex-rs): previous_response_id is reusable as long as
// the non-input fields (model, system prompt, tools, reasoning config) stay the
// same. The message history is explicitly NOT part of the fingerprint — it is
// expected to grow every turn, and the Responses API's incremental input mechanism
// handles this correctly. Including messages in the fingerprint would cause the
// fingerprint to change every turn and effectively disable chaining.
func (q *QueryLoop) previousResponseIDForRequest(envelopeFingerprint string) string {
	if q.disableResponseChain {
		return ""
	}
	if q.lastResponseID == "" {
		return ""
	}
	// Envelope fingerprint covers model/system/tools/reasoning — NOT messages.
	// If these haven't changed since the last successful response, chaining is safe.
	if q.lastEnvelopeFingerprint != "" && envelopeFingerprint != "" && q.lastEnvelopeFingerprint != envelopeFingerprint {
		return ""
	}
	return q.lastResponseID
}

// envelopeFingerprint computes a fingerprint of the *non-input* request fields.
// Aligned with Codex CLI's get_incremental_items() check: "compare non-input
// fields must be exactly identical". This intentionally excludes messages/input
// because those grow every turn.
func envelopeFingerprint(params provider.Params) string {
	payload := struct {
		Model                   string
		MaxTokens               int
		MaxOutputTokensOverride int
		SystemBlocks            []prompt.SystemPromptBlock
		Tools                   []types.ToolDefinition
		ExtraToolSchemas        []types.ServerToolDefinition
		ToolChoice              *provider.ToolChoice
		Conversation            string
		Truncation              string
		PromptCacheKey          string
		UsePromptCache          bool
		ReasoningEffort         string
		ServiceTier             provider.ServiceTier
		TaskBudgetTotal         int
		TaskRemaining           *int
		ThinkingEnabled         bool
		ThinkingBudget          int
	}{
		Model:                   params.Model,
		MaxTokens:               params.MaxTokens,
		MaxOutputTokensOverride: params.MaxOutputTokensOverride,
		SystemBlocks:            params.SystemTextBlocks(),
		Tools:                   params.Tools,
		ExtraToolSchemas:        params.ExtraToolSchemas,
		ToolChoice:              params.ToolChoice,
		Conversation:            params.Conversation,
		Truncation:              params.Truncation,
		PromptCacheKey:          params.PromptCacheKey,
		UsePromptCache:          params.UsePromptCache,
		ReasoningEffort:         params.ReasoningEffort,
		ServiceTier:             params.ServiceTier,
	}
	if params.TaskBudget != nil {
		payload.TaskBudgetTotal = params.TaskBudget.Total
		payload.TaskRemaining = params.TaskBudget.Remaining
	}
	if params.Thinking != nil {
		payload.ThinkingEnabled = params.Thinking.Enabled
		payload.ThinkingBudget = params.Thinking.BudgetTokens
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", digest)
}

// ForceCompact runs context compaction on the current message history immediately.
// Returns an error if no compactor is configured (MaxContextTokens == 0).
func (q *QueryLoop) ForceCompact(ctx context.Context) (CompactResult, error) {
	return q.ForceCompactWithInstructions(ctx, "")
}

// ForceCompactWithInstructions runs manual compaction with optional user
// summary instructions from "/compact <args>".
func (q *QueryLoop) ForceCompactWithInstructions(ctx context.Context, customInstructions string) (CompactResult, error) {
	return q.forceCompactWithInstructions(ctx, customInstructions, nil)
}

// ForceCompactWithInstructionsAndEvents is the event-bearing manual
// compaction surface used by interactive clients to account the compaction
// provider request in session usage and cost.
func (q *QueryLoop) ForceCompactWithInstructionsAndEvents(ctx context.Context, customInstructions string, onEvent func(streamevent.Event)) (CompactResult, error) {
	return q.forceCompactWithInstructions(ctx, customInstructions, onEvent)
}

type manualCompactLifecycleBuffer struct {
	mu     sync.Mutex
	events []streamevent.Event
}

func (b *manualCompactLifecycleBuffer) record(event streamevent.Event) {
	b.mu.Lock()
	b.events = append(b.events, event)
	b.mu.Unlock()
}

func (b *manualCompactLifecycleBuffer) snapshot() []streamevent.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]streamevent.Event(nil), b.events...)
}

func (b *manualCompactLifecycleBuffer) publish(onEvent func(streamevent.Event)) {
	if onEvent == nil {
		return
	}
	for _, event := range b.snapshot() {
		onEvent(event)
	}
}

func (b *manualCompactLifecycleBuffer) publishCleanupFailure(q *QueryLoop, onEvent func(streamevent.Event), cleanupErr error) {
	if onEvent == nil {
		return
	}
	turnCount := 0
	for _, event := range b.snapshot() {
		if event.Type == streamevent.EventCompactBoundary {
			continue
		}
		if event.Type == streamevent.EventProgress && event.Progress != nil {
			switch event.Progress.Stage {
			case "compact_end", "compact_failed", "compact_cancelled", "compact_success", "auto_compact_success":
				if event.Progress.Stage == "compact_end" {
					turnCount = event.TurnCount
				}
				continue
			}
		}
		if event.Type == streamevent.EventProviderUsage && event.Metadata["kind"] == "compaction" {
			metadata := make(map[string]any, len(event.Metadata))
			for key, value := range event.Metadata {
				metadata[key] = value
			}
			metadata["status"] = "failure"
			event.Metadata = metadata
		}
		onEvent(event)
	}
	q.emitCompactProgress(onEvent, turnCount, "compact_failed", "failed", "manual", "", cleanupErr)
}

type manualCompactCacheBreakPreimage struct {
	prevCacheRead   int
	prevCacheCreate int
	prevTime        time.Time
	callCount       int
	hasBaseline     bool
}

type manualCompactInstallPreimage struct {
	visible                    PreparedVisibleState
	lastResponseID             string
	disableResponseChain       bool
	lastEnvelopeFingerprint    string
	currentEnvelopeFingerprint string
	contentReplacementState    *compact.ContentReplacementState
	cachedMicrocompactPointer  *compact.CachedMicrocompactState
	cachedMicrocompactState    *compact.CachedMicrocompactState
	microcompactCfg            compact.MicrocompactConfig
	toolBudget                 *compact.ToolResultBudget
	cacheBreakDetector         *CacheBreakDetector
	cacheBreakState            manualCompactCacheBreakPreimage
}

func (q *QueryLoop) captureManualCompactInstallPreimage() (manualCompactInstallPreimage, error) {
	visible, err := q.CapturePreparedVisibleState()
	if err != nil {
		return manualCompactInstallPreimage{}, err
	}
	preimage := manualCompactInstallPreimage{
		visible:                    visible,
		lastResponseID:             q.lastResponseID,
		disableResponseChain:       q.disableResponseChain,
		lastEnvelopeFingerprint:    q.lastEnvelopeFingerprint,
		currentEnvelopeFingerprint: q.currentEnvelopeFingerprint,
		contentReplacementState:    q.contentReplacementState,
		cachedMicrocompactPointer:  q.cachedMicrocompactState,
		cachedMicrocompactState:    cloneManualCompactCachedState(q.cachedMicrocompactState),
		microcompactCfg:            q.microcompactCfg,
		toolBudget:                 q.toolBudget,
		cacheBreakDetector:         q.cacheBreakDetector,
	}
	if q.cacheBreakDetector != nil {
		q.cacheBreakDetector.mu.Lock()
		preimage.cacheBreakState = manualCompactCacheBreakPreimage{
			prevCacheRead:   q.cacheBreakDetector.prevCacheRead,
			prevCacheCreate: q.cacheBreakDetector.prevCacheCreate,
			prevTime:        q.cacheBreakDetector.prevTime,
			callCount:       q.cacheBreakDetector.callCount,
			hasBaseline:     q.cacheBreakDetector.hasBaseline,
		}
		q.cacheBreakDetector.mu.Unlock()
	}
	return preimage, nil
}

func (q *QueryLoop) restoreManualCompactInstallPreimage(preimage manualCompactInstallPreimage) {
	q.messages = executioncontract.CloneMessages(preimage.visible.messages)
	q.loadedToolNames = make(map[string]struct{}, len(preimage.visible.loadedToolNames))
	for _, name := range preimage.visible.loadedToolNames {
		q.loadedToolNames[name] = struct{}{}
	}
	q.seenToolUseIDs = make(map[string]struct{}, len(preimage.visible.seenToolUseIDs))
	for _, id := range preimage.visible.seenToolUseIDs {
		q.seenToolUseIDs[id] = struct{}{}
	}
	skillState := preimage.visible.skillState.Clone()
	q.skillCatalogMu.Lock()
	q.skillCatalogEpoch = skillState.ContextEpoch
	q.skillCatalogCursor = skillState.Cursor
	q.loadedSkillDigests = skillState.LoadedDigests
	if q.loadedSkillDigests == nil {
		q.loadedSkillDigests = make(map[skills.SkillID]SkillLoadedLedgerEntry)
	}
	q.skillCatalogMu.Unlock()
	q.internalControlScope = preimage.visible.controlScope
	q.lastResponseID = preimage.lastResponseID
	q.disableResponseChain = preimage.disableResponseChain
	q.lastEnvelopeFingerprint = preimage.lastEnvelopeFingerprint
	q.currentEnvelopeFingerprint = preimage.currentEnvelopeFingerprint
	q.contentReplacementState = preimage.contentReplacementState
	q.cachedMicrocompactState = preimage.cachedMicrocompactPointer
	if q.cachedMicrocompactState != nil {
		*q.cachedMicrocompactState = *cloneManualCompactCachedState(preimage.cachedMicrocompactState)
	}
	q.microcompactCfg = preimage.microcompactCfg
	q.toolBudget = preimage.toolBudget
	q.cacheBreakDetector = preimage.cacheBreakDetector
	if q.cacheBreakDetector != nil {
		q.cacheBreakDetector.mu.Lock()
		q.cacheBreakDetector.prevCacheRead = preimage.cacheBreakState.prevCacheRead
		q.cacheBreakDetector.prevCacheCreate = preimage.cacheBreakState.prevCacheCreate
		q.cacheBreakDetector.prevTime = preimage.cacheBreakState.prevTime
		q.cacheBreakDetector.callCount = preimage.cacheBreakState.callCount
		q.cacheBreakDetector.hasBaseline = preimage.cacheBreakState.hasBaseline
		q.cacheBreakDetector.mu.Unlock()
	}
}

func cloneManualCompactCachedState(state *compact.CachedMicrocompactState) *compact.CachedMicrocompactState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.RegisteredTools = make(map[string]struct{}, len(state.RegisteredTools))
	for name := range state.RegisteredTools {
		cloned.RegisteredTools[name] = struct{}{}
	}
	cloned.DeletedRefs = make(map[string]struct{}, len(state.DeletedRefs))
	for id := range state.DeletedRefs {
		cloned.DeletedRefs[id] = struct{}{}
	}
	cloned.ToolOrder = append([]string(nil), state.ToolOrder...)
	cloned.ToolGroups = make([][]string, len(state.ToolGroups))
	for index := range state.ToolGroups {
		cloned.ToolGroups[index] = append([]string(nil), state.ToolGroups[index]...)
	}
	cloned.PinnedEdits = append([]compact.PinnedCacheEdits(nil), state.PinnedEdits...)
	for index := range cloned.PinnedEdits {
		cloned.PinnedEdits[index].Block.Edits = append([]compact.CacheEdit(nil), state.PinnedEdits[index].Block.Edits...)
	}
	return &cloned
}

func (q *QueryLoop) forceCompactWithInstructions(ctx context.Context, customInstructions string, onEvent func(streamevent.Event)) (outcome CompactResult, err error) {
	outcome.BeforeMessageCount = len(q.messages)
	outcome.AfterMessageCount = outcome.BeforeMessageCount
	defer func() {
		outcome.AfterMessageCount = len(q.messages)
		if err != nil {
			outcome.Compacted = false
		}
	}()
	if q.compactor == nil {
		return outcome, i18n.NewError(i18n.KeyLoopQueryCompactionNotConfigured)
	}
	if err := q.validateInternalControlScope(); err != nil {
		return outcome, i18n.WrapInternalError(i18n.KeyLoopQueryControlScopeInvalid, err)
	}
	preimage, err := q.captureManualCompactInstallPreimage()
	if err != nil {
		return outcome, i18n.WrapInternalError(i18n.KeyLoopQuerySnapshotSkillCatalogFailed, err)
	}
	customInstructions = strings.TrimSpace(customInstructions)
	ctx = provider.WithDebugCall(ctx, provider.DebugCallCompaction, map[string]any{
		"trigger":       "manual",
		"message_count": len(q.messages),
	})
	var lifecycle manualCompactLifecycleBuffer
	eventSink := onEvent
	if onEvent != nil {
		eventSink = lifecycle.record
	}
	compactionInput := compact.MicrocompactWithResult(executioncontract.CloneMessages(q.messages), q.microcompactCfg).Messages
	result, semanticNoop, err := q.runCompactionAgainst(ctx, "manual", 0, eventSink, compactionInput, func() (*compact.CompactionResult, error) {
		if sc, ok := q.compactor.(*compact.SummaryCompactor); ok {
			previous := sc.CustomInstructions
			sc.CustomInstructions = customInstructions
			defer func() {
				sc.CustomInstructions = previous
			}()
		}
		return q.compactor.Compact(ctx, executioncontract.CloneMessages(compactionInput), 0)
	})
	if err != nil {
		lifecycle.publish(onEvent)
		return outcome, i18n.WrapError(i18n.KeyLoopQueryForcedCompactionFailed, err)
	}
	if result == nil && !semanticNoop {
		lifecycle.publish(onEvent)
		return outcome, nil
	}
	if result != nil {
		q.messages = q.installVisibleHistory(compact.BuildPostCompactMessages(result))
	} else if q.config.SkillManager != nil {
		// Reconcile only the runtime-owned projection. The deep-equal compactor
		// output itself is not installed and cannot borrow a microcompact delta,
		// while an authoritative catalog revoke can still advance its epoch.
		reconciled, reconcileErr := q.installPostCompactVisibleHistory(q.messages, q.messages)
		if reconcileErr != nil {
			q.restoreManualCompactInstallPreimage(preimage)
			reconcileErr = i18n.WrapInternalError(i18n.KeyRuntimeCompactionCommitFailed, reconcileErr)
			lifecycle.publishCleanupFailure(q, onEvent, reconcileErr)
			return outcome, reconcileErr
		}
		q.messages = reconciled
	}
	// A non-nil deep-equal result is a semantic no-op and must not install the
	// compactor output. Manual compact still runs authoritative cleanup against
	// the unchanged live history so catalog/ledger revocations are reconciled.
	if cleanupErr := q.RunPostCompactCleanup(ctx, q.messages); cleanupErr != nil {
		q.restoreManualCompactInstallPreimage(preimage)
		cleanupErr = i18n.WrapInternalError(i18n.KeyRuntimePostCompactCleanupFailed, cleanupErr)
		lifecycle.publishCleanupFailure(q, onEvent, cleanupErr)
		return outcome, cleanupErr
	}
	if q.ctxWindow != nil {
		q.ctxWindow.RecordCompactSuccess()
	}
	if result != nil {
		q.updatePostCompactContext(result)
		outcome.Compacted = true
	}
	lifecycle.publish(onEvent)
	return outcome, nil
}

// New creates a new QueryLoop with a Provider
func New(p provider.Provider, reg *registry.Registry, cfg Config) *QueryLoop {
	if strings.TrimSpace(cfg.Model) == "" && p != nil {
		cfg.Model = p.ModelID()
	}
	if cfg.DisableMaxTurns {
		cfg.MaxTurns = 0
	} else if cfg.MaxTurns == 0 {
		cfg.MaxTurns = defaultMaxTurns
	}
	ql := &QueryLoop{
		provider:                p,
		registry:                reg,
		config:                  cfg,
		loadedToolNames:         make(map[string]struct{}),
		seenToolUseIDs:          make(map[string]struct{}),
		toolBudget:              compact.NewToolResultBudget(),
		contentReplacementState: compact.NewContentReplacementState(),
		internalControlScope:    messagecontrol.NewLoopScope(messagecontrol.Runtime()),
		skillCatalogEpoch:       1,
		loadedSkillDigests:      make(map[skills.SkillID]SkillLoadedLedgerEntry),
		readEvidenceOwnerID:     uuid.NewString(),
		continuationLineage:     uuid.NewString(),
		continuationEpoch:       1,
		executionOwner:          executioncontract.NewOwner(),
		microcompactCfg:         compact.DefaultMicrocompactConfig(),
		cachedMicrocompactState: compact.NewCachedMicrocompactState(),
		calibratedCounter:       compact.NewCalibratedCounter(4.0),
		cacheBreakDetector:      &CacheBreakDetector{},
	}
	if cfg.AgentID != "" || cfg.QueryScope.IsSubagent {
		ql.microcompactCfg.QuerySource = compact.MicrocompactSourceNonMain
	} else {
		ql.microcompactCfg.QuerySource = compact.MicrocompactSourceMain
	}
	// Adapt MaxContextTokens to the current provider's actual limit upfront,
	// so ContextWindow is created with the correct size from the start.
	ql.adaptContextWindow()

	// Enable compaction if MaxContextTokens is set
	if ql.config.MaxContextTokens > 0 {
		cw := compact.NewContextWindow(ql.config.MaxContextTokens)
		cw.MaxOutputTokens = cfg.MaxOutputTokens
		ql.ctxWindow = cw
		ql.compactor = &compact.SummaryCompactor{
			SummarizeMessages:      compact.NewLLMStructuredSummarizeFuncWithServiceTier(p, cfg.ServiceTier),
			KeepRecent:             20,
			TranscriptPath:         cfg.TranscriptPath,
			TranscriptPathResolver: cfg.TranscriptPathResolver,
			AttachmentProvider:     ql.postCompactAttachmentProvider(),
			SessionID:              cfg.SessionID,
			CWD:                    cfg.CWD,
			HookRunner:             cfg.HookRunner,
		}
	}
	return ql
}

// CompactStatus reports the current compaction lifecycle status.
func (q *QueryLoop) CompactStatus() string {
	return q.compactStatus
}

func (q *QueryLoop) visibleToolDefinitions() []types.ToolDefinition {
	if q.registry == nil {
		return nil
	}
	return q.registry.VisibleDefinitions(q.loadedToolNames)
}

func (q *QueryLoop) learnLoadedTools(results []types.ToolResultBlock) {
	for _, result := range results {
		for _, block := range result.ContentBlocks {
			ref, ok := block.(types.ToolReferenceBlock)
			if !ok || ref.ToolName == "" {
				continue
			}
			q.loadedToolNames[ref.ToolName] = struct{}{}
		}
	}
}

func loadedToolNamesFromMessages(messages []types.Message) map[string]struct{} {
	loaded := make(map[string]struct{})
	add := func(blocks []types.ContentBlock) {
		for _, block := range blocks {
			if ref, ok := block.(types.ToolReferenceBlock); ok {
				if name := strings.TrimSpace(ref.ToolName); name != "" {
					loaded[name] = struct{}{}
				}
			}
		}
	}
	for _, message := range messages {
		for _, block := range message.Content {
			switch typed := block.(type) {
			case types.ToolReferenceBlock:
				add([]types.ContentBlock{typed})
			case types.ToolResultBlock:
				add(typed.ContentBlocks)
			}
		}
	}
	return loaded
}

// Messages returns the current conversation messages (defensive copy)
func (q *QueryLoop) Messages() []types.Message {
	out := make([]types.Message, len(q.messages))
	copy(out, q.messages)
	return out
}

const maxMessagesHardLimit = 500

// enforceMessageHistoryLimit fails closed before an over-limit history can be
// sampled. It deliberately does not rewrite msgs: semantic compaction is the
// only production path allowed to remove ordinary conversation messages.
func enforceMessageHistoryLimit(msgs []types.Message) error {
	if len(msgs) <= maxMessagesHardLimit {
		return nil
	}
	return &MessageHistoryLimitError{
		MessageCount: len(msgs),
		Limit:        maxMessagesHardLimit,
	}
}

// SetMessages replaces the model-visible conversation and resets the identity
// ledger to identities present in msgs. It is the session-transition API: use
// SetMessagesPreservingToolUseLedger for same-session message mutations.
func (q *QueryLoop) SetMessages(msgs []types.Message) {
	q.SetMessagesWithRuntimeLedgers(msgs, nil, nil)
}

// SetMessagesPreservingToolUseLedger replaces the model-visible conversation
// without forgetting identities that left the transcript through compaction.
// Same-session commands that append or rewrite messages must use this method.
func (q *QueryLoop) SetMessagesPreservingToolUseLedger(msgs []types.Message) {
	loadedToolNames := q.LoadedToolNames()
	q.SetMessagesWithRuntimeLedgers(msgs, q.SeenToolUseIDs(), nil)
	for _, name := range loadedToolNames {
		q.loadedToolNames[name] = struct{}{}
	}
}

// SetMessagesWithRuntimeLedgers restores session-lifetime identities and
// deferred tool visibility alongside a replacement model-visible history.
// Visible tool_reference blocks are always authoritative; persisted names
// only preserve schemas that survived compaction outside the visible tail.
func (q *QueryLoop) SetMessagesWithRuntimeLedgers(msgs []types.Message, persistedIDs, persistedLoadedToolNames []string) {
	q.messages = q.installVisibleHistory(msgs)
	q.loadedToolNames = loadedToolNamesFromMessages(msgs)
	for _, name := range persistedLoadedToolNames {
		if name = strings.TrimSpace(name); name != "" {
			q.loadedToolNames[name] = struct{}{}
		}
	}
	q.seenToolUseIDs = collectToolUseIDs(msgs)
	for _, id := range persistedIDs {
		if strings.TrimSpace(id) != "" {
			q.seenToolUseIDs[id] = struct{}{}
		}
	}
}

// PreparedVisibleState is an opaque, prevalidated replacement for the
// session-visible QueryLoop state. Runtime composition captures it from a
// detached loop before touching a live conversation, then installs it under
// that conversation's publication lock with no remaining failure path.
type PreparedVisibleState struct {
	messages        []types.Message
	seenToolUseIDs  []string
	loadedToolNames []string
	skillState      SkillCatalogRuntimeState
	controlScope    messagecontrol.Scope
	valid           bool
}

// CapturePreparedVisibleState snapshots a fully reconciled detached loop.
func (q *QueryLoop) CapturePreparedVisibleState() (PreparedVisibleState, error) {
	if q == nil {
		return PreparedVisibleState{}, errors.New("nil query loop")
	}
	state := q.SkillCatalogState()
	if err := state.Validate(); err != nil {
		return PreparedVisibleState{}, err
	}
	return PreparedVisibleState{
		messages:        q.Messages(),
		seenToolUseIDs:  q.SeenToolUseIDs(),
		loadedToolNames: q.LoadedToolNames(),
		skillState:      state.Clone(),
		controlScope:    q.internalControlScope,
		valid:           true,
	}, nil
}

// InstallPreparedVisibleState performs only clone and assignment operations.
// The opaque value can be constructed only by CapturePreparedVisibleState, so
// validation never occurs after a surrounding override transaction commits.
func (q *QueryLoop) InstallPreparedVisibleState(prepared PreparedVisibleState) {
	if q == nil || !prepared.valid {
		return
	}
	q.messages = q.installVisibleHistory(prepared.messages)
	q.loadedToolNames = loadedToolNamesFromMessages(prepared.messages)
	for _, name := range prepared.loadedToolNames {
		if name = strings.TrimSpace(name); name != "" {
			q.loadedToolNames[name] = struct{}{}
		}
	}
	q.seenToolUseIDs = collectToolUseIDs(prepared.messages)
	for _, id := range prepared.seenToolUseIDs {
		if strings.TrimSpace(id) != "" {
			q.seenToolUseIDs[id] = struct{}{}
		}
	}
	state := prepared.skillState.Clone()
	q.skillCatalogMu.Lock()
	q.skillCatalogEpoch = state.ContextEpoch
	q.skillCatalogCursor = state.Cursor
	q.loadedSkillDigests = state.LoadedDigests
	if q.loadedSkillDigests == nil {
		q.loadedSkillDigests = make(map[skills.SkillID]SkillLoadedLedgerEntry)
	}
	q.skillCatalogMu.Unlock()
	q.internalControlScope = prepared.controlScope
}

// CaptureCompactionContextState snapshots context usage for the outer engine
// persistence transaction. It stays separate from visible state so detached
// state installs cannot overwrite a live provider measurement.
func (q *QueryLoop) CaptureCompactionContextState() compact.CompactionTrackerSnapshot {
	if q == nil || q.ctxWindow == nil {
		return compact.CompactionTrackerSnapshot{}
	}
	return q.ctxWindow.CaptureCompactionTracker()
}

func (q *QueryLoop) RestoreCompactionContextState(snapshot compact.CompactionTrackerSnapshot) {
	if q != nil && q.ctxWindow != nil {
		q.ctxWindow.RestoreCompactionTracker(snapshot)
	}
}

// SeenToolUseIDs returns the complete session-lifetime identity ledger in a
// stable order suitable for deterministic sidecar persistence.
func (q *QueryLoop) SeenToolUseIDs() []string {
	ids := make([]string, 0, len(q.seenToolUseIDs))
	for id := range q.seenToolUseIDs {
		if strings.TrimSpace(id) != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// LoadedToolNames returns deferred tools already surfaced to the model in a
// stable order suitable for session persistence and fork restoration.
func (q *QueryLoop) LoadedToolNames() []string {
	names := make([]string, 0, len(q.loadedToolNames))
	for name := range q.loadedToolNames {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// SkillCatalogState returns an atomic, defensive view of the catalog cursor
// and loaded-body ledger for runtime persistence and SkillTool wiring.
func (q *QueryLoop) SkillCatalogState() SkillCatalogRuntimeState {
	q.skillCatalogMu.Lock()
	defer q.skillCatalogMu.Unlock()
	q.ensureSkillCatalogEpochLocked()
	return q.skillCatalogStateLocked()
}

// SetSkillCatalogState restores state that runtime composition has already
// reconciled against visible-history evidence for the same context epoch.
// Invalid or cross-epoch cursors are rejected before any live state changes.
func (q *QueryLoop) SetSkillCatalogState(state SkillCatalogRuntimeState) error {
	if err := state.Validate(); err != nil {
		return err
	}
	state = state.Clone()
	q.skillCatalogMu.Lock()
	q.skillCatalogEpoch = state.ContextEpoch
	q.skillCatalogCursor = state.Cursor
	q.loadedSkillDigests = state.LoadedDigests
	if q.loadedSkillDigests == nil {
		q.loadedSkillDigests = make(map[skills.SkillID]SkillLoadedLedgerEntry)
	}
	q.skillCatalogMu.Unlock()
	return nil
}

// SkillLoadedLedgerState returns the current epoch even when the requested
// body has not been loaded. SkillTool uses that non-zero epoch to emit a
// pending receipt, then this loop commits the receipt only after visible append.
func (q *QueryLoop) SkillLoadedLedgerState(id skills.SkillID) SkillLoadedLedgerState {
	q.skillCatalogMu.Lock()
	defer q.skillCatalogMu.Unlock()
	q.ensureSkillCatalogEpochLocked()
	state := SkillLoadedLedgerState{ContextEpoch: q.skillCatalogEpoch}
	if entry, ok := q.loadedSkillDigests[id]; ok {
		state.LoadedContextEpoch = q.skillCatalogEpoch
		state.ContentDigest = entry.ContentDigest
		state.PayloadDigest = entry.PayloadDigest
	}
	return state
}

func (q *QueryLoop) ensureSkillCatalogEpochLocked() {
	if q.skillCatalogEpoch == 0 {
		q.skillCatalogEpoch = 1
	}
	if q.loadedSkillDigests == nil {
		q.loadedSkillDigests = make(map[skills.SkillID]SkillLoadedLedgerEntry)
	}
}

func (q *QueryLoop) currentReadEvidenceEpoch() uint64 {
	q.skillCatalogMu.RLock()
	defer q.skillCatalogMu.RUnlock()
	if q.skillCatalogEpoch == 0 {
		return 1
	}
	return q.skillCatalogEpoch
}

func (q *QueryLoop) skillCatalogStateLocked() SkillCatalogRuntimeState {
	state := SkillCatalogRuntimeState{
		ContextEpoch:  q.skillCatalogEpoch,
		Cursor:        q.skillCatalogCursor.Clone(),
		LoadedDigests: make(map[skills.SkillID]SkillLoadedLedgerEntry, len(q.loadedSkillDigests)),
	}
	for id, entry := range q.loadedSkillDigests {
		state.LoadedDigests[id] = entry
	}
	return state
}

// installVisibleHistory is the single epoch fence for wholesale model-visible
// history replacement. Callers assign its return value to q.messages or a
// QueryState. It intentionally preserves config.CacheLineageID and
// the session-lifetime tool-use ledger while clearing Responses chaining and
// all ledgers whose evidence belonged to the replaced context.
func (q *QueryLoop) installVisibleHistory(messages []types.Message) []types.Message {
	messages = repairInstalledMissingToolResults(messages)
	q.advanceContinuationEpoch()
	q.skillCatalogMu.Lock()
	q.ensureSkillCatalogEpochLocked()
	q.skillCatalogEpoch++
	if q.skillCatalogEpoch == 0 { // practically unreachable overflow; keep zero reserved
		q.skillCatalogEpoch = 1
	}
	q.skillCatalogCursor = SkillCatalogCursor{}
	q.loadedSkillDigests = make(map[skills.SkillID]SkillLoadedLedgerEntry)
	q.skillCatalogMu.Unlock()

	q.lastResponseID = ""
	q.lastEnvelopeFingerprint = ""
	q.currentEnvelopeFingerprint = ""
	q.disableResponseChain = false
	q.contentReplacementState = compact.ReconstructContentReplacementStateForScope(messages, q.internalControlScope)
	if q.cachedMicrocompactState != nil {
		q.cachedMicrocompactState.Reset()
	}
	return messages
}

func (q *QueryLoop) prepareSkillCatalogForSampling(messages []types.Message, snapshot QueryConfigSnapshot, insertBefore int) ([]types.Message, *types.Message, error) {
	if snapshot.SkillManager == nil {
		return messages, nil, nil
	}
	current, err := snapshot.SkillManager.SnapshotAtGeneration(snapshot.SessionID, snapshot.SkillProjectGeneration)
	if err != nil {
		// EnterWorktree/ExitWorktree may retarget the shared Manager during the
		// current assistant run. Keep the old run's catalog frozen instead of
		// leaking the new workspace or aborting before a compensating Exit can
		// run. Skill/Agent/Team execution remains generation-fenced; the queued
		// workspace and a fresh generation become visible only on the next Run.
		if errors.Is(err, skills.ErrSkillProjectGenerationChanged) && q.hasPendingWorkspaceRuntime() {
			return messages, nil, nil
		}
		return nil, nil, i18n.WrapInternalError(i18n.KeyLoopQuerySnapshotSkillCatalogFailed, err)
	}
	runtimeState := q.SkillCatalogState()
	plan, err := PlanSkillCatalog(SkillCatalogCoordinatorInput{
		CurrentSnapshot: current,
		PriorCursor:     runtimeState.Cursor,
		ContextEpoch:    skillCatalogContextEpoch(runtimeState.ContextEpoch),
		VisibleHistory:  messages,
		CharBudget:      skills.GetCharBudget(snapshot.MaxContextTokens),
	})
	if err != nil {
		return nil, nil, i18n.WrapInternalError(i18n.KeyLoopQueryPlanSkillCatalogFailed, err)
	}
	if plan.Message != nil {
		trusted := q.sealRuntimeControlMessage(*plan.Message)
		plan.Message = &trusted
		messages = insertMessageAt(messages, trusted, insertBefore)
	}

	q.skillCatalogMu.Lock()
	defer q.skillCatalogMu.Unlock()
	q.ensureSkillCatalogEpochLocked()
	if q.skillCatalogEpoch != runtimeState.ContextEpoch {
		return nil, nil, i18n.NewError(i18n.KeyLoopQuerySkillCatalogContextChanged)
	}
	q.skillCatalogCursor = plan.Cursor.Clone()
	return messages, plan.Message, nil
}

func insertMessageAt(messages []types.Message, message types.Message, index int) []types.Message {
	if index < 0 || index > len(messages) {
		return append(messages, message)
	}
	result := make([]types.Message, 0, len(messages)+1)
	result = append(result, messages[:index]...)
	result = append(result, message)
	result = append(result, messages[index:]...)
	return result
}

func trailingPlainUserInputIndex(messages []types.Message) int {
	if len(messages) == 0 {
		return -1
	}
	index := len(messages) - 1
	message := messages[index]
	if message.Role != types.RoleUser {
		return -1
	}
	for _, block := range message.Content {
		if _, isToolResult := block.(types.ToolResultBlock); isToolResult {
			return -1
		}
	}
	return index
}

func skillCatalogInsertionIndexForTransition(messages []types.Message, transition QueryTransition) int {
	switch transition {
	case QueryTransitionReactiveCompactRetry,
		QueryTransitionMaxOutputTokensRecovery,
		QueryTransitionToolInputRecovery,
		QueryTransitionStopHookBlocking,
		QueryTransitionGoalContinuation,
		QueryTransitionTokenBudgetContinuation,
		QueryTransitionPlanModeContextRestart:
		return trailingPlainUserInputIndex(messages)
	default:
		return -1
	}
}

func messageAt(messages []types.Message, index int) (types.Message, bool) {
	if index < 0 || index >= len(messages) {
		return types.Message{}, false
	}
	return messages[index], true
}

func messageIndexFromEnd(messages []types.Message, target types.Message) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if reflect.DeepEqual(messages[index], target) {
			return index
		}
	}
	return -1
}

func (q *QueryLoop) commitVisibleSkillExecutionReceipts(messages []types.Message, toolUses []types.ToolUseBlock, results []types.ToolResultBlock) error {
	skillToolUseIDs := make(map[string]struct{})
	for _, toolUse := range toolUses {
		if toolUse.Name == "Skill" && strings.TrimSpace(toolUse.ID) != "" {
			skillToolUseIDs[toolUse.ID] = struct{}{}
		}
	}
	if len(skillToolUseIDs) == 0 {
		return nil
	}

	for _, result := range results {
		if _, isSkill := skillToolUseIDs[result.ToolUseID]; !isSkill || result.IsError || result.Outcome != types.ToolOutcomeSucceeded {
			continue
		}
		receipt, found, err := skills.DecodeSkillExecutionReceiptMetadata(result.Metadata)
		if err != nil {
			return i18n.WrapInternalError(i18n.KeyLoopQueryValidateSkillReceiptFailed, err, result.ToolUseID)
		}
		if !found || !skillExecutionReceiptVisible(messages, result, receipt) {
			continue
		}

		q.skillCatalogMu.Lock()
		q.ensureSkillCatalogEpochLocked()
		if receipt.ContextEpoch == q.skillCatalogEpoch {
			existing, alreadyLoaded := q.loadedSkillDigests[receipt.SkillID]
			if receipt.InvocationEnvelopeKind != skills.InvocationEnvelopeAlreadyLoaded ||
				(alreadyLoaded && existing.ContentDigest == receipt.ContentDigest && existing.PayloadDigest == receipt.InvocationPayloadDigest) {
				q.loadedSkillDigests[receipt.SkillID] = SkillLoadedLedgerEntry{
					ContentDigest: receipt.ContentDigest,
					PayloadDigest: receipt.InvocationPayloadDigest,
				}
			}
		}
		q.skillCatalogMu.Unlock()
	}
	return nil
}

func skillExecutionReceiptVisible(messages []types.Message, expected types.ToolResultBlock, receipt skills.SkillExecutionReceipt) bool {
	encodedReceipt := expected.Metadata[skills.SkillExecutionReceiptMetadataKey]
	if strings.TrimSpace(expected.ToolUseID) == "" || strings.TrimSpace(encodedReceipt) == "" {
		return false
	}
	if err := validateVisibleSkillInvocationEnvelope(expected.Content, receipt); err != nil {
		return false
	}
	for _, message := range messages {
		if message.Role != types.RoleUser || message.IsInternalRuntimeMessage() {
			continue
		}
		for _, block := range message.Content {
			result, ok := block.(types.ToolResultBlock)
			if !ok || result.ToolUseID != expected.ToolUseID || result.IsError || result.Outcome != types.ToolOutcomeSucceeded {
				continue
			}
			// A receipt proves that one exact rendered envelope is about to enter
			// visible history. Metadata alone is insufficient: aggregate result
			// budgeting may persist/replace the envelope while retaining metadata.
			// In that case a later invocation must load the body again rather than
			// treating a persistence stub as the complete skill text.
			if result.Content == expected.Content &&
				reflect.DeepEqual(result.ContentBlocks, expected.ContentBlocks) &&
				result.Metadata[skills.SkillExecutionReceiptMetadataKey] == encodedReceipt {
				return true
			}
		}
	}
	return false
}

type visibleSkillInvocationEnvelope struct {
	Type    string                        `json:"type"`
	Version int                           `json:"version"`
	Kind    skills.InvocationEnvelopeKind `json:"kind"`
	Skill   struct {
		ID       skills.SkillID       `json:"id"`
		Name     string               `json:"name"`
		Revision skills.SkillRevision `json:"revision"`
		Digest   skills.SkillDigest   `json:"digest"`
		Source   skills.SkillSource   `json:"source"`
		Locator  skills.SkillLocator  `json:"locator"`
	} `json:"skill"`
	Arguments      skills.InvocationArguments     `json:"arguments"`
	PayloadDigest  skills.InvocationPayloadDigest `json:"payload_digest"`
	PreviousDigest skills.SkillDigest             `json:"previous_digest,omitempty"`
	Body           *string                        `json:"body,omitempty"`
}

func validateVisibleSkillInvocationEnvelope(encoded string, receipt skills.SkillExecutionReceipt) error {
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var envelope visibleSkillInvocationEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("skill invocation envelope contains trailing JSON")
		}
		return err
	}
	if envelope.Type != "skill_invocation" || envelope.Version != skills.InvocationEnvelopeVersion {
		return errors.New("invalid skill invocation envelope header")
	}
	if envelope.Kind != receipt.InvocationEnvelopeKind || envelope.Skill.ID != receipt.SkillID ||
		envelope.Skill.Digest != receipt.ContentDigest || envelope.PayloadDigest != receipt.InvocationPayloadDigest {
		return errors.New("skill invocation envelope does not match execution receipt")
	}
	if err := envelope.Skill.ID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(envelope.Skill.Name) == "" {
		return errors.New("skill invocation envelope has incomplete skill identity")
	}
	if err := envelope.Skill.Revision.Validate(); err != nil {
		return err
	}
	if err := envelope.Skill.Digest.Validate(); err != nil {
		return err
	}
	if err := envelope.Skill.Source.Validate(); err != nil {
		return err
	}
	if source, ok := envelope.Skill.ID.Source(); !ok || source != envelope.Skill.Source {
		return errors.New("skill invocation envelope ID source mismatch")
	}
	if err := envelope.Skill.Locator.Validate(); err != nil {
		return err
	}
	if err := envelope.PayloadDigest.Validate(); err != nil {
		return err
	}
	if !envelope.Arguments.Provided && envelope.Arguments.Value != "" {
		return errors.New("omitted skill invocation arguments carry a value")
	}

	switch envelope.Kind {
	case skills.InvocationEnvelopeFull:
		if envelope.Body == nil || envelope.PreviousDigest != "" {
			return errors.New("invalid full skill invocation envelope")
		}
	case skills.InvocationEnvelopeSuperseding:
		if envelope.Body == nil || envelope.PreviousDigest.Validate() != nil || envelope.PreviousDigest == envelope.Skill.Digest {
			return errors.New("invalid superseding skill invocation envelope")
		}
	case skills.InvocationEnvelopeAlreadyLoaded:
		if envelope.Body != nil || envelope.PreviousDigest != "" {
			return errors.New("invalid already-loaded skill invocation envelope")
		}
		return nil
	default:
		return errors.New("unknown skill invocation envelope kind")
	}
	if skills.DigestInvocationPayload(*envelope.Body) != envelope.PayloadDigest {
		return errors.New("skill invocation body digest does not match payload digest")
	}
	return nil
}

// Model returns the model identifier currently configured for the loop.
func (q *QueryLoop) Model() string {
	return q.config.Model
}

// SetModel updates the model identifier used for future requests.
func (q *QueryLoop) SetModel(model string) {
	if strings.TrimSpace(model) != strings.TrimSpace(q.config.Model) {
		q.invalidateProviderContinuation()
	}
	q.config.Model = model
}

// SetReasoningEffort updates the reasoning effort used for future requests.
func (q *QueryLoop) SetReasoningEffort(effort string) {
	q.config.ReasoningEffort = effort
}

// SetProvider replaces the provider used for future requests.
// If the current provider is a *ProviderRef, it calls Swap() to notify listeners.
// Otherwise it replaces the provider field directly.
// This takes effect between queries — an in-flight stream is not interrupted.
//
// After swapping, it adapts MaxContextTokens to min(originalMax, providerMax)
// so that switching between models with different context windows works seamlessly.
func (q *QueryLoop) SetProvider(p provider.Provider) {
	q.invalidateProviderContinuation()
	if pRef, ok := q.provider.(*provider.ProviderRef); ok {
		pRef.Swap(p)
	} else {
		q.provider = p
	}
	q.adaptContextWindow()
}

// HandleProviderChange invalidates protocol- and provider-bound continuation
// state after the shared ProviderRef has already been swapped. CoreEngine uses
// this path so it cannot bypass the isolation performed by SetProvider.
func (q *QueryLoop) HandleProviderChange() {
	q.invalidateProviderContinuation()
	q.adaptContextWindow()
}

func (q *QueryLoop) invalidateProviderContinuation() {
	q.advanceContinuationEpoch()
	q.messages = stripThinkingSignatures(q.messages)
	q.lastResponseID = ""
	q.lastEnvelopeFingerprint = ""
	q.currentEnvelopeFingerprint = ""
	q.disableResponseChain = false
}

func (q *QueryLoop) advanceContinuationEpoch() {
	q.continuationEpoch++
	if q.continuationEpoch == 0 {
		q.continuationEpoch = 1
	}
}

// AdaptContextWindow re-reads the current provider's capabilities and updates
// MaxContextTokens and the ContextWindow to match. Exported so that the runtime engine can
// call it after swapping the underlying ProviderRef without going through
// SetProvider (which would double-Swap).
func (q *QueryLoop) AdaptContextWindow() {
	q.adaptContextWindow()
}

// adaptContextWindow sets MaxContextTokens to the current provider's actual
// context window size. Called after provider initialization and on every
// provider switch, so the context budget always matches the active model.
func (q *QueryLoop) adaptContextWindow() {
	caps := q.providerCapabilities()
	if caps.MaxContext <= 0 {
		return // provider didn't report a limit; keep config as-is
	}
	q.config.MaxContextTokens = caps.MaxContext
	if q.ctxWindow != nil {
		q.ctxWindow.MaxTokens = caps.MaxContext
	}
}

// SetSessionID sets the runtime conversation identity.
func (q *QueryLoop) SetSessionID(id string) {
	q.config.SessionID = id
	if sc, ok := q.compactor.(*compact.SummaryCompactor); ok {
		sc.SessionID = id
		sc.AttachmentProvider = q.PostCompactAttachmentProvider()
	}
}

// SetThinkingConfig enables or disables extended thinking for future requests.
// When enabled is false the config is cleared (thinking disabled).
// When enabled is true, budgetTokens controls the token budget (0 = provider default).
func (q *QueryLoop) SetThinkingConfig(enabled bool, budgetTokens int) {
	if !enabled {
		q.thinkingConfig = nil
		return
	}
	q.thinkingConfig = &provider.ThinkingConfig{
		Enabled:      true,
		BudgetTokens: budgetTokens,
	}
}

// ContextUsage returns the current context window usage.
// Returns (0, 0) if compaction is disabled.
func (q *QueryLoop) ContextUsage() (maxTokens, usedTokens int) {
	maxTokens, usage := q.ContextUsageDetail()
	return maxTokens, usage.UsedTokens
}

// ContextUsageDetail preserves whether the current value came from a local
// preflight estimate or authoritative provider usage.
func (q *QueryLoop) ContextUsageDetail() (maxTokens int, usage compact.ContextInputUsage) {
	if q.ctxWindow == nil {
		return 0, compact.ContextInputUsage{Measurement: compact.ContextUsageUnknown}
	}
	usage = q.ctxWindow.CurrentInputUsage()
	return max(q.ctxWindow.MaxTokens, 0), usage
}

// ContextWarningState returns the current context warning state for UI and
// slash-command display.
func (q *QueryLoop) ContextWarningState() compact.TokenWarningState {
	if q.ctxWindow == nil {
		return compact.TokenWarningState{}
	}
	return q.ctxWindow.TokenWarningState(-1, compact.ShouldUseAutoCompact())
}

func (q *QueryLoop) blockIfManualCompactReserveExceeded(estimate compact.ModelContextTokenEstimate, snapshot QueryConfigSnapshot, turnCount int, onEvent func(streamevent.Event)) error {
	if q.ctxWindow == nil || compact.ShouldUseAutoCompact() {
		return nil
	}
	switch snapshot.QuerySource {
	case QuerySourceCompact:
		return nil
	}
	tokenUsage := estimate.KnownTotalTokens
	state := q.ctxWindow.TokenWarningState(tokenUsage, false)
	if !state.IsAtBlockingLimit {
		return nil
	}
	err := &types.APIError{
		Type:    "prompt_too_long",
		Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimePromptTooLong),
		Status:  400,
	}
	onEvent(streamevent.Event{
		Type:      streamevent.EventError,
		Text:      err.Message,
		Error:     err,
		TurnCount: turnCount,
		Metadata: map[string]any{
			"reason":            "blocking_limit",
			"used_tokens":       state.UsedTokens,
			"blocking_limit":    state.BlockingLimitTokens,
			"estimate_complete": estimate.Complete,
			"unknown_overheads": append([]compact.TokenOverheadKind(nil), estimate.UnknownOverheads...),
		},
	})
	return err
}

// providerCapabilities returns the capabilities of the underlying provider,
// delegating through RetryProvider wrappers if necessary.
func (q *QueryLoop) providerCapabilities() provider.ProviderCapabilities {
	if cp, ok := q.provider.(provider.CapabilityProvider); ok {
		return cp.Capabilities()
	}
	return provider.ProviderCapabilities{}
}

func escalatedMaxOutputTokens(model string, current int) int {
	target := escalatedMaxTokens
	if maxOutput := provider.LookupMaxOutput(model); maxOutput > 0 && maxOutput < target {
		target = maxOutput
	}
	if target <= current {
		return 0
	}
	return target
}

func (q *QueryLoop) maxOutputTokensRecoveryMessage() types.Message {
	message := types.UserMessage(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopVisibleOutputTokenRecovery))
	message.IsMeta = true
	message.InternalKind = types.InternalMessageKindOutputTokenRecovery
	return q.sealRuntimeControlMessage(message)
}

func (q *QueryLoop) toolInputRecoveryMessage(toolNames string) types.Message {
	message := types.UserMessage(i18n.Format(
		i18n.DetectOrLoadLanguage(),
		i18n.KeyLoopVisibleToolInputRecovery,
		toolNames,
	))
	message.IsMeta = true
	message.InternalKind = types.InternalMessageKindToolInputRecovery
	return q.sealRuntimeControlMessage(message)
}

func invalidToolUseNames(invalid []types.InvalidToolUseBlock) string {
	return strings.Join(invalidToolUseNameList(invalid), ", ")
}

func invalidToolUseNameList(invalid []types.InvalidToolUseBlock) []string {
	seen := make(map[string]struct{}, len(invalid))
	names := make([]string, 0, len(invalid))
	for _, call := range invalid {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			name = strings.TrimSpace(call.ID)
		}
		if name == "" {
			name = "?"
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func containsToolInputCorrection(toolUses []types.ToolUseBlock, expected []string) bool {
	if len(toolUses) == 0 {
		return false
	}
	if len(expected) == 0 {
		return true
	}
	for _, name := range expected {
		found := false
		for _, toolUse := range toolUses {
			if toolUse.Name == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Run sends a user message and runs the agentic loop until completion.
func (q *QueryLoop) Run(ctx context.Context, userMessage string, onEvent func(streamevent.Event)) error {
	q.messages = append(q.messages, types.UserMessage(userMessage))
	return q.runLoop(ctx, onEvent, len(q.messages)-1)
}

// RunMessage appends a pre-classified user-role message. Runtime follow-up
// callers use it to preserve InternalKind/IsMeta without changing the content
// sent to the model.
func (q *QueryLoop) RunMessage(ctx context.Context, message types.Message, onEvent func(streamevent.Event)) error {
	q.messages = append(q.messages, message)
	return q.runLoop(ctx, onEvent, len(q.messages)-1)
}

// RunPrepared runs the agentic loop using messages already present in the
// QueryLoop. Callers use this when the initial user message has been
// constructed as structured content before the loop starts.
func (q *QueryLoop) RunPrepared(ctx context.Context, onEvent func(streamevent.Event)) error {
	return q.runLoop(ctx, onEvent, trailingPlainUserInputIndex(q.messages))
}

// RunWithContent sends a multimodal user message (e.g. containing image blocks)
// and runs the agentic loop until completion.  content must have at least one
// element; call Run for plain-text messages.
func (q *QueryLoop) RunWithContent(ctx context.Context, content []types.ContentBlock, onEvent func(streamevent.Event)) error {
	q.messages = append(q.messages, types.Message{
		Role:    types.RoleUser,
		Content: content,
	})
	return q.runLoop(ctx, onEvent, len(q.messages)-1)
}

func (q *QueryLoop) providerParams(state *QueryState, snapshot QueryConfigSnapshot, messages []types.Message) provider.Params {
	params := q.providerParamsBase(state, snapshot, messages)
	fingerprint := envelopeFingerprint(params)
	q.currentEnvelopeFingerprint = fingerprint
	params.PreviousResponseID = q.previousResponseIDForRequest(fingerprint)
	return params
}

func (q *QueryLoop) providerParamsBase(state *QueryState, snapshot QueryConfigSnapshot, messages []types.Message) provider.Params {
	messages = compact.StripProviderPrivateBlocks(messages)
	messages = collapseMaxOutputTokensRecoveryMessages(messages)
	userContext := snapshot.UserContext
	if snapshot.GoalRuntime != nil {
		current, err := snapshot.GoalRuntime.LoadGoal()
		if err == nil {
			userContext = userContext.WithGoal(current)
		} else {
			userContext = userContext.WithGoal(nil)
		}
	}
	messages = userContext.PrependTo(messages)
	tools := q.visibleToolDefinitions()
	if snapshot.VisibleTools.Valid() {
		tools = snapshot.VisibleTools.Definitions()
	}
	systemBlocks := snapshot.SystemContext.AppendTo(snapshot.SystemBlocks)
	systemBlocks = prompt.ApplyCacheScopes(systemBlocks, prompt.CacheScopeOptions{
		GlobalSafe:      q.provider.Name() == "anthropic",
		ToolCacheMarker: len(tools) > 0,
	})
	system := snapshot.System
	if len(systemBlocks) == 0 && !snapshot.SystemContext.IsZero() {
		if block, ok := snapshot.SystemContext.Block(); ok {
			if system == "" {
				system = block.Text
			} else {
				system += "\n\n" + block.Text
			}
		}
	}
	promptCacheKey := catalogCacheLineage(snapshot.CacheLineageID, snapshot)
	continuationReset := q.continuationSentAt != 0 && q.continuationSentAt != q.continuationEpoch
	params := provider.Params{
		Model:                   snapshot.Model,
		MaxTokens:               snapshot.MaxTokens,
		MaxOutputTokensOverride: state.MaxOutputTokensOverride,
		System:                  system,
		SystemBlocks:            systemBlocks,
		Messages:                messages,
		Tools:                   tools,
		PromptCacheKey:          promptCacheKey,
		UsePromptCache:          promptCacheKey != "",
		ReasoningEffort:         snapshot.ReasoningEffort,
		ServiceTier:             snapshot.ServiceTier,
		Thinking:                snapshot.Thinking,
		ContinuationLineage:     q.continuationLineage,
		ContinuationEpoch:       q.continuationEpoch,
		ContinuationReset:       continuationReset,
	}
	q.continuationSentAt = q.continuationEpoch
	params = params.WithInternalControlScope(messagecontrol.Runtime(), q.internalControlScope)
	if snapshot.TaskBudget > 0 {
		params.TaskBudget = &provider.TaskBudget{
			Total:     snapshot.TaskBudget,
			Remaining: state.TaskBudgetRemaining,
		}
	}
	return params
}

type providerAttemptIdentity struct {
	provider       string
	model          string
	requestID      string
	requestStarted time.Time
	requestElapsed time.Duration
	attempt        int
	maxRetries     int
	retryCount     int
}

func (a providerAttemptIdentity) requestStatus(now time.Time) *streamevent.RequestStatusEvent {
	status := &streamevent.RequestStatusEvent{
		RequestID:  a.requestID,
		Attempt:    a.attempt,
		MaxRetries: a.maxRetries,
		RetryCount: a.retryCount,
	}
	if !a.requestStarted.IsZero() {
		status.StartedAt = a.requestStarted.UTC().Format(time.RFC3339Nano)
		status.TotalMilliseconds = now.Sub(a.requestStarted).Milliseconds()
	}
	status.RequestMilliseconds = a.requestElapsed.Milliseconds()
	return status
}

func attachRequestUsage(status *streamevent.RequestStatusEvent, usage *types.Usage) {
	if status == nil || usage == nil {
		return
	}
	status.InputTokens = usage.InputTokens
	status.CacheReadInputTokens = usage.CacheReadInputTokens
	status.CacheWriteInputTokens = usage.CacheCreationInputTokens
	status.OutputTokens = usage.OutputTokens
}

func projectToolRoundMetrics(roundID string, logicalCalls int, metrics toolExecutionMetrics) *streamevent.ToolRoundMetricsEvent {
	return &streamevent.ToolRoundMetricsEvent{
		RoundID:                       roundID,
		LogicalModelVisibleCalls:      logicalCalls,
		PhysicalChildOperations:       metrics.PhysicalChildOperations,
		Fanout:                        metrics.PeakFanout,
		BatchCount:                    metrics.BatchCount,
		QueueMilliseconds:             metrics.QueueDuration.Milliseconds(),
		CriticalPathMilliseconds:      metrics.CriticalPathDuration.Milliseconds(),
		TotalChildLatencyMilliseconds: metrics.TotalChildLatency.Milliseconds(),
		ErrorCount:                    metrics.ErrorCount,
		RevisionFusionCount:           metrics.RevisionFusionCount,
		RevisionBarrierSkips:          metrics.RevisionBarrierSkips,
		RevisionMismatchCount:         metrics.RevisionMismatchCount,
	}
}

type firstTokenObserverContextKey struct{}

func withFirstTokenObserver(ctx context.Context, observer func()) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, firstTokenObserverContextKey{}, observer)
}

func notifyFirstTokenObserver(ctx context.Context) {
	observer, _ := ctx.Value(firstTokenObserverContextKey{}).(func())
	if observer != nil {
		observer()
	}
}

type pendingProviderAttemptUsage struct {
	identity  providerAttemptIdentity
	usage     types.Usage
	turnCount int
	emit      func(streamevent.Event)
}

func newProviderAttemptIdentity(p provider.Provider, model string) providerAttemptIdentity {
	identity := providerAttemptIdentity{model: strings.TrimSpace(model), attempt: 1}
	if p == nil {
		return identity
	}
	identity.provider = strings.TrimSpace(p.Name())
	if identity.model == "" {
		identity.model = strings.TrimSpace(p.ModelID())
	}
	return identity
}

func withProviderAttemptMetadata(metadata map[string]any, identity providerAttemptIdentity) map[string]any {
	result := make(map[string]any, len(metadata)+4)
	for key, value := range metadata {
		result[key] = value
	}
	if identity.provider != "" {
		result["provider"] = identity.provider
	}
	if identity.model != "" {
		result["model"] = identity.model
	}
	if identity.requestID != "" {
		result["request_id"] = identity.requestID
		result["usage_id"] = "provider_request:" + identity.requestID
	}
	return result
}

const uncommittedProviderResponseReason = "provider_response_uncommitted"

// emitUncommittedProviderResponseTombstone invalidates any streamed deltas from
// an attempt that never reached the provider's commit event. The payload is
// deliberately content-free: correlation IDs and counts are sufficient for
// telemetry and transactional stream consumers without exposing partial model
// text, thinking, tool inputs, paths, or commands.
func emitUncommittedProviderResponseTombstone(onEvent func(streamevent.Event), identity providerAttemptIdentity, partial *PartialStreamError, turnCount int) {
	if onEvent == nil || partial == nil {
		return
	}
	metadata := withProviderAttemptMetadata(map[string]any{
		"partial_blocks": partial.PartialBlocks,
		"open_blocks":    partial.OpenBlocks,
		"safe_to_replay": partial.SafeToReplay(),
	}, identity)
	if disposition := providerFailureDisposition(partial.Cause); disposition != "" {
		metadata["disposition"] = disposition
	}
	onEvent(streamevent.Event{
		Type:           streamevent.EventTombstone,
		TerminalReason: uncommittedProviderResponseReason,
		TurnCount:      turnCount,
		Tombstone: &streamevent.TombstoneEvent{
			Reason:   uncommittedProviderResponseReason,
			Metadata: metadata,
		},
		Metadata: metadata,
	})
}

// providerFailureDisposition projects only a bounded, content-free failure
// class into machine events. Provider-controlled messages and unknown type
// strings never cross this telemetry boundary.
func providerFailureDisposition(err error) string {
	contract := provider.ClassifyAttemptError(err)
	switch contract.Class {
	case types.ProviderErrorClassTransport:
		if apiErr, ok := provider.AsAPIError(err); ok && strings.EqualFold(strings.TrimSpace(apiErr.Type), "stream_idle_timeout") {
			return "stream_idle_timeout"
		}
		return "stream_interrupted"
	case types.ProviderErrorClassThrottle, types.ProviderErrorClassOverload:
		return "provider_transient"
	case types.ProviderErrorClassAuth:
		return "provider_auth"
	case types.ProviderErrorClassContext:
		return "provider_context"
	case types.ProviderErrorClassQuota:
		return "provider_quota"
	case types.ProviderErrorClassPermanent:
		return "provider_request"
	default:
		return "provider_error"
	}
}

// runLoop executes the agentic turn loop, assuming the latest user message has
// already been appended to q.messages.
func (q *QueryLoop) runLoop(ctx context.Context, emitEvent func(streamevent.Event), currentInputIndices ...int) error {
	currentInputIndex := -1
	if len(currentInputIndices) > 0 {
		currentInputIndex = currentInputIndices[0]
	}
	q.applyPendingWorkspaceRuntime()
	if err := q.validateInternalControlScope(); err != nil {
		return i18n.WrapInternalError(i18n.KeyLoopQueryControlScopeInvalid, err)
	}
	snapshot := newQueryConfigSnapshot(q.config, q.thinkingConfig)
	if snapshot.SkillManager != nil {
		generation := snapshot.SkillProjectGeneration
		if generation == 0 {
			binding, err := snapshot.SkillManager.SnapshotBinding(snapshot.SessionID)
			if err != nil {
				return i18n.WrapInternalError(i18n.KeyLoopQueryBindSkillGenerationFailed, err)
			}
			generation = binding.ProjectGeneration
		} else if _, err := snapshot.SkillManager.SnapshotAtGeneration(snapshot.SessionID, generation); err != nil {
			return i18n.WrapInternalError(i18n.KeyLoopQueryValidateSkillGenerationFailed, err)
		}
		snapshot.SkillProjectGeneration = generation
		q.skillRunGenerationMu.Lock()
		q.skillRunGeneration = generation
		q.skillRunGenerationMu.Unlock()
		defer func() {
			q.skillRunGenerationMu.Lock()
			q.skillRunGeneration = 0
			q.skillRunGenerationMu.Unlock()
		}()
	}
	lineage, _ := executioncontract.ToolExecutionContextFromContext(ctx)
	state := newQueryState(q.messages)
	queryID := uuid.NewString()
	flightController, err := newAgenticFlightController(q.registry, snapshot, queryID, state.Messages)
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, err)
	}
	investigationTracker := &agenticInvestigationTracker{}
	if q.executionOwner == nil {
		q.executionOwner = executioncontract.NewOwner()
	}
	executioncontract.BeginRun(q.executionOwner, queryID)
	defer executioncontract.EndRun(q.executionOwner, queryID)
	var pendingUsage *pendingProviderAttemptUsage
	var latestAttempt providerAttemptIdentity
	flushPendingUsage := func() {
		if pendingUsage == nil {
			return
		}
		pending := pendingUsage
		pendingUsage = nil
		usage := pending.usage
		pending.emit(streamevent.Event{
			Type:      streamevent.EventProviderUsage,
			Usage:     &usage,
			TurnCount: pending.turnCount,
			Metadata: withProviderAttemptMetadata(map[string]any{
				"kind":        "provider_attempt",
				"disposition": "discarded",
			}, pending.identity),
		})
	}
	defer flushPendingUsage()
	emitTurnEvent := func(turnCount int, event streamevent.Event) {
		turnID, actorID, workUnitID := queryTurnIdentity(snapshot, queryID, turnCount)
		if event.Type != streamevent.EventSystemWarning && event.ProjectRoot == "" {
			event.ProjectRoot = snapshot.ProjectRoot
		}
		if event.TurnID == "" {
			event.TurnID = turnID
		}
		if event.ActorID == "" {
			event.ActorID = actorID
		}
		if event.ActorType == "" {
			event.ActorType = snapshot.AgentType
		}
		if event.WorkUnitID == "" {
			event.WorkUnitID = workUnitID
		}
		if event.Type == streamevent.EventSystemWarning {
			warning := runtimeevent.SystemWarningRuntimeEvent(event)
			if warning.SessionID == "" {
				warning.SessionID = snapshot.SessionID
			}
			event.RuntimeEvent = &warning
		}
		emitEvent(event)
	}
	budgetTracker := NewBudgetTracker()
	turnOutputTokens := 0
	if snapshot.MemoryPrefetcher != nil {
		state.PendingMemoryPrefetch = snapshot.MemoryPrefetcher.StartMemoryPrefetch(ctx, state.Messages)
	}
	defer func() {
		q.messages = state.Messages
		if snapshot.GoalRuntime == nil {
			return
		}
		current, err := snapshot.GoalRuntime.LoadGoal()
		if err != nil {
			return
		}
		emitTurnEvent(state.TurnCount, newGoalStatusEvent(current, state.TurnCount))
	}()

	for state.shouldContinue(snapshot.MaxTurns) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := enforceMessageHistoryLimit(state.Messages); err != nil {
			return err
		}

		catalogInsertionIndex := currentInputIndex
		if currentInputIndex >= 0 {
			currentInputIndex = -1
		} else {
			catalogInsertionIndex = skillCatalogInsertionIndexForTransition(state.Messages, state.Transition)
		}
		catalogInput, catalogBeforeCurrentInput := messageAt(state.Messages, catalogInsertionIndex)
		turnCount := state.beginNextTurn()
		if err := q.refreshVisibleToolEnvelope(&snapshot); err != nil {
			return i18n.WrapInternalError(i18n.KeyLoopQueryVisibleToolCatalogInvalid, err)
		}
		turnID, actorID, workUnitID := queryTurnIdentity(snapshot, queryID, turnCount)
		fallbackRetryConfig := provider.DefaultRetryConfig()
		fallbackRetryConfig.BaseDelay = retryBaseDelay
		fallbackRetryConfig.MaxDelay = 32 * retryBaseDelay
		attemptController := provider.AttemptControllerForProvider(q.provider, fallbackRetryConfig)
		onEvent := func(event streamevent.Event) {
			if event.Type == streamevent.EventTurnEnd {
				event.Metadata = withProviderAttemptMetadata(event.Metadata, latestAttempt)
				// The provider portion of this attempt is accounted by the
				// terminal turn_end usage payload.
				pendingUsage = nil
			}
			emitTurnEvent(turnCount, event)
		}
		createProviderStream := func(params provider.Params) (<-chan types.StreamEvent, providerAttemptIdentity, error) {
			// Starting another provider request means the previous response will
			// not receive a turn_end event (retry, recovery, or continuation).
			flushPendingUsage()
			attempt := newProviderAttemptIdentity(q.provider, params.Model)
			attempt.requestID = uuid.NewString()
			attempt.requestStarted = time.Now()
			latestAttempt = attempt
			requestCtx := provider.WithRetryObserver(ctx, func(retry provider.RetryEvent) {
				attempt.retryCount = retry.Attempt
				attempt.attempt = retry.Attempt + 1
				attempt.maxRetries = retry.MaxRetries
				requestStatus := attempt.requestStatus(time.Now())
				// Attempt retains the established retry-event contract: it is the
				// one-based failed attempt that caused this retry. RetryCount is the
				// unambiguous cumulative retry counter used by telemetry parsers.
				requestStatus.Attempt = retry.Attempt
				requestStatus.RetryDelayMilliseconds = retry.Delay.Milliseconds()
				requestStatus.RetryKind = retry.Kind
				if retry.Err != nil {
					requestStatus.Error = retry.Err.Error()
				}
				onEvent(streamevent.Event{Type: streamevent.EventRequestRetry, TurnCount: turnCount, RequestStatus: requestStatus})
			})
			stream, err := provider.CreateStreamAttempt(requestCtx, attemptController, q.provider, params)
			attempt.attempt = attemptController.Attempts()
			attempt.retryCount = max(attempt.attempt-1, 0)
			attempt.maxRetries = attemptController.MaxAttempts() - 1
			attempt.requestElapsed = time.Since(attempt.requestStarted)
			status := attempt.requestStatus(time.Now())
			if err == nil {
				onEvent(streamevent.Event{
					Type: streamevent.EventRequestStart, TurnCount: turnCount,
					RequestStatus: status,
				})
			} else {
				status.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
				status.Error = err.Error()
				onEvent(streamevent.Event{
					Type: streamevent.EventRequestFailed, TurnCount: turnCount,
					RequestStatus: status,
				})
			}
			latestAttempt = attempt
			return stream, attempt, err
		}
		ctx = hooks.WithCorrelation(ctx, hooks.HookInput{
			SessionID:   snapshot.SessionID,
			ProjectRoot: snapshot.ProjectRoot,
			TurnID:      turnID,
			WorkUnitID:  workUnitID,
			AgentID:     actorID,
			AgentType:   snapshot.AgentType,
		})

		state.Messages = q.injectMCPInstructionsDelta(state.Messages)
		if snapshot.SkillPrefetcher != nil {
			state.PendingSkillPrefetch = snapshot.SkillPrefetcher.StartSkillPrefetch(ctx, state.Messages)
		}

		epochBeforePrepare := q.SkillCatalogState().ContextEpoch
		prepared, err := q.prepareMessagesForQuery(ctx, state, turnCount, snapshot.TaskBudget, snapshot.SessionID != "", onEvent, snapshot)
		if err != nil {
			return err
		}
		// Auto-compaction is a wholesale visible-history replacement. Task-specific
		// compact paths may already have installed the epoch fence; the equality
		// check keeps this sampling boundary compatible with that wiring while
		// preventing a stale catalog cursor when they have not.
		if state.AutoCompactTracking.Compacted && q.SkillCatalogState().ContextEpoch == epochBeforePrepare {
			state.Messages = q.installVisibleHistory(state.Messages)
		}
		apiMessages := compact.StripProviderPrivateBlocks(prepared.Messages)

		// Plan exactly one developer catalog projection after all preparation that
		// can replace or project history. Insert that same projection into durable
		// visible history and the provider view so "planned" cannot diverge from
		// "actually sampled". Initial/current inputs retain immediate adjacency;
		// later deltas append without rewriting the prior cached prefix.
		stateInsertBefore, apiInsertBefore := -1, -1
		if catalogBeforeCurrentInput {
			stateInsertBefore = messageIndexFromEnd(state.Messages, catalogInput)
			apiInsertBefore = messageIndexFromEnd(apiMessages, catalogInput)
		}
		var catalogMessage *types.Message
		state.Messages, catalogMessage, err = q.prepareSkillCatalogForSampling(state.Messages, snapshot, stateInsertBefore)
		if err != nil {
			return err
		}
		if catalogMessage != nil {
			apiMessages = insertMessageAt(apiMessages, *catalogMessage, apiInsertBefore)
		}
		if q.ctxWindow != nil {
			preflightEstimate := q.ctxWindow.EstimateProviderRequest(q.providerParamsBase(state, snapshot, apiMessages))
			if err := q.blockIfManualCompactReserveExceeded(preflightEstimate, snapshot, turnCount, onEvent); err != nil {
				q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(err.Error()))
				return err
			}
		}
		if err := runQueryHook(ctx, snapshot.HookRunner, hooks.HookPreQuery, hooks.HookInput{
			SessionID:   snapshot.SessionID,
			ProjectRoot: snapshot.ProjectRoot,
			TurnID:      turnID,
			WorkUnitID:  workUnitID,
			AgentID:     actorID,
			AgentType:   snapshot.AgentType,
			Messages:    hookMessages(apiMessages),
		}, onEvent); err != nil {
			return err
		}

		params := q.providerParams(state, snapshot, apiMessages)
		// A rejected previous_response_id receives exactly one full-history
		// repair within this logical generation. The shared AttemptController
		// still charges the repair's raw transport call.
		responseChainRepairUsed := false

		// --- Recovery path 1: transient API error retry ---
		// Every stream connection below consumes the stream-reconnect budget.
		// Provider-internal pre-stream HTTP failures use a fresh, independent
		// request budget for each connection attempt.
		maxStreamAttempts := attemptController.MaxAttempts()
		var stream <-chan types.StreamEvent
		var streamAttempt providerAttemptIdentity
		recoveredTerminalFailure := false
		for retry := 0; retry < maxStreamAttempts; retry++ {
			var streamErr error
			stream, streamAttempt, streamErr = createProviderStream(params)
			if streamErr == nil {
				break
			}
			if fallbackErr, ok := provider.AsFallbackTriggeredError(streamErr); ok && fallbackErr.FallbackModel != "" {
				if snapshot.PinnedModel {
					contractErr := i18n.WrapInternalError(
						i18n.KeyLoopQueryPinnedModelFallback,
						fallbackErr,
						snapshot.Model,
						fallbackErr.FallbackModel,
					)
					q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(contractErr.Error()))
					return contractErr
				}
				snapshot.Model = fallbackErr.FallbackModel
				snapshot.GoalEvaluator = bindGoalEvaluatorModel(snapshot.GoalEvaluator, snapshot.Model)
				apiMessages = stripThinkingSignatures(apiMessages)
				params = q.providerParams(state, snapshot, apiMessages)
				onEvent(NewSystemWarningEvent(
					i18n.KeyRuntimeModelFallback,
					[]any{fallbackErr.OriginalModel, fallbackErr.FallbackModel},
					fallbackErr,
					map[string]any{
						"original_model": fallbackErr.OriginalModel,
						"fallback_model": fallbackErr.FallbackModel,
					},
					turnCount,
				))
				stream, streamAttempt, streamErr = createProviderStream(params)
				if streamErr == nil {
					break
				}
			}
			// Recovery: if previous_response_id was rejected, clear it and retry
			// with full message history (graceful degradation).
			// The full-history fallback consumes the same raw-attempt budget.
			if !responseChainRepairUsed && q.lastResponseID != "" && isPreviousResponseNotFound(streamErr) {
				// Silently fall back to full message history — no user-facing warning.
				responseChainRepairUsed = true
				q.lastResponseID = ""
				params.PreviousResponseID = ""
				q.disableResponseChain = true
				// Retry immediately; CreateStreamAttempt charges this raw call.
				stream, streamAttempt, streamErr = createProviderStream(params)
				if streamErr == nil {
					break
				}
				// Fallback also failed — fall through to normal error handling
			}
			recovered, recoveryErr := q.recoverFromTerminalProviderFailure(ctx, state, apiMessages, streamErr, turnCount, onEvent)
			if recoveryErr != nil {
				onEvent(terminalProviderErrorEvent(streamErr, turnCount))
				q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(streamErr.Error()))
				return i18n.WrapError(i18n.KeyLoopQueryAPICallRecoveryFailed, streamErr, recoveryErr)
			}
			if recovered {
				recoveredTerminalFailure = true
				break
			}
			delay, canRetry := attemptController.RetryDelay(streamErr)
			if !canRetry || retry == maxStreamAttempts-1 || provider.IsAttemptLimit(streamErr) {
				onEvent(terminalProviderErrorEvent(streamErr, turnCount))
				q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(streamErr.Error()))
				return i18n.WrapError(i18n.KeyLoopQueryAPICallFailed, streamErr)
			}
			retryStatus := streamAttempt.requestStatus(time.Now())
			retryStatus.Attempt = attemptController.Attempts()
			retryStatus.RetryCount = attemptController.Attempts()
			retryStatus.MaxRetries = attemptController.MaxAttempts() - 1
			retryStatus.RetryDelayMilliseconds = delay.Milliseconds()
			retryStatus.RetryKind = "stream"
			retryStatus.Error = streamErr.Error()
			onEvent(streamevent.Event{
				Type: streamevent.EventRequestRetry, TurnCount: turnCount,
				RequestStatus: retryStatus,
			})
			onEvent(NewSystemWarningEvent(
				i18n.KeyRuntimeTransientAPIError,
				[]any{attemptController.Attempts(), attemptController.MaxAttempts() - 1},
				streamErr,
				nil,
				turnCount,
			))
			if waitErr := attemptController.Wait(ctx, delay); waitErr != nil {
				return waitErr
			}
		}
		if recoveredTerminalFailure {
			continue
		}

		// Guard against nil stream (e.g. all retries consumed by previous_response_id fallback)
		if stream == nil {
			q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopQueryStreamNotEstablished)))
			return i18n.NewError(i18n.KeyLoopQueryStreamMissingAfterAttempts, maxStreamAttempts)
		}

		newStreamingExecutor := func() *StreamingToolExecutor {
			if !snapshot.StreamingToolExecution {
				return nil
			}
			toolExecContext := q.bindToolExecutionContext(executioncontract.ToolExecutionContext{
				Messages:          append([]types.Message(nil), state.Messages...),
				SessionID:         snapshot.SessionID,
				CacheLineageID:    snapshot.CacheLineageID,
				TurnID:            turnID,
				ActorID:           actorID,
				ActorType:         snapshot.AgentType,
				WorkUnitID:        workUnitID,
				RunID:             lineage.RunID,
				BatchID:           lineage.BatchID,
				ParentRunID:       lineage.ParentRunID,
				AgentPath:         lineage.AgentPath,
				SessionProjectDir: snapshot.SessionProjectDir,
				ProjectRoot:       snapshot.ProjectRoot,
				CWD:               snapshot.CWD,
				System:            snapshot.System,
				Model:             snapshot.Model,
			}, queryID, q.skillLoadedLedgerCapability(state.Messages), snapshot.SkillProjectGeneration)
			return NewStreamingToolExecutor(flightController.bindVerificationContext(ctx), q.registry, snapshot.HookRunner, snapshot.PermissionHandler, snapshot.SessionID, toolExecContext)
		}
		processProviderStream := func(stream <-chan types.StreamEvent, attempt providerAttemptIdentity, streamingExecutor *StreamingToolExecutor) (*types.Message, *types.Usage, *types.StopReason, error) {
			firstTokenSeen := false
			streamCtx := withFirstTokenObserver(ctx, func() {
				if firstTokenSeen {
					return
				}
				firstTokenSeen = true
				now := time.Now()
				requestStatus := attempt.requestStatus(now)
				requestStatus.FirstTokenMilliseconds = now.Sub(attempt.requestStarted).Milliseconds()
				onEvent(streamevent.Event{
					Type: streamevent.EventRequestFirstToken, TurnCount: turnCount,
					RequestStatus: requestStatus,
				})
			})
			assistantMsg, usage, stopReason, err := q.processStream(streamCtx, stream, turnCount, onEvent, streamingExecutor)
			var partialErr *PartialStreamError
			if errors.As(err, &partialErr) {
				emitUncommittedProviderResponseTombstone(onEvent, attempt, partialErr, turnCount)
			}
			if usage != nil {
				pendingUsage = &pendingProviderAttemptUsage{
					identity:  attempt,
					usage:     *usage,
					turnCount: turnCount,
					emit:      onEvent,
				}
			}
			requestEventType := streamevent.EventRequestEnd
			endedAt := time.Now()
			requestStatus := attempt.requestStatus(endedAt)
			requestStatus.EndedAt = endedAt.UTC().Format(time.RFC3339Nano)
			attachRequestUsage(requestStatus, usage)
			if err != nil {
				requestEventType = streamevent.EventRequestFailed
				requestStatus.Error = err.Error()
			}
			onEvent(streamevent.Event{Type: requestEventType, TurnCount: turnCount, RequestStatus: requestStatus})
			return assistantMsg, usage, stopReason, err
		}
		streamingExecutor := newStreamingExecutor()
		assistantMsg, usage, stopReason, err := processProviderStream(stream, streamAttempt, streamingExecutor)
		// replayFullGeneration is the only recovery path for an uncommitted
		// response. It clears the uncertain response chain but retains the full
		// committed message history and prompt-cache affinity in params. Because
		// processStream does not start tools, replay cannot duplicate a tool side
		// effect.
		replayFullGeneration := func(replayCause error) error {
			q.lastResponseID = ""
			params.PreviousResponseID = ""
			for {
				var partialErr *PartialStreamError
				if errors.As(replayCause, &partialErr) {
					onEvent(NewSystemWarningEvent(
						i18n.KeyRuntimeStreamInterruptedPartial,
						[]any{partialErr.PartialBlocks},
						partialErr.Cause,
						map[string]any{"partial_blocks": partialErr.PartialBlocks},
						turnCount,
					))
				}

				delay, canRetry := attemptController.RetryDelay(replayCause)
				transportFallback := false
				contract := provider.ClassifyAttemptError(replayCause)
				if (!canRetry || provider.IsAttemptLimit(replayCause)) &&
					contract.Stage == types.ProviderErrorStageStream &&
					attemptController.Attempts() >= attemptController.MaxAttempts() {
					if from, to, activated := provider.TryFallbackTransport(q.provider); activated {
						onEvent(NewSystemWarningEvent(
							i18n.KeyRuntimeStreamTransportFallback,
							[]any{from, to},
							replayCause,
							map[string]any{"from_transport": from, "to_transport": to},
							turnCount,
						))
						attemptController = provider.AttemptControllerForProvider(q.provider, fallbackRetryConfig)
						transportFallback = true
						canRetry = true
						delay = 0
					}
				}
				if !canRetry || (provider.IsAttemptLimit(replayCause) && !transportFallback) {
					break
				}
				if !transportFallback {
					retryStatus := streamAttempt.requestStatus(time.Now())
					retryStatus.Attempt = attemptController.Attempts()
					retryStatus.RetryCount = attemptController.Attempts()
					retryStatus.MaxRetries = attemptController.MaxAttempts() - 1
					retryStatus.RetryDelayMilliseconds = delay.Milliseconds()
					retryStatus.RetryKind = "stream"
					retryStatus.Error = replayCause.Error()
					onEvent(streamevent.Event{
						Type: streamevent.EventRequestRetry, TurnCount: turnCount,
						RequestStatus: retryStatus,
					})
					onEvent(NewSystemWarningEvent(
						i18n.KeyRuntimeStreamRetryFullHistory,
						[]any{attemptController.Attempts(), attemptController.MaxAttempts() - 1, delay},
						replayCause,
						map[string]any{
							"retry_count":    attemptController.Attempts(),
							"max_retries":    attemptController.MaxAttempts() - 1,
							"retry_delay_ms": delay.Milliseconds(),
						},
						turnCount,
					))
					if waitErr := attemptController.Wait(ctx, delay); waitErr != nil {
						return waitErr
					}
				}

				stream2, retryAttempt, createErr := createProviderStream(params)
				streamAttempt = retryAttempt
				if createErr != nil {
					replayCause = createErr
					createContract := provider.ClassifyAttemptError(createErr)
					fallbackEligible := createContract.Stage == types.ProviderErrorStageStream &&
						attemptController.Attempts() >= attemptController.MaxAttempts()
					if (!attemptController.CanRetry(createErr) || provider.IsAttemptLimit(createErr)) && !fallbackEligible {
						break
					}
					continue
				}
				if streamingExecutor != nil {
					streamingExecutor.Discard()
				}
				streamingExecutor = newStreamingExecutor()
				assistantMsg, usage, stopReason, replayCause = processProviderStream(stream2, retryAttempt, streamingExecutor)
				if replayCause == nil {
					return nil
				}
				replayContract := provider.ClassifyAttemptError(replayCause)
				fallbackEligible := replayContract.Stage == types.ProviderErrorStageStream &&
					attemptController.Attempts() >= attemptController.MaxAttempts()
				if !attemptController.CanRetry(replayCause) && !fallbackEligible {
					break
				}
			}
			return replayCause
		}
		if err != nil {
			if fallbackErr, ok := provider.AsFallbackTriggeredError(err); ok && fallbackErr.FallbackModel != "" {
				emitFallbackTombstone(onEvent, assistantMsg, turnCount, fallbackErr.OriginalModel, fallbackErr.FallbackModel)
				if streamingExecutor != nil {
					streamingExecutor.Discard()
				}
				if snapshot.PinnedModel {
					contractErr := i18n.WrapInternalError(
						i18n.KeyLoopQueryPinnedModelFallback,
						fallbackErr,
						snapshot.Model,
						fallbackErr.FallbackModel,
					)
					q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(contractErr.Error()))
					return contractErr
				}
				snapshot.Model = fallbackErr.FallbackModel
				snapshot.GoalEvaluator = bindGoalEvaluatorModel(snapshot.GoalEvaluator, snapshot.Model)
				apiMessages = stripThinkingSignatures(apiMessages)
				params = q.providerParams(state, snapshot, apiMessages)
				onEvent(NewSystemWarningEvent(
					i18n.KeyRuntimeModelFallback,
					[]any{fallbackErr.OriginalModel, fallbackErr.FallbackModel},
					fallbackErr,
					map[string]any{
						"original_model": fallbackErr.OriginalModel,
						"fallback_model": fallbackErr.FallbackModel,
					},
					turnCount,
				))
				stream2, retryAttempt, retryErr := createProviderStream(params)
				if retryErr != nil {
					onEvent(terminalProviderErrorEvent(retryErr, turnCount))
					q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(retryErr.Error()))
					return i18n.WrapError(i18n.KeyLoopQueryStreamFallbackFailed, retryErr)
				}
				streamingExecutor = newStreamingExecutor()
				streamAttempt = retryAttempt
				assistantMsg, usage, stopReason, err = processProviderStream(stream2, retryAttempt, streamingExecutor)
				if err != nil && attemptController.CanRetry(err) {
					err = replayFullGeneration(err)
				}
				if err != nil {
					if streamingExecutor != nil {
						streamingExecutor.Discard()
					}
					onEvent(terminalProviderErrorEvent(err, turnCount))
					q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(err.Error()))
					return i18n.WrapError(i18n.KeyLoopQueryStreamAfterModelFallback, err)
				}
			} else {
				// --- Recovery path 4: uncommitted stream interrupt ---
				// Partial deltas have already been invalidated by a tombstone. They
				// are never appended to history and their tool batch is never run.
				if attemptController.CanRetry(err) {
					// --- Recovery path 5: upstream disconnect / response.failed retry ---
					// Aligned with Codex CLI: when an upstream disconnect or a
					// retryable response.failed occurs during streaming, the response
					// chain on the server is likely broken. The correct recovery is:
					//   1. Always clear previous_response_id (the server-side response
					//      is in an unknown state after a disconnect).
					//   2. Retry with full message history.
					//   3. Keep prompt_cache_key to preserve prompt cache affinity.

					err = replayFullGeneration(err)
					if err != nil {
						if streamingExecutor != nil {
							streamingExecutor.Discard()
						}
						onEvent(terminalProviderErrorEvent(err, turnCount))
						q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(err.Error()))
						return i18n.WrapError(i18n.KeyLoopQueryStreamAfterRetry, err)
					}

				} else if !responseChainRepairUsed && isPreviousResponseNotFound(err) {
					// --- Recovery path 5b: previous_response_id error during stream ---
					// This can happen if the error comes through as a stream-level
					// event (e.g. response.failed with previous_response_not_found).
					// Silently fall back — no user-facing warning.
					responseChainRepairUsed = true
					q.lastResponseID = ""
					params.PreviousResponseID = ""
					q.disableResponseChain = true

					stream2, retryAttempt, retryErr := createProviderStream(params)
					if retryErr != nil {
						onEvent(terminalProviderErrorEvent(retryErr, turnCount))
						q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(retryErr.Error()))
						return i18n.WrapError(i18n.KeyLoopQueryStreamFallbackFailed, retryErr)
					}
					if streamingExecutor != nil {
						streamingExecutor.Discard()
					}
					streamingExecutor = newStreamingExecutor()
					streamAttempt = retryAttempt
					assistantMsg, usage, stopReason, err = processProviderStream(stream2, retryAttempt, streamingExecutor)
					if err != nil && attemptController.CanRetry(err) {
						err = replayFullGeneration(err)
					}
					if err != nil {
						if streamingExecutor != nil {
							streamingExecutor.Discard()
						}
						onEvent(terminalProviderErrorEvent(err, turnCount))
						q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(err.Error()))
						return i18n.WrapError(i18n.KeyLoopQueryStreamAfterFallback, err)
					}
				} else {
					recovered, recoveryErr := q.recoverFromTerminalProviderFailure(ctx, state, apiMessages, err, turnCount, onEvent)
					if streamingExecutor != nil {
						streamingExecutor.Discard()
					}
					if recoveryErr != nil {
						onEvent(terminalProviderErrorEvent(err, turnCount))
						q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(err.Error()))
						return i18n.WrapError(i18n.KeyLoopQueryStreamRecoveryFailed, err, recoveryErr)
					}
					if recovered {
						continue
					}
					onEvent(terminalProviderErrorEvent(err, turnCount))
					q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(err.Error()))
					return i18n.WrapError(i18n.KeyLoopQueryStreamFailed, err)
				}
			}
		}

		// --- Recovery path 2: max_output_tokens escalation/recovery ---
		if stopReason != nil && *stopReason == types.StopReasonMaxTokens && len(assistantMsg.GetToolUses()) == 0 && len(assistantMsg.GetInvalidToolUses()) == 0 {
			goalRecoveryMustStop := func() bool {
				tracked, saveErr := saveGoalAssistantTurnUsage(snapshot.GoalRuntime, usage, time.Now())
				if saveErr != nil {
					emitGoalTurnSaveWarning(onEvent, turnCount, saveErr)
					state.Messages = append(state.Messages, *assistantMsg)
					q.startTurnSideEffects(ctx, snapshot, state.Messages)
					onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
					return true
				}
				if !tracked {
					return false
				}
				reached, current, loadErr := goalTokenBudgetReached(snapshot.GoalRuntime)
				if loadErr != nil {
					emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalLoadMaxTokens, nil, loadErr)
					state.Messages = append(state.Messages, *assistantMsg)
					q.startTurnSideEffects(ctx, snapshot, state.Messages)
					onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
					return true
				}
				if !reached {
					return false
				}
				emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalBudgetReached, []any{current.Usage, current.TokenBudget}, nil)
				state.Messages = append(state.Messages, *assistantMsg)
				q.startTurnSideEffects(ctx, snapshot, state.Messages)
				onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
				return true
			}
			onEvent(streamevent.Event{
				Type:      streamevent.EventError,
				Text:      i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeResponseTruncated),
				TurnCount: turnCount,
			})
			if target := escalatedMaxOutputTokens(snapshot.Model, snapshot.MaxTokens); state.MaxOutputTokensRecoveryCount == 0 && state.MaxOutputTokensOverride == 0 && target > snapshot.MaxTokens {
				state.MaxOutputTokensOverride = target
				state.Transition = QueryTransitionMaxOutputTokensEscalate
				onEvent(NewSystemWarningEvent(i18n.KeyRuntimeResponseRetryMaxTokens, []any{target}, nil, nil, turnCount))
				if goalRecoveryMustStop() {
					return nil
				}
				continue
			}

			if state.MaxOutputTokensRecoveryCount < maxOutputTokensRecoveryLimit {
				if goalRecoveryMustStop() {
					return nil
				}
				state.Messages = append(state.Messages, *assistantMsg)
				state.Messages = append(state.Messages, q.maxOutputTokensRecoveryMessage())
				state.MaxOutputTokensRecoveryCount++
				state.MaxOutputTokensOverride = 0
				state.Transition = QueryTransitionMaxOutputTokensRecovery
				onEvent(NewSystemWarningEvent(
					i18n.KeyRuntimeResponseRecovery,
					[]any{state.MaxOutputTokensRecoveryCount, maxOutputTokensRecoveryLimit},
					nil,
					nil,
					turnCount,
				))
				continue
			}
		}

		// --- Recovery path 3: empty response retry (once) ---
		if assistantMsg == nil || len(assistantMsg.Content) == 0 {
			emptyCause := &types.APIError{
				Type:         "empty_response",
				Message:      i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopQueryEmptyResponse),
				Stage:        types.ProviderErrorStageCommitted,
				Class:        types.ProviderErrorClassTransport,
				ReplaySafety: types.ProviderReplaySafe,
			}
			delay, canRetry := attemptController.RetryDelay(emptyCause)
			if !canRetry {
				if streamingExecutor != nil {
					streamingExecutor.Discard()
				}
				q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(emptyCause.Message))
				return i18n.NewError(i18n.KeyLoopQueryEmptyResponse)
			}
			if waitErr := attemptController.Wait(ctx, delay); waitErr != nil {
				return waitErr
			}
			stream2, retryAttempt, retryErr := createProviderStream(params)
			if retryErr == nil {
				if streamingExecutor != nil {
					streamingExecutor.Discard()
				}
				streamingExecutor = newStreamingExecutor()
				assistantMsg2, usage2, _, err2 := processProviderStream(stream2, retryAttempt, streamingExecutor)
				if err2 == nil && assistantMsg2 != nil && len(assistantMsg2.Content) > 0 {
					assistantMsg = assistantMsg2
					usage = usage2
				} else {
					if streamingExecutor != nil {
						streamingExecutor.Discard()
					}
					q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopQueryEmptyResponse)))
					return i18n.NewError(i18n.KeyLoopQueryEmptyResponseAfterRetry)
				}
			} else {
				if streamingExecutor != nil {
					streamingExecutor.Discard()
				}
				q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopQueryEmptyResponse)))
				return i18n.NewError(i18n.KeyLoopQueryEmptyResponse)
			}
		}

		// Update context window usage for compaction decisions
		if q.ctxWindow != nil && usage != nil {
			q.ctxWindow.UpdateUsage(usage)
		}
		if usage != nil {
			turnOutputTokens += usage.OutputTokens
		}

		// Check for prompt cache breaks (significant drop in cache_read_input_tokens).
		// This is informational only — server-side cache eviction/routing is not
		// actionable by the client. We still run the detector so that
		// NotifyCompaction() keeps the baseline accurate, but we no longer surface
		// it as an EventError in the UI (it confused users into thinking something
		// was broken).
		if q.cacheBreakDetector != nil && usage != nil {
			_ = q.cacheBreakDetector.Check(usage) // update baseline; result intentionally discarded
		}

		// Record the timestamp of this completed API turn so that idle-triggered
		// microcompact can determine when the prompt cache has expired.
		q.microcompactCfg.LastActivity = time.Now()

		// Calibrate token estimator with actual API usage
		if q.calibratedCounter != nil && usage != nil && usage.TotalInputTokens() > 0 {
			// Estimate chars for all messages sent in this turn
			totalChars := 0
			for _, msg := range apiMessages {
				totalChars += len(msg.GetText())
				for _, block := range msg.Content {
					if tr, ok := block.(types.ToolResultBlock); ok {
						totalChars += len(tr.TextContent())
					}
				}
			}
			if totalChars > 0 {
				q.calibratedCounter.Calibrate(totalChars, usage.TotalInputTokens())
				// Feed the updated calibrated ratio into the context window's
				// token counter so EstimateMessages uses real-world ratios instead
				// of the static TiktokenCounter default.
				if q.ctxWindow != nil {
					q.ctxWindow.Counter = q.calibratedCounter
				}
			}
		}

		invalidToolUses := assistantMsg.GetInvalidToolUses()
		if len(invalidToolUses) > 0 {
			if streamingExecutor != nil {
				streamingExecutor.Discard()
			}
			// Any provider-native continuation was authenticated against the
			// malformed output item and must never be replayed during correction.
			assistantMsg.ClearProviderContinuation()
			state.Messages = append(state.Messages, *assistantMsg)
			if _, saveErr := saveGoalAssistantTurnUsage(snapshot.GoalRuntime, usage, time.Now()); saveErr != nil {
				emitGoalTurnSaveWarning(onEvent, turnCount, saveErr)
			}

			// The committed server response contains an invalid function call, so
			// it cannot be a safe previous_response_id parent. Retry from the
			// durable full-history projection instead.
			q.lastResponseID = ""
			q.lastEnvelopeFingerprint = ""
			q.disableResponseChain = true
			toolNames := invalidToolUseNames(invalidToolUses)
			metadata := map[string]any{
				"reason":           "invalid_tool_input",
				"tool_names":       toolNames,
				"invalid_calls":    len(invalidToolUses),
				"correction_limit": toolInputRecoveryLimit,
			}
			if state.ToolInputRecoveryCount < toolInputRecoveryLimit {
				state.ToolInputRecoveryCount++
				state.ToolInputRecoveryTools = invalidToolUseNameList(invalidToolUses)
				metadata["correction_attempt"] = state.ToolInputRecoveryCount
				state.Messages = append(state.Messages, q.toolInputRecoveryMessage(toolNames))
				state.Transition = QueryTransitionToolInputRecovery
				onEvent(NewSystemWarningEvent(
					i18n.KeyRuntimeToolInputRecoveryRetry,
					[]any{toolNames, state.ToolInputRecoveryCount, toolInputRecoveryLimit},
					nil,
					metadata,
					turnCount,
				))
				onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
				continue
			}

			onEvent(NewSystemWarningEvent(
				i18n.KeyRuntimeToolInputRecoveryFailed,
				[]any{toolNames, state.ToolInputRecoveryCount},
				nil,
				metadata,
				turnCount,
			))
			q.startTurnSideEffects(ctx, snapshot, state.Messages)
			onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
			recoveryErr := i18n.NewError(i18n.KeyLoopToolInputRecoveryFailed, toolNames, state.ToolInputRecoveryCount)
			q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(recoveryErr.Error()))
			return recoveryErr
		}

		toolUses := assistantMsg.GetToolUses()
		if state.ToolInputRecoveryCount > 0 && !containsToolInputCorrection(toolUses, state.ToolInputRecoveryTools) {
			if streamingExecutor != nil {
				streamingExecutor.Discard()
			}
			state.Messages = append(state.Messages, *assistantMsg)
			if _, saveErr := saveGoalAssistantTurnUsage(snapshot.GoalRuntime, usage, time.Now()); saveErr != nil {
				emitGoalTurnSaveWarning(onEvent, turnCount, saveErr)
			}
			toolNames := strings.Join(state.ToolInputRecoveryTools, ", ")
			onEvent(NewSystemWarningEvent(
				i18n.KeyRuntimeToolInputRecoveryAbandoned,
				[]any{toolNames},
				nil,
				map[string]any{
					"reason":             "tool_input_correction_missing",
					"tool_names":         toolNames,
					"correction_attempt": state.ToolInputRecoveryCount,
				},
				turnCount,
			))
			q.startTurnSideEffects(ctx, snapshot, state.Messages)
			onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
			recoveryErr := i18n.NewError(i18n.KeyLoopToolInputRecoveryAbandoned, toolNames)
			q.runStopFailure(ctx, snapshot, onEvent, queryID, turnCount, types.AssistantMessage(recoveryErr.Error()))
			return recoveryErr
		}
		if state.ToolInputRecoveryCount > 0 {
			state.ToolInputRecoveryCount = 0
			state.ToolInputRecoveryTools = nil
		}
		if identityErr := validateToolUseIdentities(toolUses, q.seenToolUseIDs); identityErr != nil {
			if streamingExecutor != nil {
				streamingExecutor.Discard()
			}
			onEvent(toolUseIdentityErrorEvent(identityErr, turnCount))
			return identityErr
		}
		if catalogErr := validateToolUsesAgainstVisibleCatalog(toolUses, snapshot.VisibleTools); catalogErr != nil {
			if streamingExecutor != nil {
				streamingExecutor.Discard()
			}
			onEvent(toolUseCatalogErrorEvent(catalogErr, turnCount))
			return catalogErr
		}
		for _, toolUse := range toolUses {
			q.seenToolUseIDs[toolUse.ID] = struct{}{}
		}

		state.Messages = append(state.Messages, *assistantMsg)
		goalTurnTracked, goalTurnSaveErr := saveGoalAssistantTurnUsage(snapshot.GoalRuntime, usage, time.Now())
		if goalTurnSaveErr != nil {
			emitGoalTurnSaveWarning(onEvent, turnCount, goalTurnSaveErr)
		}
		postQueryMessages := append([]types.Message(nil), apiMessages...)
		postQueryMessages = append(postQueryMessages, *assistantMsg)
		if err := runQueryHook(ctx, snapshot.HookRunner, hooks.HookPostQuery, hooks.HookInput{
			SessionID:   snapshot.SessionID,
			ProjectRoot: snapshot.ProjectRoot,
			TurnID:      turnID,
			WorkUnitID:  workUnitID,
			AgentID:     actorID,
			AgentType:   snapshot.AgentType,
			Messages:    hookMessages(postQueryMessages),
			Result:      strings.TrimSpace(assistantMsg.GetText()),
		}, onEvent); err != nil {
			return err
		}
		if runner := q.postSamplingRunner(snapshot, onEvent); runner != nil {
			messagesForHook := append([]types.Message(nil), apiMessages...)
			messagesForHook = append(messagesForHook, *assistantMsg)
			postSampling := runner.RunPostSampling(ctx, messagesForHook, PostSamplingOptions{
				SessionID:  snapshot.SessionID,
				TurnID:     turnID,
				AgentID:    actorID,
				AgentType:  snapshot.AgentType,
				WorkUnitID: workUnitID,
			})
			if postSampling.Blocked {
				return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAuxPostSamplingBlockedReason, postSampling.Reason))
			}
		}

		if len(toolUses) == 0 {
			flightDecision, flightErr := flightController.requestFinal()
			if flightErr != nil {
				return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, flightErr)
			}
			if flightController != nil {
				switch flightDecision.Action {
				case agenticFlightTerminalContinue:
					onEvent(newAgenticFlightDispositionEvent(flightController, flightDecision, turnCount))
					state.Messages = append(state.Messages, flightController.verificationMessage(q, flightDecision.Blocker))
					state.Transition = QueryTransitionFlightVerification
					onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
					continue
				case agenticFlightTerminalBlocked:
					onEvent(newAgenticFlightDispositionEvent(flightController, flightDecision, turnCount))
					q.startTurnSideEffects(ctx, snapshot, state.Messages)
					onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
					return nil
				}
			}
			commitFlightFinal := func() error {
				if flightController == nil {
					return nil
				}
				if err := flightController.commitFinal(flightDecision); err != nil {
					return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, err)
				}
				onEvent(newAgenticFlightDispositionEvent(flightController, flightDecision, turnCount))
				return nil
			}
			blockFlightRuntime := func() error {
				if flightController == nil {
					return nil
				}
				decision, err := flightController.blockRuntime()
				if err != nil {
					return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, err)
				}
				onEvent(newAgenticFlightDispositionEvent(flightController, decision, turnCount))
				return nil
			}
			stopHooks, err := runStopHooks(ctx, snapshot.HookRunner, stopHookOptions{
				AssistantMessage:    *assistantMsg,
				StopHookActive:      state.StopHookActive,
				SessionID:           snapshot.SessionID,
				AgentID:             actorID,
				AgentType:           snapshot.AgentType,
				TurnID:              turnID,
				WorkUnitID:          workUnitID,
				AgentTranscriptPath: snapshot.AgentTranscriptPath,
				TeammateContext:     snapshot.TeammateContext,
			})
			if err != nil {
				return err
			}
			for i := range stopHooks.ExecutionSummaries {
				summary := stopHooks.ExecutionSummaries[i]
				onEvent(streamevent.Event{
					Type: streamevent.EventHookSummary, TurnCount: turnCount, TurnID: turnID,
					ActorID: actorID, ActorType: snapshot.AgentType, WorkUnitID: workUnitID,
					HookSummary: &summary,
				})
			}
			if stopHooks.PreventContinuation {
				if err := commitFlightFinal(); err != nil {
					return err
				}
				q.startTurnSideEffects(ctx, snapshot, state.Messages)
				onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
				return nil
			}
			if len(stopHooks.BlockingMessages) > 0 {
				state.Messages = append(state.Messages, stopHooks.BlockingMessages...)
				state.StopHookActive = true
				state.Transition = QueryTransitionStopHookBlocking
				if goalTurnSaveErr != nil {
					if err := blockFlightRuntime(); err != nil {
						return err
					}
					onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
					return nil
				}
				if goalTurnTracked {
					reached, current, err := goalTokenBudgetReached(snapshot.GoalRuntime)
					if err != nil {
						emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalLoadStopHook, nil, err)
						if blockErr := blockFlightRuntime(); blockErr != nil {
							return blockErr
						}
						q.startTurnSideEffects(ctx, snapshot, state.Messages)
						onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
						return nil
					}
					if reached {
						emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalBudgetReached, []any{current.Usage, current.TokenBudget}, nil)
						if blockErr := blockFlightRuntime(); blockErr != nil {
							return blockErr
						}
						q.startTurnSideEffects(ctx, snapshot, state.Messages)
						onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
						return nil
					}
				}
				onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
				continue
			}
			if goalTurnSaveErr != nil {
				if err := blockFlightRuntime(); err != nil {
					return err
				}
				q.startTurnSideEffects(ctx, snapshot, state.Messages)
				onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
				return nil
			}
			goalDecision := q.evaluateGoalContinuation(ctx, state, snapshot, postQueryMessages, budgetTracker, turnOutputTokens, turnCount, onEvent)
			if goalDecision.Handled {
				if !goalDecision.Continue {
					if err := commitFlightFinal(); err != nil {
						return err
					}
					q.startTurnSideEffects(ctx, snapshot, state.Messages)
				}
				onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
				if goalDecision.Continue {
					continue
				}
				return nil
			}
			// Agentic V2 Flight has already accepted this final response against
			// the current verified revision (or as a read-only task). A legacy
			// token budget is a ceiling, not a quota to consume, so it must not
			// manufacture extra provider rounds after the completion gate passes.
			if flightController != nil {
				if err := commitFlightFinal(); err != nil {
					return err
				}
				q.startTurnSideEffects(ctx, snapshot, state.Messages)
				onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
				return nil
			}
			decision := CheckTokenBudget(budgetTracker, snapshot.AgentID, snapshot.TokenBudget, turnOutputTokens)
			if decision.Continue {
				state.Messages = append(state.Messages, types.UserMessage(decision.NudgeMessage))
				state.MaxOutputTokensRecoveryCount = 0
				state.MaxOutputTokensOverride = 0
				state.Transition = QueryTransitionTokenBudgetContinuation
				onEvent(NewSystemWarningEvent(
					i18n.KeyRuntimeTokenBudgetContinuation,
					[]any{decision.ContinuationCount, decision.Percent, decision.TurnTokens, decision.Budget},
					nil,
					nil,
					turnCount,
				))
				continue
			}
			if decision.CompletionEvent != nil && decision.CompletionEvent.DiminishingReturns {
				onEvent(NewSystemWarningEvent(i18n.KeyRuntimeTokenBudgetDiminishing, []any{decision.CompletionEvent.Percent}, nil, nil, turnCount))
			}
			if err := commitFlightFinal(); err != nil {
				return err
			}
			q.startTurnSideEffects(ctx, snapshot, state.Messages)
			onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
			return nil
		}

		// Execute tools (concurrent-safe tools run in parallel)
		for i := range toolUses {
			toolUse := toolUses[i]
			onEvent(streamevent.Event{
				Type: streamevent.EventToolUse, ToolUse: &toolUse, TurnCount: turnCount,
				ActorID: actorID, ActorType: snapshot.AgentType, WorkUnitID: workUnitID,
			})
		}
		if err := flightController.openToolIntents(toolUses); err != nil {
			return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, err)
		}
		parentMessages := state.Messages[:len(state.Messages)-1]
		toolExecContext := q.bindToolExecutionContext(executioncontract.ToolExecutionContext{
			Messages:          parentMessages,
			AssistantMessage:  *assistantMsg,
			SessionID:         snapshot.SessionID,
			CacheLineageID:    snapshot.CacheLineageID,
			TurnID:            turnID,
			ActorID:           actorID,
			ActorType:         snapshot.AgentType,
			WorkUnitID:        workUnitID,
			RunID:             lineage.RunID,
			BatchID:           lineage.BatchID,
			ParentRunID:       lineage.ParentRunID,
			AgentPath:         lineage.AgentPath,
			SessionProjectDir: snapshot.SessionProjectDir,
			ProjectRoot:       snapshot.ProjectRoot,
			CWD:               snapshot.CWD,
			System:            snapshot.System,
			Model:             snapshot.Model,
		}, queryID, q.skillLoadedLedgerCapability(parentMessages), snapshot.SkillProjectGeneration)
		toolExecutionCtx := flightController.bindVerificationContext(ctx)
		if streamingExecutor != nil {
			for i := range toolUses {
				streamingExecutor.AddTool(toolUses[i], *assistantMsg)
			}
		}
		var toolExecution toolExecutionResults
		var toolErr error
		if streamingExecutor != nil {
			var events []StreamingToolEvent
			toolExecution, events, toolErr = streamingExecutor.RemainingResults(ctx)
			for _, event := range events {
				if event.Type != streamingToolEventResult || event.Result == nil {
					continue
				}
				result := *event.Result
				onEvent(streamevent.Event{
					Type: streamevent.EventToolResult, ToolResult: &result, TurnCount: turnCount,
					ActorID: actorID, ActorType: snapshot.AgentType, WorkUnitID: workUnitID,
				})
			}
		} else {
			toolExecution, toolErr = executeToolsConcurrentlyDetailed(toolExecutionCtx, q.registry, snapshot.HookRunner, snapshot.PermissionHandler, snapshot.SessionID, toolExecContext, toolUses, func(_ int, result types.ToolResultBlock) {
				onEvent(streamevent.Event{
					Type: streamevent.EventToolResult, ToolResult: &result, TurnCount: turnCount,
					ActorID: actorID, ActorType: snapshot.AgentType, WorkUnitID: workUnitID,
				})
			})
		}
		toolResults := toolExecution.Results
		reminders := toolExecution.Reminders
		onEvent(streamevent.Event{
			Type: streamevent.EventToolRoundMetrics, TurnCount: turnCount, TurnID: turnID,
			ActorID: actorID, ActorType: snapshot.AgentType, WorkUnitID: workUnitID,
			ToolRound: projectToolRoundMetrics(turnID, len(toolUses), toolExecution.Metrics),
		})
		for index := range toolExecution.HookSummaries {
			summary := toolExecution.HookSummaries[index]
			onEvent(streamevent.Event{
				Type: streamevent.EventHookSummary, TurnCount: turnCount, TurnID: turnID,
				ToolUseID: summary.ToolUseID,
				ActorID:   actorID, ActorType: snapshot.AgentType, WorkUnitID: workUnitID,
				HookSummary: &summary,
			})
		}
		usage = accountToolResultUsage(usage, toolResults, turnCount, onEvent)

		// Infrastructure error (e.g. context cancelled) — preserve tool_result
		// pairs for any transcript-visible tool_use before aborting.
		if toolErr != nil {
			if flightController != nil {
				decision, flightErr := flightController.blockRuntime()
				if flightErr != nil {
					return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, flightErr)
				}
				onEvent(newAgenticFlightDispositionEvent(flightController, decision, turnCount))
			}
			validResults := validToolResults(toolResults)
			if len(validResults) > 0 {
				state.Messages = append(state.Messages, types.ToolResultMessage(validResults...))
			}
			state.Messages = appendMissingToolResults(state.Messages, toolErr.Error(), types.ToolOutcomeFailed, toolExecContext, onEvent, turnCount)
			if isUserInterrupt(toolErr) {
				emitUserInterruption(onEvent, turnCount, "interrupt")
			}
			return i18n.WrapError(i18n.KeyLoopQueryToolExecutionFailed, toolErr)
		}
		if ctx.Err() != nil {
			if flightController != nil {
				decision, flightErr := flightController.blockRuntime()
				if flightErr != nil {
					return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, flightErr)
				}
				onEvent(newAgenticFlightDispositionEvent(flightController, decision, turnCount))
			}
			validResults := validToolResults(toolResults)
			if len(validResults) > 0 {
				state.Messages = append(state.Messages, types.ToolResultMessage(validResults...))
			}
			state.Messages = appendMissingToolResults(state.Messages, ctx.Err().Error(), types.ToolOutcomeCancelled, toolExecContext, onEvent, turnCount)
			emitUserInterruption(onEvent, turnCount, "interrupt")
			return i18n.WrapError(i18n.KeyLoopQueryToolExecutionInterrupted, ctx.Err())
		}
		// Observe flight state now, but do not return its error until the model-
		// visible tool results have been committed below. A reducer failure must
		// never leave the already-persisted assistant tool_use without its
		// protocol-mandated matching tool_result.
		flightErr := flightController.observeToolRound(toolUses, toolResults)
		investigationTracker.observe(toolUses, toolResults)

		// Persist oversized tool results to disk, keeping only a preview
		if q.resultStore != nil {
			for i := range toolResults {
				if !hasToolResultPersistenceMetadata(toolResults[i].Metadata) {
					continue
				}
				processed, err := q.resultStore.ProcessResultForTool(toolResults[i], toolUses[i].Name)
				if err != nil {
					onEvent(streamevent.Event{
						Type: streamevent.EventError, Text: err.Error(), TurnCount: turnCount,
						ToolUseID: toolResults[i].ToolUseID,
						Error:     &types.APIError{Type: "tool_result_persistence_error", Message: err.Error()},
						Metadata: map[string]any{
							"stage":   "tool_result_persistence",
							"outcome": string(types.ToolOutcomeFailed),
						},
					})
				}
				toolResults[i] = processed
			}
		}
		// NewMessages is an in-process attachment side channel. Append those
		// messages below, but do not persist a second JSON copy inside the tool
		// result: nested copies have no manifest provenance refs after restart.
		historyToolResults := append([]types.ToolResultBlock(nil), toolResults...)
		for index := range historyToolResults {
			historyToolResults[index].NewMessages = nil
		}
		toolMsg := types.ToolResultMessage(historyToolResults...)
		nextMessages := append(state.Messages, toolMsg)
		var records []compact.ContentReplacementRecord
		if q.resultStore != nil && q.contentReplacementState != nil {
			var replacementErrs []error
			nextMessages, records, replacementErrs = compact.ApplyToolResultBudget(nextMessages, q.contentReplacementState, q.resultStore, nil)
			for _, err := range replacementErrs {
				onEvent(streamevent.Event{Type: streamevent.EventError, Text: err.Error(), TurnCount: turnCount})
			}
		}
		state.Messages = nextMessages
		if len(records) > 0 {
			state.Messages = q.installContentReplacementRecords(state.Messages, records)
		}
		if flightErr != nil {
			return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, flightErr)
		}

		q.learnLoadedTools(toolResults)
		goalStopAfterTool := goalTurnSaveErr != nil
		if !goalStopAfterTool && goalTurnTracked {
			reached, current, err := goalTokenBudgetReached(snapshot.GoalRuntime)
			if err != nil {
				emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalLoadToolExecution, nil, err)
				goalStopAfterTool = true
			}
			if reached {
				emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalBudgetReached, []any{current.Usage, current.TokenBudget}, nil)
				goalStopAfterTool = true
			}
		}
		if restartMessage, restart := planModeContextRestart(toolResults); restart {
			// Clear the assistant tool_use together with the old transcript, then
			// restart from the approved-plan prose. This avoids an orphaned
			// tool_result while matching the TS clear-context handoff.
			state.Messages = q.installVisibleHistory([]types.Message{types.UserMessage(restartMessage)})
			state.Transition = QueryTransitionPlanModeContextRestart
			if goalStopAfterTool {
				onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
				q.startTurnSideEffects(ctx, snapshot, state.Messages)
				return nil
			}
			onEvent(streamevent.Event{
				Type: streamevent.EventProgress, TurnCount: turnCount,
				Progress: &streamevent.ProgressEvent{Stage: streamevent.ProgressStageLLMWaitingAfterTools},
			})
			onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
			continue
		}

		if err := q.appendPostToolAttachments(ctx, state, snapshot, toolUses, toolResults, reminders); err != nil {
			return err
		}
		commitVisibleReadEvidenceReceipts(state.Messages, toolResults)
		if err := q.commitVisibleSkillExecutionReceipts(state.Messages, toolUses, toolResults); err != nil {
			return err
		}
		if investigationTracker.takeNudge() {
			state.Messages = append(state.Messages, investigationMessage(q))
		}
		if investigationTracker.takeVerificationConvergenceNudge() {
			state.Messages = append(state.Messages, verificationConvergenceMessage(q))
		}
		if flightController != nil && flightController.repeatedFailureTriggered() {
			decision, flightErr := flightController.blockRepeatedFailure()
			if flightErr != nil {
				return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, flightErr)
			}
			onEvent(newAgenticFlightDispositionEvent(flightController, decision, turnCount))
			q.startTurnSideEffects(ctx, snapshot, state.Messages)
			onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
			return i18n.NewError(i18n.KeyLoopQueryFlightRepeatedFailure)
		}

		if goalStopAfterTool {
			if flightController != nil {
				decision, flightErr := flightController.blockRuntime()
				if flightErr != nil {
					return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, flightErr)
				}
				onEvent(newAgenticFlightDispositionEvent(flightController, decision, turnCount))
			}
			q.startTurnSideEffects(ctx, snapshot, state.Messages)
			onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
			return nil
		}
		if toolExecution.PreventContinuation {
			if flightController != nil {
				decision, flightErr := flightController.blockRuntime()
				if flightErr != nil {
					return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, flightErr)
				}
				onEvent(newAgenticFlightDispositionEvent(flightController, decision, turnCount))
			}
			onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})
			return nil
		}
		onEvent(streamevent.Event{Type: streamevent.EventTurnEnd, Usage: usage, TurnCount: turnCount})

		// Record activity time so microcompact can detect idle sessions.
		q.microcompactCfg.LastActivity = time.Now()

		// Bound the next provider request without rewriting the transcript. A
		// semantic compaction can be requested explicitly before the retry.
		if err := enforceMessageHistoryLimit(state.Messages); err != nil {
			return err
		}
		onEvent(streamevent.Event{
			Type: streamevent.EventProgress, TurnCount: turnCount,
			Progress: &streamevent.ProgressEvent{Stage: streamevent.ProgressStageLLMWaitingAfterTools},
		})
	}

	maxTurnsErr := state.maxTurnsExceeded(snapshot.MaxTurns)
	if flightController != nil {
		decision, flightErr := flightController.blockRuntime()
		if flightErr != nil {
			return i18n.WrapInternalError(i18n.KeyLoopQueryFlightStateInvalid, flightErr)
		}
		emitTurnEvent(maxTurnsErr.TurnCount, newAgenticFlightDispositionEvent(flightController, decision, maxTurnsErr.TurnCount))
	}
	emitTurnEvent(maxTurnsErr.TurnCount, newMaxTurnsReachedEvent(maxTurnsErr.MaxTurns, maxTurnsErr.TurnCount))
	return maxTurnsErr
}

type visibleReadEvidenceCommitter interface {
	CommitVisibleReadEvidence(visibleContent string) bool
}

// commitVisibleReadEvidenceReceipts is deliberately after result budgeting
// and history append. A read-only tool executed earlier in the same assistant
// message therefore cannot authorize a sibling mutation the model has not yet
// observed, and a persisted/replaced/truncated result cannot publish broader
// coverage than its exact visible envelope.
func commitVisibleReadEvidenceReceipts(messages []types.Message, results []types.ToolResultBlock) {
	for _, expected := range results {
		carrier, ok := expected.Data.(visibleReadEvidenceCommitter)
		if !ok || expected.IsError || strings.TrimSpace(expected.ToolUseID) == "" {
			continue
		}
		for _, message := range messages {
			if message.Role != types.RoleUser || message.IsInternalRuntimeMessage() {
				continue
			}
			committed := false
			for _, block := range message.Content {
				visible, ok := block.(types.ToolResultBlock)
				if !ok || visible.ToolUseID != expected.ToolUseID || visible.IsError ||
					visible.Content != expected.Content {
					continue
				}
				committed = carrier.CommitVisibleReadEvidence(visible.Content)
				break
			}
			if committed {
				break
			}
		}
	}
}

func hasToolResultPersistenceMetadata(metadata map[string]string) bool {
	for _, key := range []string{
		"maxResultSizeChars",
		"max_result_size_chars",
		"persistenceThreshold",
		"persistThreshold",
		"toolResultPersistenceThreshold",
	} {
		if _, ok := metadata[key]; ok {
			return true
		}
	}
	return false
}

func collapseMaxOutputTokensRecoveryMessages(messages []types.Message) []types.Message {
	last := -1
	count := 0
	for i, msg := range messages {
		if isMaxOutputTokensRecoveryMessage(msg) {
			last = i
			count++
		}
	}
	if count <= 1 {
		return messages
	}
	out := make([]types.Message, 0, len(messages)-count+1)
	for i, msg := range messages {
		if isMaxOutputTokensRecoveryMessage(msg) && i != last {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func isMaxOutputTokensRecoveryMessage(msg types.Message) bool {
	return msg.Role == types.RoleUser && msg.InternalKind == types.InternalMessageKindOutputTokenRecovery &&
		msg.HasInternalControlProvenance()
}

// parseToolInputJSON parses the accumulated tool input JSON string into a map.
// Some OpenAI-compatible endpoints (vLLM, proxies) emit duplicate complete JSON
// objects after the incremental fragments, producing e.g.
// `{"msg":"hi"}{"msg":"hi"}{"msg":"hi"}`. Standard json.Unmarshal rejects this.
// We use json.Decoder to extract only the first valid JSON object.
func parseToolInputJSON(raw string) (map[string]any, error) {
	input := make(map[string]any)
	if raw == "" {
		return input, nil
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	if err := dec.Decode(&input); err != nil {
		return nil, err
	}
	return input, nil
}

// blockState tracks the accumulation state for a single content block
// Phase 3: Added separate thinking accumulator to prevent mixing thinking with text
type blockState struct {
	blockType      types.ContentType
	toolID         string
	toolName       string
	toolType       types.ToolDefinitionType
	signature      string // for ThinkingBlock round-trip
	signatureKind  types.ThinkingSignatureKind
	signatureModel string
	providerItemID string
	providerStatus string
	thinkingKind   types.ThinkingKind
	text           strings.Builder // For text content
	thinking       strings.Builder // NEW: For thinking content (separate)
	toolInput      strings.Builder
	rawToolInput   strings.Builder
	// lastReportedToolInputBytes throttles content-free progress updates while
	// preserving an exact final count. It never stores or projects input text.
	lastReportedToolInputBytes int
	toolInputProgressReported  bool
	toolUseID                  string
	rawJSON                    json.RawMessage
}

// processStream reads stream events and builds the assistant message.
// It uses per-block state tracking to correctly handle interleaved
// tool_calls from OpenAI-compatible providers.
//
// EventMessageStop is the response transaction's commit record. Until it is
// observed, accumulated blocks are provisional: an EventError or a silently
// closed channel returns a nil message with *PartialStreamError. In particular,
// a closed content block is not itself evidence that the whole tool batch was
// committed.
func (q *QueryLoop) processStream(ctx context.Context, stream <-chan types.StreamEvent, turnCount int, onEvent func(streamevent.Event), streamingExecutors ...*StreamingToolExecutor) (*types.Message, *types.Usage, *types.StopReason, error) {
	type indexedContentBlock struct {
		index int
		block types.ContentBlock
	}
	var completedBlocks []indexedContentBlock
	appendContentBlock := func(index int, block types.ContentBlock) {
		completedBlocks = append(completedBlocks, indexedContentBlock{index: index, block: block})
	}
	var usage *types.Usage
	var stopReason *types.StopReason
	var providerContinuation *types.ProviderContinuation
	committed := false
	blocks := make(map[int]*blockState) // per-block accumulator keyed by index
	emitToolInputProgress := func(bs *blockState, force bool) {
		if bs == nil || bs.toolName == "" {
			return
		}
		inputBytes := bs.toolInput.Len()
		if bs.toolType == types.ToolDefinitionTypeCustom {
			inputBytes = bs.rawToolInput.Len()
		}
		previous := bs.lastReportedToolInputBytes
		if !force && bs.toolInputProgressReported && inputBytes > 0 && previous > 0 && inputBytes/1024 == previous/1024 {
			return
		}
		if bs.toolInputProgressReported && inputBytes == previous {
			return
		}
		bs.lastReportedToolInputBytes = inputBytes
		bs.toolInputProgressReported = true
		onEvent(streamevent.Event{
			Type: streamevent.EventProgress, TurnCount: turnCount,
			Progress: &streamevent.ProgressEvent{
				Stage: streamevent.ProgressStageLLMToolInput,
				Metadata: map[string]any{
					"tool_name":        bs.toolName,
					"tool_input_bytes": inputBytes,
				},
			},
		})
	}
	// Streaming executors are intentionally not started while content blocks are
	// still arriving. The complete batch is validated by runLoop before tools are
	// added, so a later duplicate ID cannot race an earlier tool side effect.
	_ = streamingExecutors

	for event := range stream {
		if ctx.Err() != nil && !committed {
			return nil, usage, stopReason, ctx.Err()
		}
		// Providers may attach usage to terminal error/stop events as well as
		// message_start/message_delta. Capture it before branching so failed
		// attempts remain billable.
		if event.Usage != nil {
			if usage == nil {
				usage = &types.Usage{}
			}
			mergeUsage(usage, event.Usage)
		}
		// MessageStop commits the exact response prefix observed before it.
		// Providers should close immediately afterwards, but if a transport emits
		// a trailing error or duplicate event, it cannot roll back an already
		// committed response. Usage on such events was still merged above.
		if committed {
			continue
		}

		switch event.Type {
		case types.EventContentBlockStart:
			bs := &blockState{}
			if event.ContentBlock != nil {
				bs.blockType = event.ContentBlock.Type
				bs.toolUseID = event.ContentBlock.ToolUseID
				bs.rawJSON = append(json.RawMessage(nil), event.ContentBlock.RawJSON...)
				if event.ContentBlock.Type == types.ContentTypeToolUse {
					bs.toolID = event.ContentBlock.ID
					bs.toolName = event.ContentBlock.Name
					bs.toolType = event.ContentBlock.ToolType
					bs.providerItemID = event.ContentBlock.ProviderItemID
					bs.providerStatus = event.ContentBlock.ProviderStatus
					// A tool-use block is the earliest honest boundary at which the
					// runtime knows that the model is generating tool input. Keep the
					// event content-free: partial JSON can be large and may contain
					// sensitive commands or paths.
					emitToolInputProgress(bs, false)
				}
				if event.ContentBlock.Type == types.ContentTypeThinking {
					bs.signature = event.ContentBlock.Signature
					bs.signatureKind = event.ContentBlock.SignatureKind
					bs.signatureModel = event.ContentBlock.SignatureModel
					bs.providerItemID = event.ContentBlock.ID
					bs.providerStatus = event.ContentBlock.ProviderStatus
					bs.thinkingKind = event.ContentBlock.ThinkingKind
				}
			}
			blocks[event.Index] = bs

		case types.EventContentBlockDelta:
			if event.Delta != nil && (event.Delta.Text != "" || event.Delta.Thinking != "" || event.Delta.PartialJSON != "" || event.Delta.PartialText != "") {
				notifyFirstTokenObserver(ctx)
			}
			bs := blocks[event.Index]
			if bs == nil {
				bs = &blockState{}
				blocks[event.Index] = bs
			}
			if event.Delta != nil {
				switch event.Delta.Type {
				case "text_delta":
					bs.text.WriteString(event.Delta.Text)
					onEvent(streamevent.Event{Type: streamevent.EventText, Text: event.Delta.Text, TurnCount: turnCount})
				case "thinking_delta":
					// Phase 3: Fix - use separate thinking accumulator instead of bs.text
					bs.thinking.WriteString(event.Delta.Thinking)
					if event.Delta.ThinkingKind == types.ThinkingKindRaw || bs.thinkingKind == "" {
						bs.thinkingKind = event.Delta.ThinkingKind
					}
					onEvent(streamevent.Event{Type: streamevent.EventThinking, Text: event.Delta.Thinking, TurnCount: turnCount})
				case "signature_delta":
					// Opaque provider continuation state is retained solely for
					// provider replay. It is never projected as a runtime event.
					bs.signature = event.Delta.Signature
					bs.signatureKind = event.Delta.SignatureKind
					bs.signatureModel = event.Delta.SignatureModel
					if event.Delta.ID != "" {
						bs.providerItemID = event.Delta.ID
					}
					if event.Delta.ProviderStatus != "" {
						bs.providerStatus = event.Delta.ProviderStatus
					}
				case "thinking_state_final":
					if event.Delta.ID != "" {
						bs.providerItemID = event.Delta.ID
					}
					if event.Delta.ProviderStatus != "" {
						bs.providerStatus = event.Delta.ProviderStatus
					}
					if event.Delta.Signature != "" {
						bs.signature = event.Delta.Signature
						bs.signatureKind = event.Delta.SignatureKind
						bs.signatureModel = event.Delta.SignatureModel
					}
				case "input_json_delta":
					bs.toolInput.WriteString(event.Delta.PartialJSON)
					emitToolInputProgress(bs, false)
				case "input_text_delta":
					bs.toolType = types.ToolDefinitionTypeCustom
					bs.rawToolInput.WriteString(event.Delta.PartialText)
					emitToolInputProgress(bs, false)
				case "tool_state_final":
					if event.Delta.ID != "" {
						bs.toolID = event.Delta.ID
					}
					if event.Delta.Name != "" {
						bs.toolName = event.Delta.Name
					}
					if event.Delta.ToolType == types.ToolDefinitionTypeCustom || bs.toolType == types.ToolDefinitionTypeCustom {
						bs.toolType = types.ToolDefinitionTypeCustom
						bs.rawToolInput.Reset()
						bs.rawToolInput.WriteString(event.Delta.PartialText)
					} else {
						bs.toolInput.Reset()
						bs.toolInput.WriteString(event.Delta.PartialJSON)
					}
					emitToolInputProgress(bs, true)
				}
			}

		case types.EventContentBlockStop:
			bs := blocks[event.Index]
			if bs == nil {
				continue
			}
			switch {
			case bs.toolName != "":
				emitToolInputProgress(bs, true)
				toolUse, err := q.toolUseFromBlockState(bs)
				if err != nil {
					onEvent(NewSystemWarningEvent(
						i18n.KeyRuntimeToolInputJSONFailed,
						[]any{bs.toolName},
						err,
						nil,
						turnCount,
					))
					appendContentBlock(event.Index, invalidToolUseFromBlockState(bs))
					delete(blocks, event.Index)
					continue
				}
				appendContentBlock(event.Index, toolUse)
			case bs.blockType == types.ContentTypeThinking:
				// Phase 3: Use thinking accumulator for thinking blocks
				if bs.thinking.Len() > 0 || bs.signature != "" {
					appendContentBlock(event.Index, types.ThinkingBlock{
						Type:           types.ContentTypeThinking,
						Thinking:       bs.thinking.String(),
						Signature:      bs.signature,
						SignatureKind:  bs.signatureKind,
						SignatureModel: bs.signatureModel,
						ProviderItemID: bs.providerItemID,
						ProviderStatus: bs.providerStatus,
						Kind:           bs.thinkingKind,
					})
				}
			case bs.blockType != "" && bs.blockType != types.ContentTypeText:
				appendContentBlock(event.Index, serverToolUnknownBlock(bs))
			default:
				if bs.text.Len() > 0 {
					appendContentBlock(event.Index, types.TextBlock{
						Type: types.ContentTypeText,
						Text: bs.text.String(),
					})
				}
			}
			delete(blocks, event.Index)

		case types.EventMessageDelta:
			if event.StopReason != nil {
				stopReason = event.StopReason
			}

		case types.EventError:
			if event.Error != nil {
				return nil, usage, stopReason, &PartialStreamError{
					Cause:         event.Error,
					PartialBlocks: len(completedBlocks) + len(blocks),
					OpenBlocks:    len(blocks),
				}
			}

		case types.EventMessageStop:
			committed = true
			providerContinuation = event.ProviderContinuation.Clone()
			// Capture ResponseID for Responses API chaining (previous_response_id)
			if event.ResponseID != "" {
				q.lastResponseID = event.ResponseID
				q.lastEnvelopeFingerprint = q.currentEnvelopeFingerprint
				// A successful full-history repair establishes a fresh, valid
				// server parent. Re-enable incremental chaining for the next turn.
				q.disableResponseChain = false
			}
		}
	}

	if !committed {
		return nil, usage, stopReason, &PartialStreamError{
			Cause:         i18n.NewError(i18n.KeyLoopStreamClosedBeforeCommit),
			PartialBlocks: len(completedBlocks) + len(blocks),
			OpenBlocks:    len(blocks),
		}
	}

	// A provider commit authorizes flushing any remaining per-block accumulator.
	// Without the commit, the early return above makes the whole batch inert.
	remainingKeys := make([]int, 0, len(blocks))
	for k := range blocks {
		remainingKeys = append(remainingKeys, k)
	}
	sort.Ints(remainingKeys)
	for _, k := range remainingKeys {
		bs := blocks[k]
		if bs.toolName != "" {
			toolUse, err := q.toolUseFromBlockState(bs)
			if err != nil {
				onEvent(NewSystemWarningEvent(
					i18n.KeyRuntimeToolInputJSONFlushFailed,
					[]any{bs.toolName},
					err,
					nil,
					turnCount,
				))
				appendContentBlock(k, invalidToolUseFromBlockState(bs))
				continue
			}
			appendContentBlock(k, toolUse)
		} else if bs.blockType == types.ContentTypeThinking && (bs.thinking.Len() > 0 || bs.signature != "") {
			// Phase 3: Use thinking accumulator for thinking blocks in flush
			appendContentBlock(k, types.ThinkingBlock{
				Type:           types.ContentTypeThinking,
				Thinking:       bs.thinking.String(),
				Signature:      bs.signature,
				SignatureKind:  bs.signatureKind,
				SignatureModel: bs.signatureModel,
				ProviderItemID: bs.providerItemID,
				ProviderStatus: bs.providerStatus,
				Kind:           bs.thinkingKind,
			})
		} else if bs.blockType != "" && bs.blockType != types.ContentTypeText {
			appendContentBlock(k, serverToolUnknownBlock(bs))
		} else if bs.text.Len() > 0 {
			appendContentBlock(k, types.TextBlock{
				Type: types.ContentTypeText, Text: bs.text.String(),
			})
		}
	}

	sort.SliceStable(completedBlocks, func(left, right int) bool {
		return completedBlocks[left].index < completedBlocks[right].index
	})
	contentBlocks := make([]types.ContentBlock, 0, len(completedBlocks))
	for _, completed := range completedBlocks {
		contentBlocks = append(contentBlocks, completed.block)
	}
	msg := &types.Message{
		Role:    types.RoleAssistant,
		Content: contentBlocks,
	}
	msg.AttachProviderContinuation(providerContinuation)

	return msg, usage, stopReason, nil
}

func invalidToolUseFromBlockState(state *blockState) types.InvalidToolUseBlock {
	if state == nil {
		return types.InvalidToolUseBlock{Type: types.ContentTypeInvalidToolUse, FailureKind: types.ToolInputFailureInvalidJSON, Recoverable: true}
	}
	raw := state.toolInput.String()
	failureKind := types.ToolInputFailureInvalidJSON
	if state.toolType == types.ToolDefinitionTypeCustom {
		raw = state.rawToolInput.String()
		failureKind = types.ToolInputFailureCustomDecode
	}
	digest := sha256.Sum256([]byte(raw))
	preview := raw
	truncated := len(preview) > invalidToolInputPreviewBytes
	if truncated {
		preview = preview[:invalidToolInputPreviewBytes]
		for len(preview) > 0 && !utf8.ValidString(preview) {
			preview = preview[:len(preview)-1]
		}
	}
	return types.InvalidToolUseBlock{
		Type:              types.ContentTypeInvalidToolUse,
		ID:                state.toolID,
		Name:              state.toolName,
		ToolType:          state.toolType,
		RawInput:          preview,
		InputBytes:        len(raw),
		InputDigest:       fmt.Sprintf("sha256:%x", digest),
		RawInputTruncated: truncated,
		ProviderItemID:    state.providerItemID,
		ProviderStatus:    state.providerStatus,
		FailureKind:       failureKind,
		Recoverable:       true,
	}
}

func (q *QueryLoop) toolUseFromBlockState(state *blockState) (types.ToolUseBlock, error) {
	if state == nil {
		return types.ToolUseBlock{}, i18n.NewError(i18n.KeyRuntimeToolSkippedMalformed, "")
	}
	if state.toolType == types.ToolDefinitionTypeCustom {
		if q == nil || q.registry == nil {
			return types.ToolUseBlock{}, i18n.NewError(i18n.KeyRuntimeToolSkippedMalformed, state.toolName)
		}
		decoder, ok := q.registry.Get(state.toolName).(types.CustomToolInputDecoder)
		if !ok {
			return types.ToolUseBlock{}, i18n.NewError(i18n.KeyRuntimeToolSkippedMalformed, state.toolName)
		}
		raw := state.rawToolInput.String()
		input, err := decoder.DecodeCustomToolInput(raw)
		if err != nil {
			return types.ToolUseBlock{}, err
		}
		return types.ToolUseBlock{
			Type: types.ContentTypeToolUse, ID: state.toolID, Name: state.toolName,
			Input: input, ToolType: types.ToolDefinitionTypeCustom, RawInput: raw,
		}, nil
	}
	input, err := parseToolInputJSON(state.toolInput.String())
	if err != nil {
		return types.ToolUseBlock{}, err
	}
	return types.ToolUseBlock{
		Type: types.ContentTypeToolUse, ID: state.toolID, Name: state.toolName, Input: input,
	}, nil
}

func serverToolUnknownBlock(state *blockState) types.UnknownBlock {
	if state == nil {
		return types.UnknownBlock{}
	}
	raw := append(json.RawMessage(nil), state.rawJSON...)
	if state.blockType == types.ContentTypeServerToolUse && state.toolInput.Len() > 0 {
		var envelope map[string]any
		var input any
		if json.Unmarshal(raw, &envelope) == nil && json.Unmarshal([]byte(state.toolInput.String()), &input) == nil {
			envelope["input"] = input
			if encoded, err := json.Marshal(envelope); err == nil {
				raw = encoded
			}
		}
	}
	return types.UnknownBlock{Type: state.blockType, Raw: raw}
}

func mergeUsage(dst, src *types.Usage) {
	if dst == nil || src == nil {
		return
	}
	if src.InputTokens > 0 {
		dst.InputTokens = src.InputTokens
	}
	if src.OutputTokens > 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if src.CacheCreationInputTokens > 0 {
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	}
	if src.CacheReadInputTokens > 0 {
		dst.CacheReadInputTokens = src.CacheReadInputTokens
	}
	if src.ServerToolUse.WebSearchRequests > 0 {
		dst.ServerToolUse.WebSearchRequests = src.ServerToolUse.WebSearchRequests
	}
	if src.ServerToolUse.WebFetchRequests > 0 {
		dst.ServerToolUse.WebFetchRequests = src.ServerToolUse.WebFetchRequests
	}
}

func mergeToolResultUsage(outer *types.Usage, results []types.ToolResultBlock) *types.Usage {
	for _, result := range results {
		if result.Usage == nil {
			continue
		}
		if outer == nil {
			outer = &types.Usage{}
		}
		outer.InputTokens += result.Usage.InputTokens
		outer.OutputTokens += result.Usage.OutputTokens
		outer.CacheCreationInputTokens += result.Usage.CacheCreationInputTokens
		outer.CacheReadInputTokens += result.Usage.CacheReadInputTokens
		outer.ServerToolUse.WebSearchRequests += result.Usage.ServerToolUse.WebSearchRequests
		outer.ServerToolUse.WebFetchRequests += result.Usage.ServerToolUse.WebFetchRequests
	}
	return outer
}

func accountToolResultUsage(outer *types.Usage, results []types.ToolResultBlock, turnCount int, onEvent func(streamevent.Event)) *types.Usage {
	for _, result := range results {
		if result.Usage == nil {
			continue
		}
		providerName := result.Metadata["usage.provider"]
		model := result.Metadata["usage.model"]
		if onEvent != nil && (providerName != "" || model != "") {
			usage := *result.Usage
			metadata := map[string]any{
				"kind":     "nested_tool",
				"provider": providerName,
				"model":    model,
			}
			if result.ToolUseID != "" {
				metadata["usage_id"] = "nested_tool:" + result.ToolUseID
			}
			onEvent(streamevent.Event{
				Type:      streamevent.EventProviderUsage,
				Usage:     &usage,
				TurnCount: turnCount,
				Metadata:  metadata,
			})
			continue
		}
		outer = mergeToolResultUsage(outer, []types.ToolResultBlock{result})
	}
	return outer
}
