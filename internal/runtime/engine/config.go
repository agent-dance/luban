package engine

import (
	"strings"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
)

const sharedDefaultMaxOutputTokens = 16384

// Config holds all pluggable components for a CoreEngine.
// Fields with nil values fall back to sensible defaults (documented per field).
type Config struct {
	// Provider is the LLM backend. Pass a ProviderRef when the backend must be
	// hot-swappable. Required — ErrNoProvider is returned if nil.
	Provider provider.Provider

	// Registry holds the registered tools. If nil a new empty registry is used.
	Registry *registry.Registry

	// Sessions is the persistence layer for conversations. If nil, ProjectRoot
	// selects a repository-backed store rooted at the default directory.
	Sessions SessionManager

	// GoalEvaluator overrides automatic goal completion evaluation. When nil,
	// repository-backed conversations use the active ProviderRef.
	GoalEvaluator loop.GoalEvaluator

	// Permission controls tool-call authorisation.
	// If nil AllowAllHandler is used (all calls permitted).
	Permission permission.PermissionHandler

	// HookRunner runs pre/post-tool hooks.
	// If nil hooks are disabled.
	HookRunner *hooks.Runner

	// SystemPrompt is the default system prompt injected into every query.
	SystemPrompt string
	// SystemPromptBlocks is the ordered block form used by providers with
	// native multi-block system prompt support.
	SystemPromptBlocks []prompt.SystemPromptBlock
	// UserContext is prepended as model-visible meta context. It carries
	// workspace instructions independently from the provider system envelope.
	UserContext prompt.UserContext
	// VisibleTools binds a generated system prompt to the exact immutable
	// provider-facing catalog. The zero value preserves legacy dynamic catalog
	// behavior for embedders and custom prompts.
	VisibleTools        registry.VisibleToolSnapshot
	ToolPromptConfig    prompt.Config
	GeneratedToolPrompt bool

	// Model overrides the provider's default model.
	// If empty the provider's ModelID() is used.
	Model string

	// MaxTokens caps the response length per API call (0 = provider default).
	MaxTokens int

	// TaskBudget is the API-side task output budget passed to providers.
	// Zero disables output_config.task_budget.
	TaskBudget int

	// MaxContextTokens enables context-window compaction when non-zero.
	MaxContextTokens int

	// ProgressiveContext controls cache-aware, reversible provider-view
	// projection before semantic compaction. Its zero value is disabled.
	ProgressiveContext compact.ProgressiveConfig

	// MaxTurns is the default agentic turn limit per query (0 = 100).
	MaxTurns int

	// ReasoningEffort controls reasoning model effort: "low", "medium", "high".
	ReasoningEffort string

	// ServiceTier pins the provider scheduling class for every generation,
	// including compaction and goal evaluation. Empty leaves it unspecified.
	ServiceTier provider.ServiceTier

	// PinnedModel rejects provider-directed model fallback. Benchmark and other
	// contract-bound runs use it to keep every scored generation on one model.
	PinnedModel bool

	// EventBufferSize is the channel buffer for Query events (0 = 64).
	EventBufferSize int

	// CWD is the default working directory for tool execution.
	// Per-query override is available via QueryRequest.CWD.
	CWD string

	// ProjectRoot is the default immutable workspace identity. CWD is never
	// used to infer this identity.
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
	BackgroundTasks  compact.BackgroundTaskProvider
	MCPState         compact.MCPStateProvider
	AgentDefinitions compact.AgentDefinitionProvider
}

// defaults fills in zero-value fields with their defaults.
func (c *Config) defaults() {
	if c.Provider != nil && c.MaxTokens <= 0 {
		model := strings.TrimSpace(c.Model)
		if model == "" {
			model = c.Provider.ModelID()
		}
		c.MaxTokens = provider.ResolveRequestMaxOutput(c.Provider.Name(), model, c.MaxTokens)
		if c.MaxTokens <= 0 {
			c.MaxTokens = provider.DefaultMaxOutputTokens(c.Provider.Name(), model)
		}
	}
	if c.Registry == nil {
		c.Registry = registry.New()
	}
	if c.Permission == nil {
		c.Permission = permission.AllowAllHandler{}
	}
	if c.EventBufferSize == 0 {
		c.EventBufferSize = 64
	}
	if c.MaxTurns == 0 {
		c.MaxTurns = 100
	}
}
