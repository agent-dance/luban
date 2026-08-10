package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/agent-dance/luban/types"
)

const (
	responsesWebSocketTestModel        = "gpt-5.6-sol"
	responsesWebSocketTestEncrypted    = "opaque-ws-encrypted-reasoning"
	responsesWebSocketTestRawMarker    = "private-ws-continuation-marker"
	responsesWebSocketTestVisibleThink = "checked constraints"
)

var responsesWebSocketTestUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

func TestResponsesWebSocketFallbackPermanentlySelectsHTTPS(t *testing.T) {
	responses := NewResponses(Config{
		ProviderName:       "openai",
		BaseURL:            "https://api.openai.com/v1",
		ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
		ResponsesWebSocket: CapabilitySupported,
	})
	if got := responses.Capabilities().ResponsesWebSocket; got != CapabilitySupported {
		t.Fatalf("initial WebSocket capability = %v", got)
	}
	from, to, activated := TryFallbackTransport(NewRetryProvider(responses, DefaultRetryConfig()))
	if !activated || from != "WebSocket" || to != "HTTPS" {
		t.Fatalf("fallback = %q -> %q, activated=%t", from, to, activated)
	}
	if got := responses.Capabilities().ResponsesWebSocket; got != CapabilityUnsupported {
		t.Fatalf("post-fallback WebSocket capability = %v, want unsupported", got)
	}
	if responses.snapshotRequestProfile().webSocketEligible(Params{ContinuationLineage: "lineage"}) {
		t.Fatal("post-fallback request remained WebSocket eligible")
	}
	if _, _, activated := responses.TryFallbackTransport(); activated {
		t.Fatal("fallback activated more than once")
	}
}

type responsesWebSocketTestRecord struct {
	connection    int
	turn          int
	authorization string
	body          map[string]any
}

func TestResponsesWebSocketCapabilityFailClosed(t *testing.T) {
	var upgradeAttempts atomic.Int32
	var httpRequests atomic.Int32
	liteHeaders := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if websocket.IsWebSocketUpgrade(request) {
			upgradeAttempts.Add(1)
			http.Error(writer, "unexpected WebSocket handshake", http.StatusBadRequest)
			return
		}
		httpRequests.Add(1)
		liteHeaders <- request.Header.Get("x-openai-internal-codex-responses-lite")
		writeResponsesWebSocketTestSSE(writer, "resp_http")
	}))
	defer server.Close()

	tests := []struct {
		name           string
		config         Config
		wantCapability CapabilitySupport
		wantLite       bool
	}{
		{
			name: "public endpoint defaults to unknown",
			config: Config{
				ProviderName: "openai", ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
				APIKey: "test-key", BaseURL: server.URL, Model: responsesWebSocketTestModel,
			},
			wantCapability: CapabilityUnknown,
		},
		{
			name: "compatible endpoint rejects explicit assertion",
			config: Config{
				ProviderName: "compatible", ResponsesSemantics: ResponsesSemanticsCompatible,
				ResponsesWebSocket: CapabilitySupported,
				APIKey:             "test-key", BaseURL: server.URL, Model: responsesWebSocketTestModel,
			},
			wantCapability: CapabilityUnsupported,
		},
		{
			name: "Codex Lite rejects explicit assertion",
			config: Config{
				ProviderName: "openai", ResponsesSemantics: ResponsesSemanticsOpenAICodex,
				ResponsesWebSocket: CapabilitySupported,
				AuthToken:          "test-token", BaseURL: server.URL, Model: responsesWebSocketTestModel,
			},
			wantCapability: CapabilityUnsupported,
			wantLite:       true,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := NewResponses(test.config)
			defer responses.Close()
			if got := responses.Capabilities().ResponsesWebSocket; got != test.wantCapability {
				t.Fatalf("ResponsesWebSocket capability = %v, want %v", got, test.wantCapability)
			}
			stream, err := responses.CreateStream(context.Background(), Params{
				Model:               responsesWebSocketTestModel,
				Messages:            []types.Message{types.UserMessage(fmt.Sprintf("request-%d", index))},
				ContinuationLineage: fmt.Sprintf("fail-closed-%d", index),
			})
			if err != nil {
				t.Fatalf("CreateStream: %v", err)
			}
			for range stream {
			}
			if got := <-liteHeaders; (got == "true") != test.wantLite {
				t.Fatalf("Responses Lite header = %q, wantLite=%v", got, test.wantLite)
			}
		})
	}
	if got := upgradeAttempts.Load(); got != 0 {
		t.Fatalf("WebSocket upgrade attempts = %d, want 0", got)
	}
	if got := httpRequests.Load(); got != int32(len(tests)) {
		t.Fatalf("HTTP requests = %d, want %d", got, len(tests))
	}
}

func TestResponsesWebSocketContinuationUsesSameConnection(t *testing.T) {
	records := make(chan responsesWebSocketTestRecord, 2)
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connectionID := int(connections.Add(1))
		connection, err := responsesWebSocketTestUpgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer connection.Close()
		for turn := 1; turn <= 2; turn++ {
			var body map[string]any
			if err := connection.ReadJSON(&body); err != nil {
				t.Errorf("read response.create turn %d: %v", turn, err)
				return
			}
			records <- responsesWebSocketTestRecord{
				connection: connectionID, turn: turn,
				authorization: request.Header.Get("Authorization"), body: body,
			}
			if turn == 1 {
				if err := writeResponsesWebSocketTestReasoningTurn(
					connection, "resp_ws_1", responsesWebSocketTestEncrypted, responsesWebSocketTestRawMarker,
					map[string]any{"input_tokens": 80, "output_tokens": 12},
				); err != nil {
					t.Errorf("write first turn: %v", err)
					return
				}
				continue
			}
			if err := writeResponsesWebSocketTestCompleted(connection, "resp_ws_2", map[string]any{
				"input_tokens": 100, "output_tokens": 20,
				"input_tokens_details": map[string]any{"cached_tokens": 70, "cache_write_tokens": 20},
			}); err != nil {
				t.Errorf("write second turn: %v", err)
			}
		}
	}))
	defer server.Close()

	responses := newResponsesWebSocketTestProvider(server.URL, "same-socket-key")
	defer responses.Close()
	tools := responsesWebSocketTestTools()
	firstParams := Params{
		Model: responsesWebSocketTestModel, System: "stable instructions",
		Messages: []types.Message{types.UserMessage("inspect the workspace")}, Tools: tools,
		ReasoningEffort: "xhigh", ContinuationLineage: "same-socket-lineage", ContinuationEpoch: 7,
	}
	firstStream, err := responses.CreateStream(context.Background(), firstParams)
	if err != nil {
		t.Fatalf("first CreateStream: %v", err)
	}
	firstEvents := collectStreamEvents(firstStream)
	assistant, firstResponseID := responsesWebSocketTestAssistant(t, firstEvents)
	if firstResponseID != "resp_ws_1" {
		t.Fatalf("first response ID = %q, want resp_ws_1", firstResponseID)
	}
	continuation, ok := assistant.ValidatedProviderContinuation()
	if !ok || continuation == nil || len(continuation.Items) != 2 {
		t.Fatalf("encrypted continuation = %#v, want two output items", continuation)
	}
	rawReasoning := continuation.Items[0].RawJSON()
	if !bytes.Contains(rawReasoning, []byte(responsesWebSocketTestEncrypted)) ||
		!bytes.Contains(rawReasoning, []byte(responsesWebSocketTestRawMarker)) {
		t.Fatalf("reasoning continuation lost private bytes: %s", rawReasoning)
	}
	assistant.AttachProviderContinuation(continuation)

	secondParams := firstParams
	secondParams.Messages = []types.Message{
		types.UserMessage("inspect the workspace"),
		assistant,
		types.ToolResultMessage(types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: "call_ws_1", Content: "workspace result",
		}),
	}
	secondParams.PreviousResponseID = firstResponseID
	secondStream, err := responses.CreateStream(context.Background(), secondParams)
	if err != nil {
		t.Fatalf("second CreateStream: %v", err)
	}
	secondEvents := collectStreamEvents(secondStream)
	usage := responsesWebSocketTestUsage(secondEvents)
	if usage == nil || usage.InputTokens != 100 || usage.OutputTokens != 20 ||
		usage.CacheReadInputTokens != 70 || usage.CacheCreationInputTokens != 20 ||
		usage.UncachedInputTokens() != 10 {
		t.Fatalf("second-turn usage = %#v", usage)
	}

	firstRecord := responsesWebSocketTestReceiveRecord(t, records)
	secondRecord := responsesWebSocketTestReceiveRecord(t, records)
	if firstRecord.connection != 1 || secondRecord.connection != 1 || connections.Load() != 1 {
		t.Fatalf("connection IDs = %d/%d, total=%d; want one persistent connection", firstRecord.connection, secondRecord.connection, connections.Load())
	}
	if firstRecord.authorization != "Bearer same-socket-key" {
		t.Fatalf("handshake authorization = %q", firstRecord.authorization)
	}
	assertResponsesWebSocketTestEnvelope(t, firstRecord.body)
	assertResponsesWebSocketTestEnvelope(t, secondRecord.body)
	if _, exists := firstRecord.body["previous_response_id"]; exists {
		t.Fatalf("first request unexpectedly chained: %#v", firstRecord.body["previous_response_id"])
	}
	firstInput := responsesWebSocketTestInput(t, firstRecord.body)
	if len(firstInput) != 1 || responsesWebSocketTestMap(firstInput[0])["role"] != "user" {
		t.Fatalf("first input = %#v, want full initial user input", firstInput)
	}
	if secondRecord.body["previous_response_id"] != "resp_ws_1" {
		t.Fatalf("second previous_response_id = %#v", secondRecord.body["previous_response_id"])
	}
	if !containsString(firstRecord.body["include"], "reasoning.encrypted_content") ||
		!containsString(secondRecord.body["include"], "reasoning.encrypted_content") {
		t.Fatalf("encrypted reasoning include = %#v/%#v", firstRecord.body["include"], secondRecord.body["include"])
	}
	for turn, body := range []map[string]any{firstRecord.body, secondRecord.body} {
		reasoning, _ := body["reasoning"].(map[string]any)
		if reasoning["effort"] != "xhigh" || reasoning["context"] != "all_turns" {
			t.Fatalf("turn %d reasoning config = %#v", turn+1, reasoning)
		}
	}
	secondInput := responsesWebSocketTestInput(t, secondRecord.body)
	if len(secondInput) != 1 {
		t.Fatalf("second input = %#v, want only new tool output", secondInput)
	}
	toolOutput := responsesWebSocketTestMap(secondInput[0])
	if toolOutput["type"] != "function_call_output" || toolOutput["call_id"] != "call_ws_1" || toolOutput["output"] != "workspace result" {
		t.Fatalf("incremental tool output = %#v", toolOutput)
	}
	secondWire, err := json.Marshal(secondRecord.body)
	if err != nil {
		t.Fatal(err)
	}
	fullReplayBody, _, _, err := responses.buildResponsesRequestBody(
		secondParams,
		responses.snapshotRequestProfile(),
		"",
		responsesTransportWebSocket,
	)
	if err != nil {
		t.Fatalf("build full-replay comparison body: %v", err)
	}
	fullReplayWire, err := json.Marshal(fullReplayBody)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondWire) >= len(fullReplayWire) {
		t.Fatalf("incremental wire bytes = %d, full replay = %d; expected a smaller incremental request", len(secondWire), len(fullReplayWire))
	}
	t.Logf("fixture request bytes: incremental=%d full_replay=%d reduction=%.1f%%",
		len(secondWire), len(fullReplayWire), 100*(1-float64(len(secondWire))/float64(len(fullReplayWire))))
	if bytes.Contains(secondWire, []byte(responsesWebSocketTestEncrypted)) || bytes.Contains(secondWire, []byte(responsesWebSocketTestRawMarker)) {
		t.Fatalf("same-socket continuation replayed private history: %s", secondWire)
	}
}

func TestResponsesWebSocketHandshakeFailureFallsBackToHTTP(t *testing.T) {
	var upgradeAttempts atomic.Int32
	var httpRequests atomic.Int32
	var mu sync.Mutex
	var fallbackBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if websocket.IsWebSocketUpgrade(request) {
			upgradeAttempts.Add(1)
			http.Error(writer, "temporary handshake failure", http.StatusServiceUnavailable)
			return
		}
		httpRequests.Add(1)
		raw, _ := io.ReadAll(request.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		fallbackBody = body
		mu.Unlock()
		writeResponsesWebSocketTestSSE(writer, "resp_http_fallback")
	}))
	defer server.Close()

	responses := newResponsesWebSocketTestProvider(server.URL, "fallback-key")
	defer responses.Close()
	var retries []RetryEvent
	ctx := WithRetryObserver(context.Background(), func(event RetryEvent) {
		retries = append(retries, event)
	})
	stream, err := NewRetryProvider(responses, DefaultRetryConfig()).CreateStream(ctx, Params{
		Model: responsesWebSocketTestModel,
		Messages: []types.Message{
			types.UserMessage("first"), types.AssistantMessage("answer"), types.UserMessage("second"),
		},
		PreviousResponseID: "untrusted-external-id", ContinuationLineage: "handshake-fallback",
	})
	if err != nil {
		t.Fatalf("CreateStream after handshake failure: %v", err)
	}
	for range stream {
	}
	if upgradeAttempts.Load() != 1 || httpRequests.Load() != 1 {
		t.Fatalf("upgrade/http attempts = %d/%d, want 1/1", upgradeAttempts.Load(), httpRequests.Load())
	}
	if len(retries) != 1 || retries[0].Attempt != 1 || retries[0].MaxRetries != 4 ||
		retries[0].Delay != 0 || retries[0].Kind != "request" || retries[0].Err == nil {
		t.Fatalf("visible handshake fallback retry = %+v, want request retry 1/4 with problem details", retries)
	}
	mu.Lock()
	body := fallbackBody
	mu.Unlock()
	if body["stream"] != true || body["store"] != false {
		t.Fatalf("HTTP fallback transport fields = %#v", body)
	}
	if _, exists := body["type"]; exists {
		t.Fatalf("HTTP fallback retained WebSocket event type: %#v", body["type"])
	}
	if _, exists := body["previous_response_id"]; exists {
		t.Fatalf("HTTP fallback retrieved store=false response state: %#v", body["previous_response_id"])
	}
	if input := responsesWebSocketTestInput(t, body); len(input) != 3 {
		t.Fatalf("HTTP fallback input length = %d, want full history; input=%#v", len(input), input)
	}
}

func TestResponsesWebSocketFailedTurnDoesNotCommitChain(t *testing.T) {
	records := make(chan responsesWebSocketTestRecord, 2)
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connectionID := int(connections.Add(1))
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
		records <- responsesWebSocketTestRecord{connection: connectionID, turn: 1, body: body}
		if connectionID == 1 {
			if err := writeResponsesWebSocketTestEvents(connection,
				map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_failed", "model": responsesWebSocketTestModel}},
				map[string]any{"type": "response.failed", "response": map[string]any{
					"id": "resp_failed", "status": "failed", "status_code": 500,
					"error": map[string]any{"message": "injected failure", "code": "server_error", "status": 500},
				}},
			); err != nil {
				t.Errorf("write failed turn: %v", err)
			}
			return
		}
		if err := writeResponsesWebSocketTestCompleted(connection, "resp_recovered", map[string]any{"input_tokens": 12, "output_tokens": 2}); err != nil {
			t.Errorf("write recovered turn: %v", err)
		}
	}))
	defer server.Close()

	responses := newResponsesWebSocketTestProvider(server.URL, "failed-turn-key")
	defer responses.Close()
	lineage := "failed-turn-lineage"
	failedHistory := []types.Message{
		types.UserMessage("prior user"),
		types.AssistantMessage("prior committed answer"),
		types.UserMessage("turn that fails"),
	}
	firstParams := Params{
		Model: responsesWebSocketTestModel, System: "stable instructions", Messages: failedHistory,
		ContinuationLineage: lineage, ContinuationEpoch: 4,
	}
	stream, err := responses.CreateStream(context.Background(), firstParams)
	if err != nil {
		t.Fatalf("failed-turn CreateStream: %v", err)
	}
	failedEvents := collectStreamEvents(stream)
	sawError := false
	for _, event := range failedEvents {
		if event.Type == types.EventMessageStop {
			t.Fatalf("failed response emitted MessageStop with ID %q", event.ResponseID)
		}
		if event.Type == types.EventError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("failed response events = %#v, want EventError", failedEvents)
	}

	secondParams := firstParams
	secondParams.Messages = append(append([]types.Message(nil), failedHistory...), types.UserMessage("retry from local history"))
	secondParams.PreviousResponseID = "resp_failed"
	stream, err = responses.CreateStream(context.Background(), secondParams)
	if err != nil {
		t.Fatalf("recovery CreateStream: %v", err)
	}
	for range stream {
	}

	firstRecord := responsesWebSocketTestReceiveRecord(t, records)
	secondRecord := responsesWebSocketTestReceiveRecord(t, records)
	if firstRecord.connection != 1 || secondRecord.connection != 2 || connections.Load() != 2 {
		t.Fatalf("failed/recovery connection IDs = %d/%d, total=%d; want fresh socket", firstRecord.connection, secondRecord.connection, connections.Load())
	}
	if _, exists := secondRecord.body["previous_response_id"]; exists {
		t.Fatalf("recovery request committed failed response ID: %#v", secondRecord.body["previous_response_id"])
	}
	input := responsesWebSocketTestInput(t, secondRecord.body)
	if len(input) != 4 {
		t.Fatalf("recovery input length = %d, want complete four-message history; input=%#v", len(input), input)
	}
	last := responsesWebSocketTestMap(input[len(input)-1])
	if last["role"] != "user" || last["content"] != "retry from local history" {
		t.Fatalf("recovery last input = %#v", last)
	}
}

func TestResponsesWebSocketDisconnectReplaysFullHistory(t *testing.T) {
	records := make(chan responsesWebSocketTestRecord, 2)
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connectionID := int(connections.Add(1))
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
		records <- responsesWebSocketTestRecord{connection: connectionID, turn: 1, body: body}
		if connectionID == 1 {
			if err := writeResponsesWebSocketTestReasoningTurn(
				connection, "resp_before_disconnect", responsesWebSocketTestEncrypted, responsesWebSocketTestRawMarker,
				map[string]any{"input_tokens": 40, "output_tokens": 10},
			); err != nil {
				t.Errorf("write first connection: %v", err)
			}
			return
		}
		if err := writeResponsesWebSocketTestCompleted(connection, "resp_after_reconnect", map[string]any{"input_tokens": 90, "output_tokens": 4}); err != nil {
			t.Errorf("write second connection: %v", err)
		}
	}))
	defer server.Close()

	responses := newResponsesWebSocketTestProvider(server.URL, "reconnect-key")
	defer responses.Close()
	lineage := "disconnect-lineage"
	baseParams := Params{
		Model: responsesWebSocketTestModel, System: "stable instructions",
		Messages: []types.Message{types.UserMessage("inspect")}, Tools: responsesWebSocketTestTools(),
		ReasoningEffort: "xhigh", ContinuationLineage: lineage, ContinuationEpoch: 9,
	}
	stream, err := responses.CreateStream(context.Background(), baseParams)
	if err != nil {
		t.Fatalf("first CreateStream: %v", err)
	}
	firstEvents := collectStreamEvents(stream)
	assistant, responseID := responsesWebSocketTestAssistant(t, firstEvents)
	if responseID != "resp_before_disconnect" {
		t.Fatalf("first response ID = %q", responseID)
	}

	session := responses.responsesWebSocketSession(lineage)
	session.mu.Lock()
	firstWire := session.wire
	session.mu.Unlock()
	if firstWire == nil {
		t.Fatal("first WebSocket wire was not retained after completed response")
	}
	select {
	case <-firstWire.done:
	case <-time.After(3 * time.Second):
		t.Fatal("client did not observe server disconnect")
	}

	secondParams := baseParams
	secondParams.Messages = []types.Message{
		types.UserMessage("inspect"),
		assistant,
		types.ToolResultMessage(types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: "call_ws_1", Content: "replayed result",
		}),
	}
	secondParams.PreviousResponseID = responseID
	stream, err = responses.CreateStream(context.Background(), secondParams)
	if err != nil {
		t.Fatalf("second CreateStream: %v", err)
	}
	for range stream {
	}

	firstRecord := responsesWebSocketTestReceiveRecord(t, records)
	secondRecord := responsesWebSocketTestReceiveRecord(t, records)
	if firstRecord.connection != 1 || secondRecord.connection != 2 || connections.Load() != 2 {
		t.Fatalf("connection IDs = %d/%d, total=%d; want reconnect", firstRecord.connection, secondRecord.connection, connections.Load())
	}
	if _, exists := secondRecord.body["previous_response_id"]; exists {
		t.Fatalf("reconnected request used volatile response ID: %#v", secondRecord.body["previous_response_id"])
	}
	input := responsesWebSocketTestInput(t, secondRecord.body)
	if len(input) != 4 {
		t.Fatalf("reconnected input length = %d, want user+reasoning+call+result; input=%#v", len(input), input)
	}
	if responsesWebSocketTestMap(input[0])["role"] != "user" {
		t.Fatalf("first replay item = %#v, want initial user message", input[0])
	}
	reasoning := responsesWebSocketTestMap(input[1])
	if reasoning["type"] != "reasoning" || reasoning["encrypted_content"] != responsesWebSocketTestEncrypted ||
		reasoning["private_marker"] != responsesWebSocketTestRawMarker {
		t.Fatalf("reasoning replay = %#v", reasoning)
	}
	call := responsesWebSocketTestMap(input[2])
	if call["type"] != "function_call" || call["call_id"] != "call_ws_1" {
		t.Fatalf("function-call replay = %#v", call)
	}
	result := responsesWebSocketTestMap(input[3])
	if result["type"] != "function_call_output" || result["call_id"] != "call_ws_1" || result["output"] != "replayed result" {
		t.Fatalf("tool-result replay = %#v", result)
	}
}

func TestResponsesWebSocketLineageConcurrency(t *testing.T) {
	t.Run("different lineages run in parallel", func(t *testing.T) {
		started := make(chan struct{}, 2)
		release := make(chan struct{})
		var connections atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			connectionID := connections.Add(1)
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
			started <- struct{}{}
			<-release
			if err := writeResponsesWebSocketTestCompleted(connection, fmt.Sprintf("resp_parallel_%d", connectionID), map[string]any{"input_tokens": 1, "output_tokens": 1}); err != nil {
				t.Errorf("write response: %v", err)
			}
		}))
		defer server.Close()

		responses := newResponsesWebSocketTestProvider(server.URL, "parallel-key")
		defer responses.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done := make(chan error, 2)
		for _, lineage := range []string{"parallel-a", "parallel-b"} {
			lineage := lineage
			go func() {
				stream, err := responses.CreateStream(ctx, Params{
					Model: responsesWebSocketTestModel, Messages: []types.Message{types.UserMessage(lineage)},
					ContinuationLineage: lineage,
				})
				if err == nil {
					for range stream {
					}
				}
				done <- err
			}()
		}
		for count := 0; count < 2; count++ {
			select {
			case <-started:
			case <-ctx.Done():
				close(release)
				t.Fatalf("parallel lineages did not both reach the server: %v", ctx.Err())
			}
		}
		close(release)
		for count := 0; count < 2; count++ {
			if err := <-done; err != nil {
				t.Fatalf("parallel CreateStream: %v", err)
			}
		}
		if got := connections.Load(); got != 2 {
			t.Fatalf("parallel connection count = %d, want 2", got)
		}
	})

	t.Run("same lineage is single flight", func(t *testing.T) {
		firstReceived := make(chan struct{}, 1)
		secondReceived := make(chan struct{}, 1)
		releaseFirst := make(chan struct{})
		var connections atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			connections.Add(1)
			connection, err := responsesWebSocketTestUpgrader.Upgrade(writer, request, nil)
			if err != nil {
				t.Errorf("upgrade: %v", err)
				return
			}
			defer connection.Close()
			var first map[string]any
			if err := connection.ReadJSON(&first); err != nil {
				t.Errorf("read first response.create: %v", err)
				return
			}
			firstReceived <- struct{}{}
			<-releaseFirst
			if err := writeResponsesWebSocketTestCompleted(connection, "resp_serial_1", map[string]any{"input_tokens": 1, "output_tokens": 1}); err != nil {
				t.Errorf("write first response: %v", err)
				return
			}
			var second map[string]any
			if err := connection.ReadJSON(&second); err != nil {
				t.Errorf("read second response.create: %v", err)
				return
			}
			secondReceived <- struct{}{}
			if err := writeResponsesWebSocketTestCompleted(connection, "resp_serial_2", map[string]any{"input_tokens": 1, "output_tokens": 1}); err != nil {
				t.Errorf("write second response: %v", err)
			}
		}))
		defer server.Close()

		responses := newResponsesWebSocketTestProvider(server.URL, "serial-key")
		defer responses.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		firstDone := make(chan error, 1)
		go func() {
			stream, err := responses.CreateStream(ctx, Params{
				Model: responsesWebSocketTestModel, Messages: []types.Message{types.UserMessage("first")},
				ContinuationLineage: "serial-lineage",
			})
			if err == nil {
				for range stream {
				}
			}
			firstDone <- err
		}()
		select {
		case <-firstReceived:
		case <-ctx.Done():
			close(releaseFirst)
			t.Fatalf("first request did not reach server: %v", ctx.Err())
		}

		secondCreated := make(chan error, 1)
		secondDone := make(chan error, 1)
		go func() {
			stream, err := responses.CreateStream(ctx, Params{
				Model: responsesWebSocketTestModel, Messages: []types.Message{types.UserMessage("second")},
				ContinuationLineage: "serial-lineage",
			})
			secondCreated <- err
			if err == nil {
				for range stream {
				}
			}
			secondDone <- err
		}()
		timer := time.NewTimer(150 * time.Millisecond)
		select {
		case err := <-secondCreated:
			timer.Stop()
			t.Errorf("second same-lineage call returned before first completed: %v", err)
		case <-secondReceived:
			timer.Stop()
			t.Error("server received overlapping same-lineage response.create")
		case <-timer.C:
		}
		close(releaseFirst)
		if err := <-firstDone; err != nil {
			t.Fatalf("first CreateStream: %v", err)
		}
		select {
		case <-secondReceived:
		case <-ctx.Done():
			t.Fatalf("second request did not start after first terminal event: %v", ctx.Err())
		}
		select {
		case err := <-secondDone:
			if err != nil {
				t.Fatalf("second CreateStream: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("second request did not complete: %v", ctx.Err())
		}
		if got := connections.Load(); got != 1 {
			t.Fatalf("same-lineage connection count = %d, want 1", got)
		}
	})
}

func TestResponsesWebSocketDebugRedactsPrivateState(t *testing.T) {
	const apiKey = "ws-auth-secret-token"
	authHeaders := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authHeaders <- request.Header.Get("Authorization")
		connection, err := responsesWebSocketTestUpgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer connection.Close()
		for turn := 1; turn <= 2; turn++ {
			var body map[string]any
			if err := connection.ReadJSON(&body); err != nil {
				t.Errorf("read response.create turn %d: %v", turn, err)
				return
			}
			if turn == 1 {
				if err := writeResponsesWebSocketTestReasoningTurn(
					connection, "resp_debug_1", responsesWebSocketTestEncrypted, responsesWebSocketTestRawMarker,
					map[string]any{"input_tokens": 7, "output_tokens": 3},
				); err != nil {
					t.Errorf("write first turn: %v", err)
				}
				continue
			}
			if err := writeResponsesWebSocketTestCompleted(connection, "resp_debug_2", map[string]any{"input_tokens": 8, "output_tokens": 2}); err != nil {
				t.Errorf("write second turn: %v", err)
			}
		}
	}))
	defer server.Close()

	responses := newResponsesWebSocketTestProvider(server.URL, apiKey)
	defer responses.Close()
	ref := NewProviderRef(responses)
	var debug bytes.Buffer
	ref.SetDebugObserver(NewDebugWriterObserver(&debug))
	baseParams := Params{
		Model: responsesWebSocketTestModel, Messages: []types.Message{types.UserMessage("debug request")},
		Tools: responsesWebSocketTestTools(), ReasoningEffort: "xhigh",
		ContinuationLineage: "debug-lineage", ContinuationEpoch: 3,
	}
	stream, err := ref.CreateStream(context.Background(), baseParams)
	if err != nil {
		t.Fatalf("first CreateStream: %v", err)
	}
	firstEvents := collectStreamEvents(stream)
	assistant, responseID := responsesWebSocketTestAssistant(t, firstEvents)

	secondParams := baseParams
	secondParams.Messages = []types.Message{
		types.UserMessage("debug request"), assistant,
		types.ToolResultMessage(types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: "call_ws_1", Content: "debug-visible-result",
		}),
	}
	secondParams.PreviousResponseID = responseID
	stream, err = ref.CreateStream(context.Background(), secondParams)
	if err != nil {
		t.Fatalf("second CreateStream: %v", err)
	}
	for range stream {
	}
	if got := <-authHeaders; got != "Bearer "+apiKey {
		t.Fatalf("server authorization = %q", got)
	}

	log := debug.String()
	for _, forbidden := range []string{
		apiKey,
		responsesWebSocketTestEncrypted,
		responsesWebSocketTestRawMarker,
		string(types.ThinkingSignatureOpenAIEncryptedReasoning),
	} {
		if strings.Contains(log, forbidden) {
			t.Fatalf("debug output leaked %q:\n%s", forbidden, log)
		}
	}
	for _, visible := range []string{responsesWebSocketTestVisibleThink, "debug-visible-result"} {
		if !strings.Contains(log, visible) {
			t.Fatalf("debug output lost visible diagnostic %q:\n%s", visible, log)
		}
	}
}

func newResponsesWebSocketTestProvider(baseURL, apiKey string) *ResponsesProvider {
	return NewResponses(Config{
		ProviderName: "openai", ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
		ResponsesWebSocket: CapabilitySupported,
		APIKey:             apiKey, BaseURL: baseURL, Model: responsesWebSocketTestModel,
	})
}

func responsesWebSocketTestTools() []types.ToolDefinition {
	return []types.ToolDefinition{{
		Name: "Inspect", Description: "Inspect a workspace path",
		InputSchema: types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"path": map[string]any{"type": "string"},
			},
			Required: []string{"path"},
		},
	}}
}

func writeResponsesWebSocketTestReasoningTurn(
	connection *websocket.Conn,
	responseID string,
	encrypted string,
	rawMarker string,
	usage map[string]any,
) error {
	reasoning := map[string]any{
		"type": "reasoning", "id": "rs_ws_1", "status": "completed",
		"summary": []any{}, "encrypted_content": encrypted,
	}
	if rawMarker != "" {
		reasoning["private_marker"] = rawMarker
	}
	call := map[string]any{
		"type": "function_call", "id": "fc_ws_1", "call_id": "call_ws_1",
		"name": "Inspect", "status": "completed", "arguments": `{"path":"."}`,
	}
	events := []map[string]any{
		{"type": "response.created", "response": map[string]any{"id": responseID, "model": responsesWebSocketTestModel}},
		{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "reasoning", "id": "rs_ws_1", "status": "in_progress", "summary": []any{}}},
		{"type": "response.reasoning_summary_text.delta", "output_index": 0, "delta": responsesWebSocketTestVisibleThink},
		{"type": "response.output_item.done", "output_index": 0, "item": reasoning},
		{"type": "response.output_item.added", "output_index": 1, "item": map[string]any{"type": "function_call", "id": "fc_ws_1", "call_id": "call_ws_1", "name": "Inspect", "status": "in_progress"}},
		{"type": "response.function_call_arguments.delta", "output_index": 1, "item_id": "fc_ws_1", "delta": `{"path":"."}`},
		{"type": "response.function_call_arguments.done", "output_index": 1, "item_id": "fc_ws_1", "name": "Inspect", "arguments": `{"path":"."}`},
		{"type": "response.output_item.done", "output_index": 1, "item": call},
		{"type": "response.completed", "response": map[string]any{
			"id": responseID, "model": responsesWebSocketTestModel, "status": "completed",
			"usage": usage, "output": []any{reasoning, call},
		}},
	}
	return writeResponsesWebSocketTestEvents(connection, events...)
}

func writeResponsesWebSocketTestCompleted(connection *websocket.Conn, responseID string, usage map[string]any) error {
	return writeResponsesWebSocketTestEvents(connection,
		map[string]any{"type": "response.created", "response": map[string]any{"id": responseID, "model": responsesWebSocketTestModel}},
		map[string]any{"type": "response.completed", "response": map[string]any{
			"id": responseID, "model": responsesWebSocketTestModel, "status": "completed", "usage": usage, "output": []any{},
		}},
	)
}

func writeResponsesWebSocketTestEvents(connection *websocket.Conn, events ...map[string]any) error {
	for _, event := range events {
		if err := connection.WriteJSON(event); err != nil {
			return err
		}
	}
	return nil
}

func writeResponsesWebSocketTestSSE(writer http.ResponseWriter, responseID string) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, buildSSEStream([]sseEvent{
		{Type: "response.created", Data: fmt.Sprintf(`{"response":{"id":%q,"model":%q}}`, responseID, responsesWebSocketTestModel)},
		{Type: "response.completed", Data: fmt.Sprintf(`{"response":{"id":%q,"model":%q,"status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[]}}`, responseID, responsesWebSocketTestModel)},
	}))
}

func responsesWebSocketTestAssistant(t *testing.T, events []types.StreamEvent) (types.Message, string) {
	t.Helper()
	reasoning := types.ThinkingBlock{Type: types.ContentTypeThinking}
	var continuation *types.ProviderContinuation
	responseID := ""
	for _, event := range events {
		if event.Type == types.EventContentBlockStart && event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeThinking {
			reasoning.ProviderItemID = event.ContentBlock.ID
			reasoning.ProviderStatus = event.ContentBlock.ProviderStatus
			reasoning.Signature = event.ContentBlock.Signature
			reasoning.SignatureKind = event.ContentBlock.SignatureKind
			reasoning.SignatureModel = event.ContentBlock.SignatureModel
		}
		if event.Type == types.EventContentBlockDelta && event.Delta != nil {
			switch event.Delta.Type {
			case "thinking_delta":
				reasoning.Thinking += event.Delta.Thinking
			case "signature_delta":
				reasoning.ProviderItemID = event.Delta.ID
				reasoning.ProviderStatus = event.Delta.ProviderStatus
				reasoning.Signature = event.Delta.Signature
				reasoning.SignatureKind = event.Delta.SignatureKind
				reasoning.SignatureModel = event.Delta.SignatureModel
			}
		}
		if event.Type == types.EventMessageStop {
			responseID = event.ResponseID
			continuation = event.ProviderContinuation
		}
	}
	if reasoning.Thinking != responsesWebSocketTestVisibleThink || reasoning.Signature != responsesWebSocketTestEncrypted ||
		reasoning.SignatureKind != types.ThinkingSignatureOpenAIEncryptedReasoning || reasoning.SignatureModel != responsesWebSocketTestModel ||
		reasoning.ProviderItemID != "rs_ws_1" || reasoning.ProviderStatus != "completed" {
		t.Fatalf("reasoning projection = %#v", reasoning)
	}
	if continuation == nil || responseID == "" {
		t.Fatalf("terminal continuation = %#v, response ID = %q", continuation, responseID)
	}
	assistant := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		reasoning,
		types.ToolUseBlock{
			Type: types.ContentTypeToolUse, ID: "call_ws_1", Name: "Inspect",
			Input: map[string]any{"path": "."},
		},
	}}
	assistant.AttachProviderContinuation(continuation)
	if !assistant.HasProviderContinuation() {
		t.Fatal("assistant did not retain provider continuation")
	}
	return assistant, responseID
}

func responsesWebSocketTestUsage(events []types.StreamEvent) *types.Usage {
	var usage *types.Usage
	for _, event := range events {
		if event.Usage != nil {
			usage = event.Usage
		}
	}
	return usage
}

func responsesWebSocketTestReceiveRecord(t *testing.T, records <-chan responsesWebSocketTestRecord) responsesWebSocketTestRecord {
	t.Helper()
	select {
	case record := <-records:
		return record
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for fake WebSocket request")
		return responsesWebSocketTestRecord{}
	}
}

func assertResponsesWebSocketTestEnvelope(t *testing.T, body map[string]any) {
	t.Helper()
	if body["type"] != "response.create" || body["model"] != responsesWebSocketTestModel {
		t.Fatalf("WebSocket envelope identity = %#v", body)
	}
	if stored, ok := body["store"].(bool); !ok || stored {
		t.Fatalf("WebSocket store = %#v, want false", body["store"])
	}
	for _, forbidden := range []string{"stream", "background", "stream_options"} {
		if value, exists := body[forbidden]; exists {
			t.Fatalf("WebSocket envelope included %s=%#v", forbidden, value)
		}
	}
}

func responsesWebSocketTestInput(t *testing.T, body map[string]any) []any {
	t.Helper()
	input, ok := body["input"].([]any)
	if !ok {
		t.Fatalf("input = %#v, want array", body["input"])
	}
	return input
}

func responsesWebSocketTestMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}
