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

func TestVisibilityRuntimeGatesTaskFamiliesAndDeniedTools(t *testing.T) {
	reg := New()
	for _, name := range []string{"Read", "TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "TodoWrite", "TeamCreate", "SendMessage", "WebSearch", "ToolSearch"} {
		reg.Register(&mockTool{name: name})
	}
	runtime := &runtimeContextStub{ctx: types.ToolRuntimeContext{
		Interactive: true,
		Features: map[string]bool{
			types.ToolFeatureTaskV2:     true,
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
	for _, name := range []string{"Read", "TodoWrite", "TeamCreate", "SendMessage", "ToolSearch"} {
		if visible[name] {
			t.Errorf("expected %s to be hidden", name)
		}
	}

	runtime.ctx.Features[types.ToolFeatureTaskV2] = false
	visible = toolNames(reg.VisibleTools(nil))
	if !visible["TodoWrite"] {
		t.Fatal("TodoWrite should become visible when task-v2 is disabled")
	}
	if visible["TaskCreate"] {
		t.Fatal("TaskCreate should become hidden when task-v2 is disabled")
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
		t.Fatal("deferred Read must remain in the enabled SDK/runtime tool pool")
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
	if visible["Read"] {
		t.Fatal("deferred Read must stay out of the model-visible pool until loaded")
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
	reg.Register(&mockTool{name: "Read"})
	reg.Register(&mockTool{name: "Write"})
	reg.Register(&mockTool{name: "Grep"})
	reg.Register(&mockTool{name: "ExitWorktree"})

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
