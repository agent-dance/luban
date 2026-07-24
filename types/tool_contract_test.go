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
func (contractTestTool) ToolContract() ToolContract {
	return ToolContract{
		OutputSchema: &JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"value": map[string]any{"type": "string"},
			},
			Required:             []string{"value"},
			AdditionalProperties: false,
		},
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 1234,
	}
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

func TestToolContractDefinitionCarriesOutputSchemaAndMetadata(t *testing.T) {
	def := ToDefinition(contractTestTool{})
	if def.OutputSchema == nil || def.OutputSchema.Type != "object" {
		t.Fatalf("output schema missing from definition: %#v", def.OutputSchema)
	}
	if !def.Strict {
		t.Fatal("strict API flag was not preserved")
	}
	if !def.Metadata.ReadOnly || !def.Metadata.ConcurrencySafe {
		t.Fatalf("tool metadata not preserved: %#v", def.Metadata)
	}
	if def.Metadata.MaxResultSizeChars != 1234 {
		t.Fatalf("max result size = %d, want 1234", def.Metadata.MaxResultSizeChars)
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
