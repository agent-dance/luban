package registry

import (
	"context"
	"reflect"
	"strings"
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
	if len(defs) != 2 {
		t.Fatalf("expected 2 visible definitions before load, got %d", len(defs))
	}
	if defs[0].Name != "Read" || defs[1].Name != "ToolSearch" {
		t.Fatalf("unexpected visible definitions before load: %#v", defs)
	}

	loaded := map[string]struct{}{"TaskCreate": {}}
	defs = reg.VisibleDefinitions(loaded)
	if len(defs) != 3 {
		t.Fatalf("expected 3 visible definitions after load, got %d", len(defs))
	}
	if defs[0].Name != "Read" || defs[2].Name != "TaskCreate" {
		t.Fatalf("expected deferred tool to become visible after load, got %#v", defs)
	}
}

func TestVisibleDefinitionsKeepEagerCoreCanonicalAcrossDiscovery(t *testing.T) {
	reg := New()
	for _, name := range []string{"TaskCreate", "Run", "ApplyPatch", "Inspect", "Grep", "ToolSearch", "Glob", "Edit", "Write", "Read", "PowerShell", "Bash"} {
		reg.Register(&mockTool{name: name})
	}

	wantCore := []string{"Bash", "PowerShell", "Read", "Write", "Edit", "Glob", "Grep", "Inspect", "ApplyPatch", "Run"}
	before := reg.VisibleDefinitions(nil)
	if len(before) != len(wantCore)+1 {
		t.Fatalf("visible definitions before discovery = %#v", before)
	}
	for index, name := range wantCore {
		if before[index].Name != name {
			t.Fatalf("core definition %d = %q, want %q", index, before[index].Name, name)
		}
	}

	after := reg.VisibleDefinitions(map[string]struct{}{"TaskCreate": {}})
	if len(after) != len(before)+1 {
		t.Fatalf("visible definitions after discovery = %#v", after)
	}
	for index := range wantCore {
		if !reflect.DeepEqual(before[index], after[index]) {
			t.Fatalf("core definition changed at index %d: before=%#v after=%#v", index, before[index], after[index])
		}
	}
}

func TestVisibleDefinitionsAgenticV2CoreOrder(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "Run"})
	reg.Register(&mockTool{name: "ApplyPatch"})
	reg.Register(&mockTool{name: "Inspect"})

	definitions := reg.VisibleDefinitions(nil)
	want := []string{"Inspect", "ApplyPatch", "Run"}
	if len(definitions) != len(want) {
		t.Fatalf("V2 definitions = %#v", definitions)
	}
	for index, name := range want {
		if definitions[index].Name != name {
			t.Fatalf("V2 definition %d = %q, want %q", index, definitions[index].Name, name)
		}
	}
}

func TestEagerCoreCannotBeDeferredByToolMetadata(t *testing.T) {
	for _, name := range eagerCoreToolOrder {
		tool := &deferredMockTool{
			mockTool: mockTool{name: name},
			meta:     toolmeta.Metadata{ShouldDefer: true},
		}
		if IsDeferredTool(tool) {
			t.Fatalf("%s must remain eager even when tool metadata requests deferral", name)
		}
		if meta := DiscoveryMetadata(tool); !meta.AlwaysLoad {
			t.Fatalf("%s discovery metadata = %+v, want AlwaysLoad", name, meta)
		}
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

func TestRunDiscoveryMetadataIsAlwaysLoaded(t *testing.T) {
	meta := DiscoveryMetadata(&mockTool{name: "Run"})
	if !meta.AlwaysLoad || meta.ShouldDefer || !strings.Contains(meta.SearchHint, "verification") {
		t.Fatalf("Run discovery metadata = %+v", meta)
	}
}
