package registry

import (
	"context"
	"sync"
	"testing"

	"github.com/agent-dance/luban/types"
)

type errorTool struct{}

func (t *errorTool) Name() string        { return "error_tool" }
func (t *errorTool) Description() string { return "always errors" }
func (t *errorTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *errorTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, &testError{"tool exploded"}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestRegisterDuplicateOverwrites(t *testing.T) {
	reg := New()
	tool1 := &mockTool{name: "dup"}
	tool2 := &mockTool{name: "dup"}
	reg.Register(tool1)
	reg.Register(tool2) // should overwrite, not panic

	got := reg.Get("dup")
	if got != tool2 {
		t.Error("expected duplicate registration to overwrite with new tool")
	}
	// Should not duplicate in order slice
	if reg.Count() != 1 {
		t.Errorf("expected 1 tool after duplicate register, got %d", reg.Count())
	}
}

func TestExecuteToolReturnsError(t *testing.T) {
	reg := New()
	reg.Register(&errorTool{})

	result := reg.ExecuteTool(context.Background(), "error_tool", map[string]any{})
	if !result.IsError {
		t.Error("expected IsError for tool that returns error")
	}
	if result.Content == "" {
		t.Error("expected error message in content")
	}
}

func TestExecuteToolWithErrorSurfacesInfraError(t *testing.T) {
	reg := New()
	reg.Register(&errorTool{})

	result, err := reg.ExecuteToolWithError(context.Background(), "error_tool", map[string]any{})
	if err == nil {
		t.Fatal("expected non-nil error for infrastructure failure")
	}
	if err.Error() != "tool exploded" {
		t.Errorf("expected 'tool exploded', got '%s'", err.Error())
	}
	// Result should be zero-value (not populated)
	if result.Content != "" {
		t.Errorf("expected empty content on infra error, got '%s'", result.Content)
	}
}

func TestExecuteToolWithErrorBusinessError(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "biz_err"})

	result, err := reg.ExecuteToolWithError(context.Background(), "biz_err", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	// mockTool returns successful result
	if result.Content != "executed biz_err" {
		t.Errorf("expected 'executed biz_err', got '%s'", result.Content)
	}
}

func TestExecuteToolWithErrorUnknownTool(t *testing.T) {
	reg := New()

	result, err := reg.ExecuteToolWithError(context.Background(), "nope", map[string]any{})
	if err != nil {
		t.Fatalf("unknown tool should not be an infra error, got: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError for unknown tool")
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	reg := New()

	var wg sync.WaitGroup
	// 10 concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "tool_" + string(rune('A'+n))
			reg.Register(&mockTool{name: name})
		}(i)
	}

	// 10 concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = reg.All()
			_ = reg.Names()
			_ = reg.Count()
			_ = reg.Get("tool_A")
		}()
	}

	wg.Wait()
	if reg.Count() != 10 {
		t.Errorf("expected 10 tools, got %d", reg.Count())
	}
}
