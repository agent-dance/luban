package types

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recursiveContractTestTool struct {
	schema JSONSchema
}

func (t recursiveContractTestTool) Name() string        { return "RecursiveContract" }
func (t recursiveContractTestTool) Description() string { return "test" }
func (t recursiveContractTestTool) Schema() JSONSchema  { return t.schema }
func (t recursiveContractTestTool) Execute(context.Context, map[string]any) (ToolResult, error) {
	return ToolResult{}, nil
}

func TestValidateToolInputRejectsNestedUnknownRequiredTypeAndBounds(t *testing.T) {
	tool := recursiveContractTestTool{schema: StrictObjectSchema(map[string]any{
		"job": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"name", "steps"},
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "minLength": 3, "maxLength": 8, "pattern": "^[a-z]+$"},
				"steps": map[string]any{
					"type": "array", "minItems": 2, "maxItems": 3,
					"items": map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
				},
			},
		},
	}, "job")}

	err := ValidateToolInput(tool, map[string]any{"job": map[string]any{
		"name": "A", "steps": []any{0.5}, "private-secret": "must-not-leak",
	}})
	var validation *ToolInputValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T, want structured validation", err)
	}
	want := map[ToolContractViolationCode]bool{
		ToolContractViolationMinLength:          false,
		ToolContractViolationPattern:            false,
		ToolContractViolationMinItems:           false,
		ToolContractViolationType:               false,
		ToolContractViolationUnexpectedProperty: false,
	}
	for _, violation := range validation.Violations {
		if _, tracked := want[violation.Code]; tracked {
			want[violation.Code] = true
		}
		if violation.Phase == "" || violation.Keyword == "" || !violation.Retryable {
			t.Errorf("incomplete input violation: %+v", violation)
		}
		if strings.Contains(violation.InstancePath, "must-not-leak") {
			t.Fatalf("path leaked rejected value: %+v", violation)
		}
	}
	for code, found := range want {
		if !found {
			t.Errorf("missing violation %s: %+v", code, validation.Violations)
		}
	}
	if strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("rendered error leaked rejected value: %q", err)
	}
}

func TestValidateToolInputReportsMissingPropertyAtEscapedJSONPointer(t *testing.T) {
	tool := recursiveContractTestTool{schema: StrictObjectSchema(map[string]any{
		"outer": map[string]any{
			"type": "object", "required": []any{"a/b~c"},
			"properties": map[string]any{"a/b~c": map[string]any{"type": "string"}},
		},
	}, "outer")}
	err := ValidateToolInput(tool, map[string]any{"outer": map[string]any{}})
	var validation *ToolInputValidationError
	if !errors.As(err, &validation) || len(validation.Violations) != 1 {
		t.Fatalf("validation = %#v err=%v", validation, err)
	}
	got := validation.Violations[0]
	if got.Code != ToolContractViolationRequired || got.InstancePath != "/outer/a~1b~0c" || got.Keyword != "required" {
		t.Fatalf("violation = %+v", got)
	}
}

func TestValidateToolInputEnforcesEnumAndBooleanCombinators(t *testing.T) {
	tool := recursiveContractTestTool{schema: StrictObjectSchema(map[string]any{
		"mode": map[string]any{"enum": []any{"new", "continue"}},
		"payload": map[string]any{
			"oneOf": []any{
				map[string]any{"type": "object", "required": []any{"requests"}},
				map[string]any{"type": "object", "required": []any{"cursor"}},
			},
		},
		"choice": map[string]any{
			"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}},
		},
		"allowed": map[string]any{"not": map[string]any{"enum": []any{"blocked"}}},
	}, "mode", "payload", "choice", "allowed")}

	valid := map[string]any{
		"mode": "new", "payload": map[string]any{"requests": []any{}}, "choice": 1.0, "allowed": "ready",
	}
	if err := ValidateToolInput(tool, valid); err != nil {
		t.Fatalf("valid XOR input rejected: %v", err)
	}

	err := ValidateToolInput(tool, map[string]any{
		"mode": "other", "payload": map[string]any{"requests": []any{}, "cursor": "c"},
		"choice": true, "allowed": "blocked",
	})
	var validation *ToolInputValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T, want validation", err)
	}
	want := map[ToolContractViolationCode]bool{
		ToolContractViolationEnum: false, ToolContractViolationOneOf: false,
		ToolContractViolationAnyOf: false, ToolContractViolationNot: false,
	}
	for _, violation := range validation.Violations {
		if _, ok := want[violation.Code]; ok {
			want[violation.Code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Errorf("missing %s in %+v", code, validation.Violations)
		}
	}
}

func TestValidateToolInputAdditionalPropertiesSchema(t *testing.T) {
	tool := recursiveContractTestTool{schema: JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		AdditionalProperties: map[string]any{
			"type": "string", "maxLength": 2,
		},
	}}
	if err := ValidateToolInput(tool, map[string]any{"dynamic": "ok"}); err != nil {
		t.Fatalf("schema-constrained additional property rejected: %v", err)
	}
	err := ValidateToolInput(tool, map[string]any{"dynamic": "oversized-sensitive-value"})
	var validation *ToolInputValidationError
	if !errors.As(err, &validation) || validation.Violations[0].Code != ToolContractViolationMaxLength {
		t.Fatalf("validation = %#v err=%v", validation, err)
	}
	if strings.Contains(err.Error(), "oversized-sensitive-value") {
		t.Fatal("rejected additional property value leaked")
	}
}

func TestValidateToolDefinitionInputUsesTheSameRecursiveContract(t *testing.T) {
	definition := ToolDefinition{Name: "Defined", InputSchema: StrictObjectSchema(map[string]any{
		"count": map[string]any{"type": "integer", "minimum": 1},
	}, "count")}
	err := ValidateToolDefinitionInput(definition, map[string]any{"count": 0.0})
	var validation *ToolInputValidationError
	if !errors.As(err, &validation) || validation.ToolName != "Defined" || validation.Violations[0].Code != ToolContractViolationMinimum {
		t.Fatalf("definition validation = %#v err=%v", validation, err)
	}
}
