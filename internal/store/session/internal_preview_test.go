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
