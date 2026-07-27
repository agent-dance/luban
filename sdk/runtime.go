package sdk

import (
	"context"

	"github.com/agent-dance/luban/runtimeevent"
)

// Runtime is the stable execution boundary consumed by SDKServer. Runtime
// implementations adapt the application engine without exposing engine or
// loop contracts to SDK consumers.
type Runtime interface {
	Query(context.Context, QueryRequest) (<-chan QueryEvent, error)
	Resume(context.Context, string) (int, error)
	Compact(context.Context, string) (CompactResult, error)
	Interrupt(string)
	SetModel(string, string) error
	SetThinkingConfig(string, bool, int) error
	ContextUsage(string) (*ContextUsageInfo, error)
	Tools() []string
	ModelID() string
	SetSystemPrompt(string)
	SetPermission(PermissionHandler)
}

// CompactResult is the runtime's authoritative outcome for one manual
// compaction request. Compacted is false for a successful semantic no-op.
type CompactResult struct {
	Compacted          bool
	BeforeMessageCount int
	AfterMessageCount  int
	ContextGeneration  uint64
}

// QueryRequest is an SDK-originated user turn.
type QueryRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message"`
}

// QueryEvent is one item in a Runtime query stream.
type QueryEvent struct {
	SessionID string `json:"session_id"`
	Event     Event  `json:"event"`
	Final     bool   `json:"final"`
	Error     error  `json:"-"`
}

// ContextUsageInfo reports token consumption for an SDK session.
type ContextUsageInfo struct {
	TotalTokens     int    `json:"total_tokens"`
	UsedTokens      int    `json:"used_tokens"`
	RemainingTokens int    `json:"remaining_tokens"`
	Measurement     string `json:"measurement,omitempty"`
}

// EventType identifies an SDK runtime event.
type EventType string

const (
	EventRequestStart      EventType = "request_start"
	EventRequestRetry      EventType = "request_retry"
	EventRequestFirstToken EventType = "request_first_token"
	EventRequestEnd        EventType = "request_end"
	EventRequestFailed     EventType = "request_failed"
	EventToolRoundMetrics  EventType = "tool_round_metrics"
	EventText              EventType = "text"
	EventThinking          EventType = "thinking"
	EventToolUse           EventType = "tool_use"
	EventToolResult        EventType = "tool_result"
	EventToolUseSummary    EventType = "tool_use_summary"
	EventTurnEnd           EventType = "turn_end"
	EventProviderUsage     EventType = "provider_usage"
	EventError             EventType = "error"
	EventSystemWarning     EventType = "system_warning"
	EventTombstone         EventType = "tombstone"
	EventCompactBoundary   EventType = "compact_boundary"
	EventMaxTurnsReached   EventType = "max_turns_reached"
	EventUserInterruption  EventType = "user_interruption"
	EventHookSummary       EventType = "hook_summary"
	EventProgress          EventType = "progress"
	EventGoalEvaluation    EventType = "goal_evaluation"
	EventGoalStatus        EventType = "goal_status"
)

// Event is the SDK-owned event projection produced by Runtime.
type Event struct {
	Type        EventType   `json:"type"`
	Text        string      `json:"text,omitempty"`
	ToolUse     *ToolUse    `json:"tool_use,omitempty"`
	ToolResult  *ToolResult `json:"tool_result,omitempty"`
	Usage       *Usage      `json:"usage,omitempty"`
	TurnCount   int         `json:"turn_count,omitempty"`
	ProjectRoot string      `json:"project_root,omitempty"`
	TurnID      string      `json:"turn_id,omitempty"`
	ActorID     string      `json:"actor_id,omitempty"`
	ActorType   string      `json:"actor_type,omitempty"`
	WorkUnitID  string      `json:"work_unit_id,omitempty"`

	Error          *APIError              `json:"error,omitempty"`
	ToolUseID      string                 `json:"tool_use_id,omitempty"`
	TerminalReason string                 `json:"terminal_reason,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
	Compact        *CompactBoundaryEvent  `json:"compact,omitempty"`
	MaxTurns       *MaxTurnsEvent         `json:"max_turns,omitempty"`
	Tombstone      *TombstoneEvent        `json:"tombstone,omitempty"`
	ToolSummary    *ToolUseSummaryEvent   `json:"tool_summary,omitempty"`
	HookSummary    *HookSummaryEvent      `json:"hook_summary,omitempty"`
	Progress       *RuntimeProgressEvent  `json:"progress,omitempty"`
	RequestStatus  *RequestStatusEvent    `json:"request_status,omitempty"`
	ToolRound      *ToolRoundMetricsEvent `json:"tool_round,omitempty"`
	RuntimeEvent   *RuntimeEvent          `json:"-"`
}

type ToolUse struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type ToolOutcome string

const (
	ToolOutcomeSucceeded ToolOutcome = "succeeded"
	ToolOutcomeFailed    ToolOutcome = "failed"
	ToolOutcomePartial   ToolOutcome = "partial"
	ToolOutcomeDenied    ToolOutcome = "denied"
	ToolOutcomeCancelled ToolOutcome = "cancelled"
	ToolOutcomeTimedOut  ToolOutcome = "timed_out"
)

type ToolResult struct {
	ToolUseID     string            `json:"tool_use_id"`
	Content       string            `json:"content,omitempty"`
	ContentBlocks []any             `json:"content_blocks,omitempty"`
	Data          any               `json:"data,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Usage         *Usage            `json:"usage,omitempty"`
	Outcome       ToolOutcome       `json:"outcome"`
}

// MachineEventSchemaVersion is the content-free SDK tool-event schema. Raw
// inputs and results are represented only by content-addressed references.
const MachineEventSchemaVersion = runtimeevent.MachineEventSchemaVersion

type MachineContentReference = runtimeevent.ContentReference
type MachineToolEventMetrics = runtimeevent.ToolEventMetrics

type Usage struct {
	InputTokens              int             `json:"input_tokens"`
	OutputTokens             int             `json:"output_tokens"`
	CacheCreationInputTokens int             `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int             `json:"cache_read_input_tokens,omitempty"`
	ServerToolUse            ServerToolUsage `json:"server_tool_use,omitempty"`
}

type ServerToolUsage struct {
	WebSearchRequests int `json:"web_search_requests,omitempty"`
	WebFetchRequests  int `json:"web_fetch_requests,omitempty"`
}

type APIError struct {
	Type          string `json:"type"`
	Message       string `json:"message"`
	Status        int    `json:"status,omitempty"`
	RetryAfter    string `json:"retry_after,omitempty"`
	OriginalModel string `json:"original_model,omitempty"`
	FallbackModel string `json:"fallback_model,omitempty"`
}

type CompactBoundaryEvent struct {
	BoundaryID                string                    `json:"boundary_id,omitempty"`
	Trigger                   string                    `json:"trigger,omitempty"`
	PreCompactTokenCount      int                       `json:"pre_compact_token_count,omitempty"`
	PostCompactTokenCount     int                       `json:"post_compact_token_count,omitempty"`
	TruePostCompactTokenCount int                       `json:"true_post_compact_token_count,omitempty"`
	PreviousTailIdentifier    string                    `json:"previous_tail_identifier,omitempty"`
	PreCompactDiscoveredTools []string                  `json:"pre_compact_discovered_tools,omitempty"`
	PreservedSegment          *PreservedSegmentMetadata `json:"preserved_segment,omitempty"`
	Summary                   string                    `json:"summary,omitempty"`
	UserDisplayMessage        string                    `json:"user_display_message,omitempty"`
}

type PreservedSegmentMetadata struct {
	StartIndex int    `json:"start_index"`
	Count      int    `json:"count"`
	Anchor     string `json:"anchor,omitempty"`
	Direction  string `json:"direction,omitempty"`
}

type MaxTurnsEvent struct {
	MaxTurns  int `json:"max_turns"`
	TurnCount int `json:"turn_count"`
}

type TombstoneEvent struct {
	Reason   string         `json:"reason,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ToolUseSummaryEvent struct {
	ToolUseID     string `json:"tool_use_id,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	Status        string `json:"status,omitempty"`
	OutputSummary string `json:"output_summary,omitempty"`
}

type HookSummaryEvent struct {
	HookExecutionID string         `json:"hook_execution_id,omitempty"`
	ToolUseID       string         `json:"tool_use_id,omitempty"`
	HookName        string         `json:"hook_name,omitempty"`
	Status          string         `json:"status,omitempty"`
	Summary         string         `json:"summary,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type RuntimeProgressEvent struct {
	Stage         string         `json:"stage,omitempty"`
	Message       string         `json:"message,omitempty"`
	Current       int            `json:"current,omitempty"`
	Total         int            `json:"total,omitempty"`
	Disposition   string         `json:"disposition,omitempty"`
	Blocker       string         `json:"blocker,omitempty"`
	MutationEpoch uint64         `json:"mutation_epoch,omitempty"`
	VerifiedEpoch uint64         `json:"verified_epoch,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// RequestStatusEvent is the public, raw-safe lifecycle of one provider
// transport request. ErrorMessage is semantic copy; raw provider errors never
// cross the SDK boundary.
type RequestStatusEvent struct {
	RequestID              string `json:"request_id"`
	Phase                  string `json:"phase"`
	Status                 string `json:"status"`
	StartedAt              string `json:"started_at,omitempty"`
	EndedAt                string `json:"ended_at,omitempty"`
	Attempt                int    `json:"attempt,omitempty"`
	MaxAttempts            int    `json:"max_attempts,omitempty"`
	RetryCount             int    `json:"retry_count,omitempty"`
	RetryDelayMilliseconds int64  `json:"retry_delay_ms,omitempty"`
	RequestMilliseconds    int64  `json:"request_ms,omitempty"`
	FirstTokenMilliseconds int64  `json:"first_token_ms,omitempty"`
	TotalMilliseconds      int64  `json:"total_ms,omitempty"`
	InputTokens            int    `json:"input_tokens,omitempty"`
	CacheReadInputTokens   int    `json:"cache_read_input_tokens,omitempty"`
	CacheWriteInputTokens  int    `json:"cache_write_input_tokens,omitempty"`
	OutputTokens           int    `json:"output_tokens,omitempty"`
	ErrorCode              string `json:"error_code,omitempty"`
	ErrorMessage           string `json:"error_message,omitempty"`
}

// ToolRoundMetricsEvent is the public content-free performance summary for a
// model-visible tool batch.
type ToolRoundMetricsEvent struct {
	RoundID                       string `json:"round_id"`
	LogicalModelVisibleCalls      int    `json:"logical_model_visible_calls"`
	PhysicalChildOperations       int    `json:"physical_child_operations"`
	Fanout                        int    `json:"fanout"`
	BatchCount                    int    `json:"batch_count,omitempty"`
	QueueMilliseconds             int64  `json:"queue_ms"`
	CriticalPathMilliseconds      int64  `json:"critical_path_ms"`
	TotalChildLatencyMilliseconds int64  `json:"total_child_latency_ms"`
	ErrorCount                    int    `json:"error_count"`
	RevisionFusionCount           int    `json:"revision_fusion_count,omitempty"`
	RevisionBarrierSkips          int    `json:"revision_barrier_skips,omitempty"`
	RevisionMismatchCount         int    `json:"revision_mismatch_count,omitempty"`
}

type RuntimeIdentity struct {
	EventID           string `json:"event_id"`
	SessionID         string `json:"session_id,omitempty"`
	Epoch             uint64 `json:"epoch,omitempty"`
	ContextGeneration uint64 `json:"context_generation,omitempty"`
	TurnID            string `json:"turn_id,omitempty"`
	ToolUseID         string `json:"tool_use_id,omitempty"`
	WorkUnitID        string `json:"work_unit_id,omitempty"`
	ActorID           string `json:"actor_id,omitempty"`
	ActorType         string `json:"actor_type,omitempty"`
}

type RuntimeEvidenceRef struct {
	ID     string `json:"id"`
	Digest string `json:"digest,omitempty"`
}

// ProjectedRuntimeEvent is the SDK-owned, public-only runtime event wire
// schema. It intentionally has no private diagnostic fields.
type ProjectedRuntimeEvent struct {
	Type           string `json:"type"`
	SchemaVersion  string `json:"schema_version"`
	Audience       string `json:"audience"`
	RedactionLevel string `json:"redaction_level"`
	Kind           string `json:"kind,omitempty"`

	EventID           string `json:"event_id,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	Epoch             uint64 `json:"epoch,omitempty"`
	ContextGeneration uint64 `json:"context_generation,omitempty"`
	TurnID            string `json:"turn_id,omitempty"`
	ToolUseID         string `json:"tool_use_id,omitempty"`
	WorkUnitID        string `json:"work_unit_id,omitempty"`
	ActorID           string `json:"actor_id,omitempty"`
	ActorType         string `json:"actor_type,omitempty"`

	Outcome     ToolOutcome         `json:"outcome,omitempty"`
	Code        string              `json:"code"`
	Message     string              `json:"message"`
	PublicKey   string              `json:"public_key,omitempty"`
	PublicArgs  []any               `json:"public_args,omitempty"`
	EvidenceRef *RuntimeEvidenceRef `json:"evidence_ref,omitempty"`
}

// RuntimeEvent carries the projection authority needed at the SDK output
// boundary. Private fields are never encoded directly.
type RuntimeEvent struct {
	SchemaVersion string `json:"schema_version"`
	Kind          string `json:"kind"`
	RuntimeIdentity
	Outcome ToolOutcome `json:"outcome,omitempty"`

	PublicKey  string `json:"public_key"`
	PublicArgs []any  `json:"public_args,omitempty"`

	DiagnosticCode  string              `json:"diagnostic_code"`
	PrivateCause    error               `json:"-"`
	PrivateMetadata map[string]any      `json:"-"`
	EvidenceRef     *RuntimeEvidenceRef `json:"evidence_ref,omitempty"`
}
