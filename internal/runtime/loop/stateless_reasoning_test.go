package loop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

func TestProcessStreamPersistsEncryptedReasoningWithoutRuntimeEventLeak(t *testing.T) {
	const (
		ciphertext = "opaque-encrypted-reasoning-secret"
		itemID     = "reasoning-provider-item-secret"
		model      = "gpt-5.6-sol"
	)
	providerStream := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeThinking, ID: itemID, SignatureKind: types.ThinkingSignatureOpenAIEncryptedReasoning,
			SignatureModel: model, ProviderStatus: "in_progress",
		}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type: "signature_delta", ID: itemID, Signature: ciphertext,
			SignatureKind: types.ThinkingSignatureOpenAIEncryptedReasoning, SignatureModel: model,
			ProviderStatus: "completed",
		}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop},
	)
	var events []streamevent.Event
	message, _, _, err := (&QueryLoop{}).processStream(context.Background(), providerStream, 1, func(event streamevent.Event) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(message.Content) != 1 {
		t.Fatalf("content blocks = %d, want one opaque continuation block", len(message.Content))
	}
	reasoning, ok := message.Content[0].(types.ThinkingBlock)
	if !ok {
		t.Fatalf("content = %#v, want ThinkingBlock", message.Content[0])
	}
	if reasoning.Thinking != "" || reasoning.Signature != ciphertext || reasoning.SignatureKind != types.ThinkingSignatureOpenAIEncryptedReasoning || reasoning.SignatureModel != model || reasoning.ProviderItemID != itemID || reasoning.ProviderStatus != "completed" {
		t.Fatalf("opaque continuation state = %#v", reasoning)
	}
	wire, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{ciphertext, itemID, string(types.ThinkingSignatureOpenAIEncryptedReasoning)} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("runtime event stream leaked %q: %s", forbidden, wire)
		}
	}
}

func TestSetModelInvalidatesEncryptedReasoningAndResponseChain(t *testing.T) {
	q := &QueryLoop{
		config: Config{Model: "gpt-5.6-sol"},
		messages: []types.Message{{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ThinkingBlock{
			Type: types.ContentTypeThinking, Thinking: "visible summary", Signature: "ciphertext",
			SignatureKind: types.ThinkingSignatureOpenAIEncryptedReasoning, SignatureModel: "gpt-5.6-sol",
			ProviderItemID: "reasoning-item", ProviderStatus: "completed",
		}}}},
		lastResponseID: "response-id", lastEnvelopeFingerprint: "last", currentEnvelopeFingerprint: "current",
		disableResponseChain: true,
	}
	q.messages[0].AttachProviderContinuation(&types.ProviderContinuation{
		Protocol: "responses/openai_public/standard", RequestedModel: "gpt-5.6-sol", ServedModel: "gpt-5.6-sol",
		ReasoningContext: "all_turns", ResponseStatus: "completed",
		Items: []types.ProviderContinuationItem{types.NewProviderContinuationItem(0, json.RawMessage(`{"type":"reasoning","encrypted_content":"private-model-bound-cipher"}`))},
	})
	q.SetModel("gpt-5.6-sol-next")
	block, ok := q.messages[0].Content[0].(types.ThinkingBlock)
	if !ok {
		t.Fatalf("content = %#v, want ThinkingBlock", q.messages[0].Content[0])
	}
	if block.Thinking != "visible summary" {
		t.Fatalf("visible summary changed: %#v", block)
	}
	if block.Signature != "" || block.SignatureKind != "" || block.SignatureModel != "" || block.ProviderItemID != "" || block.ProviderStatus != "" {
		t.Fatalf("model-bound continuation state survived model change: %#v", block)
	}
	if q.messages[0].HasProviderContinuation() {
		t.Fatal("model-bound private continuation survived model change")
	}
	if q.lastResponseID != "" || q.lastEnvelopeFingerprint != "" || q.currentEnvelopeFingerprint != "" || q.disableResponseChain {
		t.Fatalf("response chain was not reset: %#v", q)
	}
}
