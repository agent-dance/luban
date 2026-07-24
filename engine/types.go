package engine

import (
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/types"
)

// QueryRequest is the input to Engine.Query.
type QueryRequest struct {
	// SessionID identifies the conversation.
	// If empty, a new UUID session is created automatically.
	SessionID string `json:"session_id,omitempty"`

	// Message is the user turn to send.
	Message string `json:"message"`

	// Content, when non-empty, is used instead of Message to build the user
	// turn.  It allows callers to send multimodal content (e.g. images) by
	// constructing the content blocks directly.
	Content []types.ContentBlock `json:"content,omitempty"`

	// InternalKind marks a runtime-generated model turn. It remains in model
	// context but is excluded from human transcript projection and previews.
	InternalKind types.InternalMessageKind `json:"internal_kind,omitempty"`
	// InternalControlCapability is required for an ordinary Query to request a
	// runtime-owned message kind. Its type is behind Go's internal boundary.
	InternalControlCapability messagecontrol.Capability `json:"-"`

	// RuntimeEventID is the stable idempotency key for a runtime-generated
	// follow-up. Ordinary Query calls discard it; QueryFollowUp persists it as
	// the internal message identity before reporting success.
	RuntimeEventID string `json:"runtime_event_id,omitempty"`

	// MaxTurns overrides the default agentic turn limit for this query.
	// Zero means use the engine default.
	MaxTurns int `json:"max_turns,omitempty"`

	// SystemPromptOverride replaces the configured system prompt for this query only.
	SystemPromptOverride string `json:"system_prompt_override,omitempty"`

	// CWD overrides the engine's default CWD for this query only.
	// Empty string = use engine default. Used by Daemon mode for per-session isolation.
	CWD string `json:"cwd,omitempty"`

	// ProjectRoot is the immutable workspace identity for this query. It is
	// deliberately separate from CWD because a tool may execute in a nested
	// directory without changing the owning project/session namespace.
	ProjectRoot string `json:"project_root,omitempty"`

	// SessionProjectDir is the exact durable repository namespace. Worktree
	// entry changes ProjectRoot/CWD but deliberately keeps this value stable so
	// the same conversation history continues in its original session store.
	SessionProjectDir string `json:"session_project_dir,omitempty"`
}

// Event wraps a loop.Event with session context and lifecycle flags.
type Event struct {
	// SessionID identifies which session emitted this event.
	SessionID string `json:"session_id"`

	// Inner is the underlying loop event.
	Inner loop.Event `json:"inner"`

	// Final is true for the last event in a Query stream (whether success or error).
	Final bool `json:"final"`

	// Error carries a terminal error when Final is true and the query failed.
	Error error `json:"error,omitempty"`
}

// ContextUsageInfo reports token consumption for a session.
type ContextUsageInfo struct {
	TotalTokens      int      `json:"total_tokens"`
	UsedTokens       int      `json:"used_tokens"`
	RemainingTokens  int      `json:"remaining_tokens"`
	Measurement      string   `json:"measurement,omitempty"`
	EstimateComplete bool     `json:"estimate_complete"`
	UnknownOverheads []string `json:"unknown_overheads,omitempty"`
}
