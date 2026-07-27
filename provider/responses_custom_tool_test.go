package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/types"
)

const responsesCustomPatchFixture = "*** Begin Patch\n*** Add File: note.txt\n+hello \\ world\n*** End Patch"

func responsesCustomToolFixture() types.ToolDefinition {
	return types.ToolDefinition{
		Name:        "ApplyPatch",
		Description: "patch workspace",
		InputSchema: types.StrictObjectSchema(map[string]any{"patch": map[string]any{"type": "string"}}, "patch"),
		Type:        types.ToolDefinitionTypeCustom,
		Format: &types.ToolInputFormat{
			Type: "grammar", Syntax: "lark", Definition: "start: PATCH\nPATCH: /.+/s",
		},
	}
}

func TestResponsesCustomToolWireUsesGrammarWithoutJSONParameters(t *testing.T) {
	definition := responsesCustomToolFixture()
	tools := convertToolsToResponsesAPIForSemantics([]types.ToolDefinition{definition}, true, ResponsesSemanticsOpenAIPublic)
	if len(tools) != 1 {
		t.Fatalf("tools = %#v", tools)
	}
	tool := tools[0]
	if tool["type"] != "custom" || tool["name"] != "ApplyPatch" {
		t.Fatalf("custom tool identity = %#v", tool)
	}
	if _, exists := tool["parameters"]; exists {
		t.Fatalf("custom tool leaked JSON parameters: %#v", tool)
	}
	if _, exists := tool["strict"]; exists {
		t.Fatalf("custom tool leaked function strict flag: %#v", tool)
	}
	wantFormat := map[string]any{"type": "grammar", "syntax": "lark", "definition": definition.Format.Definition}
	if !reflect.DeepEqual(tool["format"], wantFormat) {
		t.Fatalf("format = %#v, want %#v", tool["format"], wantFormat)
	}
	if got := responseToolChoiceType([]types.ToolDefinition{definition}, "ApplyPatch"); got != "custom" {
		t.Fatalf("tool choice type = %q", got)
	}
}

func TestResponsesCustomCapabilityUsesExplicitSemanticsNotHostname(t *testing.T) {
	compatibleOnOpenAIHost := NewResponses(Config{
		ProviderName: "openai", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.6-sol",
		ResponsesSemantics: ResponsesSemanticsCompatible,
	})
	publicThroughProxy := NewResponses(Config{
		ProviderName: "openai", BaseURL: "https://benchmark.invalid/v1", Model: "gpt-5.6-sol",
		ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
	})
	if compatibleOnOpenAIHost.Capabilities().CustomTools == CapabilitySupported {
		t.Fatal("hostname overrode explicit compatible semantics")
	}
	if publicThroughProxy.Capabilities().CustomTools != CapabilitySupported {
		t.Fatal("explicit public semantics were downgraded by proxy hostname")
	}
}

func TestResponsesCustomToolPublicRequestIsStandardAndExplicit(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-openai-internal-codex-responses-lite") != "" {
			t.Error("public custom request used private Responses Lite")
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, buildSSEStream([]sseEvent{
			{Type: "response.created", Data: `{"response":{"id":"resp_custom","model":"gpt-5.6-sol"}}`},
			{Type: "response.completed", Data: `{"response":{"id":"resp_custom","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[]}}`},
		}))
	}))
	defer server.Close()

	provider := NewResponses(Config{
		ProviderName: "openai", BaseURL: server.URL, APIKey: "test", Model: "gpt-5.6-sol",
		ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
	})
	stream, err := provider.CreateStream(context.Background(), Params{
		Model: "gpt-5.6-sol", Messages: []types.Message{types.UserMessage("change it")},
		Tools:      []types.ToolDefinition{responsesCustomToolFixture()},
		ToolChoice: &ToolChoice{Type: "tool", Name: "ApplyPatch"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for event := range stream {
		if event.Type == types.EventError {
			t.Fatalf("stream error: %v", event.Error)
		}
	}
	tools, ok := request["tools"].([]any)
	if !ok || len(tools) != 1 || tools[0].(map[string]any)["type"] != "custom" {
		t.Fatalf("top-level custom tools = %#v", request["tools"])
	}
	choice := request["tool_choice"].(map[string]any)
	if choice["type"] != "custom" || choice["name"] != "ApplyPatch" {
		t.Fatalf("tool_choice = %#v", choice)
	}
	input := request["input"].([]any)
	for _, item := range input {
		if object, ok := item.(map[string]any); ok && object["type"] == "additional_tools" {
			t.Fatalf("public request contained private additional_tools: %#v", input)
		}
	}
}

func TestResponsesCustomToolUnsupportedProfilesFailBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	tests := []struct {
		name      string
		semantics ResponsesSemantics
		model     string
	}{
		{name: "generic compatible", semantics: ResponsesSemanticsCompatible, model: "gpt-5.6-sol"},
		{name: "Codex Lite", semantics: ResponsesSemanticsOpenAICodex, model: "gpt-5.6-sol"},
		{name: "unverified public model", semantics: ResponsesSemanticsOpenAIPublic, model: "gpt-5.5"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := NewResponses(Config{
				ProviderName: "openai", BaseURL: server.URL, APIKey: "test", Model: test.model,
				ResponsesSemantics: test.semantics,
			})
			_, err := provider.CreateStream(context.Background(), Params{
				Model: test.model, Tools: []types.ToolDefinition{responsesCustomToolFixture()},
			})
			if err == nil || !strings.Contains(err.Error(), "does not explicitly support Responses custom tools") {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("unsupported custom tools made %d HTTP requests", got)
	}
}

func TestChatCustomToolFailsBeforeHTTPWithoutFunctionFallback(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	chat := NewOpenAI(Config{ProviderName: "openai", BaseURL: server.URL, APIKey: "test", Model: "gpt-5.6-sol"})
	_, err := chat.CreateStream(context.Background(), Params{
		Model: "gpt-5.6-sol", Tools: []types.ToolDefinition{responsesCustomToolFixture()},
	})
	if err == nil || requests.Load() != 0 {
		t.Fatalf("chat fallback error=%v requests=%d", err, requests.Load())
	}
}

func TestResponsesCustomToolSemanticHistoryUsesCustomCallAndOutput(t *testing.T) {
	assistant := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{
		Type: types.ContentTypeToolUse, ID: "call_patch", Name: "ApplyPatch",
		ToolType: types.ToolDefinitionTypeCustom, RawInput: responsesCustomPatchFixture,
		Input: map[string]any{"patch": responsesCustomPatchFixture},
	}}}
	result := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "call_patch", Content: "applied",
		ToolType: types.ToolDefinitionTypeCustom,
	}
	items, err := convertMessagesToResponsesAPIForRequest(Params{
		Model:    "gpt-5.6-sol",
		Messages: []types.Message{assistant, types.ToolResultMessage(result)},
	}, "", ResponsesSemanticsOpenAIPublic, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	call := items[0].(map[string]any)
	if call["type"] != "custom_tool_call" || call["input"] != responsesCustomPatchFixture {
		t.Fatalf("custom replay call = %#v", call)
	}
	output := items[1].(map[string]any)
	if output["type"] != "custom_tool_call_output" || output["call_id"] != "call_patch" {
		t.Fatalf("custom replay output = %#v", output)
	}

	assistant.Content[0] = types.ToolUseBlock{
		Type: types.ContentTypeToolUse, ID: "call_patch", Name: "ApplyPatch",
		ToolType: types.ToolDefinitionTypeCustom, RawInput: responsesCustomPatchFixture,
		Input: map[string]any{"patch": "different"},
	}
	if _, err := convertMessagesToResponsesAPIForRequest(Params{
		Model: "gpt-5.6-sol", Messages: []types.Message{assistant, types.ToolResultMessage(result)},
	}, "", ResponsesSemanticsOpenAIPublic, false); err == nil {
		t.Fatal("mismatched raw and execution input was replayed")
	}
}

func TestResponsesCustomToolStreamPreservesExactFreeformInput(t *testing.T) {
	inputJSON, _ := json.Marshal(responsesCustomPatchFixture)
	callJSON := `{"type":"custom_tool_call","id":"ctc_1","call_id":"call_patch","name":"ApplyPatch","status":"completed","input":` + string(inputJSON) + `}`
	sse := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"response":{"id":"resp_1","model":"gpt-5.6-sol"}}`},
		{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"custom_tool_call","id":"ctc_1","call_id":"call_patch","name":"ApplyPatch","status":"in_progress"}}`},
		{Type: "response.custom_tool_call_input.delta", Data: `{"output_index":0,"delta":"*** Begin Patch\n"}`},
		{Type: "response.custom_tool_call_input.delta", Data: `{"output_index":0,"delta":"*** Add File: note.txt\n+hello \\ world\n*** End Patch"}`},
		{Type: "response.custom_tool_call_input.done", Data: `{"output_index":0,"input":` + string(inputJSON) + `}`},
		{Type: "response.output_item.done", Data: `{"output_index":0,"item":` + callJSON + `}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":10,"output_tokens":8},"output":[` + callJSON + `]}}`},
	})
	events := collectResponsesProtocolEvents(t, sse, ResponsesSemanticsOpenAIPublic)
	var raw strings.Builder
	var final string
	var start *types.ContentDelta
	var stopReason types.StopReason
	for _, event := range events {
		if event.Type == types.EventError {
			t.Fatalf("protocol error: %v", event.Error)
		}
		if event.Type == types.EventContentBlockStart {
			start = event.ContentBlock
		}
		if event.Type == types.EventContentBlockDelta && event.Delta != nil {
			switch event.Delta.Type {
			case "input_text_delta":
				raw.WriteString(event.Delta.PartialText)
			case "tool_state_final":
				final = event.Delta.PartialText
			}
		}
		if event.StopReason != nil {
			stopReason = *event.StopReason
		}
	}
	if start == nil || start.ToolType != types.ToolDefinitionTypeCustom || start.ID != "call_patch" {
		t.Fatalf("custom start = %#v", start)
	}
	if raw.String() != responsesCustomPatchFixture || final != responsesCustomPatchFixture {
		t.Fatalf("raw=%q final=%q", raw.String(), final)
	}
	if stopReason != types.StopReasonToolUse {
		t.Fatalf("stop reason = %q", stopReason)
	}
}

func TestResponsesCustomToolDoneOnlyRecoversAuthoritativeInput(t *testing.T) {
	inputJSON, _ := json.Marshal(responsesCustomPatchFixture)
	callJSON := `{"type":"custom_tool_call","id":"ctc_1","call_id":"call_patch","name":"ApplyPatch","status":"completed","input":` + string(inputJSON) + `}`
	sse := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"response":{"id":"resp_1","model":"gpt-5.6-sol"}}`},
		{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"custom_tool_call","id":"ctc_1","call_id":"call_patch","name":"ApplyPatch","status":"in_progress"}}`},
		{Type: "response.output_item.done", Data: `{"output_index":0,"item":` + callJSON + `}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":10,"output_tokens":8},"output":[` + callJSON + `]}}`},
	})

	events := collectResponsesProtocolEvents(t, sse, ResponsesSemanticsOpenAIPublic)
	var final string
	for _, event := range events {
		if event.Type == types.EventError {
			t.Fatalf("protocol error: %v", event.Error)
		}
		if event.Type == types.EventContentBlockDelta && event.Delta != nil && event.Delta.Type == "tool_state_final" {
			final = event.Delta.PartialText
		}
	}
	if final != responsesCustomPatchFixture {
		t.Fatalf("authoritative final = %q", final)
	}
}

func TestResponsesFunctionDoneWithoutArgumentsDoesNotEraseDeltas(t *testing.T) {
	sse := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"response":{"id":"resp_fn"}}`},
		{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"function_call","call_id":"call_fn","name":"Run"}}`},
		{Type: "response.function_call_arguments.delta", Data: `{"output_index":0,"delta":"{\"command\":\"go test ./...\"}"}`},
		{Type: "response.output_item.done", Data: `{"output_index":0}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_fn","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[{"type":"function_call"}]}}`},
	})
	events := collectResponsesProtocolEvents(t, sse, ResponsesSemanticsCompatible)
	var fragments, finals []string
	for _, event := range events {
		if event.Delta == nil {
			continue
		}
		if event.Delta.Type == "input_json_delta" {
			fragments = append(fragments, event.Delta.PartialJSON)
		}
		if event.Delta.Type == "tool_state_final" {
			finals = append(finals, event.Delta.PartialJSON)
		}
	}
	if strings.Join(fragments, "") != `{"command":"go test ./..."}` || len(finals) != 0 {
		t.Fatalf("fragments=%q finals=%#v", strings.Join(fragments, ""), finals)
	}
}

func TestResponsesCustomToolProtocolFailsClosed(t *testing.T) {
	inputJSON, _ := json.Marshal(responsesCustomPatchFixture)
	differentJSON, _ := json.Marshal(responsesCustomPatchFixture + "\n")
	validCall := `{"type":"custom_tool_call","id":"ctc_1","call_id":"call_patch","name":"ApplyPatch","status":"completed","input":` + string(inputJSON) + `}`
	mismatchedCall := `{"type":"custom_tool_call","id":"ctc_1","call_id":"call_patch","name":"ApplyPatch","status":"completed","input":` + string(differentJSON) + `}`
	added := sseEvent{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"custom_tool_call","id":"ctc_1","call_id":"call_patch","name":"ApplyPatch","status":"in_progress"}}`}
	delta := sseEvent{Type: "response.custom_tool_call_input.delta", Data: `{"output_index":0,"delta":` + string(inputJSON) + `}`}

	tests := []struct {
		name   string
		events []sseEvent
	}{
		{
			name: "delta and authoritative final disagree",
			events: []sseEvent{
				{Type: "response.created", Data: `{"response":{"id":"resp_1"}}`}, added, delta,
				{Type: "response.output_item.done", Data: `{"output_index":0,"item":` + mismatchedCall + `}`},
			},
		},
		{
			name: "completed output has no output item commit",
			events: []sseEvent{
				{Type: "response.created", Data: `{"response":{"id":"resp_1"}}`}, added, delta,
				{Type: "response.completed", Data: `{"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[` + validCall + `]}}`},
			},
		},
		{
			name: "incomplete response cannot commit patch",
			events: []sseEvent{
				{Type: "response.created", Data: `{"response":{"id":"resp_1"}}`}, added, delta,
				{Type: "response.output_item.done", Data: `{"output_index":0,"item":` + validCall + `}`},
				{Type: "response.completed", Data: `{"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":1,"output_tokens":1},"output":[` + validCall + `]}}`},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := collectResponsesProtocolEvents(t, buildSSEStream(test.events), ResponsesSemanticsOpenAIPublic)
			hasError, hasCommit := false, false
			for _, event := range events {
				hasError = hasError || event.Type == types.EventError
				hasCommit = hasCommit || event.Type == types.EventMessageStop
			}
			if !hasError || hasCommit {
				t.Fatalf("events = %#v", events)
			}
		})
	}
}

func TestResponsesCustomToolContinuationValidation(t *testing.T) {
	valid := json.RawMessage(`{"type":"custom_tool_call","id":"ctc_1","call_id":"call_patch","name":"ApplyPatch","status":"completed","input":"*** Begin Patch\\n*** End Patch"}`)
	continuation, err := buildResponsesContinuation(
		[]json.RawMessage{valid}, "gpt-5.6-sol", "gpt-5.6-sol", "completed", ResponsesSemanticsOpenAIPublic, false,
	)
	if err != nil || continuation == nil || len(continuation.Items) != 1 {
		t.Fatalf("continuation=%#v err=%v", continuation, err)
	}
	for _, invalid := range []json.RawMessage{
		json.RawMessage(`{"type":"custom_tool_call","id":"ctc_1","call_id":"call_patch","name":"ApplyPatch","status":"in_progress","input":"patch"}`),
		json.RawMessage(`{"type":"future_tool_call","id":"future_1"}`),
	} {
		if _, err := buildResponsesContinuation(
			[]json.RawMessage{invalid}, "gpt-5.6-sol", "gpt-5.6-sol", "completed", ResponsesSemanticsOpenAIPublic, false,
		); err == nil {
			t.Fatalf("invalid continuation item was accepted: %s", invalid)
		}
	}
}

func collectResponsesProtocolEvents(t *testing.T, sse string, semantics ResponsesSemantics) []types.StreamEvent {
	t.Helper()
	ch := make(chan types.StreamEvent, 64)
	go func() {
		defer close(ch)
		processResponsesStreamForRequest(context.Background(), strings.NewReader(sse), ch, "gpt-5.6-sol", semantics, false)
	}()
	return collectStreamEvents(ch)
}
