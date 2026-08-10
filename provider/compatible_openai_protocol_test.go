package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestUserCompatibleResponsesRequestPreservesToolsAndReasoning(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" {
			t.Errorf("request path = %q, want /v1/responses", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "event: response.completed\ndata: {\"response\":{\"id\":\"resp_compatible\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	// Reproduce auth.json entries written by the old discovery path: the
	// persisted model says Chat even though the built-in model is Responses.
	registry, config := newPersistedLegacyCompatibleProtocolTestRegistry(t, baseURL)
	client, err := registry.Create("custom-gateway", config, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	retry, ok := client.(*RetryProvider)
	if !ok {
		t.Fatalf("provider = %T, want *RetryProvider", client)
	}
	if _, ok := retry.inner.(*openAIProtocolProvider); !ok {
		t.Fatalf("inner provider = %T, want *openAIProtocolProvider", retry.inner)
	}

	stream, err := client.CreateStream(context.Background(), compatibleProtocolTestParams())
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	responseID := ""
	for event := range stream {
		if event.Type == types.EventMessageStop {
			responseID = event.ResponseID
		}
	}
	if responseID != "resp_compatible" {
		t.Fatalf("response ID = %q, want native Responses ID", responseID)
	}

	reasoning, _ := requestBody["reasoning"].(map[string]any)
	if reasoning["effort"] != "xhigh" {
		t.Fatalf("Responses reasoning = %#v, want xhigh effort", requestBody["reasoning"])
	}
	tools, _ := requestBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("Responses tools = %#v, want one tool", requestBody["tools"])
	}
	tool, _ := tools[0].(map[string]any)
	if tool["type"] != "function" || tool["name"] != "Read" || tool["parameters"] == nil {
		t.Fatalf("Responses tool = %#v, want native function shape", tool)
	}
	if _, nested := tool["function"]; nested {
		t.Fatalf("Responses tool used Chat Completions nesting: %#v", tool)
	}
}

func TestUserCompatibleExplicitChatSelectionRemainsAuthoritative(t *testing.T) {
	var paths []string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_explicit\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	registry := newUserCompatibleProtocolTestRegistry(baseURL, "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleOpenAI, APIFormat: "chat-completions", BaseURL: baseURL,
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	retry, ok := client.(*RetryProvider)
	if !ok {
		t.Fatalf("provider = %T, want *RetryProvider", client)
	}
	if _, ok := retry.inner.(*OpenAIProvider); !ok {
		t.Fatalf("inner provider = %T, want *OpenAIProvider", retry.inner)
	}

	stream, err := client.CreateStream(context.Background(), compatibleProtocolTestParams())
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	if len(paths) != 1 || paths[0] != "/v1/chat/completions" {
		t.Fatalf("request paths = %v, want Chat Completions only", paths)
	}
	if requestBody["reasoning_effort"] != "xhigh" {
		t.Fatalf("Chat reasoning_effort = %#v, want xhigh", requestBody["reasoning_effort"])
	}
	tools, _ := requestBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("Chat tools = %#v, want one tool", requestBody["tools"])
	}
	tool, _ := tools[0].(map[string]any)
	if _, nested := tool["function"].(map[string]any); !nested {
		t.Fatalf("Chat tool = %#v, want nested function shape", tool)
	}
}

func TestUserCompatibleResponsesFallbackIsRemembered(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths = append(paths, request.URL.Path)
		mu.Unlock()
		switch request.URL.Path {
		case "/v1/responses":
			http.NotFound(writer, request)
		case "/v1/chat/completions":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_fallback\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		default:
			http.Error(writer, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	registry := newUserCompatibleProtocolTestRegistry(baseURL, "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleOpenAI, BaseURL: baseURL,
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		stream, streamErr := client.CreateStream(context.Background(), compatibleProtocolTestParams())
		if streamErr != nil {
			t.Fatalf("CreateStream attempt %d: %v", attempt+1, streamErr)
		}
		for range stream {
		}
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"/v1/responses", "/v1/chat/completions", "/v1/chat/completions"}
	if len(paths) != len(want) {
		t.Fatalf("request paths = %v, want %v", paths, want)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("request paths = %v, want %v", paths, want)
		}
	}
}

func TestOpenAICustomEndpointGenericResponses400FallsBackToChatAndRemembers(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths = append(paths, request.URL.Path)
		mu.Unlock()
		switch request.URL.Path {
		case "/v1/responses":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `{"error":{"message":"Upstream request failed","type":"api_error"}}`)
		case "/v1/chat/completions":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_fallback\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		default:
			http.Error(writer, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	registerOpenAI(registry)
	client, err := registry.Create("openai", Config{
		APIKey: "test-key", BaseURL: server.URL + "/v1",
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		stream, streamErr := client.CreateStream(context.Background(), compatibleProtocolTestParams())
		if streamErr != nil {
			t.Fatalf("CreateStream attempt %d: %v", attempt+1, streamErr)
		}
		for range stream {
		}
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"/v1/responses", "/v1/chat/completions", "/v1/chat/completions"}
	if len(paths) != len(want) {
		t.Fatalf("request paths = %v, want %v", paths, want)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("request paths = %v, want %v", paths, want)
		}
	}
}

func TestConfirmedToollessResponsesFallsBackWhenLaterToolRequestIsRejected(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	responsesCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths = append(paths, request.URL.Path)
		mu.Unlock()
		switch request.URL.Path {
		case "/v1/responses":
			responsesCalls++
			if responsesCalls == 1 {
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(writer, "event: response.completed\ndata: {\"response\":{\"id\":\"resp_compact\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `{"error":{"message":"Upstream request failed","type":"api_error"}}`)
		case "/v1/chat/completions":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_after_compact\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		default:
			http.Error(writer, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	registerOpenAI(registry)
	client, err := registry.Create("openai", Config{APIKey: "test-key", BaseURL: server.URL + "/v1"}, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}

	// Manual compaction intentionally sends no tool definitions. Its success
	// must not pin the provider to Responses for a later full conversation.
	stream, err := client.CreateStream(context.Background(), Params{
		Model: "gpt-5.6-sol", Messages: []types.Message{types.UserMessage("summarize")},
	})
	if err != nil {
		t.Fatalf("tool-less Responses request: %v", err)
	}
	for range stream {
	}
	for attempt := 0; attempt < 2; attempt++ {
		stream, err = client.CreateStream(context.Background(), compatibleProtocolTestParams())
		if err != nil {
			t.Fatalf("conversation request %d: %v", attempt+1, err)
		}
		for range stream {
		}
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"/v1/responses", "/v1/responses", "/v1/chat/completions", "/v1/chat/completions"}
	if len(paths) != len(want) {
		t.Fatalf("request paths = %v, want %v", paths, want)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("request paths = %v, want %v", paths, want)
		}
	}
}

func TestCompactedSessionConfirmedResponsesFallsBackToRememberedChatWithFullCatalog(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var chatBodies []map[string]any
	responsesCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths = append(paths, request.URL.Path)
		mu.Unlock()
		switch request.URL.Path {
		case "/v1/responses":
			responsesCalls++
			if responsesCalls == 1 {
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(writer, "event: response.completed\ndata: {\"response\":{\"id\":\"resp_compact\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `{"error":{"message":"Upstream request failed","type":"api_error"}}`)
		case "/v1/chat/completions":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode Chat request: %v", err)
			}
			mu.Lock()
			chatBodies = append(chatBodies, body)
			mu.Unlock()
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_after_compact\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		default:
			http.Error(writer, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	registerOpenAI(registry)
	client, err := registry.Create("openai", Config{APIKey: "test-key", BaseURL: server.URL + "/v1"}, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}

	// A real manual compaction summarizes with no client tools. That request
	// can prove only the base Responses envelope, not that the gateway accepts
	// the restored conversation's complete tool catalog.
	stream, err := client.CreateStream(context.Background(), Params{
		Model: "gpt-5.6-sol", Messages: []types.Message{types.UserMessage("summarize prior context")},
	})
	if err != nil {
		t.Fatalf("tool-less compaction request: %v", err)
	}
	for range stream {
	}

	params := compactedFullCatalogProtocolParams()
	for attempt := 0; attempt < 2; attempt++ {
		stream, err = client.CreateStream(context.Background(), params)
		if err != nil {
			t.Fatalf("restored conversation request %d: %v", attempt+1, err)
		}
		for range stream {
		}
	}

	mu.Lock()
	defer mu.Unlock()
	wantPaths := []string{
		"/v1/responses",
		"/v1/responses", "/v1/chat/completions",
		"/v1/chat/completions",
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("request paths = %v, want %v", paths, wantPaths)
	}
	if len(chatBodies) != 2 {
		t.Fatalf("Chat request bodies = %d, want 2", len(chatBodies))
	}
	for index, body := range chatBodies {
		assertCompactedFullCatalogChatBody(t, index+1, body)
	}
}

func compactedFullCatalogProtocolParams() Params {
	capability := messagecontrol.Runtime()
	scope := messagecontrol.NewScope("compacted-session", "provider-protocol-test", 2)
	compactSummary := types.Message{
		ID: "compact-summary", Role: types.RoleUser, IsMeta: true,
		InternalKind: types.InternalMessageKindCompactSummary,
		Content:      []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "earlier context summarized"}},
	}.WithInternalControlProvenance(capability, scope)
	assistantPatch := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.TextBlock{Type: types.ContentTypeText, Text: "applying focused patch"},
		types.ToolUseBlock{
			Type: types.ContentTypeToolUse, ID: "call_patch", Name: "ApplyPatch",
			Input: map[string]any{"patch": "*** Begin Patch\n*** End Patch"},
		},
	}}
	patchResult := types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "call_patch", Content: "patch applied",
		Outcome: types.ToolOutcomeSucceeded,
	})
	assistantRun := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.ToolUseBlock{
			Type: types.ContentTypeToolUse, ID: "call_run", Name: "Run",
			Input: map[string]any{"steps": []any{map[string]any{"id": "tests", "argv": []any{"go", "test", "./..."}}}},
		},
	}}
	runResult := types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "call_run", Content: "tests passed",
		Outcome: types.ToolOutcomeSucceeded,
	})
	skillCatalog := types.Message{
		ID: "skill-catalog", Role: types.RoleDeveloper, IsMeta: true,
		InternalKind: types.InternalMessageKindSkillCatalog,
		DeveloperMetadata: &types.DeveloperMessageMetadata{
			Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 1,
		},
		Content: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: `{"type":"skill_catalog_snapshot","revision":1,"skills":[]}`}},
	}.WithInternalControlProvenance(capability, scope)
	compactReminder := types.Message{
		ID: "compact-reminder", Role: types.RoleUser, IsMeta: true,
		InternalKind: types.InternalMessageKindCompactReminder,
		Content:      []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "restored tool catalog"}},
	}.WithInternalControlProvenance(capability, scope)

	objectSchema := func(properties map[string]any, required ...string) types.JSONSchema {
		return types.JSONSchema{
			Type: "object", Properties: properties, Required: required, AdditionalProperties: false,
		}
	}
	params := Params{
		Model: "gpt-5.6-sol", MaxTokens: 1024, ReasoningEffort: "high",
		System: "system contract",
		Messages: []types.Message{
			compactSummary, assistantPatch, patchResult, assistantRun, runResult,
			skillCatalog, compactReminder, types.UserMessage("report the final result"),
		},
		Tools: []types.ToolDefinition{
			{
				Name: "Inspect", Description: "inspect repository",
				InputSchema: objectSchema(map[string]any{"requests": map[string]any{"type": "array"}}, "requests"), Strict: true,
			},
			{
				Name: "ApplyPatch", Description: "apply patch",
				InputSchema: objectSchema(map[string]any{"patch": map[string]any{"type": "string"}}, "patch"), Strict: true,
			},
			{
				Name: "Run", Description: "run commands",
				InputSchema: objectSchema(map[string]any{"steps": map[string]any{"type": "array"}}, "steps"), Strict: true,
			},
		},
		PromptCacheKey: "compacted-session", UsePromptCache: true,
	}
	return params.WithInternalControlScope(capability, scope)
}

func assertCompactedFullCatalogChatBody(t *testing.T, attempt int, body map[string]any) {
	t.Helper()
	tools, _ := body["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("Chat request %d tools = %#v, want full catalog", attempt, body["tools"])
	}
	var toolNames []string
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		toolNames = append(toolNames, function["name"].(string))
		if _, strictOnWire := function["strict"]; strictOnWire {
			t.Fatalf("Chat request %d leaked strict compatibility extension: %#v", attempt, function)
		}
	}
	if want := []string{"Inspect", "ApplyPatch", "Run"}; !reflect.DeepEqual(toolNames, want) {
		t.Fatalf("Chat request %d tool names = %v, want %v", attempt, toolNames, want)
	}

	messages, _ := body["messages"].([]any)
	var compactSummary, skillCatalogReminder bool
	var callIDs, resultIDs []string
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		content, _ := message["content"].(string)
		compactSummary = compactSummary || strings.Contains(content, "earlier context summarized")
		skillCatalogReminder = skillCatalogReminder || strings.Contains(content, "skill_catalog_snapshot")
		if calls, ok := message["tool_calls"].([]any); ok {
			for _, rawCall := range calls {
				call, _ := rawCall.(map[string]any)
				callIDs = append(callIDs, call["id"].(string))
			}
		}
		if message["role"] == "tool" {
			resultIDs = append(resultIDs, message["tool_call_id"].(string))
		}
	}
	if !compactSummary || !skillCatalogReminder {
		t.Fatalf("Chat request %d lost compact summary/control projection: %#v", attempt, messages)
	}
	if !reflect.DeepEqual(callIDs, []string{"call_patch", "call_run"}) || !reflect.DeepEqual(resultIDs, callIDs) {
		t.Fatalf("Chat request %d tool history calls=%v results=%v", attempt, callIDs, resultIDs)
	}
}

func TestOpenAICustomEndpointGeneric400ProjectsCustomApplyPatchAndContinuesToolResult(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var chatBodies []map[string]any
	chatRequest := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths = append(paths, request.URL.Path)
		mu.Unlock()
		switch request.URL.Path {
		case "/v1/responses":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("X-Request-ID", "req-responses-400")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `{"error":{"message":"Upstream request failed","type":"api_error"}}`)
		case "/v1/chat/completions":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode Chat request: %v", err)
			}
			mu.Lock()
			chatBodies = append(chatBodies, body)
			chatRequest++
			current := chatRequest
			mu.Unlock()
			writer.Header().Set("Content-Type", "text/event-stream")
			if current == 1 {
				arguments, _ := json.Marshal(map[string]any{"patch": responsesCustomPatchFixture})
				chunk, _ := json.Marshal(map[string]any{
					"id": "chatcmpl_patch", "object": "chat.completion.chunk", "created": 1, "model": "gpt-5.6-sol",
					"choices": []any{map[string]any{
						"index": 0, "finish_reason": "tool_calls",
						"delta": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
							"index": 0, "id": "call_patch", "type": "function",
							"function": map[string]any{"name": "ApplyPatch", "arguments": string(arguments)},
						}}},
					}},
				})
				_, _ = io.WriteString(writer, "data: "+string(chunk)+"\n\ndata: [DONE]\n\n")
				return
			}
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_after_tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		default:
			http.Error(writer, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	registerOpenAI(registry)
	client, err := registry.Create("openai", Config{APIKey: "test-key", BaseURL: server.URL + "/v1"}, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	retrying := client.(*RetryProvider)
	negotiating := retrying.inner.(*openAIProtocolProvider)
	if negotiating.Capabilities().CustomTools != CapabilitySupported {
		t.Fatal("negotiating provider did not advertise its custom-to-function projection")
	}

	definition := responsesCustomToolFixture()
	projected := projectCustomToolsForChat(Params{Tools: []types.ToolDefinition{definition}})
	if len(projected.Tools) != 1 || projected.Tools[0].Type != types.ToolDefinitionTypeFunction ||
		projected.Tools[0].Format != nil || !projected.Tools[0].Strict {
		t.Fatalf("custom-to-function projection = %#v", projected.Tools)
	}
	first := Params{
		Model: "gpt-5.6-sol", Messages: []types.Message{types.UserMessage("apply patch")},
		Tools: []types.ToolDefinition{definition}, ToolChoice: &ToolChoice{Type: "tool", Name: "ApplyPatch"},
	}
	stream, err := client.CreateStream(context.Background(), first)
	if err != nil {
		t.Fatalf("first CreateStream: %v", err)
	}
	var toolID, toolName string
	var arguments strings.Builder
	for event := range stream {
		switch event.Type {
		case types.EventContentBlockStart:
			if event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeToolUse {
				toolID, toolName = event.ContentBlock.ID, event.ContentBlock.Name
				if event.ContentBlock.ToolType != types.ToolDefinitionTypeFunction {
					t.Fatalf("fallback call tool type = %q, want function", event.ContentBlock.ToolType)
				}
			}
		case types.EventContentBlockDelta:
			if event.Delta != nil && event.Delta.Type == "input_json_delta" {
				arguments.WriteString(event.Delta.PartialJSON)
			}
		}
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(arguments.String()), &input); err != nil {
		t.Fatalf("fallback tool arguments %q: %v", arguments.String(), err)
	}
	if toolID != "call_patch" || toolName != "ApplyPatch" || input["patch"] != responsesCustomPatchFixture {
		t.Fatalf("fallback tool call = %s/%s %#v", toolID, toolName, input)
	}
	if definition.Type != types.ToolDefinitionTypeCustom || definition.Format == nil {
		t.Fatal("caller-owned custom definition was mutated")
	}
	if negotiating.Capabilities().CustomTools != CapabilitySupported {
		t.Fatal("remembered Chat projection no longer admits custom definitions")
	}

	assistant := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{
		Type: types.ContentTypeToolUse, ID: toolID, Name: toolName, Input: input,
	}}}
	result := types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolID, Content: "applied"}
	second := first
	second.Messages = []types.Message{types.UserMessage("apply patch"), assistant, types.ToolResultMessage(result)}
	stream, err = client.CreateStream(context.Background(), second)
	if err != nil {
		t.Fatalf("second CreateStream: %v", err)
	}
	for range stream {
	}

	mu.Lock()
	defer mu.Unlock()
	wantPaths := []string{"/v1/responses", "/v1/chat/completions", "/v1/chat/completions"}
	if len(paths) != len(wantPaths) {
		t.Fatalf("request paths = %v, want %v", paths, wantPaths)
	}
	for index := range wantPaths {
		if paths[index] != wantPaths[index] {
			t.Fatalf("request paths = %v, want %v", paths, wantPaths)
		}
	}
	if len(chatBodies) != 2 {
		t.Fatalf("Chat request bodies = %d", len(chatBodies))
	}
	tools, _ := chatBodies[0]["tools"].([]any)
	tool, _ := tools[0].(map[string]any)
	function, _ := tool["function"].(map[string]any)
	if tool["type"] != "function" || function["name"] != "ApplyPatch" || function["parameters"] == nil {
		t.Fatalf("projected ApplyPatch schema = %#v", tool)
	}
	if _, strictOnWire := function["strict"]; strictOnWire {
		t.Fatalf("compatibility fallback leaked unsupported strict extension: %#v", function)
	}
	messages, _ := chatBodies[1]["messages"].([]any)
	var sawAssistantCall, sawToolResult bool
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] == "assistant" {
			calls, _ := message["tool_calls"].([]any)
			sawAssistantCall = len(calls) == 1
		}
		if message["role"] == "tool" && message["tool_call_id"] == "call_patch" {
			sawToolResult = true
		}
	}
	if !sawAssistantCall || !sawToolResult {
		t.Fatalf("second Chat history lost tool call/result: %#v", messages)
	}
}

func TestNegotiatedResponsesDoesNotFallbackOnAuthOrThrottle(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			apiErr := &types.APIError{Status: status, Type: "api_error", Message: "gateway failure"}
			if responsesEndpointUnavailable(apiErr) {
				t.Fatalf("status %d incorrectly enabled protocol fallback", status)
			}
		})
	}
}

func TestRememberedChatFailureRetainsAttemptedFormats(t *testing.T) {
	chatCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/responses":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `{"error":{"message":"Upstream request failed","type":"api_error"}}`)
		case "/v1/chat/completions":
			chatCalls++
			if chatCalls == 1 {
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_ok\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("X-Request-ID", "req-chat-failed")
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(writer, `{"error":{"message":"invalid key","type":"invalid_api_key"}}`)
		}
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	registerOpenAI(registry)
	client, err := registry.Create("openai", Config{APIKey: "test-key", BaseURL: server.URL + "/v1"}, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.CreateStream(context.Background(), Params{Messages: []types.Message{types.UserMessage("first")}})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	_, err = client.CreateStream(context.Background(), Params{Messages: []types.Message{types.UserMessage("second")}})
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("remembered Chat error = %v, want APIError", err)
	}
	if len(apiErr.AttemptedAPIFormats) != 2 || apiErr.AttemptedAPIFormats[0] != "responses" ||
		apiErr.AttemptedAPIFormats[1] != "chat-completions" || apiErr.RequestID != "req-chat-failed" {
		t.Fatalf("remembered fallback diagnostics = %+v", apiErr)
	}
}

func TestProtocolFallbackConsumesSharedAttemptBudget(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		http.NotFound(writer, request)
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	registry := newUserCompatibleProtocolTestRegistry(baseURL, "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleOpenAI, BaseURL: baseURL,
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	retrying := client.(*RetryProvider)
	retrying.config = normalizeRetryConfig(RetryConfig{MaxAttempts: 1})
	if _, err = client.CreateStream(context.Background(), compatibleProtocolTestParams()); !IsAttemptLimit(err) {
		t.Fatalf("protocol fallback error = %v, want shared attempt limit", err)
	}
	if len(paths) != 1 || paths[0] != "/v1/responses" {
		t.Fatalf("request paths = %v, want only budgeted Responses attempt", paths)
	}
}

func TestUserCompatibleAnthropicStyleRemainsAuthoritativeOverWireOverride(t *testing.T) {
	registry := newUserCompatibleProtocolTestRegistry("https://gateway.example", "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleAnthropic, APIFormat: "responses", BaseURL: "https://gateway.example",
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	retry, ok := client.(*RetryProvider)
	if !ok {
		t.Fatalf("provider = %T, want *RetryProvider", client)
	}
	if _, ok := retry.inner.(*AnthropicProvider); !ok {
		t.Fatalf("inner provider = %T, want *AnthropicProvider", retry.inner)
	}
}

func TestUserCompatibleExplicitResponsesDoesNotFallback(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		http.NotFound(writer, request)
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	registry := newUserCompatibleProtocolTestRegistry(baseURL, "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleOpenAI, APIFormat: "responses", BaseURL: baseURL,
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	retry, ok := client.(*RetryProvider)
	if !ok {
		t.Fatalf("provider = %T, want *RetryProvider", client)
	}
	if _, ok := retry.inner.(*ResponsesProvider); !ok {
		t.Fatalf("inner provider = %T, want *ResponsesProvider", retry.inner)
	}
	if _, streamErr := client.CreateStream(context.Background(), compatibleProtocolTestParams()); streamErr == nil {
		t.Fatal("explicit Responses request unexpectedly succeeded")
	}
	if len(paths) != 1 || paths[0] != "/v1/responses" {
		t.Fatalf("request paths = %v, want explicit Responses only", paths)
	}
}

func newUserCompatibleProtocolTestRegistry(baseURL, apiFormat string) *ProviderRegistry {
	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	registry.RegisterCompatibleProvider(CompatibleProviderDefinition{
		Name: "custom-gateway", DisplayName: "Custom Gateway", UserDefined: true,
		BaseURLs: map[APIStyle]string{APIStyleOpenAI: baseURL, APIStyleAnthropic: baseURL},
	})
	registry.ReplaceProviderModels("custom-gateway", []ModelInfo{{
		ID: "gpt-5.6-sol", Name: "GPT-5.6 Sol", Provider: "custom-gateway",
		CanReason: true, CanUseTools: true, APIFormat: apiFormat, CostCurrency: "USD",
	}})
	return registry
}

func newPersistedLegacyCompatibleProtocolTestRegistry(t *testing.T, baseURL string) (*ProviderRegistry, Config) {
	t.Helper()
	store, err := NewCredentialStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatalf("create credential store: %v", err)
	}
	if err := store.Set(CredentialEntry{
		Provider: "custom-gateway", AuthMethod: "api_key", APIKey: "test-key",
		BaseURL: baseURL, APIStyle: APIStyleOpenAI, UserDefined: true,
		Models: []ModelInfo{{
			ID: "gpt-5.6-sol", Name: "GPT-5.6 Sol", Provider: "custom-gateway",
			CanReason: true, CanUseTools: true, APIFormat: "chat-completions", CostCurrency: "USD",
		}},
	}); err != nil {
		t.Fatalf("persist legacy credential: %v", err)
	}
	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	registry.SetCredentialStore(store)
	config, err := ResolveCredentialConfig(registry, "custom-gateway")
	if err != nil {
		t.Fatalf("resolve persisted config: %v", err)
	}
	if config.APIFormat != "" {
		t.Fatalf("legacy catalog metadata became explicit wire override: %q", config.APIFormat)
	}
	return registry, config
}

func compatibleProtocolTestParams() Params {
	return Params{
		Messages:        []types.Message{types.UserMessage("hello")},
		ReasoningEffort: "xhigh",
		Tools: []types.ToolDefinition{{
			Name: "Read", Description: "Read a file",
			InputSchema: types.JSONSchema{
				Type: "object",
				Properties: map[string]any{
					"path": map[string]any{"type": "string"},
				},
				Required: []string{"path"},
			},
		}},
	}
}
