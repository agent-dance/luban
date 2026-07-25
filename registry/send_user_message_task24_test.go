package registry_test

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
	toolinteraction "github.com/agent-dance/luban/internal/tools/interaction"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type task24RuntimeProvider struct{ runtime types.ToolRuntimeContext }

func (p task24RuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext { return p.runtime }

func TestToolVisibilitySendUserMessageAlwaysLoaded(t *testing.T) {
	tool := toolinteraction.NewSendUserMessageTool(func() string { return t.TempDir() })
	metadata := registry.DiscoveryMetadata(tool)
	if !metadata.AlwaysLoad {
		t.Fatal("SendUserMessage must remain always loaded when enabled")
	}
	if metadata.SearchHint != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageDiscoveryHint) {
		t.Fatalf("search hint = %q", metadata.SearchHint)
	}
	if registry.IsDeferredTool(tool) {
		t.Fatal("SendUserMessage must not require ToolSearch discovery")
	}
}

func TestToolVisibilitySendUserMessageRequiresRuntimeFeature(t *testing.T) {
	t.Setenv("LUBAN_CODE_SEND_USER_MESSAGE", "true")
	reg := registry.New()
	tool := toolinteraction.NewSendUserMessageTool(func() string { return t.TempDir() })
	reg.Register(tool)
	reg.SetRuntimeContextProvider(task24RuntimeProvider{runtime: types.ToolRuntimeContext{
		Features: map[string]bool{types.ToolFeatureSendUserMessage: false},
	}})
	if reg.IsToolEnabled(tool) {
		t.Fatal("tool-local environment fallback must not override runtime feature opt-out")
	}
	reg.SetRuntimeContextProvider(task24RuntimeProvider{runtime: types.ToolRuntimeContext{
		Features: map[string]bool{types.ToolFeatureSendUserMessage: true},
	}})
	if !reg.IsToolEnabled(tool) {
		t.Fatal("runtime feature opt-in must enable SendUserMessage")
	}
}
