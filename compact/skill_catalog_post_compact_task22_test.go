package compact

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type task22SkillCatalogAttachments []types.Message

func (attachments task22SkillCatalogAttachments) PostCompactSkillAttachments(context.Context, PostCompactAttachmentState) []types.Message {
	return append([]types.Message(nil), attachments...)
}

func TestPostCompactSkillCatalogProviderSupersedesLegacyNameReminder(t *testing.T) {
	snapshot := types.DeveloperMessage(`{"type":"skill_catalog_snapshot","revision":7,"skills":[]}`,
		types.DeveloperMessageMetadata{Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 7})
	provider := &RuntimeAttachmentProvider{
		SessionID:     "task22-session",
		SkillCatalog:  task22SkillCatalogAttachments{snapshot},
		InvokedSkills: fakeSkillInvocations{{Name: "legacy-name", ToolUseID: "toolu_legacy"}},
	}

	got := provider.PostCompactAttachments(context.Background(), PostCompactAttachmentState{})
	if len(got) != 1 || got[0].Role != types.RoleDeveloper || got[0].DeveloperMetadata == nil ||
		got[0].DeveloperMetadata.Kind != types.DeveloperMessageKindSkillCatalogSnapshot {
		t.Fatalf("live skill projection = %#v, want one developer snapshot", got)
	}
	if strings.Contains(joinMessageText(got), "legacy-name") {
		t.Fatalf("live catalog path also emitted legacy invoked-name reminder: %#v", got)
	}
}

func TestPostCompactSkillLegacyFallbackRemainsInformationalOnly(t *testing.T) {
	provider := &RuntimeAttachmentProvider{
		SessionID:     "task22-legacy",
		InvokedSkills: fakeSkillInvocations{{Name: "review", ToolUseID: "toolu_1", Source: "project"}},
	}
	got := provider.PostCompactAttachments(context.Background(), PostCompactAttachmentState{})
	if len(got) != 1 || !strings.Contains(got[0].GetText(), "Post-compaction invoked skills") {
		t.Fatalf("compatibility fallback = %#v", got)
	}
	if IsLegacyInvokedSkillsAttachment(got[0]) || IsReinjectedAttachment(got[0]) {
		t.Fatalf("exported attachment producer acted as a seal oracle: %#v", got[0])
	}
	result := authorizeCompactionResultForTest(&CompactionResult{Attachments: got})
	if !IsLegacyInvokedSkillsAttachment(result.Attachments[0]) || !IsReinjectedAttachment(result.Attachments[0]) {
		t.Fatal("legacy reminder must not be recursively summarized")
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
