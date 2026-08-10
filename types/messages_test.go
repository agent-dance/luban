package types

import (
	"encoding/json"
	"testing"
)

func TestUserMessage(t *testing.T) {
	msg := UserMessage("hello")
	if msg.Role != RoleUser {
		t.Errorf("expected role %s, got %s", RoleUser, msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(msg.Content))
	}
	if msg.GetText() != "hello" {
		t.Errorf("expected text 'hello', got '%s'", msg.GetText())
	}
}

func TestAssistantMessageFunc(t *testing.T) {
	msg := AssistantMessage("world")
	if msg.Role != RoleAssistant {
		t.Errorf("expected role %s, got %s", RoleAssistant, msg.Role)
	}
	if msg.GetText() != "world" {
		t.Errorf("expected text 'world', got '%s'", msg.GetText())
	}
}

func TestToolResultMessage(t *testing.T) {
	result := ToolResultBlock{
		ToolUseID: "test-id",
		Content:   "result text",
	}
	msg := ToolResultMessage(result)
	if msg.Role != RoleUser {
		t.Errorf("expected role %s, got %s", RoleUser, msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(msg.Content))
	}
	if msg.Content[0].GetType() != ContentTypeToolResult {
		t.Errorf("expected type %s, got %s", ContentTypeToolResult, msg.Content[0].GetType())
	}
}

func TestToolResultMessageStructuredRoundTrip(t *testing.T) {
	msg := ToolResultMessage(ToolResultBlock{
		ToolUseID: "tool_1",
		Content:   "summary",
		ContentBlocks: []ContentBlock{
			TextBlock{Type: ContentTypeText, Text: "hello"},
			ImageBlock{
				Type: ContentTypeImage,
				Source: &ImageSource{
					Type:      "base64",
					MediaType: "image/png",
					Data:      "abc123",
				},
			},
		},
	})

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(decoded.Content))
	}
	tr, ok := decoded.Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("expected ToolResultBlock, got %#v", decoded.Content[0])
	}
	if !tr.HasStructuredContent() {
		t.Fatalf("expected structured content")
	}
	if got := tr.TextContent(); got != "hello\n[image]" {
		t.Fatalf("TextContent = %q, want %q", got, "hello\n[image]")
	}
	if len(tr.ContentBlocks) != 2 {
		t.Fatalf("expected 2 nested content blocks, got %d", len(tr.ContentBlocks))
	}
}

func TestToolResultMessageToolReferenceRoundTrip(t *testing.T) {
	msg := ToolResultMessage(ToolResultBlock{
		ToolUseID: "tool_search_1",
		Content:   "Loaded 1 tool",
		ContentBlocks: []ContentBlock{
			ToolReferenceBlock{
				Type:     ContentTypeToolReference,
				ToolName: "TaskCreate",
			},
		},
	})

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	tr, ok := decoded.Content[0].(ToolResultBlock)
	if !ok {
		t.Fatalf("expected ToolResultBlock, got %#v", decoded.Content[0])
	}
	if len(tr.ContentBlocks) != 1 {
		t.Fatalf("expected 1 nested content block, got %d", len(tr.ContentBlocks))
	}
	ref, ok := tr.ContentBlocks[0].(ToolReferenceBlock)
	if !ok {
		t.Fatalf("expected ToolReferenceBlock, got %#v", tr.ContentBlocks[0])
	}
	if ref.ToolName != "TaskCreate" {
		t.Fatalf("unexpected tool reference %q", ref.ToolName)
	}
}

func TestGetToolUses(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			TextBlock{Type: ContentTypeText, Text: "I'll help you."},
			ToolUseBlock{Type: ContentTypeToolUse, ID: "t1", Name: "Bash", Input: map[string]any{"command": "ls"}},
		},
	}
	uses := msg.GetToolUses()
	if len(uses) != 1 {
		t.Fatalf("expected 1 tool use, got %d", len(uses))
	}
	if uses[0].Name != "Bash" {
		t.Errorf("expected tool name 'Bash', got '%s'", uses[0].Name)
	}
}

func TestMessageMarshalJSON(t *testing.T) {
	msg := UserMessage("hello")
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Role != RoleUser {
		t.Errorf("expected role %s, got %s", RoleUser, decoded.Role)
	}
	if decoded.GetText() != "hello" {
		t.Errorf("expected text 'hello', got '%s'", decoded.GetText())
	}
}

func TestMessageWithToolUseMarshalRoundTrip(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			TextBlock{Type: ContentTypeText, Text: "Let me check."},
			ToolUseBlock{
				Type:  ContentTypeToolUse,
				ID:    "tool_123",
				Name:  "Bash",
				Input: map[string]any{"command": "ls -la"},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	uses := decoded.GetToolUses()
	if len(uses) != 1 {
		t.Fatalf("expected 1 tool use, got %d", len(uses))
	}
	if uses[0].ID != "tool_123" {
		t.Errorf("expected tool ID 'tool_123', got '%s'", uses[0].ID)
	}
	if uses[0].Name != "Bash" {
		t.Errorf("expected tool name 'Bash', got '%s'", uses[0].Name)
	}
}

func TestMessageWithInvalidToolUseAndThinkingKindRoundTrip(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			ThinkingBlock{Type: ContentTypeThinking, Thinking: "summary", Kind: ThinkingKindSummary, ProviderStatus: "completed"},
			InvalidToolUseBlock{
				Type: ContentTypeInvalidToolUse, ID: "call_bad", Name: "Inspect",
				RawInput: "{", InputBytes: 1, InputDigest: "sha256:test",
				FailureKind: ToolInputFailureInvalidJSON, Recoverable: true,
			},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	thinking, ok := decoded.Content[0].(ThinkingBlock)
	if !ok || thinking.Kind != ThinkingKindSummary || thinking.ProviderStatus != "completed" {
		t.Fatalf("thinking round trip = %#v", decoded.Content[0])
	}
	invalid := decoded.GetInvalidToolUses()
	if len(invalid) != 1 || invalid[0].Name != "Inspect" || invalid[0].FailureKind != ToolInputFailureInvalidJSON || !invalid[0].Recoverable {
		t.Fatalf("invalid tool round trip = %#v", invalid)
	}
}
