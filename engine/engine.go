package engine

import (
	"context"
	"errors"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// ErrContextGenerationUnavailable indicates that an exact authoritative
// generation cannot be resolved. Callers must fail closed instead of treating
// it as legacy generation zero.
var ErrContextGenerationUnavailable = errors.New("context generation unavailable")

// Engine is the core interface for the LUBAN Code runtime.
// It powers CLI, SDK (stdin/stdout JSON), MCP Server, and HTTP Daemon modes.
type Engine interface {
	// Query sends a user message and streams events back via the returned channel.
	// The channel is closed when the query finishes (Final event is also sent).
	// If a query is already in flight for the same session it is cancelled and replaced.
	Query(ctx context.Context, req QueryRequest) (<-chan Event, error)

	// Resume restores a previous session by session ID and returns the message count.
	Resume(ctx context.Context, sessionID string) (int, error)

	// Compact runs context compaction on the stored session history.
	// customInstructions optionally carries "/compact <args>".
	Compact(ctx context.Context, sessionID string, customInstructions ...string) error

	// Interrupt cancels any in-flight query for the given session.
	// It is a no-op if no query is running.
	Interrupt(sessionID string)

	// SetModel changes the model used for future queries in a session.
	SetModel(sessionID string, model string) error

	// SetReasoningEffort changes the reasoning effort used for future queries in a session.
	SetReasoningEffort(sessionID string, effort string) error

	// SetThinkingConfig enables or disables extended thinking for future queries in a session.
	// When enabled is false, thinking is disabled. budgetTokens controls the token budget (0 = provider default).
	SetThinkingConfig(sessionID string, enabled bool, budgetTokens int) error

	// ContextUsage returns token usage statistics for a session.
	ContextUsage(sessionID string) (*ContextUsageInfo, error)

	// Tools returns the names of tools enabled in the current runtime context.
	Tools() []string

	// ToolDefinitions returns schemas for tools enabled in the current runtime
	// context, including tools deferred from the current model request.
	ToolDefinitions() []types.ToolDefinition

	// Provider returns the underlying LLM provider.
	Provider() provider.Provider

	// SetProvider atomically replaces the active provider for all future queries.
	// In-flight queries continue using the provider they started with.
	SetProvider(p provider.Provider)

	// ProviderRef returns the shared ProviderRef so callers can register
	// OnChange listeners or do their own atomic reads.
	ProviderRef() *provider.ProviderRef

	// Sessions returns the session manager.
	Sessions() SessionManager

	// SetPermission replaces the PermissionHandler used for all subsequent queries.
	// It is safe to call before the first Query.
	SetPermission(h PermissionHandler)

	// Shutdown gracefully terminates all in-flight queries and flushes sessions.
	Shutdown(ctx context.Context) error
}

// FollowUpEngine accepts runtime-generated conversation turns, such as a
// completed background task. Unlike Query, a follow-up waits for the current
// turn instead of cancelling it.
type FollowUpEngine interface {
	QueryFollowUp(ctx context.Context, req QueryRequest) (<-chan Event, error)
}

// ContextGenerationProvider exposes the authoritative persisted model-context
// generation without widening the core Engine interface for lightweight test
// and embedding implementations.
type ContextGenerationProvider interface {
	ContextGeneration(sessionID string) (uint64, error)
}

// ScopedContextGenerationProvider resolves a generation in an exact durable
// project namespace. Background work uses it to avoid ambiguous bare session
// IDs across projects.
type ScopedContextGenerationProvider interface {
	ContextGenerationForSession(sessionID, projectDir string) (uint64, error)
}

// ContextGenerationState distinguishes a durable manifest generation from a
// genuinely new, not-yet-persisted conversation. Generation zero is therefore
// never used as a wildcard by presentation fences.
type ContextGenerationState struct {
	Generation uint64
	Persisted  bool
}

// ContextGenerationStateProvider exposes the explicit persisted/unpersisted
// state for embedders that do not have a project namespace.
type ContextGenerationStateProvider interface {
	ContextGenerationState(sessionID string) (ContextGenerationState, error)
}

// ScopedContextGenerationStateProvider resolves the explicit state in one
// exact durable namespace. An implementation must not fall back to another
// project when projectDir is non-empty.
type ScopedContextGenerationStateProvider interface {
	ContextGenerationStateForSession(sessionID, projectDir string) (ContextGenerationState, error)
}

// SessionHistoryDeleter removes both persisted history and the live engine
// conversation so late follow-ups and shutdown cannot recreate it.
type SessionHistoryDeleter interface {
	DeleteSessionHistory(ctx context.Context, sessionID, projectDir string) error
}

// RuntimeContext is mutable engine configuration that depends on the active
// session/workspace rather than the provider itself.
type RuntimeContext struct {
	SystemPrompt       string
	SystemPromptBlocks []prompt.SystemPromptBlock
	HookRunner         *hooks.Runner
	AllowedDirs        []string
	ProjectRoot        string
	CWD                string
}

// RuntimeConfigurable is implemented by engines that can be retargeted to a
// different session/workspace without reconstruction.
type RuntimeConfigurable interface {
	UpdateRuntimeContext(ctx RuntimeContext)
}

// WorkspaceRuntimeRebinder updates an existing running conversation's
// workspace runtime without moving its durable SessionProjectDir. Production
// worktree tools call this only from a loop-owned execution context.
type WorkspaceRuntimeRebinder interface {
	RebindWorkspaceRuntime(ctx context.Context, sessionID string, runtime RuntimeContext) error
}

// RuntimeContextResumer stages a session using its target workspace runtime
// without changing the engine-wide defaults. Callers can therefore validate
// and prepare a resume before publishing a new active-session identity.
type RuntimeContextResumer interface {
	ResumeWithRuntimeContext(ctx context.Context, sessionID, projectDir string, runtime RuntimeContext) (int, error)
}

// PreparedRuntimeContextResume is a detached conversation candidate. Commit is
// the only operation that makes it reachable by Query; Abort discards it.
type PreparedRuntimeContextResume interface {
	MessageCount() int
	Commit() error
	Abort()
}

// ContextualPreparedRuntimeContextResume lets cancellation compete with the
// publication step itself. A cancellation observed before CommitContext owns
// the conversation-map mutation leaves the prepared resume unpublished.
type ContextualPreparedRuntimeContextResume interface {
	PreparedRuntimeContextResume
	CommitContext(context.Context) error
}

// RuntimeContextResumePreparer supports two-phase session switching. Loading a
// transcript and constructing its workspace runtime cannot mutate the engine's
// active conversation map until the caller has completed external validation.
type RuntimeContextResumePreparer interface {
	PrepareRuntimeContextResume(ctx context.Context, sessionID, projectDir string, runtime RuntimeContext) (PreparedRuntimeContextResume, error)
}

// SessionResumePreparer prepares a session against the engine's current
// workspace defaults without publishing it. It is used for atomic clear/new
// conversation transitions within one workspace.
type SessionResumePreparer interface {
	PrepareResume(ctx context.Context, sessionID string) (PreparedRuntimeContextResume, error)
}

// SystemPromptConfigurable is implemented by engines whose default system
// prompt can be retargeted after construction.
type SystemPromptConfigurable interface {
	SetSystemPrompt(systemPrompt prompt.SystemPrompt)
}
