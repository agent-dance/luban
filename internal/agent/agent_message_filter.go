package agent

// agent_message_filter.go mirrors the TS filterIncompleteToolCalls helper from
// src/tools/AgentTool/runAgent.ts. It scrubs orphaned tool_use blocks (i.e.
// tool_use blocks whose corresponding tool_result never landed in the
// transcript, typically because the loop was aborted mid-call) before the
// agent's tail history is forwarded to the model.
//
// Without this scrub the next API request fails with a 400
// "unmatched-tool_use_id" error and the agent gets permanently stuck — the
// only recovery is editing the transcript by hand. The filter walks every
// message in arrival order, collects the set of tool_use_ids that were
// answered by a subsequent tool_result, and drops any tool_use whose id is
// not in that set. Assistant messages that end up with no content blocks
// after filtering are removed entirely so the model is never sent an empty
// assistant turn.

import (
	"github.com/agent-dance/luban/types"
)

// FilterIncompleteToolCalls returns a copy of messages with orphaned tool_use
// blocks (and any assistant message that becomes empty afterwards) removed.
// The input slice is not modified.
func FilterIncompleteToolCalls(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return messages
	}
	resolved := collectResolvedToolUseIDs(messages)
	out := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role != types.RoleAssistant || len(msg.Content) == 0 {
			out = append(out, msg)
			continue
		}
		filtered := make([]types.ContentBlock, 0, len(msg.Content))
		for _, block := range msg.Content {
			if tu, ok := block.(types.ToolUseBlock); ok {
				if _, found := resolved[tu.ID]; !found {
					continue
				}
			}
			filtered = append(filtered, block)
		}
		if len(filtered) == 0 {
			// Drop assistant messages that are entirely orphaned tool_use
			// blocks; sending an empty assistant turn confuses the model.
			continue
		}
		out = append(out, types.Message{Role: msg.Role, Content: filtered})
	}
	return out
}

// collectResolvedToolUseIDs walks the message history once and returns the
// set of tool_use IDs that were answered by a tool_result block in any
// subsequent user/tool message.
func collectResolvedToolUseIDs(messages []types.Message) map[string]struct{} {
	resolved := map[string]struct{}{}
	for _, msg := range messages {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok && tr.ToolUseID != "" {
				resolved[tr.ToolUseID] = struct{}{}
			}
		}
	}
	return resolved
}
