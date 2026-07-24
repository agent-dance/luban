package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/types"
)

type promptRawCaller struct {
	t      *testing.T
	calls  []promptRawCall
	result map[string]any
}

type promptRawCall struct {
	method string
	params any
}

func (f *promptRawCaller) CallRaw(_ context.Context, method string, params any, out any) error {
	f.calls = append(f.calls, promptRawCall{method: method, params: params})
	data, err := json.Marshal(f.result[method])
	if err != nil {
		f.t.Fatal(err)
	}
	return json.Unmarshal(data, out)
}

func TestMCPPromptGetSendsNameAndZippedArguments(t *testing.T) {
	raw := &promptRawCaller{
		t: t,
		result: map[string]any{
			"prompts/get": GetPromptResult{
				Messages: []PromptMessage{{Role: "user", Content: PromptContent{Type: "text", Text: "ok"}}},
			},
		},
	}
	client := NewClient(raw, nil)
	args := PromptArgumentsFromString([]string{"owner", "repo", "issue"}, "anthropic claude-code")
	result, err := client.GetPrompt(context.Background(), "review", args)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages = %#v", result.Messages)
	}
	if len(raw.calls) != 1 || raw.calls[0].method != "prompts/get" {
		t.Fatalf("calls = %#v", raw.calls)
	}
	params := raw.calls[0].params.(map[string]any)
	if params["name"] != "review" {
		t.Fatalf("name param = %#v", params["name"])
	}
	gotArgs := params["arguments"].(map[string]string)
	wantArgs := map[string]string{"owner": "anthropic", "repo": "claude-code", "issue": ""}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("arguments = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestMCPPromptDescriptorsGateOnConnectedPromptsCapability(t *testing.T) {
	states := []MCPServerConnection{
		{
			Name:         "my server",
			Type:         MCPStateConnected,
			Capabilities: ServerCapabilities{"prompts": map[string]any{"listChanged": true}},
			Prompts: []PromptDefinition{{
				Name:        "review issue",
				Description: "Review an issue",
				Arguments: []PromptArgument{
					{Name: "owner", Required: true},
					{Name: "repo"},
				},
			}},
		},
		{
			Name:         "no-prompts",
			Type:         MCPStateConnected,
			Capabilities: ServerCapabilities{"tools": map[string]any{}},
			Prompts:      []PromptDefinition{{Name: "hidden"}},
		},
		{
			Name:         "needs-auth",
			Type:         MCPStateNeedsAuth,
			Capabilities: ServerCapabilities{"prompts": map[string]any{}},
			Prompts:      []PromptDefinition{{Name: "hidden"}},
		},
	}

	descriptors := PromptCommandDescriptorsFromConnections(states)
	if len(descriptors) != 1 {
		t.Fatalf("descriptors = %#v", descriptors)
	}
	got := descriptors[0]
	if got.Name != "mcp__my_server__review_issue" {
		t.Fatalf("name = %q", got.Name)
	}
	if got.PromptName != "review issue" || got.ServerName != "my server" {
		t.Fatalf("raw identifiers not preserved: %#v", got)
	}
	if got.ArgumentHint != "<owner> [repo]" {
		t.Fatalf("argument hint = %q", got.ArgumentHint)
	}
	if !reflect.DeepEqual(got.ArgumentNames, []string{"owner", "repo"}) {
		t.Fatalf("arg names = %#v", got.ArgumentNames)
	}
	if !reflect.DeepEqual(got.RequiredArguments, []string{"owner"}) {
		t.Fatalf("required args = %#v", got.RequiredArguments)
	}
}

func TestMCPPromptMessagesPreserveRichContentSemantics(t *testing.T) {
	messages := []PromptMessage{
		{Role: "user", Content: PromptContent{Type: "text", Text: "hello"}},
		{Role: "assistant", Content: PromptContent{Type: "image", Data: "aW1n", MimeType: "image/png"}},
		{Role: "user", Content: PromptContent{Type: "resource", Resource: &ResourceContent{URI: "file://a.txt", Text: "body", MimeType: "text/plain"}}},
		{Role: "user", Content: PromptContent{Type: "resource_link", Name: "Spec", URI: "file://spec.md", Description: "design"}},
	}
	out := TransformPromptMessages(messages, "srv")
	if len(out) != 4 {
		t.Fatalf("messages = %#v", out)
	}
	if out[0].Role != types.RoleUser || out[0].Content[0].(types.TextBlock).Text != "hello" {
		t.Fatalf("text message = %#v", out[0])
	}
	img := out[1].Content[0].(types.ImageBlock)
	if out[1].Role != types.RoleAssistant || img.Source == nil || img.Source.MediaType != "image/png" || img.Source.Data != "aW1n" {
		t.Fatalf("image message = %#v", out[1])
	}
	if got := out[2].Content[0].(types.TextBlock).Text; got != "[Resource from srv at file://a.txt] body" {
		t.Fatalf("resource text = %q", got)
	}
	if got := out[3].Content[0].(types.TextBlock).Text; got != "[Resource link: Spec] file://spec.md (design)" {
		t.Fatalf("resource link text = %q", got)
	}
}

func TestMCPPromptCacheInvalidationClearsCachedPromptsAndRunsHook(t *testing.T) {
	manager := NewManager()
	manager.AddConfig("srv", MCPServerConfig{Type: TransportStdio, Command: "fake"})
	manager.cache.setPrompts("srv", &ListPromptsResult{Prompts: []PromptDefinition{{Name: "old"}}})
	manager.mu.Lock()
	state := manager.states["srv"]
	state.Type = MCPStateConnected
	state.Capabilities = ServerCapabilities{"prompts": map[string]any{}}
	state.Prompts = []PromptDefinition{{Name: "old"}}
	manager.setStateLocked(state)
	manager.mu.Unlock()

	var invalidated []string
	remove := RegisterPromptCacheInvalidationHook(func(serverName string) {
		invalidated = append(invalidated, serverName)
	})
	defer remove()

	manager.InvalidatePromptCache("srv")
	if _, ok := manager.Cache().Prompts("srv"); ok {
		t.Fatal("expected prompt cache to be cleared")
	}
	state, ok := manager.State("srv")
	if !ok || len(state.Prompts) != 0 {
		t.Fatalf("state prompts = %#v", state.Prompts)
	}
	if !reflect.DeepEqual(invalidated, []string{"srv"}) {
		t.Fatalf("hooks = %#v", invalidated)
	}
}
