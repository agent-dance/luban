package provider

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/types"
)

// --- Message conversion tests (OpenAI) ---

func TestConvertMessagesToOpenAI_SystemPrompt(t *testing.T) {
	params := Params{
		System: "You are helpful.",
		Messages: []types.Message{
			types.UserMessage("hello"),
		},
	}

	msgs := convertMessagesToOpenAI(params)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected system role, got %s", msgs[0].Role)
	}
	if msgs[0].Content != "You are helpful." {
		t.Errorf("expected system content, got %s", msgs[0].Content)
	}
	if msgs[1].Role != "user" {
		t.Errorf("expected user role, got %s", msgs[1].Role)
	}
}

func TestConvertMessagesToOpenAI_SystemBlocksJoined(t *testing.T) {
	params := Params{
		System: "legacy",
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "first", Cache: true, CacheScope: prompt.CacheScopeGlobal},
			{Text: "second", CacheScope: prompt.CacheScopeOrg},
		},
		Messages: []types.Message{
			types.UserMessage("hello"),
		},
	}

	msgs := convertMessagesToOpenAI(params)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Fatalf("expected system role, got %s", msgs[0].Role)
	}
	if msgs[0].Content != "first\n\nsecond" {
		t.Fatalf("expected joined system blocks, got %q", msgs[0].Content)
	}
}

func TestConvertMessagesToOpenAI_ToolResults(t *testing.T) {
	params := Params{
		Messages: []types.Message{
			types.UserMessage("hi"),
			{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{
					types.TextBlock{Type: types.ContentTypeText, Text: "Let me help."},
					types.ToolUseBlock{
						Type:  types.ContentTypeToolUse,
						ID:    "tool_1",
						Name:  "Bash",
						Input: map[string]any{"command": "ls"},
					},
				},
			},
			types.ToolResultMessage(types.ToolResultBlock{
				ToolUseID: "tool_1",
				Content:   "file1.txt\nfile2.txt",
			}),
		},
	}

	msgs := convertMessagesToOpenAI(params)

	// Should be: user, assistant (with tool_calls), tool result
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Assistant message should have tool_calls
	assistantMsg := msgs[1]
	if assistantMsg.Role != "assistant" {
		t.Errorf("expected assistant role, got %s", assistantMsg.Role)
	}
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(assistantMsg.ToolCalls))
	}
	if assistantMsg.ToolCalls[0].ID != "tool_1" {
		t.Errorf("expected tool_1 ID, got %s", assistantMsg.ToolCalls[0].ID)
	}
	if assistantMsg.ToolCalls[0].Function.Name != "Bash" {
		t.Errorf("expected Bash function, got %s", assistantMsg.ToolCalls[0].Function.Name)
	}

	// Tool result should be a "tool" role message
	toolMsg := msgs[2]
	if toolMsg.Role != "tool" {
		t.Errorf("expected tool role, got %s", toolMsg.Role)
	}
	if toolMsg.ToolCallID != "tool_1" {
		t.Errorf("expected tool_1 call ID, got %s", toolMsg.ToolCallID)
	}
}

func TestConvertMessagesToOpenAI_EmptyToolResultUsesOriginalPlaceholder(t *testing.T) {
	params := Params{
		Messages: []types.Message{
			types.UserMessage("run a silent command"),
			{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{
					types.ToolUseBlock{
						Type:  types.ContentTypeToolUse,
						ID:    "tool_silent",
						Name:  "Bash",
						Input: map[string]any{"command": "true"},
					},
				},
			},
			types.ToolResultMessage(types.ToolResultBlock{
				ToolUseID: "tool_silent",
				Content:   " \n\t",
			}),
		},
	}

	messages := convertMessagesToOpenAI(params)
	if len(messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(messages))
	}
	if got := messages[2].Content; got != "(Bash completed with no output)" {
		t.Fatalf("empty tool result content = %q", got)
	}

	wire, err := json.Marshal(struct {
		Messages any `json:"messages"`
	}{Messages: messages})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var payload struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(wire, &payload); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	content, ok := payload.Messages[2]["content"]
	if !ok || string(content) != `"(Bash completed with no output)"` {
		t.Fatalf("wire tool content = %s, present=%v; request=%s", content, ok, wire)
	}
}

func TestConvertMessagesToOpenAI_StructuredToolResults(t *testing.T) {
	params := Params{
		Messages: []types.Message{
			types.UserMessage("hi"),
			{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{
					types.ToolUseBlock{
						Type:  types.ContentTypeToolUse,
						ID:    "tool_1",
						Name:  "Read",
						Input: map[string]any{"file_path": "/tmp/pic.png"},
					},
				},
			},
			types.ToolResultMessage(types.ToolResultBlock{
				ToolUseID: "tool_1",
				Content:   "Image file read",
				ContentBlocks: []types.ContentBlock{
					types.TextBlock{Type: types.ContentTypeText, Text: "[Image metadata]"},
					types.ImageBlock{
						Type: types.ContentTypeImage,
						Source: &types.ImageSource{
							Type:      "base64",
							MediaType: "image/png",
							Data:      "iVBORw0KGgo=",
						},
					},
				},
			}),
		},
	}

	msgs := convertMessagesToOpenAI(params)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[2].Role != "tool" || msgs[2].Content != "[Image metadata]\n[image]" {
		t.Fatalf("unexpected tool result message: %#v", msgs[2])
	}
	if msgs[3].Role != "user" {
		t.Fatalf("expected follow-up user message, got %s", msgs[3].Role)
	}
	if len(msgs[3].MultiContent) != 2 {
		t.Fatalf("expected text+image multipart follow-up, got %d parts", len(msgs[3].MultiContent))
	}
}

func TestConvertMessagesToOpenAI_ToolReferenceResults(t *testing.T) {
	params := Params{
		Messages: []types.Message{
			types.UserMessage("hi"),
			{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{
					types.ToolUseBlock{
						Type:  types.ContentTypeToolUse,
						ID:    "tool_1",
						Name:  "ToolSearch",
						Input: map[string]any{"query": "select:TaskCreate"},
					},
				},
			},
			types.ToolResultMessage(types.ToolResultBlock{
				ToolUseID: "tool_1",
				Content:   `Loaded 1 tool(s) for "select:TaskCreate": TaskCreate.`,
				ContentBlocks: []types.ContentBlock{
					types.ToolReferenceBlock{
						Type:     types.ContentTypeToolReference,
						ToolName: "TaskCreate",
					},
				},
			}),
		},
	}

	msgs := convertMessagesToOpenAI(params)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[2].Role != "tool" || msgs[2].Content != `Loaded 1 tool(s) for "select:TaskCreate": TaskCreate.` {
		t.Fatalf("unexpected tool result message: %#v", msgs[2])
	}
	if msgs[3].Role != "user" {
		t.Fatalf("expected follow-up user message, got %s", msgs[3].Role)
	}
	if msgs[3].Content != "[tool:TaskCreate]" {
		t.Fatalf("unexpected tool-reference follow-up: %#v", msgs[3])
	}
}

func TestConvertMessagesToOpenAI_ParallelStructuredToolResultsStayAdjacent(t *testing.T) {
	params := Params{Messages: []types.Message{
		types.UserMessage("delegate both"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "agent_1", Name: "Agent", Input: map[string]any{"prompt": "first"}},
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "agent_2", Name: "Agent", Input: map[string]any{"prompt": "second"}},
			},
		},
		types.ToolResultMessage(
			types.ToolResultBlock{ToolUseID: "agent_1", ContentBlocks: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "first result"}}},
			types.ToolResultBlock{ToolUseID: "agent_2", ContentBlocks: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "second result"}}},
		),
	}}

	msgs := convertMessagesToOpenAI(params)
	if len(msgs) != 6 {
		t.Fatalf("messages = %#v, want user, assistant, two tool results, then two attachment messages", msgs)
	}
	if msgs[2].Role != "tool" || msgs[2].ToolCallID != "agent_1" || msgs[3].Role != "tool" || msgs[3].ToolCallID != "agent_2" {
		t.Fatalf("parallel tool results are not adjacent: %#v", msgs)
	}
	if msgs[4].Role != "user" || msgs[4].Content != "first result" || msgs[5].Role != "user" || msgs[5].Content != "second result" {
		t.Fatalf("structured follow-ups = %#v", msgs[4:])
	}
}

func TestConvertMessagesToOpenAI_NoSystem(t *testing.T) {
	params := Params{
		Messages: []types.Message{
			types.UserMessage("hello"),
		},
	}

	msgs := convertMessagesToOpenAI(params)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected user role, got %s", msgs[0].Role)
	}
}

func TestConvertMessagesToOpenAI_ThinkingBlocksIgnored(t *testing.T) {
	// OpenAI doesn't support thinking blocks — they should be dropped gracefully
	params := Params{
		Messages: []types.Message{
			{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{
					types.ThinkingBlock{
						Type:     types.ContentTypeThinking,
						Thinking: "Let me think...",
					},
					types.TextBlock{Type: types.ContentTypeText, Text: "Here's my answer."},
				},
			},
		},
	}

	msgs := convertMessagesToOpenAI(params)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	// Text should only include the text block, not thinking
	if msgs[0].Content != "Here's my answer." {
		t.Errorf("expected only text content, got %s", msgs[0].Content)
	}
}

func TestConvertMessagesToOpenAI_UserImageMessage(t *testing.T) {
	params := Params{
		Messages: []types.Message{
			{
				Role: types.RoleUser,
				Content: []types.ContentBlock{
					types.TextBlock{Type: types.ContentTypeText, Text: "describe"},
					types.ImageBlock{
						Type: types.ContentTypeImage,
						Source: &types.ImageSource{
							Type:      "base64",
							MediaType: "image/png",
							Data:      "iVBORw0KGgo=",
						},
					},
				},
			},
		},
	}

	msgs := convertMessagesToOpenAI(params)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Fatalf("expected user role, got %s", msgs[0].Role)
	}
	if len(msgs[0].MultiContent) != 2 {
		t.Fatalf("expected 2 multipart items, got %d", len(msgs[0].MultiContent))
	}
	if msgs[0].MultiContent[0].Type != "text" || msgs[0].MultiContent[0].Text != "describe" {
		t.Fatalf("unexpected text part: %#v", msgs[0].MultiContent[0])
	}
	if msgs[0].MultiContent[1].Type != "image_url" || msgs[0].MultiContent[1].ImageURL == nil {
		t.Fatalf("unexpected image part: %#v", msgs[0].MultiContent[1])
	}
	if got := msgs[0].MultiContent[1].ImageURL.URL; got != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatalf("unexpected image url: %q", got)
	}
}

// --- Tool conversion tests (OpenAI) ---

func TestConvertToolsToOpenAI(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "Bash",
			Description: "Run a bash command",
			InputSchema: types.JSONSchema{
				Type: "object",
				Properties: map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The command to run",
					},
				},
				Required: []string{"command"},
			},
		},
	}

	result := convertToolsToOpenAI(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0].Function.Name != "Bash" {
		t.Errorf("expected Bash, got %s", result[0].Function.Name)
	}
	if result[0].Function.Description != "Run a bash command" {
		t.Errorf("expected description, got %s", result[0].Function.Description)
	}
}

// --- OpenAI stream processing tests ---

func TestProcessStream_TextOnly(t *testing.T) {
	ch := make(chan types.StreamEvent, 20)

	// Simulate a simple text-only stream
	go func() {
		ch <- types.StreamEvent{
			Type:    types.EventMessageStart,
			Message: &types.APIMessage{Role: types.RoleAssistant},
		}
		ch <- types.StreamEvent{
			Type:         types.EventContentBlockStart,
			Index:        0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "Hello "},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "world!"},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockStop,
			Index: 0,
		}
		ch <- types.StreamEvent{
			Type: types.EventMessageDelta,
			Usage: &types.Usage{
				OutputTokens: 10,
			},
		}
		ch <- types.StreamEvent{Type: types.EventMessageStop}
		close(ch)
	}()

	// Use processStream from query loop logic (test the event channel consumption)
	var textParts []string
	var usage *types.Usage

	for event := range ch {
		switch event.Type {
		case types.EventContentBlockDelta:
			if event.Delta != nil && event.Delta.Type == "text_delta" {
				textParts = append(textParts, event.Delta.Text)
			}
		case types.EventMessageDelta:
			if event.Usage != nil {
				usage = event.Usage
			}
		}
	}

	fullText := ""
	for _, p := range textParts {
		fullText += p
	}
	if fullText != "Hello world!" {
		t.Errorf("expected 'Hello world!', got '%s'", fullText)
	}
	if usage == nil || usage.OutputTokens != 10 {
		t.Errorf("expected 10 output tokens, got %+v", usage)
	}
}

func TestProcessStream_ToolUse(t *testing.T) {
	ch := make(chan types.StreamEvent, 20)

	// Simulate a tool_use stream
	go func() {
		ch <- types.StreamEvent{
			Type:    types.EventMessageStart,
			Message: &types.APIMessage{Role: types.RoleAssistant},
			Usage:   &types.Usage{InputTokens: 100},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockStart,
			Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   "tool_abc",
				Name: "Bash",
			},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{
				Type:        "input_json_delta",
				PartialJSON: `{"comma`,
			},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{
				Type:        "input_json_delta",
				PartialJSON: `nd":"ls -la"}`,
			},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockStop,
			Index: 0,
		}
		ch <- types.StreamEvent{Type: types.EventMessageStop}
		close(ch)
	}()

	var toolName, toolID string
	var jsonParts []string

	for event := range ch {
		switch event.Type {
		case types.EventContentBlockStart:
			if event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeToolUse {
				toolName = event.ContentBlock.Name
				toolID = event.ContentBlock.ID
			}
		case types.EventContentBlockDelta:
			if event.Delta != nil && event.Delta.Type == "input_json_delta" {
				jsonParts = append(jsonParts, event.Delta.PartialJSON)
			}
		}
	}

	if toolName != "Bash" {
		t.Errorf("expected Bash tool, got %s", toolName)
	}
	if toolID != "tool_abc" {
		t.Errorf("expected tool_abc ID, got %s", toolID)
	}

	fullJSON := ""
	for _, p := range jsonParts {
		fullJSON += p
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(fullJSON), &input); err != nil {
		t.Fatalf("failed to parse tool input JSON: %v", err)
	}
	if input["command"] != "ls -la" {
		t.Errorf("expected 'ls -la', got '%v'", input["command"])
	}
}

func TestProcessStream_ThinkingBlock(t *testing.T) {
	ch := make(chan types.StreamEvent, 20)

	go func() {
		ch <- types.StreamEvent{
			Type:    types.EventMessageStart,
			Message: &types.APIMessage{Role: types.RoleAssistant},
		}
		// Thinking block
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockStart,
			Index: 0,
			ContentBlock: &types.ContentDelta{
				Type:      types.ContentTypeThinking,
				Signature: "sig_abc",
			},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: "Let me reason..."},
		}
		ch <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
		// Text block
		ch <- types.StreamEvent{
			Type:         types.EventContentBlockStart,
			Index:        1,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
		}
		ch <- types.StreamEvent{
			Type:  types.EventContentBlockDelta,
			Index: 1,
			Delta: &types.ContentDelta{Type: "text_delta", Text: "The answer is 42."},
		}
		ch <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 1}
		ch <- types.StreamEvent{Type: types.EventMessageStop}
		close(ch)
	}()

	var thinkingText, responseText string
	var gotSignature string

	for event := range ch {
		switch event.Type {
		case types.EventContentBlockStart:
			if event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeThinking {
				gotSignature = event.ContentBlock.Signature
			}
		case types.EventContentBlockDelta:
			if event.Delta != nil {
				switch event.Delta.Type {
				case "thinking_delta":
					thinkingText += event.Delta.Thinking
				case "text_delta":
					responseText += event.Delta.Text
				}
			}
		}
	}

	if thinkingText != "Let me reason..." {
		t.Errorf("expected thinking text, got '%s'", thinkingText)
	}
	if responseText != "The answer is 42." {
		t.Errorf("expected response text, got '%s'", responseText)
	}
	if gotSignature != "sig_abc" {
		t.Errorf("expected signature sig_abc, got '%s'", gotSignature)
	}
}

func TestProcessStream_ErrorEvent(t *testing.T) {
	ch := make(chan types.StreamEvent, 5)

	go func() {
		ch <- types.StreamEvent{
			Type:    types.EventMessageStart,
			Message: &types.APIMessage{Role: types.RoleAssistant},
		}
		ch <- types.StreamEvent{
			Type:  types.EventError,
			Error: &types.APIError{Type: "overloaded_error", Message: "API is overloaded"},
		}
		close(ch)
	}()

	var gotError *types.APIError
	for event := range ch {
		if event.Type == types.EventError && event.Error != nil {
			gotError = event.Error
		}
	}

	if gotError == nil {
		t.Fatal("expected error event")
	}
	if gotError.Type != "overloaded_error" {
		t.Errorf("expected overloaded_error, got %s", gotError.Type)
	}
	if gotError.Message != "API is overloaded" {
		t.Errorf("expected 'API is overloaded', got %s", gotError.Message)
	}
}

// --- Anthropic conversion tests ---

func TestConvertToAnthropicMessages_RoundTrip(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("What is 2+2?"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "Let me calculate."},
				types.ToolUseBlock{
					Type:  types.ContentTypeToolUse,
					ID:    "tool_1",
					Name:  "Calculator",
					Input: map[string]any{"expr": "2+2"},
				},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "tool_1",
			Content:   "4",
		}),
	}

	result := convertToAnthropicMessages(msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// Verify structure can be serialized (smoke test for SDK compatibility)
	for i, msg := range result {
		data, err := json.Marshal(msg)
		if err != nil {
			t.Errorf("message %d failed to marshal: %v", i, err)
		}
		if len(data) == 0 {
			t.Errorf("message %d produced empty JSON", i)
		}
	}
}

func TestConvertToAnthropicMessages_ThinkingBlock(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ThinkingBlock{
					Type:      types.ContentTypeThinking,
					Thinking:  "Deep thoughts...",
					Signature: "sig_xyz",
				},
				types.TextBlock{Type: types.ContentTypeText, Text: "My answer."},
			},
		},
	}

	result := convertToAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	// Serialize and verify thinking block is included
	data, err := json.Marshal(result[0])
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	s := string(data)
	if !contains(s, "thinking") {
		t.Error("expected thinking block in serialized output")
	}
	if !contains(s, "sig_xyz") {
		t.Error("expected signature in serialized output")
	}
}

func TestConvertToAnthropicMessages_ToolReferenceResult(t *testing.T) {
	msgs := []types.Message{
		types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "tool_search_1",
			Content:   "Loaded TaskCreate",
			ContentBlocks: []types.ContentBlock{
				types.ToolReferenceBlock{
					Type:     types.ContentTypeToolReference,
					ToolName: "TaskCreate",
				},
			},
		}),
	}

	result := convertToAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	data, err := json.Marshal(result[0])
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(data)
	if !contains(s, `"tool_reference"`) || !contains(s, `"TaskCreate"`) {
		t.Fatalf("expected tool_reference block in serialized output, got %s", s)
	}
}

func TestConvertToAnthropicMessages_CacheEditsDedupesAndAddsCacheReferences(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tool_1", Content: "old result"},
				types.UnknownBlock{Type: "cache_edits", Raw: json.RawMessage(`{"type":"cache_edits","edits":[{"type":"delete","cache_reference":"tool_1"},{"type":"delete","cache_reference":"tool_2"}]}`)},
				types.UnknownBlock{Type: "cache_edits", Raw: json.RawMessage(`{"type":"cache_edits","edits":[{"type":"delete","cache_reference":"tool_1"}]}`)},
			},
		},
		types.UserMessage("tail cache marker"),
	}

	result := convertToAnthropicMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("messages = %d, want 2", len(result))
	}
	data, err := json.Marshal(result[0])
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if strings.Count(body, `"type":"cache_edits"`) != 1 {
		t.Fatalf("cache_edits block count mismatch in %s", body)
	}
	if strings.Count(body, `"cache_reference":"tool_1"`) != 2 {
		t.Fatalf("tool_1 should appear once in cache_edits and once on tool_result: %s", body)
	}
	if strings.Count(body, `"cache_reference":"tool_2"`) != 1 {
		t.Fatalf("tool_2 should appear once in cache_edits: %s", body)
	}
	if !strings.Contains(body, `"tool_use_id":"tool_1"`) || !strings.Contains(body, `"cache_reference":"tool_1"`) {
		t.Fatalf("tool_result missing cache_reference: %s", body)
	}
}

func TestConvertToAnthropicTools(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "Read",
			Description: "Reads a file",
			InputSchema: types.JSONSchema{
				Type: "object",
				Properties: map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "Path to the file",
					},
				},
				Required: []string{"file_path"},
			},
		},
		{
			Name:        "Bash",
			Description: "Runs a command",
			InputSchema: types.JSONSchema{
				Type: "object",
				Properties: map[string]any{
					"command": map[string]any{
						"type": "string",
					},
				},
				Required: []string{"command"},
			},
		},
	}

	result, err := convertToAnthropicTools(tools)
	if err != nil {
		t.Fatalf("convertToAnthropicTools returned unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}

	// Verify tools are serializable (smoke test)
	for i, tool := range result {
		data, err := json.Marshal(tool)
		if err != nil {
			t.Errorf("tool %d failed to marshal: %v", i, err)
		}
		if len(data) == 0 {
			t.Errorf("tool %d produced empty JSON", i)
		}
	}
}

func TestMergedAnthropicBetaHeaderAddsToolSearchBeta(t *testing.T) {
	t.Setenv("ENABLE_TOOL_SEARCH", "1")
	t.Setenv("CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS", "")

	got := mergedAnthropicBetaHeader("existing-beta")
	if !contains(got, "existing-beta") || !contains(got, toolSearchBetaHeader1P) {
		t.Fatalf("unexpected merged beta header %q", got)
	}
}

// --- NewFromEnv tests ---

func TestNewFromEnv_MissingAPIKey(t *testing.T) {
	t.Setenv("PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("OAUTH_ACCESS_TOKEN", "")

	registry := DefaultRegistry()
	oldStore := registry.CredentialStoreRef()
	oldHook := registry.OAuthHookRef()
	registry.SetCredentialStore(nil)
	registry.SetOAuthHook(nil)
	t.Cleanup(func() {
		registry.SetCredentialStore(oldStore)
		registry.SetOAuthHook(oldHook)
	})

	p, err := NewFromEnv()
	if err != nil {
		t.Fatalf("unexpected startup error: %v", err)
	}
	if _, ok := p.(*UnconfiguredProvider); !ok {
		t.Fatalf("expected unconfigured provider, got %T", p)
	}
}

func TestNewFromEnv_Ollama(t *testing.T) {
	t.Setenv("PROVIDER", "ollama")

	p, err := NewFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("expected ollama provider, got %s", p.Name())
	}
}

func TestNewFromEnv_OpenAI(t *testing.T) {
	t.Setenv("PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")

	p, err := NewFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("expected openai provider, got %s", p.Name())
	}
}

func TestNewFromEnv_OpenAIMissingKey(t *testing.T) {
	t.Setenv("PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "")

	p, err := NewFromEnv()
	if err != nil {
		t.Fatalf("unexpected startup error: %v", err)
	}
	if p.Name() != "openai" {
		t.Fatalf("expected openai provider, got %s", p.Name())
	}
	if _, ok := p.(*UnconfiguredProvider); !ok {
		t.Fatalf("expected unconfigured provider, got %T", p)
	}
}

func TestNewFromEnv_DeepSeekMissingKey(t *testing.T) {
	t.Setenv("PROVIDER", "deepseek")
	t.Setenv("DEEPSEEK_API_KEY", "")

	p, err := NewFromEnv()
	if err != nil {
		t.Fatalf("unexpected startup error: %v", err)
	}
	if p.Name() != "deepseek" {
		t.Fatalf("expected deepseek provider, got %s", p.Name())
	}
	if _, ok := p.(*UnconfiguredProvider); !ok {
		t.Fatalf("expected unconfigured provider, got %T", p)
	}
}

func TestNewFromEnv_Anthropic(t *testing.T) {
	t.Setenv("PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	p, err := NewFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("expected anthropic provider, got %s", p.Name())
	}
}

func TestNewFromEnv_UnknownProviderReturnsError(t *testing.T) {
	t.Setenv("PROVIDER", "myprovider")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")

	_, err := NewFromEnvWithOverrides("myprovider", "")
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if !contains(strings.ToLower(err.Error()), "unknown provider") {
		t.Errorf("expected error to mention 'unknown provider', got: %v", err)
	}
}

// --- Provider interface compliance ---

func TestAnthropicProviderInterface(t *testing.T) {
	p := NewAnthropic(Config{APIKey: "test"})
	var _ Provider = p // compile-time check

	if p.Name() != "anthropic" {
		t.Errorf("expected anthropic, got %s", p.Name())
	}
	if p.ModelID() == "" {
		t.Error("expected non-empty model ID")
	}
}

func TestOpenAIProviderInterface(t *testing.T) {
	p := NewOpenAI(Config{APIKey: "test"})
	var _ Provider = p // compile-time check

	if p.Name() != "openai" {
		t.Errorf("expected openai, got %s", p.Name())
	}
	if p.ModelID() == "" {
		t.Error("expected non-empty model ID")
	}
}

func TestOpenAIProviderDefaults(t *testing.T) {
	p := NewOpenAI(Config{})
	if p.ModelID() != "gpt-5.6-sol" {
		t.Errorf("expected gpt-5.6-sol default, got %s", p.ModelID())
	}
}

func TestAnthropicProviderDefaults(t *testing.T) {
	p := NewAnthropic(Config{})
	if p.ModelID() != "claude-sonnet-5" {
		t.Errorf("expected claude-sonnet-5 default, got %s", p.ModelID())
	}
}

func TestAnthropicProvider_CountTokens(t *testing.T) {
	var requestBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages/count_tokens" {
			t.Fatalf("path = %q, want token-count endpoint", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &requestBody); err != nil {
			t.Fatalf("decode token-count request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":7}`))
	}))
	defer srv.Close()

	p := NewAnthropic(Config{APIKey: "test-key", BaseURL: srv.URL, Model: "claude-sonnet-4-6"})
	got, err := p.CountTokens(context.Background(), "alpha beta")
	if err != nil {
		t.Fatalf("CountTokens: %v", err)
	}
	if got != 7 {
		t.Fatalf("CountTokens = %d, want 7", got)
	}
	if requestBody["model"] != "claude-sonnet-4-6" {
		t.Fatalf("model = %#v", requestBody["model"])
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v", requestBody["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok || message["role"] != "user" {
		t.Fatalf("message = %#v", messages[0])
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 1 || content[0].(map[string]any)["text"] != "alpha beta" {
		t.Fatalf("content = %#v", message["content"])
	}
}

func TestNewAnthropic_PrefersAuthTokenOverAPIKey(t *testing.T) {
	var gotAuth string
	var gotAPIKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	p := NewAnthropic(Config{
		APIKey:    "stale-api-key",
		AuthToken: "bearer-token",
		BaseURL:   srv.URL,
		Model:     "claude-sonnet-4-6",
	})

	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	if gotAuth != "Bearer bearer-token" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer bearer-token")
	}
	if gotAPIKey != "" {
		t.Fatalf("X-Api-Key = %q, want empty", gotAPIKey)
	}
}

func TestAnthropicProvider_SendsMultipleSystemBlocks(t *testing.T) {
	var reqBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	p := NewAnthropic(Config{
		AuthToken: "bearer-token",
		BaseURL:   srv.URL,
		Model:     "claude-sonnet-4-6",
	})
	ch, err := p.CreateStream(context.Background(), Params{
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "first", Cache: true},
			{Text: "second"},
		},
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	system, ok := reqBody["system"].([]any)
	if !ok {
		t.Fatalf("system = %#v, want array", reqBody["system"])
	}
	if len(system) != 2 {
		t.Fatalf("system blocks = %d, want 2", len(system))
	}
	first := system[0].(map[string]any)
	second := system[1].(map[string]any)
	if first["text"] != "first" || second["text"] != "second" {
		t.Fatalf("unexpected system texts: %#v", system)
	}
	if _, ok := first["cache_control"]; !ok {
		t.Fatalf("first system block missing cache_control: %#v", first)
	}
	if _, ok := second["cache_control"]; ok {
		t.Fatalf("second system block should not have cache_control: %#v", second)
	}
}

func TestAnthropicProvider_EmitsCacheControlOnlyForEligibleSystemBlocks(t *testing.T) {
	var reqBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	p := NewAnthropic(Config{
		AuthToken: "bearer-token",
		BaseURL:   srv.URL,
		Model:     "claude-sonnet-4-6",
	})
	ch, err := p.CreateStream(context.Background(), Params{
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "global static", Cache: true, CacheScope: prompt.CacheScopeGlobal},
			{Text: "org static", Cache: true, CacheScope: prompt.CacheScopeOrg},
			{Text: "dynamic"},
		},
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	system, ok := reqBody["system"].([]any)
	if !ok {
		t.Fatalf("system = %#v, want array", reqBody["system"])
	}
	if len(system) != 3 {
		t.Fatalf("system blocks = %d, want 3", len(system))
	}
	global := system[0].(map[string]any)
	org := system[1].(map[string]any)
	dynamic := system[2].(map[string]any)

	if cc := global["cache_control"].(map[string]any); cc["type"] != "ephemeral" || cc["scope"] != nil {
		t.Fatalf("global cache_control = %#v, want only documented ephemeral type", cc)
	}
	if cc := org["cache_control"].(map[string]any); cc["type"] != "ephemeral" || cc["scope"] != nil {
		t.Fatalf("org cache_control = %#v, want only documented ephemeral type", cc)
	}
	if _, ok := dynamic["cache_control"]; ok {
		t.Fatalf("dynamic block should not have cache_control: %#v", dynamic)
	}
}

func TestAnthropicProvider_MaxOutputTokensOverride(t *testing.T) {
	var reqBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	p := NewAnthropic(Config{
		AuthToken: "bearer-token",
		BaseURL:   srv.URL,
		Model:     "claude-sonnet-4-6",
	})
	ch, err := p.CreateStream(context.Background(), Params{
		MaxTokens:               1024,
		MaxOutputTokensOverride: 64000,
		Messages:                []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	if got := reqBody["max_tokens"]; got != float64(64000) {
		t.Fatalf("max_tokens = %#v, want 64000", got)
	}
}

func TestNewAnthropic_SendsCustomHeaders(t *testing.T) {
	var gotCustom string
	var gotTrace string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustom = r.Header.Get("X-Custom-Proxy")
		gotTrace = r.Header.Get("X-Trace-ID")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	p := NewAnthropic(Config{
		AuthToken: "bearer-token",
		BaseURL:   srv.URL,
		Model:     "claude-sonnet-4-6",
		Headers: map[string]string{
			"X-Custom-Proxy": "enabled",
			"X-Trace-ID":     "trace-123",
		},
	})

	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	if gotCustom != "enabled" {
		t.Fatalf("X-Custom-Proxy = %q", gotCustom)
	}
	if gotTrace != "trace-123" {
		t.Fatalf("X-Trace-ID = %q", gotTrace)
	}
}

func TestAnthropicDebugLogPath_Default(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ANTHROPIC_DEBUG_SSE", "1")
	t.Setenv("ANTHROPIC_DEBUG_SSE_FILE", "")

	got, ok := anthropicDebugLogPathFromEnv()
	if !ok {
		t.Fatal("expected debug log path to be enabled")
	}
	want := filepath.Join(home, brand.ConfigDirName, "logs", "anthropic-sse.log")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestAnthropicDebugLogPath_Override(t *testing.T) {
	t.Setenv("ANTHROPIC_DEBUG_SSE", "1")
	t.Setenv("ANTHROPIC_DEBUG_SSE_FILE", "/tmp/custom-anthropic.log")

	got, ok := anthropicDebugLogPathFromEnv()
	if !ok {
		t.Fatal("expected debug log path to be enabled")
	}
	if got != "/tmp/custom-anthropic.log" {
		t.Fatalf("path = %q", got)
	}
}

func TestNormalizeAnthropicResponse_InflatesGzipSSEWithoutHeader(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(bytes.NewReader(buf.Bytes())),
	}

	got, err := normalizeAnthropicResponse(resp, nil)
	if err != nil {
		t.Fatalf("normalizeAnthropicResponse: %v", err)
	}
	body, err := io.ReadAll(got.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n" {
		t.Fatalf("body = %q", string(body))
	}
	if got.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q", got.Header.Get("Content-Encoding"))
	}
}

func TestNewAnthropicBaseTransport_DisablesCompression(t *testing.T) {
	rt := newAnthropicBaseTransport()
	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T", rt)
	}
	if !tr.DisableCompression {
		t.Fatal("expected DisableCompression=true")
	}
}

// --- CreateStream context cancellation ---

func TestOpenAICreateStream_CancelledContext(t *testing.T) {
	p := NewOpenAI(Config{APIKey: "test-key", BaseURL: "http://localhost:1"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := p.CreateStream(ctx, Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
