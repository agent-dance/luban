package provider

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/types"
)

type serviceTierCaptureProvider struct {
	calls []Params
}

func (*serviceTierCaptureProvider) Name() string    { return "service-tier-capture" }
func (*serviceTierCaptureProvider) ModelID() string { return "service-tier-model" }
func (*serviceTierCaptureProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{ServiceTier: CapabilitySupported}
}
func (p *serviceTierCaptureProvider) CreateStream(_ context.Context, params Params) (<-chan types.StreamEvent, error) {
	p.calls = append(p.calls, params)
	ch := make(chan types.StreamEvent)
	close(ch)
	return ch, nil
}

func TestProviderRefServiceTierFallbackDoesNotOverwriteExplicitValues(t *testing.T) {
	p := &serviceTierCaptureProvider{}
	ref := NewProviderRef(p)
	ref.SetServiceTier(ServiceTierDefault)

	stream, err := ref.CreateStream(context.Background(), Params{ServiceTier: ServiceTierDefault})
	if err != nil {
		t.Fatalf("explicit default CreateStream: %v", err)
	}
	for range stream {
	}
	stream, err = ref.CreateStream(context.Background(), Params{})
	if err != nil {
		t.Fatalf("inherited default CreateStream: %v", err)
	}
	for range stream {
	}
	if len(p.calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(p.calls))
	}
	for index, call := range p.calls {
		if call.ServiceTier != ServiceTierDefault {
			t.Fatalf("provider call %d ServiceTier = %q, want default", index, call.ServiceTier)
		}
	}

	if _, err := ref.CreateStream(context.Background(), Params{ServiceTier: ServiceTier("auto")}); err == nil {
		t.Fatal("explicit auto service tier was overwritten instead of rejected")
	}
	if len(p.calls) != 2 {
		t.Fatalf("invalid auto reached provider; calls = %d, want 2", len(p.calls))
	}
}

func TestProviderRefApplyRequestPolicyCoversSnapshottedAuxiliaryCalls(t *testing.T) {
	p := &serviceTierCaptureProvider{}
	ref := NewProviderRef(p)
	ref.SetServiceTier(ServiceTierDefault)

	params := ref.ApplyRequestPolicy(Params{Model: p.ModelID()})
	if params.ServiceTier != ServiceTierDefault {
		t.Fatalf("policy ServiceTier = %q, want default", params.ServiceTier)
	}
	explicit := ref.ApplyRequestPolicy(Params{Model: p.ModelID(), ServiceTier: ServiceTier("auto")})
	if explicit.ServiceTier != ServiceTier("auto") {
		t.Fatalf("explicit ServiceTier = %q, want auto", explicit.ServiceTier)
	}
}

func TestProviderRefOmittedServiceTierStaysEmptyWithoutPolicy(t *testing.T) {
	p := &serviceTierCaptureProvider{}
	ref := NewProviderRef(p)
	ref.SetServiceTier("")

	stream, err := ref.CreateStream(context.Background(), Params{})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	if len(p.calls) != 1 || p.calls[0].ServiceTier != "" {
		t.Fatalf("provider calls = %#v, want one omitted service tier", p.calls)
	}
}
