package types

import (
	"encoding/json"
	"testing"
)

func TestGetTextMultipleBlocks(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			TextBlock{Type: ContentTypeText, Text: "hello"},
			TextBlock{Type: ContentTypeText, Text: "world"},
		},
	}
	if msg.GetText() != "hello\nworld" {
		t.Errorf("expected 'hello\\nworld', got '%s'", msg.GetText())
	}
}

func TestGetTextNoTextBlocks(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			ToolUseBlock{Type: ContentTypeToolUse, Name: "Bash"},
		},
	}
	if msg.GetText() != "" {
		t.Errorf("expected empty, got '%s'", msg.GetText())
	}
}

func TestGetTextEmptyMessage(t *testing.T) {
	msg := Message{Role: RoleUser}
	if msg.GetText() != "" {
		t.Errorf("expected empty, got '%s'", msg.GetText())
	}
}

func TestToolResultMessageMultiple(t *testing.T) {
	msg := ToolResultMessage(
		ToolResultBlock{ToolUseID: "t1", Content: "r1"},
		ToolResultBlock{ToolUseID: "t2", Content: "r2"},
		ToolResultBlock{ToolUseID: "t3", Content: "r3"},
	)
	if len(msg.Content) != 3 {
		t.Errorf("expected 3 blocks, got %d", len(msg.Content))
	}
	for _, block := range msg.Content {
		if block.GetType() != ContentTypeToolResult {
			t.Errorf("expected tool_result type, got %s", block.GetType())
		}
	}
}

func TestUnmarshalThinkingBlock(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			ThinkingBlock{Type: ContentTypeThinking, Thinking: "deep thought"},
		},
	}
	data, _ := json.Marshal(msg)
	var decoded Message
	json.Unmarshal(data, &decoded)

	if len(decoded.Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(decoded.Content))
	}
	tb, ok := decoded.Content[0].(ThinkingBlock)
	if !ok {
		t.Fatal("expected ThinkingBlock")
	}
	if tb.Thinking != "deep thought" {
		t.Errorf("expected 'deep thought', got '%s'", tb.Thinking)
	}
}

func TestUnmarshalEmptyContent(t *testing.T) {
	data := []byte(`{"role":"user","content":[]}`)
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg.Content) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(msg.Content))
	}
}

func TestUnmarshalInvalidJSON(t *testing.T) {
	data := []byte(`{invalid}`)
	var msg Message
	if err := json.Unmarshal(data, &msg); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestUnmarshalUnknownContentType(t *testing.T) {
	data := []byte(`{"role":"assistant","content":[{"type":"unknown_new_type","text":"fallback"}]}`)
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(msg.Content))
	}
	// Should be preserved as UnknownBlock with the original type and raw JSON.
	ub, ok := msg.Content[0].(UnknownBlock)
	if !ok {
		t.Fatalf("expected UnknownBlock for unknown type, got %T", msg.Content[0])
	}
	if ub.Type != "unknown_new_type" {
		t.Errorf("expected type 'unknown_new_type', got %q", ub.Type)
	}
	if len(ub.Raw) == 0 {
		t.Error("expected Raw to contain the original JSON")
	}
}
