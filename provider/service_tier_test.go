package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/agent-dance/luban/types"
)

func TestResponsesHTTPServiceTierPinAndMismatch(t *testing.T) {
	for _, test := range []struct {
		name         string
		responseTier string
		wantMismatch bool
	}{
		{name: "exact default", responseTier: "default"},
		{name: "different response tier", responseTier: "flex", wantMismatch: true},
		{name: "missing response tier", wantMismatch: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			var requestBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				raw, _ := io.ReadAll(request.Body)
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				mu.Lock()
				requestBody = body
				mu.Unlock()
				writePinnedServiceTierSSE(writer, test.responseTier)
			}))
			defer server.Close()

			responses := NewResponses(Config{
				ProviderName: "openai", ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
				APIKey: "test-key", BaseURL: server.URL, Model: responsesWebSocketTestModel,
			})
			stream, err := responses.CreateStream(context.Background(), Params{
				Model: responsesWebSocketTestModel, ServiceTier: ServiceTierDefault,
				Messages: []types.Message{types.UserMessage("pin scheduling")},
			})
			if err != nil {
				t.Fatalf("CreateStream: %v", err)
			}
			events := collectStreamEvents(stream)
			mu.Lock()
			body := requestBody
			mu.Unlock()
			if body["service_tier"] != "default" || body["stream"] != true || body["store"] != false {
				t.Fatalf("HTTP request contract = %#v", body)
			}
			assertServiceTierTerminal(t, events, test.wantMismatch)
		})
	}
}

func TestResponsesWebSocketServiceTierPinAndMismatch(t *testing.T) {
	for _, test := range []struct {
		name         string
		responseTier string
		wantMismatch bool
	}{
		{name: "exact default", responseTier: "default"},
		{name: "different response tier", responseTier: "priority", wantMismatch: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			requestBodies := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				connection, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(writer, request, nil)
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
				requestBodies <- body
				if err := connection.WriteJSON(map[string]any{
					"type": "response.created", "response": map[string]any{
						"id": "resp_service_tier", "model": responsesWebSocketTestModel,
					},
				}); err != nil {
					t.Errorf("write created: %v", err)
					return
				}
				if err := connection.WriteJSON(serviceTierCompletedEvent(test.responseTier)); err != nil {
					t.Errorf("write completed: %v", err)
				}
			}))
			defer server.Close()

			responses := newResponsesWebSocketTestProvider(server.URL, "service-tier-key")
			defer responses.Close()
			stream, err := responses.CreateStream(context.Background(), Params{
				Model: responsesWebSocketTestModel, ServiceTier: ServiceTierDefault,
				Messages:            []types.Message{types.UserMessage("pin websocket scheduling")},
				ContinuationLineage: "service-tier-" + test.name,
			})
			if err != nil {
				t.Fatalf("CreateStream: %v", err)
			}
			events := collectStreamEvents(stream)
			body := <-requestBodies
			if body["service_tier"] != "default" || body["type"] != "response.create" || body["store"] != false {
				t.Fatalf("WebSocket request contract = %#v", body)
			}
			if _, exists := body["stream"]; exists {
				t.Fatalf("WebSocket request unexpectedly included stream: %#v", body)
			}
			assertServiceTierTerminal(t, events, test.wantMismatch)
			if test.wantMismatch {
				session := responses.responsesWebSocketSession("service-tier-" + test.name)
				session.mu.Lock()
				committedID := session.lastResponseID
				session.mu.Unlock()
				if committedID != "" {
					t.Fatalf("mismatched tier committed response chain %q", committedID)
				}
			}
		})
	}
}

func TestProviderRefInjectsPinnedServiceTierIntoAuxiliaryRequest(t *testing.T) {
	capture := &auxiliaryServiceTierCaptureProvider{}
	ref := NewProviderRef(capture)
	ref.SetServiceTier(ServiceTierDefault)
	stream, err := ref.CreateStream(context.Background(), Params{
		Model: "gpt-5.6-sol", Messages: []types.Message{types.UserMessage("auxiliary request")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	if capture.params.ServiceTier != ServiceTierDefault {
		t.Fatalf("injected ServiceTier = %q, want default", capture.params.ServiceTier)
	}
}

func TestServiceTierRequiresExplicitEndpointCapability(t *testing.T) {
	for _, test := range []struct {
		name    string
		params  Params
		caps    ProviderCapabilities
		wantErr bool
	}{
		{name: "default supported", params: Params{ServiceTier: ServiceTierDefault}, caps: ProviderCapabilities{ServiceTier: CapabilitySupported}},
		{name: "default unknown", params: Params{ServiceTier: ServiceTierDefault}, wantErr: true},
		{name: "default unsupported", params: Params{ServiceTier: ServiceTierDefault}, caps: ProviderCapabilities{ServiceTier: CapabilityUnsupported}, wantErr: true},
		{name: "auto rejected even when endpoint claims support", params: Params{ServiceTier: ServiceTier("auto")}, caps: ProviderCapabilities{ServiceTier: CapabilitySupported}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &capMockProvider{name: "service-tier-test", caps: test.caps}
			err := ValidateParams(provider, test.params)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateParams() error = %v, wantErr=%v", err, test.wantErr)
			}
		})
	}
}

type auxiliaryServiceTierCaptureProvider struct {
	params Params
}

func (p *auxiliaryServiceTierCaptureProvider) Name() string { return "capture" }
func (p *auxiliaryServiceTierCaptureProvider) ModelID() string {
	return responsesWebSocketTestModel
}
func (p *auxiliaryServiceTierCaptureProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{ServiceTier: CapabilitySupported}
}
func (p *auxiliaryServiceTierCaptureProvider) CreateStream(_ context.Context, params Params) (<-chan types.StreamEvent, error) {
	p.params = params
	stream := make(chan types.StreamEvent)
	close(stream)
	return stream, nil
}

func writePinnedServiceTierSSE(writer http.ResponseWriter, responseTier string) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	completed, _ := json.Marshal(serviceTierCompletedEvent(responseTier))
	_, _ = io.WriteString(writer, buildSSEStream([]sseEvent{
		{Type: "response.created", Data: fmt.Sprintf(`{"type":"response.created","response":{"id":"resp_service_tier","model":%q}}`, responsesWebSocketTestModel)},
		{Type: "response.completed", Data: string(completed)},
	}))
}

func serviceTierCompletedEvent(responseTier string) map[string]any {
	response := map[string]any{
		"id": "resp_service_tier", "model": responsesWebSocketTestModel, "status": "completed",
		"usage": map[string]any{"input_tokens": 9, "output_tokens": 2}, "output": []any{},
	}
	if responseTier != "" {
		response["service_tier"] = responseTier
	}
	return map[string]any{"type": "response.completed", "response": response}
}

func assertServiceTierTerminal(t *testing.T, events []types.StreamEvent, wantMismatch bool) {
	t.Helper()
	sawStop := false
	sawMismatch := false
	var usage *types.Usage
	for _, event := range events {
		if event.Type == types.EventMessageStop {
			sawStop = true
		}
		if event.Type == types.EventError && event.Error != nil && event.Error.Type == "service_tier_mismatch" {
			sawMismatch = true
		}
		if event.Usage != nil {
			usage = event.Usage
		}
	}
	if wantMismatch {
		if sawStop || !sawMismatch {
			t.Fatalf("mismatch terminal: sawStop=%v sawMismatch=%v events=%#v", sawStop, sawMismatch, events)
		}
		if usage == nil || usage.InputTokens != 9 || usage.OutputTokens != 2 {
			t.Fatalf("mismatch lost billable usage: %#v", usage)
		}
		return
	}
	if !sawStop || sawMismatch {
		t.Fatalf("matching terminal: sawStop=%v sawMismatch=%v events=%#v", sawStop, sawMismatch, events)
	}
}
