package sdk

import (
	"time"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

// eventAdapter converts loop.Event values to SDK output messages.
// Call process for each non-Final engine.Event.Inner.
type eventAdapter struct {
	sessionID   string
	projectRoot string
	startTime   time.Time

	// accumulated text content for the current assistant turn
	textBuf   string
	turnCount int
}

func newEventAdapter(sessionID string) *eventAdapter {
	return &eventAdapter{
		sessionID: sessionID,
		startTime: time.Now(),
	}
}

// ─── typed event message structs ─────────────────────────────────────────────

// StreamlinedTextMsg is the typed form of a "streamlined_text" message.
type StreamlinedTextMsg struct {
	Type        string `json:"type"` // "streamlined_text"
	Text        string `json:"text"`
	UUID        string `json:"uuid"`
	SessionID   string `json:"session_id"`
	ProjectRoot string `json:"project_root,omitempty"`
	TurnID      string `json:"turn_id,omitempty"`
	ActorID     string `json:"actor_id,omitempty"`
	ActorType   string `json:"actor_type,omitempty"`
	WorkUnitID  string `json:"work_unit_id,omitempty"`
}

// StreamlinedToolUseSummaryMsg is the typed form of "streamlined_tool_use_summary".
type StreamlinedToolUseSummaryMsg struct {
	Type          string               `json:"type"` // "streamlined_tool_use_summary"
	ToolUseID     string               `json:"tool_use_id"`
	ToolName      string               `json:"tool_name,omitempty"`
	Status        string               `json:"status"` // "started" | "completed" | "error"
	Outcome       types.ToolOutcome    `json:"outcome,omitempty"`
	Input         map[string]any       `json:"input,omitempty"`
	OutputSummary string               `json:"output_summary,omitempty"`
	ContentBlocks []types.ContentBlock `json:"content_blocks,omitempty"`
	Data          any                  `json:"data,omitempty"`
	Metadata      map[string]string    `json:"metadata,omitempty"`
	Usage         *types.Usage         `json:"usage,omitempty"`
	UUID          string               `json:"uuid"`
	SessionID     string               `json:"session_id"`
	ProjectRoot   string               `json:"project_root,omitempty"`
	TurnID        string               `json:"turn_id,omitempty"`
	ActorID       string               `json:"actor_id,omitempty"`
	ActorType     string               `json:"actor_type,omitempty"`
	WorkUnitID    string               `json:"work_unit_id,omitempty"`
}

// StreamEventMsg is the typed form of a "stream_event" message.
type StreamEventMsg struct {
	Type        string `json:"type"` // "stream_event"
	Event       any    `json:"event"`
	UUID        string `json:"uuid"`
	SessionID   string `json:"session_id"`
	ProjectRoot string `json:"project_root,omitempty"`
}

// LoopEventPayload is the SDK-visible structured form of loop-only events.
type LoopEventPayload struct {
	Type           string                     `json:"type"`
	SessionID      string                     `json:"session_id"`
	ProjectRoot    string                     `json:"project_root,omitempty"`
	TurnID         string                     `json:"turn_id,omitempty"`
	ActorID        string                     `json:"actor_id,omitempty"`
	ActorType      string                     `json:"actor_type,omitempty"`
	WorkUnitID     string                     `json:"work_unit_id,omitempty"`
	Text           string                     `json:"text,omitempty"`
	MessageID      string                     `json:"message_id,omitempty"`
	ToolUseID      string                     `json:"tool_use_id,omitempty"`
	TerminalReason string                     `json:"terminal_reason,omitempty"`
	Metadata       map[string]any             `json:"metadata,omitempty"`
	Compact        *loop.CompactBoundaryEvent `json:"compact,omitempty"`
	MaxTurns       *loop.MaxTurnsEvent        `json:"max_turns,omitempty"`
	Tombstone      *loop.TombstoneEvent       `json:"tombstone,omitempty"`
	ToolSummary    *loop.ToolUseSummaryEvent  `json:"tool_summary,omitempty"`
	HookSummary    *loop.HookSummaryEvent     `json:"hook_summary,omitempty"`
	Progress       *loop.ProgressEvent        `json:"progress,omitempty"`
}

// process converts a loop.Event into one or more SDK output values.
// It returns nil for event types that don't produce immediate output.
func (a *eventAdapter) process(ev loop.Event) []any {
	// EventError.ProjectRoot is private diagnostic context and is intentionally
	// excluded from both the projection and adapter state. A normal SDK event
	// may establish the session project root for the final result envelope.
	if ev.ProjectRoot != "" && ev.Type != loop.EventError && ev.Type != loop.EventSystemWarning {
		a.projectRoot = ev.ProjectRoot
	}
	switch ev.Type {
	case loop.EventText:
		a.textBuf += ev.Text
		// Emit a streamlined text message so clients can stream token-by-token.
		return []any{StreamlinedTextMsg{
			Type:        "streamlined_text",
			Text:        ev.Text,
			UUID:        uuid.New().String(),
			SessionID:   a.sessionID,
			ProjectRoot: ev.ProjectRoot,
			TurnID:      ev.TurnID,
			ActorID:     ev.ActorID,
			ActorType:   ev.ActorType,
			WorkUnitID:  ev.WorkUnitID,
		}}

	case loop.EventThinking:
		// Emit as a stream event with a thinking block — opaque to most clients.
		return []any{StreamEventMsg{
			Type:        "stream_event",
			Event:       loopEventPayload(a.sessionID, ev),
			UUID:        uuid.New().String(),
			SessionID:   a.sessionID,
			ProjectRoot: ev.ProjectRoot,
		}}

	case loop.EventToolUse:
		if ev.ToolUse == nil {
			return nil
		}
		return []any{StreamlinedToolUseSummaryMsg{
			Type:        "streamlined_tool_use_summary",
			ToolUseID:   ev.ToolUse.ID,
			ToolName:    ev.ToolUse.Name,
			Status:      "started",
			Input:       ev.ToolUse.Input,
			UUID:        uuid.New().String(),
			SessionID:   a.sessionID,
			ProjectRoot: ev.ProjectRoot,
			TurnID:      ev.TurnID,
			ActorID:     ev.ActorID,
			ActorType:   ev.ActorType,
			WorkUnitID:  ev.WorkUnitID,
		}}

	case loop.EventToolResult:
		if ev.ToolResult == nil {
			return nil
		}
		status := "completed"
		if ev.ToolResult.IsError {
			status = "error"
		}
		return []any{StreamlinedToolUseSummaryMsg{
			Type:          "streamlined_tool_use_summary",
			ToolUseID:     ev.ToolResult.ToolUseID,
			Status:        status,
			Outcome:       ev.ToolResult.Outcome,
			OutputSummary: ev.ToolResult.TextContent(),
			ContentBlocks: ev.ToolResult.ContentBlocks,
			Data:          ev.ToolResult.Data,
			Metadata:      ev.ToolResult.Metadata,
			Usage:         ev.ToolResult.Usage,
			UUID:          uuid.New().String(),
			SessionID:     a.sessionID,
			ProjectRoot:   ev.ProjectRoot,
			TurnID:        ev.TurnID,
			ActorID:       ev.ActorID,
			ActorType:     ev.ActorType,
			WorkUnitID:    ev.WorkUnitID,
		}}

	case loop.EventTurnEnd:
		a.turnCount = ev.TurnCount
		// Turn end doesn't produce an SDK message on its own — the final
		// SDKResultMessage is emitted separately by the transport layer.
		return nil

	case loop.EventError:
		return []any{a.runtimeErrorMessage(ev)}

	case loop.EventSystemWarning:
		return []any{a.runtimeWarningMessage(ev)}

	case loop.EventToolUseSummary:
		if ev.ToolSummary == nil {
			return nil
		}
		return []any{StreamlinedToolUseSummaryMsg{
			Type:          "streamlined_tool_use_summary",
			ToolUseID:     ev.ToolSummary.ToolUseID,
			ToolName:      ev.ToolSummary.ToolName,
			Status:        ev.ToolSummary.Status,
			OutputSummary: ev.ToolSummary.OutputSummary,
			UUID:          uuid.New().String(),
			SessionID:     a.sessionID,
			ProjectRoot:   ev.ProjectRoot,
			TurnID:        ev.TurnID,
			ActorID:       ev.ActorID,
			ActorType:     ev.ActorType,
			WorkUnitID:    ev.WorkUnitID,
		}}

	case loop.EventRequestStart,
		loop.EventRequestRetry,
		loop.EventRequestFirstToken,
		loop.EventRequestEnd,
		loop.EventRequestFailed:
		// Request timing is interactive terminal chrome, not transcript output.
		return nil

	case loop.EventTombstone,
		loop.EventCompactBoundary,
		loop.EventMaxTurnsReached,
		loop.EventUserInterruption,
		loop.EventHookSummary,
		loop.EventProgress:
		return []any{StreamEventMsg{
			Type:        "stream_event",
			Event:       loopEventPayload(a.sessionID, ev),
			UUID:        uuid.New().String(),
			SessionID:   a.sessionID,
			ProjectRoot: ev.ProjectRoot,
		}}
	}
	return nil
}

// runtimeWarningMessage projects semantic warning copy for the SDK. Legacy
// raw warning fields are first folded into private diagnostics by the loop
// adapter, then excluded from the strict SDK envelope.
func (a *eventAdapter) runtimeWarningMessage(ev loop.Event) SDKSystemMessage {
	warning := ev.SystemWarningRuntimeEvent()
	if warning.SessionID == "" {
		warning.SessionID = a.sessionID
	}
	projection, err := runtimeevent.NewAudienceProjector().Project(warning, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceSDK, Redaction: runtimeevent.RedactionStrict,
	})
	if err != nil {
		return SDKSystemMessage{
			Type: "system", Subtype: "warning",
			Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeWarningPublicSummary),
		}
	}
	return SDKSystemMessage{
		Type: "system", Subtype: "warning", Message: projection.Message,
		SessionID: projection.SessionID, TurnID: projection.TurnID, ToolUseID: projection.ToolUseID,
		ActorID: projection.ActorID, ActorType: projection.ActorType, WorkUnitID: projection.WorkUnitID,
		RuntimeEvent: &projection,
	}
}

// runtimeErrorMessage retains the legacy SDK system envelope while replacing
// its raw EventError payload with the versioned AudienceSDK/Public projection.
// Private provider errors, metadata, paths, and raw text require a separate,
// explicitly authorized audit/diagnostic channel.
func (a *eventAdapter) runtimeErrorMessage(ev loop.Event) SDKSystemMessage {
	identity := types.RuntimeIdentity{
		SessionID: a.sessionID, TurnID: ev.TurnID, ToolUseID: ev.ToolUseID,
		WorkUnitID: ev.WorkUnitID, ActorID: ev.ActorID, ActorType: ev.ActorType,
	}
	event := runtimeevent.NewErrorEvent(identity, ev.Text, ev.Error, ev.Metadata)
	projection, err := runtimeevent.NewAudienceProjector().Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceSDK, Redaction: runtimeevent.RedactionPublic,
	})
	if err != nil {
		return SDKSystemMessage{
			Type: "system", Subtype: "error",
			Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary),
		}
	}
	return SDKSystemMessage{
		Type: "system", Subtype: "error", Message: projection.Message,
		SessionID: projection.SessionID, TurnID: projection.TurnID, ToolUseID: projection.ToolUseID,
		ActorID: projection.ActorID, ActorType: projection.ActorType, WorkUnitID: projection.WorkUnitID,
		RuntimeEvent: &projection,
	}
}

func loopEventPayload(sessionID string, ev loop.Event) LoopEventPayload {
	payload := LoopEventPayload{
		Type:           string(ev.Type),
		SessionID:      sessionID,
		ProjectRoot:    ev.ProjectRoot,
		TurnID:         ev.TurnID,
		ActorID:        ev.ActorID,
		ActorType:      ev.ActorType,
		WorkUnitID:     ev.WorkUnitID,
		Text:           ev.Text,
		MessageID:      ev.MessageID,
		ToolUseID:      ev.ToolUseID,
		TerminalReason: ev.TerminalReason,
		Metadata:       ev.Metadata,
		Compact:        ev.Compact,
		MaxTurns:       ev.MaxTurns,
		Tombstone:      ev.Tombstone,
		ToolSummary:    ev.ToolSummary,
		HookSummary:    ev.HookSummary,
		Progress:       ev.Progress,
	}
	if payload.ToolUseID == "" && ev.ToolUse != nil {
		payload.ToolUseID = ev.ToolUse.ID
	}
	if payload.ToolUseID == "" && ev.ToolResult != nil {
		payload.ToolUseID = ev.ToolResult.ToolUseID
	}
	return payload
}

// resultMessage builds the terminal SDKResultMessage after a query finishes.
func (a *eventAdapter) resultMessage(lang i18n.Language, sessionID, msgUUID string, queryErr error) SDKResultMessage {
	subtype := "success"
	isError := false
	errs := []string(nil)

	if queryErr != nil {
		subtype = "error_during_execution"
		isError = true
		errs = []string{engine.UserFacingError(lang, queryErr)}
	}

	return SDKResultMessage{
		Type:        "result",
		Subtype:     subtype,
		SessionID:   sessionID,
		ProjectRoot: a.projectRoot,
		UUID:        msgUUID,
		IsError:     isError,
		Result:      a.textBuf,
		NumTurns:    a.turnCount,
		DurationMs:  float64(time.Since(a.startTime).Milliseconds()),
		Errors:      errs,
	}
}
