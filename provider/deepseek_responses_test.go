package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestDeepSeekFlashFactoryUsesNativeResponsesAPI(t *testing.T) {
	var path string
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, buildSSEStream([]sseEvent{
			{Type: "response.created", Data: `{"response":{"id":"resp_ds","model":"deepseek-v4-flash","status":"in_progress"}}`},
			{Type: "response.completed", Data: `{"response":{"id":"resp_ds","model":"deepseek-v4-flash","status":"completed","usage":{"input_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens":1,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":2},"output":[]}}`},
		}))
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registerBuiltinProviders(registry)
	created, err := registry.Create("deepseek", Config{
		APIKey:    "deepseek-test-key",
		BaseURL:   server.URL,
		APIFormat: "responses",
	}, "deepseek-v4-flash")
	if err != nil {
		t.Fatal(err)
	}
	retry, ok := created.(*RetryProvider)
	if !ok {
		t.Fatalf("DeepSeek provider = %T, want *RetryProvider", created)
	}
	responses, ok := retry.inner.(*ResponsesProvider)
	if !ok {
		t.Fatalf("DeepSeek inner provider = %T, want *ResponsesProvider", retry.inner)
	}
	if caps := responses.Capabilities(); !caps.Thinking || caps.Vision || caps.CustomTools != CapabilitySupported || caps.CacheRouting != CacheRoutingDeepSeekUserID {
		t.Fatalf("DeepSeek Responses capabilities = %#v", caps)
	}

	stream, err := created.CreateStream(context.Background(), Params{
		Model:           "deepseek-v4-flash",
		Messages:        []types.Message{types.UserMessage("change it")},
		Tools:           []types.ToolDefinition{responsesCustomToolFixture()},
		ToolChoice:      &ToolChoice{Type: "tool", Name: "ApplyPatch"},
		MaxTokens:       321,
		ReasoningEffort: "high",
		PromptCacheKey:  "session-lineage",
		UsePromptCache:  true,
		Truncation:      "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	for event := range stream {
		if event.Type == types.EventError {
			t.Fatalf("stream error: %#v", event.Error)
		}
	}

	if path != "/responses" {
		t.Fatalf("path = %q, want /responses", path)
	}
	if request["model"] != "deepseek-v4-flash" || request["max_output_tokens"] != float64(321) {
		t.Fatalf("model/output limit = %#v/%#v", request["model"], request["max_output_tokens"])
	}
	reasoning, _ := request["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" {
		t.Fatalf("reasoning = %#v, want high effort", request["reasoning"])
	}
	text, _ := request["text"].(map[string]any)
	if text["verbosity"] != "low" {
		t.Fatalf("text = %#v, want low verbosity", request["text"])
	}
	tools, _ := request["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "apply_patch" {
		t.Fatalf("DeepSeek custom tools = %#v", request["tools"])
	}
	choice, _ := request["tool_choice"].(map[string]any)
	if choice["type"] != "custom" || choice["name"] != "apply_patch" {
		t.Fatalf("DeepSeek tool choice = %#v", request["tool_choice"])
	}
	if request["user"] == "" {
		t.Fatalf("DeepSeek request omitted credential-scoped user: %#v", request)
	}
	for _, unsupported := range []string{"include", "store", "service_tier", "previous_response_id", "prompt_cache_key", "truncation", "output_config"} {
		if value, exists := request[unsupported]; exists {
			t.Fatalf("DeepSeek request sent unsupported %s=%#v", unsupported, value)
		}
	}
}

func TestDeepSeekFactoryKeepsProAndExplicitFlashOverrideOnChatCompletions(t *testing.T) {
	registry := NewProviderRegistry()
	registerBuiltinProviders(registry)
	for _, test := range []struct {
		name   string
		model  string
		config Config
	}{
		{name: "Pro catalog protocol", model: "deepseek-v4-pro", config: Config{APIKey: "key"}},
		{name: "Flash explicit override", model: "deepseek-v4-flash", config: Config{APIKey: "key", APIFormat: "chat-completions"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			created, err := registry.Create("deepseek", test.config, test.model)
			if err != nil {
				t.Fatal(err)
			}
			retry := created.(*RetryProvider)
			if _, ok := retry.inner.(*OpenAIProvider); !ok {
				t.Fatalf("DeepSeek %s inner provider = %T, want *OpenAIProvider", test.model, retry.inner)
			}
		})
	}
}

func TestDeepSeekFactoryAppliesHarnessDefaultOutputBudgetAndPreservesOverride(t *testing.T) {
	registry := NewProviderRegistry()
	registerBuiltinProviders(registry)
	for _, test := range []struct {
		name       string
		model      string
		configured int
		want       int
	}{
		{name: "Flash Responses default", model: "deepseek-v4-flash", want: 256000},
		{name: "Pro Chat default", model: "deepseek-v4-pro", want: 256000},
		{name: "explicit configuration wins", model: "deepseek-v4-flash", configured: 32000, want: 32000},
	} {
		t.Run(test.name, func(t *testing.T) {
			created, err := registry.Create("deepseek", Config{APIKey: "key", MaxTokens: test.configured}, test.model)
			if err != nil {
				t.Fatal(err)
			}
			retry := created.(*RetryProvider)
			switch inner := retry.inner.(type) {
			case *ResponsesProvider:
				if inner.maxTokens != test.want {
					t.Fatalf("Responses maxTokens = %d, want %d", inner.maxTokens, test.want)
				}
			case *OpenAIProvider:
				if inner.maxTokens != test.want {
					t.Fatalf("Chat maxTokens = %d, want %d", inner.maxTokens, test.want)
				}
			default:
				t.Fatalf("DeepSeek inner provider = %T", retry.inner)
			}
		})
	}
}

func TestDeepSeekResponsesReplaysPlainReasoningAndCustomToolIdentity(t *testing.T) {
	provider := NewResponses(Config{
		ProviderName:       "deepseek",
		ResponsesSemantics: ResponsesSemanticsDeepSeek,
		Model:              "deepseek-v4-flash",
	})
	params := Params{
		Model: "deepseek-v4-flash",
		Messages: []types.Message{
			{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{
					types.ThinkingBlock{Type: types.ContentTypeThinking, Thinking: "check the repository"},
					types.ToolUseBlock{
						Type: types.ContentTypeToolUse, ID: "call_patch", Name: "ApplyPatch",
						ToolType: types.ToolDefinitionTypeCustom, RawInput: responsesCustomPatchFixture,
						Input: map[string]any{"patch": responsesCustomPatchFixture},
					},
				},
			},
			{
				Role: types.RoleUser,
				Content: []types.ContentBlock{types.ToolResultBlock{
					Type: types.ContentTypeToolResult, ToolUseID: "call_patch",
					ToolType: types.ToolDefinitionTypeCustom, Content: "done",
				}},
			},
		},
	}
	body, _, _, err := provider.buildResponsesRequestBody(params, provider.snapshotRequestProfile(), "", responsesTransportHTTP)
	if err != nil {
		t.Fatal(err)
	}
	input, _ := body["input"].([]any)
	if len(input) != 3 {
		t.Fatalf("input = %#v, want reasoning, custom call, and output", input)
	}
	reasoning := input[0].(map[string]any)
	content := reasoning["content"].([]map[string]string)
	if reasoning["type"] != "reasoning" || len(content) != 1 || content[0]["type"] != "reasoning_text" || content[0]["text"] != "check the repository" {
		t.Fatalf("reasoning replay = %#v", reasoning)
	}
	call := input[1].(map[string]any)
	if call["type"] != "custom_tool_call" || call["name"] != "apply_patch" {
		t.Fatalf("custom call replay = %#v", call)
	}
	output := input[2].(map[string]any)
	if output["type"] != "custom_tool_call_output" || output["call_id"] != "call_patch" {
		t.Fatalf("custom output replay = %#v", output)
	}
}

func TestDeepSeekResponsesUsesAuthoritativeFinalArgumentsAndReasoningStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, buildSSEStream([]sseEvent{
			{Type: "response.created", Data: `{"response":{"id":"resp_ds_final","model":"deepseek-v4-flash","status":"in_progress"}}`},
			{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"reasoning","id":"rs_ds","status":"in_progress"}}`},
			{Type: "response.reasoning_text.delta", Data: `{"output_index":0,"delta":"check the path"}`},
			{Type: "response.output_item.done", Data: `{"output_index":0,"item":{"type":"reasoning","id":"rs_ds","status":"completed"}}`},
			{Type: "response.output_item.added", Data: `{"output_index":1,"item":{"type":"function_call","id":"fc_ds","call_id":"call_ds","name":"Inspect","status":"in_progress"}}`},
			{Type: "response.function_call_arguments.delta", Data: `{"output_index":1,"delta":"{\"path\":"}`},
			{Type: "response.function_call_arguments.done", Data: `{"output_index":1,"item_id":"fc_ds","arguments":"{\"path\":\".\"}"}`},
			{Type: "response.output_item.done", Data: `{"output_index":1,"item":{"type":"function_call","id":"fc_ds","call_id":"call_ds","name":"Inspect","status":"completed","arguments":"{\"path\":\".\"}"}}`},
			{Type: "response.completed", Data: `{"response":{"id":"resp_ds_final","model":"deepseek-v4-flash","status":"completed","usage":{"input_tokens":10,"output_tokens":4},"output":[{"type":"reasoning"},{"type":"function_call","id":"fc_ds","call_id":"call_ds","name":"Inspect","status":"completed","arguments":"{\"path\":\".\"}"}]}}`},
		}))
	}))
	defer server.Close()

	responses := NewResponses(Config{
		ProviderName: "deepseek", ResponsesSemantics: ResponsesSemanticsDeepSeek,
		APIKey: "test-key", BaseURL: server.URL, Model: "deepseek-v4-flash",
	})
	stream, err := responses.CreateStream(context.Background(), Params{
		Model: "deepseek-v4-flash", Messages: []types.Message{types.UserMessage("inspect")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var finalArguments string
	var reasoningStatus string
	for _, event := range collectStreamEvents(stream) {
		if event.Type == types.EventError {
			t.Fatalf("stream error: %#v", event.Error)
		}
		if event.Type == types.EventContentBlockDelta && event.Delta != nil {
			switch event.Delta.Type {
			case "tool_state_final":
				finalArguments = event.Delta.PartialJSON
			case "thinking_state_final":
				reasoningStatus = event.Delta.ProviderStatus
			}
		}
	}
	if finalArguments != `{"path":"."}` {
		t.Fatalf("authoritative function arguments = %q", finalArguments)
	}
	if reasoningStatus != "completed" {
		t.Fatalf("reasoning status = %q, want completed", reasoningStatus)
	}
}

func TestDeepSeekResponsesDoesNotReplayReasoningOutsideToolTurn(t *testing.T) {
	items := convertAssistantMessageToResponsesAPIForSemantics(types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.ThinkingBlock{Type: types.ContentTypeThinking, Thinking: "private raw reasoning", Kind: types.ThinkingKindRaw},
			types.TextBlock{Type: types.ContentTypeText, Text: "final answer"},
		},
	}, "deepseek-v4-flash", ResponsesSemanticsDeepSeek)
	if len(items) != 1 {
		t.Fatalf("non-tool assistant replay = %#v, want only final text", items)
	}
	item := items[0].(map[string]any)
	if item["role"] != "assistant" || item["content"] != "final answer" {
		t.Fatalf("non-tool assistant replay = %#v", item)
	}
}

func TestDeepSeekResponsesIncompleteIsTerminalMaxTokens(t *testing.T) {
	sse := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"response":{"id":"resp_incomplete","model":"deepseek-v4-flash","status":"in_progress"}}`},
		{Type: "response.incomplete", Data: `{"response":{"id":"resp_incomplete","model":"deepseek-v4-flash","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":3,"output_tokens":5},"output":[]}}`},
	})
	events := collectResponsesProtocolEvents(t, sse, ResponsesSemanticsDeepSeek)
	var failed, stopped, maxTokens, toolReceipt bool
	for _, event := range events {
		if event.Type == types.EventError {
			failed = true
		}
		if event.Type == types.EventMessageDelta && event.StopReason != nil && *event.StopReason == types.StopReasonMaxTokens {
			maxTokens = true
		}
		if event.Type == types.EventMessageStop {
			stopped = true
			toolReceipt = event.ProviderCommitReceipt != nil
		}
	}
	if failed || !stopped || !maxTokens || toolReceipt {
		t.Fatalf("incomplete events = %#v", events)
	}
}

func TestDeepSeekResponsesUnknownIncompleteReasonIsProtocolError(t *testing.T) {
	sse := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"response":{"id":"resp_incomplete","model":"deepseek-v4-flash","status":"in_progress"}}`},
		{Type: "response.incomplete", Data: `{"response":{"id":"resp_incomplete","model":"deepseek-v4-flash","status":"incomplete","incomplete_details":{"reason":"future_reason"},"usage":{"input_tokens":3,"output_tokens":5},"output":[]}}`},
	})
	events := collectResponsesProtocolEvents(t, sse, ResponsesSemanticsDeepSeek)
	var failure *types.APIError
	var stopped, maxTokens bool
	for _, event := range events {
		if event.Type == types.EventError {
			failure = event.Error
		}
		stopped = stopped || event.Type == types.EventMessageStop
		maxTokens = maxTokens || event.Type == types.EventMessageDelta && event.StopReason != nil && *event.StopReason == types.StopReasonMaxTokens
	}
	if failure == nil || failure.Type != "response_incomplete" || failure.FailureDiagnostic == nil {
		t.Fatalf("unknown incomplete reason events = %#v", events)
	}
	if failure.FailureDiagnostic.FailurePoint != types.ProviderFailureResponseIncomplete || failure.FailureDiagnostic.IncompleteReason != "unknown" || stopped || maxTokens {
		t.Fatalf("unknown incomplete reason was misclassified: %#v", events)
	}
}
