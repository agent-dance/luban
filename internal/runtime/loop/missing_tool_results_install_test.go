package loop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestInstallVisibleHistoryRepairsOrphanedCustomToolBeforeLaterMessages(t *testing.T) {
	const patch = "*** Begin Patch\n*** Add File: repaired.txt\n+repaired\n*** End Patch"
	history := []types.Message{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{types.ToolUseBlock{
				Type: types.ContentTypeToolUse, ID: "call_patch", Name: "ApplyPatch",
				ToolType: types.ToolDefinitionTypeCustom, RawInput: patch,
				Input: map[string]any{"patch": patch},
			}},
		},
		{Role: types.RoleDeveloper, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "catalog"},
		}},
		types.UserMessage("continue"),
	}

	bodyCh := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		bodyCh <- body
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "event: response.created\ndata: {\"id\":\"resp_repaired\",\"model\":\"gpt-5.6-sol\"}\n\nevent: response.completed\ndata: {\"response\":{\"id\":\"resp_repaired\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
	}))
	defer server.Close()

	responses := provider.NewResponses(provider.Config{
		ProviderName: "openai", ResponsesSemantics: provider.ResponsesSemanticsOpenAIPublic,
		APIKey: "test-key", BaseURL: server.URL, Model: "gpt-5.6-sol",
	})
	query := New(responses, registry.New(), Config{Model: "gpt-5.6-sol"})
	query.SetMessages(history)
	repaired := query.Messages()
	if len(repaired) != 4 {
		t.Fatalf("repaired messages = %d, want 4: %#v", len(repaired), repaired)
	}
	result, ok := repaired[1].Content[0].(types.ToolResultBlock)
	if !ok {
		t.Fatalf("inserted block = %T, want ToolResultBlock", repaired[1].Content[0])
	}
	if result.ToolUseID != "call_patch" || result.ToolType != types.ToolDefinitionTypeCustom ||
		!result.IsError || result.Outcome != types.ToolOutcomeCancelled || result.Content == "" {
		t.Fatalf("inserted result = %#v", result)
	}
	if repaired[2].Role != types.RoleDeveloper || repaired[3].GetText() != "continue" {
		t.Fatalf("repair crossed later messages: %#v", repaired)
	}

	stream, err := responses.CreateStream(context.Background(), provider.Params{
		Model: "gpt-5.6-sol", Messages: repaired,
	})
	if err != nil {
		t.Fatalf("Responses continuation rejected repaired history: %v", err)
	}
	for range stream {
	}
	body := <-bodyCh
	input, ok := body["input"].([]any)
	if !ok || len(input) != 4 {
		t.Fatalf("Responses input = %#v", body["input"])
	}
	call, _ := input[0].(map[string]any)
	output, _ := input[1].(map[string]any)
	if call["type"] != "custom_tool_call" || call["call_id"] != "call_patch" {
		t.Fatalf("Responses call = %#v", call)
	}
	if output["type"] != "custom_tool_call_output" || output["call_id"] != "call_patch" || output["output"] == "" {
		t.Fatalf("Responses output = %#v", output)
	}
}

func TestRepairInstalledMissingToolResultsCompletesOnlyMissingParallelSibling(t *testing.T) {
	history := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "call_a", Name: "Inspect", Input: map[string]any{}},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "call_b", Name: "Inspect", Input: map[string]any{}},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "call_c", Name: "Inspect", Input: map[string]any{}},
		}},
		types.ToolResultMessage(
			types.ToolResultBlock{ToolUseID: "call_a", Content: "a", Outcome: types.ToolOutcomeSucceeded},
			types.ToolResultBlock{ToolUseID: "call_c", Content: "c", Outcome: types.ToolOutcomeSucceeded},
		),
		types.UserMessage("next"),
	}

	repaired := repairInstalledMissingToolResults(history)
	if len(repaired) != 4 {
		t.Fatalf("repaired messages = %d, want 4: %#v", len(repaired), repaired)
	}
	if got := repaired[1].Content; len(got) != 2 {
		t.Fatalf("existing parallel result batch changed: %#v", got)
	}
	inserted, ok := repaired[2].Content[0].(types.ToolResultBlock)
	if !ok || inserted.ToolUseID != "call_b" || !inserted.IsError || inserted.Outcome != types.ToolOutcomeCancelled {
		t.Fatalf("inserted parallel result = %#v", repaired[2].Content)
	}
	if repaired[3].GetText() != "next" {
		t.Fatalf("ordinary user message moved: %#v", repaired)
	}

	again := repairInstalledMissingToolResults(repaired)
	if !reflect.DeepEqual(again, repaired) {
		t.Fatalf("repair is not idempotent\nfirst:  %#v\nsecond: %#v", repaired, again)
	}
}

func TestRepairInstalledMissingToolResultsLeavesValidHistoryUnchanged(t *testing.T) {
	history := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "call_a", Name: "Inspect", Input: map[string]any{}},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "call_b", Name: "Inspect", Input: map[string]any{}},
		}},
		types.ToolResultMessage(
			types.ToolResultBlock{ToolUseID: "call_a", Content: "a", Outcome: types.ToolOutcomeSucceeded},
			types.ToolResultBlock{ToolUseID: "call_b", Content: "b", Outcome: types.ToolOutcomeSucceeded},
		),
		types.UserMessage("next"),
	}

	repaired := repairInstalledMissingToolResults(history)
	if !reflect.DeepEqual(repaired, history) {
		t.Fatalf("valid history changed\nwant: %#v\ngot:  %#v", history, repaired)
	}
}

func TestRepairInstalledMissingToolResultsMovesLateResultBeforeOrdinaryMessage(t *testing.T) {
	history := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "call_late", Name: "Inspect", Input: map[string]any{}},
		}},
		{Role: types.RoleDeveloper, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "catalog"},
		}},
		types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "call_late", Content: "persisted result", Outcome: types.ToolOutcomeFailed, IsError: true,
		}),
		types.UserMessage("continue"),
	}

	repaired := repairInstalledMissingToolResults(history)
	if len(repaired) != 4 {
		t.Fatalf("repaired messages = %d, want 4: %#v", len(repaired), repaired)
	}
	result, ok := repaired[1].Content[0].(types.ToolResultBlock)
	if !ok || result.ToolUseID != "call_late" || result.Content != "persisted result" || result.Outcome != types.ToolOutcomeFailed {
		t.Fatalf("moved result = %#v", repaired[1].Content)
	}
	if repaired[2].Role != types.RoleDeveloper || repaired[3].GetText() != "continue" {
		t.Fatalf("late result was not moved before ordinary messages: %#v", repaired)
	}
	if again := repairInstalledMissingToolResults(repaired); !reflect.DeepEqual(again, repaired) {
		t.Fatalf("moved-result repair is not idempotent\nfirst:  %#v\nsecond: %#v", repaired, again)
	}
}
