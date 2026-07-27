package loop

import (
	"context"
	"errors"
	"testing"

	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

func TestStreamIdleTimeoutPreservesUsageAndContentFreeDisposition(t *testing.T) {
	wantUsage := types.Usage{
		InputTokens:          321,
		OutputTokens:         7,
		CacheReadInputTokens: 256,
	}
	providerStream := make(chan types.StreamEvent, 5)
	providerStream <- types.StreamEvent{
		Type:  types.EventMessageStart,
		Usage: &types.Usage{InputTokens: wantUsage.InputTokens, CacheReadInputTokens: wantUsage.CacheReadInputTokens},
	}
	providerStream <- types.StreamEvent{
		Type:         types.EventContentBlockStart,
		Index:        0,
		ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
	}
	providerStream <- types.StreamEvent{
		Type:  types.EventContentBlockDelta,
		Index: 0,
		Delta: &types.ContentDelta{Type: "text_delta", Text: "uncommitted"},
	}
	providerStream <- types.StreamEvent{
		Type:  types.EventError,
		Usage: &types.Usage{OutputTokens: wantUsage.OutputTokens},
		Error: &types.APIError{Type: "stream_idle_timeout", Message: "external diagnostic must not enter metadata"},
	}
	close(providerStream)

	_, usage, _, err := (&QueryLoop{}).processStream(context.Background(), providerStream, 1, func(streamevent.Event) {})
	var partial *PartialStreamError
	if !errors.As(err, &partial) {
		t.Fatalf("processStream error = %v, want PartialStreamError", err)
	}
	if usage == nil || *usage != wantUsage {
		t.Fatalf("failed attempt usage = %#v, want %#v", usage, wantUsage)
	}

	var tombstone streamevent.Event
	emitUncommittedProviderResponseTombstone(func(event streamevent.Event) {
		tombstone = event
	}, providerAttemptIdentity{provider: "openai", model: "gpt-5.6-sol", requestID: "request-id"}, partial, 1)
	if tombstone.Type != streamevent.EventTombstone || tombstone.Tombstone == nil {
		t.Fatalf("tombstone = %#v", tombstone)
	}
	if got := tombstone.Metadata["disposition"]; got != "stream_idle_timeout" {
		t.Fatalf("disposition = %#v, want stream_idle_timeout", got)
	}
	for key, value := range tombstone.Metadata {
		if value == "external diagnostic must not enter metadata" {
			t.Fatalf("raw provider error leaked through metadata key %q", key)
		}
	}
}

func TestProviderFailureDispositionBoundsUnknownExternalType(t *testing.T) {
	err := &types.APIError{Type: "provider supplied arbitrary secret-like value", Message: "raw message"}
	if got := providerFailureDisposition(err); got != "provider_error" {
		t.Fatalf("disposition = %q, want bounded provider_error", got)
	}
}
