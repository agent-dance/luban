package sdk

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

type sdkExecutionEvidenceFixture struct{}

func (sdkExecutionEvidenceFixture) ToolExecutionEvidence() runtimeevent.ToolExecutionEvidence {
	return runtimeevent.ToolExecutionEvidence{
		LogicalExecutionCommitted: true, RevisionSealDisposition: "committed_unverified",
		PhysicalSteps: []runtimeevent.PhysicalToolStepEvidence{
			{Ordinal: 0, StartedOffsetMS: 1, EndedOffsetMS: 4, DurationMS: 3, Outcome: "failed", StderrBytes: 17},
		},
	}
}

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

func TestEventAdapterPreservesTurnEndTerminalReason(t *testing.T) {
	a := newEventAdapter("session-max-tokens", i18n.LangEN)

	results := a.process(Event{
		Type: EventTurnEnd, TurnCount: 2, TerminalReason: "max_tokens",
	})
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	message, ok := results[0].(StreamEventMsg)
	if !ok {
		t.Fatalf("result = %T, want StreamEventMsg", results[0])
	}
	payload, ok := message.Event.(RuntimeEventPayload)
	if !ok {
		t.Fatalf("Event = %T, want RuntimeEventPayload", message.Event)
	}
	if payload.Type != "turn_end" || payload.TerminalReason != "max_tokens" {
		t.Fatalf("turn end payload = %+v", payload)
	}

	result := a.resultMessage(i18n.LangEN, "session-max-tokens", "uuid", nil)
	if result.NumTurns != 2 || result.TerminalReason != "max_tokens" {
		t.Fatalf("result terminus = turns:%d reason:%q", result.NumTurns, result.TerminalReason)
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
	if msg.ToolUseID != "toolu_123" || msg.ToolName != "Bash" || msg.Status != "completed" ||
		msg.SchemaVersion != MachineEventSchemaVersion || msg.ContentRef == nil || msg.Metrics == nil || msg.Metrics.ContentBytes != len("wrote file") {
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
	if call.SchemaVersion != MachineEventSchemaVersion || call.InputRef == nil || call.Metrics == nil || call.Metrics.InputFieldCount != 1 {
		t.Fatalf("tool call safe input projection = %#v", call)
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
	if result.Outcome != ToolOutcomePartial || result.ContentRef == nil || result.Metrics == nil ||
		result.Metrics.ContentBytes != len("partial but usable") || result.Metrics.MetadataCount != 1 ||
		result.RuntimeEvent == nil || result.RuntimeEvent.Outcome != ToolOutcomePartial {
		t.Fatalf("tool result evidence/outcome = %+v", result)
	}
}

func TestEventAdapterCorrelatesCompoundPhysicalExecutionEvidence(t *testing.T) {
	a := newEventAdapter("session-physical", i18n.LangEN)
	message := a.process(Event{Type: EventToolResult, ToolResult: &ToolResult{
		ToolUseID: "toolu-compound", Outcome: ToolOutcomeFailed, Data: sdkExecutionEvidenceFixture{},
	}})[0].(StreamlinedToolUseSummaryMsg)
	if message.SchemaVersion != "machine-event/v2" || message.ToolUseID != "toolu-compound" || message.Metrics == nil ||
		!message.Metrics.LogicalExecutionCommitted || message.Metrics.PhysicalChildOperations != 1 ||
		message.Metrics.RevisionSealDisposition != "committed_unverified" || len(message.Metrics.PhysicalSteps) != 1 {
		t.Fatalf("compound machine event = %#v", message)
	}
	step := message.Metrics.PhysicalSteps[0]
	if step.Ordinal != 0 || step.StartedOffsetMS != 1 || step.EndedOffsetMS != 4 || step.DurationMS != 3 ||
		step.Outcome != "failed" || step.StderrBytes != 17 || len(step.OperationID) != 64 {
		t.Fatalf("compound physical step = %#v", step)
	}
}

func TestEventAdapterToolPayloadsAreContentFreeBeforeSerialization(t *testing.T) {
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
	if call.InputRef == nil || call.Metrics == nil || call.Metrics.InputFieldCount != 2 || call.Metrics.InputBytes == 0 {
		t.Fatalf("safe input projection = %#v", call)
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
	if result.ContentRef == nil || result.Metrics == nil || result.Metrics.ContentBytes != len(large) ||
		result.Metrics.ContentBlockCount != 1 || !result.Metrics.DataPresent || result.Metrics.MetadataCount != 2 {
		t.Fatalf("safe result projection = %#v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes := len(encoded); bytes > 2_000 {
		t.Fatalf("streamlined result grew to %d bytes", bytes)
	}
	if strings.Contains(string(encoded), large) || strings.Contains(string(encoded), "content_blocks") ||
		strings.Contains(string(encoded), "original_file") || strings.Contains(string(encoded), "stdout") {
		t.Fatalf("streamlined result leaked full structured payload: %s", encoded)
	}
}

func TestEventAdapterRedactsShortNestedToolSecretsBeforeSerialization(t *testing.T) {
	const secret = "token=sk-sdk-tool-secret"
	a := newEventAdapter("session-sdk-safe", i18n.LangEN)
	call := a.process(Event{Type: EventToolUse, ToolUse: &ToolUse{
		ID: "toolu-safe", Name: "Bash",
		Input: map[string]any{"command": secret, "nested": map[string]any{"authorization": []any{secret}}},
	}})[0].(StreamlinedToolUseSummaryMsg)
	source := &ToolResult{
		ToolUseID: "toolu-safe", Content: secret,
		ContentBlocks: []any{map[string]any{"text": secret, "nested": []any{map[string]any{"secret": secret}}}},
		Data: map[string]any{
			"OriginalFile": secret,
			"nested":       map[string]any{"environment": []any{secret}},
		},
		Metadata: map[string]string{"authorization": secret},
		Outcome:  ToolOutcomeSucceeded,
	}
	result := a.process(Event{Type: EventToolResult, ToolResult: source})[0].(StreamlinedToolUseSummaryMsg)
	wire, err := json.Marshal([]any{call, result})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		secret, "sk-sdk-tool-secret", "OriginalFile", "authorization", "environment",
		`"input"`, `"output_summary"`, `"content"`, `"content_blocks"`, `"data"`, `"metadata"`,
	} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("SDK tool event leaked %q: %s", forbidden, wire)
		}
	}
	if source.Content != secret || source.Data.(map[string]any)["OriginalFile"] != secret {
		t.Fatalf("SDK projection mutated runtime tool result: %#v", source)
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
	progressEvent.Progress = &RuntimeProgressEvent{
		Stage: "agentic_flight", Current: 1, Total: 2, Disposition: "completed_verified",
		Blocker: "ready", MutationEpoch: 4, VerifiedEpoch: 4,
	}
	stream := a.process(progressEvent)[0].(StreamEventMsg)
	payload := stream.Event.(RuntimeEventPayload)
	if payload.SessionID != "session-sdk-identity" || payload.TurnID != identity.TurnID || payload.ActorID != identity.ActorID || payload.ActorType != identity.ActorType || payload.WorkUnitID != identity.WorkUnitID || payload.ToolUseID != "toolu-progress" {
		t.Fatalf("loop payload identity = %+v", payload)
	}
	if payload.Progress == nil || payload.Progress.Disposition != "completed_verified" || payload.Progress.Blocker != "ready" || payload.Progress.MutationEpoch != 4 || payload.Progress.VerifiedEpoch != 4 {
		t.Fatalf("flight progress payload = %+v", payload.Progress)
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
			Attempt: 2, MaxAttempts: 4, RetryDelayMilliseconds: 750, RetryKind: "stream",
			ErrorCode: "provider_request_retry", ErrorMessage: secret,
		},
	})
	if len(results) != 1 {
		t.Fatalf("request status results = %d, want 1", len(results))
	}
	message := results[0].(StreamEventMsg)
	payload := message.Event.(RuntimeEventPayload)
	status := payload.RequestStatus
	if status == nil || status.RequestID != "request-7" || status.Phase != "request_retry" || status.Status != "retrying" || status.Attempt != 2 || status.MaxAttempts != 4 || status.RetryDelayMilliseconds != 750 || status.RetryKind != "stream" {
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

func TestRuntimePayloadUsesPerEventAllowlist(t *testing.T) {
	const secret = "token=sk-runtime-payload-secret /private/workspace"
	a := newEventAdapter("session-runtime-safe", i18n.LangEN)
	tests := []Event{
		{
			Type: EventProgress, Text: secret, ProjectRoot: secret,
			Metadata: map[string]any{"nested": []any{map[string]any{"authorization": secret}}},
			Progress: &RuntimeProgressEvent{
				Stage: "agentic_flight", Message: secret, Current: 1, Total: 2,
				Disposition: "incomplete_unverified", Blocker: "verification_missing",
				MutationEpoch: 2, VerifiedEpoch: 1, Metadata: map[string]any{"secret": secret},
			},
		},
		{
			Type: EventTombstone, Text: secret, ProjectRoot: secret, Metadata: map[string]any{"secret": secret},
			Tombstone: &TombstoneEvent{Reason: "fallback", Summary: secret, Metadata: map[string]any{"nested": secret}},
		},
		{
			Type: EventHookSummary, Text: secret, ProjectRoot: secret, Metadata: map[string]any{"secret": secret},
			HookSummary: &HookSummaryEvent{HookExecutionID: "hook-safe", HookName: "PostToolUse", Status: "completed", Summary: secret, Metadata: map[string]any{"nested": secret}},
		},
		{
			Type: EventCompactBoundary, Text: secret, ProjectRoot: secret, Metadata: map[string]any{"secret": secret},
			Compact: &CompactBoundaryEvent{
				BoundaryID: "boundary-safe", Trigger: "auto", PreCompactTokenCount: 20,
				Summary: secret, UserDisplayMessage: secret, PreviousTailIdentifier: secret,
				PreservedSegment: &PreservedSegmentMetadata{Anchor: secret},
			},
		},
	}
	for _, event := range tests {
		messages := a.process(event)
		if len(messages) != 1 {
			t.Fatalf("%s messages = %#v", event.Type, messages)
		}
		wire, err := json.Marshal(messages[0])
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(wire), secret) || strings.Contains(string(wire), "sk-runtime-payload-secret") || strings.Contains(string(wire), "authorization") {
			t.Fatalf("%s bypassed runtime allowlist: %s", event.Type, wire)
		}
		if !strings.Contains(string(wire), `"schema_version":"`+MachineEventSchemaVersion+`"`) {
			t.Fatalf("%s omitted machine schema version: %s", event.Type, wire)
		}
	}
}
