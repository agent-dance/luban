package loop

import (
	"github.com/agent-dance/luban/i18n"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/contracts/stream"
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
			missing = append(missing, missingToolResultBlock(use, reason, ""))
		}
	}
	return missing
}

// repairInstalledMissingToolResults closes orphaned assistant tool calls at a
// wholesale history-install fence. It deliberately does not participate in the
// live execution path: an assistant tool call there may still be running.
//
// Results are inserted after any already-persisted sibling results and before
// the next ordinary message. Appending them to the end would leave a developer
// catalog or a later user turn between the call and its required result, which
// is not a valid provider continuation.
func repairInstalledMissingToolResults(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return messages
	}
	reason := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeMissingToolResult)

	resultLocations := make(map[string][]installedToolResultLocation)
	for messageIndex, message := range messages {
		if message.Role != types.RoleUser {
			continue
		}
		for blockIndex, block := range message.Content {
			result, ok := block.(types.ToolResultBlock)
			if ok && result.ToolUseID != "" {
				resultLocations[result.ToolUseID] = append(resultLocations[result.ToolUseID], installedToolResultLocation{
					messageIndex: messageIndex,
					blockIndex:   blockIndex,
					result:       result,
				})
			}
		}
	}

	insertBefore := make(map[int][]types.ToolResultBlock)
	removeBlocks := make(map[int]map[int]struct{})
	scheduled := make(map[string]struct{})
	for messageIndex, message := range messages {
		if message.Role != types.RoleAssistant {
			continue
		}
		for _, use := range message.GetToolUses() {
			if use.ID == "" {
				continue
			}
			if _, duplicate := scheduled[use.ID]; duplicate {
				continue
			}
			scheduled[use.ID] = struct{}{}
			boundary := nextInstalledToolResultBoundary(messages, messageIndex+1)
			locations := resultLocations[use.ID]
			if hasInstalledToolResultBeforeBoundary(messages, locations, messageIndex, boundary) {
				continue
			}

			// A result that appears after an ordinary developer/user message is
			// authoritative but out of protocol order. Move it to the call's result
			// boundary instead of synthesizing a duplicate.
			if late, ok := firstInstalledToolResultAfterBoundary(locations, messageIndex, boundary); ok {
				result := late.result
				result.Type = types.ContentTypeToolResult
				result.ToolType = use.ToolType
				insertBefore[boundary] = append(insertBefore[boundary], result)
				if removeBlocks[late.messageIndex] == nil {
					removeBlocks[late.messageIndex] = make(map[int]struct{})
				}
				removeBlocks[late.messageIndex][late.blockIndex] = struct{}{}
				continue
			}
			insertBefore[boundary] = append(insertBefore[boundary], missingToolResultBlock(use, reason, types.ToolOutcomeCancelled))
		}
	}
	if len(insertBefore) == 0 && len(removeBlocks) == 0 {
		return messages
	}

	repaired := make([]types.Message, 0, len(messages)+len(insertBefore))
	for messageIndex, message := range messages {
		if results := insertBefore[messageIndex]; len(results) > 0 {
			repaired = append(repaired, types.ToolResultMessage(results...))
		}
		if removals := removeBlocks[messageIndex]; len(removals) > 0 {
			content := make([]types.ContentBlock, 0, len(message.Content)-len(removals))
			for blockIndex, block := range message.Content {
				if _, remove := removals[blockIndex]; !remove {
					content = append(content, block)
				}
			}
			if len(content) == 0 {
				continue
			}
			message.Content = content
		}
		repaired = append(repaired, message)
	}
	if results := insertBefore[len(messages)]; len(results) > 0 {
		repaired = append(repaired, types.ToolResultMessage(results...))
	}
	return repaired
}

type installedToolResultLocation struct {
	messageIndex int
	blockIndex   int
	result       types.ToolResultBlock
}

func hasInstalledToolResultBeforeBoundary(messages []types.Message, locations []installedToolResultLocation, callIndex, boundary int) bool {
	for _, location := range locations {
		if location.messageIndex <= callIndex {
			continue
		}
		if location.messageIndex < boundary {
			return true
		}
		// Responses projects tool results from a mixed user message before its
		// ordinary content, so a matching result at that user boundary is valid.
		if location.messageIndex == boundary && boundary < len(messages) && messages[boundary].Role == types.RoleUser {
			return true
		}
	}
	return false
}

func firstInstalledToolResultAfterBoundary(locations []installedToolResultLocation, callIndex, boundary int) (installedToolResultLocation, bool) {
	for _, location := range locations {
		if location.messageIndex > callIndex && location.messageIndex > boundary {
			return location, true
		}
	}
	return installedToolResultLocation{}, false
}

func nextInstalledToolResultBoundary(messages []types.Message, start int) int {
	for messageIndex := start; messageIndex < len(messages); messageIndex++ {
		if !isPureToolResultMessage(messages[messageIndex]) {
			return messageIndex
		}
	}
	return len(messages)
}

func isPureToolResultMessage(message types.Message) bool {
	if message.Role != types.RoleUser || len(message.Content) == 0 {
		return false
	}
	for _, block := range message.Content {
		if _, ok := block.(types.ToolResultBlock); !ok {
			return false
		}
	}
	return true
}

func missingToolResultBlock(use types.ToolUseBlock, reason string, outcome types.ToolOutcome) types.ToolResultBlock {
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: use.ID,
		ToolType:  use.ToolType,
		Content:   reason,
		IsError:   true,
		Outcome:   outcome,
	}
}

func appendMissingToolResults(messages []types.Message, reason string, outcome types.ToolOutcome, exec executioncontract.ToolExecutionContext, onEvent func(stream.Event), turnCount int) []types.Message {
	results := missingToolResultBlocks(messages, reason)
	if len(results) == 0 {
		return messages
	}
	for i := range results {
		result := results[i]
		result.Outcome = outcome
		if onEvent != nil {
			onEvent(stream.Event{Type: stream.EventToolResult, ToolResult: &result, TurnCount: turnCount, TurnID: exec.TurnID,
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
