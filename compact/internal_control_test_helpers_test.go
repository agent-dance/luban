package compact

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func trustedCompactBoundaryForTest(metadata CompactBoundaryMetadata) types.Message {
	return NewCompactBoundaryMessage(metadata, messagecontrol.Runtime())
}

func trustedCompactSummaryForTest(text string) types.Message {
	return newCompactSummaryMessage(text).WithInternalControlProvenance(messagecontrol.Runtime())
}

func trustedPostCompactReminderForTest(lang i18n.Language, key i18n.Key, body string) *types.Message {
	message := newPostCompactReminderMessage(lang, key, body)
	if message == nil {
		return nil
	}
	trusted := message.WithInternalControlProvenance(messagecontrol.Runtime())
	return &trusted
}

func trustedPostCompactFileRecoveryForTest(lang i18n.Language, path, content string) types.Message {
	return newPostCompactFileRecoveryMessage(lang, path, content).WithInternalControlProvenance(messagecontrol.Runtime())
}

func authorizeCompactionResultForTest(result *CompactionResult) *CompactionResult {
	AuthorizeCompactionResult(messagecontrol.Runtime(), result)
	return result
}
