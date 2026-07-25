package types

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestRuntimeEventPrivateCausePreservesIdentityWithoutLeaking(t *testing.T) {
	private := errors.New("private-provider-secret")
	event := NewRuntimeEvent(
		RuntimeEventKindError,
		RuntimeIdentity{EventID: "event-1", SessionID: "session-1", ContextGeneration: 7},
		ToolOutcomeFailed,
		i18n.KeyRuntimeErrorPublicSummary,
		nil,
		"runtime.operation_failed",
		private,
	)
	if !errors.Is(event, private) {
		t.Fatal("private cause identity was not preserved")
	}
	if strings.Contains(event.Error(), private.Error()) || strings.Contains(event.PrivateCause.Error(), private.Error()) {
		t.Fatalf("public error leaked private cause: event=%q cause=%q", event.Error(), event.PrivateCause.Error())
	}
	if event.ContextGeneration != 7 || event.EventID != "event-1" {
		t.Fatalf("identity changed: %#v", event.RuntimeIdentity)
	}
}

func TestRuntimeEventCannotBypassAudienceProjectorJSON(t *testing.T) {
	event := NewRuntimeEvent(
		RuntimeEventKindError, RuntimeIdentity{EventID: "event-json"}, ToolOutcomeFailed,
		i18n.KeyRuntimeErrorPublicSummary, nil, "runtime.operation_failed", errors.New("private-json-secret"),
	)
	if encoded, err := json.Marshal(event); err == nil || len(encoded) != 0 {
		t.Fatalf("direct JSON projection succeeded: %s, err=%v", encoded, err)
	}
}

func TestToolResultRuntimeEventLeavesUnassignedOutcomeInvalidInsteadOfInferringIt(t *testing.T) {
	failedPayloadWithoutOutcome := NewToolResultRuntimeEvent(
		RuntimeIdentity{EventID: "event-failed-payload"},
		ToolResultBlock{ToolUseID: "tool-1", IsError: true, Content: `{"status":"failed","code":"fatal"}`},
		i18n.KeyRuntimeErrorPublicSummary,
		nil,
	)
	if failedPayloadWithoutOutcome.Outcome != "" || failedPayloadWithoutOutcome.HasAuthoritativeOutcome() {
		t.Fatalf("incomplete event acquired an inferred outcome: %q", failedPayloadWithoutOutcome.Outcome)
	}

	successPayloadWithDeniedOutcome := NewToolResultRuntimeEvent(
		RuntimeIdentity{EventID: "event-denied"},
		ToolResultBlock{ToolUseID: "tool-2", IsError: false, Content: `{"status":"ok"}`, Outcome: ToolOutcomeDenied},
		i18n.KeyRuntimeErrorPublicSummary,
		nil,
	)
	if successPayloadWithDeniedOutcome.Outcome != ToolOutcomeDenied || !successPayloadWithDeniedOutcome.HasAuthoritativeOutcome() {
		t.Fatalf("authoritative outcome not retained: %q", successPayloadWithDeniedOutcome.Outcome)
	}
}

func TestNewRuntimeEventIDIsOpaqueAndUnique(t *testing.T) {
	first := NewRuntimeEventID()
	second := NewRuntimeEventID()
	if first == second || !strings.HasPrefix(first, "evt_") || !strings.HasPrefix(second, "evt_") {
		t.Fatalf("event IDs are not opaque and unique: %q %q", first, second)
	}
}
