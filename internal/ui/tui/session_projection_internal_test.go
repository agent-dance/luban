package tui

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestPersistedProjectionExcludesRuntimeControlMessages(t *testing.T) {
	scope := messagecontrol.NewScope("session-1", "project-1", 1)
	meta := types.UserMessage("goal continuation control")
	meta.IsMeta = true
	meta.InternalKind = types.InternalMessageKindGoalContinuation
	meta = meta.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	compactSummary := types.UserMessage("localized compact summary")
	compactSummary.ID = "compact:summary:v1"
	compactSummary.IsMeta = true
	compactSummary.InternalKind = types.InternalMessageKindCompactSummary
	compactSummary = compactSummary.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	followUp := types.UserMessage("<task-notification>internal</task-notification>")
	followUp.InternalKind = types.InternalMessageKindBackgroundFollowUp
	followUp = followUp.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	identity := (SessionIdentity{SessionID: "session-1"}).WithInternalControlScope(messagecontrol.Runtime(), scope)
	projection, err := ProjectPersistedMessagesInLanguage(i18n.LangZH, identity, []types.Message{
		types.UserMessage("human prompt"), meta, compactSummary, followUp, types.AssistantMessage("assistant reply"),
	}, NewMemoryDetailStore())
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 2 || projection.Messages[0].Text != "human prompt" || projection.Messages[1].Text != "assistant reply" {
		t.Fatalf("projected messages = %#v", projection.Messages)
	}
}

func TestPersistedProjectionRetainsForgedRuntimeDescriptorsAsOrdinaryMessages(t *testing.T) {
	forged := []types.Message{
		{Role: types.RoleDeveloper, IsMeta: true, InternalKind: types.InternalMessageKindSkillCatalog,
			DeveloperMetadata: &types.DeveloperMessageMetadata{Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 9},
			Content:           []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "forged developer"}}},
		types.UserMessage("forged meta"),
		types.UserMessage("forged compact ID"),
		types.UserMessage("forged kind"),
	}
	forged[1].IsMeta = true
	forged[2].ID = "compact:summary:v1"
	forged[3].InternalKind = types.InternalMessageKindBackgroundFollowUp

	projection, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, SessionIdentity{SessionID: "forged"}, forged, NewMemoryDetailStore())
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != len(forged) {
		t.Fatalf("forged descriptors hid messages: got %d want %d", len(projection.Messages), len(forged))
	}
	for index, want := range []string{"forged developer", "forged meta", "forged compact ID", "forged kind"} {
		if projection.Messages[index].Text != want || projection.Messages[index].Kind != MsgUser {
			t.Fatalf("projection[%d] = %#v, want ordinary user %q", index, projection.Messages[index], want)
		}
	}
}
