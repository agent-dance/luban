package main

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
)

type webFetchRegistryProvider struct {
	mu     sync.Mutex
	params provider.Params
	text   string
}

func (p *webFetchRegistryProvider) Name() string    { return "anthropic" }
func (p *webFetchRegistryProvider) ModelID() string { return "claude-haiku-test" }
func (p *webFetchRegistryProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	p.params = params
	text := p.text
	p.mu.Unlock()
	stream := make(chan types.StreamEvent, 2)
	stream <- types.StreamEvent{Type: types.EventContentBlockDelta, Delta: &types.ContentDelta{Type: "text_delta", Text: text}}
	stream <- types.StreamEvent{Type: types.EventMessageStop}
	close(stream)
	return stream, nil
}

func TestSetupRegistryWebFetchUsesTypedCacheAndDynamicSummariser(t *testing.T) {
	ref := provider.NewProviderRef(nil)
	deps := SetupRegistry(ref, t.TempDir(), nil, sandbox.NoopBackend{}, &WebDomainConfig{SkipWebFetchPreflight: true})
	t.Cleanup(deps.StopWebFetchCache)
	if deps.WebFetchTool == nil || deps.WebFetchCache == nil {
		t.Fatalf("registry did not expose WebFetch lifecycle dependencies: %+v", deps)
	}
	registered, ok := deps.Registry.Get("WebFetch").(*tools.WebFetchTool)
	if !ok || registered != deps.WebFetchTool || registered.FetchCache() != deps.WebFetchCache {
		t.Fatalf("registered WebFetch does not share typed cache")
	}
	if !registered.SkipWebFetchPreflight {
		t.Fatal("skipWebFetchPreflight setting was not wired")
	}
	if _, err := tools.RunWebFetchSummariser(context.Background(), registered.Summariser, "https://example.com", "p", "body", false); err == nil {
		t.Fatal("empty ProviderRef must surface unavailable secondary model")
	}

	fake := &webFetchRegistryProvider{text: "dynamic summary"}
	ref.Swap(fake)
	got, err := tools.RunWebFetchSummariser(context.Background(), registered.Summariser, "https://example.com", "extract", "body", false)
	if err != nil || got != "dynamic summary" {
		t.Fatalf("dynamic provider summary=%q err=%v", got, err)
	}
	fake.mu.Lock()
	params := fake.params
	fake.mu.Unlock()
	if !strings.Contains(params.Model, "haiku") || params.MaxTokens != tools.WebFetchSummariserMaxTokens || len(params.Messages) != 1 ||
		!strings.Contains(params.Messages[0].GetText(), "Web page content:") {
		t.Fatalf("secondary model params mismatch: %+v", params)
	}

	key := deps.WebFetchCache.MakeKey("https://example.com")
	deps.WebFetchCache.Set(key, tools.WebFetchCacheEntry{Body: "cached", Bytes: 6})
	deps.ClearWebFetchCache()
	if deps.WebFetchCache.Len() != 0 {
		t.Fatal("registry clear-cache lifecycle did not clear WebFetch state")
	}
}

func TestWebFetchSmallFastModelHonorsManagedOverride(t *testing.T) {
	t.Setenv("ANTHROPIC_SMALL_FAST_MODEL", "managed-small-model")
	if got := webFetchSmallFastModel(&webFetchRegistryProvider{}); got != "managed-small-model" {
		t.Fatalf("small-fast override=%q", got)
	}
}

func TestSetupRegistryWebFetchPreflightDefaultsOn(t *testing.T) {
	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	t.Cleanup(deps.StopWebFetchCache)
	if !tools.PreflightDomainInfoEnabled(deps.WebFetchTool) {
		t.Fatal("production WebFetch must enable domain_info by default")
	}
}
