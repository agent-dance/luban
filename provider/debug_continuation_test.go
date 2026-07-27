package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestSanitizeDebugMessagesRemovesContinuationFromValueAndPointer(t *testing.T) {
	pointer := &types.ThinkingBlock{
		Type:           types.ContentTypeThinking,
		Thinking:       "pointer-visible-summary",
		Signature:      "pointer-secret-signature",
		SignatureKind:  types.ThinkingSignatureOpenAIEncryptedReasoning,
		SignatureModel: "pointer-secret-model",
		ProviderItemID: "pointer-secret-item",
		ProviderStatus: "pointer-secret-status",
	}
	var nilPointer *types.ThinkingBlock
	messages := []types.Message{{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.ThinkingBlock{
				Type:           types.ContentTypeThinking,
				Thinking:       "value-visible-summary",
				Signature:      "value-secret-signature",
				SignatureKind:  types.ThinkingSignatureAnthropic,
				SignatureModel: "value-secret-model",
				ProviderItemID: "value-secret-item",
				ProviderStatus: "value-secret-status",
			},
			pointer,
			nilPointer,
		},
	}}
	messages[0].AttachProviderContinuation(&types.ProviderContinuation{
		Protocol: "responses/openai_public/standard", RequestedModel: "secret-request-model", ServedModel: "secret-served-model",
		ReasoningContext: "all_turns", ResponseStatus: "completed",
		Items: []types.ProviderContinuationItem{types.NewProviderContinuationItem(0, json.RawMessage(`{"type":"reasoning","encrypted_content":"private-ledger-secret"}`))},
	})

	sanitized := sanitizeDebugMessages(messages)
	if sanitized[0].HasProviderContinuation() {
		t.Fatal("debug observer retained private provider continuation")
	}
	wire, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatal(err)
	}
	got := string(wire)
	for _, forbidden := range []string{
		"value-secret-signature", "anthropic_thinking", "value-secret-model", "value-secret-item", "value-secret-status",
		"pointer-secret-signature", "openai_encrypted_reasoning", "pointer-secret-model", "pointer-secret-item", "pointer-secret-status",
		"private-ledger-secret", "secret-request-model", "secret-served-model",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("debug messages leaked %q: %s", forbidden, got)
		}
	}
	for _, visible := range []string{"value-visible-summary", "pointer-visible-summary"} {
		if !strings.Contains(got, visible) {
			t.Fatalf("debug messages lost visible thinking %q: %s", visible, got)
		}
	}
	if pointer.Signature != "pointer-secret-signature" || pointer.ProviderItemID != "pointer-secret-item" {
		t.Fatalf("sanitizer mutated caller-owned pointer: %#v", pointer)
	}
}
