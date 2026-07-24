package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestDeepSeekCacheLineageSerializedAsUserID(t *testing.T) {
	request := captureOpenAIChatRequestTask10(t, Config{
		ProviderName: "deepseek",
		Model:        "deepseek-v4-flash",
	}, Params{
		System:         "stable system",
		Messages:       []types.Message{types.UserMessage("hello")},
		PromptCacheKey: "root-session-123",
		UsePromptCache: true,
	})
	if got := request["user_id"]; got != "root-session-123" {
		t.Fatalf("DeepSeek user_id = %#v, want inherited cache lineage", got)
	}
}

func TestDeepSeekCacheLineageWireBodyStableAcrossProviderInstances(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		mu.Lock()
		bodies = append(bodies, append([]byte(nil), body...))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_cache\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"deepseek-v4-flash\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	params := Params{
		System: "stable system",
		Tools: []types.ToolDefinition{{
			Name:        "Read",
			Description: "read a file",
			InputSchema: types.StrictObjectSchema(map[string]any{"file_path": map[string]any{"type": "string"}}, "file_path"),
		}},
		Messages:       []types.Message{types.UserMessage("hello")},
		PromptCacheKey: "shared-root-session",
		UsePromptCache: true,
	}
	for range 2 {
		client := NewOpenAI(Config{
			ProviderName: "deepseek",
			Model:        "deepseek-v4-flash",
			APIKey:       "test-key",
			BaseURL:      server.URL,
		})
		stream, err := client.CreateStream(context.Background(), params)
		if err != nil {
			t.Fatal(err)
		}
		for range stream {
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("captured bodies = %d, want 2", len(bodies))
	}
	if !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatalf("fresh DeepSeek providers serialized different wire bodies\nfirst:  %s\nsecond: %s", bodies[0], bodies[1])
	}
}

func TestDeepSeekCacheLineageNormalizesUnsupportedUserID(t *testing.T) {
	got := deepSeekCacheUserID("lineage/with spaces/和中文")
	if len(got) != len("cache_")+64 || got[:len("cache_")] != "cache_" {
		t.Fatalf("normalized DeepSeek user_id = %q", got)
	}
	for _, char := range got {
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_') {
			t.Fatalf("normalized DeepSeek user_id contains unsupported character %q: %q", char, got)
		}
	}
}

func TestDeepSeekCacheLineageOmittedWhenCacheDisabled(t *testing.T) {
	request := captureOpenAIChatRequestTask10(t, Config{
		ProviderName: "deepseek",
		Model:        "deepseek-v4-flash",
	}, Params{
		Messages:       []types.Message{types.UserMessage("hello")},
		PromptCacheKey: "root-session-123",
		UsePromptCache: false,
	})
	if got, found := request["user_id"]; found {
		t.Fatalf("DeepSeek user_id = %#v while prompt cache is disabled", got)
	}
}

func TestDeepSeekChatUsesDocumentedGenerationFields(t *testing.T) {
	request := captureOpenAIChatRequestTask10(t, Config{
		ProviderName: "deepseek",
		Model:        "deepseek-v4-flash",
	}, Params{
		Messages:        []types.Message{types.UserMessage("hello")},
		MaxTokens:       321,
		ReasoningEffort: "high",
	})
	if got := request["max_tokens"]; got != float64(321) {
		t.Fatalf("DeepSeek max_tokens = %#v, want 321", got)
	}
	if got, found := request["max_completion_tokens"]; found {
		t.Fatalf("DeepSeek sent undocumented max_completion_tokens = %#v", got)
	}
	if got := request["reasoning_effort"]; got != "high" {
		t.Fatalf("DeepSeek reasoning_effort = %#v, want high", got)
	}
	thinking, ok := request["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("DeepSeek default thinking control = %#v, want explicitly disabled", request["thinking"])
	}

	enabled := captureOpenAIChatRequestTask10(t, Config{
		ProviderName: "deepseek",
		Model:        "deepseek-v4-flash",
	}, Params{
		Messages: []types.Message{types.UserMessage("hello")},
		Thinking: &ThinkingConfig{Enabled: true},
	})
	thinking, ok = enabled["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("DeepSeek enabled thinking control = %#v", enabled["thinking"])
	}
}

func TestDeepSeekNativeReasoningAndSystemFingerprint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_reasoning\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"deepseek-v4-flash\",\"system_fingerprint\":\"fp_cache_test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"check constraints\"},\"finish_reason\":null}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_reasoning\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"deepseek-v4-flash\",\"system_fingerprint\":\"fp_cache_test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewOpenAI(Config{ProviderName: "deepseek", Model: "deepseek-v4-flash", APIKey: "test-key", BaseURL: server.URL})
	stream, err := client.CreateStream(context.Background(), Params{Messages: []types.Message{types.UserMessage("hello")}})
	if err != nil {
		t.Fatal(err)
	}
	var events []types.StreamEvent
	for event := range stream {
		events = append(events, event)
	}
	var thinking, text string
	var thinkingStarts, thinkingStops int
	for _, event := range events {
		if event.Type == types.EventContentBlockStart && event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeThinking {
			thinkingStarts++
		}
		if event.Type == types.EventContentBlockDelta && event.Delta != nil {
			thinking += event.Delta.Thinking
			text += event.Delta.Text
		}
		if event.Type == types.EventContentBlockStop && event.Index == 0 {
			thinkingStops++
		}
	}
	if thinking != "check constraints" || text != "answer" || thinkingStarts != 1 || thinkingStops != 1 {
		t.Fatalf("DeepSeek reasoning events: thinking=%q text=%q starts=%d stops=%d events=%#v", thinking, text, thinkingStarts, thinkingStops, events)
	}
	if len(events) == 0 || events[len(events)-1].Type != types.EventMessageStop || events[len(events)-1].SystemFingerprint != "fp_cache_test" {
		t.Fatalf("DeepSeek terminal fingerprint missing: %#v", events)
	}
}
