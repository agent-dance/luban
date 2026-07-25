package registry

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/types"
)

type mockTool struct {
	name     string
	metadata types.ToolMetadata
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return "mock " + m.name }
func (m *mockTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (m *mockTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "executed " + m.name}, nil
}
func (m *mockTool) ToolMetadata(map[string]any) types.ToolMetadata { return m.metadata }

type deferredMockTool struct {
	mockTool
	meta toolmeta.Metadata
}

func (m *deferredMockTool) ToolDiscoveryMetadata() toolmeta.Metadata { return m.meta }

type barrierUnawareRuntimeProvider struct {
	called bool
}

func (p *barrierUnawareRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	p.called = true
	return types.ToolRuntimeContext{PermissionMode: "default"}
}

type barrierSafeRuntimeProvider struct {
	barrierUnawareRuntimeProvider
}

func (*barrierSafeRuntimeProvider) ToolRuntimeContextUnbarriered() types.ToolRuntimeContext {
	return types.ToolRuntimeContext{PermissionMode: "bypassPermissions"}
}

func TestRuntimeContextWithinSessionBarrierRequiresExplicitSafeProvider(t *testing.T) {
	reg := New()
	unsafe := &barrierUnawareRuntimeProvider{}
	reg.SetRuntimeContextProvider(unsafe)
	if _, ok := reg.RuntimeContextWithinSessionBarrier(); ok {
		t.Fatal("barrier-unaware provider was sampled while the caller owns the session barrier")
	}
	if unsafe.called {
		t.Fatal("barrier-unaware ToolRuntimeContext was called and can recursively deadlock")
	}

	safe := &barrierSafeRuntimeProvider{}
	reg.SetRuntimeContextProvider(safe)
	runtime, ok := reg.RuntimeContextWithinSessionBarrier()
	if !ok || runtime.PermissionMode != "bypassPermissions" {
		t.Fatalf("barrier-safe runtime snapshot = (%+v, %v), want Auto snapshot", runtime, ok)
	}
	if safe.called {
		t.Fatal("general runtime method was called instead of the barrier-safe snapshot method")
	}
}

func TestRegisterAndGet(t *testing.T) {
	reg := New()
	tool := &mockTool{name: "test_tool"}
	reg.Register(tool)

	got := reg.Get("test_tool")
	if got == nil {
		t.Fatal("expected to find tool")
	}
	if got.Name() != "test_tool" {
		t.Errorf("expected name 'test_tool', got '%s'", got.Name())
	}
}

func TestGetUnknown(t *testing.T) {
	reg := New()
	if reg.Get("nonexistent") != nil {
		t.Error("expected nil for unknown tool")
	}
}

func TestUnregisterRemovesCanonicalTool(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "PowerShell"})
	reg.Register(&mockTool{name: "Read"})
	if !reg.Unregister("PowerShell") {
		t.Fatal("Unregister(PowerShell) = false, want true")
	}
	if reg.Get("PowerShell") != nil {
		t.Fatalf("unregistered tool remains: %v", reg.Get("PowerShell"))
	}
	if names := reg.Names(); len(names) != 1 || names[0] != "Read" {
		t.Fatalf("registry order after unregister = %v", names)
	}
}

func TestAll(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "a"})
	reg.Register(&mockTool{name: "b"})
	reg.Register(&mockTool{name: "c"})

	all := reg.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(all))
	}
	// Verify registration order
	if all[0].Name() != "a" || all[1].Name() != "b" || all[2].Name() != "c" {
		t.Error("tools not in registration order")
	}
}

func TestExecuteTool(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "my_tool"})

	result := reg.ExecuteTool(context.Background(), "my_tool", map[string]any{})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if result.Content != "executed my_tool" {
		t.Errorf("expected 'executed my_tool', got '%s'", result.Content)
	}
}

func TestExecuteToolUnknown(t *testing.T) {
	reg := New()
	result := reg.ExecuteTool(context.Background(), "unknown", map[string]any{})
	if !result.IsError {
		t.Error("expected error for unknown tool")
	}
}

func TestDefinitions(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "x"})
	reg.Register(&mockTool{name: "y"})

	defs := reg.Definitions()
	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(defs))
	}
	if defs[0].Name != "x" || defs[1].Name != "y" {
		t.Error("definitions not in order")
	}
}

func TestVisibleDefinitions_HidesDeferredToolsUntilLoaded(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "Read"})
	reg.Register(&mockTool{name: "ToolSearch"})
	reg.Register(&deferredMockTool{
		mockTool: mockTool{name: "TaskCreate"},
		meta:     toolmeta.Metadata{ShouldDefer: true},
	})

	defs := reg.VisibleDefinitions(nil)
	if len(defs) != 1 {
		t.Fatalf("expected 1 visible definition before load, got %d", len(defs))
	}
	if defs[0].Name != "ToolSearch" {
		t.Fatalf("unexpected visible definitions before load: %#v", defs)
	}

	loaded := map[string]struct{}{"Read": {}, "TaskCreate": {}}
	defs = reg.VisibleDefinitions(loaded)
	if len(defs) != 3 {
		t.Fatalf("expected 3 visible definitions after load, got %d", len(defs))
	}
	if defs[0].Name != "Read" || defs[2].Name != "TaskCreate" {
		t.Fatalf("expected deferred tool to become visible after load, got %#v", defs)
	}
}

func TestCount(t *testing.T) {
	reg := New()
	if reg.Count() != 0 {
		t.Error("expected 0 count")
	}
	reg.Register(&mockTool{name: "a"})
	if reg.Count() != 1 {
		t.Error("expected 1 count")
	}
}

func TestGrepDiscoveryMetadataHasSearchHint(t *testing.T) {
	meta := DiscoveryMetadata(&mockTool{name: "Grep"})
	if meta.SearchHint != "search file contents with regex (ripgrep)" {
		t.Fatalf("Grep search hint mismatch: %+v", meta)
	}
}
