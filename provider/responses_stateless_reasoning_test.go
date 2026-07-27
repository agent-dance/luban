package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestResponsesStatelessEncryptedReasoningRoundTripPreservesItemOrder(t *testing.T) {
	const encrypted = "opaque-encrypted-reasoning"
	firstStream := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"response":{"id":"resp_1","model":"gpt-5.6-sol"}}`},
		{Type: "response.output_item.added", Data: `{"output_index":0,"item":{"type":"reasoning","id":"rs_1","status":"in_progress","summary":[]}}`},
		{Type: "response.reasoning_summary_text.delta", Data: `{"output_index":0,"delta":"checked constraints"}`},
		{Type: "response.output_item.done", Data: `{"output_index":0,"item":{"type":"reasoning","id":"rs_1","status":"completed","summary":[],"encrypted_content":"` + encrypted + `"}}`},
		{Type: "response.output_item.added", Data: `{"output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Inspect","status":"in_progress"}}`},
		{Type: "response.function_call_arguments.delta", Data: `{"output_index":1,"delta":"{\"path\":\".\"}"}`},
		{Type: "response.output_item.done", Data: `{"output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Inspect","status":"completed","arguments":"{\"path\":\".\"}"}}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":30,"output_tokens":8},"output":[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"` + encrypted + `","status":"completed"},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"Inspect","status":"completed","arguments":"{\"path\":\".\"}"}]}}`},
	})
	secondStream := buildSSEStream([]sseEvent{
		{Type: "response.created", Data: `{"response":{"id":"resp_2","model":"gpt-5.6-sol"}}`},
		{Type: "response.completed", Data: `{"response":{"id":"resp_2","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":40,"output_tokens":3},"output":[]}}`},
	})

	var mu sync.Mutex
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := io.ReadAll(request.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		bodies = append(bodies, body)
		requestNumber := len(bodies)
		mu.Unlock()
		writer.Header().Set("Content-Type", "text/event-stream")
		if requestNumber == 1 {
			_, _ = io.WriteString(writer, firstStream)
			return
		}
		_, _ = io.WriteString(writer, secondStream)
	}))
	defer server.Close()

	responses := NewResponses(Config{
		ProviderName:       "openai",
		ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
		APIKey:             "test-key",
		BaseURL:            server.URL,
		Model:              "gpt-5.6-sol",
	})
	stream, err := responses.CreateStream(context.Background(), Params{
		Messages:        []types.Message{types.UserMessage("inspect")},
		ReasoningEffort: "xhigh",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectStreamEvents(stream)
	var reasoning types.ThinkingBlock
	var continuation *types.ProviderContinuation
	for _, event := range events {
		if event.Type == types.EventContentBlockStart && event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeThinking {
			reasoning = types.ThinkingBlock{
				Type: types.ContentTypeThinking, ProviderItemID: event.ContentBlock.ID,
				ProviderStatus: event.ContentBlock.ProviderStatus,
				Signature:      event.ContentBlock.Signature, SignatureKind: event.ContentBlock.SignatureKind,
				SignatureModel: event.ContentBlock.SignatureModel,
			}
		}
		if event.Type == types.EventContentBlockDelta && event.Delta != nil && event.Delta.Type == "thinking_delta" {
			reasoning.Thinking += event.Delta.Thinking
		}
		if event.Type == types.EventContentBlockDelta && event.Delta != nil && event.Delta.Type == "signature_delta" {
			reasoning.Signature = event.Delta.Signature
			reasoning.SignatureKind = event.Delta.SignatureKind
			reasoning.SignatureModel = event.Delta.SignatureModel
			reasoning.ProviderItemID = event.Delta.ID
			reasoning.ProviderStatus = event.Delta.ProviderStatus
		}
		if event.Type == types.EventMessageStop {
			continuation = event.ProviderContinuation
		}
	}
	if reasoning.Signature != encrypted || reasoning.ProviderItemID != "rs_1" || reasoning.ProviderStatus != "completed" || reasoning.SignatureModel != "gpt-5.6-sol" || reasoning.SignatureKind != types.ThinkingSignatureOpenAIEncryptedReasoning {
		t.Fatalf("reasoning continuation was not reconstructed: %#v", reasoning)
	}

	assistant := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		reasoning,
		types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "call_1", Name: "Inspect", Input: map[string]any{"path": "."}},
	}}
	assistant.AttachProviderContinuation(continuation)
	stream, err = responses.CreateStream(context.Background(), Params{
		Messages: []types.Message{
			types.UserMessage("inspect"), assistant,
			types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "call_1", Content: "result"}),
			types.UserMessage("continue"),
		},
		ReasoningEffort:    "xhigh",
		PreviousResponseID: "resp_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	for index, body := range bodies {
		if stored, ok := body["store"].(bool); !ok || stored {
			t.Fatalf("request %d store = %#v", index, body["store"])
		}
		if !containsString(body["include"], "reasoning.encrypted_content") {
			t.Fatalf("request %d include = %#v", index, body["include"])
		}
		if _, exists := body["previous_response_id"]; exists {
			t.Fatalf("request %d retained previous_response_id", index)
		}
	}
	input, _ := bodies[1]["input"].([]any)
	var ordered []string
	for _, raw := range input {
		item, _ := raw.(map[string]any)
		kind, _ := item["type"].(string)
		if kind == "reasoning" || kind == "function_call" || kind == "function_call_output" {
			ordered = append(ordered, kind)
		}
		if kind == "reasoning" {
			if item["id"] != "rs_1" || item["encrypted_content"] != encrypted || item["status"] != "completed" {
				t.Fatalf("reasoning replay shape = %#v", item)
			}
		}
		if kind == "function_call" {
			if item["id"] != "fc_1" || item["status"] != "completed" || item["arguments"] != `{"path":"."}` {
				t.Fatalf("function-call replay shape = %#v", item)
			}
		}
	}
	if want := []string{"reasoning", "function_call", "function_call_output"}; !reflect.DeepEqual(ordered, want) {
		t.Fatalf("replay order = %v, want %v; input=%#v", ordered, want, input)
	}
}

func TestResponsesSemanticsProfileMakesDirectAndProxyBodiesIdentical(t *testing.T) {
	request := Params{
		Messages:           []types.Message{types.UserMessage("hello")},
		ReasoningEffort:    "xhigh",
		PreviousResponseID: "must-not-be-used",
	}
	capture := func(baseURL string) (map[string]any, string) {
		t.Helper()
		var body map[string]any
		var liteHeader string
		responses := NewResponses(Config{
			ProviderName: "openai", ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
			APIKey: "same-key", BaseURL: baseURL, Model: "gpt-5.6-sol",
		})
		responses.client = &http.Client{Transport: responseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			raw, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(raw, &body)
			liteHeader = req.Header.Get("x-openai-internal-codex-responses-lite")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(buildSSEStream([]sseEvent{
					{Type: "response.created", Data: `{"response":{"id":"resp","model":"gpt-5.6-sol"}}`},
					{Type: "response.completed", Data: `{"response":{"id":"resp","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[]}}`},
				}))),
			}, nil
		})}
		stream, err := responses.CreateStream(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		for range stream {
		}
		return body, liteHeader
	}
	direct, directLite := capture("https://api.openai.com/v1")
	proxied, proxyLite := capture("https://content-blind-proxy.invalid/openai/v1")
	if !reflect.DeepEqual(direct, proxied) {
		directJSON, _ := json.Marshal(direct)
		proxyJSON, _ := json.Marshal(proxied)
		t.Fatalf("URL changed canonical body\ndirect=%s\nproxy=%s", directJSON, proxyJSON)
	}
	if directLite != "" || proxyLite != "" {
		t.Fatalf("public Responses unexpectedly used Lite headers direct=%q proxy=%q", directLite, proxyLite)
	}
}

func TestResponsesEncryptedReasoningProfileMatrix(t *testing.T) {
	tests := []struct {
		name        string
		semantics   ResponsesSemantics
		model       string
		wantInclude bool
		wantContext bool
		wantLite    bool
	}{
		{name: "public", semantics: ResponsesSemanticsOpenAIPublic, model: "gpt-5.5", wantInclude: true, wantContext: true},
		{name: "public gpt-5.6 standard responses", semantics: ResponsesSemanticsOpenAIPublic, model: "gpt-5.6-sol", wantInclude: true, wantContext: true},
		{name: "codex", semantics: ResponsesSemanticsOpenAICodex, model: "gpt-5.5", wantInclude: true, wantContext: true},
		{name: "codex responses lite", semantics: ResponsesSemanticsOpenAICodex, model: "gpt-5.6-sol", wantInclude: true, wantContext: true, wantLite: true},
		{name: "generic compatible", semantics: ResponsesSemanticsCompatible, model: "gpt-5.5"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body map[string]any
			var lite string
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				raw, _ := io.ReadAll(request.Body)
				_ = json.Unmarshal(raw, &body)
				lite = request.Header.Get("x-openai-internal-codex-responses-lite")
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(writer, buildSSEStream([]sseEvent{
					{Type: "response.created", Data: `{"response":{"id":"resp"}}`},
					{Type: "response.completed", Data: `{"response":{"id":"resp","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[]}}`},
				}))
			}))
			defer server.Close()
			responses := NewResponses(Config{ProviderName: "openai", ResponsesSemantics: test.semantics, APIKey: "key", BaseURL: server.URL, Model: test.model})
			stream, err := responses.CreateStream(context.Background(), Params{
				Messages: []types.Message{types.UserMessage("hello")}, ReasoningEffort: "xhigh",
			})
			if err != nil {
				t.Fatal(err)
			}
			for range stream {
			}
			if got := containsString(body["include"], "reasoning.encrypted_content"); got != test.wantInclude {
				t.Fatalf("include=%#v, wantEncrypted=%v", body["include"], test.wantInclude)
			}
			reasoning, _ := body["reasoning"].(map[string]any)
			_, hasContext := reasoning["context"]
			if hasContext != test.wantContext {
				t.Fatalf("reasoning=%#v, wantContext=%v", reasoning, test.wantContext)
			}
			if (lite == "true") != test.wantLite {
				t.Fatalf("lite header=%q, want=%v", lite, test.wantLite)
			}
		})
	}
}

func TestEncryptedReasoningNeverEntersProviderDebugSnapshots(t *testing.T) {
	const secret = "encrypted-debug-secret"
	params := Params{Messages: []types.Message{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ThinkingBlock{
					Type: types.ContentTypeThinking, Thinking: "safe summary", Signature: secret,
					SignatureKind: types.ThinkingSignatureOpenAIEncryptedReasoning, SignatureModel: "gpt-5.6-sol",
					ProviderItemID: "rs-secret", ProviderStatus: "completed",
				},
			},
		},
	}}
	requestJSON, err := json.Marshal(newDebugRequest(params, "gpt-5.6-sol"))
	if err != nil {
		t.Fatal(err)
	}
	responseJSON, err := json.Marshal(newDebugResponse([]types.StreamEvent{{
		Type: types.EventContentBlockStart, Index: 0,
		ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeThinking, Signature: secret,
			SignatureKind:  types.ThinkingSignatureOpenAIEncryptedReasoning,
			SignatureModel: "gpt-5.6-sol", ID: "rs-secret",
		},
	}}, ""))
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"request": requestJSON, "response": responseJSON} {
		if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte("rs-secret")) || bytes.Contains(raw, []byte("openai_encrypted_reasoning")) {
			t.Fatalf("%s debug snapshot exposed provider continuation state: %s", name, raw)
		}
	}
}

func TestEncryptedReasoningReplayRejectsDifferentModel(t *testing.T) {
	message := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.ThinkingBlock{
			Type: types.ContentTypeThinking, Signature: "model-bound-cipher",
			SignatureKind:  types.ThinkingSignatureOpenAIEncryptedReasoning,
			SignatureModel: "gpt-5.6-sol", ProviderItemID: "rs_1",
		},
		types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "call_1", Name: "Inspect", Input: map[string]any{}},
	}}
	items := convertAssistantMessageToResponsesAPIForModel(message, "gpt-5.6-terra")
	if len(items) != 1 || items[0].(map[string]any)["type"] != "function_call" {
		t.Fatalf("different model replayed encrypted reasoning: %#v", items)
	}
}

func containsString(value any, expected string) bool {
	items, _ := value.([]any)
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

type responseRoundTripFunc func(*http.Request) (*http.Response, error)

func (function responseRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
