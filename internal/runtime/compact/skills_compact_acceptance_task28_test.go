package compact

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestSkillsCompactAcceptanceHistoryReplacementAdvancesEpochAndReconcilesLedger(t *testing.T) {
	id := skills.SkillID("skill:project:compact")
	entry := session.SessionCatalogEntryDigest("sha256:" + strings.Repeat("a", 64))
	loaded := session.SessionLoadedSkillDigest{
		ContentDigest: skills.SkillDigest("sha256:" + strings.Repeat("b", 64)),
		PayloadDigest: skills.InvocationPayloadDigest("sha256:" + strings.Repeat("c", 64)),
	}
	persisted := &session.SessionSkillsMeta{
		ContextEpoch:      7,
		AnnouncedRevision: 4,
		AnnouncedEntries:  map[skills.SkillID]session.SessionCatalogEntryDigest{id: entry},
		LoadedDigests:     map[skills.SkillID]session.SessionLoadedSkillDigest{id: loaded},
	}
	oldCatalog := types.DeveloperMessage("old task28 catalog", types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 4,
	})
	oldBody := types.UserMessage("old task28 loaded SKILL body")
	messages := []types.Message{
		types.UserMessage("head user"),
		types.AssistantMessage("head assistant"),
		oldCatalog,
		oldBody,
		types.AssistantMessage("body acknowledged"),
		types.UserMessage("tail user"),
		types.AssistantMessage("tail assistant"),
	}

	boundary := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "auto"})
	result := &CompactionResult{
		BoundaryMarker: &boundary,
		MessagesToKeep: append(append([]types.Message(nil), messages[:2]...), messages[len(messages)-2:]...),
	}
	result = authorizeCompactionResultForTest(result)
	post := BuildPostCompactMessages(result)
	joined := task28CompactJoin(post)
	if strings.Contains(joined, "old task28 catalog") || strings.Contains(joined, "old task28 loaded SKILL body") {
		t.Fatalf("real compact retained replaced epoch skill evidence: %q", joined)
	}
	if len(post) == 0 || !IsCompactBoundaryMessage(post[0]) {
		t.Fatalf("real compact omitted boundary: %#v", post)
	}

	// History replacement creates epoch 8. With the old catalog/body absent,
	// the same reconciliation used on resume must preserve overrides only and
	// force a full catalog/body rebuild.
	reconciled, err := session.ReconcileSessionSkillsMeta(persisted, session.SessionSkillsVisibleState{ContextEpoch: 8})
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.ContextEpoch != 8 || reconciled.AnnouncedRevision != 0 ||
		len(reconciled.AnnouncedEntries) != 0 || len(reconciled.LoadedDigests) != 0 {
		t.Fatalf("post-compact ledger trusted old epoch=%#v", reconciled)
	}

	currentCatalog := types.DeveloperMessage("current task28 catalog", types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 5,
	})
	currentBody := types.UserMessage("current task28 loaded SKILL body")
	attachments := (&RuntimeAttachmentProvider{
		SessionID:    "task28-session",
		SkillCatalog: task28CompactSkillProvider{currentCatalog, currentBody},
	}).PostCompactAttachments(context.Background(), PostCompactAttachmentState{SessionID: "task28-session"})
	if len(attachments) != 2 || attachments[0].Role != types.RoleDeveloper || attachments[1].Role != types.RoleUser {
		t.Fatalf("rebuilt skill attachments=%#v", attachments)
	}
	result.PreparedMessages = append(append([]types.Message(nil), post...), attachments...)
	prepared := BuildPostCompactMessages(result)
	preparedText := task28CompactJoin(prepared)
	if !strings.Contains(preparedText, "current task28 catalog") || !strings.Contains(preparedText, "current task28 loaded SKILL body") {
		t.Fatalf("rebuilt epoch omitted current evidence=%q", preparedText)
	}
	if strings.Contains(preparedText, "old task28 catalog") || strings.Contains(preparedText, "old task28 loaded SKILL body") {
		t.Fatalf("rebuilt epoch mixed old evidence=%q", preparedText)
	}
}

type task28CompactSkillProvider []types.Message

func (provider task28CompactSkillProvider) PostCompactSkillAttachments(context.Context, PostCompactAttachmentState) []types.Message {
	return append([]types.Message(nil), provider...)
}

func task28CompactJoin(messages []types.Message) string {
	parts := make([]string, len(messages))
	for index, message := range messages {
		parts[index] = message.GetText()
	}
	return strings.Join(parts, "\n")
}
