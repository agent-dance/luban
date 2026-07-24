// Package sdk implements the NDJSON stream-JSON transport layer that allows
// external processes (e.g. the Python SDK) to drive the engine via stdin/stdout.
package sdk

import (
	"encoding/json"

	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

// ─── Stdin messages (Client → Server) ───────────────────────────────────────

// SDKUserMessage is a user-originated conversation turn sent over stdin.
type SDKUserMessage struct {
	Type      string          `json:"type"` // "user"
	Message   json.RawMessage `json:"message"`
	SessionID string          `json:"session_id,omitempty"`
	UUID      string          `json:"uuid,omitempty"`
}

// SDKControlRequest wraps a typed control request from the client.
type SDKControlRequest struct {
	Type      string          `json:"type"` // "control_request"
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"` // peek "subtype" to route
}

// SDKControlResponse wraps a typed control response from the client.
// Used to deliver permission decisions (and other replies) back to the server.
type SDKControlResponse struct {
	Type     string          `json:"type"`     // "control_response"
	Response json.RawMessage `json:"response"` // ControlSuccess or ControlError
}

// SDKKeepAlive is a heartbeat message; the server ignores it.
type SDKKeepAlive struct {
	Type string `json:"type"` // "keep_alive"
}

// ─── Control request inner types ────────────────────────────────────────────

// InitializeRequest is sent by the client on startup.
type InitializeRequest struct {
	Subtype      string `json:"subtype"` // "initialize"
	SystemPrompt string `json:"systemPrompt,omitempty"`
}

// InterruptRequest asks the server to cancel any in-flight query.
type InterruptRequest struct {
	Subtype   string `json:"subtype"` // "interrupt"
	SessionID string `json:"session_id,omitempty"`
}

// SetModelRequest changes the active model for a session.
type SetModelRequest struct {
	Subtype   string `json:"subtype"` // "set_model"
	Model     string `json:"model"`
	SessionID string `json:"session_id,omitempty"`
}

// SetPermissionModeRequest sets the permission mode for the SDK server.
type SetPermissionModeRequest struct {
	Subtype string `json:"subtype"` // "set_permission_mode"
	Mode    string `json:"mode"`    // "default", "plan", "auto-edit", "full-auto"
}

// SetMaxThinkingTokensRequest configures extended thinking for future queries.
// MaxThinkingTokens == nil disables thinking; a non-nil value enables it.
type SetMaxThinkingTokensRequest struct {
	Subtype           string `json:"subtype"`             // "set_max_thinking_tokens"
	MaxThinkingTokens *int   `json:"max_thinking_tokens"` // null = disable
	SessionID         string `json:"session_id,omitempty"`
}

// ResumeRequest asks the server to load a previous session by ID.
type ResumeRequest struct {
	Subtype   string `json:"subtype"` // "resume"
	SessionID string `json:"session_id"`
}

// ResumeResponse is the payload in ControlSuccess.Response for "resume".
type ResumeResponse struct {
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"`
}

// CompactRequest asks the server to run context compaction on a session.
type CompactRequest struct {
	Subtype   string `json:"subtype"` // "compact"
	SessionID string `json:"session_id"`
}

// GetContextUsageRequest asks for token usage stats for a session.
type GetContextUsageRequest struct {
	Subtype   string `json:"subtype"` // "get_context_usage"
	SessionID string `json:"session_id,omitempty"`
}

// PermissionResultMsg is the client's reply to a can_use_tool request.
// It arrives either as a control_response or embedded inside a control_request.
type PermissionResultMsg struct {
	Subtype   string `json:"subtype"` // "can_use_tool"
	RequestID string `json:"request_id"`
	Behavior  string `json:"behavior"`          // "allow" | "deny"
	Message   string `json:"message,omitempty"` // human-readable reason (deny only)
}

// ─── Control response inner types ───────────────────────────────────────────

// ControlSuccess wraps a successful response payload sent to the client.
type ControlSuccess struct {
	Subtype   string          `json:"subtype"` // "success"
	RequestID string          `json:"request_id"`
	Response  json.RawMessage `json:"response,omitempty"`
}

// ControlError wraps an error response sent to the client.
type ControlError struct {
	Subtype   string `json:"subtype"` // "error"
	RequestID string `json:"request_id"`
	Error     string `json:"error"`
}

// InitializeResponse is the payload in ControlSuccess.Response for "initialize".
type InitializeResponse struct {
	Tools           []string `json:"tools"`
	Model           string   `json:"model"`
	Models          []string `json:"models,omitempty"`
	OutputStyle     string   `json:"output_style,omitempty"`
	AvailableStyles []string `json:"available_output_styles,omitempty"`
	ProtocolVersion string   `json:"protocol_version,omitempty"`
}

// ─── Stdout messages (Server → Client) ──────────────────────────────────────

// SDKAssistantMessage is a complete or streaming assistant turn.
type SDKAssistantMessage struct {
	Type      string          `json:"type"`    // "assistant"
	Message   json.RawMessage `json:"message"` // API-format message object
	SessionID string          `json:"session_id"`
	UUID      string          `json:"uuid"`
}

// SDKResultMessage is the terminal message for a query stream.
type SDKResultMessage struct {
	Type         string   `json:"type"`    // "result"
	Subtype      string   `json:"subtype"` // "success" | "error_during_execution" | ...
	SessionID    string   `json:"session_id"`
	ProjectRoot  string   `json:"project_root,omitempty"`
	UUID         string   `json:"uuid"`
	IsError      bool     `json:"is_error"`
	Result       string   `json:"result,omitempty"` // last text output on success
	NumTurns     int      `json:"num_turns"`
	DurationMs   float64  `json:"duration_ms"`
	TotalCostUSD float64  `json:"total_cost_usd"`
	Errors       []string `json:"errors,omitempty"` // populated on error subtypes
}

// SDKSystemMessage carries system-level notifications (init, error, status).
type SDKSystemMessage struct {
	Type        string          `json:"type"`    // "system"
	Subtype     string          `json:"subtype"` // "init" | "error" | "status"
	Message     string          `json:"message,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	ProjectRoot string          `json:"project_root,omitempty"`
	UUID        string          `json:"uuid,omitempty"`
	TurnID      string          `json:"turn_id,omitempty"`
	ToolUseID   string          `json:"tool_use_id,omitempty"`
	ActorID     string          `json:"actor_id,omitempty"`
	ActorType   string          `json:"actor_type,omitempty"`
	WorkUnitID  string          `json:"work_unit_id,omitempty"`
	Error       *types.APIError `json:"error,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
	// RuntimeEvent is the explicit audience/redaction projection for runtime
	// failures and warnings. Default SDK output never populates Error, Metadata,
	// or ProjectRoot from a loop error/warning event.
	RuntimeEvent *runtimeevent.ProjectedRuntimeEvent `json:"runtime_event,omitempty"`
	// init-specific
	Tools []string `json:"tools,omitempty"`
	Model string   `json:"model,omitempty"`
}

// SDKControlRequestOut is a control_request sent from server → client.
// Currently used for can_use_tool permission challenges.
type SDKControlRequestOut struct {
	Type      string          `json:"type"` // "control_request"
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
}

// PermissionRequestMsg is the can_use_tool challenge sent to the client.
type PermissionRequestMsg struct {
	Subtype            string                   `json:"subtype"` // "can_use_tool"
	SessionID          string                   `json:"session_id,omitempty"`
	ExecutionSessionID string                   `json:"execution_session_id,omitempty"`
	TurnID             string                   `json:"turn_id,omitempty"`
	DecisionID         string                   `json:"decision_id,omitempty"`
	RequestID          string                   `json:"request_id"`
	ToolName           string                   `json:"tool_name"`
	Input              map[string]any           `json:"input"`
	ToolUseID          string                   `json:"tool_use_id"`
	ActorID            string                   `json:"actor_id,omitempty"`
	ActorType          string                   `json:"actor_type,omitempty"`
	WorkUnitID         string                   `json:"work_unit_id,omitempty"`
	Kind               string                   `json:"kind,omitempty"`
	Action             string                   `json:"action,omitempty"`
	Target             string                   `json:"target,omitempty"`
	Impact             string                   `json:"impact,omitempty"`
	RiskReason         string                   `json:"risk_reason,omitempty"`
	RuleSource         string                   `json:"rule_source,omitempty"`
	ApprovalScope      string                   `json:"approval_scope,omitempty"`
	Choices            []string                 `json:"choices,omitempty"`
	Body               string                   `json:"body,omitempty"`
	ReviewDetails      []string                 `json:"review_details,omitempty"`
	PostMode           string                   `json:"post_mode,omitempty"`
	Description        string                   `json:"description,omitempty"`
	Mode               string                   `json:"mode,omitempty"`
	AvoidPrompts       bool                     `json:"avoid_prompts,omitempty"`
	Message            string                   `json:"message,omitempty"`
	Suggestions        []types.PermissionUpdate `json:"suggestions,omitempty"`
	BlockedPath        string                   `json:"blocked_path,omitempty"`
}
