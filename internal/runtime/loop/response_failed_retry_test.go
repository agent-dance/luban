package loop

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/i18n"
	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type unclassifiedResponseFailedProvider struct {
	calls atomic.Int32
}

func (*unclassifiedResponseFailedProvider) Name() string    { return "response-failed" }
func (*unclassifiedResponseFailedProvider) ModelID() string { return "test-model" }

func (p *unclassifiedResponseFailedProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	if p.calls.Add(1) == 1 {
		stream := make(chan types.StreamEvent, 1)
		stream <- types.StreamEvent{Type: types.EventError, Error: &types.APIError{
			Type:         "response_failed",
			Message:      "opaque upstream diagnostic",
			Stage:        types.ProviderErrorStageStream,
			ReplaySafety: types.ProviderReplaySafe,
		}}
		close(stream)
		return stream, nil
	}
	stream := make(chan types.StreamEvent, 8)
	for _, event := range textEvents("recovered") {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func TestUnclassifiedResponseFailedSafelyReplaysAndPublishesRetryStatus(t *testing.T) {
	providerInstance := &unclassifiedResponseFailedProvider{}
	queryLoop := New(providerInstance, registry.New(), Config{MaxTurns: 1, MaxTokens: 1024})

	var retries []streamevent.RequestStatusEvent
	if err := queryLoop.Run(context.Background(), "request", func(event streamevent.Event) {
		if event.Type == streamevent.EventRequestRetry && event.RequestStatus != nil {
			retries = append(retries, *event.RequestStatus)
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls := providerInstance.calls.Load(); calls != 2 {
		t.Fatalf("provider calls = %d, want one failed attempt and one safe replay", calls)
	}
	if len(retries) != 1 {
		t.Fatalf("retry status events = %+v, want one transparent retry", retries)
	}
	status := retries[0]
	if status.Attempt != 1 || status.RetryCount != 1 || status.MaxRetries != 5 || status.Error == "" {
		t.Fatalf("retry status = %+v, want attempt 1/5 with cause", status)
	}
	if status.RetryKind != "stream" {
		t.Fatalf("retry kind = %q, want stream", status.RetryKind)
	}
}

type streamTransportFallbackProvider struct {
	calls    atomic.Int32
	fallback atomic.Bool
}

func (*streamTransportFallbackProvider) Name() string    { return "transport-fallback" }
func (*streamTransportFallbackProvider) ModelID() string { return "test-model" }

func (p *streamTransportFallbackProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.calls.Add(1)
	stream := make(chan types.StreamEvent, 8)
	if !p.fallback.Load() {
		stream <- types.StreamEvent{Type: types.EventError, Error: &types.APIError{
			Type:         "stream_interrupted",
			Message:      "connection closed",
			Stage:        types.ProviderErrorStageStream,
			Class:        types.ProviderErrorClassTransport,
			ReplaySafety: types.ProviderReplaySafe,
		}}
		close(stream)
		return stream, nil
	}
	for _, event := range textEvents("recovered over HTTPS") {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func (p *streamTransportFallbackProvider) TryFallbackTransport() (from, to string, activated bool) {
	if !p.fallback.CompareAndSwap(false, true) {
		return "", "", false
	}
	return "WebSocket", "HTTPS", true
}

func TestStreamRetryExhaustionFallsBackTransportAndRecovers(t *testing.T) {
	providerInstance := &streamTransportFallbackProvider{}
	queryLoop := New(providerInstance, registry.New(), Config{MaxTurns: 1, MaxTokens: 1024})

	var retries []streamevent.RequestStatusEvent
	var fallbackWarnings int
	if err := queryLoop.Run(context.Background(), "request", func(event streamevent.Event) {
		if event.Type == streamevent.EventRequestRetry && event.RequestStatus != nil {
			retries = append(retries, *event.RequestStatus)
		}
		if event.Type == streamevent.EventSystemWarning && event.RuntimeEvent != nil &&
			event.RuntimeEvent.PublicKey == i18n.KeyRuntimeStreamTransportFallback {
			fallbackWarnings++
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if calls := providerInstance.calls.Load(); calls != 7 {
		t.Fatalf("provider calls = %d, want six WebSocket attempts and one HTTPS attempt", calls)
	}
	if !providerInstance.fallback.Load() {
		t.Fatal("transport fallback was not activated")
	}
	if len(retries) != 5 {
		t.Fatalf("stream retries = %+v, want five visible reconnects", retries)
	}
	for index, status := range retries {
		if status.Attempt != index+1 || status.MaxRetries != 5 || status.RetryKind != "stream" {
			t.Fatalf("retry[%d] = %+v, want visible stream reconnect %d/5", index, status, index+1)
		}
	}
	if fallbackWarnings != 1 {
		t.Fatalf("fallback warnings = %d, want one durable transition", fallbackWarnings)
	}
}
