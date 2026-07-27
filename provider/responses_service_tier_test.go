package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/agent-dance/luban/types"
)

func TestResponsesHTTPWireCarriesExplicitDefaultServiceTier(t *testing.T) {
	rawRequests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := io.ReadAll(request.Body)
		rawRequests <- append([]byte(nil), raw...)
		writeServiceTierSSE(writer, "resp_http_service_tier", "default")
	}))
	defer server.Close()

	responses := newServiceTierWireProvider(server.URL, CapabilityUnsupported)
	stream, err := responses.CreateStream(context.Background(), Params{
		Messages:    []types.Message{types.UserMessage("hello")},
		ServiceTier: ServiceTierDefault,
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	assertServiceTierStreamSucceeded(t, collectStreamEvents(stream))

	raw := <-rawRequests
	if !bytes.Contains(raw, []byte(`"service_tier":"default"`)) {
		t.Fatalf("raw HTTP request omitted explicit default service tier: %s", raw)
	}
	var request map[string]any
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("decode raw HTTP request: %v", err)
	}
	if request["service_tier"] != "default" {
		t.Fatalf("HTTP service_tier = %#v, want default", request["service_tier"])
	}
}

func TestResponsesHTTPWireDoesNotCanonicalizeOmittedServiceTier(t *testing.T) {
	rawRequests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := io.ReadAll(request.Body)
		rawRequests <- append([]byte(nil), raw...)
		writeServiceTierSSE(writer, "resp_http_omitted_service_tier", "")
	}))
	defer server.Close()

	responses := newServiceTierWireProvider(server.URL, CapabilityUnsupported)
	stream, err := responses.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	assertServiceTierStreamSucceeded(t, collectStreamEvents(stream))

	raw := <-rawRequests
	var request map[string]any
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("decode raw HTTP request: %v", err)
	}
	if value, exists := request["service_tier"]; exists {
		t.Fatalf("omitted service tier was canonicalized on HTTP wire: %#v", value)
	}
}

func TestResponsesRejectsAutoServiceTierBeforeHTTPWire(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeServiceTierSSE(writer, "unexpected", "auto")
	}))
	defer server.Close()

	responses := newServiceTierWireProvider(server.URL, CapabilityUnsupported)
	if _, err := responses.CreateStream(context.Background(), Params{
		Messages:    []types.Message{types.UserMessage("hello")},
		ServiceTier: ServiceTier("auto"),
	}); err == nil {
		t.Fatal("auto service tier unexpectedly reached request construction")
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("HTTP requests = %d, want 0 for invalid auto tier", got)
	}
}

func TestResponsesHTTPRejectsActualServiceTierDrift(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeServiceTierSSE(writer, "resp_http_service_tier_drift", "auto")
	}))
	defer server.Close()

	responses := newServiceTierWireProvider(server.URL, CapabilityUnsupported)
	stream, err := responses.CreateStream(context.Background(), Params{
		Messages:    []types.Message{types.UserMessage("hello")},
		ServiceTier: ServiceTierDefault,
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	events := collectStreamEvents(stream)
	for _, event := range events {
		if event.Type == types.EventError && event.Error != nil && event.Error.Type == "service_tier_mismatch" {
			return
		}
	}
	t.Fatalf("response tier drift did not emit service_tier_mismatch: %#v", events)
}

func TestResponsesWebSocketWireCarriesExplicitDefaultServiceTier(t *testing.T) {
	rawRequests := make(chan []byte, 1)
	serverErrors := make(chan error, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			serverErrors <- err
			return
		}
		defer connection.Close()
		messageType, raw, err := connection.ReadMessage()
		if err != nil {
			serverErrors <- err
			return
		}
		if messageType != websocket.TextMessage {
			serverErrors <- fmt.Errorf("WebSocket message type = %d, want text", messageType)
			return
		}
		rawRequests <- append([]byte(nil), raw...)
		if err := connection.WriteJSON(map[string]any{
			"type":     "response.created",
			"response": map[string]any{"id": "resp_ws_service_tier", "model": "gpt-5.6-sol"},
		}); err != nil {
			serverErrors <- err
			return
		}
		if err := connection.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id": "resp_ws_service_tier", "model": "gpt-5.6-sol", "status": "completed",
				"service_tier": "default",
				"usage":        map[string]any{"input_tokens": 1, "output_tokens": 1},
				"output":       []any{},
			},
		}); err != nil {
			serverErrors <- err
		}
	}))
	defer server.Close()

	responses := newServiceTierWireProvider(server.URL, CapabilitySupported)
	defer responses.Close()
	stream, err := responses.CreateStream(context.Background(), Params{
		Messages:            []types.Message{types.UserMessage("hello")},
		ServiceTier:         ServiceTierDefault,
		ContinuationLineage: "service-tier-wire-lineage",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	assertServiceTierStreamSucceeded(t, collectStreamEvents(stream))

	select {
	case serverErr := <-serverErrors:
		t.Fatalf("WebSocket server: %v", serverErr)
	default:
	}
	raw := <-rawRequests
	if !bytes.Contains(raw, []byte(`"service_tier":"default"`)) {
		t.Fatalf("raw WebSocket request omitted explicit default service tier: %s", raw)
	}
	var request map[string]any
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("decode raw WebSocket request: %v", err)
	}
	if request["type"] != "response.create" || request["service_tier"] != "default" {
		t.Fatalf("WebSocket request envelope = %#v", request)
	}
}

func newServiceTierWireProvider(baseURL string, webSocket CapabilitySupport) *ResponsesProvider {
	return NewResponses(Config{
		ProviderName:       "openai",
		ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
		ResponsesWebSocket: webSocket,
		APIKey:             "test-key",
		BaseURL:            baseURL,
		Model:              "gpt-5.6-sol",
	})
}

func writeServiceTierSSE(writer http.ResponseWriter, responseID, serviceTier string) {
	response := map[string]any{
		"id": responseID, "model": "gpt-5.6-sol", "status": "completed",
		"usage":  map[string]any{"input_tokens": 1, "output_tokens": 1},
		"output": []any{},
	}
	if serviceTier != "" {
		response["service_tier"] = serviceTier
	}
	created, _ := json.Marshal(map[string]any{
		"type":     "response.created",
		"response": map[string]any{"id": responseID, "model": "gpt-5.6-sol"},
	})
	completed, _ := json.Marshal(map[string]any{"type": "response.completed", "response": response})
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(writer, "event: response.created\ndata: %s\n\nevent: response.completed\ndata: %s\n\n", created, completed)
}

func assertServiceTierStreamSucceeded(t *testing.T, events []types.StreamEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type == types.EventError {
			t.Fatalf("service tier stream error: %#v", event.Error)
		}
	}
}
