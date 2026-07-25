package types

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type contractTestOutput struct {
	Value string `json:"value"`
}

type contractTestTool struct{}

func (contractTestTool) Name() string        { return "ContractTest" }
func (contractTestTool) Description() string { return "contract test tool" }
func (contractTestTool) Schema() JSONSchema {
	return StrictObjectSchema(map[string]any{
		"value": map[string]any{"type": "string"},
	}, "value")
}
func (contractTestTool) Execute(context.Context, map[string]any) (ToolResult, error) {
	return ToolResult{}, nil
}
func (contractTestTool) ToolMetadata(map[string]any) ToolMetadata {
	return ToolMetadata{MaxResultSizeChars: 1234}
}
func (contractTestTool) MapToolResultToToolResultBlock(data any, toolUseID string) ToolResultBlock {
	out := data.(contractTestOutput)
	return ToolResultBlock{ToolUseID: toolUseID, Content: "mapped: " + out.Value}
}

func TestStrictObjectSchemaContract(t *testing.T) {
	schema := StrictObjectSchema(map[string]any{
		"value": map[string]any{"type": "string"},
	}, "value")
	if !schema.RejectsUnknownFields() {
		t.Fatal("strict object schema must reject unknown fields")
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"additionalProperties":false`) {
		t.Fatalf("strict schema JSON = %s, want additionalProperties:false", raw)
	}
}

func TestToolDefinitionDerivesStrictFromInputSchema(t *testing.T) {
	def := ToDefinition(contractTestTool{})
	if !def.Strict {
		t.Fatal("strict API flag was not derived from the input schema")
	}
}

func TestDecodeStrictToolInputRejectsUnknownFields(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}

	_, err := DecodeStrictToolInput[input](map[string]any{
		"value": "ok",
		"extra": true,
	})
	if err == nil || !strings.Contains(err.Error(), `unknown field "extra"`) {
		t.Fatalf("DecodeStrictToolInput error = %v, want unknown field", err)
	}
}

func TestMapToolResultSeparatesTypedDataFromModelContent(t *testing.T) {
	data := contractTestOutput{Value: "typed"}
	result := ToolResult{
		Content:  `{"value":"go-only fallback"}`,
		Data:     data,
		Metadata: map[string]string{"source": "test"},
	}

	block := MapToolResult(contractTestTool{}, result, "toolu_1")
	if block.Content != "mapped: typed" {
		t.Fatalf("mapped content = %q", block.Content)
	}
	if got, ok := block.Data.(contractTestOutput); !ok || got != data {
		t.Fatalf("typed data was not preserved: %#v", block.Data)
	}
	if block.Metadata["maxResultSizeChars"] != "1234" {
		t.Fatalf("result-size contract not bridged to result metadata: %#v", block.Metadata)
	}
	if block.Metadata["source"] != "test" {
		t.Fatalf("tool result metadata was dropped by mapper: %#v", block.Metadata)
	}

	raw, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"value"`) || strings.Contains(string(raw), "go-only") {
		t.Fatalf("provider-bound block leaked typed/internal data: %s", raw)
	}
	if !strings.Contains(string(raw), "mapped: typed") {
		t.Fatalf("provider-bound block missing mapped model content: %s", raw)
	}
}
