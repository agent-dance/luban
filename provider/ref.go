package provider

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/agent-dance/luban/types"
)

// ProviderRef is a thread-safe, swappable Provider reference.
// It implements the Provider and CapabilityProvider interfaces as a
// transparent proxy, so all existing code can receive a *ProviderRef
// anywhere a Provider is expected — zero call-site changes needed.
//
// The provider can be atomically replaced at runtime via Swap().
// All method calls are forwarded to the current underlying provider.
// Provider replacement takes effect between queries; an in-flight
// CreateStream call uses the provider snapshot taken at call time.
type ProviderRef struct {
	mu            sync.RWMutex
	current       Provider
	onChange      []func(Provider)
	debugObserver DebugObserver
	serviceTier   ServiceTier
	debugSequence atomic.Uint64
}

// NewProviderRef creates a ProviderRef wrapping the given initial provider.
func NewProviderRef(p Provider) *ProviderRef {
	return &ProviderRef{current: p}
}

// Get returns the current underlying provider.
func (r *ProviderRef) Get() Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}

// Swap atomically replaces the underlying provider and notifies all
// registered onChange listeners. Returns the previous provider.
func (r *ProviderRef) Swap(p Provider) Provider {
	r.mu.Lock()
	old := r.current
	r.current = p
	listeners := make([]func(Provider), len(r.onChange))
	copy(listeners, r.onChange)
	r.mu.Unlock()

	// Notify listeners outside the lock to avoid deadlocks.
	for _, fn := range listeners {
		fn(p)
	}

	return old
}

// OnChange registers a callback that fires after each Swap().
// Callbacks run outside the lock and receive the new provider.
func (r *ProviderRef) OnChange(fn func(Provider)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = append(r.onChange, fn)
}

// SetDebugObserver installs or clears the observer used for complete LLM
// request/response snapshots. It is safe to call while requests are in flight;
// each request retains the observer that was active when it started.
func (r *ProviderRef) SetDebugObserver(observer DebugObserver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.debugObserver = observer
}

// SetServiceTier installs a process-wide request policy for calls routed
// through this reference. Explicit per-call values remain visible to
// validation, while omitted values inherit the pinned tier. This covers
// compaction, goal evaluation, web helpers, and subagents that share the ref.
func (r *ProviderRef) SetServiceTier(serviceTier ServiceTier) {
	r.mu.Lock()
	r.serviceTier = serviceTier
	r.mu.Unlock()
}

// ApplyRequestPolicy projects process-wide request policy onto a parameter
// snapshot without replacing an explicit per-call value. Callers that must
// retain a Provider returned by Get can use this before invoking that exact
// provider, avoiding policy bypass during auxiliary generations.
func (r *ProviderRef) ApplyRequestPolicy(params Params) Params {
	r.mu.RLock()
	serviceTier := r.serviceTier
	r.mu.RUnlock()
	if params.ServiceTier == "" {
		params.ServiceTier = serviceTier
	}
	return params
}

// Close releases the current provider transport. The application calls this
// only after engines and background consumers have drained.
func (r *ProviderRef) Close() error {
	p := r.Get()
	if closer, ok := p.(CloseProvider); ok {
		return closer.Close()
	}
	return nil
}

// ── Provider interface ──────────────────────────────────────────────────────

// Name delegates to the current provider.
func (r *ProviderRef) Name() string {
	return r.Get().Name()
}

// ModelID delegates to the current provider.
func (r *ProviderRef) ModelID() string {
	return r.Get().ModelID()
}

// APIFormat exposes the snapshotted provider's wire protocol when available.
func (r *ProviderRef) APIFormat() string {
	p := r.Get()
	if formatted, ok := p.(interface{ APIFormat() string }); ok {
		return formatted.APIFormat()
	}
	return ""
}

func (r *ProviderRef) UnsupportedRequestFields(model string) []string {
	p := r.Get()
	if observable, ok := p.(interface{ UnsupportedRequestFields(string) []string }); ok {
		return observable.UnsupportedRequestFields(model)
	}
	return nil
}

// CreateStream delegates to the current provider.
// The provider is snapshotted at the start of the call, so a concurrent
// Swap() does not affect an in-flight stream.
func (r *ProviderRef) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	r.mu.RLock()
	p := r.current
	observer := r.debugObserver
	r.mu.RUnlock()
	params = r.ApplyRequestPolicy(params)
	if err := ValidateParams(p, params); err != nil {
		return nil, err
	}
	if observer == nil {
		return p.CreateStream(ctx, params)
	}

	id := r.debugSequence.Add(1)
	call := debugCallContextFrom(ctx)
	model := params.Model
	if model == "" {
		model = p.ModelID()
	}
	notifyDebugObserver(observer, DebugEvent{
		ID:       id,
		Phase:    DebugPhaseRequest,
		Kind:     call.kind,
		Provider: p.Name(),
		Model:    model,
		Metadata: call.metadata,
		Request:  newDebugRequest(params, model),
	})

	stream, err := p.CreateStream(ctx, params)
	if err != nil {
		notifyDebugObserver(observer, DebugEvent{
			ID:       id,
			Phase:    DebugPhaseResponse,
			Kind:     call.kind,
			Provider: p.Name(),
			Model:    model,
			Metadata: call.metadata,
			Response: newDebugResponse(nil, err.Error()),
		})
		return nil, err
	}

	forwarded := make(chan types.StreamEvent)
	go func() {
		defer close(forwarded)
		events := make([]types.StreamEvent, 0, 32)
		responseError := ""
		for event := range stream {
			events = append(events, event)
			select {
			case forwarded <- event:
			case <-ctx.Done():
				responseError = ctx.Err().Error()
				notifyDebugObserver(observer, DebugEvent{
					ID:       id,
					Phase:    DebugPhaseResponse,
					Kind:     call.kind,
					Provider: p.Name(),
					Model:    model,
					Metadata: call.metadata,
					Response: newDebugResponse(events, responseError),
				})
				return
			}
		}
		notifyDebugObserver(observer, DebugEvent{
			ID:       id,
			Phase:    DebugPhaseResponse,
			Kind:     call.kind,
			Provider: p.Name(),
			Model:    model,
			Metadata: call.metadata,
			Response: newDebugResponse(events, responseError),
		})
	}()
	return forwarded, nil
}

func notifyDebugObserver(observer DebugObserver, event DebugEvent) {
	defer func() {
		_ = recover()
	}()
	observer(event)
}

// ── CapabilityProvider interface ─────────────────────────────────────────────

// Capabilities delegates to the current provider if it implements
// CapabilityProvider, otherwise returns zero-value capabilities.
func (r *ProviderRef) Capabilities() ProviderCapabilities {
	p := r.Get()
	if cp, ok := p.(CapabilityProvider); ok {
		return cp.Capabilities()
	}
	return ProviderCapabilities{}
}

// Compile-time interface checks.
var (
	_ Provider           = (*ProviderRef)(nil)
	_ CapabilityProvider = (*ProviderRef)(nil)
	_ CloseProvider      = (*ProviderRef)(nil)
)
