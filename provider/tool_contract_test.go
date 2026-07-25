package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type providerContractTool struct{}

func (providerContractTool) Name() string        { return "ProviderContract" }
func (providerContractTool) Description() string { return "provider contract tool" }
func (providerContractTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}
func (providerContractTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}
func (providerContractTool) MapToolResultToToolResultBlock(_ any, toolUseID string) types.ToolResultBlock {
	return types.ToolResultBlock{ToolUseID: toolUseID, Content: "model-visible"}
}

func TestStrictToolDefinitionPropagatesAcrossProviders(t *testing.T) {
	defs := []types.ToolDefinition{{
		Name:        "StrictTool",
		Description: "strict tool",
		InputSchema: types.StrictObjectSchema(map[string]any{}, ""),
		Strict:      true,
	}}

	anthropicTools, err := convertToAnthropicTools(defs)
	if err != nil {
		t.Fatal(err)
	}
	if len(anthropicTools) != 1 || anthropicTools[0].GetStrict() == nil || !*anthropicTools[0].GetStrict() {
		t.Fatalf("Anthropic strict flag missing: %#v", anthropicTools)
	}

	responsesTools := convertToolsToResponsesAPIWithStrictMode(defs, true)
	if got, ok := responsesTools[0]["strict"].(bool); !ok || !got {
		t.Fatalf("Responses strict flag = %#v", responsesTools[0]["strict"])
	}

	openAITools := convertToolsToOpenAIWithStrictMode(defs, true)
	if len(openAITools) != 1 || openAITools[0].Function == nil || !openAITools[0].Function.Strict {
		t.Fatalf("OpenAI strict flag missing: %#v", openAITools)
	}

	compatibleTools := convertToolsToOpenAIWithStrictMode(defs, false)
	if len(compatibleTools) != 1 || compatibleTools[0].Function == nil || compatibleTools[0].Function.Strict {
		t.Fatalf("OpenAI-compatible strict flag was not disabled: %#v", compatibleTools)
	}
}

func TestMappedToolResultDataDoesNotLeakAcrossProviders(t *testing.T) {
	block := types.MapToolResult(providerContractTool{}, types.ToolResult{
		Data: map[string]any{"internal_secret": "do-not-leak"},
	}, "toolu_contract")
	message := types.ToolResultMessage(block)

	anthropicMessages := convertToAnthropicMessagesForParams(Params{Messages: []types.Message{message}})
	assertProviderResultSeparation(t, "Anthropic", anthropicMessages)

	responsesInput := convertAllMessagesForResponsesAPIWithParams(Params{Messages: []types.Message{message}})
	assertProviderResultSeparation(t, "Responses", responsesInput)

	openAIMessages := convertUserMessage(message)
	assertProviderResultSeparation(t, "OpenAI", openAIMessages)
}

func assertProviderResultSeparation(t *testing.T, providerName string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", providerName, err)
	}
	payload := string(raw)
	if !strings.Contains(payload, "model-visible") {
		t.Fatalf("%s payload lost mapped model text: %s", providerName, raw)
	}
	if strings.Contains(payload, "internal_secret") || strings.Contains(payload, "do-not-leak") {
		t.Fatalf("%s payload leaked typed data: %s", providerName, raw)
	}
}
