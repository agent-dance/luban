package sdk

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/types"
)

func TestEventAdapter_StructuredLoopEvents(t *testing.T) {
	a := newEventAdapter("session-structured")

	tests := []struct {
		name      string
		event     loop.Event
		wantType  string
		assertion func(*testing.T, LoopEventPayload)
	}{
		{
			name: "max turns",
			event: loop.Event{
				Type:           loop.EventMaxTurnsReached,
				TerminalReason: "max_turns_reached",
				MaxTurns:       &loop.MaxTurnsEvent{MaxTurns: 3, TurnCount: 4},
			},
			wantType: "max_turns_reached",
			assertion: func(t *testing.T, payload LoopEventPayload) {
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
			event: loop.Event{
				Type:    loop.EventCompactBoundary,
				Compact: &loop.CompactBoundaryEvent{Trigger: "auto", PreCompactTokenCount: 1200},
			},
			wantType: "compact_boundary",
			assertion: func(t *testing.T, payload LoopEventPayload) {
				t.Helper()
				if payload.Compact == nil || payload.Compact.Trigger != "auto" {
					t.Fatalf("Compact payload = %+v, want trigger auto", payload.Compact)
				}
			},
		},
		{
			name: "tombstone",
			event: loop.Event{
				Type:      loop.EventTombstone,
				MessageID: "msg_1",
				Tombstone: &loop.TombstoneEvent{
					MessageID: "msg_1",
					Reason:    "fallback",
				},
			},
			wantType: "tombstone",
			assertion: func(t *testing.T, payload LoopEventPayload) {
				t.Helper()
				if payload.Tombstone == nil || payload.Tombstone.Reason != "fallback" {
					t.Fatalf("Tombstone payload = %+v, want fallback", payload.Tombstone)
				}
				if payload.MessageID != "msg_1" {
					t.Fatalf("MessageID = %q, want msg_1", payload.MessageID)
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
			payload, ok := msg.Event.(LoopEventPayload)
			if !ok {
				t.Fatalf("Event = %T, want LoopEventPayload", msg.Event)
			}
			if payload.Type != tt.wantType {
				t.Fatalf("payload.Type = %q, want %q", payload.Type, tt.wantType)
			}
			tt.assertion(t, payload)
		})
	}
}

func TestEventAdapter_ToolUseSummaryEvent(t *testing.T) {
	a := newEventAdapter("session-tool-summary")

	results := a.process(loop.Event{
		Type: loop.EventToolUseSummary,
		ToolSummary: &loop.ToolUseSummaryEvent{
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
	a := newEventAdapter("session-sdk")
	callEvent := loop.Event{
		Type: loop.EventToolUse, TurnID: "session-sdk:query-a:turn-2", ActorID: "agent-a", ActorType: "reviewer", WorkUnitID: "review-a",
		ToolUse: &types.ToolUseBlock{ID: "toolu-sdk", Name: "Read", Input: map[string]any{"file_path": "/tmp/full"}},
	}
	call := a.process(callEvent)[0].(StreamlinedToolUseSummaryMsg)
	if call.TurnID != callEvent.TurnID || call.ActorID != "agent-a" || call.ActorType != "reviewer" || call.WorkUnitID != "review-a" || call.SessionID != "session-sdk" {
		t.Fatalf("tool call identity = %+v", call)
	}
	if call.Input["file_path"] != "/tmp/full" {
		t.Fatalf("tool call input = %#v", call.Input)
	}

	resultEvent := callEvent
	resultEvent.Type = loop.EventToolResult
	resultEvent.ToolUse = nil
	resultEvent.ToolResult = &types.ToolResultBlock{
		ToolUseID: "toolu-sdk", Content: "partial but usable", Outcome: types.ToolOutcomePartial,
		Metadata: map[string]string{"source": "disk"},
	}
	result := a.process(resultEvent)[0].(StreamlinedToolUseSummaryMsg)
	if result.TurnID != resultEvent.TurnID || result.ActorID != "agent-a" || result.ActorType != "reviewer" || result.WorkUnitID != "review-a" || result.SessionID != "session-sdk" {
		t.Fatalf("tool result identity = %+v", result)
	}
	if result.Outcome != types.ToolOutcomePartial || result.OutputSummary != "partial but usable" || result.Metadata["source"] != "disk" {
		t.Fatalf("tool result evidence/outcome = %+v", result)
	}
}

func TestEventAdapter_TextAndLoopPayloadPreserveStableIdentity(t *testing.T) {
	a := newEventAdapter("session-sdk-identity")
	identity := loop.Event{TurnID: "session-sdk-identity:query-a:turn-4", ActorID: "agent-b", ActorType: "executor", WorkUnitID: "work-b"}
	textEvent := identity
	textEvent.Type, textEvent.Text = loop.EventText, "token"
	text := a.process(textEvent)[0].(StreamlinedTextMsg)
	if text.SessionID != "session-sdk-identity" || text.TurnID != identity.TurnID || text.ActorID != identity.ActorID || text.ActorType != identity.ActorType || text.WorkUnitID != identity.WorkUnitID {
		t.Fatalf("text identity = %+v", text)
	}

	progressEvent := identity
	progressEvent.Type = loop.EventProgress
	progressEvent.ToolUseID = "toolu-progress"
	progressEvent.Progress = &loop.ProgressEvent{Stage: "verify", Current: 1, Total: 2}
	stream := a.process(progressEvent)[0].(StreamEventMsg)
	payload := stream.Event.(LoopEventPayload)
	if payload.SessionID != "session-sdk-identity" || payload.TurnID != identity.TurnID || payload.ActorID != identity.ActorID || payload.ActorType != identity.ActorType || payload.WorkUnitID != identity.WorkUnitID || payload.ToolUseID != "toolu-progress" {
		t.Fatalf("loop payload identity = %+v", payload)
	}
}

func TestEventAdapterRuntimeErrorUsesSDKPublicProjection(t *testing.T) {
	a := newEventAdapter("session-sdk-error")
	secret := "/workspace/private/.env token=sk-sdk-secret"
	event := loop.Event{
		Type: loop.EventError, Text: secret, ToolUseID: "toolu-error",
		ProjectRoot: "/workspace/project", TurnID: "session-sdk-error:query-a:turn-5", ActorID: "agent-error", ActorType: "executor", WorkUnitID: "work-error",
		Error:    &types.APIError{Type: "private_provider_error", Message: secret, Status: 500},
		Metadata: map[string]any{"authorization": "Bearer private-token"},
	}
	message := a.process(event)[0].(SDKSystemMessage)
	if message.Type != "system" || message.Subtype != "error" {
		t.Fatalf("SDK compatibility envelope = %+v", message)
	}
	if message.SessionID != "session-sdk-error" || message.TurnID != event.TurnID || message.ActorID != event.ActorID || message.ActorType != event.ActorType || message.WorkUnitID != event.WorkUnitID || message.ToolUseID != event.ToolUseID {
		t.Fatalf("error identity = %+v", message)
	}
	if message.ProjectRoot != "" || message.Error != nil || message.Metadata != nil {
		t.Fatalf("default SDK error retained raw diagnostic fields: %+v", message)
	}
	projection := message.RuntimeEvent
	if projection == nil || projection.SchemaVersion != types.RuntimeEventSchemaVersion ||
		projection.Audience != "sdk" || projection.RedactionLevel != "public" ||
		projection.Kind != types.RuntimeEventKindError || projection.Outcome != types.ToolOutcomeFailed ||
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
	for _, private := range []string{secret, "sk-sdk-secret", event.ProjectRoot, "private_provider_error", "private-token", "authorization", "private_cause", "private_metadata"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("default SDK runtime-error projection leaked %q: %s", private, encoded)
		}
	}
	if result := a.resultMessage(i18n.LangEN, "session-sdk-error", "query-error", nil); result.ProjectRoot != "" {
		t.Fatalf("EventError project_root escaped through final result state: %q", result.ProjectRoot)
	}
}

func TestEventAdapterSystemWarningUsesSDKStrictProjection(t *testing.T) {
	a := newEventAdapter("session-sdk-warning")
	secret := "/workspace/private/.env token=sk-sdk-warning\x1b[2J"
	event := loop.NewSystemWarningEvent(
		i18n.KeyRuntimeAutoCompactFailed,
		nil,
		&types.APIError{Type: "private_warning", Message: secret},
		map[string]any{"authorization": "Bearer private-token", "project_root": "/private/project"},
		4,
	)
	event.Text = secret
	event.Error = &types.APIError{Type: "raw_warning", Message: secret}
	event.Metadata = map[string]any{"authorization": "Bearer raw-private-token"}
	event.ProjectRoot = "/private/project"
	event.TurnID = "private-turn"
	event.ActorID = "private-actor"
	event.WorkUnitID = "private-work"

	message := a.process(event)[0].(SDKSystemMessage)
	if message.Type != "system" || message.Subtype != "warning" || message.ProjectRoot != "" || message.Error != nil || message.Metadata != nil {
		t.Fatalf("SDK warning envelope retained raw diagnostics: %+v", message)
	}
	projection := message.RuntimeEvent
	if projection == nil || projection.Kind != types.RuntimeEventKindWarning ||
		projection.Audience != "sdk" || projection.RedactionLevel != "strict" ||
		projection.Message != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeAutoCompactFailed) {
		t.Fatalf("SDK warning projection = %#v", projection)
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
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
