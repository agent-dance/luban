package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// SkillLoadedLedgerEntry identifies one exact invocation payload already
// visible in the current context epoch. The entry deliberately omits the
// epoch: SkillCatalogRuntimeState binds the entire loaded ledger atomically.
type SkillLoadedLedgerEntry struct {
	ContentDigest skills.SkillDigest
	PayloadDigest skills.InvocationPayloadDigest
}

// Validate rejects incomplete loaded-body evidence.
func (entry SkillLoadedLedgerEntry) Validate() error {
	if err := entry.ContentDigest.Validate(); err != nil {
		return fmt.Errorf("skill loaded ledger content digest: %w", err)
	}
	if err := entry.PayloadDigest.Validate(); err != nil {
		return fmt.Errorf("skill loaded ledger payload digest: %w", err)
	}
	return nil
}

// SkillLoadedLedgerState is the loop-side projection consumed by SkillTool's
// resolver. A zero LoadedContextEpoch means that no body is proven visible for
// the requested ID, while ContextEpoch still lets a successful execution emit
// a pending receipt for the current visible context.
type SkillLoadedLedgerState struct {
	ContextEpoch       uint64
	LoadedContextEpoch uint64
	ContentDigest      skills.SkillDigest
	PayloadDigest      skills.InvocationPayloadDigest
}

// SkillCatalogRuntimeState is the complete per-query-loop model projection
// ledger. Runtime composition may persist and restore it only together with
// visible-history evidence for the same context epoch.
type SkillCatalogRuntimeState struct {
	ContextEpoch  uint64
	Cursor        SkillCatalogCursor
	LoadedDigests map[skills.SkillID]SkillLoadedLedgerEntry
}

// Clone returns a defensive copy suitable for crossing runtime boundaries.
func (state SkillCatalogRuntimeState) Clone() SkillCatalogRuntimeState {
	clone := SkillCatalogRuntimeState{
		ContextEpoch: state.ContextEpoch,
		Cursor:       state.Cursor.Clone(),
	}
	if len(state.LoadedDigests) > 0 {
		clone.LoadedDigests = make(map[skills.SkillID]SkillLoadedLedgerEntry, len(state.LoadedDigests))
		for id, entry := range state.LoadedDigests {
			clone.LoadedDigests[id] = entry
		}
	}
	return clone
}

// Validate binds the coordinator cursor and every loaded digest to one
// non-zero visible context epoch.
func (state SkillCatalogRuntimeState) Validate() error {
	if state.ContextEpoch == 0 {
		return errors.New("skill catalog runtime context epoch is zero")
	}
	if !state.Cursor.Empty() {
		if err := state.Cursor.Validate(); err != nil {
			return fmt.Errorf("skill catalog runtime cursor: %w", err)
		}
		if state.Cursor.ContextEpoch != skillCatalogContextEpoch(state.ContextEpoch) {
			return errors.New("skill catalog runtime cursor belongs to another context epoch")
		}
	}
	for id, entry := range state.LoadedDigests {
		if err := id.Validate(); err != nil {
			return fmt.Errorf("skill loaded ledger ID: %w", err)
		}
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("skill loaded ledger %s: %w", id, err)
		}
	}
	return nil
}

func skillCatalogContextEpoch(epoch uint64) SkillCatalogContextEpoch {
	return SkillCatalogContextEpoch(fmt.Sprintf("context-%d", epoch))
}

// QueryTransition names the cross-turn transition that produced the current
// state. Most transitions are structural placeholders for follow-up parity
// tasks; task_01 only wires the next-turn and max-turn paths.
type QueryTransition string

const (
	QueryTransitionNextTurn                QueryTransition = "next_turn"
	QueryTransitionReactiveCompactRetry    QueryTransition = "reactive_compact_retry"
	QueryTransitionMaxOutputTokensEscalate QueryTransition = "max_output_tokens_escalate"
	QueryTransitionMaxOutputTokensRecovery QueryTransition = "max_output_tokens_recovery"
	QueryTransitionStopHookBlocking        QueryTransition = "stop_hook_blocking"
	QueryTransitionGoalContinuation        QueryTransition = "goal_continuation"
	QueryTransitionTokenBudgetContinuation QueryTransition = "token_budget_continuation"
	QueryTransitionFlightVerification      QueryTransition = "flight_verification"
	QueryTransitionPlanModeContextRestart  QueryTransition = "plan_mode_context_restart"
)

// QuerySource identifies loop entrypoints that must not recursively run
// automatic compaction while they are themselves producing compact context.
type QuerySource string

const (
	QuerySourceMain    QuerySource = ""
	QuerySourceCompact QuerySource = "compact"
)

// QueryConfigSnapshot freezes query-loop configuration at Run entry. The loop
// reads from this snapshot instead of q.config so in-flight config mutations do
// not affect later turns within the same Run.
type QueryConfigSnapshot struct {
	MaxTurns               int
	System                 string
	UserContext            prompt.UserContext
	SystemContext          prompt.SystemContext
	VisibleTools           registry.VisibleToolSnapshot
	ToolPromptConfig       prompt.Config
	GeneratedToolPrompt    bool
	GoalRuntime            GoalRuntime
	GoalEvaluator          GoalEvaluator
	SystemBlocks           []prompt.SystemPromptBlock
	Model                  string
	MaxTokens              int
	MaxContextTokens       int
	MaxOutputTokens        int
	TokenBudget            int
	TaskBudget             int
	HookRunner             *hooks.Runner
	PostSamplingRunner     PostSamplingRunner
	TurnSideEffects        TurnSideEffects
	BareMode               bool
	SessionID              string
	CacheLineageID         string
	SessionProjectDir      string
	ProjectRoot            string
	CWD                    string
	AgentID                string
	AgentType              string
	AgentTranscriptPath    string
	ReasoningEffort        string
	ServiceTier            provider.ServiceTier
	PinnedModel            bool
	StreamingToolExecution bool
	PermissionHandler      permission.PermissionHandler
	SkillManager           *skills.Manager
	SkillProjectGeneration skills.ProjectSourceGeneration
	CommandQueue           CommandQueue
	MemoryPrefetcher       MemoryPrefetcher
	SkillPrefetcher        SkillPrefetcher
	ToolRefresher          ToolRefresher
	QueryScope             QueryScope
	Thinking               *provider.ThinkingConfig
	TeammateContext        TeammateContextProvider
	PostCompactCleanup     func(context.Context) error
	QuerySource            QuerySource
}

func newQueryConfigSnapshot(cfg Config, thinking *provider.ThinkingConfig) QueryConfigSnapshot {
	var thinkingSnapshot *provider.ThinkingConfig
	if thinking != nil {
		v := *thinking
		thinkingSnapshot = &v
	}
	systemBlocks := append([]prompt.SystemPromptBlock(nil), cfg.SystemBlocks...)
	goalEvaluator := bindGoalEvaluatorModel(cfg.GoalEvaluator, cfg.Model)
	cacheLineageID := strings.TrimSpace(cfg.CacheLineageID)
	if cacheLineageID == "" {
		cacheLineageID = strings.TrimSpace(cfg.SessionID)
	}

	return QueryConfigSnapshot{
		MaxTurns:               cfg.MaxTurns,
		System:                 cfg.System,
		UserContext:            cfg.UserContext,
		SystemContext:          cfg.SystemContext,
		VisibleTools:           cfg.VisibleTools,
		ToolPromptConfig:       cfg.ToolPromptConfig,
		GeneratedToolPrompt:    cfg.GeneratedToolPrompt,
		GoalRuntime:            cfg.GoalRuntime,
		GoalEvaluator:          goalEvaluator,
		SystemBlocks:           systemBlocks,
		Model:                  cfg.Model,
		MaxTokens:              cfg.MaxTokens,
		MaxContextTokens:       cfg.MaxContextTokens,
		MaxOutputTokens:        cfg.MaxOutputTokens,
		TokenBudget:            cfg.TokenBudget,
		TaskBudget:             cfg.TaskBudget,
		HookRunner:             cfg.HookRunner,
		PostSamplingRunner:     cfg.PostSamplingRunner,
		TurnSideEffects:        cfg.TurnSideEffects,
		BareMode:               cfg.BareMode,
		SessionID:              cfg.SessionID,
		CacheLineageID:         cacheLineageID,
		SessionProjectDir:      cfg.SessionProjectDir,
		ProjectRoot:            cfg.ProjectRoot,
		CWD:                    cfg.CWD,
		AgentID:                cfg.AgentID,
		AgentType:              cfg.AgentType,
		AgentTranscriptPath:    cfg.AgentTranscriptPath,
		ReasoningEffort:        cfg.ReasoningEffort,
		ServiceTier:            cfg.ServiceTier,
		PinnedModel:            cfg.PinnedModel,
		StreamingToolExecution: cfg.StreamingToolExecution,
		PermissionHandler:      cfg.PermissionHandler,
		SkillManager:           cfg.SkillManager,
		SkillProjectGeneration: cfg.SkillProjectGeneration,
		CommandQueue:           cfg.CommandQueue,
		MemoryPrefetcher:       cfg.MemoryPrefetcher,
		SkillPrefetcher:        cfg.SkillPrefetcher,
		ToolRefresher:          cfg.ToolRefresher,
		QueryScope:             cfg.QueryScope,
		Thinking:               thinkingSnapshot,
		TeammateContext:        cfg.TeammateContext,
		PostCompactCleanup:     cfg.PostCompactCleanup,
		QuerySource:            cfg.QuerySource,
	}
}

func bindGoalEvaluatorModel(evaluator GoalEvaluator, model string) GoalEvaluator {
	if evaluator == nil {
		return nil
	}
	return evaluator.GoalEvaluatorForModel(model)
}

// AutoCompactTracking mirrors the loop-local lifecycle state used by the
// pre-call auto-compact orchestrator.
type AutoCompactTracking struct {
	Compacted           bool
	TurnCounter         int
	TurnID              string
	ConsecutiveFailures int
}

// QueryState centralizes mutable state that must survive across loop
// iterations. Several fields are not behaviorally active yet; they reserve the
// same state surface the subsequent parity tasks will attach to.
type QueryState struct {
	Messages                     []types.Message
	TurnCount                    int
	AutoCompactTracking          AutoCompactTracking
	MaxOutputTokensRecoveryCount int
	HasAttemptedReactiveCompact  bool
	MaxOutputTokensOverride      int
	TaskBudgetRemaining          *int
	PendingToolUseSummary        any
	StopHookActive               bool
	Transition                   QueryTransition
	PendingMemoryPrefetch        PendingAttachmentPrefetch
	MemoryPrefetchConsumed       bool
	PendingSkillPrefetch         PendingAttachmentPrefetch
}

func (s *QueryState) recordTaskBudgetCompaction(total int, preCompactTokens int) {
	if total <= 0 {
		return
	}
	if preCompactTokens < 0 {
		preCompactTokens = 0
	}
	remaining := total
	if s.TaskBudgetRemaining != nil {
		remaining = *s.TaskBudgetRemaining
	}
	remaining -= preCompactTokens
	if remaining < 0 {
		remaining = 0
	}
	s.TaskBudgetRemaining = &remaining
}

func newQueryState(messages []types.Message) *QueryState {
	return &QueryState{
		Messages:   messages,
		TurnCount:  0,
		Transition: QueryTransitionNextTurn,
	}
}

func (s *QueryState) shouldContinue(maxTurns int) bool {
	return maxTurns <= 0 || s.TurnCount < maxTurns
}

func (s *QueryState) beginNextTurn() int {
	s.TurnCount++
	s.Transition = QueryTransitionNextTurn
	return s.TurnCount
}

func (s *QueryState) maxTurnsExceeded(maxTurns int) *MaxTurnsError {
	return &MaxTurnsError{MaxTurns: maxTurns, TurnCount: s.TurnCount + 1}
}
