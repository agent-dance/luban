package engine

import (
	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
)

// Config holds all pluggable components for a CoreEngine.
// Fields with nil values fall back to sensible defaults (documented per field).
type Config struct {
	// Provider is the LLM backend. Required — ErrNoProvider is returned if nil.
	Provider provider.Provider

	// ProviderRef is an optional thread-safe swappable provider reference.
	// When set, it takes precedence over Provider (which is still required for
	// backward compatibility). If nil, New() auto-wraps Provider in a ProviderRef.
	ProviderRef *provider.ProviderRef

	// Registry holds the registered tools. If nil a new empty registry is used.
	Registry *registry.Registry

	// Sessions is the persistence layer for conversations.
	// If nil a FileStore rooted at the default directory is used.
	Sessions SessionManager

	// GoalEvaluator overrides automatic goal completion evaluation. When nil,
	// repository-backed conversations use the active ProviderRef.
	GoalEvaluator loop.GoalEvaluator

	// Permission controls tool-call authorisation.
	// If nil AllowAllHandler is used (all calls permitted).
	Permission PermissionHandler

	// HookRunner runs pre/post-tool hooks.
	// If nil hooks are disabled.
	HookRunner *hooks.Runner

	// SystemPrompt is the default system prompt injected into every query.
	SystemPrompt string
	// SystemPromptBlocks is the ordered block form used by providers with
	// native multi-block system prompt support. SystemPrompt remains as the
	// backward-compatible fallback.
	SystemPromptBlocks []prompt.SystemPromptBlock

	// Model overrides the provider's default model.
	// If empty the provider's ModelID() is used.
	Model string

	// AllowedDirs are directories allowed for post-compact file recovery.
	AllowedDirs []string

	// MaxTokens caps the response length per API call (0 = provider default).
	MaxTokens int

	// TaskBudget is the API-side task output budget passed to providers.
	// Zero disables output_config.task_budget.
	TaskBudget int

	// MaxContextTokens enables context-window compaction when non-zero.
	MaxContextTokens int

	// MaxTurns is the default agentic turn limit per query (0 = 100).
	MaxTurns int

	// ReasoningEffort controls reasoning model effort: "low", "medium", "high".
	ReasoningEffort string

	// EventBufferSize is the channel buffer for Query events (0 = 64).
	EventBufferSize int

	// CWD is the default working directory for tool execution.
	// Per-query override is available via QueryRequest.CWD.
	CWD string

	// ProjectRoot is the default immutable workspace identity. When empty,
	// legacy callers retain the prior CWD-based behavior.
	ProjectRoot string

	// SkillManager provides skill discovery for listing injection into conversations.
	// If nil, no skill listing is injected (skills won't be discoverable by the model).
	SkillManager *skills.Manager

	// SkillSessionOverrides is the process-shared, session-scoped visibility
	// layer used by SkillManager. Keeping the exact same layer here lets the
	// engine persist and atomically restore session overrides without creating a
	// second registry or override authority.
	SkillSessionOverrides *skills.MemorySessionOverrideLayer

	PlanState        compact.PlanStateProvider
	InvokedSkills    compact.InvokedSkillProvider
	BackgroundTasks  compact.BackgroundTaskProvider
	MCPState         compact.MCPStateProvider
	AgentDefinitions compact.AgentDefinitionProvider
}

// defaults fills in zero-value fields with their defaults.
func (c *Config) defaults() {
	if c.Registry == nil {
		c.Registry = registry.New()
	}
	if c.Permission == nil {
		c.Permission = AllowAllHandler{}
	}
	if c.EventBufferSize == 0 {
		c.EventBufferSize = 64
	}
	if c.MaxTurns == 0 {
		c.MaxTurns = 100
	}
}
