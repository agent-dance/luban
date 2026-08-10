// Package stream defines the surface-neutral events emitted by the model
// runtime and consumed by presentation, orchestration, and embedding layers.
package stream

import (
	"encoding/json"

	"github.com/agent-dance/luban/types"
)

// EventType identifies a runtime stream transition.
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

// Event is the authoritative runtime stream carrier.
type Event struct {
	Type        EventType              `json:"type"`
	Text        string                 `json:"text,omitempty"`
	ToolUse     *types.ToolUseBlock    `json:"tool_use,omitempty"`
	ToolResult  *types.ToolResultBlock `json:"tool_result,omitempty"`
	Usage       *types.Usage           `json:"usage,omitempty"`
	TurnCount   int                    `json:"turn_count,omitempty"`
	ProjectRoot string                 `json:"project_root,omitempty"`
	TurnID      string                 `json:"turn_id,omitempty"`
	ActorID     string                 `json:"actor_id,omitempty"`
	ActorType   string                 `json:"actor_type,omitempty"`
	WorkUnitID  string                 `json:"work_unit_id,omitempty"`

	Error          *types.APIError        `json:"error,omitempty"`
	ToolUseID      string                 `json:"tool_use_id,omitempty"`
	TerminalReason string                 `json:"terminal_reason,omitempty"`
	Metadata       map[string]any         `json:"metadata,omitempty"`
	Compact        *CompactBoundaryEvent  `json:"compact,omitempty"`
	MaxTurns       *MaxTurnsEvent         `json:"max_turns,omitempty"`
	Tombstone      *TombstoneEvent        `json:"tombstone,omitempty"`
	ToolSummary    *ToolUseSummaryEvent   `json:"tool_summary,omitempty"`
	HookSummary    *HookSummaryEvent      `json:"hook_summary,omitempty"`
	Progress       *ProgressEvent         `json:"progress,omitempty"`
	GoalStatus     *GoalStatusEvent       `json:"goal_status,omitempty"`
	RequestStatus  *RequestStatusEvent    `json:"request_status,omitempty"`
	ToolRound      *ToolRoundMetricsEvent `json:"tool_round,omitempty"`
	RuntimeEvent   *types.RuntimeEvent    `json:"-"`
}

// MarshalJSON rejects system warnings because a generic stream has no
// audience authority. RuntimeEvent supplies the canonical semantic error.
func (e Event) MarshalJSON() ([]byte, error) {
	if e.Type == EventSystemWarning {
		return json.Marshal(types.RuntimeEvent{})
	}
	type eventJSON Event
	return json.Marshal(eventJSON(e))
}

type RequestStatusEvent struct {
	RequestID              string `json:"request_id"`
	StartedAt              string `json:"started_at,omitempty"`
	EndedAt                string `json:"ended_at,omitempty"`
	Attempt                int    `json:"attempt,omitempty"`
	MaxRetries             int    `json:"max_retries,omitempty"`
	RetryCount             int    `json:"retry_count,omitempty"`
	RetryDelayMilliseconds int64  `json:"retry_delay_ms,omitempty"`
	RetryKind              string `json:"retry_kind,omitempty"`
	RequestMilliseconds    int64  `json:"request_ms,omitempty"`
	FirstTokenMilliseconds int64  `json:"first_token_ms,omitempty"`
	TotalMilliseconds      int64  `json:"total_ms,omitempty"`
	InputTokens            int    `json:"input_tokens,omitempty"`
	CacheReadInputTokens   int    `json:"cache_read_input_tokens,omitempty"`
	CacheWriteInputTokens  int    `json:"cache_write_input_tokens,omitempty"`
	OutputTokens           int    `json:"output_tokens,omitempty"`
	Error                  string `json:"error,omitempty"`
}

// ToolRoundMetricsEvent is a content-free performance projection for one
// model-visible tool batch. It intentionally carries only counts, durations,
// and correlation IDs: tool names, inputs, paths, commands, and outputs never
// enter this envelope.
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

type MaxTurnsEvent struct {
	MaxTurns  int `json:"max_turns"`
	TurnCount int `json:"turn_count"`
}

type PreservedSegmentMetadata struct {
	StartIndex int    `json:"start_index"`
	Count      int    `json:"count"`
	Anchor     string `json:"anchor,omitempty"`
	Direction  string `json:"direction,omitempty"`
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

type ProgressEvent struct {
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

const (
	// ProgressStageLLMToolInput is emitted when the provider opens a tool-use
	// block and as content-free received-byte milestones arrive. It exposes
	// neither partial tool input nor a synthetic percentage.
	ProgressStageLLMToolInput = "llm_tool_input"
	// ProgressStageLLMWaitingAfterTools is emitted only after tool results and
	// continuation gates have committed and another model round will begin.
	ProgressStageLLMWaitingAfterTools = "llm_waiting_after_tools"
)

type GoalStatusEvent struct {
	Status    string                     `json:"status,omitempty"`
	Objective string                     `json:"objective,omitempty"`
	Revision  int                        `json:"revision,omitempty"`
	Criteria  []GoalCriterionStatusEvent `json:"criteria,omitempty"`
}

type GoalCriterionStatusEvent struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}
