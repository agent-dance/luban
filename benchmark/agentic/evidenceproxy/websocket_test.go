package evidenceproxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketRelaysPrewarmAndBoundIncrementalInferenceOnOneConnection(t *testing.T) {
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("w", 48)
	upstreamDone := make(chan struct{})
	var upstreamAuthorization string
	var upstreamCompression string
	var upstreamFailure atomic.Value
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer close(upstreamDone)
		upstreamAuthorization = request.Header.Get("Authorization")
		upstreamCompression = request.Header.Get("Sec-WebSocket-Extensions")
		responseHeaders := http.Header{
			"OpenAI-Model":         []string{"gpt-5.6-sol"},
			"X-Reasoning-Included": []string{"1"},
			"X-Codex-Turn-State":   []string{"sticky-state"},
		}
		connection, err := (&websocket.Upgrader{EnableCompression: true}).Upgrade(writer, request, responseHeaders)
		if err != nil {
			upstreamFailure.Store(err)
			return
		}
		defer connection.Close()

		_, firstPayload, err := connection.ReadMessage()
		if err != nil {
			upstreamFailure.Store(err)
			return
		}
		first := decodeTestJSONMap(t, firstPayload)
		if first["type"] != "response.create" || first["generate"] != false || first["store"] != false || first["service_tier"] != "default" {
			upstreamFailure.Store(errors.New("first websocket frame is not a store-disabled prewarm"))
			return
		}
		journal, err := readJSONLines[AttemptStartJournalEntry](AttemptJournalPath(evidencePath))
		if err != nil || len(journal) != 1 || journal[0].ProviderAttemptKind != "prewarm" || journal[0].WebSocketRequestSequence != 0 {
			upstreamFailure.Store(errors.New("prewarm frame crossed upstream before its WAL receipt was durable"))
			return
		}
		writeTestWebSocketJSON(t, connection, map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id": "resp-warm", "status": "in_progress",
				"headers": map[string]any{"openai-model": "gpt-5.6-sol", "x-request-id": "req-warm"},
			},
		})
		writeTestWebSocketJSON(t, connection, map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id": "resp-warm", "status": "completed", "output": []any{},
			},
		})

		_, secondPayload, err := connection.ReadMessage()
		if err != nil {
			upstreamFailure.Store(err)
			return
		}
		second := decodeTestJSONMap(t, secondPayload)
		if second["type"] != "response.create" || second["previous_response_id"] != "resp-warm" || second["service_tier"] != "default" {
			upstreamFailure.Store(errors.New("incremental inference did not bind the prewarm response id"))
			return
		}
		journal, err = readJSONLines[AttemptStartJournalEntry](AttemptJournalPath(evidencePath))
		if err != nil || len(journal) != 2 || journal[1].ProviderAttemptKind != "inference" || journal[1].WebSocketRequestSequence != 1 {
			upstreamFailure.Store(errors.New("inference frame crossed upstream before its WAL receipt was durable"))
			return
		}
		writeTestWebSocketJSON(t, connection, map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id": "resp-inference", "status": "in_progress",
				"headers": map[string]any{"openai-model": "gpt-5.6-sol"},
			},
		})
		writeTestWebSocketJSON(t, connection, map[string]any{
			"type": "response.output_item.done",
			"item": map[string]any{
				"type": "apply_patch_call", "id": "patch-item", "call_id": "call-patch",
				"status":    "completed",
				"operation": map[string]any{"type": "update_file", "path": "main.go", "diff": "@@ -1 +1 @@\n-old\n+new"},
			},
		})
		writeTestWebSocketJSON(t, connection, map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id": "resp-inference", "status": "completed", "model": "gpt-5.6-sol",
				"usage": map[string]any{
					"input_tokens": 9, "input_tokens_details": map[string]any{"cached_tokens": 7},
					"output_tokens": 2, "output_tokens_details": map[string]any{"reasoning_tokens": 1},
				},
			},
		})
	}))
	defer upstream.Close()

	config := configureCodexStaticProof(t, Config{
		Upstream: upstream.URL, EvidencePath: evidencePath, AccessPath: accessPath,
		Credential: "provider-secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh",
		RunIdentity: testRunIdentity, WebSocketDialer: tlsWebSocketDialer(upstream),
	})
	handler, err := NewHandler(config)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http") + accessPath + "/v1/responses"
	downstreamHeaders := http.Header{"Authorization": []string{"Bearer placeholder"}}
	clientDialer := websocket.Dialer{EnableCompression: true}
	client, response, err := clientDialer.Dial(wsURL, downstreamHeaders)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if response.StatusCode != http.StatusSwitchingProtocols || response.Header.Get("OpenAI-Model") != "gpt-5.6-sol" || response.Header.Get("X-Codex-Turn-State") != "sticky-state" {
		t.Fatalf("projected websocket handshake status=%d headers=%v", response.StatusCode, response.Header)
	}

	prewarm := `{"type":"response.create","model":"gpt-5.6-sol","instructions":"test","prompt_cache_key":"ws-cache-key","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"task"}]}],"tools":[{"type":"apply_patch"}],"tool_choice":"auto","parallel_tool_calls":true,"reasoning":{"effort":"xhigh"},"store":false,"stream":true,"include":[],"generate":false}`
	if err := client.WriteMessage(websocket.TextMessage, []byte(prewarm)); err != nil {
		t.Fatal(err)
	}
	readUntilCompleted(t, client)
	inference := `{"type":"response.create","model":"gpt-5.6-sol","instructions":"test","prompt_cache_key":"ws-cache-key","previous_response_id":"resp-warm","input":[],"tools":[{"type":"apply_patch"}],"tool_choice":"auto","parallel_tool_calls":true,"reasoning":{"effort":"xhigh"},"store":false,"stream":true,"include":[]}`
	if err := client.WriteMessage(websocket.TextMessage, []byte(inference)); err != nil {
		t.Fatal(err)
	}
	readUntilCompleted(t, client)
	_ = client.Close()
	<-upstreamDone
	if value := upstreamFailure.Load(); value != nil {
		t.Fatal(value)
	}
	waitForEvidenceRecords(t, evidencePath, 2)
	handler.shutdownWebSockets()
	handler.webSocketWG.Wait()
	if err := handler.SealEvidence(); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEvidenceSeal(evidencePath, testRunIdentity); err != nil {
		t.Fatal(err)
	}
	if upstreamAuthorization != "Bearer provider-secret" {
		t.Fatalf("upstream authorization = %q", upstreamAuthorization)
	}
	if !strings.Contains(strings.ToLower(upstreamCompression), "permessage-deflate") {
		t.Fatalf("upstream compression offer = %q", upstreamCompression)
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 2 {
		t.Fatalf("record count = %d", len(records))
	}
	warmup, scored := records[0], records[1]
	if warmup.Transport != "websocket" || warmup.ProviderAttemptKind != "prewarm" || !warmup.ProtocolValid || warmup.Disposition != "prewarm_transport" || warmup.UsagePresent || warmup.WebSocketConnectionHash == "" || warmup.WebSocketConnectionReused || warmup.WebSocketRequestSequence != 0 || warmup.UpstreamRequestIDHash == "" {
		t.Fatalf("warmup evidence = %#v", warmup)
	}
	if scored.ProviderAttemptKind != "inference" || !scored.ProtocolValid || scored.Disposition != "valid" || !scored.UsagePresent || scored.UpstreamRequestIDHash != "" || !scored.PreviousResponseIDPresent || !scored.WebSocketChainBound || !scored.WebSocketConnectionReused || scored.WebSocketRequestSequence != 1 || scored.WebSocketConnectionHash != warmup.WebSocketConnectionHash {
		t.Fatalf("inference evidence = %#v", scored)
	}
	for _, record := range []Record{warmup, scored} {
		if !record.CachePolicyObserved || !record.PromptCacheKeyPresent || !isLowerHex64(record.PromptCacheKeyHash) || record.PromptCacheOptionsPresent || record.PromptCacheOptionsMode != "" || record.PromptCacheTTLSeconds != nil || record.PromptCacheRetentionPresent || record.CacheBreakpointCount != 0 || len(record.CacheBreakpointPositionHashes) != 0 {
			t.Fatalf("WebSocket omitted cache-policy evidence = %#v", record)
		}
		if record.ClientAgentID != "codex" || record.RequestedServiceTierPresent || record.RequestedServiceTier != "" ||
			record.RequestedServiceTierCanonical != "default" || record.RequestedServiceTierRepresentation != "client_canonicalized_default" ||
			!isLowerHex64(record.ClientCanonicalizationStaticProofSHA256) || record.OriginalServiceTierPresent ||
			!record.ForwardedServiceTierPresent || record.ForwardedServiceTier != "default" ||
			record.ServiceTierTransformation != serviceTierTransformationInjectDefault || !record.ServiceTierTransformationExactDiff ||
			!isLowerHex64(record.ServiceTierTransformationProofSHA256) || record.OriginalRequestBodySHA256 == record.ForwardedRequestBodySHA256 ||
			record.OriginalRequestWithoutServiceTierSHA256 != record.ForwardedRequestWithoutServiceTierSHA256 || !record.ServiceTierComparable {
			t.Fatalf("Codex WebSocket service-tier normalization evidence = %#v", record)
		}
	}
	if warmup.PromptCacheKeyHash != scored.PromptCacheKeyHash {
		t.Fatalf("WebSocket cache key changed within one connection: %q != %q", warmup.PromptCacheKeyHash, scored.PromptCacheKeyHash)
	}
	if scored.InputTokens == nil || *scored.InputTokens != 9 || scored.CachedInputTokens == nil || *scored.CachedInputTokens != 7 || scored.OutputTokens == nil || *scored.OutputTokens != 2 {
		t.Fatalf("inference usage = %#v", scored)
	}
	if scored.ResponseOutputItemCount != 1 || len(scored.ToolCalls) != 1 || scored.ToolCalls[0].Kind != "apply_patch_call" || scored.ToolCalls[0].Name != "apply_patch" || scored.ToolCalls[0].InputBytes == 0 {
		t.Fatalf("output_item.done tool evidence = %#v", scored)
	}
}

func TestWebSocketRejectsCrossConnectionPreviousResponseBeforeProviderFrame(t *testing.T) {
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("x", 48)
	var upstreamFrames atomic.Int64
	upstreamClosed := make(chan struct{})
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer close(upstreamClosed)
		writer.Header().Set("OpenAI-Model", "gpt-5.6-sol")
		connection, err := (&websocket.Upgrader{}).Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, _, err := connection.ReadMessage(); err == nil {
			upstreamFrames.Add(1)
		}
	}))
	defer upstream.Close()
	handler, err := NewHandler(Config{
		Upstream: upstream.URL, EvidencePath: evidencePath, AccessPath: accessPath,
		Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh",
		RunIdentity: testRunIdentity, WebSocketDialer: tlsWebSocketDialer(upstream),
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+accessPath+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	payload := withDefaultServiceTier(t, `{"type":"response.create","model":"gpt-5.6-sol","previous_response_id":"resp-from-another-connection","input":[],"tools":[],"tool_choice":"auto","parallel_tool_calls":true,"reasoning":{"effort":"xhigh"},"store":false,"stream":true,"include":[]}`)
	if err := client.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		t.Fatal(err)
	}
	_, errorPayload, err := client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(errorPayload), "websocket_previous_response_id_unbound") {
		t.Fatalf("proxy error = %s", errorPayload)
	}
	_ = client.Close()
	<-upstreamClosed
	if upstreamFrames.Load() != 0 {
		t.Fatal("unbound previous response id crossed the provider boundary")
	}
	waitForEvidenceRecords(t, evidencePath, 1)
	record := readEvidenceRecords(t, evidencePath)[0]
	if record.ProviderAttemptStarted || record.ProtocolValid || record.Disposition != "experiment_invalid" || record.ErrorCode != "websocket_previous_response_id_unbound" {
		t.Fatalf("rejected websocket evidence = %#v", record)
	}
	if journal, err := readJSONLines[AttemptStartJournalEntry](AttemptJournalPath(evidencePath)); !errors.Is(err, os.ErrNotExist) || len(journal) != 0 {
		t.Fatalf("unexpected provider WAL for rejected frame: %#v err=%v", journal, err)
	}
}

func TestWebSocketRejectsProReasoningModeBeforeProviderFrame(t *testing.T) {
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("p", 48)
	var upstreamFrames atomic.Int64
	upstreamClosed := make(chan struct{})
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer close(upstreamClosed)
		writer.Header().Set("OpenAI-Model", "gpt-5.6-sol")
		connection, err := (&websocket.Upgrader{}).Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, _, err := connection.ReadMessage(); err == nil {
			upstreamFrames.Add(1)
		}
	}))
	defer upstream.Close()
	handler, err := NewHandler(Config{
		Upstream: upstream.URL, EvidencePath: evidencePath, AccessPath: accessPath,
		Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh",
		RunIdentity: testRunIdentity, WebSocketDialer: tlsWebSocketDialer(upstream),
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+accessPath+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	payload := withDefaultServiceTier(t, `{"type":"response.create","model":"gpt-5.6-sol","input":[],"tools":[],"tool_choice":"auto","parallel_tool_calls":true,"reasoning":{"effort":"xhigh","mode":"pro"},"store":false,"stream":true,"include":[]}`)
	if err := client.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		t.Fatal(err)
	}
	_, errorPayload, err := client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(errorPayload), "reasoning_mode_not_comparable") {
		t.Fatalf("proxy error = %s", errorPayload)
	}
	_ = client.Close()
	<-upstreamClosed
	if upstreamFrames.Load() != 0 {
		t.Fatal("pro reasoning frame crossed the provider boundary")
	}
	waitForEvidenceRecords(t, evidencePath, 1)
	record := readEvidenceRecords(t, evidencePath)[0]
	if record.ProviderAttemptStarted || record.ProtocolValid || record.RequestedReasoningMode != "pro" || record.RequestedReasoningModeCanonical != "pro" || record.ErrorCode != "reasoning_mode_not_comparable" || record.Disposition != "experiment_invalid" {
		t.Fatalf("rejected pro websocket evidence = %#v", record)
	}
}

func TestWebSocketHandshake426RemainsCapabilityFallbackWithZeroProviderAttempts(t *testing.T) {
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("y", 48)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Authorization", "Bearer reflected-provider-secret")
		writer.Header().Set("Location", "https://redirect.invalid/steal")
		writer.Header().Set("Set-Cookie", "provider-secret=reflected")
		writer.WriteHeader(http.StatusUpgradeRequired)
		_, _ = io.WriteString(writer, "reflected-provider-secret")
	}))
	defer upstream.Close()
	handler, err := NewHandler(Config{
		Upstream: upstream.URL, EvidencePath: evidencePath, AccessPath: accessPath,
		Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh",
		RunIdentity: testRunIdentity, WebSocketDialer: tlsWebSocketDialer(upstream),
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+accessPath+"/v1/responses", nil)
	if connection != nil {
		_ = connection.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("426 fallback connection=%v response=%v err=%v", connection, response, err)
	}
	responseBody, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatal(readErr)
	}
	_ = response.Body.Close()
	if response.Header.Get("Authorization") != "" || response.Header.Get("Location") != "" || response.Header.Get("Set-Cookie") != "" ||
		strings.Contains(string(responseBody), "reflected-provider-secret") {
		t.Fatalf("upstream handshake authority was reflected: headers=%v body=%q", response.Header, responseBody)
	}
	if _, err := os.Stat(evidencePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("handshake fallback unexpectedly created provider evidence: %v", err)
	}
	if _, err := os.Stat(AttemptJournalPath(evidencePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("handshake fallback unexpectedly created provider WAL: %v", err)
	}
}

func TestWebSocketRoundsRecordVerifiedTLSIdentityForApprovedOrigin(t *testing.T) {
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("z", 48)
	upstreamDone := make(chan struct{})
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer close(upstreamDone)
		connection, err := (&websocket.Upgrader{}).Upgrade(writer, request, http.Header{"OpenAI-Model": []string{"gpt-5.6-sol"}})
		if err != nil {
			return
		}
		defer connection.Close()
		for index, responseID := range []string{"tls-warm", "tls-inference"} {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
			writeTestWebSocketJSON(t, connection, map[string]any{
				"type": "response.created", "response": map[string]any{"id": responseID, "model": "gpt-5.6-sol"},
			})
			completed := map[string]any{
				"id": responseID, "model": "gpt-5.6-sol", "status": "completed", "output": []any{},
			}
			if index == 1 {
				completed["usage"] = map[string]any{
					"input_tokens": 2, "input_tokens_details": map[string]any{"cached_tokens": 1}, "output_tokens": 1,
				}
			}
			writeTestWebSocketJSON(t, connection, map[string]any{"type": "response.completed", "response": completed})
		}
	}))
	defer upstream.Close()

	const approvedOrigin = "https://example.com"
	const semantics = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	handler, err := NewHandler(Config{
		Upstream: approvedOrigin, ApprovedOrigin: approvedOrigin,
		EndpointSemanticsSHA256: semantics, RequireTLSPeerEvidence: true,
		EvidencePath: evidencePath, AccessPath: accessPath, Credential: "provider-secret",
		ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
		WebSocketDialer: tlsWebSocketDialerForOrigin(upstream, "example.com"),
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+accessPath+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"type":"response.create","model":"gpt-5.6-sol","service_tier":"default","reasoning":{"effort":"xhigh"},"store":false,"stream":true,"generate":false,"input":[],"tools":[]}`
	if err := client.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		t.Fatal(err)
	}
	readUntilCompleted(t, client)
	inference := `{"type":"response.create","model":"gpt-5.6-sol","service_tier":"default","reasoning":{"effort":"xhigh"},"store":false,"stream":true,"previous_response_id":"tls-warm","input":[],"tools":[]}`
	if err := client.WriteMessage(websocket.TextMessage, []byte(inference)); err != nil {
		t.Fatal(err)
	}
	readUntilCompleted(t, client)
	_ = client.Close()
	<-upstreamDone
	waitForEvidenceRecords(t, evidencePath, 2)
	records := readEvidenceRecords(t, evidencePath)
	record := records[0]
	leaf := upstream.Certificate()
	if record.ApprovedOrigin != approvedOrigin || record.SemanticsSHA256 != semantics ||
		record.TLSServerName != "example.com" || !record.TLSVerified || record.TLSObservedAt.IsZero() || record.TLSObservedAt.Location() != time.UTC ||
		record.TLSPeerLeafCertSHA256 != hashOpaque(string(leaf.Raw)) ||
		record.TLSPeerSPKISHA256 != hashOpaque(string(leaf.RawSubjectPublicKeyInfo)) || !record.ProtocolValid {
		t.Fatalf("WebSocket TLS evidence=%#v", record)
	}
	if !records[1].TLSVerified || records[1].TLSObservedAt != record.TLSObservedAt ||
		records[1].TLSPeerLeafCertSHA256 != record.TLSPeerLeafCertSHA256 ||
		records[1].TLSPeerSPKISHA256 != record.TLSPeerSPKISHA256 || !records[1].ProtocolValid {
		t.Fatalf("reused WebSocket TLS evidence first=%#v second=%#v", record, records[1])
	}
	handler.shutdownWebSockets()
	handler.webSocketWG.Wait()
	if err := handler.SealEvidence(); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEvidenceSeal(evidencePath, testRunIdentity); err != nil {
		t.Fatal(err)
	}
}

func TestWebSocketCodexServiceTierOmissionIsInjectedWithExactDiffEvidence(t *testing.T) {
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("j", 48)
	upstreamPayload := make(chan []byte, 1)
	upstreamDone := make(chan struct{})
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer close(upstreamDone)
		connection, err := (&websocket.Upgrader{}).Upgrade(writer, request, http.Header{"OpenAI-Model": []string{"gpt-5.6-sol"}})
		if err != nil {
			return
		}
		defer connection.Close()
		_, payload, err := connection.ReadMessage()
		if err != nil {
			return
		}
		upstreamPayload <- append([]byte(nil), payload...)
		writeTestWebSocketJSON(t, connection, map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id": "codex-tier-warm", "model": "gpt-5.6-sol",
				"headers": map[string]any{"x-request-id": "codex-tier-request"},
			},
		})
		writeTestWebSocketJSON(t, connection, map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id": "codex-tier-warm", "model": "gpt-5.6-sol", "status": "completed", "output": []any{},
			},
		})
	}))
	defer upstream.Close()

	config := configureCodexStaticProof(t, Config{
		Upstream: upstream.URL, EvidencePath: evidencePath, AccessPath: accessPath,
		Credential: "provider-secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh",
		RunIdentity: testRunIdentity, WebSocketDialer: tlsWebSocketDialer(upstream),
	})
	handler, err := NewHandler(config)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+accessPath+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(" {\n\"type\":\"response.create\",\"model\":\"gpt-5.6-sol\",\"reasoning\":{\"effort\":\"xhigh\"},\"store\":false,\"stream\":true,\"generate\":false,\"input\":[],\"tools\":[],\"sentinel\":9007199254740993\n} ")
	if err := client.WriteMessage(websocket.TextMessage, original); err != nil {
		t.Fatal(err)
	}
	readUntilCompleted(t, client)
	_ = client.Close()
	<-upstreamDone
	forwarded := <-upstreamPayload
	originalObject := decodeTestJSONObject(t, original)
	forwardedObject := decodeTestJSONObject(t, forwarded)
	if forwardedObject["service_tier"] != "default" {
		t.Fatalf("forwarded WebSocket service tier = %#v", forwardedObject["service_tier"])
	}
	delete(forwardedObject, "service_tier")
	if !reflect.DeepEqual(originalObject, forwardedObject) {
		t.Fatalf("WebSocket non-tier semantics changed:\noriginal=%#v\nforwarded=%#v", originalObject, forwardedObject)
	}
	waitForEvidenceRecords(t, evidencePath, 1)
	record := readEvidenceRecords(t, evidencePath)[0]
	if !record.ProtocolValid || record.Disposition != "prewarm_transport" || record.ServiceTierTransformation != serviceTierTransformationInjectDefault ||
		!record.ServiceTierTransformationExactDiff || !record.ServiceTierComparable || record.OriginalServiceTierPresent ||
		!record.ForwardedServiceTierPresent || record.ForwardedServiceTier != "default" ||
		record.OriginalRequestBodySHA256 != stableSHA256Bytes(original) || record.ForwardedRequestBodySHA256 != stableSHA256Bytes(forwarded) ||
		record.OriginalRequestWithoutServiceTierSHA256 != record.ForwardedRequestWithoutServiceTierSHA256 ||
		!isLowerHex64(record.ServiceTierTransformationProofSHA256) || !isLowerHex64(record.ClientCanonicalizationStaticProofSHA256) {
		t.Fatalf("WebSocket service-tier transformation evidence = %#v", record)
	}
	handler.shutdownWebSockets()
	handler.webSocketWG.Wait()
}

func TestDefaultWebSocketDialerIsDirectAndTLS12Minimum(t *testing.T) {
	dialer := newDefaultWebSocketDialer("example.com")
	if dialer == websocket.DefaultDialer || dialer.Proxy != nil || dialer.TLSClientConfig == nil ||
		dialer.TLSClientConfig.MinVersion < tls.VersionTLS12 || dialer.TLSClientConfig.ServerName != "example.com" {
		t.Fatalf("default WebSocket dialer=%#v", dialer)
	}
}

func tlsWebSocketDialerForOrigin(server *httptest.Server, serverName string) *websocket.Dialer {
	transport, _ := server.Client().Transport.(*http.Transport)
	config := transport.TLSClientConfig.Clone()
	config.ServerName = serverName
	return &websocket.Dialer{
		TLSClientConfig: config,
		NetDialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
}

func tlsWebSocketDialer(server *httptest.Server) *websocket.Dialer {
	transport, _ := server.Client().Transport.(*http.Transport)
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if transport != nil && transport.TLSClientConfig != nil {
		config = transport.TLSClientConfig.Clone()
	}
	return &websocket.Dialer{TLSClientConfig: config, EnableCompression: true}
}

func decodeTestJSONMap(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func writeTestWebSocketJSON(t *testing.T, connection *websocket.Conn, payload any) {
	t.Helper()
	if event, ok := payload.(map[string]any); ok && event["type"] == "response.completed" {
		if response, ok := event["response"].(map[string]any); ok {
			if _, present := response["service_tier"]; !present {
				response["service_tier"] = "default"
			}
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.WriteMessage(websocket.TextMessage, encoded); err != nil {
		t.Fatal(err)
	}
}

func readUntilCompleted(t *testing.T, connection *websocket.Conn) {
	t.Helper()
	for {
		_, payload, err := connection.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		decoded := decodeTestJSONMap(t, payload)
		if decoded["type"] == "response.completed" {
			return
		}
	}
}

func waitForEvidenceRecords(t *testing.T, path string, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		records, err := readJSONLines[Record](path)
		if err == nil && len(records) == count {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	records, err := readJSONLines[Record](path)
	t.Fatalf("evidence records=%d want=%d err=%v", len(records), count, err)
}
