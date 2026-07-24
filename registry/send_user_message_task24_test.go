package registry_test

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
)

type task24RuntimeProvider struct{ runtime types.ToolRuntimeContext }

func (p task24RuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext { return p.runtime }

func TestToolVisibilitySendUserMessageAlwaysLoaded(t *testing.T) {
	tool := tools.NewSendUserMessageTool()
	metadata := registry.DiscoveryMetadata(tool)
	if !metadata.AlwaysLoad {
		t.Fatal("SendUserMessage must remain always loaded when Brief is enabled")
	}
	if !strings.Contains(metadata.SearchHint, "primary visible output channel") {
		t.Fatalf("search hint = %q", metadata.SearchHint)
	}
	if registry.IsDeferredTool(tool) {
		t.Fatal("SendUserMessage must not require ToolSearch discovery")
	}
}

func TestToolVisibilitySendUserMessageEnvironmentOverridesRuntimeOptOut(t *testing.T) {
	t.Setenv("CLAUDE_CODE_BRIEF", "true")
	reg := registry.New()
	tool := tools.NewSendUserMessageTool()
	reg.Register(tool)
	reg.SetRuntimeContextProvider(task24RuntimeProvider{runtime: types.ToolRuntimeContext{
		Features: map[string]bool{types.ToolFeatureBrief: false},
	}})
	if !reg.IsToolEnabled(tool) {
		t.Fatal("CLAUDE_CODE_BRIEF must override a stale runtime feature opt-out")
	}
}
