package engine

import (
	"context"
	"testing"
	"time"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestCoreEngineSetProviderInvalidatesContinuationState(t *testing.T) {
	const (
		ciphertext = "provider-bound-ciphertext"
		itemID     = "provider-bound-item"
	)
	first := &mockProvider{
		name: "openai", modelID: "gpt-5.6-sol",
		responses: [][]types.StreamEvent{{
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeThinking, ID: itemID,
				SignatureKind: types.ThinkingSignatureOpenAIEncryptedReasoning, SignatureModel: "gpt-5.6-sol",
			}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
				Type: "signature_delta", ID: itemID, Signature: ciphertext,
				SignatureKind: types.ThinkingSignatureOpenAIEncryptedReasoning, SignatureModel: "gpt-5.6-sol",
				ProviderStatus: "completed",
			}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageStop, ResponseID: "provider-bound-response"},
		}},
	}
	eng, err := New(Config{Provider: first, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := eng.Query(context.Background(), QueryRequest{SessionID: "provider-switch", Message: "first"})
	if err != nil {
		t.Fatal(err)
	}
	_ = drainEvents(t, stream, 5*time.Second)

	key := eng.currentConversationKey("provider-switch")
	eng.convsMu.RLock()
	conv := eng.convs[key]
	eng.convsMu.RUnlock()
	if conv == nil {
		t.Fatal("conversation was not created")
	}
	before := conv.ql.Messages()
	assertContinuationPresence(t, before, true)

	second := &mockProvider{name: "anthropic", modelID: "claude-test"}
	eng.SetProvider(second)
	after := conv.ql.Messages()
	assertContinuationPresence(t, after, false)

	stream, err = eng.Query(context.Background(), QueryRequest{SessionID: "provider-switch", Message: "second"})
	if err != nil {
		t.Fatal(err)
	}
	_ = drainEvents(t, stream, 5*time.Second)
	if second.lastParams.PreviousResponseID != "" {
		t.Fatalf("provider switch retained response chain %q", second.lastParams.PreviousResponseID)
	}
	assertContinuationPresence(t, second.lastParams.Messages, false)
}

func TestCoreEngineSetProviderWaitsForInFlightContinuationMutation(t *testing.T) {
	reasoningAccepted := make(chan struct{})
	finishStream := make(chan struct{})
	first := &mockProvider{name: "openai", modelID: "gpt-5.6-sol"}
	first.defaultFn = func(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
		stream := make(chan types.StreamEvent)
		go func() {
			defer close(stream)
			for _, event := range []types.StreamEvent{
				{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
					Type: types.ContentTypeThinking, ID: "in-flight-item",
					SignatureKind: types.ThinkingSignatureOpenAIEncryptedReasoning, SignatureModel: "gpt-5.6-sol",
				}},
				{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
					Type: "signature_delta", ID: "in-flight-item", Signature: "in-flight-cipher",
					SignatureKind: types.ThinkingSignatureOpenAIEncryptedReasoning, SignatureModel: "gpt-5.6-sol",
				}},
				{Type: types.EventContentBlockStop, Index: 0},
			} {
				select {
				case stream <- event:
				case <-ctx.Done():
					return
				}
			}
			close(reasoningAccepted)
			select {
			case <-finishStream:
			case <-ctx.Done():
				return
			}
			stream <- types.StreamEvent{Type: types.EventMessageStop, ResponseID: "in-flight-response"}
		}()
		return stream, nil
	}
	eng, err := New(Config{Provider: first, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := eng.Query(context.Background(), QueryRequest{SessionID: "in-flight-switch", Message: "first"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-reasoningAccepted:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not emit in-flight continuation")
	}

	second := &mockProvider{name: "anthropic", modelID: "claude-test"}
	switched := make(chan struct{})
	go func() {
		eng.SetProvider(second)
		close(switched)
	}()
	select {
	case <-switched:
		t.Fatal("provider switch completed before the in-flight mutation committed")
	case <-time.After(50 * time.Millisecond):
	}
	if eng.Provider() != first {
		t.Fatal("new provider was published while the old query was still in flight")
	}

	close(finishStream)
	_ = drainEvents(t, stream, 5*time.Second)
	select {
	case <-switched:
	case <-time.After(5 * time.Second):
		t.Fatal("provider switch did not finish after the in-flight query")
	}
	if eng.Provider() != second {
		t.Fatal("new provider was not published")
	}
	eng.convsMu.RLock()
	conv := eng.convs[eng.currentConversationKey("in-flight-switch")]
	eng.convsMu.RUnlock()
	assertContinuationPresence(t, conv.ql.Messages(), false)
}

func assertContinuationPresence(t *testing.T, messages []types.Message, want bool) {
	t.Helper()
	found := false
	for _, message := range messages {
		for _, content := range message.Content {
			thinking, ok := content.(types.ThinkingBlock)
			if !ok {
				continue
			}
			if thinking.Signature != "" || thinking.SignatureKind != "" || thinking.SignatureModel != "" || thinking.ProviderItemID != "" || thinking.ProviderStatus != "" {
				found = true
			}
		}
	}
	if found != want {
		t.Fatalf("continuation state presence = %v, want %v: %#v", found, want, messages)
	}
}
