package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type parityProviderTool struct{}

func (parityProviderTool) Name() string             { return "ParityProvider" }
func (parityProviderTool) Description() string      { return "provider parity test tool" }
func (parityProviderTool) Schema() types.JSONSchema { return types.JSONSchema{Type: "object"} }
func (parityProviderTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}
func (parityProviderTool) MapToolResultToToolResultBlock(_ any, toolUseID string) types.ToolResultBlock {
	return types.ToolResultBlock{ToolUseID: toolUseID, Content: "model-visible"}
}

// Typed tool data is for Go-side observers. Providers must serialize only the
// TS-style mapped tool_result text, otherwise golden data leaks into context.
func TestParityProvidersSerializeModelTextWithoutTypedData(t *testing.T) {
	block := types.MapToolResult(parityProviderTool{}, types.ToolResult{
		Data: map[string]any{"internal_secret": "do-not-leak"},
	}, "toolu_parity")
	message := types.ToolResultMessage(block)

	assertParityProviderResult(t, "Anthropic", convertToAnthropicMessages([]types.Message{message}))
	assertParityProviderResult(t, "Responses", convertAllMessagesForResponsesAPI([]types.Message{message}))
	assertParityProviderResult(t, "OpenAI", convertUserMessage(message))
}

func assertParityProviderResult(t *testing.T, providerName string, value any) {
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
