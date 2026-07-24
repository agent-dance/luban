package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestResponsesSkillCatalogFullHistoryPreservesDeveloperItems(t *testing.T) {
	messages := responsesSkillCatalogHistory()
	body := captureResponsesSkillCatalogRequest(t, false, Params{Messages: messages})

	input := responsesSkillCatalogInput(t, body)
	assertResponsesSkillCatalogItems(t, input, []map[string]any{
		{"role": "developer", "content": "catalog snapshot revision 1"},
		{"role": "user", "content": "first question"},
		{"role": "assistant", "content": "first answer"},
		{"role": "developer", "content": "catalog delta revision 2"},
		{"role": "user", "content": "second question"},
	})

	for i, item := range input {
		if _, ok := item["developer_metadata"]; ok {
			t.Fatalf("input[%d] leaked persistence metadata: %#v", i, item)
		}
		if _, ok := item["is_meta"]; ok {
			t.Fatalf("input[%d] leaked internal presentation state: %#v", i, item)
		}
	}
}

func TestResponsesPreviousResponseDeveloperSuffixAndEnvelopeStability(t *testing.T) {
	params := Params{
		Model:              "gpt-5",
		MaxTokens:          4096,
		System:             "stable instructions",
		Messages:           responsesSkillCatalogHistory(),
		Tools:              []types.ToolDefinition{{Name: "Skill", Description: "Load a skill", InputSchema: types.JSONSchema{Type: "object"}}},
		PromptCacheKey:     "stable-session",
		UsePromptCache:     true,
		PreviousResponseID: "resp_previous",
		ReasoningEffort:    "high",
	}

	fullParams := params
	fullParams.Messages = params.Messages[:2]
	fullParams.PreviousResponseID = ""
	fullBody := captureResponsesSkillCatalogRequest(t, false, fullParams)
	incrementalBody := captureResponsesSkillCatalogRequest(t, false, params)

	input := responsesSkillCatalogInput(t, incrementalBody)
	assertResponsesSkillCatalogItems(t, input, []map[string]any{
		{"role": "developer", "content": "catalog delta revision 2"},
		{"role": "user", "content": "second question"},
	})
	if got := incrementalBody["previous_response_id"]; got != "resp_previous" {
		t.Fatalf("previous_response_id = %#v, want resp_previous", got)
	}

	for _, key := range []string{
		"model",
		"max_output_tokens",
		"instructions",
		"tools",
		"parallel_tool_calls",
		"prompt_cache_key",
		"reasoning",
	} {
		if !reflect.DeepEqual(incrementalBody[key], fullBody[key]) {
			t.Fatalf("catalog revision changed non-input field %q\nfull: %#v\nincremental: %#v", key, fullBody[key], incrementalBody[key])
		}
	}
}

func TestResponsesSkillCatalogChatGPTFallbackKeepsDeveloperItems(t *testing.T) {
	body := captureResponsesSkillCatalogRequest(t, true, Params{
		Messages:           responsesSkillCatalogHistory(),
		PreviousResponseID: "resp_ignored_for_http_fallback",
	})

	if _, ok := body["previous_response_id"]; ok {
		t.Fatalf("ChatGPT HTTP fallback unexpectedly sent previous_response_id: %#v", body)
	}
	input := responsesSkillCatalogInput(t, body)
	assertResponsesSkillCatalogItems(t, input, []map[string]any{
		{"role": "developer", "content": "catalog snapshot revision 1"},
		{"role": "user", "content": "first question"},
		{"role": "assistant", "content": "first answer"},
		{"role": "developer", "content": "catalog delta revision 2"},
		{"role": "user", "content": "second question"},
	})
}

func responsesSkillCatalogHistory() []types.Message {
	return []types.Message{
		trustedDeveloperMessageForTest("catalog snapshot revision 1", types.DeveloperMessageMetadata{
			Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
			Revision: 1,
		}),
		types.UserMessage("first question"),
		types.AssistantMessage("first answer"),
		trustedDeveloperMessageForTest("catalog delta revision 2", types.DeveloperMessageMetadata{
			Kind:     types.DeveloperMessageKindSkillCatalogDelta,
			Revision: 2,
		}),
		types.UserMessage("second question"),
	}
}

func captureResponsesSkillCatalogRequest(t *testing.T, chatGPTCodexBackend bool, params Params) map[string]any {
	t.Helper()
	params = params.WithInternalControlScope(messagecontrol.Runtime(), providerTestControlScope, false)

	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		} else if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("unmarshal request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.created\ndata: {\"id\":\"resp_catalog\"}\n\nevent: response.completed\ndata: {\"response\":{\"id\":\"resp_catalog\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
	}))
	defer server.Close()

	responses := NewResponses(Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-5"})
	responses.chatGPTCodexBackend = chatGPTCodexBackend
	stream, err := responses.CreateStream(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	if captured == nil {
		t.Fatal("request was not captured")
	}
	return captured
}

func responsesSkillCatalogInput(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["input"].([]any)
	if !ok {
		t.Fatalf("input = %#v, want array", body["input"])
	}
	input := make([]map[string]any, len(raw))
	for i, item := range raw {
		mapped, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("input[%d] = %#v, want object", i, item)
		}
		input[i] = mapped
	}
	return input
}

func assertResponsesSkillCatalogItems(t *testing.T, got, want []map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input items mismatch\n got: %#v\nwant: %#v", got, want)
	}
}
