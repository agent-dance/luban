package provider

import (
	"context"
	"strings"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/types"
)

// Provider is the abstraction for any LLM backend (Anthropic, OpenAI, Ollama, etc.)
type Provider interface {
	// Name returns the provider identifier
	Name() string

	// CreateStream sends a streaming message request and returns events via channel
	CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error)

	// ModelID returns the default model for this provider
	ModelID() string
}

// TokenCountingProvider is an optional precise text-token counter. Read uses
// it only near configured limits and falls back to its local tokenizer when a
// provider does not expose the API or the request fails.
type TokenCountingProvider interface {
	CountTokens(ctx context.Context, content string) (int, error)
}

// ToolChoice controls which tool (if any) the model must call.
// Type can be "auto" (default), "any" (must call some tool), or "tool" (must call specific tool).
type ToolChoice struct {
	Type string // "auto", "any", "tool"
	Name string // only used when Type == "tool"
}

// ThinkingConfig controls extended thinking for a request
type ThinkingConfig struct {
	Enabled      bool
	BudgetTokens int // token budget for thinking; 0 means use model default (1024 minimum required by API)
}

// TaskBudget controls API-side task output budgeting across compaction
// boundaries. Remaining is omitted until the loop has compacted at least once.
type TaskBudget struct {
	Total     int
	Remaining *int
}

// Params holds LLM-agnostic request parameters
type Params struct {
	Model     string
	MaxTokens int
	// MaxOutputTokensOverride temporarily overrides MaxTokens for output-limit
	// recovery paths. Zero preserves the provider's normal MaxTokens behavior.
	MaxOutputTokensOverride int
	System                  string                     // single system prompt block
	SystemBlocks            []prompt.SystemPromptBlock // ordered system prompt blocks with optional metadata
	Messages                []types.Message
	Tools                   []types.ToolDefinition
	ExtraToolSchemas        []types.ServerToolDefinition
	ToolChoice              *ToolChoice     // nil = auto (default)
	Thinking                *ThinkingConfig // nil = thinking disabled
	TaskBudget              *TaskBudget     // nil = API-side task budget disabled

	// Responses API specific fields
	Conversation       string // Session ID for conversation tracking
	PreviousResponseID string // Link to previous response for context continuity
	Truncation         string // "auto", "disabled", "required_or_error"

	// Prompt cache routing fields shared by Responses and compatible Chat APIs.
	PromptCacheKey string // Stable session lineage lowered to a user-level routing shard by capable providers
	UsePromptCache bool   // Mirror Codex CLI: keep prompt_cache_key enabled even after previous_response_id fallback
	// PromptCacheTTL is provider-selected. Empty preserves the provider's
	// default; "1h" requests the longest documented Anthropic-compatible TTL.
	PromptCacheTTL  string
	ReasoningEffort string // "low", "medium", "high" for reasoning models

	internalControlScope    messagecontrol.Scope
	internalControlScopeSet bool
}

// WithInternalControlScope installs the runtime-owned authority fence used by
// provider adapters. Its capability type is internal to this module, so SDK
// callers cannot select a scope or turn arbitrary developer messages into
// privileged instructions.
func (p Params) WithInternalControlScope(capability messagecontrol.Capability, scope messagecontrol.Scope) Params {
	p.internalControlScope = messagecontrol.Scope{}
	p.internalControlScopeSet = false
	if capability.Valid() {
		p.internalControlScope = scope
		p.internalControlScopeSet = true
	}
	return p
}

func (p Params) isTrustedDeveloperMessage(message types.Message) bool {
	if p.internalControlScopeSet {
		return message.IsTrustedDeveloperMessageForScope(p.internalControlScope)
	}
	return false
}

// SystemTextBlocks resolves the system prompt fields into provider-ready text
// blocks. SystemBlocks take priority over the single System string.
func (p Params) SystemTextBlocks() []prompt.SystemPromptBlock {
	if len(p.SystemBlocks) > 0 {
		blocks := make([]prompt.SystemPromptBlock, 0, len(p.SystemBlocks))
		for _, block := range p.SystemBlocks {
			if block.Text == "" {
				continue
			}
			blocks = append(blocks, block)
		}
		return blocks
	}
	if p.System == "" {
		return nil
	}
	return []prompt.SystemPromptBlock{{
		Text:       p.System,
		Cache:      true,
		CacheScope: "ephemeral",
	}}
}

// JoinedSystemPrompt returns the stable string representation used by providers
// that do not have native multi-block system prompt support.
func (p Params) JoinedSystemPrompt() string {
	blocks := p.SystemTextBlocks()
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Text == "" {
			continue
		}
		texts = append(texts, block.Text)
	}
	return strings.Join(texts, "\n\n")
}

// Config holds provider-agnostic configuration
type Config struct {
	ProviderName           string
	APIStyle               APIStyle
	APIKey                 string
	AuthToken              string
	BaseURL                string
	Model                  string
	MaxTokens              int
	Headers                map[string]string
	Timeout                int // seconds
	DisableStrictTools     bool
	CacheRoutingPreference CacheRoutingPreference
}

// CacheRoutingPreference is an operator override for request-level cache
// routing fields. Auto uses the provider's documented or best-effort policy,
// On upgrades best-effort prompt_cache_key to strict delivery, and Off omits
// routing fields without changing native content-block cache_control support.
type CacheRoutingPreference string

const (
	CacheRoutingAuto CacheRoutingPreference = ""
	CacheRoutingOn   CacheRoutingPreference = "on"
	CacheRoutingOff  CacheRoutingPreference = "off"
)

// ParseCacheRoutingPreference accepts the persisted auto/on/off spelling.
// Unknown values safely retain the default automatic policy.
func ParseCacheRoutingPreference(value string) CacheRoutingPreference {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on":
		return CacheRoutingOn
	case "off":
		return CacheRoutingOff
	default:
		return CacheRoutingAuto
	}
}

// CacheRoutingMode describes how a cache lineage is lowered onto the wire.
// It is deliberately separate from CacheControl, which means Anthropic-style
// content-block cache breakpoints rather than request routing affinity.
type CacheRoutingMode string

const (
	CacheRoutingNone                     CacheRoutingMode = ""
	CacheRoutingPromptCacheKey           CacheRoutingMode = "prompt_cache_key"
	CacheRoutingPromptCacheKeyBestEffort CacheRoutingMode = "prompt_cache_key_best_effort"
	CacheRoutingDeepSeekUserID           CacheRoutingMode = "deepseek_user_id"
)

// ProviderCapabilities describes the features a provider supports.
// Zero values mean "not supported" or "unknown".
type ProviderCapabilities struct {
	Thinking     bool
	ToolUse      bool
	CacheControl bool
	CacheRouting CacheRoutingMode
	SystemParts  bool
	Vision       bool
	MaxContext   int // token limit; 0 = unknown
}

// CapabilityProvider is an optional interface for providers that can report
// their capabilities. Callers should type-assert: if cp, ok := p.(CapabilityProvider); ok { ... }
type CapabilityProvider interface {
	Capabilities() ProviderCapabilities
}
