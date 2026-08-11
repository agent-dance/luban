package registry

import (
	"context"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/types"
)

type versionedVisibleTool struct {
	name        string
	description string
}

type visibleSnapshotRuntimeProvider struct {
	runtime types.ToolRuntimeContext
}

func (p visibleSnapshotRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	return p.runtime
}

func (t *versionedVisibleTool) Name() string        { return t.name }
func (t *versionedVisibleTool) Description() string { return t.description }
func (t *versionedVisibleTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"value": map[string]any{"type": "string", "description": t.description},
	}, "value")
}
func (t *versionedVisibleTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}

func TestAgenticV2VisibleSnapshotIsExactStableAndContentAddressed(t *testing.T) {
	reg := New()
	reg.SetModelToolProfile(ModelToolProfileAgenticV2)
	for _, name := range []string{"TaskCreate", "Run", "Agent", "Inspect", "ToolSearch", "ApplyPatch"} {
		reg.Register(&versionedVisibleTool{name: name, description: "v1"})
	}

	first, err := reg.SnapshotVisibleTools(map[string]struct{}{
		"Agent": {}, "TaskCreate": {},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"Inspect", "ApplyPatch", "Run"}
	if got := first.Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("visible names = %v, want %v", got, wantNames)
	}
	for _, name := range wantNames {
		if !first.Allows(name) {
			t.Fatalf("snapshot rejected visible tool %q", name)
		}
	}
	for _, name := range []string{"Bash", "Read", "run", ""} {
		if first.Allows(name) {
			t.Fatalf("snapshot accepted tool outside exact catalog %q", name)
		}
	}
	if got := reg.VisibleDefinitions(map[string]struct{}{"Agent": {}}); len(got) != 3 {
		t.Fatalf("direct visible definitions = %#v, want exact V2 core", got)
	}

	// Unrelated registry growth does not perturb the semantic model envelope.
	reg.Register(&versionedVisibleTool{name: "WebFetch", description: "v1"})
	second, err := reg.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest() != second.Digest() {
		t.Fatalf("unrelated tool changed V2 digest: %s != %s", first.Digest(), second.Digest())
	}
	if first.Generation() == second.Generation() {
		t.Fatal("source generation did not advance after registration")
	}

	reg.Register(&versionedVisibleTool{name: "Run", description: "v2"})
	third, err := reg.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Digest() == third.Digest() {
		t.Fatal("visible schema replacement did not change the content digest")
	}

	// Accessors are defensive: mutating a returned schema cannot alter either
	// the snapshot digest or a later provider projection.
	definitions := third.Definitions()
	definitions[0].Name = "Corrupted"
	definitions[0].InputSchema.Properties["injected"] = map[string]any{"type": "boolean"}
	if got := third.Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("snapshot was mutated through accessor: %v", got)
	}
	if _, exists := third.Definitions()[0].InputSchema.Properties["injected"]; exists {
		t.Fatal("nested schema map was not defensively cloned")
	}
}

func TestAgenticV2VisibleSnapshotFailsClosedWhenCoreIsIncomplete(t *testing.T) {
	reg := New()
	reg.SetModelToolProfile(ModelToolProfileAgenticV2)
	reg.Register(&versionedVisibleTool{name: "Inspect", description: "v1"})
	reg.Register(&versionedVisibleTool{name: "ApplyPatch", description: "v1"})
	if _, err := reg.SnapshotVisibleTools(nil); err == nil {
		t.Fatal("incomplete Agentic V2 catalog was accepted")
	}
}

func TestAgenticV2VisibleSnapshotIncludesStableOptionalContextUpdate(t *testing.T) {
	reg := New()
	reg.SetModelToolProfile(ModelToolProfileAgenticV2)
	for _, name := range []string{"Run", "ContextUpdate", "ApplyPatch", "Inspect"} {
		reg.Register(&versionedVisibleTool{name: name, description: "v1"})
	}
	snapshot, err := reg.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Inspect", "ApplyPatch", "Run", "ContextUpdate"}
	if got := snapshot.Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible names = %v, want %v", got, want)
	}
	second, err := reg.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Digest() != second.Digest() {
		t.Fatalf("optional context catalog digest drifted: %s != %s", snapshot.Digest(), second.Digest())
	}
}

func TestAgenticV2VisibleSnapshotIsIndependentFromExecutionAllowList(t *testing.T) {
	reg := New()
	reg.SetModelToolProfile(ModelToolProfileAgenticV2)
	reg.SetRuntimeContextProvider(visibleSnapshotRuntimeProvider{runtime: types.ToolRuntimeContext{
		AllowedTools: map[string]bool{"Run": true},
	}})
	for _, name := range []string{"Inspect", "ApplyPatch", "Run"} {
		reg.Register(&versionedVisibleTool{name: name, description: "v1"})
	}

	snapshot, err := reg.SnapshotVisibleTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"Inspect", "ApplyPatch", "Run"}
	if got := snapshot.Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("visible names = %v, want fixed Agentic V2 contract %v", got, wantNames)
	}
	if reg.IsToolEnabled(reg.Get("Inspect")) {
		t.Fatal("Inspect execution unexpectedly bypassed the Run-only allow-list")
	}
	if !reg.IsToolEnabled(reg.Get("Run")) {
		t.Fatal("Run execution was disabled by its own allow-list")
	}
}
