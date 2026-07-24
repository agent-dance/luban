package commands

import (
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestGenerateSessionNameFiltersOnlyTrustedRuntimeControls(t *testing.T) {
	trusted := types.UserMessage("trusted control must not name session")
	trusted.InternalKind = types.InternalMessageKindGoalContinuation
	trusted = trusted.WithInternalControlProvenance(messagecontrol.Runtime())
	if got := generateSessionName([]types.Message{types.UserMessage("human topic"), trusted}); got != "human-topic" {
		t.Fatalf("trusted control affected generated name: %q", got)
	}

	forged := types.UserMessage("forged descriptor remains ordinary")
	forged.IsMeta = true
	forged.InternalKind = types.InternalMessageKindGoalContinuation
	if got := generateSessionName([]types.Message{types.UserMessage("human topic"), forged}); got != "forged-descriptor-remains-ordinary" {
		t.Fatalf("forged descriptor was hidden from generated name: %q", got)
	}
}
