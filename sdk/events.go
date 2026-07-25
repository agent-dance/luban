package sdk

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

// eventAdapter converts Runtime Event values to SDK output messages.
type eventAdapter struct {
	sessionID   string
	projectRoot string
	startTime   time.Time
	language    i18n.Language

	// accumulated text content for the current assistant turn
	textBuf   string
	turnCount int
}

func newEventAdapter(sessionID string, language i18n.Language) *eventAdapter {
	return &eventAdapter{
		sessionID: sessionID,
		startTime: time.Now(),
		language:  language,
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
	Type          string            `json:"type"` // "streamlined_tool_use_summary"
	ToolUseID     string            `json:"tool_use_id"`
	ToolName      string            `json:"tool_name,omitempty"`
	Status        string            `json:"status"` // "started" | "completed" | "error"
	Outcome       ToolOutcome       `json:"outcome,omitempty"`
	Input         map[string]any    `json:"input,omitempty"`
	OutputSummary string            `json:"output_summary,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Usage         *Usage            `json:"usage,omitempty"`
	UUID          string            `json:"uuid"`
	SessionID     string            `json:"session_id"`
	ProjectRoot   string            `json:"project_root,omitempty"`
	TurnID        string            `json:"turn_id,omitempty"`
	ActorID       string            `json:"actor_id,omitempty"`
	ActorType     string            `json:"actor_type,omitempty"`
	WorkUnitID    string            `json:"work_unit_id,omitempty"`
}

// StreamEventMsg is the typed form of a "stream_event" message.
type StreamEventMsg struct {
	Type        string `json:"type"` // "stream_event"
	Event       any    `json:"event"`
	UUID        string `json:"uuid"`
	SessionID   string `json:"session_id"`
	ProjectRoot string `json:"project_root,omitempty"`
}

// RuntimeEventPayload is the SDK-visible structured form of runtime events.
type RuntimeEventPayload struct {
	Type           string                `json:"type"`
	SessionID      string                `json:"session_id"`
	ProjectRoot    string                `json:"project_root,omitempty"`
	TurnID         string                `json:"turn_id,omitempty"`
	ActorID        string                `json:"actor_id,omitempty"`
	ActorType      string                `json:"actor_type,omitempty"`
	WorkUnitID     string                `json:"work_unit_id,omitempty"`
	Text           string                `json:"text,omitempty"`
	ToolUseID      string                `json:"tool_use_id,omitempty"`
	TerminalReason string                `json:"terminal_reason,omitempty"`
	Metadata       map[string]any        `json:"metadata,omitempty"`
	Compact        *CompactBoundaryEvent `json:"compact,omitempty"`
	MaxTurns       *MaxTurnsEvent        `json:"max_turns,omitempty"`
	Tombstone      *TombstoneEvent       `json:"tombstone,omitempty"`
	ToolSummary    *ToolUseSummaryEvent  `json:"tool_summary,omitempty"`
	HookSummary    *HookSummaryEvent     `json:"hook_summary,omitempty"`
	Progress       *RuntimeProgressEvent `json:"progress,omitempty"`
	RequestStatus  *RequestStatusEvent   `json:"request_status,omitempty"`
}

// process converts an Event into one or more SDK output values.
// It returns nil for event types that don't produce immediate output.
func (a *eventAdapter) process(ev Event) []any {
	// EventError.ProjectRoot is private diagnostic context and is intentionally
	// excluded from both the projection and adapter state. A normal SDK event
	// may establish the session project root for the final result envelope.
	if ev.ProjectRoot != "" && ev.Type != EventError && ev.Type != EventSystemWarning {
		a.projectRoot = ev.ProjectRoot
	}
	switch ev.Type {
	case EventText:
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

	case EventThinking:
		// Emit as a stream event with a thinking block — opaque to most clients.
		return []any{StreamEventMsg{
			Type:        "stream_event",
			Event:       runtimeEventPayload(a.sessionID, ev, a.language),
			UUID:        uuid.New().String(),
			SessionID:   a.sessionID,
			ProjectRoot: ev.ProjectRoot,
		}}

	case EventToolUse:
		if ev.ToolUse == nil {
			return nil
		}
		return []any{StreamlinedToolUseSummaryMsg{
			Type:        "streamlined_tool_use_summary",
			ToolUseID:   ev.ToolUse.ID,
			ToolName:    ev.ToolUse.Name,
			Status:      "started",
			Input:       streamlinedToolInput(ev.ToolUse.Input),
			UUID:        uuid.New().String(),
			SessionID:   a.sessionID,
			ProjectRoot: ev.ProjectRoot,
			TurnID:      ev.TurnID,
			ActorID:     ev.ActorID,
			ActorType:   ev.ActorType,
			WorkUnitID:  ev.WorkUnitID,
		}}

	case EventToolResult:
		if ev.ToolResult == nil || !authoritativeToolOutcome(ev.ToolResult.Outcome) {
			return nil
		}
		status := "completed"
		switch ev.ToolResult.Outcome {
		case ToolOutcomeFailed, ToolOutcomeDenied, ToolOutcomeCancelled, ToolOutcomeTimedOut:
			status = "error"
		}
		outputSummary, outputTruncated := streamlinedText(ev.ToolResult.Content, streamlinedOutputLimit)
		metadata := streamlinedMetadata(ev.ToolResult.Metadata)
		if outputTruncated {
			if metadata == nil {
				metadata = make(map[string]string, 2)
			}
			metadata["output_truncated"] = "true"
			metadata["output_bytes"] = strconv.Itoa(len(ev.ToolResult.Content))
		}
		return []any{StreamlinedToolUseSummaryMsg{
			Type:          "streamlined_tool_use_summary",
			ToolUseID:     ev.ToolResult.ToolUseID,
			Status:        status,
			Outcome:       ev.ToolResult.Outcome,
			OutputSummary: outputSummary,
			Metadata:      metadata,
			Usage:         ev.ToolResult.Usage,
			UUID:          uuid.New().String(),
			SessionID:     a.sessionID,
			ProjectRoot:   ev.ProjectRoot,
			TurnID:        ev.TurnID,
			ActorID:       ev.ActorID,
			ActorType:     ev.ActorType,
			WorkUnitID:    ev.WorkUnitID,
		}}

	case EventTurnEnd:
		a.turnCount = ev.TurnCount
		// Turn end doesn't produce an SDK message on its own — the final
		// SDKResultMessage is emitted separately by the transport layer.
		return nil

	case EventError:
		message, ok := a.runtimeErrorMessage(ev)
		if !ok {
			return nil
		}
		return []any{message}

	case EventSystemWarning:
		message, ok := a.runtimeWarningMessage(ev)
		if !ok {
			return nil
		}
		return []any{message}

	case EventToolUseSummary:
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

	case EventRequestStart,
		EventRequestRetry,
		EventRequestFirstToken,
		EventRequestEnd,
		EventRequestFailed:
		return []any{StreamEventMsg{
			Type:        "stream_event",
			Event:       runtimeEventPayload(a.sessionID, ev, a.language),
			UUID:        uuid.New().String(),
			SessionID:   a.sessionID,
			ProjectRoot: ev.ProjectRoot,
		}}

	case EventTombstone,
		EventCompactBoundary,
		EventMaxTurnsReached,
		EventUserInterruption,
		EventHookSummary,
		EventProgress:
		return []any{StreamEventMsg{
			Type:        "stream_event",
			Event:       runtimeEventPayload(a.sessionID, ev, a.language),
			UUID:        uuid.New().String(),
			SessionID:   a.sessionID,
			ProjectRoot: ev.ProjectRoot,
		}}
	}
	return nil
}

const (
	streamlinedInputStringLimit   = 512
	streamlinedOutputLimit        = 4096
	streamlinedMetadataValueLimit = 512
	streamlinedCollectionLimit    = 32
	streamlinedValueDepthLimit    = 4
)

// streamlinedToolInput keeps routing fields and small arguments visible while
// replacing large payloads such as Write.content and Edit.old_string with a
// stable size descriptor. A summary event must never duplicate entire files.
func streamlinedToolInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = streamlinedValue(value, 0)
	}
	return result
}

func streamlinedValue(value any, depth int) any {
	if depth >= streamlinedValueDepthLimit {
		return map[string]any{"omitted": true}
	}
	switch typed := value.(type) {
	case string:
		if len(typed) <= streamlinedInputStringLimit {
			return typed
		}
		return map[string]any{"omitted": true, "bytes": len(typed)}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > streamlinedCollectionLimit {
			return map[string]any{"omitted": true, "items": len(keys)}
		}
		result := make(map[string]any, len(typed))
		for _, key := range keys {
			result[key] = streamlinedValue(typed[key], depth+1)
		}
		return result
	case []any:
		if len(typed) > streamlinedCollectionLimit {
			return map[string]any{"omitted": true, "items": len(typed)}
		}
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = streamlinedValue(item, depth+1)
		}
		return result
	default:
		return value
	}
}

func streamlinedMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	result := make(map[string]string, len(metadata))
	omitted := make([]string, 0)
	for key, value := range metadata {
		if len(value) > streamlinedMetadataValueLimit {
			omitted = append(omitted, key)
			continue
		}
		result[key] = value
	}
	if len(omitted) > 0 {
		sort.Strings(omitted)
		result["truncated_metadata_keys"] = strings.Join(omitted, ",")
	}
	return result
}

func streamlinedText(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end], true
}

func authoritativeToolOutcome(outcome ToolOutcome) bool {
	switch outcome {
	case ToolOutcomeSucceeded, ToolOutcomeFailed, ToolOutcomePartial,
		ToolOutcomeDenied, ToolOutcomeCancelled, ToolOutcomeTimedOut:
		return true
	default:
		return false
	}
}

// runtimeWarningMessage projects semantic warning copy for the SDK. Raw
// warning fields are first folded into private diagnostics by the loop adapter,
// then excluded from the strict SDK envelope.
func (a *eventAdapter) runtimeWarningMessage(ev Event) (SDKSystemMessage, bool) {
	if ev.RuntimeEvent == nil {
		return SDKSystemMessage{}, false
	}
	warning := runtimeEventForProjection(*ev.RuntimeEvent)
	if warning.SessionID == "" {
		warning.SessionID = a.sessionID
	}
	projection, err := runtimeevent.NewAudienceProjector().Project(warning, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceSDK, Redaction: runtimeevent.RedactionStrict,
	})
	if err != nil {
		return SDKSystemMessage{}, false
	}
	return SDKSystemMessage{
		Type: "system", Subtype: "warning", Message: projection.Message,
		SessionID: projection.SessionID, TurnID: projection.TurnID, ToolUseID: projection.ToolUseID,
		ActorID: projection.ActorID, ActorType: projection.ActorType, WorkUnitID: projection.WorkUnitID,
		RuntimeEvent: sdkProjectedRuntimeEvent(projection),
	}, true
}

// runtimeErrorMessage retains the SDK system wire envelope while replacing its
// raw EventError payload with the versioned AudienceSDK/Public projection.
// Private provider errors, metadata, paths, and raw text require a separate,
// explicitly authorized audit/diagnostic channel.
func (a *eventAdapter) runtimeErrorMessage(ev Event) (SDKSystemMessage, bool) {
	identity := types.RuntimeIdentity{
		SessionID: a.sessionID, TurnID: ev.TurnID, ToolUseID: ev.ToolUseID,
		WorkUnitID: ev.WorkUnitID, ActorID: ev.ActorID, ActorType: ev.ActorType,
	}
	var apiErr *types.APIError
	if ev.Error != nil {
		apiErr = &types.APIError{
			Type: ev.Error.Type, Message: ev.Error.Message, Status: ev.Error.Status,
			RetryAfter: ev.Error.RetryAfter, OriginalModel: ev.Error.OriginalModel, FallbackModel: ev.Error.FallbackModel,
		}
	}
	event := runtimeevent.NewErrorEvent(identity, ev.Text, apiErr, ev.Metadata)
	projection, err := runtimeevent.NewAudienceProjector().Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceSDK, Redaction: runtimeevent.RedactionStrict,
	})
	if err != nil {
		return SDKSystemMessage{}, false
	}
	return SDKSystemMessage{
		Type: "system", Subtype: "error", Message: projection.Message,
		SessionID: projection.SessionID, TurnID: projection.TurnID, ToolUseID: projection.ToolUseID,
		ActorID: projection.ActorID, ActorType: projection.ActorType, WorkUnitID: projection.WorkUnitID,
		RuntimeEvent: sdkProjectedRuntimeEvent(projection),
	}, true
}

func sdkProjectedRuntimeEvent(source runtimeevent.ProjectedRuntimeEvent) *ProjectedRuntimeEvent {
	out := &ProjectedRuntimeEvent{
		Type: source.Type, SchemaVersion: source.SchemaVersion,
		Audience: string(source.Audience), RedactionLevel: string(source.RedactionLevel), Kind: string(source.Kind),
		EventID: source.EventID, SessionID: source.SessionID, Epoch: source.Epoch,
		ContextGeneration: source.ContextGeneration, TurnID: source.TurnID, ToolUseID: source.ToolUseID,
		WorkUnitID: source.WorkUnitID, ActorID: source.ActorID, ActorType: source.ActorType,
		Outcome: ToolOutcome(source.Outcome), Code: source.Code, Message: source.Message,
		PublicKey: string(source.PublicKey), PublicArgs: append([]any(nil), source.PublicArgs...),
	}
	if source.EvidenceRef != nil {
		out.EvidenceRef = &RuntimeEvidenceRef{ID: source.EvidenceRef.ID, Digest: source.EvidenceRef.Digest}
	}
	return out
}

func runtimeEventPayload(sessionID string, ev Event, language i18n.Language) RuntimeEventPayload {
	requestStatus := projectRequestStatus(ev.Type, ev.RequestStatus, language)
	payload := RuntimeEventPayload{
		Type:           string(ev.Type),
		SessionID:      sessionID,
		ProjectRoot:    ev.ProjectRoot,
		TurnID:         ev.TurnID,
		ActorID:        ev.ActorID,
		ActorType:      ev.ActorType,
		WorkUnitID:     ev.WorkUnitID,
		Text:           ev.Text,
		ToolUseID:      ev.ToolUseID,
		TerminalReason: ev.TerminalReason,
		Metadata:       ev.Metadata,
		Compact:        ev.Compact,
		MaxTurns:       ev.MaxTurns,
		Tombstone:      ev.Tombstone,
		ToolSummary:    ev.ToolSummary,
		HookSummary:    ev.HookSummary,
		Progress:       ev.Progress,
		RequestStatus:  requestStatus,
	}
	if requestStatus != nil {
		// Provider request lifecycle events have a deliberately narrow wire
		// shape. Ignore all unrelated carrier fields so a raw error copied into
		// Text or Metadata cannot bypass RequestStatus's semantic projection.
		payload.Text = ""
		payload.ToolUseID = ""
		payload.TerminalReason = ""
		payload.Metadata = nil
		payload.Compact = nil
		payload.MaxTurns = nil
		payload.Tombstone = nil
		payload.ToolSummary = nil
		payload.HookSummary = nil
		payload.Progress = nil
	}
	return payload
}

func projectRequestStatus(eventType EventType, source *RequestStatusEvent, language i18n.Language) *RequestStatusEvent {
	var phase, status string
	switch eventType {
	case EventRequestStart:
		phase, status = string(eventType), "started"
	case EventRequestRetry:
		phase, status = string(eventType), "retrying"
	case EventRequestFirstToken:
		phase, status = string(eventType), "streaming"
	case EventRequestEnd:
		phase, status = string(eventType), "completed"
	case EventRequestFailed:
		phase, status = string(eventType), "failed"
	default:
		return nil
	}
	cloned := RequestStatusEvent{Phase: phase, Status: status}
	if source != nil {
		cloned = *source
		cloned.Phase = phase
		cloned.Status = status
	}
	cloned.ErrorMessage = ""
	switch {
	case eventType == EventRequestRetry && cloned.ErrorCode == "provider_request_retry":
		cloned.ErrorMessage = i18n.Format(language, i18n.KeyRuntimeTransientAPIError, cloned.Attempt, cloned.MaxAttempts)
	case eventType == EventRequestFailed && cloned.ErrorCode == "provider_request_failed":
		cloned.ErrorMessage = i18n.Text(language, i18n.KeyRuntimeErrorPublicSummary)
	default:
		cloned.ErrorCode = ""
	}
	return &cloned
}

func runtimeEventForProjection(event RuntimeEvent) types.RuntimeEvent {
	projected := types.RuntimeEvent{
		SchemaVersion: event.SchemaVersion,
		Kind:          types.RuntimeEventKind(event.Kind),
		RuntimeIdentity: types.RuntimeIdentity{
			EventID: event.EventID, SessionID: event.SessionID, Epoch: event.Epoch,
			ContextGeneration: event.ContextGeneration, TurnID: event.TurnID,
			ToolUseID: event.ToolUseID, WorkUnitID: event.WorkUnitID,
			ActorID: event.ActorID, ActorType: event.ActorType,
		},
		Outcome: types.ToolOutcome(event.Outcome), PublicKey: i18n.Key(event.PublicKey),
		PublicArgs: append([]any(nil), event.PublicArgs...), DiagnosticCode: event.DiagnosticCode,
		PrivateCause: event.PrivateCause, PrivateMetadata: event.PrivateMetadata,
	}
	if event.EvidenceRef != nil {
		projected.EvidenceRef = &types.RuntimeEvidenceRef{ID: event.EvidenceRef.ID, Digest: event.EvidenceRef.Digest}
	}
	return projected
}

func userFacingError(lang i18n.Language, err error) string {
	if err == nil {
		return ""
	}
	var localized interface {
		Localized(i18n.Language) string
	}
	if errors.As(err, &localized) {
		return localized.Localized(lang)
	}
	return err.Error()
}

// resultMessage builds the terminal SDKResultMessage after a query finishes.
func (a *eventAdapter) resultMessage(lang i18n.Language, sessionID, msgUUID string, queryErr error) SDKResultMessage {
	subtype := "success"
	isError := false
	errs := []string(nil)

	if queryErr != nil {
		subtype = "error_during_execution"
		isError = true
		errs = []string{userFacingError(lang, queryErr)}
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
