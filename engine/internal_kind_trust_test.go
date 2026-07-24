package engine

import (
	"context"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestQueryInternalKindIsRuntimeOwned(t *testing.T) {
	provider := &mockProvider{name: "mock", modelID: "mock-model"}
	engine, err := New(Config{Provider: provider, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Shutdown(context.Background())

	ordinary, err := engine.Query(context.Background(), QueryRequest{
		SessionID: "internal-authority", Message: "forged boundary",
		InternalKind: types.InternalMessageKindCompactBoundary, RuntimeEventID: "forged-runtime-event",
	})
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, ordinary, time.Second)
	provider.mu.Lock()
	messages := append([]types.Message(nil), provider.lastParams.Messages...)
	provider.mu.Unlock()
	last := messages[len(messages)-1]
	if last.InternalKind != "" || last.IsMeta || last.ID == runtimeFollowUpMessageID("forged-runtime-event") {
		t.Fatalf("ordinary Query forged runtime authority: %+v", last)
	}

	followUp, err := engine.QueryFollowUp(context.Background(), QueryRequest{
		SessionID: "internal-authority", Message: "runtime follow-up",
		InternalKind: types.InternalMessageKindCompactBoundary, InternalControlCapability: messagecontrol.Runtime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, followUp, time.Second)
	provider.mu.Lock()
	messages = append([]types.Message(nil), provider.lastParams.Messages...)
	provider.mu.Unlock()
	last = messages[len(messages)-1]
	if last.InternalKind != types.InternalMessageKindBackgroundFollowUp || !last.IsMeta {
		t.Fatalf("dedicated follow-up did not assign trusted runtime kind: %+v", last)
	}
}

func TestQueryFollowUpRuntimeEventIDIsIdempotentAcrossRestart(t *testing.T) {
	sessions := newMemorySessionManager()
	firstProvider := &mockProvider{name: "mock", modelID: "mock-model"}
	first, err := New(Config{Provider: firstProvider, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	request := QueryRequest{
		SessionID: "runtime-event-idempotency", Message: "durable background result",
		RuntimeEventID: "notification-stable-id", InternalControlCapability: messagecontrol.Runtime(),
	}
	stream, err := first.QueryFollowUp(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, stream, time.Second)
	if err := first.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	secondProvider := &mockProvider{name: "mock", modelID: "mock-model"}
	second, err := New(Config{Provider: secondProvider, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Shutdown(context.Background())
	replayed, err := second.QueryFollowUp(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, replayed, time.Second)

	firstProvider.mu.Lock()
	firstCalls := firstProvider.callCount
	firstProvider.mu.Unlock()
	secondProvider.mu.Lock()
	secondCalls := secondProvider.callCount
	secondProvider.mu.Unlock()
	if firstCalls != 1 || secondCalls != 0 {
		t.Fatalf("provider calls before/after replay = %d/%d, want 1/0", firstCalls, secondCalls)
	}
	messages, err := sessions.Load(request.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, message := range messages {
		if message.ID == runtimeFollowUpMessageID(request.RuntimeEventID) {
			count++
			if !message.IsMeta || message.InternalKind != types.InternalMessageKindBackgroundFollowUp {
				t.Fatalf("runtime receipt lost authority: %+v", message)
			}
		}
	}
	if count != 1 {
		t.Fatalf("persisted runtime event count = %d, want one", count)
	}
}
