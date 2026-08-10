package compact

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestStripProviderPrivateBlocksKeepsAuditHistoryOutOfModelView(t *testing.T) {
	messages := []types.Message{{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.TextBlock{Type: types.ContentTypeText, Text: "lead-in"},
		types.InvalidToolUseBlock{
			Type: types.ContentTypeInvalidToolUse, ID: "bad", Name: "Inspect",
			RawInput: "sensitive-invalid-input", InputBytes: 23, InputDigest: "sha256:test",
			FailureKind: types.ToolInputFailureInvalidJSON, Recoverable: true,
		},
	}}}
	projected := StripProviderPrivateBlocks(messages)
	if len(projected) != 1 || len(projected[0].Content) != 1 || projected[0].GetText() != "lead-in" {
		t.Fatalf("provider projection = %#v", projected)
	}
	if len(messages[0].GetInvalidToolUses()) != 1 {
		t.Fatalf("durable source was mutated: %#v", messages)
	}
}
