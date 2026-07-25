package compact

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func assistantToolUse(id string) types.Message {
	return types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: id, Name: "Read", Input: map[string]any{"file_path": "/tmp/a"}},
		},
	}
}

func toolResult(id string) types.Message {
	return types.ToolResultMessage(types.ToolResultBlock{ToolUseID: id, Content: "ok"})
}

func TestAdjustIndexToPreserveAPIInvariantsToolResultAtTail(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("old"),
		assistantToolUse("tu_1"),
		toolResult("tu_1"),
		types.AssistantMessage("done"),
	}

	if got := AdjustIndexToPreserveAPIInvariants(msgs, 2); got != 1 {
		t.Fatalf("adjusted index = %d, want 1", got)
	}
}

func TestAdjustIndexToPreserveAPIInvariantsOrphanToolUse(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("old"),
		assistantToolUse("orphan"),
		types.AssistantMessage("next"),
	}

	if got := AdjustIndexToPreserveAPIInvariants(msgs, 1); got != 1 {
		t.Fatalf("orphan tool_use adjusted index = %d, want 1", got)
	}
}

func TestAdjustIndexToPreserveAPIInvariantsAssistantFragmentsSameID(t *testing.T) {
	msgs := []types.Message{
		{
			ID:   "msg_1",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ThinkingBlock{Type: types.ContentTypeThinking, Thinking: "thinking"},
			},
		},
		{
			ID:   "msg_1",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_1", Name: "Read", Input: map[string]any{"file_path": "/tmp/a"}},
			},
		},
		toolResult("tu_1"),
	}

	if got := AdjustIndexToPreserveAPIInvariants(msgs, 1); got != 0 {
		t.Fatalf("same assistant id adjusted index = %d, want 0", got)
	}
}

func TestAdjustIndexToPreserveAPIInvariantsAssistantFirstSlicedHistory(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("assistant tail"),
		types.UserMessage("new user"),
	}

	if got := AdjustIndexToPreserveAPIInvariants(msgs, 1); got != 1 {
		t.Fatalf("assistant-first tail adjusted index = %d, want 1", got)
	}
}

func TestAdjustIndexToPreserveAPIInvariantsSkipsOrphanLeadingToolResult(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("old"),
		toolResult("missing"),
		types.AssistantMessage("safe"),
	}

	if got := AdjustIndexToPreserveAPIInvariants(msgs, 1); got != 2 {
		t.Fatalf("orphan leading tool_result adjusted index = %d, want 2", got)
	}
}
