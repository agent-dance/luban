package loop

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

// EventType represents the type of event emitted by the query loop.
type EventType string

const (
	EventRequestStart      EventType = "request_start"
	EventRequestRetry      EventType = "request_retry"
	EventRequestFirstToken EventType = "request_first_token"
	EventRequestEnd        EventType = "request_end"
	EventRequestFailed     EventType = "request_failed"
	EventText              EventType = "text"
	EventThinking          EventType = "thinking"
	EventToolUse           EventType = "tool_use"
	EventToolResult        EventType = "tool_result"
	EventToolUseSummary    EventType = "tool_use_summary"
	EventTurnEnd           EventType = "turn_end"
	EventProviderUsage     EventType = "provider_usage"
	EventContextUsage      EventType = "context_usage"
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

// Event represents an event from the query loop.
//
// The original fields remain valid for existing callers. New structured fields
// are optional so producers can add event detail without forcing consumers to
// change their switch statements.
type Event struct {
	Type        EventType              `json:"type"`
	Text        string                 `json:"text,omitempty"` // for text/thinking/error/warning events
	ToolUse     *types.ToolUseBlock    `json:"tool_use,omitempty"`
	ToolResult  *types.ToolResultBlock `json:"tool_result,omitempty"`
	Usage       *types.Usage           `json:"usage,omitempty"` // for turn_end and provider_usage events
	TurnCount   int                    `json:"turn_count,omitempty"`
	ProjectRoot string                 `json:"project_root,omitempty"`
	TurnID      string                 `json:"turn_id,omitempty"`
	ActorID     string                 `json:"actor_id,omitempty"`
	ActorType   string                 `json:"actor_type,omitempty"`
	WorkUnitID  string                 `json:"work_unit_id,omitempty"`

	Error          *types.APIError       `json:"error,omitempty"`
	MessageID      string                `json:"message_id,omitempty"`
	ToolUseID      string                `json:"tool_use_id,omitempty"`
	TerminalReason string                `json:"terminal_reason,omitempty"`
	Metadata       map[string]any        `json:"metadata,omitempty"`
	Compact        *CompactBoundaryEvent `json:"compact,omitempty"`
	MaxTurns       *MaxTurnsEvent        `json:"max_turns,omitempty"`
	Tombstone      *TombstoneEvent       `json:"tombstone,omitempty"`
	ToolSummary    *ToolUseSummaryEvent  `json:"tool_summary,omitempty"`
	HookSummary    *HookSummaryEvent     `json:"hook_summary,omitempty"`
	Progress       *ProgressEvent        `json:"progress,omitempty"`
	GoalStatus     *GoalStatusEvent      `json:"goal_status,omitempty"`
	RequestStatus  *RequestStatusEvent   `json:"request_status,omitempty"`
	ContextUsage   *ContextUsageEvent    `json:"context_usage,omitempty"`
	// RuntimeEvent is the sole warning/error authority for presentation-safe
	// projection. It is deliberately excluded from generic JSON so callers must
	// choose an audience and redaction level through runtimeevent.AudienceProjector.
	RuntimeEvent *types.RuntimeEvent `json:"-"`
}

// NewSystemWarningEvent creates a warning with semantic public copy and
// private diagnostic material. PublicArgs must contain only values explicitly
// intended for display; causes, paths, tokens, provider payloads, and other
// diagnostics belong in cause or metadata.
func NewSystemWarningEvent(publicKey i18n.Key, publicArgs []any, cause error, metadata map[string]any, turnCount int) Event {
	warning := runtimeevent.NewWarningEvent(types.RuntimeIdentity{}, publicKey, publicArgs, cause, metadata)
	return Event{Type: EventSystemWarning, TurnCount: turnCount, RuntimeEvent: &warning}
}

// SystemWarningRuntimeEvent returns the authoritative warning event. Legacy
// raw fields are accepted only as private diagnostics and are replaced by the
// generic warning key, so old producers fail closed instead of bypassing the
// audience projector.
func (e Event) SystemWarningRuntimeEvent() types.RuntimeEvent {
	if e.RuntimeEvent != nil && e.RuntimeEvent.Kind == types.RuntimeEventKindWarning {
		warning := *e.RuntimeEvent
		warning.RuntimeIdentity = mergeRuntimeWarningIdentity(warning.RuntimeIdentity, e)
		return warning
	}

	metadata := make(map[string]any, len(e.Metadata)+1)
	for key, value := range e.Metadata {
		metadata[key] = value
	}
	if e.ProjectRoot != "" {
		metadata["project_root"] = e.ProjectRoot
	}
	var cause error
	if e.Error != nil && e.Text != "" && e.Text != e.Error.Message {
		cause = errors.Join(e.Error, errors.New(e.Text))
	} else if e.Error != nil {
		cause = e.Error
	} else if e.Text != "" {
		cause = errors.New(e.Text)
	}
	return runtimeevent.NewWarningEvent(
		mergeRuntimeWarningIdentity(types.RuntimeIdentity{}, e),
		i18n.KeyRuntimeWarningPublicSummary,
		nil,
		cause,
		metadata,
	)
}

func mergeRuntimeWarningIdentity(identity types.RuntimeIdentity, event Event) types.RuntimeIdentity {
	if identity.TurnID == "" {
		identity.TurnID = event.TurnID
	}
	if identity.ToolUseID == "" {
		identity.ToolUseID = event.ToolUseID
	}
	if identity.WorkUnitID == "" {
		identity.WorkUnitID = event.WorkUnitID
	}
	if identity.ActorID == "" {
		identity.ActorID = event.ActorID
	}
	if identity.ActorType == "" {
		identity.ActorType = event.ActorType
	}
	return identity
}

// MarshalJSON rejects system warnings because Event has no audience authority.
// Callers must obtain SystemWarningRuntimeEvent and use AudienceProjector.
// Other event kinds retain the legacy JSON shape.
func (e Event) MarshalJSON() ([]byte, error) {
	if e.Type == EventSystemWarning {
		return nil, i18n.NewError(i18n.KeyRuntimeEventProjectionRejected)
	}
	type eventJSON Event
	return json.Marshal(eventJSON(e))
}

// ContextUsageEvent carries a scoped effective-input-context measurement.
// local_estimate values are never represented as provider-reported usage.
type ContextUsageEvent struct {
	UsedTokens       int      `json:"used_tokens"`
	CapacityTokens   int      `json:"capacity_tokens"`
	Measurement      string   `json:"measurement"`
	EstimateComplete bool     `json:"estimate_complete"`
	UnknownOverheads []string `json:"unknown_overheads,omitempty"`
}

// RequestStatusEvent carries the live lifecycle and timing of one LLM API
// request. Durations are measured from the start of the API call so consumers
// can render request setup, time to first token, and total response time.
type RequestStatusEvent struct {
	RequestID              string `json:"request_id"`
	Attempt                int    `json:"attempt,omitempty"`
	MaxRetries             int    `json:"max_retries,omitempty"`
	RetryDelayMilliseconds int64  `json:"retry_delay_ms,omitempty"`
	RequestMilliseconds    int64  `json:"request_ms,omitempty"`
	FirstTokenMilliseconds int64  `json:"first_token_ms,omitempty"`
	TotalMilliseconds      int64  `json:"total_ms,omitempty"`
	Error                  string `json:"error,omitempty"`
}

// MaxTurnsEvent carries the terminal details for a max-turn stop.
type MaxTurnsEvent struct {
	MaxTurns  int `json:"max_turns"`
	TurnCount int `json:"turn_count"`
}

// CompactBoundaryEvent marks a context compaction boundary visible to UI/SDK
// consumers without requiring them to parse model-visible boundary messages.
type CompactBoundaryEvent struct {
	Trigger                   string                            `json:"trigger,omitempty"`
	PreCompactTokenCount      int                               `json:"pre_compact_token_count,omitempty"`
	PostCompactTokenCount     int                               `json:"post_compact_token_count,omitempty"`
	TruePostCompactTokenCount int                               `json:"true_post_compact_token_count,omitempty"`
	PreviousTailIdentifier    string                            `json:"previous_tail_identifier,omitempty"`
	PreCompactDiscoveredTools []string                          `json:"pre_compact_discovered_tools,omitempty"`
	PreservedSegment          *compact.PreservedSegmentMetadata `json:"preserved_segment,omitempty"`
	Summary                   string                            `json:"summary,omitempty"`
	UserDisplayMessage        string                            `json:"user_display_message,omitempty"`
}

// TombstoneEvent is a structured carrier for future fallback/tombstone work.
// It intentionally does not implement fallback behavior.
type TombstoneEvent struct {
	MessageID string         `json:"message_id,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ToolUseSummaryEvent summarizes a tool call without requiring callers to read
// a full tool_result content block.
type ToolUseSummaryEvent struct {
	ToolUseID     string `json:"tool_use_id,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	Status        string `json:"status,omitempty"`
	OutputSummary string `json:"output_summary,omitempty"`
}

// HookSummaryEvent is a structured UI/SDK carrier for hook execution summaries.
type HookSummaryEvent struct {
	HookExecutionID string         `json:"hook_execution_id,omitempty"`
	ToolUseID       string         `json:"tool_use_id,omitempty"`
	HookName        string         `json:"hook_name,omitempty"`
	Status          string         `json:"status,omitempty"`
	Summary         string         `json:"summary,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// ProgressEvent carries non-terminal progress updates.
type ProgressEvent struct {
	Stage    string         `json:"stage,omitempty"`
	Message  string         `json:"message,omitempty"`
	Current  int            `json:"current,omitempty"`
	Total    int            `json:"total,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// GoalStatusEvent is the presentation-safe projection of the session goal at
// a query boundary. An empty status/objective represents an absent goal.
type GoalStatusEvent struct {
	Status    string                     `json:"status,omitempty"`
	Objective string                     `json:"objective,omitempty"`
	Revision  int                        `json:"revision,omitempty"`
	Criteria  []GoalCriterionStatusEvent `json:"criteria,omitempty"`
}

// GoalCriterionStatusEvent is safe for direct TUI projection. Text and Reason
// remain Agent/evaluator-authored raw data; Status is a stable protocol value.
type GoalCriterionStatusEvent struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func newMaxTurnsReachedEvent(maxTurns, turnCount int) Event {
	return Event{
		Type:           EventMaxTurnsReached,
		TurnCount:      turnCount,
		TerminalReason: "max_turns_reached",
		MaxTurns:       &MaxTurnsEvent{MaxTurns: maxTurns, TurnCount: turnCount},
		Metadata: map[string]any{
			"max_turns":  maxTurns,
			"turn_count": turnCount,
		},
	}
}

func newGoalEvaluationEvent(usage *types.Usage, turnCount int, model string) Event {
	usageCopy := *usage
	metadata := map[string]any{"kind": "goal_evaluator"}
	if model = strings.TrimSpace(model); model != "" {
		metadata["model"] = model
	}
	return Event{
		Type:      EventGoalEvaluation,
		Usage:     &usageCopy,
		TurnCount: turnCount,
		Metadata:  metadata,
	}
}

func newGoalStatusEvent(current *goal.Goal, turnCount int) Event {
	projection := &GoalStatusEvent{}
	if current != nil {
		normalized := goal.Normalize(*current)
		projection.Status = string(normalized.Status)
		projection.Objective = normalized.Objective
		projection.Revision = normalized.Revision
		results := make(map[string]goal.AcceptanceCriterionEvaluation)
		if normalized.LastAcceptanceEvaluation != nil && normalized.LastAcceptanceEvaluation.Revision == normalized.Revision {
			for _, result := range normalized.LastAcceptanceEvaluation.Criteria {
				results[strings.ToUpper(result.CriterionID)] = result
			}
		}
		for _, criterion := range normalized.AcceptanceCriteria {
			item := GoalCriterionStatusEvent{ID: criterion.ID, Text: criterion.Text, Status: "pending"}
			if result, ok := results[strings.ToUpper(criterion.ID)]; ok {
				item.Reason = result.Reason
				item.Status = "unmet"
				if result.Met {
					item.Status = "met"
				}
			}
			projection.Criteria = append(projection.Criteria, item)
		}
	}
	return Event{Type: EventGoalStatus, TurnCount: turnCount, GoalStatus: projection}
}

func newCompactBoundaryEvent(result *compact.CompactionResult, trigger string, turnCount int) Event {
	evt := Event{
		Type:      EventCompactBoundary,
		TurnCount: turnCount,
		Compact: &CompactBoundaryEvent{
			Trigger: trigger,
		},
	}
	if result != nil {
		evt.Compact.PostCompactTokenCount = result.PostCompactTokenCount
		evt.Compact.TruePostCompactTokenCount = result.TruePostCompactTokenCount
		evt.Compact.Summary = compactMessagesText(result.SummaryMessages)
		evt.Compact.UserDisplayMessage = result.UserDisplayMessage
		if result.BoundaryMarker != nil {
			if metadata, ok := compact.ParseCompactBoundaryMessage(*result.BoundaryMarker); ok {
				evt.Compact.Trigger = metadata.Trigger
				evt.Compact.PreCompactTokenCount = metadata.PreCompactTokenCount
				evt.Compact.PreviousTailIdentifier = metadata.PreviousTailIdentifier
				evt.Compact.PreCompactDiscoveredTools = append([]string(nil), metadata.PreCompactDiscoveredTools...)
				if metadata.PreservedSegment != nil {
					preserved := *metadata.PreservedSegment
					evt.Compact.PreservedSegment = &preserved
				}
			}
		}
	}
	if evt.Compact.Trigger == "" {
		evt.Compact.Trigger = trigger
	}
	evt.Metadata = map[string]any{
		"trigger": evt.Compact.Trigger,
	}
	return evt
}

func compactMessagesText(messages []types.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if text := strings.TrimSpace(message.GetText()); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}
