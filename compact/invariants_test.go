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

func TestGroupMessagesByAPIRoundUsesAssistantID(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("start"),
		{ID: "a", Role: types.RoleAssistant, Content: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "part 1"}}},
		toolResult("tu_1"),
		{ID: "a", Role: types.RoleAssistant, Content: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "part 2"}}},
		{ID: "b", Role: types.RoleAssistant, Content: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "next"}}},
	}

	groups := GroupMessagesByAPIRound(msgs)
	if len(groups) != 3 {
		t.Fatalf("group count = %d, want 3", len(groups))
	}
	if len(groups[0]) != 1 || groups[0][0].Role != types.RoleUser {
		t.Fatalf("first group = %#v, want initial user message", groups[0])
	}
	if len(groups[1]) != 3 || groups[1][0].ID != "a" || groups[1][2].ID != "a" {
		t.Fatalf("second group did not preserve assistant id fragments: %#v", groups[1])
	}
	if len(groups[2]) != 1 || groups[2][0].ID != "b" {
		t.Fatalf("third group = %#v, want assistant b only", groups[2])
	}
}

func TestHistorySnipUsesAPIInvariantHelper(t *testing.T) {
	snip := &HistorySnip{KeepFirst: 1, KeepLast: 3}
	msgs := []types.Message{
		types.UserMessage("first"),
		types.UserMessage("snipped"),
		assistantToolUse("tu_1"),
		toolResult("tu_1"),
		types.AssistantMessage("recent"),
		types.UserMessage("last"),
	}

	result, err := snip.Compact(nil, msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := result.MessagesToKeep
	if len(got) != 5 {
		t.Fatalf("messages to keep len = %d, want 5", len(got))
	}
	if !got[1].HasToolUse() {
		t.Fatalf("expected helper to preserve tool_use before tail tool_result, got %#v", got)
	}
	if _, ok := got[2].Content[0].(types.ToolResultBlock); !ok {
		t.Fatalf("expected preserved tool_result after tool_use, got %#v", got[2])
	}
}
