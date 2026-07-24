package tools

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestAgentFilterIncompleteToolCalls_DropsOrphanedToolUse(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "do x"},
		}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "ok"},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_1", Name: "Read", Input: map[string]any{}},
		}},
		// tool_result for tu_1 missing — this is the abort case.
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_2", Name: "Read", Input: map[string]any{}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tu_2", Content: "fine"},
		}},
	}
	out := FilterIncompleteToolCalls(messages)
	if len(out) != 4 {
		t.Fatalf("expected 4 messages after filtering, got %d", len(out))
	}
	// The first assistant message should still have its TextBlock but no orphan tu_1.
	if len(out[1].Content) != 1 {
		t.Fatalf("expected first assistant msg to retain only text, got %d blocks", len(out[1].Content))
	}
	if _, ok := out[1].Content[0].(types.TextBlock); !ok {
		t.Fatalf("expected residual TextBlock, got %T", out[1].Content[0])
	}
	// tu_2 must remain because it was resolved.
	if tu, ok := out[2].Content[0].(types.ToolUseBlock); !ok || tu.ID != "tu_2" {
		t.Fatalf("expected tu_2 preserved, got %T %v", out[2].Content[0], out[2].Content[0])
	}
	if _, ok := out[3].Content[0].(types.ToolResultBlock); !ok {
		t.Fatalf("expected tool result preserved, got %T", out[3].Content[0])
	}
}

func TestAgentFilterIncompleteToolCalls_DropsEmptyAssistantTurn(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_orphan", Name: "Bash", Input: map[string]any{}},
		}},
	}
	out := FilterIncompleteToolCalls(messages)
	if len(out) != 0 {
		t.Fatalf("expected empty assistant turn dropped, got %d messages", len(out))
	}
}

func TestAgentFilterIncompleteToolCalls_PreservesEmptyInput(t *testing.T) {
	if got := FilterIncompleteToolCalls(nil); got != nil && len(got) != 0 {
		t.Fatalf("expected nil/empty result for nil input, got %v", got)
	}
}
