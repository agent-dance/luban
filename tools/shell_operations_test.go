package tools

import (
	"context"
	"os"
	"testing"
)

func TestEnvGetTool(t *testing.T) {
	tool := &EnvGetTool{}
	ctx := context.Background()

	os.Setenv("TEST_VAR", "test_value")
	result, err := tool.Execute(ctx, map[string]any{
		"name": "TEST_VAR",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}

	result, err = tool.Execute(ctx, map[string]any{
		"name": "NONEXISTENT_VAR_12345",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for non-existent variable")
	}
}

func TestEnvSetTool(t *testing.T) {
	tool := &EnvSetTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]any{
		"name":  "TEST_SET_VAR",
		"value": "test_value",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}

	value, exists := os.LookupEnv("TEST_SET_VAR")
	if !exists || value != "test_value" {
		t.Errorf("environment variable not set correctly")
	}
}

func TestEnvListTool(t *testing.T) {
	tool := &EnvListTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]any{})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}

	os.Setenv("FILTERED_VAR_1", "value1")
	result, err = tool.Execute(ctx, map[string]any{
		"filter": "FILTERED_VAR",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}
}

func TestPwdTool(t *testing.T) {
	tool := &PwdTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]any{})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}
}

func TestCwdTool(t *testing.T) {
	tool := &CwdTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]any{})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}
}

func TestWdTool(t *testing.T) {
	tool := &WdTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]any{})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}
}

func TestCdTool(t *testing.T) {
	tool := &CdTool{
		AllowedDirs: []string{},
	}
	ctx := context.Background()

	initialCwd, _ := os.Getwd()

	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	result, err := tool.Execute(ctx, map[string]any{
		"path": tmpDir,
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}

	os.Chdir(initialCwd)

	result, err = tool.Execute(ctx, map[string]any{
		"path": "/nonexistent/directory",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for non-existent directory")
	}
}
