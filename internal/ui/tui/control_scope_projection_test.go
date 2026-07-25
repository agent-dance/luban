package tui

import (
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestPersistedProjectionHidesControlsOnlyInExactCurrentScope(t *testing.T) {
	current := messagecontrol.NewScope("session", "/project", 5)
	stale := messagecontrol.NewScope("session", "/project", 4)
	crossSession := messagecontrol.NewScope("other", "/project", 5)
	control := func(text string, scope messagecontrol.Scope) types.Message {
		message := types.UserMessage(text)
		message.IsMeta = true
		message.InternalKind = types.InternalMessageKindCompactReminder
		return message.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	}
	identity := (SessionIdentity{Namespace: "/project", SessionID: "session", Epoch: 1}).
		WithInternalControlScope(messagecontrol.Runtime(), current)
	projection, err := ProjectPersistedMessages(identity, []types.Message{
		control("current hidden", current),
		control("stale visible", stale),
		control("cross-session visible", crossSession),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 2 || projection.Messages[0].Text != "stale visible" || projection.Messages[1].Text != "cross-session visible" {
		t.Fatalf("scope-fenced projection=%#v", projection.Messages)
	}
}

func TestProjectionWithoutAuthorityDoesNotHideUnboundControlBearer(t *testing.T) {
	message := types.UserMessage("unbound control remains visible")
	message.IsMeta = true
	message.InternalKind = types.InternalMessageKindCompactReminder
	message = message.WithInternalControlProvenance(messagecontrol.Runtime())
	projection, err := ProjectPersistedMessages(SessionIdentity{SessionID: "session"}, []types.Message{message}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 1 || projection.Messages[0].Text != "unbound control remains visible" {
		t.Fatalf("unscoped presentation elevated unbound bearer: %#v", projection.Messages)
	}
}
