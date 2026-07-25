package sdk

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestEventAdapterStructuredRuntimeEvents(t *testing.T) {
	a := newEventAdapter("session-structured", i18n.LangEN)

	tests := []struct {
		name      string
		event     Event
		wantType  string
		assertion func(*testing.T, RuntimeEventPayload)
	}{
		{
			name: "max turns",
			event: Event{
				Type:           EventMaxTurnsReached,
				TerminalReason: "max_turns_reached",
				MaxTurns:       &MaxTurnsEvent{MaxTurns: 3, TurnCount: 4},
			},
			wantType: "max_turns_reached",
			assertion: func(t *testing.T, payload RuntimeEventPayload) {
				t.Helper()
				if payload.MaxTurns == nil || payload.MaxTurns.MaxTurns != 3 || payload.MaxTurns.TurnCount != 4 {
					t.Fatalf("MaxTurns payload = %+v, want 3/4", payload.MaxTurns)
				}
				if payload.TerminalReason != "max_turns_reached" {
					t.Fatalf("TerminalReason = %q, want max_turns_reached", payload.TerminalReason)
				}
			},
		},
		{
			name: "compact boundary",
			event: Event{
				Type:    EventCompactBoundary,
				Compact: &CompactBoundaryEvent{Trigger: "auto", PreCompactTokenCount: 1200},
			},
			wantType: "compact_boundary",
			assertion: func(t *testing.T, payload RuntimeEventPayload) {
				t.Helper()
				if payload.Compact == nil || payload.Compact.Trigger != "auto" {
					t.Fatalf("Compact payload = %+v, want trigger auto", payload.Compact)
				}
			},
		},
		{
			name: "tombstone",
			event: Event{
				Type: EventTombstone,
				Tombstone: &TombstoneEvent{
					Reason: "fallback",
				},
			},
			wantType: "tombstone",
			assertion: func(t *testing.T, payload RuntimeEventPayload) {
				t.Helper()
				if payload.Tombstone == nil || payload.Tombstone.Reason != "fallback" {
					t.Fatalf("Tombstone payload = %+v, want fallback", payload.Tombstone)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			results := a.process(tt.event)
			if len(results) != 1 {
				t.Fatalf("results len = %d, want 1", len(results))
			}
			msg, ok := results[0].(StreamEventMsg)
			if !ok {
				t.Fatalf("result = %T, want StreamEventMsg", results[0])
			}
			payload, ok := msg.Event.(RuntimeEventPayload)
			if !ok {
				t.Fatalf("Event = %T, want RuntimeEventPayload", msg.Event)
			}
			if payload.Type != tt.wantType {
				t.Fatalf("payload.Type = %q, want %q", payload.Type, tt.wantType)
			}
			tt.assertion(t, payload)
		})
	}
}

func TestEventAdapter_ToolUseSummaryEvent(t *testing.T) {
	a := newEventAdapter("session-tool-summary", i18n.LangEN)

	results := a.process(Event{
		Type: EventToolUseSummary,
		ToolSummary: &ToolUseSummaryEvent{
			ToolUseID:     "toolu_123",
			ToolName:      "Bash",
			Status:        "completed",
			OutputSummary: "wrote file",
		},
	})
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	msg, ok := results[0].(StreamlinedToolUseSummaryMsg)
	if !ok {
		t.Fatalf("result = %T, want StreamlinedToolUseSummaryMsg", results[0])
	}
	if msg.ToolUseID != "toolu_123" || msg.ToolName != "Bash" || msg.Status != "completed" || msg.OutputSummary != "wrote file" {
		t.Fatalf("unexpected tool summary: %+v", msg)
	}
}

func TestEventAdapter_ToolEventsPreserveStableIdentityAndExplicitOutcome(t *testing.T) {
	a := newEventAdapter("session-sdk", i18n.LangEN)
	callEvent := Event{
		Type: EventToolUse, TurnID: "session-sdk:query-a:turn-2", ActorID: "agent-a", ActorType: "reviewer", WorkUnitID: "review-a",
		ToolUse: &ToolUse{ID: "toolu-sdk", Name: "Read", Input: map[string]any{"file_path": "/tmp/full"}},
	}
	call := a.process(callEvent)[0].(StreamlinedToolUseSummaryMsg)
	if call.TurnID != callEvent.TurnID || call.ActorID != "agent-a" || call.ActorType != "reviewer" || call.WorkUnitID != "review-a" || call.SessionID != "session-sdk" {
		t.Fatalf("tool call identity = %+v", call)
	}
	if call.Input["file_path"] != "/tmp/full" {
		t.Fatalf("tool call input = %#v", call.Input)
	}

	resultEvent := callEvent
	resultEvent.Type = EventToolResult
	resultEvent.ToolUse = nil
	resultEvent.ToolResult = &ToolResult{
		ToolUseID: "toolu-sdk", Content: "partial but usable", Outcome: ToolOutcomePartial,
		Metadata: map[string]string{"source": "disk"},
	}
	result := a.process(resultEvent)[0].(StreamlinedToolUseSummaryMsg)
	if result.TurnID != resultEvent.TurnID || result.ActorID != "agent-a" || result.ActorType != "reviewer" || result.WorkUnitID != "review-a" || result.SessionID != "session-sdk" {
		t.Fatalf("tool result identity = %+v", result)
	}
	if result.Outcome != ToolOutcomePartial || result.OutputSummary != "partial but usable" || result.Metadata["source"] != "disk" {
		t.Fatalf("tool result evidence/outcome = %+v", result)
	}
}

func TestEventAdapterStreamlinedToolPayloadsAreBounded(t *testing.T) {
	a := newEventAdapter("session-sdk-bounded", i18n.LangEN)
	large := strings.Repeat("界", 3_000)

	call := a.process(Event{
		Type: EventToolUse,
		ToolUse: &ToolUse{
			ID:   "toolu-write",
			Name: "Write",
			Input: map[string]any{
				"file_path": "/tmp/game.js",
				"content":   large,
			},
		},
	})[0].(StreamlinedToolUseSummaryMsg)
	if call.Input["file_path"] != "/tmp/game.js" {
		t.Fatalf("small routing field was not preserved: %#v", call.Input)
	}
	descriptor, ok := call.Input["content"].(map[string]any)
	if !ok || descriptor["omitted"] != true || descriptor["bytes"] != len(large) {
		t.Fatalf("large input descriptor = %#v", call.Input["content"])
	}

	result := a.process(Event{
		Type: EventToolResult,
		ToolResult: &ToolResult{
			ToolUseID:     "toolu-write",
			Outcome:       ToolOutcomeSucceeded,
			Content:       large,
			ContentBlocks: []any{map[string]any{"text": large}},
			Data:          map[string]any{"original_file": large},
			Metadata: map[string]string{
				"exitCode": "0",
				"stdout":   large,
			},
		},
	})[0].(StreamlinedToolUseSummaryMsg)
	if len(result.OutputSummary) > streamlinedOutputLimit || result.Metadata["output_truncated"] != "true" {
		t.Fatalf("bounded output = %d bytes, metadata %#v", len(result.OutputSummary), result.Metadata)
	}
	if result.Metadata["exitCode"] != "0" || result.Metadata["truncated_metadata_keys"] != "stdout" {
		t.Fatalf("bounded metadata = %#v", result.Metadata)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes := len(encoded); bytes > 6_000 {
		t.Fatalf("streamlined result grew to %d bytes", bytes)
	}
	if strings.Contains(string(encoded), "content_blocks") || strings.Contains(string(encoded), "original_file") {
		t.Fatalf("streamlined result leaked full structured payload: %s", encoded)
	}
}

func TestEventAdapterRejectsToolResultWithoutAuthoritativeOutcome(t *testing.T) {
	a := newEventAdapter("session-sdk-missing-outcome", i18n.LangEN)
	results := a.process(Event{Type: EventToolResult, ToolResult: &ToolResult{
		ToolUseID: "toolu-missing-outcome", Content: "untyped failure",
	}})
	if len(results) != 0 {
		t.Fatalf("missing-outcome tool result emitted SDK message: %#v", results)
	}
}

func TestEventAdapterToolResultStatusUsesAuthoritativeOutcome(t *testing.T) {
	a := newEventAdapter("session-sdk-outcome", i18n.LangEN)
	results := a.process(Event{Type: EventToolResult, ToolResult: &ToolResult{
		ToolUseID: "toolu-outcome", Outcome: ToolOutcomeSucceeded,
	}})
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	message := results[0].(StreamlinedToolUseSummaryMsg)
	if message.Status != "completed" || message.Outcome != ToolOutcomeSucceeded {
		t.Fatalf("tool result status = %+v, want authoritative success", message)
	}
}

func TestEventAdapterTextAndRuntimePayloadPreserveStableIdentity(t *testing.T) {
	a := newEventAdapter("session-sdk-identity", i18n.LangEN)
	identity := Event{TurnID: "session-sdk-identity:query-a:turn-4", ActorID: "agent-b", ActorType: "executor", WorkUnitID: "work-b"}
	textEvent := identity
	textEvent.Type, textEvent.Text = EventText, "token"
	text := a.process(textEvent)[0].(StreamlinedTextMsg)
	if text.SessionID != "session-sdk-identity" || text.TurnID != identity.TurnID || text.ActorID != identity.ActorID || text.ActorType != identity.ActorType || text.WorkUnitID != identity.WorkUnitID {
		t.Fatalf("text identity = %+v", text)
	}

	progressEvent := identity
	progressEvent.Type = EventProgress
	progressEvent.ToolUseID = "toolu-progress"
	progressEvent.Progress = &RuntimeProgressEvent{Stage: "verify", Current: 1, Total: 2}
	stream := a.process(progressEvent)[0].(StreamEventMsg)
	payload := stream.Event.(RuntimeEventPayload)
	if payload.SessionID != "session-sdk-identity" || payload.TurnID != identity.TurnID || payload.ActorID != identity.ActorID || payload.ActorType != identity.ActorType || payload.WorkUnitID != identity.WorkUnitID || payload.ToolUseID != "toolu-progress" {
		t.Fatalf("loop payload identity = %+v", payload)
	}
}

func TestEventAdapterRuntimeErrorUsesSDKPublicProjection(t *testing.T) {
	a := newEventAdapter("session-sdk-error", i18n.LangEN)
	secret := "/workspace/private/.env token=sk-sdk-secret"
	event := Event{
		Type: EventError, Text: secret, ToolUseID: "toolu-error",
		ProjectRoot: "/workspace/project", TurnID: "session-sdk-error:query-a:turn-5", ActorID: "agent-error", ActorType: "executor", WorkUnitID: "work-error",
		Error:    &APIError{Type: "private_provider_error", Message: secret, Status: 500},
		Metadata: map[string]any{"authorization": "Bearer private-token"},
	}
	message := a.process(event)[0].(SDKSystemMessage)
	if message.Type != "system" || message.Subtype != "error" {
		t.Fatalf("SDK system envelope = %+v", message)
	}
	if message.SessionID != "session-sdk-error" || message.TurnID != event.TurnID || message.ActorID != event.ActorID || message.ActorType != event.ActorType || message.WorkUnitID != event.WorkUnitID || message.ToolUseID != event.ToolUseID {
		t.Fatalf("error identity = %+v", message)
	}
	projection := message.RuntimeEvent
	if projection == nil || projection.SchemaVersion != types.RuntimeEventSchemaVersion ||
		projection.Audience != "sdk" || projection.RedactionLevel != "strict" ||
		projection.Kind != string(types.RuntimeEventKindError) || projection.Outcome != ToolOutcomeFailed ||
		projection.EventID == "" || projection.SessionID != message.SessionID || projection.TurnID != message.TurnID ||
		projection.ToolUseID != message.ToolUseID || projection.ActorID != message.ActorID ||
		projection.ActorType != message.ActorType || projection.WorkUnitID != message.WorkUnitID {
		t.Fatalf("SDK runtime-event projection = %#v", projection)
	}
	if message.Message != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary) || projection.Message != message.Message {
		t.Fatalf("SDK public error message = %q, projection = %#v", message.Message, projection)
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"project_root", "error", "metadata"} {
		if _, present := wire[forbidden]; present {
			t.Fatalf("default SDK error exposed forbidden field %q: %s", forbidden, encoded)
		}
	}
	for _, private := range []string{secret, "sk-sdk-secret", event.ProjectRoot, "private_provider_error", "private-token", "authorization", "private_cause", "private_metadata"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("default SDK runtime-error projection leaked %q: %s", private, encoded)
		}
	}
	if result := a.resultMessage(i18n.LangEN, "session-sdk-error", "query-error", nil); result.ProjectRoot != "" {
		t.Fatalf("EventError project_root escaped through final result state: %q", result.ProjectRoot)
	}
}

func TestEventAdapterDropsInvalidRuntimeWarningInsteadOfEmittingUntypedEnvelope(t *testing.T) {
	a := newEventAdapter("session-sdk-invalid-warning", i18n.LangEN)
	invalid := RuntimeEvent{Kind: string(types.RuntimeEventKindWarning)}
	results := a.process(Event{Type: EventSystemWarning, RuntimeEvent: &invalid})
	if len(results) != 0 {
		t.Fatalf("invalid warning emitted SDK envelope: %#v", results)
	}
}

func TestEventAdapterSystemWarningUsesSDKStrictProjection(t *testing.T) {
	a := newEventAdapter("session-sdk-warning", i18n.LangEN)
	secret := "/workspace/private/.env token=sk-sdk-warning\x1b[2J"
	event := Event{Type: EventSystemWarning, TurnCount: 4, RuntimeEvent: &RuntimeEvent{
		SchemaVersion: types.RuntimeEventSchemaVersion, Kind: string(types.RuntimeEventKindWarning),
		RuntimeIdentity: RuntimeIdentity{EventID: types.NewRuntimeEventID()}, Outcome: ToolOutcomeFailed,
		PublicKey: string(i18n.KeyRuntimeAutoCompactFailed), DiagnosticCode: "runtime.warning",
		PrivateCause:    errors.New(secret),
		PrivateMetadata: map[string]any{"authorization": "Bearer private-token", "project_root": "/private/project"},
	}}
	event.Text = secret
	event.Error = &APIError{Type: "raw_warning", Message: secret}
	event.Metadata = map[string]any{"authorization": "Bearer raw-private-token"}
	event.ProjectRoot = "/private/project"
	event.TurnID = "private-turn"
	event.ActorID = "private-actor"
	event.WorkUnitID = "private-work"

	message := a.process(event)[0].(SDKSystemMessage)
	if message.Type != "system" || message.Subtype != "warning" {
		t.Fatalf("SDK warning envelope retained raw diagnostics: %+v", message)
	}
	projection := message.RuntimeEvent
	if projection == nil || projection.Kind != string(types.RuntimeEventKindWarning) ||
		projection.Audience != "sdk" || projection.RedactionLevel != "strict" ||
		projection.Message != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeAutoCompactFailed) {
		t.Fatalf("SDK warning projection = %#v", projection)
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"project_root", "error", "metadata"} {
		if _, present := wire[forbidden]; present {
			t.Fatalf("SDK warning exposed forbidden field %q: %s", forbidden, encoded)
		}
	}
	for _, private := range []string{
		secret, "sk-sdk-warning", "/private/project", "private_warning", "raw_warning",
		"private-token", "raw-private-token", "authorization", "\x1b[2J", "private_cause", "private_metadata",
	} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("SDK warning projection leaked %q: %s", private, encoded)
		}
	}
	if result := a.resultMessage(i18n.LangEN, "session-sdk-warning", "query-warning", nil); result.ProjectRoot != "" {
		t.Fatalf("EventSystemWarning project_root escaped through final result state: %q", result.ProjectRoot)
	}
}

func TestEventAdapterForwardsLocalizedRawSafeRequestStatus(t *testing.T) {
	const secret = "provider token=sk-private-request-error"
	a := newEventAdapter("session-request-status", i18n.LangZH)
	results := a.process(Event{
		Type: EventRequestRetry, Text: secret, TerminalReason: secret,
		Metadata: map[string]any{"provider_error": secret},
		RequestStatus: &RequestStatusEvent{
			RequestID: "request-7", Phase: "request_retry", Status: "retrying",
			Attempt: 2, MaxAttempts: 4, RetryDelayMilliseconds: 750,
			ErrorCode: "provider_request_retry", ErrorMessage: secret,
		},
	})
	if len(results) != 1 {
		t.Fatalf("request status results = %d, want 1", len(results))
	}
	message := results[0].(StreamEventMsg)
	payload := message.Event.(RuntimeEventPayload)
	status := payload.RequestStatus
	if status == nil || status.RequestID != "request-7" || status.Phase != "request_retry" || status.Status != "retrying" || status.Attempt != 2 || status.MaxAttempts != 4 || status.RetryDelayMilliseconds != 750 {
		t.Fatalf("request status payload = %+v", status)
	}
	if status.ErrorMessage != i18n.Format(i18n.LangZH, i18n.KeyRuntimeTransientAPIError, 2, 4) || status.ErrorCode != "provider_request_retry" {
		t.Fatalf("request status error projection = %+v", status)
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "sk-private-request-error") {
		t.Fatalf("request status leaked raw provider error: %s", encoded)
	}

	unknown := a.process(Event{Type: EventRequestFailed, RequestStatus: &RequestStatusEvent{
		RequestID: "request-unknown", ErrorCode: secret, ErrorMessage: secret,
	}})[0].(StreamEventMsg)
	unknownStatus := unknown.Event.(RuntimeEventPayload).RequestStatus
	if unknownStatus.ErrorCode != "" || unknownStatus.ErrorMessage != "" {
		t.Fatalf("unknown request error authority was forwarded: %+v", unknownStatus)
	}
}
