package provider

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestProviderToolWireOrderIsCanonicalAndDoesNotMutateInputs(t *testing.T) {
	forward := []types.ToolDefinition{
		{
			Name:        "zeta_tool",
			Description: "last tool",
			InputSchema: types.StrictObjectSchema(map[string]any{"zeta": map[string]any{"type": "string"}}, "zeta"),
		},
		{
			Name:        "alpha_tool",
			Description: "first tool",
			InputSchema: types.StrictObjectSchema(map[string]any{"alpha": map[string]any{"type": "string"}}, "alpha"),
		},
	}
	reverse := []types.ToolDefinition{forward[1], forward[0]}
	forwardNames := toolDefinitionNames(forward)
	reverseNames := toolDefinitionNames(reverse)

	tests := []struct {
		name    string
		convert func([]types.ToolDefinition) any
	}{
		{
			name: "OpenAI Chat",
			convert: func(definitions []types.ToolDefinition) any {
				return convertToolsToOpenAIWithStrictMode(definitions, true)
			},
		},
		{
			name: "OpenAI Responses",
			convert: func(definitions []types.ToolDefinition) any {
				return convertToolsToResponsesAPIWithStrictMode(definitions, true)
			},
		},
		{
			name: "Anthropic Messages",
			convert: func(definitions []types.ToolDefinition) any {
				converted, err := convertToAnthropicTools(definitions)
				if err != nil {
					t.Fatalf("convert Anthropic tools: %v", err)
				}
				return converted
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			forwardWire := mustMarshalToolWire(t, test.convert(forward))
			reverseWire := mustMarshalToolWire(t, test.convert(reverse))
			if string(forwardWire) != string(reverseWire) {
				t.Fatalf("tool order changed wire payload\nforward: %s\nreverse: %s", forwardWire, reverseWire)
			}
			var decoded []map[string]any
			if err := json.Unmarshal(forwardWire, &decoded); err != nil {
				t.Fatalf("decode tool wire payload: %v", err)
			}
			if got := wireToolName(decoded[0]); got != "alpha_tool" {
				t.Fatalf("first wire tool = %q, want alpha_tool: %s", got, forwardWire)
			}
		})
	}

	if got := toolDefinitionNames(forward); !reflect.DeepEqual(got, forwardNames) {
		t.Fatalf("forward caller slice mutated: got %v, want %v", got, forwardNames)
	}
	if got := toolDefinitionNames(reverse); !reflect.DeepEqual(got, reverseNames) {
		t.Fatalf("reverse caller slice mutated: got %v, want %v", got, reverseNames)
	}
}

func TestAnthropicServerToolWireOrderIsCanonicalAndDoesNotMutateInputs(t *testing.T) {
	forward := []types.ServerToolDefinition{
		{
			Type:           "web_search_20250305",
			Name:           "web_search",
			AllowedDomains: []string{"zeta.example"},
			MaxUses:        9,
		},
		{
			Type:           "web_search_20250305",
			Name:           "web_search",
			AllowedDomains: []string{"alpha.example"},
			MaxUses:        2,
		},
	}
	reverse := []types.ServerToolDefinition{forward[1], forward[0]}
	forwardSnapshot := append([]types.ServerToolDefinition(nil), forward...)
	reverseSnapshot := append([]types.ServerToolDefinition(nil), reverse...)

	convert := func(definitions []types.ServerToolDefinition) []byte {
		t.Helper()
		converted, err := convertToAnthropicServerTools(definitions)
		if err != nil {
			t.Fatalf("convert Anthropic server tools: %v", err)
		}
		return mustMarshalToolWire(t, converted)
	}
	forwardWire := convert(forward)
	reverseWire := convert(reverse)
	if string(forwardWire) != string(reverseWire) {
		t.Fatalf("server tool order changed wire payload\nforward: %s\nreverse: %s", forwardWire, reverseWire)
	}
	if !reflect.DeepEqual(forward, forwardSnapshot) {
		t.Fatalf("forward server tool caller slice mutated: got %#v, want %#v", forward, forwardSnapshot)
	}
	if !reflect.DeepEqual(reverse, reverseSnapshot) {
		t.Fatalf("reverse server tool caller slice mutated: got %#v, want %#v", reverse, reverseSnapshot)
	}
}

func TestAgenticToolWireOrderMatchesInspectMutateVerifyWorkflow(t *testing.T) {
	definitions := []types.ToolDefinition{
		{Name: "Run", InputSchema: types.StrictObjectSchema(map[string]any{})},
		{Name: "Inspect", InputSchema: types.StrictObjectSchema(map[string]any{})},
		{Name: "ApplyPatch", InputSchema: types.StrictObjectSchema(map[string]any{})},
	}
	ordered := canonicalToolDefinitions(definitions)
	if got := toolDefinitionNames(ordered); !reflect.DeepEqual(got, []string{"Inspect", "ApplyPatch", "Run"}) {
		t.Fatalf("agentic wire order = %v", got)
	}
	if got := toolDefinitionNames(definitions); !reflect.DeepEqual(got, []string{"Run", "Inspect", "ApplyPatch"}) {
		t.Fatalf("caller definitions were mutated: %v", got)
	}
}

func mustMarshalToolWire(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal tool wire payload: %v", err)
	}
	return raw
}

func toolDefinitionNames(definitions []types.ToolDefinition) []string {
	names := make([]string, len(definitions))
	for i, definition := range definitions {
		names[i] = definition.Name
	}
	return names
}

func wireToolName(tool map[string]any) string {
	if function, ok := tool["function"].(map[string]any); ok {
		return function["name"].(string)
	}
	return tool["name"].(string)
}
