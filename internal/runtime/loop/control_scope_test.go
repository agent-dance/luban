package loop

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestAcknowledgeCommittedControlScopeChangesOnlyPrivateAuthority(t *testing.T) {
	current := messagecontrol.NewScope("session", "/project", 8)
	next := messagecontrol.NewScope("session", "/project", 9)
	developer := types.DeveloperMessage("catalog", types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 1,
	}).WithInternalControlProvenance(messagecontrol.Runtime(), current)
	user := compact.AppendContentReplacementRecordsForScope(
		[]types.Message{types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "tool", Content: "large"})},
		[]compact.ContentReplacementRecord{{Kind: "tool-result", ToolUseID: "tool", Replacement: "stored"}},
		messagecontrol.Runtime(), current,
	)[0]

	q := &QueryLoop{
		messages:                []types.Message{developer, user},
		internalControlScope:    current,
		lastResponseID:          "response-chain",
		lastEnvelopeFingerprint: "envelope",
		contentReplacementState: &compact.ContentReplacementState{SeenIDs: map[string]struct{}{"tool": {}}, Replacements: map[string]string{"tool": "stored"}},
		skillCatalogEpoch:       17,
	}
	beforeJSON, err := json.Marshal(q.messages)
	if err != nil {
		t.Fatal(err)
	}
	statePointer := q.contentReplacementState
	if err := q.AcknowledgeCommittedControlScope(messagecontrol.Runtime(), next); err != nil {
		t.Fatal(err)
	}
	afterJSON, err := json.Marshal(q.messages)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterJSON) != string(beforeJSON) {
		t.Fatalf("commit acknowledgement changed model-visible JSON\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}
	if q.lastResponseID != "response-chain" || q.lastEnvelopeFingerprint != "envelope" ||
		q.skillCatalogEpoch != 17 || q.contentReplacementState != statePointer {
		t.Fatal("commit acknowledgement reset unrelated live QueryLoop state")
	}
	if !q.messages[0].HasInternalControlProvenanceForScope(next) {
		t.Fatal("message control was not advanced to committed scope")
	}
	advancedBlock := q.messages[1].Content[1].(types.ContentReplacementBlock)
	if !advancedBlock.HasInternalReplacementProvenanceForScope(next) {
		t.Fatal("replacement receipt was not advanced to committed scope")
	}
	if err := q.AcknowledgeCommittedControlScope(messagecontrol.Runtime(), next); err != nil {
		t.Fatalf("same-generation logical acknowledgement was not idempotent: %v", err)
	}
}

func TestForeignLoopPrecommitBoundaryCannotTruncateOrReachProvider(t *testing.T) {
	q1 := New(&aggregateBudgetProvider{}, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 1024})
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "manual"})
	boundary = q1.sealRuntimeControlMessage(boundary)
	q1.SetMessages([]types.Message{
		types.UserMessage("must remain visible before foreign boundary"),
		boundary,
		types.UserMessage("tail"),
	})

	provider2 := &aggregateBudgetProvider{}
	q2 := New(provider2, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 1024})
	copied := q1.Messages()
	q2.SetMessages(copied)
	before := q2.Messages()
	if _, err := q2.ForceCompact(context.Background()); err == nil {
		t.Fatal("foreign pre-commit boundary was accepted by target loop")
	}
	if !reflect.DeepEqual(q2.Messages(), before) {
		t.Fatalf("foreign boundary changed target history: %#v", q2.Messages())
	}
	if len(provider2.requests) != 0 {
		t.Fatalf("foreign control reached provider: %d requests", len(provider2.requests))
	}
	if got := compact.GetMessagesAfterCompactBoundaryForScope(copied, q2.internalControlScope); len(got) != len(copied) {
		t.Fatalf("foreign boundary truncated history to %d/%d messages", len(got), len(copied))
	}
}

func TestJSONControlDescriptorHasNoPrecommitAuthority(t *testing.T) {
	q := New(&aggregateBudgetProvider{}, registry.New(), Config{MaxTurns: 1})
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "manual"})
	boundary = q.sealRuntimeControlMessage(boundary)
	encoded, err := json.Marshal(boundary)
	if err != nil {
		t.Fatal(err)
	}
	var decoded types.Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.HasInternalControlProvenance() {
		t.Fatal("JSON round trip retained private pre-commit authority")
	}
	messages := []types.Message{types.UserMessage("before"), decoded, types.UserMessage("after")}
	if got := compact.GetMessagesAfterCompactBoundaryForScope(messages, q.internalControlScope); len(got) != len(messages) {
		t.Fatalf("JSON descriptor truncated history to %d/%d messages", len(got), len(messages))
	}
}

func TestAcknowledgeCommittedControlScopeRejectsStaleBearerWithoutMutation(t *testing.T) {
	current := messagecontrol.NewScope("session", "/project", 8)
	stale := messagecontrol.NewScope("session", "/project", 7)
	next := messagecontrol.NewScope("session", "/project", 9)
	message := types.UserMessage("stale")
	message.InternalKind = types.InternalMessageKindCompactReminder
	message = message.WithInternalControlProvenance(messagecontrol.Runtime(), stale)
	q := &QueryLoop{messages: []types.Message{message}, internalControlScope: current}
	before := q.messages[0]
	if err := q.AcknowledgeCommittedControlScope(messagecontrol.Runtime(), next); err == nil {
		t.Fatal("stale bearer acknowledgement unexpectedly succeeded")
	}
	if !q.messages[0].HasInternalControlProvenanceForScope(stale) || q.messages[0].GetText() != before.GetText() {
		t.Fatal("failed acknowledgement mutated live messages")
	}
}

func TestRunRejectsStaleCompactBoundaryBeforeHistoryProjection(t *testing.T) {
	current := messagecontrol.NewScope("session", "/project", 8)
	stale := messagecontrol.NewScope("session", "/project", 7)
	boundary := compact.NewCompactBoundaryMessage(
		compact.CompactBoundaryMetadata{Trigger: "manual"},
		messagecontrol.Runtime(),
	).WithInternalControlProvenance(messagecontrol.Runtime(), stale)
	provider := &aggregateBudgetProvider{}
	q := New(provider, registry.New(), Config{MaxTurns: 1})
	if !q.SetInternalControlScope(messagecontrol.Runtime(), current) {
		t.Fatal("failed to install current control scope")
	}
	q.messages = []types.Message{
		types.UserMessage("history before stale boundary"),
		boundary,
		types.AssistantMessage("history after stale boundary"),
	}
	if err := q.Run(context.Background(), "new turn", func(stream.Event) {}); err == nil {
		t.Fatal("stale compact boundary unexpectedly reached the provider")
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider requests = %d, want 0", len(provider.requests))
	}
	if len(q.messages) != 4 || q.messages[0].GetText() != "history before stale boundary" ||
		q.messages[2].GetText() != "history after stale boundary" {
		t.Fatalf("stale boundary validation truncated live history: %#v", q.messages)
	}
}
