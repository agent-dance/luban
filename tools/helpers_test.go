package tools

import (
	"fmt"
	"testing"
)

func TestParseInputOrError(t *testing.T) {
	type TestInput struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	// Test successful parse
	input := map[string]any{
		"name":  "test",
		"count": 42,
	}

	parsed, result := ParseInputOrError[TestInput](input)
	if result != nil {
		t.Errorf("expected no error, got %v", result)
	}
	if parsed.Name != "test" || parsed.Count != 42 {
		t.Errorf("expected Name=test Count=42, got Name=%s Count=%d", parsed.Name, parsed.Count)
	}

	// Test invalid input
	badInput := map[string]any{
		"count": "not a number",
	}

	parsed, result = ParseInputOrError[TestInput](badInput)
	if result == nil || !result.IsError {
		t.Error("expected error for invalid input")
	}
}

func TestValidateRequired(t *testing.T) {
	// Test success case
	input := map[string]any{
		"file_path": "/tmp/test",
		"content":   "data",
	}

	if err := ValidateRequired(input, "file_path", "content"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Test missing field
	if err := ValidateRequired(input, "file_path", "missing"); err == nil {
		t.Error("expected error for missing field")
	}

	// Test empty field
	emptyInput := map[string]any{
		"file_path": "",
	}

	if err := ValidateRequired(emptyInput, "file_path"); err == nil {
		t.Error("expected error for empty field")
	}
}

func TestResponseJSON(t *testing.T) {
	content := map[string]string{"status": "success", "message": "ok"}

	result, err := ResponseJSON(content)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false")
	}
	if result.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestErrorResponse(t *testing.T) {
	err := fmt.Errorf("test error message")

	result := ErrorResponse(err)
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if result.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestErrorResponsef(t *testing.T) {
	result := ErrorResponsef("file not found: %s", "test.txt")
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if result.Content != "file not found: test.txt" {
		t.Errorf("expected 'file not found: test.txt', got %q", result.Content)
	}
}

func TestStringResponse(t *testing.T) {
	result, err := StringResponse("success")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false")
	}
	if result.Content != "success" {
		t.Errorf("expected 'success', got %q", result.Content)
	}
}

func TestGetFields(t *testing.T) {
	input := map[string]any{
		"name":   "test",
		"active": true,
		"count":  42.0, // JSON unmarshals numbers as float64
	}

	// Test GetStringField
	if val := GetStringField(input, "name", "default"); val != "test" {
		t.Errorf("expected 'test', got %q", val)
	}
	if val := GetStringField(input, "missing", "default"); val != "default" {
		t.Errorf("expected 'default', got %q", val)
	}

	// Test GetBoolField
	if val := GetBoolField(input, "active", false); !val {
		t.Error("expected true")
	}
	if val := GetBoolField(input, "missing", true); !val {
		t.Error("expected true (default)")
	}

	// Test GetIntField with float64
	if val := GetIntField(input, "count", 0); val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
	if val := GetIntField(input, "missing", 100); val != 100 {
		t.Errorf("expected 100 (default), got %d", val)
	}
}

func TestMustGetFields(t *testing.T) {
	input := map[string]any{
		"name":   "test",
		"active": true,
	}

	// Test MustGetStringField success
	val, err := MustGetStringField(input, "name")
	if err != nil || val != "test" {
		t.Errorf("expected 'test', got %q with error %v", val, err)
	}

	// Test MustGetStringField failure
	val, err = MustGetStringField(input, "missing")
	if err == nil {
		t.Error("expected error for missing field")
	}

	// Test MustGetBoolField success
	bval, err := MustGetBoolField(input, "active")
	if err != nil || !bval {
		t.Errorf("expected true with no error, got %v with error %v", bval, err)
	}

	// Test MustGetBoolField failure
	bval, err = MustGetBoolField(input, "name")
	if err == nil {
		t.Error("expected error for wrong type")
	}
}
