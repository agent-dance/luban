package tools

import (
	"encoding/json"
	"fmt"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ParseInputOrError attempts to unmarshal input into target type.
// If parsing fails, returns a ToolResult with IsError=true and the error message.
// This is the primary way tools should parse their input.
//
// Example:
//
//	type FileReadInput struct {
//	    FilePath string `json:"file_path"`
//	}
//	input, result := ParseInputOrError[FileReadInput](params)
//	if result != nil {
//	    return *result, nil  // Return tool error (not infrastructure error)
//	}
func ParseInputOrError[T any](params map[string]any) (*T, *types.ToolResult) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, &types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolRuntimeInputMarshalFailed, err),
			IsError: true,
		}
	}

	var target T
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, &types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolRuntimeInputFormatInvalid, err),
			IsError: true,
		}
	}

	return &target, nil
}

// ValidateRequired checks that all required fields are present in input.
// Returns an error if any required field is missing or empty.
//
// Example:
//
//	if err := ValidateRequired(input, "file_path", "content"); err != nil {
//	    return types.ToolResult{Content: err.Error(), IsError: true}, nil
//	}
func ValidateRequired(input map[string]any, fields ...string) error {
	for _, field := range fields {
		val, ok := input[field]
		if !ok {
			return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeRequiredFieldMissing, field))
		}

		// Check for empty string
		if str, isStr := val.(string); isStr && str == "" {
			return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeRequiredFieldEmpty, field))
		}
	}
	return nil
}

// ResponseJSON wraps content in a ToolResult with IsError=false.
// Use this to return successful tool results.
//
// Example:
//
//	return ResponseJSON(map[string]string{"status": "success"})
func ResponseJSON(content any) (types.ToolResult, error) {
	data, err := json.Marshal(content)
	if err != nil {
		return types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolRuntimeResponseMarshalFailed, err),
			IsError: true,
		}, nil
	}

	return types.ToolResult{
		Content: string(data),
		IsError: false,
	}, nil
}

// ErrorResponse wraps an error in a ToolResult with IsError=true.
// Use this to return tool errors (not infrastructure errors).
//
// Example:
//
//	return ErrorResponse(fmt.Errorf("file not found: %s", path)), nil
func ErrorResponse(err error) types.ToolResult {
	return types.ToolResult{
		Content: err.Error(),
		IsError: true,
	}
}

// ErrorResponsef is a convenience function for formatting error responses.
//
// Example:
//
//	return ErrorResponsef("file not found: %s", path), nil
func ErrorResponsef(format string, args ...any) types.ToolResult {
	return ErrorResponse(fmt.Errorf(format, args...))
}

func toolRuntimeError(key i18n.Key) types.ToolResult {
	return types.ToolResult{Content: toolRuntimeText(key), IsError: true}
}

func toolRuntimeErrorf(key i18n.Key, args ...any) types.ToolResult {
	return types.ToolResult{Content: toolRuntimeFormat(key, args...), IsError: true}
}

// StringResponse wraps a string in a ToolResult with IsError=false.
// Use this for tools that return plain text results.
//
// Example:
//
//	return StringResponse("command executed successfully"), nil
func StringResponse(content string) (types.ToolResult, error) {
	return types.ToolResult{
		Content: content,
		IsError: false,
	}, nil
}

// GetStringField gets a string field from input with a default value.
// Returns the string value if present, otherwise returns the default.
func GetStringField(input map[string]any, field string, defaultValue string) string {
	if val, ok := input[field].(string); ok {
		return val
	}
	return defaultValue
}

// GetBoolField gets a bool field from input with a default value.
func GetBoolField(input map[string]any, field string, defaultValue bool) bool {
	if val, ok := input[field].(bool); ok {
		return val
	}
	return defaultValue
}

// GetIntField gets an int field from input with a default value.
// Accepts both int and float64 (common in JSON unmarshaling).
func GetIntField(input map[string]any, field string, defaultValue int) int {
	switch val := input[field].(type) {
	case float64:
		return int(val)
	case int:
		return val
	default:
		return defaultValue
	}
}

// MustGetStringField gets a string field from input.
// Returns an error if the field is missing or not a string.
func MustGetStringField(input map[string]any, field string) (string, error) {
	val, ok := input[field].(string)
	if !ok {
		return "", fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeFieldStringRequired, field))
	}
	return val, nil
}

// MustGetBoolField gets a bool field from input.
// Returns an error if the field is missing or not a bool.
func MustGetBoolField(input map[string]any, field string) (bool, error) {
	val, ok := input[field].(bool)
	if !ok {
		return false, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeFieldBooleanRequired, field))
	}
	return val, nil
}
