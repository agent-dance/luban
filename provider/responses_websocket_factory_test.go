package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/agent-dance/luban/types"
)

type webSocketFactoryCaptureProvider struct {
	*mockProvider
}

func (p *webSocketFactoryCaptureProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{ResponsesWebSocket: CapabilitySupported}
}

func TestRuntimeOverridesReachProviderFactoryExactly(t *testing.T) {
	registry := NewProviderRegistry()
	var captured Config
	registry.Register(ProviderInfo{Name: "openai"}, func(cfg Config, _ string) (Provider, error) {
		captured = cfg
		return &webSocketFactoryCaptureProvider{mockProvider: &mockProvider{name: "openai", results: []mockResult{{events: successEvents()}}}}, nil
	})

	created, err := newFromRegistryWithRuntimeOverrides(registry, "openai", "gpt-5.6-sol", RuntimeOverrides{
		APIFormat:          "responses",
		ResponsesWebSocket: CapabilitySupported,
	})
	if err != nil {
		t.Fatalf("newFromRegistryWithRuntimeOverrides: %v", err)
	}
	if created == nil || captured.APIFormat != "responses" || captured.ResponsesWebSocket != CapabilitySupported {
		t.Fatalf("captured factory config = %#v", captured)
	}
}

func TestOpenAIFactoryExplicitWebSocketFakeCanary(t *testing.T) {
	var upgrades atomic.Int32
	var httpRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !websocket.IsWebSocketUpgrade(request) {
			httpRequests.Add(1)
			writeResponsesWebSocketTestSSE(writer, "unexpected_http")
			return
		}
		upgrades.Add(1)
		connection, err := responsesWebSocketTestUpgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer connection.Close()
		var body map[string]any
		if err := connection.ReadJSON(&body); err != nil {
			t.Errorf("read response.create: %v", err)
			return
		}
		assertResponsesWebSocketTestEnvelope(t, body)
		if err := writeResponsesWebSocketTestCompleted(connection, "factory_ws_response", map[string]any{"input_tokens": 1, "output_tokens": 1}); err != nil {
			t.Errorf("write completion: %v", err)
		}
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registerOpenAI(registry)
	created, err := registry.Create("openai", Config{
		APIKey: "fake-key", BaseURL: server.URL, APIFormat: "responses",
		ResponsesWebSocket: CapabilitySupported,
	}, responsesWebSocketTestModel)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() {
		if err := created.(CloseProvider).Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	capabilities := created.(CapabilityProvider).Capabilities()
	if capabilities.ResponsesWebSocket != CapabilitySupported {
		t.Fatalf("factory WebSocket capability = %v", capabilities.ResponsesWebSocket)
	}

	stream, err := created.CreateStream(context.Background(), Params{
		Model:               responsesWebSocketTestModel,
		Messages:            []types.Message{types.UserMessage("fake canary")},
		ContinuationLineage: "factory-fake-canary",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	events := collectStreamEvents(stream)
	if !streamEventsContain(events, types.EventMessageStop) {
		t.Fatalf("fake canary did not complete: %#v", events)
	}
	if upgrades.Load() != 1 || httpRequests.Load() != 0 {
		t.Fatalf("transport counts: websocket=%d http=%d", upgrades.Load(), httpRequests.Load())
	}
}

func TestOpenAIFactoryWebSocketDefaultsOffAndUsesHTTP(t *testing.T) {
	var upgrades atomic.Int32
	var httpRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if websocket.IsWebSocketUpgrade(request) {
			upgrades.Add(1)
			http.Error(writer, "unexpected WebSocket upgrade", http.StatusBadRequest)
			return
		}
		httpRequests.Add(1)
		writeResponsesWebSocketTestSSE(writer, "factory_http_response")
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registerOpenAI(registry)
	created, err := registry.Create("openai", Config{
		APIKey: "fake-key", BaseURL: server.URL, APIFormat: "responses",
	}, responsesWebSocketTestModel)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = created.(CloseProvider).Close() }()
	if got := created.(CapabilityProvider).Capabilities().ResponsesWebSocket; got != CapabilityUnknown {
		t.Fatalf("default WebSocket capability = %v, want unknown", got)
	}

	stream, err := created.CreateStream(context.Background(), Params{
		Model:               responsesWebSocketTestModel,
		Messages:            []types.Message{types.UserMessage("default off")},
		ContinuationLineage: "factory-default-off",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	if events := collectStreamEvents(stream); !streamEventsContain(events, types.EventMessageStop) {
		t.Fatalf("HTTP default-off request did not complete: %#v", events)
	}
	if upgrades.Load() != 0 || httpRequests.Load() != 1 {
		t.Fatalf("transport counts: websocket=%d http=%d", upgrades.Load(), httpRequests.Load())
	}
}

func TestResponsesWebSocketFactoryRejectsNonPublicProfilesBeforeWire(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		config       Config
		register     func(*ProviderRegistry)
	}{
		{
			name:         "OpenAI Chat Completions",
			providerName: "openai",
			config:       Config{APIKey: "fake-key", APIFormat: "chat-completions", ResponsesWebSocket: CapabilitySupported},
			register:     registerOpenAI,
		},
		{
			name:         "OpenAI Codex OAuth and Lite",
			providerName: "openai",
			config:       Config{AuthToken: "fake-token", APIFormat: "responses", ResponsesWebSocket: CapabilitySupported},
			register:     registerOpenAI,
		},
		{
			name:         "ChatGPT Codex URL with API key",
			providerName: "openai",
			config: Config{
				APIKey: "fake-key", BaseURL: openAIChatGPTCodexBaseURL, APIFormat: "responses",
				ResponsesWebSocket: CapabilitySupported,
			},
			register: registerOpenAI,
		},
		{
			name:         "explicit compatible semantics",
			providerName: "openai",
			config:       Config{APIKey: "fake-key", APIFormat: "responses", ResponsesSemantics: ResponsesSemanticsCompatible, ResponsesWebSocket: CapabilitySupported},
			register:     registerOpenAI,
		},
		{
			name:         "negotiating custom gateway",
			providerName: "openai",
			config: Config{
				APIKey: "fake-key", BaseURL: "https://gateway.example/v1",
				ResponsesWebSocket: CapabilitySupported,
			},
			register: registerOpenAI,
		},
		{
			name:         "compatible provider",
			providerName: "gateway",
			config:       Config{APIKey: "fake-key", APIFormat: "responses", ResponsesWebSocket: CapabilitySupported},
			register: func(registry *ProviderRegistry) {
				registry.Register(ProviderInfo{Name: "gateway"}, func(Config, string) (Provider, error) {
					t.Fatal("compatible factory was invoked")
					return nil, nil
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := NewProviderRegistry()
			test.register(registry)
			if _, err := registry.Create(test.providerName, test.config, responsesWebSocketTestModel); err == nil {
				t.Fatal("unsupported WebSocket profile unexpectedly created a provider")
			}
		})
	}
}

func TestProviderRegistryRejectsFactoryThatDropsExplicitWebSocketCapability(t *testing.T) {
	registry := NewProviderRegistry()
	registry.Register(ProviderInfo{Name: "openai"}, func(Config, string) (Provider, error) {
		return &mockProvider{name: "openai", results: []mockResult{{events: successEvents()}}}, nil
	})
	if _, err := registry.Create("openai", Config{ResponsesWebSocket: CapabilitySupported}, responsesWebSocketTestModel); err == nil {
		t.Fatal("factory silently dropped explicit WebSocket capability")
	}
}

func streamEventsContain(events []types.StreamEvent, eventType types.StreamEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
