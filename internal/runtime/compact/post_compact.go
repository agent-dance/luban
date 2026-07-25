package compact

import "github.com/agent-dance/luban/types"

// IsReinjectedAttachment reports whether msg is an automatic attachment that
// should be regenerated, rather than summarized again, after compaction.
func IsReinjectedAttachment(msg types.Message) bool {
	if msg.Role != types.RoleUser || !msg.HasInternalControlProvenance() {
		return false
	}
	return isPostCompactReminderMessage(msg)
}

// StripReinjectedAttachments removes regenerated attachments before
// summarization so summaries do not recursively describe runtime projections.
func StripReinjectedAttachments(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return messages
	}
	out := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if IsReinjectedAttachment(msg) {
			continue
		}
		out = append(out, msg)
	}
	if len(out) == len(messages) {
		return messages
	}
	return out
}
