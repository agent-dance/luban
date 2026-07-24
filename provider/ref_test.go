package provider

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/types"
)

// stubProvider is a minimal Provider for testing ProviderRef.
type stubProvider struct {
	name    string
	modelID string
	caps    ProviderCapabilities
}

func (s *stubProvider) Name() string    { return s.name }
func (s *stubProvider) ModelID() string { return s.modelID }
func (s *stubProvider) CreateStream(_ context.Context, _ Params) (<-chan types.StreamEvent, error) {
	ch := make(chan types.StreamEvent)
	close(ch)
	return ch, nil
}
func (s *stubProvider) Capabilities() ProviderCapabilities { return s.caps }

// Compile-time check that stubProvider satisfies both interfaces.
var (
	_ Provider           = (*stubProvider)(nil)
	_ CapabilityProvider = (*stubProvider)(nil)
)

func TestProviderRef_InterfaceCompliance(t *testing.T) {
	p := &stubProvider{name: "test", modelID: "m1"}
	ref := NewProviderRef(p)

	// ProviderRef must satisfy Provider.
	var _ Provider = ref
	// ProviderRef must satisfy CapabilityProvider.
	var _ CapabilityProvider = ref
}

func TestProviderRef_Delegates(t *testing.T) {
	p := &stubProvider{name: "alpha", modelID: "model-a"}
	ref := NewProviderRef(p)

	if ref.Name() != "alpha" {
		t.Fatalf("Name() = %q, want %q", ref.Name(), "alpha")
	}
	if ref.ModelID() != "model-a" {
		t.Fatalf("ModelID() = %q, want %q", ref.ModelID(), "model-a")
	}
}

func TestProviderRef_Get(t *testing.T) {
	p := &stubProvider{name: "a", modelID: "m"}
	ref := NewProviderRef(p)

	got := ref.Get()
	if got != p {
		t.Fatal("Get() did not return the initial provider")
	}
}

func TestProviderRef_Swap(t *testing.T) {
	p1 := &stubProvider{name: "first", modelID: "m1"}
	p2 := &stubProvider{name: "second", modelID: "m2"}
	ref := NewProviderRef(p1)

	old := ref.Swap(p2)
	if old != p1 {
		t.Fatal("Swap() should return the previous provider")
	}
	if ref.Name() != "second" {
		t.Fatalf("after Swap, Name() = %q, want %q", ref.Name(), "second")
	}
	if ref.ModelID() != "m2" {
		t.Fatalf("after Swap, ModelID() = %q, want %q", ref.ModelID(), "m2")
	}
}

func TestProviderRef_OnChange(t *testing.T) {
	p1 := &stubProvider{name: "first", modelID: "m1"}
	p2 := &stubProvider{name: "second", modelID: "m2"}
	ref := NewProviderRef(p1)

	var received Provider
	ref.OnChange(func(p Provider) { received = p })

	ref.Swap(p2)
	if received != p2 {
		t.Fatal("OnChange listener should receive the new provider")
	}
}

func TestProviderRef_OnChange_MultipleListeners(t *testing.T) {
	p1 := &stubProvider{name: "a", modelID: "m"}
	p2 := &stubProvider{name: "b", modelID: "m"}
	ref := NewProviderRef(p1)

	var count int32
	for i := 0; i < 5; i++ {
		ref.OnChange(func(p Provider) { atomic.AddInt32(&count, 1) })
	}

	ref.Swap(p2)
	if atomic.LoadInt32(&count) != 5 {
		t.Fatalf("expected 5 listener calls, got %d", count)
	}
}

func TestProviderRef_ConcurrentSwap(t *testing.T) {
	p := &stubProvider{name: "init", modelID: "m"}
	ref := NewProviderRef(p)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			np := &stubProvider{name: "g", modelID: "m"}
			ref.Swap(np)
			_ = ref.Name()
			_ = ref.ModelID()
			_ = ref.Get()
		}(i)
	}

	wg.Wait()

	// After all swaps, Get() should return a non-nil provider.
	if ref.Get() == nil {
		t.Fatal("Get() returned nil after concurrent swaps")
	}
}

func TestProviderRef_CreateStream_Snapshot(t *testing.T) {
	p1 := &stubProvider{name: "snap1", modelID: "m1"}
	ref := NewProviderRef(p1)

	// Call CreateStream — it should snapshot p1.
	stream, err := ref.CreateStream(context.Background(), Params{})
	if err != nil {
		t.Fatalf("CreateStream error: %v", err)
	}
	// Drain the (closed) channel.
	for range stream {
	}

	// Swap to p2 — the previous CreateStream call shouldn't be affected.
	p2 := &stubProvider{name: "snap2", modelID: "m2"}
	ref.Swap(p2)

	if ref.Name() != "snap2" {
		t.Fatal("after swap, ref should delegate to new provider")
	}
}

func TestProviderRef_Capabilities_WithCapabilityProvider(t *testing.T) {
	caps := ProviderCapabilities{
		MaxContext: 200000,
		Thinking:  true,
	}
	p := &stubProvider{name: "c", modelID: "m", caps: caps}
	ref := NewProviderRef(p)

	got := ref.Capabilities()
	if got.MaxContext != 200000 {
		t.Fatalf("MaxContext = %d, want 200000", got.MaxContext)
	}
	if !got.Thinking {
		t.Fatal("Thinking should be true")
	}
}

// plainProvider is a Provider that does NOT implement CapabilityProvider.
type plainProvider struct{}

func (p *plainProvider) Name() string    { return "plain" }
func (p *plainProvider) ModelID() string { return "pm" }
func (p *plainProvider) CreateStream(_ context.Context, _ Params) (<-chan types.StreamEvent, error) {
	ch := make(chan types.StreamEvent)
	close(ch)
	return ch, nil
}

func TestProviderRef_Capabilities_WithPlainProvider(t *testing.T) {
	ref := NewProviderRef(&plainProvider{})

	got := ref.Capabilities()
	if got.MaxContext != 0 {
		t.Fatalf("expected zero-value capabilities, got MaxContext=%d", got.MaxContext)
	}
}
