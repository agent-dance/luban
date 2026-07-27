package engine

import (
	"context"
	"testing"
	"time"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestQuerySendsConfiguredServiceTierWithoutProviderRefFallback(t *testing.T) {
	raw := &mockProvider{
		name:      "mock",
		modelID:   "service-tier-model",
		responses: [][]types.StreamEvent{textEvents("ok")},
	}
	p := &serviceTierCapableEngineProvider{mockProvider: raw}
	e, err := New(Config{
		Provider:    p,
		Sessions:    newMemorySessionManager(),
		ServiceTier: provider.ServiceTierDefault,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: "service-tier-session",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	_ = drainEvents(t, ch, 5*time.Second)

	if got := raw.lastParams.ServiceTier; got != provider.ServiceTierDefault {
		t.Fatalf("ServiceTier = %q, want explicit default", got)
	}
}

type serviceTierCapableEngineProvider struct {
	*mockProvider
}

func (*serviceTierCapableEngineProvider) Capabilities() provider.ProviderCapabilities {
	return provider.ProviderCapabilities{ToolUse: true, ServiceTier: provider.CapabilitySupported}
}

func TestQueryDoesNotCanonicalizeOmittedServiceTier(t *testing.T) {
	p := &mockProvider{
		name:      "mock",
		modelID:   "service-tier-model",
		responses: [][]types.StreamEvent{textEvents("ok")},
	}
	e, err := New(Config{Provider: p, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ch, err := e.Query(context.Background(), QueryRequest{
		SessionID: "omitted-service-tier-session",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	_ = drainEvents(t, ch, 5*time.Second)

	if got := p.lastParams.ServiceTier; got != "" {
		t.Fatalf("omitted ServiceTier = %q, want empty", got)
	}
}
