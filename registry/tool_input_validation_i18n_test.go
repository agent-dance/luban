package registry

import (
	"context"
	"strings"
	"testing"
)

func TestRegistryStrictValidationUsesSingleLocalizedProtocolEnvelope(t *testing.T) {
	tool := &strictContractTool{readOnly: true}
	reg := New()
	reg.Register(tool)

	result := reg.ExecuteTool(context.Background(), tool.Name(), map[string]any{
		"message": "hello",
		"extra":   true,
	})
	if strings.Count(result.Content, "<tool_use_error>") != 1 ||
		strings.Count(result.Content, "InputValidationError") != 1 ||
		!strings.Contains(result.Content, "StrictContract") ||
		!strings.Contains(result.Content, "`extra`") {
		t.Fatalf("strict validation result = %q, want one localized protocol envelope with raw identifiers", result.Content)
	}
	if !result.IsError || tool.executed {
		t.Fatalf("strict validation result = %#v, executed=%v", result, tool.executed)
	}
}
