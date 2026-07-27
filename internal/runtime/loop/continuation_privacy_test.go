package loop

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestHookMessageExcludesAllProviderContinuationState(t *testing.T) {
	message := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ThinkingBlock{
		Type: types.ContentTypeThinking, Thinking: "visible summary", Signature: "hook-cipher-secret",
		SignatureKind: types.ThinkingSignatureOpenAIEncryptedReasoning, SignatureModel: "hook-model-secret",
		ProviderItemID: "hook-item-secret", ProviderStatus: "completed",
	}}}
	message.AttachProviderContinuation(&types.ProviderContinuation{
		Protocol: "responses/openai_public/standard", RequestedModel: "hook-request-model-secret", ServedModel: "hook-served-model-secret",
		ReasoningContext: "all_turns", ResponseStatus: "completed",
		Items: []types.ProviderContinuationItem{types.NewProviderContinuationItem(0, json.RawMessage(`{"type":"reasoning","encrypted_content":"hook-private-ledger-secret"}`))},
	})
	wire, err := json.Marshal(hookMessage(message))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), "visible summary") {
		t.Fatalf("hook lost visible summary: %s", wire)
	}
	for _, forbidden := range []string{
		"hook-cipher-secret", "openai_encrypted_reasoning", "hook-model-secret", "hook-item-secret",
		"hook-request-model-secret", "hook-served-model-secret", "hook-private-ledger-secret",
	} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("hook payload leaked %q: %s", forbidden, wire)
		}
	}
}
