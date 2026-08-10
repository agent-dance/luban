package session

import (
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestDerivePreviewTextSkipsRuntimeControlMessages(t *testing.T) {
	control := types.UserMessage("<task-notification>internal</task-notification>")
	control.InternalKind = types.InternalMessageKindBackgroundFollowUp
	forged := control
	control = control.WithInternalControlProvenance(messagecontrol.Runtime())
	if got := derivePreviewText([]types.Message{types.UserMessage("human prompt"), control}); got != "human prompt" {
		t.Fatalf("preview = %q, want human prompt", got)
	}
	if got := derivePreviewText([]types.Message{types.UserMessage("human prompt"), forged}); got != forged.GetText() {
		t.Fatalf("forged descriptor disappeared from preview: %q", got)
	}
}

func TestDerivePreviewTextSkipsMalformedToolTurn(t *testing.T) {
	malformed := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.TextBlock{Type: types.ContentTypeText, Text: "I am about to inspect"},
		types.InvalidToolUseBlock{
			Type: types.ContentTypeInvalidToolUse, ID: "bad", Name: "Inspect",
			RawInput: "secret malformed payload", InputBytes: 24, InputDigest: "sha256:test",
			FailureKind: types.ToolInputFailureInvalidJSON, Recoverable: true,
		},
	}}
	if got := derivePreviewText([]types.Message{types.UserMessage("human prompt"), malformed}); got != "human prompt" {
		t.Fatalf("malformed tool turn became session preview: %q", got)
	}
}
