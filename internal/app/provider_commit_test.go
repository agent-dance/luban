package app

import (
	"sort"

	"github.com/agent-dance/luban/types"
)

func authorizeAppTestToolStreams(events []types.StreamEvent) []types.StreamEvent {
	calls := make(map[int]types.ProviderToolCallCommit)
	for index := range events {
		event := &events[index]
		switch event.Type {
		case types.EventContentBlockStart:
			if block := event.ContentBlock; block != nil && block.Type == types.ContentTypeToolUse {
				calls[event.Index] = types.ProviderToolCallCommit{
					OutputIndex: event.Index, ToolType: block.ToolType, ProviderItemID: block.ProviderItemID,
					CallID: block.ID, Name: block.Name,
				}
			}
		case types.EventContentBlockDelta:
			call, ok := calls[event.Index]
			if !ok || event.Delta == nil {
				continue
			}
			switch event.Delta.Type {
			case "input_json_delta":
				call.RawInput += event.Delta.PartialJSON
			case "input_text_delta":
				call.ToolType = types.ToolDefinitionTypeCustom
				call.RawInput += event.Delta.PartialText
			case "tool_state_final":
				if call.ToolType == types.ToolDefinitionTypeCustom || event.Delta.ToolType == types.ToolDefinitionTypeCustom {
					call.ToolType = types.ToolDefinitionTypeCustom
					call.RawInput = event.Delta.PartialText
				} else {
					call.RawInput = event.Delta.PartialJSON
				}
			}
			calls[event.Index] = call
		case types.EventMessageStop:
			if event.ProviderCommitReceipt != nil || len(calls) == 0 {
				continue
			}
			ordered := make([]types.ProviderToolCallCommit, 0, len(calls))
			for _, call := range calls {
				ordered = append(ordered, call)
			}
			sort.Slice(ordered, func(left, right int) bool { return ordered[left].OutputIndex < ordered[right].OutputIndex })
			event.ProviderCommitReceipt = types.NewProviderToolCommitReceipt("test", "test", "completed", ordered)
		}
	}
	return events
}
