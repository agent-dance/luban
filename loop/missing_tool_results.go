package loop

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func missingToolResultBlocks(messages []types.Message, reason string) []types.ToolResultBlock {
	if reason == "" {
		reason = i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeMissingToolResult)
	}
	resultIDs := make(map[string]struct{})
	for _, msg := range messages {
		for _, block := range msg.Content {
			if result, ok := block.(types.ToolResultBlock); ok && result.ToolUseID != "" {
				resultIDs[result.ToolUseID] = struct{}{}
			}
		}
	}

	var missing []types.ToolResultBlock
	for _, msg := range messages {
		if msg.Role != types.RoleAssistant {
			continue
		}
		for _, use := range msg.GetToolUses() {
			if _, ok := resultIDs[use.ID]; ok {
				continue
			}
			resultIDs[use.ID] = struct{}{}
			missing = append(missing, types.ToolResultBlock{
				Type:      types.ContentTypeToolResult,
				ToolUseID: use.ID,
				Content:   reason,
				IsError:   true,
			})
		}
	}
	return missing
}

func appendMissingToolResults(messages []types.Message, reason string, outcome types.ToolOutcome, exec ToolExecutionContext, onEvent func(Event), turnCount int) []types.Message {
	results := missingToolResultBlocks(messages, reason)
	if len(results) == 0 {
		return messages
	}
	for i := range results {
		result := results[i]
		result.Outcome = outcome
		if onEvent != nil {
			onEvent(Event{Type: EventToolResult, ToolResult: &result, TurnCount: turnCount, TurnID: exec.TurnID,
				ActorID: exec.ActorID, ActorType: exec.ActorType, WorkUnitID: exec.WorkUnitID})
		}
	}
	return append(messages, types.ToolResultMessage(results...))
}

func validToolResults(results []types.ToolResultBlock) []types.ToolResultBlock {
	out := make([]types.ToolResultBlock, 0, len(results))
	for _, result := range results {
		if result.ToolUseID == "" {
			continue
		}
		out = append(out, result)
	}
	return out
}
