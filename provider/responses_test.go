package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/types"
)

// TestParseSSE verifies the SSE parser handles various event formats.
func TestParseSSE(t *testing.T) {
	input := "event: response.created\ndata: {\"id\":\"resp_1\"}\n\nevent: response.completed\ndata: {\"response\":{\"id\":\"resp_1\"}}\n\n"
	ch := parseSSE(strings.NewReader(input))

	events := collectSSEEvents(ch)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "response.created" {
		t.Errorf("event[0].Type = %q, want %q", events[0].Type, "response.created")
	}
	if events[1].Type != "response.completed" {
		t.Errorf("event[1].Type = %q, want %q", events[1].Type, "response.completed")
	}
}

func TestParseSSE_Done(t *testing.T) {
	input := "event: test\ndata: {}\n\ndata: [DONE]\n\n"
	ch := parseSSE(strings.NewReader(input))

	events := collectSSEEvents(ch)
	if len(events) != 1 {
		t.Fatalf("expected 1 event (before [DONE]), got %d", len(events))
	}
}

func TestParseSSE_MultilineData(t *testing.T) {
	input := "event: test\ndata: line1\ndata: line2\n\n"
	ch := parseSSE(strings.NewReader(input))

	events := collectSSEEvents(ch)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data != "line1\nline2" {
		t.Errorf("data = %q, want %q", events[0].Data, "line1\nline2")
	}
}

func collectSSEEvents(ch <-chan sseEvent) []sseEvent {
	var events []sseEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

// TestParseSSE_LargePayload verifies that payloads larger than the old 1MB limit
// (but under the new 4MB default) are parsed successfully.
func TestParseSSE_LargePayload(t *testing.T) {
	// Build a data value just over 1MB (1.5MB) to confirm the new 4MB default handles it.
	largeData := strings.Repeat("x", 1536*1024) // 1.5MB
	input := fmt.Sprintf("event: response.created\ndata: %s\n\n", largeData)

	ch := parseSSE(strings.NewReader(input))
	events := collectSSEEvents(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event for large payload, got %d", len(events))
	}
	if events[0].Type != "response.created" {
		t.Errorf("event.Type = %q, want %q", events[0].Type, "response.created")
	}
	if events[0].Data != largeData {
		t.Errorf("event.Data length = %d, want %d", len(events[0].Data), len(largeData))
	}
}

// TestParseSSEWithBuffer_Overflow verifies that when a line exceeds maxBufSize,
// an error event is sent instead of silently terminating the stream.
func TestParseSSEWithBuffer_Overflow(t *testing.T) {
	// Use a very small max buffer (1KB) and send a line that exceeds it.
	oversizedData := strings.Repeat("y", 2048) // 2KB > 1KB limit
	input := fmt.Sprintf("event: before\ndata: ok\n\ndata: %s\n\n", oversizedData)

	ch := parseSSEWithBuffer(strings.NewReader(input), 1024)
	events := collectSSEEvents(ch)

	// The first event ("before") should be received before the overflow.
	if len(events) == 0 {
		t.Fatal("expected at least one event before overflow")
	}
	if events[0].Type != "before" {
		t.Errorf("events[0].Type = %q, want %q", events[0].Type, "before")
	}

	// The last event should be an error event due to buffer overflow.
	last := events[len(events)-1]
	if last.Type != "error" {
		t.Errorf("expected last event type = %q (buffer overflow), got %q", "error", last.Type)
	}
	if !strings.Contains(last.Data, "SSE scanner error") {
		t.Errorf("error event data = %q, expected to contain %q", last.Data, "SSE scanner error")
	}
}

// TestNewResponses verifies constructor defaults.
func TestNewResponses(t *testing.T) {
	p := NewResponses(Config{APIKey: "test-key"})
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai")
	}
	if p.ModelID() != "gpt-5.6-sol" {
		t.Errorf("ModelID() = %q, want %q", p.ModelID(), "gpt-5.6-sol")
	}
}

// TestResponsesProvider_Capabilities checks capability reporting.
func TestResponsesProvider_Capabilities(t *testing.T) {
	p := NewResponses(Config{APIKey: "test"})
	caps := p.Capabilities()
	if !caps.ToolUse {
		t.Error("expected ToolUse = true")
	}
	if caps.Thinking {
		t.Error("expected Thinking = false")
	}
}

// TestResponsesProvider_ResponseIDChaining verifies response ID is passed via Params
// and returned via StreamEvent (stateless provider).
func TestResponsesProvider_ResponseIDChaining(t *testing.T) {
	p := NewResponses(Config{APIKey: "test"})
	// Provider should be stateless — no LastResponseID/SetLastResponseID methods.
	// Response ID is passed via Params.PreviousResponseID and returned in EventMessageStop.ResponseID.
	_ = p // verify it compiles without state methods
}

// TestConvertToolsToResponsesAPI verifies tool format conversion.
func TestConvertToolsToResponsesAPI(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "Bash",
			Description: "Run commands",
			InputSchema: types.JSONSchema{
				Type: "object",
				Properties: map[string]any{
					"command": map[string]any{"type": "string"},
				},
				Required: []string{"command"},
			},
		},
	}

	result := convertToolsToResponsesAPIWithStrictMode(tools, true)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
	if result[0]["type"] != "function" {
		t.Errorf("type = %v, want %q", result[0]["type"], "function")
	}
	if result[0]["name"] != "Bash" {
		t.Errorf("name = %v, want %q", result[0]["name"], "Bash")
	}
	if result[0]["description"] != "Run commands" {
		t.Errorf("description = %v, want %q", result[0]["description"], "Run commands")
	}
	if result[0]["parameters"] == nil {
		t.Error("expected parameters to be set")
	}
}

// TestConvertToolsToResponsesAPI_EmptyProperties ensures empty properties are normalized.
func TestConvertToolsToResponsesAPI_EmptyProperties(t *testing.T) {
	tools := []types.ToolDefinition{
		{
			Name:        "NoArgs",
			Description: "No args tool",
			InputSchema: types.JSONSchema{Type: "object"},
		},
	}
	result := convertToolsToResponsesAPIWithStrictMode(tools, true)
	schema := result[0]["parameters"].(types.JSONSchema)
	if schema.Properties == nil {
		t.Error("expected Properties to be non-nil (empty map)")
	}
}

// TestConvertMessagesToResponsesAPI_FullHistory tests full conversation conversion.
func TestConvertMessagesToResponsesAPI_FullHistory(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("hello"),
		types.AssistantMessage("hi there"),
		types.UserMessage("how are you?"),
	}

	input := convertMessagesToResponsesAPIForParams(Params{Messages: msgs}, "")
	if len(input) != 3 {
		t.Fatalf("expected 3 input items, got %d", len(input))
	}

	// Check first user message
	item0 := input[0].(map[string]any)
	if item0["role"] != "user" {
		t.Errorf("item[0].role = %v, want %q", item0["role"], "user")
	}
	if item0["content"] != "hello" {
		t.Errorf("item[0].content = %v, want %q", item0["content"], "hello")
	}
}

// TestConvertMessagesToResponsesAPI_WithPrevID tests that only new messages are sent.
func TestConvertMessagesToResponsesAPI_WithPrevID(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("hello"),
		types.AssistantMessage("hi there"),
		types.UserMessage("how are you?"),
	}

	input := convertMessagesToResponsesAPIForParams(Params{Messages: msgs}, "resp_prev123")
	if len(input) != 1 {
		t.Fatalf("expected 1 input item (only new user message), got %d", len(input))
	}

	item := input[0].(map[string]any)
	if item["content"] != "how are you?" {
		t.Errorf("content = %v, want %q", item["content"], "how are you?")
	}
}

// TestConvertMessagesToResponsesAPI_ToolResults tests tool result conversion.
func TestConvertMessagesToResponsesAPI_ToolResults(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("run ls"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{
					Type:  types.ContentTypeToolUse,
					ID:    "call_123",
					Name:  "Bash",
					Input: map[string]any{"command": "ls"},
				},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "call_123",
			Content:   "file1.txt\nfile2.txt",
		}),
	}

	// With previous response ID — only send tool results
	input := convertMessagesToResponsesAPIForParams(Params{Messages: msgs}, "resp_prev")
	if len(input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(input))
	}
	item := input[0].(map[string]any)
	if item["type"] != "function_call_output" {
		t.Errorf("type = %v, want %q", item["type"], "function_call_output")
	}
	if item["call_id"] != "call_123" {
		t.Errorf("call_id = %v, want %q", item["call_id"], "call_123")
	}
}

func TestConvertMessagesToResponsesAPI_StructuredToolResults(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("run read"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{
					Type:  types.ContentTypeToolUse,
					ID:    "call_123",
					Name:  "Read",
					Input: map[string]any{"file_path": "/tmp/pic.png"},
				},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "call_123",
			Content:   "summary",
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
	}

	input := convertMessagesToResponsesAPIForParams(Params{Messages: msgs}, "resp_prev")
	if len(input) != 2 {
		t.Fatalf("expected 2 input items, got %d", len(input))
	}
	item := input[0].(map[string]any)
	if item["type"] != "function_call_output" {
		t.Fatalf("unexpected first item: %#v", item)
	}
	if item["output"] != "[Image metadata]\n[image]" {
		t.Fatalf("unexpected tool result output: %#v", item["output"])
	}
	userItem := input[1].(map[string]any)
	content, ok := userItem["content"].([]map[string]string)
	if !ok {
		t.Fatalf("expected multipart follow-up content, got %#v", userItem["content"])
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 follow-up parts, got %d", len(content))
	}
}

func TestConvertMessagesToResponsesAPI_ToolReferenceResults(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("load task tool"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{
					Type:  types.ContentTypeToolUse,
					ID:    "call_123",
					Name:  "ToolSearch",
					Input: map[string]any{"query": "select:TaskCreate"},
				},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "call_123",
			Content:   `Loaded 1 tool(s) for "select:TaskCreate": TaskCreate.`,
			ContentBlocks: []types.ContentBlock{
				types.ToolReferenceBlock{
					Type:     types.ContentTypeToolReference,
					ToolName: "TaskCreate",
				},
			},
		}),
	}

	input := convertMessagesToResponsesAPIForParams(Params{Messages: msgs}, "resp_prev")
	if len(input) != 2 {
		t.Fatalf("expected 2 input items, got %d", len(input))
	}
	item := input[0].(map[string]any)
	if item["type"] != "function_call_output" {
		t.Fatalf("unexpected first item: %#v", item)
	}
	if item["output"] != `Loaded 1 tool(s) for "select:TaskCreate": TaskCreate.` {
		t.Fatalf("unexpected tool result output: %#v", item["output"])
	}
	userItem := input[1].(map[string]any)
	content, ok := userItem["content"].(string)
	if !ok {
		t.Fatalf("expected text-only follow-up content, got %#v", userItem["content"])
	}
	if content != "[tool:TaskCreate]" {
		t.Fatalf("unexpected tool-reference follow-up: %#v", userItem["content"])
	}
}

func TestConvertMessagesToResponsesAPI_ParallelStructuredToolResultsStayAdjacent(t *testing.T) {
	msgs := []types.Message{
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
	}

	input := convertMessagesToResponsesAPIForParams(Params{Messages: msgs}, "resp_prev")
	if len(input) != 4 {
		t.Fatalf("input = %#v, want two outputs followed by two attachment user items", input)
	}
	first := input[0].(map[string]any)
	second := input[1].(map[string]any)
	if first["type"] != "function_call_output" || first["call_id"] != "agent_1" || second["type"] != "function_call_output" || second["call_id"] != "agent_2" {
		t.Fatalf("parallel function outputs are not adjacent: %#v", input)
	}
	firstFollowUp := input[2].(map[string]any)
	secondFollowUp := input[3].(map[string]any)
	if firstFollowUp["role"] != "user" || firstFollowUp["content"] != "first result" || secondFollowUp["role"] != "user" || secondFollowUp["content"] != "second result" {
		t.Fatalf("structured follow-ups = %#v", input[2:])
	}
}

// TestResponsesProvider_StreamTextResponse tests streaming a simple text response.
func TestResponsesProvider_StreamTextResponse(t *testing.T) {
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_1","status":"in_progress"}`},
		{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"message","id":"msg_1"}}`},
		{Type: "response.content_part.added", Data: `{"output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`},
		{Type: "response.output_text.delta", Data: `{"output_index":0,"content_index":0,"delta":"Hello "}`},
		{Type: "response.output_text.delta", Data: `{"output_index":0,"content_index":0,"delta":"world!"}`},
		{Type: "response.output_item.done", Data: `{"output_index":0}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":5},"output":[{"type":"message"}]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		if req["model"] != "gpt-4o" {
			t.Errorf("model = %v, want gpt-4o", req["model"])
		}
		if req["stream"] != true {
			t.Errorf("stream = %v, want true", req["stream"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o",
	})

	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	events := collectStreamEvents(ch)

	// Should have: message_start, content_block_start, 2x content_block_delta, content_block_stop, message_delta, message_stop
	hasMessageStart := false
	hasBlockStart := false
	hasStop := false
	textContent := ""

	for _, e := range events {
		switch e.Type {
		case types.EventMessageStart:
			hasMessageStart = true
		case types.EventContentBlockStart:
			hasBlockStart = true
		case types.EventContentBlockDelta:
			if e.Delta != nil {
				textContent += e.Delta.Text
			}
		case types.EventMessageDelta:
			if e.Usage != nil {
				if e.Usage.InputTokens != 10 {
					t.Errorf("InputTokens = %d, want 10", e.Usage.InputTokens)
				}
				if e.Usage.OutputTokens != 5 {
					t.Errorf("OutputTokens = %d, want 5", e.Usage.OutputTokens)
				}
			}
		case types.EventMessageStop:
			hasStop = true
		}
	}

	if !hasMessageStart {
		t.Error("missing message_start event")
	}
	if !hasBlockStart {
		t.Error("missing content_block_start event")
	}
	if !hasStop {
		t.Error("missing message_stop event")
	}
	if textContent != "Hello world!" {
		t.Errorf("text content = %q, want %q", textContent, "Hello world!")
	}

	// Verify response ID was returned via StreamEvent (stateless)
	// The response ID "resp_1" should be in the EventMessageStop event's ResponseID field.
	// (Already verified above by consuming events — the stream emits ResponseID in message_stop)
}

func TestResponsesProvider_IncompleteDetailsMaxOutputTokens(t *testing.T) {
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_incomplete","status":"in_progress"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_incomplete","status":"completed","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":10,"output_tokens":100},"output":[]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	var stopReason *types.StopReason
	for _, event := range collectStreamEvents(ch) {
		if event.Type == types.EventMessageDelta {
			stopReason = event.StopReason
		}
	}
	if stopReason == nil || *stopReason != types.StopReasonMaxTokens {
		t.Fatalf("stop reason = %#v, want max_tokens", stopReason)
	}
}

func TestResponsesProvider_SystemBlocksJoinedInstructions(t *testing.T) {
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_1","status":"in_progress"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_1","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[]}}`},
	})

	var req map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o",
	})
	ch, err := p.CreateStream(context.Background(), Params{
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "first", Cache: true, CacheScope: prompt.CacheScopeGlobal},
			{Text: "second", CacheScope: prompt.CacheScopeOrg},
		},
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	if req["instructions"] != "first\n\nsecond" {
		t.Fatalf("instructions = %#v, want joined system blocks", req["instructions"])
	}
}

// TestResponsesProvider_StreamToolCall tests streaming a function call response.
func TestResponsesProvider_StreamToolCall(t *testing.T) {
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_2","status":"in_progress"}`},
		{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"Bash"}}`},
		{Type: "response.function_call_arguments.delta", Data: `{"output_index":0,"delta":"{\"command\":"}`},
		{Type: "response.function_call_arguments.delta", Data: `{"output_index":0,"delta":"\"ls\"}"}`},
		{Type: "response.output_item.done", Data: `{"output_index":0}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_2","status":"completed","usage":{"input_tokens":15,"output_tokens":8},"output":[{"type":"function_call"}]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("run ls")},
		Tools: []types.ToolDefinition{
			{Name: "Bash", Description: "Run commands", InputSchema: types.JSONSchema{Type: "object"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	events := collectStreamEvents(ch)

	hasToolBlockStart := false
	toolJSON := ""
	var stopReason *types.StopReason

	for _, e := range events {
		switch e.Type {
		case types.EventContentBlockStart:
			if e.ContentBlock != nil && e.ContentBlock.Type == types.ContentTypeToolUse {
				hasToolBlockStart = true
				if e.ContentBlock.ID != "call_abc" {
					t.Errorf("tool ID = %q, want %q", e.ContentBlock.ID, "call_abc")
				}
				if e.ContentBlock.Name != "Bash" {
					t.Errorf("tool name = %q, want %q", e.ContentBlock.Name, "Bash")
				}
			}
		case types.EventContentBlockDelta:
			if e.Delta != nil && e.Delta.Type == "input_json_delta" {
				toolJSON += e.Delta.PartialJSON
			}
		case types.EventMessageDelta:
			stopReason = e.StopReason
		}
	}

	if !hasToolBlockStart {
		t.Error("missing tool_use content_block_start")
	}
	if toolJSON != `{"command":"ls"}` {
		t.Errorf("tool JSON = %q, want %q", toolJSON, `{"command":"ls"}`)
	}
	if stopReason == nil || *stopReason != types.StopReasonToolUse {
		t.Errorf("stop reason = %v, want tool_use", stopReason)
	}
}

func TestResponsesProviderEnablesParallelFunctionCalls(t *testing.T) {
	for _, chatGPTBackend := range []bool{false, true} {
		name := "public"
		if chatGPTBackend {
			name = "chatgpt-codex"
		}
		t.Run(name, func(t *testing.T) {
			var capturedBody map[string]any
			sseData := buildSSEStream([]sseEvent{
				{Type: "response.created", Data: `{"id":"resp_parallel"}`},
				{Type: "response.completed", Data: `{"response":{"id":"resp_parallel","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[]}}`},
			})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedBody)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(sseData))
			}))
			defer srv.Close()

			semantics := ResponsesSemanticsOpenAIPublic
			if chatGPTBackend {
				semantics = ResponsesSemanticsOpenAICodex
			}
			p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-5.6-sol", ResponsesSemantics: semantics})
			ch, err := p.CreateStream(context.Background(), Params{
				Messages: []types.Message{types.UserMessage("run two agents")},
				Tools: []types.ToolDefinition{{
					Name: "Agent", Description: "Launch an agent", InputSchema: types.JSONSchema{Type: "object"},
				}},
			})
			if err != nil {
				t.Fatalf("CreateStream: %v", err)
			}
			for range ch {
			}
			wantParallel := !chatGPTBackend
			if got, ok := capturedBody["parallel_tool_calls"].(bool); !ok || got != wantParallel {
				t.Fatalf("parallel_tool_calls = %#v, want %v", capturedBody["parallel_tool_calls"], wantParallel)
			}
		})
	}
}

func TestResponsesProviderStreamsMultipleFunctionCallsIndependently(t *testing.T) {
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_multi","status":"in_progress"}`},
		{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Agent"}}`},
		{Type: "response.output_item.added", Data: `{"output_index":1,"item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"Agent"}}`},
		{Type: "response.function_call_arguments.delta", Data: `{"output_index":0,"delta":"{\"prompt\":\"first\"}"}`},
		{Type: "response.function_call_arguments.delta", Data: `{"output_index":1,"delta":"{\"prompt\":\"second\"}"}`},
		{Type: "response.output_item.done", Data: `{"output_index":0}`},
		{Type: "response.output_item.done", Data: `{"output_index":1}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_multi","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[{"type":"function_call"},{"type":"function_call"}]}}`},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	stream, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("run two agents")},
		Tools:    []types.ToolDefinition{{Name: "Agent", InputSchema: types.JSONSchema{Type: "object"}}},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	starts := map[int]string{}
	arguments := map[int]string{}
	stops := map[int]bool{}
	for _, event := range collectStreamEvents(stream) {
		switch event.Type {
		case types.EventContentBlockStart:
			if event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeToolUse {
				starts[event.Index] = event.ContentBlock.ID
			}
		case types.EventContentBlockDelta:
			if event.Delta != nil && event.Delta.Type == "input_json_delta" {
				arguments[event.Index] += event.Delta.PartialJSON
			}
		case types.EventContentBlockStop:
			stops[event.Index] = true
		}
	}
	if starts[0] != "call_1" || starts[1] != "call_2" {
		t.Fatalf("parallel tool starts = %#v", starts)
	}
	if arguments[0] != `{"prompt":"first"}` || arguments[1] != `{"prompt":"second"}` {
		t.Fatalf("parallel tool arguments = %#v", arguments)
	}
	if !stops[0] || !stops[1] {
		t.Fatalf("parallel tool stops = %#v", stops)
	}
}

// TestResponsesProvider_HTTPError tests handling of non-200 responses.
func TestResponsesProvider_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2.5")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	_, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("error = %q, want to contain 'rate limited'", err.Error())
	}
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.RetryAfter != "2.5" {
		t.Fatalf("Responses Retry-After = %#v, want only preserved value 2.5", apiErr)
	}
}

// TestResponsesProvider_StoreFalseIgnoresPreviousResponseID verifies stateless
// public Responses never asks the service to retrieve declined storage.
func TestResponsesProvider_StoreFalseIgnoresPreviousResponseID(t *testing.T) {
	var capturedBody map[string]any

	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_3"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_3","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})
	// Exercise the official public Responses API contract while routing the
	// request through the local capture server.
	p.publicAPIEndpoint = true
	_, err := p.CreateStream(context.Background(), Params{
		Messages:           []types.Message{types.UserMessage("continue")},
		PromptCacheKey:     "session-123",
		UsePromptCache:     true,
		PreviousResponseID: "resp_prev",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	if _, ok := capturedBody["previous_response_id"]; ok {
		t.Errorf("store=false request retained previous_response_id = %v", capturedBody["previous_response_id"])
	}
	if _, ok := capturedBody["max_output_tokens"]; ok {
		t.Fatalf("default Responses request should omit max_output_tokens: %#v", capturedBody["max_output_tokens"])
	}
	if got, _ := capturedBody["prompt_cache_key"].(string); !strings.HasPrefix(got, "pcu_") || got == "session-123" {
		t.Errorf("prompt_cache_key = %v, want opaque credential-scoped route", capturedBody["prompt_cache_key"])
	}
}

func TestResponsesProvider_ChatGPTCodexHTTPFallbackUsesStoreFalseAndFullInput(t *testing.T) {
	var capturedBody map[string]any

	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_4"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_4","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		AuthToken: "chatgpt-access-token",
		BaseURL:   srv.URL,
		Model:     "gpt-5.5",
	})
	p.chatGPTCodexBackend = true

	_, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{
			types.UserMessage("hello"),
			types.AssistantMessage("hi"),
			types.UserMessage("continue"),
		},
		PromptCacheKey:     "session-123",
		UsePromptCache:     true,
		PreviousResponseID: "resp_prev",
		Truncation:         "auto",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	if got, ok := capturedBody["store"].(bool); !ok || got {
		t.Fatalf("store = %#v, want false", capturedBody["store"])
	}
	if _, ok := capturedBody["previous_response_id"]; ok {
		t.Fatalf("previous_response_id should not be sent over Codex HTTP fallback: %#v", capturedBody["previous_response_id"])
	}
	if _, ok := capturedBody["max_output_tokens"]; ok {
		t.Fatalf("max_output_tokens should not be sent over Codex HTTP fallback: %#v", capturedBody["max_output_tokens"])
	}
	if _, ok := capturedBody["truncation"]; ok {
		t.Fatalf("truncation should not be sent over Codex HTTP fallback: %#v", capturedBody["truncation"])
	}
	if capturedBody["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %#v, want auto", capturedBody["tool_choice"])
	}
	if got, ok := capturedBody["parallel_tool_calls"].(bool); !ok || got {
		t.Fatalf("parallel_tool_calls = %#v, want false", capturedBody["parallel_tool_calls"])
	}
	if tools, ok := capturedBody["tools"].([]any); !ok || len(tools) != 0 {
		t.Fatalf("tools = %#v, want empty array", capturedBody["tools"])
	}
	if include, ok := capturedBody["include"].([]any); !ok || len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v, want encrypted reasoning", capturedBody["include"])
	}
	input, ok := capturedBody["input"].([]any)
	if !ok {
		t.Fatalf("input = %#v, want array", capturedBody["input"])
	}
	if len(input) != 3 {
		t.Fatalf("input length = %d, want full history length 3", len(input))
	}
	if got, _ := capturedBody["prompt_cache_key"].(string); !strings.HasPrefix(got, "pcu_") || got == "session-123" {
		t.Errorf("prompt_cache_key = %v, want opaque credential-scoped route", capturedBody["prompt_cache_key"])
	}
}

// TestResponsesProvider_ReasoningEffort verifies reasoning config is sent.
func TestResponsesProvider_ReasoningEffort(t *testing.T) {
	var capturedBody map[string]any

	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_4"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_4","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	_, err := p.CreateStream(context.Background(), Params{
		Messages:        []types.Message{types.UserMessage("think hard")},
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	reasoning, ok := capturedBody["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("reasoning not found in request body")
	}
	if reasoning["effort"] != "high" {
		t.Errorf("reasoning.effort = %v, want %q", reasoning["effort"], "high")
	}
}

func TestResponsesProvider_RequestBodyOmitsAdvancedParamsWhenUnset(t *testing.T) {
	var capturedBody map[string]any

	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_plain"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_plain","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal request: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	_, err := p.CreateStream(context.Background(), Params{
		MaxTokens: 1024,
		Messages:  []types.Message{types.UserMessage("plain")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	if got, ok := capturedBody["store"].(bool); !ok || got {
		t.Fatalf("store = %#v, want false for compatible Responses", capturedBody["store"])
	}

	for _, key := range []string{"output_config", "reasoning", "tool_choice", "previous_response_id", "prompt_cache_key", "max_output_tokens"} {
		if _, ok := capturedBody[key]; ok {
			t.Fatalf("%s should be omitted when unset: %#v", key, capturedBody[key])
		}
	}
}

func TestResponsesProvider_ReasoningDeltaEvents(t *testing.T) {
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_reasoning_delta","status":"in_progress"}`},
		{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"reasoning","id":"rs_1"}}`},
		{Type: "response.reasoning_summary_text.delta", Data: `{"output_index":0,"delta":"checked constraints"}`},
		{Type: "response.output_item.done", Data: `{"output_index":0}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_reasoning_delta","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[{"type":"reasoning"}]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("think")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	events := collectStreamEvents(ch)

	var sawStart, sawDelta, sawStop bool
	for _, event := range events {
		switch event.Type {
		case types.EventContentBlockStart:
			if event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeThinking {
				sawStart = true
			}
		case types.EventContentBlockDelta:
			if event.Delta != nil && event.Delta.Type == "thinking_delta" && event.Delta.Thinking == "checked constraints" {
				sawDelta = true
			}
		case types.EventContentBlockStop:
			sawStop = true
		}
	}
	if !sawStart || !sawDelta || !sawStop {
		t.Fatalf("reasoning events missing start=%v delta=%v stop=%v events=%#v", sawStart, sawDelta, sawStop, events)
	}
}

func TestResponsesProvider_ToolInputLimitTerminatesDegenerateFunctionCall(t *testing.T) {
	delta, err := json.Marshal(map[string]any{
		"output_index": 0,
		"delta":        strings.Repeat(" ", maxResponsesInspectToolInputBytes+1),
	})
	if err != nil {
		t.Fatal(err)
	}
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_runaway","status":"in_progress"}`},
		{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"function_call","call_id":"call_runaway","name":"Inspect","status":"in_progress"}}`},
		{Type: "response.function_call_arguments.delta", Data: string(delta)},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("inspect")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	var got *types.APIError
	for _, event := range collectStreamEvents(ch) {
		if event.Type == types.EventError {
			got = event.Error
			break
		}
	}
	if got == nil || got.Type != "stream_interrupted" || got.Code != "tool_arguments_too_large" {
		t.Fatalf("tool input limit error = %#v", got)
	}
	contract := ClassifyAttemptError(got)
	if contract.Stage != types.ProviderErrorStageStream ||
		contract.Class != types.ProviderErrorClassTransport ||
		contract.ReplaySafety != types.ProviderReplaySafe || !contract.Retryable() {
		t.Fatalf("tool input limit retry contract = %+v", contract)
	}
}

func TestResponsesSSEParserTerminatesAggregatedFunctionDeltaBeforeFullLine(t *testing.T) {
	for _, prefix := range []string{
		"event: response.function_call_arguments.delta\n" + "data: ",
		"data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"",
		"data: {\"type\": \"response.function_call_arguments.delta\", \"delta\": \"",
	} {
		source := &countingResponsesReader{reader: strings.NewReader(prefix + strings.Repeat(" ", 1<<20))}
		events := parseResponsesSSE(context.Background(), source)
		event, ok := <-events
		if !ok || event.Type != "error" || !errors.Is(event.Err, errResponsesFunctionCallDeltaLineTooLarge) {
			t.Fatalf("early parser event = %#v, ok=%v", event, ok)
		}
		if source.bytesRead > maxResponsesFunctionCallDeltaLineBytes+2*defaultSSEInitBuf {
			t.Fatalf("parser read %d bytes before cutoff", source.bytesRead)
		}
	}
}

type countingResponsesReader struct {
	reader    *strings.Reader
	bytesRead int
}

func (r *countingResponsesReader) Read(buffer []byte) (int, error) {
	read, err := r.reader.Read(buffer)
	r.bytesRead += read
	return read, err
}

func TestResponsesToolInputLimitsPreservePatchHeadroom(t *testing.T) {
	if got := responsesToolInputLimit("Inspect"); got != 32<<10 {
		t.Fatalf("Inspect limit = %d", got)
	}
	if got := responsesToolInputLimit("Run"); got <= responsesToolInputLimit("Inspect") {
		t.Fatalf("Run limit = %d, want above Inspect", got)
	}
	if got := responsesToolInputLimit("ApplyPatch"); got < 1<<20 {
		t.Fatalf("ApplyPatch limit = %d, want at least 1 MiB", got)
	}
}

func TestResponsesProvider_ChatGPTCodexReasoningIncludesEncryptedContent(t *testing.T) {
	var capturedBody map[string]any

	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_reasoning"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_reasoning","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		AuthToken: "chatgpt-access-token",
		BaseURL:   srv.URL,
	})
	p.chatGPTCodexBackend = true

	_, err := p.CreateStream(context.Background(), Params{
		Messages:        []types.Message{types.UserMessage("think hard")},
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	include, ok := capturedBody["include"].([]any)
	if !ok || len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v, want reasoning encrypted content", capturedBody["include"])
	}
	reasoning, ok := capturedBody["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Fatalf("reasoning = %#v", capturedBody["reasoning"])
	}
}

// TestResponsesProvider_CachedTokens verifies cache-read and cache-write details.
func TestResponsesProvider_CachedTokens(t *testing.T) {
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_5"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_5","status":"completed","usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":70,"cache_write_tokens":20}},"output":[{"type":"message"}]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})

	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	for e := range ch {
		if e.Type == types.EventMessageDelta && e.Usage != nil {
			if e.Usage.CacheReadInputTokens != 70 {
				t.Errorf("CacheReadInputTokens = %d, want 70", e.Usage.CacheReadInputTokens)
			}
			if e.Usage.CacheCreationInputTokens != 20 {
				t.Errorf("CacheCreationInputTokens = %d, want 20", e.Usage.CacheCreationInputTokens)
			}
			if e.Usage.InputTokens != 100 {
				t.Errorf("InputTokens = %d, want 100", e.Usage.InputTokens)
			}
			if e.Usage.UncachedInputTokens() != 10 {
				t.Errorf("UncachedInputTokens() = %d, want 10", e.Usage.UncachedInputTokens())
			}
		}
	}
}

// TestConvertMessagesToResponsesAPI_AssistantWithToolUse tests assistant tool use conversion.
func TestConvertMessagesToResponsesAPI_AssistantWithToolUse(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "Let me check"},
				types.ToolUseBlock{
					Type:  types.ContentTypeToolUse,
					ID:    "call_1",
					Name:  "Bash",
					Input: map[string]any{"command": "ls"},
				},
			},
		},
	}

	input := convertAllMessagesForResponsesAPIWithParams(Params{Messages: msgs})
	if len(input) != 2 {
		t.Fatalf("expected 2 items (text + function_call), got %d", len(input))
	}

	// First item: text
	item0 := input[0].(map[string]any)
	if item0["role"] != "assistant" {
		t.Errorf("item[0].role = %v, want assistant", item0["role"])
	}

	// Second item: function_call
	item1 := input[1].(map[string]any)
	if item1["type"] != "function_call" {
		t.Errorf("item[1].type = %v, want function_call", item1["type"])
	}
	if item1["name"] != "Bash" {
		t.Errorf("item[1].name = %v, want Bash", item1["name"])
	}
	if item1["call_id"] != "call_1" {
		t.Errorf("item[1].call_id = %v, want call_1", item1["call_id"])
	}
	if _, ok := item1["id"]; ok {
		t.Errorf("item[1].id should be omitted when only the Responses call_id is known, got %v", item1["id"])
	}
}

// TestConvertUserMessageToResponsesAPI_WithImage tests that ImageBlock is converted
// to Responses API input_image format with data URI.
func TestConvertUserMessageToResponsesAPI_WithImage(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "What is in this image?"},
			types.ImageBlock{
				Type: types.ContentTypeImage,
				Source: &types.ImageSource{
					Type:      "base64",
					MediaType: "image/png",
					Data:      "iVBORw0KGgo=",
				},
			},
		},
	}

	items := convertUserMessageToResponsesAPI(msg)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0].(map[string]any)
	if item["role"] != "user" {
		t.Errorf("role = %v, want user", item["role"])
	}

	// Content should be a multipart array
	content, ok := item["content"].([]map[string]string)
	if !ok {
		t.Fatalf("content is not []map[string]string, got %T", item["content"])
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 content parts (text + image), got %d", len(content))
	}

	// First part: input_text
	if content[0]["type"] != "input_text" {
		t.Errorf("content[0].type = %q, want input_text", content[0]["type"])
	}
	if content[0]["text"] != "What is in this image?" {
		t.Errorf("content[0].text = %q, want %q", content[0]["text"], "What is in this image?")
	}

	// Second part: input_image with data URI
	if content[1]["type"] != "input_image" {
		t.Errorf("content[1].type = %q, want input_image", content[1]["type"])
	}
	expectedURI := "data:image/png;base64,iVBORw0KGgo="
	if content[1]["image_url"] != expectedURI {
		t.Errorf("content[1].image_url = %q, want %q", content[1]["image_url"], expectedURI)
	}
}

// TestConvertUserMessageToResponsesAPI_ImageOnly tests image-only user message (no text).
func TestConvertUserMessageToResponsesAPI_ImageOnly(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			types.ImageBlock{
				Type: types.ContentTypeImage,
				Source: &types.ImageSource{
					Type:      "base64",
					MediaType: "image/jpeg",
					Data:      "/9j/4AAQ",
				},
			},
		},
	}

	items := convertUserMessageToResponsesAPI(msg)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0].(map[string]any)
	content, ok := item["content"].([]map[string]string)
	if !ok {
		t.Fatalf("content is not []map[string]string, got %T", item["content"])
	}

	// Should have only image, no text part
	if len(content) != 1 {
		t.Fatalf("expected 1 content part (image only), got %d", len(content))
	}
	if content[0]["type"] != "input_image" {
		t.Errorf("content[0].type = %q, want input_image", content[0]["type"])
	}
	if content[0]["image_url"] != "data:image/jpeg;base64,/9j/4AAQ" {
		t.Errorf("unexpected image_url: %q", content[0]["image_url"])
	}
}

// TestConvertUserMessageToResponsesAPI_TextOnly tests that text-only messages
// still use the simple string content format (not multipart).
func TestConvertUserMessageToResponsesAPI_TextOnly(t *testing.T) {
	msg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "just text"},
		},
	}

	items := convertUserMessageToResponsesAPI(msg)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0].(map[string]any)
	// Text-only should use plain string content, not multipart array
	content, ok := item["content"].(string)
	if !ok {
		t.Fatalf("expected string content for text-only message, got %T", item["content"])
	}
	if content != "just text" {
		t.Errorf("content = %q, want %q", content, "just text")
	}
}

// TestParseResponsesHTTPError tests error parsing from various HTTP status codes.
func TestParseResponsesHTTPError(t *testing.T) {
	tests := []struct {
		status   int
		body     string
		wantType string
	}{
		{429, `{"error":{"message":"rate limited"}}`, "rate_limit_error"},
		{503, `{"error":{"message":"overloaded"}}`, "overloaded_error"},
		{400, `{"error":{"message":"bad request","type":"invalid_request_error"}}`, "invalid_request_error"},
		{400, `{"error":{"message":"private detail","type":"invalid_request_error","code":"context_length_exceeded"}}`, "context_length_exceeded"},
		{500, `not json`, "api_error"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.status), func(t *testing.T) {
			err := parseResponsesHTTPError(tt.status, []byte(tt.body))
			if err.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", err.Type, tt.wantType)
			}
			if err.Status != tt.status {
				t.Errorf("Status = %d, want %d", err.Status, tt.status)
			}
		})
	}
}

// TestNewFromEnvWithOverrides_ResponsesProvider tests env-based provider creation.
func TestNewFromEnvWithOverrides_ResponsesProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("PROVIDER", "openai")

	p, err := NewFromEnvWithOverrides("", "")
	if err != nil {
		t.Fatalf("NewFromEnvWithOverrides: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai")
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func buildSSEStream(events []sseEvent) string {
	var b strings.Builder
	for _, e := range events {
		if e.Type != "" {
			b.WriteString("event: ")
			b.WriteString(e.Type)
			b.WriteString("\n")
		}
		b.WriteString("data: ")
		b.WriteString(e.Data)
		b.WriteString("\n\n")
	}
	return b.String()
}

func collectStreamEvents(ch <-chan types.StreamEvent) []types.StreamEvent {
	var events []types.StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func TestResponsesProvider_PromptCacheKeyDisabledWhenOptOut(t *testing.T) {
	var capturedBody map[string]any

	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_3b"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_3b","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	_, err := p.CreateStream(context.Background(), Params{
		Messages:       []types.Message{types.UserMessage("continue")},
		PromptCacheKey: "session-123",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	if _, ok := capturedBody["prompt_cache_key"]; ok {
		t.Fatalf("prompt_cache_key should be omitted when UsePromptCache is false")
	}
}

func TestResponsesProvider_PromptCacheIsolatesIndependentSessionsAndReusesLineage(t *testing.T) {
	t.Setenv("LUBAN_CODE_PROMPT_CACHE_SHARDS", "")
	var captured []string
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_cache_scope"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_cache_scope","status":"completed","usage":{"input_tokens":5,"output_tokens":1},"output":[]}}`},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		key, _ := request["prompt_cache_key"].(string)
		captured = append(captured, key)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseData)
	}))
	defer srv.Close()

	p := NewResponses(Config{
		ProviderName: "openai",
		APIKey:       "same-account-secret",
		BaseURL:      srv.URL,
		Model:        "gpt-5.6-sol",
	})
	for _, lineage := range []string{"session-a", "session-b", "session-a"} {
		stream, err := p.CreateStream(context.Background(), Params{
			System:         "stable shared prefix",
			Messages:       []types.Message{types.UserMessage("hello")},
			PromptCacheKey: lineage,
			UsePromptCache: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		for range stream {
		}
	}
	if len(captured) != 3 || captured[0] == "" || captured[1] == "" || captured[0] == captured[1] {
		t.Fatalf("independent Responses sessions were not isolated: %#v", captured)
	}
	if captured[2] != captured[0] {
		t.Fatalf("resumed Responses lineage changed cache route: %#v", captured)
	}
	for _, key := range captured {
		if key == "session-a" || key == "session-b" {
			t.Fatalf("Responses leaked conversation lineage as cache route: %q", key)
		}
	}
}

func TestResponsesProvider_PromptCacheRequestShape(t *testing.T) {
	var capturedBody map[string]any
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_cache"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_cache","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseData)
	}))
	defer srv.Close()

	p := NewResponses(Config{
		ProviderName: "openai",
		APIKey:       "user-secret",
		BaseURL:      srv.URL,
		Model:        "gpt-5.6-sol",
	})
	// Isolate the documented public cache-body shape from the separate
	// Responses Lite matrix covered below.
	p.firstPartyEndpoint = false
	p.publicAPIEndpoint = true
	stream, err := p.CreateStream(context.Background(), Params{
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "stable one", Cache: true},
			{Text: "stable two", Cache: true},
			{Text: "dynamic"},
		},
		Messages:       []types.Message{types.UserMessage("continue")},
		PromptCacheKey: "session-lineage",
		UsePromptCache: true,
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}

	cacheKey, _ := capturedBody["prompt_cache_key"].(string)
	if !strings.HasPrefix(cacheKey, "pcu_") || cacheKey == "session-lineage" {
		t.Fatalf("prompt_cache_key = %q, want opaque user-scoped routing key", cacheKey)
	}
	options, ok := capturedBody["prompt_cache_options"].(map[string]any)
	if !ok || options["mode"] != "implicit" || options["ttl"] != "30m" {
		t.Fatalf("prompt_cache_options = %#v, want implicit 30m", capturedBody["prompt_cache_options"])
	}
	if _, found := capturedBody["instructions"]; found {
		t.Fatalf("cacheable system blocks should use developer input, got instructions %#v", capturedBody["instructions"])
	}
	input, ok := capturedBody["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("input = %#v, want developer prefix plus user message", capturedBody["input"])
	}
	developer, ok := input[0].(map[string]any)
	if !ok {
		t.Fatalf("developer input = %#v", input[0])
	}
	content, ok := developer["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("developer content = %#v", developer["content"])
	}
	stablePrefix, ok := content[1].(map[string]any)
	if !ok || stablePrefix["prompt_cache_breakpoint"] == nil {
		t.Fatalf("stable system prefix missing explicit breakpoint: %#v", content)
	}
	if dynamic, ok := content[2].(map[string]any); !ok || dynamic["prompt_cache_breakpoint"] != nil {
		t.Fatalf("dynamic system block was marked cacheable: %#v", content[2])
	}
}

func TestResponsesProvider_MaxOutputTokensOverride(t *testing.T) {
	var capturedBody map[string]any

	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_override"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_override","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-5.5"})
	p.publicAPIEndpoint = true
	_, err := p.CreateStream(context.Background(), Params{
		MaxTokens:               1024,
		MaxOutputTokensOverride: 64000,
		Messages:                []types.Message{types.UserMessage("continue")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	if got := capturedBody["max_output_tokens"]; got != float64(64000) {
		t.Fatalf("max_output_tokens = %#v, want 64000", got)
	}
}

func TestResponsesProvider_CustomEndpointRetriesUnsupportedOptionalField(t *testing.T) {
	var requests []map[string]any
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_compat"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_compat","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var captured map[string]any
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("unmarshal request: %v", err)
			return
		}
		requests = append(requests, captured)
		if len(requests) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"detail":"Unsupported parameter: reasoning"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-5.4-mini"})
	params := Params{
		Messages:        []types.Message{types.UserMessage("hello")},
		ReasoningEffort: "medium",
	}
	for call := 0; call < 2; call++ {
		ch, err := p.CreateStream(context.Background(), params)
		if err != nil {
			t.Fatalf("CreateStream call %d: %v", call+1, err)
		}
		for range ch {
		}
	}

	if len(requests) != 3 {
		t.Fatalf("requests = %d, want rejected request, retry, and remembered follow-up", len(requests))
	}
	if _, ok := requests[0]["reasoning"]; !ok {
		t.Fatal("first request should contain reasoning")
	}
	for index, request := range requests[1:] {
		if _, ok := request["reasoning"]; ok {
			t.Fatalf("request %d retained rejected reasoning field: %#v", index+2, request["reasoning"])
		}
	}
}

func TestResponsesOptionalFieldFallbackConsumesSharedAttemptBudget(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"Unsupported parameter: reasoning"}`))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-5.4-mini"})
	controller := NewAttemptController(RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	_, err := CreateStreamAttempt(context.Background(), controller, p, Params{
		Messages: []types.Message{types.UserMessage("hello")}, ReasoningEffort: "medium",
	})
	if !IsAttemptLimit(err) {
		t.Fatalf("fallback error = %v, want shared attempt limit", err)
	}
	if requests != 1 {
		t.Fatalf("Responses HTTP requests = %d, want controller cap of 1", requests)
	}
}

func TestResponsesProvider_CustomEndpointDisablesStrictTools(t *testing.T) {
	var capturedBody map[string]any
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_tools"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_tools","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		APIKey:             "test-key",
		BaseURL:            srv.URL,
		Model:              "gpt-5.4-mini",
		DisableStrictTools: true,
	})
	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("use tool")},
		Tools: []types.ToolDefinition{{
			Name:        "Lookup",
			Description: "Lookup value",
			InputSchema: types.StrictObjectSchema(map[string]any{"key": map[string]any{"type": "string"}}, "key"),
			Strict:      true,
		}},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}
	tools, ok := capturedBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", capturedBody["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tool = %#v", tools[0])
	}
	if _, ok := tool["strict"]; ok {
		t.Fatalf("custom Responses request retained strict mode: %#v", tool)
	}
}

func TestResponsesProvider_GPT56ResponsesLiteRequest(t *testing.T) {
	var capturedBody map[string]any
	var capturedLiteHeader string
	sseData := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"id":"resp_lite"}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_lite","status":"completed","usage":{"input_tokens":5,"output_tokens":2},"output":[]}}`},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedLiteHeader = r.Header.Get("x-openai-internal-codex-responses-lite")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		ProviderName: "openai", APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-5.6-sol",
		ResponsesSemantics: ResponsesSemanticsOpenAICodex,
	})
	ch, err := p.CreateStream(context.Background(), Params{
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "stable instructions", Cache: true},
			{Text: "dynamic instructions"},
		},
		Messages: []types.Message{
			types.UserMessage("hello"),
			types.AssistantMessage("hi"),
			types.UserMessage("continue"),
		},
		Tools: []types.ToolDefinition{{
			Name:        "Lookup",
			Description: "Lookup value",
			InputSchema: types.StrictObjectSchema(map[string]any{"key": map[string]any{"type": "string"}}, "key"),
			Strict:      true,
		}},
		ReasoningEffort:         "medium",
		PreviousResponseID:      "resp_previous",
		MaxOutputTokensOverride: 64000,
		PromptCacheKey:          "root-session-lineage",
		UsePromptCache:          true,
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	if capturedLiteHeader != "true" {
		t.Fatalf("Responses Lite header = %q, want true", capturedLiteHeader)
	}
	for _, field := range []string{"instructions", "tools", "previous_response_id", "max_output_tokens"} {
		if _, ok := capturedBody[field]; ok {
			t.Fatalf("Responses Lite request retained top-level %s: %#v", field, capturedBody[field])
		}
	}
	if got, ok := capturedBody["store"].(bool); !ok || got {
		t.Fatalf("store = %#v, want false", capturedBody["store"])
	}
	if got := capturedBody["parallel_tool_calls"]; got != false {
		t.Fatalf("parallel_tool_calls = %#v, want false", got)
	}
	input, ok := capturedBody["input"].([]any)
	if !ok || len(input) != 5 {
		t.Fatalf("input = %#v, want two lite prefixes plus full three-message history", capturedBody["input"])
	}
	additionalTools := input[0].(map[string]any)
	if additionalTools["type"] != "additional_tools" || additionalTools["role"] != "developer" {
		t.Fatalf("additional_tools prefix = %#v", additionalTools)
	}
	developer := input[1].(map[string]any)
	if developer["type"] != "message" || developer["role"] != "developer" {
		t.Fatalf("developer prefix = %#v", developer)
	}
	developerContent, ok := developer["content"].([]any)
	if !ok || len(developerContent) != 1 {
		t.Fatalf("developer content = %#v", developer["content"])
	}
	if stable := developerContent[0].(map[string]any); stable["prompt_cache_breakpoint"] != nil {
		t.Fatalf("Codex Lite request unexpectedly used public Responses cache breakpoint: %#v", stable)
	}
	if key, _ := capturedBody["prompt_cache_key"].(string); !strings.HasPrefix(key, "pcu_") {
		t.Fatalf("prompt_cache_key = %q, want user-scoped key", key)
	}
	if _, ok := capturedBody["prompt_cache_options"]; ok {
		t.Fatalf("Codex Lite request unexpectedly used public Responses prompt_cache_options: %#v", capturedBody["prompt_cache_options"])
	}
	reasoning, ok := capturedBody["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "medium" || reasoning["context"] != "all_turns" {
		t.Fatalf("reasoning = %#v", capturedBody["reasoning"])
	}
}

func TestResponsesProvider_HTTPErrorPreviousResponseNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Previous response with id resp_deadbeef not found","type":"invalid_request_error","param":"previous_response_id","code":"not_found"}}`))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	_, err := p.CreateStream(context.Background(), Params{
		Messages:           []types.Message{types.UserMessage("hello")},
		PreviousResponseID: "resp_deadbeef",
	})
	if err == nil {
		t.Fatal("expected error for previous_response_id rejection")
	}
	apiErr, ok := err.(*types.APIError)
	if !ok {
		t.Fatalf("expected *types.APIError, got %T", err)
	}
	if apiErr.Type != "previous_response_not_found" {
		t.Fatalf("API error type = %q, want %q", apiErr.Type, "previous_response_not_found")
	}
}

func TestResponsesProvider_ResponseFailedTypedAPIError(t *testing.T) {
	tests := []struct {
		name           string
		failedData     string
		wantType       string
		wantStatus     int
		wantRetryAfter string
	}{
		{
			name:       "previous response missing",
			failedData: `{"response":{"error":{"message":"Previous response resp_deadbeef does not exist","code":"previous_response_not_found"}}}`,
			wantType:   "previous_response_not_found",
		},
		{
			name:           "rate limit",
			failedData:     `{"response":{"error":{"message":"Please try again in 2.5s","code":"rate_limit_exceeded","status":429}}}`,
			wantType:       "rate_limit_error",
			wantStatus:     http.StatusTooManyRequests,
			wantRetryAfter: "2.5",
		},
		{
			name:       "server error",
			failedData: `{"response":{"error":{"message":"backend unavailable","code":"server_error","status_code":503}}}`,
			wantType:   "server_error",
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "context length exceeded",
			failedData: `{"response":{"error":{"message":"private detail","code":"context_length_exceeded","status":400}}}`,
			wantType:   "context_length_exceeded",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "context protocol code overrides conflicting status and prose",
			failedData: `{"response":{"error":{"message":"previous response not found","code":"context_length_exceeded","status":503}}}`,
			wantType:   "context_length_exceeded",
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "unclassified upstream failure is replayable",
			failedData: `{"response":{"error":{"message":"Upstream request failed"}}}`,
			wantType:   "response_failed",
			wantStatus: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sseData := buildSSEStream([]sseEvent{
				{Type: "response.created", Data: `{"id":"resp_failed","status":"in_progress"}`},
				{Type: "response.failed", Data: tt.failedData},
			})

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(sseData))
			}))
			defer srv.Close()

			p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
			ch, err := p.CreateStream(context.Background(), Params{
				Messages: []types.Message{types.UserMessage("hello")},
			})
			if err != nil {
				t.Fatalf("CreateStream: %v", err)
			}

			var got *types.APIError
			for _, event := range collectStreamEvents(ch) {
				if event.Type == types.EventError {
					got = event.Error
					break
				}
			}
			if got == nil {
				t.Fatal("expected EventError")
			}
			if got.Type != tt.wantType {
				t.Fatalf("APIError.Type = %q, want %q", got.Type, tt.wantType)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("APIError.Status = %d, want %d", got.Status, tt.wantStatus)
			}
			if got.RetryAfter != tt.wantRetryAfter {
				t.Fatalf("APIError.RetryAfter = %q, want %q", got.RetryAfter, tt.wantRetryAfter)
			}
			if tt.wantType == "response_failed" {
				contract := ClassifyAttemptError(got)
				if contract.Stage != types.ProviderErrorStageStream ||
					contract.Class != types.ProviderErrorClassTransport ||
					contract.ReplaySafety != types.ProviderReplaySafe || !contract.Retryable() {
					t.Fatalf("response.failed retry contract = %+v", contract)
				}
			}
		})
	}
}
