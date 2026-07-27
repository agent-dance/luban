package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type runtimeContextStub struct {
	ctx types.ToolRuntimeContext
}

func (s *runtimeContextStub) ToolRuntimeContext() types.ToolRuntimeContext { return s.ctx }

func toolNames(tools []types.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, tool := range tools {
		out[tool.Name()] = true
	}
	return out
}

func TestVisibilityKeepsTaskToolsAndAppliesRuntimeGatesAndDeniedTools(t *testing.T) {
	reg := New()
	for _, name := range []string{"Read", "TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "TeamCreate", "SendMessage", "WebSearch", "ToolSearch"} {
		reg.Register(&mockTool{name: name})
	}
	runtime := &runtimeContextStub{ctx: types.ToolRuntimeContext{
		Interactive: true,
		Features: map[string]bool{
			types.ToolFeatureTeams:      false,
			types.ToolFeatureWebSearch:  true,
			types.ToolFeatureToolSearch: false,
		},
		DeniedTools: map[string]bool{"Read": true},
	}}
	reg.SetRuntimeContextProvider(runtime)

	visible := toolNames(reg.VisibleTools(nil))
	for _, name := range []string{"TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "WebSearch"} {
		if !visible[name] {
			t.Errorf("expected %s to be visible", name)
		}
	}
	for _, name := range []string{"Read", "TeamCreate", "SendMessage", "ToolSearch"} {
		if visible[name] {
			t.Errorf("expected %s to be hidden", name)
		}
	}
}

func TestEnabledToolsKeepDeferredToolsButExcludeRuntimeDisabledTools(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "Read"})
	reg.Register(&mockTool{name: "AskUserQuestion"})
	reg.Register(&mockTool{name: "WebSearch"})
	reg.Register(&mockTool{name: "ToolSearch"})
	reg.SetRuntimeContextProvider(&runtimeContextStub{ctx: types.ToolRuntimeContext{
		Features: map[string]bool{
			types.ToolFeatureWebSearch:  false,
			types.ToolFeatureToolSearch: true,
		},
	}})

	enabled := toolNames(reg.EnabledTools())
	if !enabled["Read"] {
		t.Fatal("core Read must remain in the enabled SDK/runtime tool pool")
	}
	if !enabled["AskUserQuestion"] {
		t.Fatal("deferred AskUserQuestion must remain in the enabled SDK/runtime tool pool")
	}
	if enabled["WebSearch"] {
		t.Fatal("runtime-disabled WebSearch must not be advertised")
	}
	if !enabled["ToolSearch"] {
		t.Fatal("enabled ToolSearch must be advertised")
	}

	visible := toolNames(reg.VisibleTools(nil))
	if !visible["Read"] {
		t.Fatal("core Read must be visible without ToolSearch discovery")
	}
	if visible["AskUserQuestion"] {
		t.Fatal("deferred AskUserQuestion must stay out of the model-visible pool until loaded")
	}
}

func TestRuntimeGateBlocksDirectRegistryExecution(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "RemoteTrigger"})
	reg.SetRuntimeContextProvider(&runtimeContextStub{ctx: types.ToolRuntimeContext{
		Features: map[string]bool{types.ToolFeatureRemoteTrigger: false},
	}})

	result := reg.ExecuteTool(context.Background(), "RemoteTrigger", nil)
	if !result.IsError {
		t.Fatal("disabled tool should not execute through the registry")
	}
}

func TestUnknownToolMessageExcludesRuntimeDisabledTools(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "ToolSearch"})
	reg.Register(&mockTool{name: "WebSearch"})
	reg.SetRuntimeContextProvider(&runtimeContextStub{ctx: types.ToolRuntimeContext{
		Features: map[string]bool{
			types.ToolFeatureToolSearch: true,
			types.ToolFeatureWebSearch:  false,
		},
	}})

	result := reg.ExecuteTool(context.Background(), "Missing", nil)
	if !result.IsError || !strings.Contains(result.Content, "ToolSearch") {
		t.Fatalf("unknown-tool result = %q", result.Content)
	}
	if strings.Contains(result.Content, "WebSearch") {
		t.Fatalf("unknown-tool result advertised a disabled tool: %q", result.Content)
	}
}

func TestToolMetadataMapsReadWriteSearchAndDestructive(t *testing.T) {
	reg := New()
	reg.Register(&mockTool{name: "Read", metadata: types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true}})
	reg.Register(&mockTool{name: "Write", metadata: types.ToolMetadata{Write: true}})
	reg.Register(&mockTool{name: "Grep", metadata: types.ToolMetadata{ReadOnly: true, Search: true, ConcurrencySafe: true}})
	reg.Register(&inputAwareMetadataTool{
		mockTool: mockTool{name: "ExitWorktree"},
		metadata: func(input map[string]any) types.ToolMetadata {
			remove, _ := input["action"].(string)
			return types.ToolMetadata{Write: true, Destructive: strings.EqualFold(remove, "remove")}
		},
	})

	if got := reg.ToolMetadata("Read", nil); !got.ReadOnly || !got.ConcurrencySafe || got.Write {
		t.Fatalf("unexpected Read metadata: %+v", got)
	}
	if got := reg.ToolMetadata("Write", nil); got.ReadOnly || !got.Write {
		t.Fatalf("unexpected Write metadata: %+v", got)
	}
	if got := reg.ToolMetadata("Grep", nil); !got.ReadOnly || !got.Search {
		t.Fatalf("unexpected Grep metadata: %+v", got)
	}
	if got := reg.ToolMetadata("ExitWorktree", map[string]any{"action": "remove"}); !got.Destructive || !got.Write {
		t.Fatalf("unexpected ExitWorktree(remove) metadata: %+v", got)
	}
}

func TestPermissionResultKeepsDetachedReviewContextFromCheck(t *testing.T) {
	reg := New()
	metadata := types.ToolMetadata{ReadOnly: true, Search: true, ConcurrencySafe: true}
	reg.Register(&mockTool{name: "ReviewProbe", metadata: metadata})
	runtime := &runtimeContextStub{ctx: types.ToolRuntimeContext{
		ProjectRoot: "/workspace/project",
		AllowedDirs: []string{"/workspace/project"},
		Features:    map[string]bool{"review": true},
		AskRules:    []types.PermissionRuleValue{{ToolName: "ReviewProbe"}},
	}}
	reg.SetRuntimeContextProvider(runtime)

	result, err := reg.CheckToolPermissions(context.Background(), "ReviewProbe", map[string]any{"path": "docs"}, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("CheckToolPermissions: %v", err)
	}
	if result.RuntimeSnapshot == nil || result.ToolMetadata != metadata {
		t.Fatalf("review context = runtime %#v metadata %#v", result.RuntimeSnapshot, result.ToolMetadata)
	}

	runtime.ctx.AllowedDirs[0] = "/workspace/changed"
	runtime.ctx.Features["review"] = false
	runtime.ctx.AskRules[0].ToolName = "ChangedProbe"
	if got := result.RuntimeSnapshot; got.AllowedDirs[0] != "/workspace/project" || !got.Features["review"] || got.AskRules[0].ToolName != "ReviewProbe" {
		t.Fatalf("permission review context followed later runtime mutation: %#v", *got)
	}
}

type metadataFallbackProbe struct{}

func (*metadataFallbackProbe) Name() string        { return "Read" }
func (*metadataFallbackProbe) Description() string { return "metadata fallback probe" }
func (*metadataFallbackProbe) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (*metadataFallbackProbe) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}
func (*metadataFallbackProbe) IsReadOnly() bool       { return true }
func (*metadataFallbackProbe) IsConcurrentSafe() bool { return true }

func TestToolMetadataDoesNotInferFromNameOrOptionalInterfaces(t *testing.T) {
	reg := New()
	reg.Register(&metadataFallbackProbe{})

	if got := reg.ToolMetadata("Read", nil); got != (types.ToolMetadata{}) {
		t.Fatalf("undeclared metadata = %+v, want zero value", got)
	}
}

type inputAwareMetadataTool struct {
	mockTool
	metadata func(map[string]any) types.ToolMetadata
}

func (t *inputAwareMetadataTool) ToolMetadata(input map[string]any) types.ToolMetadata {
	return t.metadata(input)
}
