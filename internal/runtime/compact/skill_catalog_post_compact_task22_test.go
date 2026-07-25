package compact

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/types"
)

type task22SkillCatalogAttachments []types.Message

func (attachments task22SkillCatalogAttachments) PostCompactSkillAttachments(context.Context, PostCompactAttachmentState) []types.Message {
	return append([]types.Message(nil), attachments...)
}

func TestPostCompactSkillCatalogProviderEmitsCurrentSnapshot(t *testing.T) {
	snapshot := types.DeveloperMessage(`{"type":"skill_catalog_snapshot","revision":7,"skills":[]}`,
		types.DeveloperMessageMetadata{Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 7})
	provider := &RuntimeAttachmentProvider{
		SessionID:    "task22-session",
		SkillCatalog: task22SkillCatalogAttachments{snapshot},
	}

	got := provider.PostCompactAttachments(context.Background(), PostCompactAttachmentState{})
	if len(got) != 1 || got[0].Role != types.RoleDeveloper || got[0].DeveloperMetadata == nil ||
		got[0].DeveloperMetadata.Kind != types.DeveloperMessageKindSkillCatalogSnapshot {
		t.Fatalf("live skill projection = %#v, want one developer snapshot", got)
	}
}

func TestSkillCatalogPostCompactPreparedMessagesOverrideSegmentsDefensively(t *testing.T) {
	prepared := []types.Message{
		types.DeveloperMessage("current snapshot", types.DeveloperMessageMetadata{
			Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 11,
		}),
		types.UserMessage("current user"),
	}
	result := &CompactionResult{
		SummaryMessages:  []types.Message{types.UserMessage("stale segment")},
		PreparedMessages: prepared,
	}
	got := BuildPostCompactMessages(result)
	if len(got) != 2 || got[0].GetText() != "current snapshot" || got[1].GetText() != "current user" {
		t.Fatalf("prepared post-compact messages = %#v", got)
	}
	got[0] = types.UserMessage("mutated copy")
	again := BuildPostCompactMessages(result)
	if again[0].GetText() != "current snapshot" {
		t.Fatalf("BuildPostCompactMessages leaked PreparedMessages backing array: %#v", again)
	}
}
