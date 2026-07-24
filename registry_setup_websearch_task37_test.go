package main

import (
	"context"
	"sync"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
)

type webSearchRegistryProvider struct {
	mu     sync.Mutex
	params []provider.Params
}

func (p *webSearchRegistryProvider) Name() string    { return "anthropic" }
func (p *webSearchRegistryProvider) ModelID() string { return "claude-sonnet-4-6" }
func (p *webSearchRegistryProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	p.params = append(p.params, params)
	p.mu.Unlock()
	stream := make(chan types.StreamEvent, 8)
	stream <- types.StreamEvent{Type: types.EventMessageStart, Usage: &types.Usage{InputTokens: 10}}
	stream <- types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeServerToolUse, ID: "srv_1", Name: "web_search"}}
	stream <- types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"query":"golang docs"}`}}
	stream <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
	stream <- types.StreamEvent{Type: types.EventContentBlockStart, Index: 1, ContentBlock: &types.ContentDelta{Type: types.ContentTypeWebSearchToolResult, ToolUseID: "srv_1", RawJSON: []byte(`{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[{"type":"web_search_result","title":"Go","url":"https://go.dev"}]}`)}}
	stream <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 1}
	stream <- types.StreamEvent{Type: types.EventMessageDelta, Usage: &types.Usage{OutputTokens: 3, ServerToolUse: types.ServerToolUsage{WebSearchRequests: 1}}}
	stream <- types.StreamEvent{Type: types.EventMessageStop}
	close(stream)
	return stream, nil
}

func TestSetupRegistryWebSearchUsesDynamicProviderNativeAdapter(t *testing.T) {
	fake := &webSearchRegistryProvider{}
	ref := provider.NewProviderRef(fake)
	deps := SetupRegistry(ref, t.TempDir(), nil, sandbox.NoopBackend{}, &WebDomainConfig{AllowedDomains: []string{"go.dev"}})
	t.Cleanup(deps.StopWebFetchCache)
	registered, ok := deps.Registry.Get("WebSearch").(*tools.WebSearchTool)
	if !ok || registered != deps.WebSearchTool || !registered.HasWebSearchServerTool() {
		t.Fatalf("production WebSearch is not wired to provider native adapter: %T", deps.Registry.Get("WebSearch"))
	}
	result, err := registered.Execute(context.Background(), map[string]any{"query": "golang", "allowed_domains": []any{"go.dev"}})
	if err != nil || result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	fake.mu.Lock()
	params := fake.params[len(fake.params)-1]
	fake.mu.Unlock()
	if len(params.ExtraToolSchemas) != 1 {
		t.Fatalf("extra schemas = %+v", params.ExtraToolSchemas)
	}
	schema := params.ExtraToolSchemas[0]
	if schema.Type != tools.WebSearchServerToolName || schema.Name != "web_search" || schema.MaxUses != 8 || len(schema.AllowedDomains) != 1 {
		t.Fatalf("server schema = %+v", schema)
	}
}

func TestSetupRegistryWebSearchVisibilityTracksProviderReadiness(t *testing.T) {
	ref := provider.NewProviderRef(nil)
	deps := SetupRegistry(ref, t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	t.Cleanup(deps.StopWebFetchCache)
	tool := deps.Registry.Get("WebSearch")
	if deps.Registry.IsToolEnabled(tool) {
		t.Fatal("WebSearch must be hidden until a supported provider is active")
	}
	ref.Swap(&registrySetupReadProvider{name: "openai", model: "gpt-5.5"})
	if !deps.Registry.IsToolEnabled(tool) {
		t.Fatal("WebSearch must use its local fallback for OpenAI")
	}
	ref.Swap(&registrySetupReadProvider{name: "vertex", model: "claude-sonnet-3-7"})
	if !deps.Registry.IsToolEnabled(tool) {
		t.Fatal("WebSearch must use its local fallback for pre-Claude-4 Vertex models")
	}
	ref.Swap(&registrySetupReadProvider{name: "vertex", model: "claude-sonnet-4-6"})
	if !deps.Registry.IsToolEnabled(tool) {
		t.Fatal("WebSearch must be visible for supported Vertex Claude 4 models")
	}
}
