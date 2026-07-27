package evidenceproxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketDialer is the small part of gorilla/websocket.Dialer required by
// the benchmark proxy. It is configurable so the fake provider can prove the
// exact handshake and frame ordering without weakening production TLS.
type WebSocketDialer interface {
	DialContext(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error)
}

type webSocketRelay struct {
	handler         *Handler
	downstream      *websocket.Conn
	upstream        *websocket.Conn
	connectionHash  string
	handshakeStatus int
	handshakeModel  string
	connectedAt     time.Time
	requestSequence uint64
	lastResponseID  string
	lineageHash     string
	tlsPeer         tlsPeerEvidence
	closeOnce       sync.Once
	done            chan struct{}
}

type webSocketFrame struct {
	messageType int
	payload     []byte
	err         error
}

type webSocketActiveRound struct {
	record      Record
	collector   *streamCollector
	replayPlan  continuationReplayPlan
	replayValid bool
	responseID  string
	requestID   string
}

type webSocketRequestEnvelope struct {
	typeName                    string
	generateSpecified           bool
	generate                    bool
	previousResponseID          string
	previousResponseIDSpecified bool
	lineageValue                string
}

type webSocketResponseEvent struct {
	valid     bool
	typeName  string
	response  map[string]any
	headers   []map[string]any
	status    int
	model     string
	requestID string
}

func isWebSocketUpgrade(request *http.Request) bool {
	return websocket.IsWebSocketUpgrade(request)
}

func (handler *Handler) serveWebSocket(writer http.ResponseWriter, request *http.Request, providerPath string) {
	// Register before any possible HTTP hijack. Server.Shutdown waits for the
	// pre-upgrade handler, so no Add can race the final Wait in Run.
	handler.webSocketWG.Add(1)
	defer handler.webSocketWG.Done()
	upstreamURL := *handler.target
	switch upstreamURL.Scheme {
	case "https":
		upstreamURL.Scheme = "wss"
	case "http":
		upstreamURL.Scheme = "ws"
	default:
		// NewHandler only accepts HTTPS, so this is a fail-closed invariant guard.
		http.Error(writer, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	upstreamURL.Path = joinURLPath(handler.target.Path, providerPath)
	upstreamURL.RawPath = ""

	upstreamHeaders := request.Header.Clone()
	removeHopHeaders(upstreamHeaders)
	removeWebSocketHandshakeHeaders(upstreamHeaders)
	removeContinuationHeaders(upstreamHeaders)
	upstreamHeaders.Set("Authorization", "Bearer "+handler.credential)

	dialer := handler.webSocketDialer
	if dialer == nil {
		dialer = newDefaultWebSocketDialer(handler.tlsServerName)
	}
	upstream, handshake, err := dialer.DialContext(request.Context(), upstreamURL.String(), upstreamHeaders)
	upstreamHeaders.Del("Authorization")
	if err != nil {
		relayWebSocketHandshakeFailure(writer, handshake)
		return
	}
	if handshake == nil {
		_ = upstream.Close()
		http.Error(writer, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	if handshake.Body != nil {
		_ = handshake.Body.Close()
	}
	tlsEvidence, err := handler.projectTLSPeerEvidence(webSocketTLSConnectionState(upstream, handshake))
	if err != nil {
		_ = upstream.Close()
		http.Error(writer, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	downstreamHeaders := projectedWebSocketResponseHeaders(handshake.Header)
	upgrader := websocket.Upgrader{EnableCompression: true}
	downstream, err := upgrader.Upgrade(writer, request, downstreamHeaders)
	if err != nil {
		_ = upstream.Close()
		return
	}
	upstream.EnableWriteCompression(true)
	downstream.EnableWriteCompression(true)
	upstream.SetReadLimit(handler.maxRequestBytes)
	downstream.SetReadLimit(handler.maxRequestBytes)

	connectionNonce := make([]byte, 32)
	if _, err := rand.Read(connectionNonce); err != nil {
		_ = upstream.Close()
		_ = downstream.Close()
		return
	}
	connectionHash := handler.hashBindingValue(connectionNonce)
	zero(connectionNonce)
	relay := &webSocketRelay{
		handler: handler, downstream: downstream, upstream: upstream,
		connectionHash: connectionHash, handshakeStatus: handshake.StatusCode,
		handshakeModel: firstHeader(handshake.Header, "openai-model", "x-openai-model"),
		connectedAt:    time.Now().UTC(), tlsPeer: tlsEvidence, done: make(chan struct{}),
	}
	handler.registerWebSocket(relay)
	defer handler.unregisterWebSocket(relay)
	relay.run(providerPath)
}

func newDefaultWebSocketDialer(serverName string) *websocket.Dialer {
	clone := *websocket.DefaultDialer
	clone.Proxy = nil
	clone.EnableCompression = true
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
	if clone.TLSClientConfig != nil {
		tlsConfig = clone.TLSClientConfig.Clone()
		if tlsConfig.MinVersion == 0 || tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
		tlsConfig.ServerName = serverName
	}
	clone.TLSClientConfig = tlsConfig
	return &clone
}

func relayWebSocketHandshakeFailure(writer http.ResponseWriter, response *http.Response) {
	if response == nil {
		http.Error(writer, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	status := http.StatusBadGateway
	if response.StatusCode == http.StatusUpgradeRequired {
		status = http.StatusUpgradeRequired
	}
	http.Error(writer, http.StatusText(status), status)
}

type tlsConnectionStateProvider interface {
	ConnectionState() tls.ConnectionState
}

func webSocketTLSConnectionState(connection *websocket.Conn, response *http.Response) *tls.ConnectionState {
	if connection != nil {
		if provider, ok := connection.NetConn().(tlsConnectionStateProvider); ok {
			state := provider.ConnectionState()
			return &state
		}
	}
	if response != nil {
		return response.TLS
	}
	return nil
}

func projectedWebSocketResponseHeaders(source http.Header) http.Header {
	result := make(http.Header)
	for _, name := range []string{
		"OpenAI-Model", "X-OpenAI-Model", "X-Reasoning-Included",
		"X-Models-Etag", "X-Codex-Turn-State",
	} {
		for _, value := range source.Values(name) {
			result.Add(name, value)
		}
	}
	return result
}

func removeWebSocketHandshakeHeaders(headers http.Header) {
	for _, name := range []string{
		"Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions",
		"Sec-WebSocket-Protocol",
	} {
		headers.Del(name)
	}
}

func (handler *Handler) registerWebSocket(relay *webSocketRelay) {
	handler.webSocketMu.Lock()
	handler.webSocketRelays[relay] = struct{}{}
	handler.webSocketMu.Unlock()
}

func (handler *Handler) unregisterWebSocket(relay *webSocketRelay) {
	relay.close()
	handler.webSocketMu.Lock()
	delete(handler.webSocketRelays, relay)
	handler.webSocketMu.Unlock()
}

func (handler *Handler) shutdownWebSockets() {
	handler.webSocketMu.Lock()
	relays := make([]*webSocketRelay, 0, len(handler.webSocketRelays))
	for relay := range handler.webSocketRelays {
		relays = append(relays, relay)
	}
	handler.webSocketMu.Unlock()
	for _, relay := range relays {
		relay.close()
	}
}

func (relay *webSocketRelay) close() {
	relay.closeOnce.Do(func() {
		close(relay.done)
		_ = relay.downstream.Close()
		_ = relay.upstream.Close()
	})
}

func (relay *webSocketRelay) run(providerPath string) {
	clientFrames := make(chan webSocketFrame, 1)
	serverFrames := make(chan webSocketFrame, 1)
	go readWebSocketFrames(relay.downstream, clientFrames, relay.done)
	go readWebSocketFrames(relay.upstream, serverFrames, relay.done)

	var active *webSocketActiveRound
	for {
		select {
		case frame := <-clientFrames:
			if frame.err != nil {
				if active != nil {
					relay.finishRound(active, "downstream_websocket_closed")
				}
				return
			}
			if active != nil {
				relay.finishRound(active, "overlapping_websocket_response_create")
				relay.writeProtocolError("overlapping_websocket_response_create")
				return
			}
			if frame.messageType != websocket.TextMessage {
				relay.writeProtocolError("websocket_request_not_text")
				return
			}
			prepared, forwardedPayload, accepted := relay.prepareRound(providerPath, frame.payload)
			if !accepted {
				zero(frame.payload)
				zero(forwardedPayload)
				return
			}
			active = prepared
			if err := relay.handler.appendAttemptStart(AttemptStartJournalEntry{
				SchemaVersion: "agentic-bench/provider-attempt-start-v1",
				RunIdentity:   relay.handler.runIdentity, Round: active.record.Round,
				StartedAt: active.record.StartedAt, Transport: active.record.Transport,
				ProviderAttemptKind:      active.record.ProviderAttemptKind,
				WebSocketConnectionHash:  active.record.WebSocketConnectionHash,
				WebSocketRequestSequence: active.record.WebSocketRequestSequence,
			}); err != nil {
				relay.handler.recordPersistenceError(err)
				zero(frame.payload)
				zero(forwardedPayload)
				relay.writeProtocolError("benchmark_evidence_journal_unavailable")
				return
			}
			active.record.ProviderAttemptStarted = true
			if err := relay.upstream.WriteMessage(websocket.TextMessage, forwardedPayload); err != nil {
				zero(frame.payload)
				zero(forwardedPayload)
				relay.finishRound(active, "upstream_websocket_write")
				return
			}
			zero(frame.payload)
			zero(forwardedPayload)

		case frame := <-serverFrames:
			if frame.err != nil {
				if active != nil {
					relay.finishRound(active, "upstream_websocket_closed")
				}
				return
			}
			if frame.messageType != websocket.TextMessage {
				if active != nil {
					relay.finishRound(active, "upstream_websocket_binary_frame")
				}
				relay.writeProtocolError("upstream_websocket_binary_frame")
				return
			}
			if active != nil {
				active.record.ResponseBytes += int64(len(frame.payload))
				if active.record.FirstResponseByteAt.IsZero() {
					active.record.FirstResponseByteAt = time.Now().UTC()
				}
				event := inspectWebSocketResponseEvent(frame.payload)
				relay.observeResponseEvent(active, event)
				active.collector.consume(frame.payload)
				terminalError := ""
				terminal := false
				switch event.typeName {
				case "response.completed":
					terminal = true
				case "response.failed", "response.incomplete", "error":
					terminal = true
					terminalError = "provider_websocket_error"
				}
				if terminal && !relay.finishRound(active, terminalError) {
					zero(frame.payload)
					return
				}
				if err := relay.downstream.WriteMessage(websocket.TextMessage, frame.payload); err != nil {
					if !terminal {
						relay.finishRound(active, "downstream_websocket_write")
					}
					zero(frame.payload)
					return
				}
				zero(frame.payload)
				if terminal {
					active = nil
				}
				continue
			}
			// Metadata and rate-limit frames may be connection scoped. Preserve
			// them byte-for-byte, but never let them manufacture a provider round.
			if err := relay.downstream.WriteMessage(websocket.TextMessage, frame.payload); err != nil {
				zero(frame.payload)
				return
			}
			zero(frame.payload)

		case <-relay.done:
			if active != nil {
				relay.finishRound(active, "websocket_proxy_shutdown")
			}
			return
		}
	}
}

func readWebSocketFrames(connection *websocket.Conn, destination chan<- webSocketFrame, done <-chan struct{}) {
	for {
		messageType, payload, err := connection.ReadMessage()
		frame := webSocketFrame{messageType: messageType, payload: payload, err: err}
		select {
		case destination <- frame:
		case <-done:
			zero(payload)
			return
		}
		if err != nil {
			return
		}
	}
}

func (relay *webSocketRelay) prepareRound(providerPath string, payload []byte) (*webSocketActiveRound, []byte, bool) {
	envelope, validEnvelope := inspectWebSocketRequestEnvelope(payload)
	if !validEnvelope || envelope.typeName != "response.create" {
		relay.writeProtocolError("websocket_request_invalid")
		return nil, nil, false
	}
	round := int(relay.handler.nextRound.Add(1) - 1)
	sequence := relay.requestSequence
	relay.requestSequence++
	forwardedPayload, transformationEvidence, transformationErr := relay.handler.transformServiceTierRequest(payload)
	metadata := inspectRequest(payload, relay.handler.hashBindingValue)
	serviceTierCanonical, serviceTierRepresentation, serviceTierProof, serviceTierRequestValid := serviceTierRequestEvidence(metadata, relay.handler.agentID, relay.handler.canonicalizationStaticProof)
	serviceTierRequestValid = serviceTierRequestValid && transformationErr == nil &&
		metadata.ServiceTierPresent == transformationEvidence.OriginalServiceTierPresent &&
		metadata.ServiceTier == transformationEvidence.OriginalServiceTier &&
		transformationEvidence.ExactDiff && isLowerHex64(transformationEvidence.ProofSHA256)
	attemptKind := "inference"
	if envelope.generateSpecified && !envelope.generate {
		attemptKind = "prewarm"
	}
	record := Record{
		SchemaVersion: "agentic-bench/provider-http-v6", Round: round,
		RunIdentity: relay.handler.runIdentity, Transport: "websocket",
		ProviderAttemptKind: attemptKind, StartedAt: time.Now().UTC(),
		Method: "WS_TEXT", Path: providerPath, RequestBytes: int64(len(payload)),
		WebSocketConnectionHash:   relay.connectionHash,
		WebSocketRequestSequence:  sequence,
		WebSocketConnectionReused: sequence > 0,
		WebSocketHandshakeStatus:  relay.handshakeStatus,
		WebSocketHandshakeModel:   relay.handshakeModel,
		GenerateSpecified:         envelope.generateSpecified, Generate: envelope.generate,
		RequestedModel: metadata.Model, RequestedReasoningEffort: metadata.ReasoningEffort,
		RequestedReasoningContext:               metadata.ReasoningContext,
		RequestedReasoningMode:                  metadata.ReasoningMode,
		RequestedReasoningModeCanonical:         canonicalReasoningMode(metadata.ReasoningMode),
		RequestedTextVerbosity:                  metadata.TextVerbosity,
		MaxOutputTokensSpecified:                metadata.MaxOutputTokensSpecified,
		MaxOutputTokens:                         metadata.MaxOutputTokens,
		RequestedServiceTier:                    metadata.ServiceTier,
		RequestedServiceTierPresent:             metadata.ServiceTierPresent,
		RequestedServiceTierCanonical:           serviceTierCanonical,
		RequestedServiceTierRepresentation:      serviceTierRepresentation,
		ClientCanonicalizationStaticProofSHA256: serviceTierProof,
		ClientAgentID:                           relay.handler.agentID,
		StoreSpecified:                          metadata.StoreSpecified, Store: metadata.Store,
		PreviousResponseIDPresent:      metadata.PreviousResponseIDPresent,
		PreviousResponseIDHash:         metadata.PreviousResponseIDHash,
		PromptCacheKeyPresent:          metadata.PromptCacheKeyPresent,
		PromptCacheKeyHash:             metadata.PromptCacheKeyHash,
		CachePolicyObserved:            metadata.CachePolicyObserved,
		PromptCacheOptionsPresent:      metadata.PromptCacheOptionsPresent,
		PromptCacheOptionsMode:         metadata.PromptCacheOptionsMode,
		PromptCacheTTLSeconds:          metadata.PromptCacheTTLSeconds,
		PromptCacheRetentionPresent:    metadata.PromptCacheRetentionPresent,
		PromptCacheRetention:           metadata.PromptCacheRetention,
		CacheBreakpointCount:           metadata.CacheBreakpointCount,
		CacheBreakpointPositionHashes:  append([]string(nil), metadata.CacheBreakpointPositionHashes...),
		EncryptedReasoningRequested:    metadata.EncryptedReasoningRequested,
		EncryptedReasoningReplayHashes: append([]string(nil), metadata.EncryptedReasoningReplayHashes...),
		ReplayOutputItemHashes:         append([]string(nil), metadata.ReplayOutputItemHashes...),
		ToolDefinitions:                append([]ToolDefinitionEvidence(nil), metadata.ToolDefinitions...),
		ToolCatalogHash:                metadata.ToolCatalogHash,
		ToolCatalogSemanticSHA256:      metadata.ToolCatalogSemanticSHA256,
		ToolCatalogCanonicalBytes:      metadata.ToolCatalogCanonicalBytes,
		ToolResults:                    append([]ToolResult(nil), metadata.ToolResults...),
		ContinuationLineagePresent:     true,
		ContinuationEpoch:              1,
	}
	applyServiceTierRequestTransformationEvidence(&record, transformationEvidence)
	applyTLSPeerEvidence(&record, relay.tlsPeer)
	lineageHash := relay.connectionHash
	lineageSource := "websocket_connection"
	if envelope.lineageValue != "" {
		lineageHash = relay.handler.hashBindingValue("websocket-client-lineage:" + envelope.lineageValue)
		lineageSource = "websocket_client_metadata"
	}
	record.ContinuationLineageHash = lineageHash
	record.ContinuationLineageSource = lineageSource
	if !metadata.ReasoningModeTypeValid {
		record.RequestedReasoningModeCanonical = "invalid"
	}
	record.EncryptedReasoningReplayCount = len(record.EncryptedReasoningReplayHashes)
	record.ReplayOutputItemCount = len(record.ReplayOutputItemHashes)
	record.ToolDefinitionCount = len(record.ToolDefinitions)
	record.WebSocketChainBound = !envelope.previousResponseIDSpecified ||
		(envelope.previousResponseID != "" && relay.lastResponseID != "" && envelope.previousResponseID == relay.lastResponseID)

	plan, replayValid := relay.handler.planContinuationReplay(
		lineageHash, 1, false, false, metadata.Model,
		record.ReplayOutputItemHashes, record.EncryptedReasoningReplayHashes,
		record.ToolResults, record.ToolCatalogHash,
	)
	replayValid = replayValid && metadata.ToolResultsValid
	record.ContinuationResetUnknown = plan.resetUnknown
	record.ToolCatalogCompared = plan.catalogCompared
	record.ToolCatalogStable = plan.catalogStable
	record.ReplayOutputItemsBound = replayValid && len(record.ReplayOutputItemHashes) > 0
	record.EncryptedReasoningReplayBound = replayValid && len(record.EncryptedReasoningReplayHashes) > 0
	record.ToolResultHistoryValid = replayValid

	errorCode := ""
	switch {
	case transformationErr != nil:
		errorCode = "request_body_transformation_invalid"
	case !metadata.ToolDefinitionsValid:
		errorCode = "tool_catalog_uninspectable"
	case !metadata.CachePolicyValid:
		errorCode = "cache_policy_uninspectable"
	case metadata.Model != relay.handler.expectedModel || metadata.ReasoningEffort != relay.handler.expectedEffort:
		errorCode = "pinned_request_mismatch"
	case !metadata.ReasoningModeTypeValid || record.RequestedReasoningModeCanonical != "standard":
		errorCode = "reasoning_mode_not_comparable"
	case !serviceTierRequestValid:
		errorCode = "service_tier_request_not_comparable"
	case !metadata.StoreSpecified || !metadata.StoreTypeValid || metadata.Store:
		errorCode = "response_storage_not_disabled"
	case relay.lineageHash != "" && relay.lineageHash != lineageHash:
		errorCode = "websocket_lineage_changed_on_connection"
	case envelope.previousResponseIDSpecified != metadata.PreviousResponseIDPresent:
		errorCode = "websocket_previous_response_id_invalid"
	case !record.WebSocketChainBound:
		errorCode = "websocket_previous_response_id_unbound"
	case !replayValid:
		errorCode = "unbound_websocket_output_replay"
	case relay.handshakeModel != "" && relay.handshakeModel != relay.handler.expectedModel:
		errorCode = "served_model_mismatch"
	}
	active := &webSocketActiveRound{
		record: record, replayPlan: plan, replayValid: replayValid,
	}
	active.collector = newStreamCollector(&active.record, relay.handler.hashBindingValue)
	if errorCode != "" {
		active.record.ErrorCode = errorCode
		active.record.Disposition = "experiment_invalid"
		active.record.FinishedAt = time.Now().UTC()
		if err := relay.handler.append(active.record); err != nil {
			relay.handler.recordPersistenceError(err)
		}
		relay.writeProtocolError(errorCode)
		return nil, forwardedPayload, false
	}
	if relay.lineageHash == "" {
		relay.lineageHash = lineageHash
	}
	return active, forwardedPayload, true
}

func inspectWebSocketRequestEnvelope(payload []byte) (webSocketRequestEnvelope, bool) {
	var source map[string]any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&source); err != nil {
		return webSocketRequestEnvelope{}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return webSocketRequestEnvelope{}, false
	}
	envelope := webSocketRequestEnvelope{}
	envelope.typeName, _ = source["type"].(string)
	if value, present := source["generate"]; present {
		envelope.generateSpecified = true
		var ok bool
		envelope.generate, ok = value.(bool)
		if !ok {
			return webSocketRequestEnvelope{}, false
		}
	}
	if value, present := source["previous_response_id"]; present {
		envelope.previousResponseIDSpecified = true
		var ok bool
		envelope.previousResponseID, ok = value.(string)
		if !ok || envelope.previousResponseID == "" {
			return webSocketRequestEnvelope{}, false
		}
	}
	if metadata, ok := source["client_metadata"].(map[string]any); ok {
		for _, key := range []string{"thread_id", "session_id"} {
			if value, _ := metadata[key].(string); value != "" {
				envelope.lineageValue = key + ":" + value
				break
			}
		}
	}
	return envelope, true
}

func inspectWebSocketResponseEvent(payload []byte) webSocketResponseEvent {
	var source map[string]any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if decoder.Decode(&source) != nil {
		return webSocketResponseEvent{}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return webSocketResponseEvent{}
	}
	event := webSocketResponseEvent{valid: true}
	event.typeName, _ = source["type"].(string)
	event.response, _ = source["response"].(map[string]any)
	if headers, ok := source["headers"].(map[string]any); ok {
		event.headers = append(event.headers, headers)
	}
	if headers, ok := event.response["headers"].(map[string]any); ok {
		event.headers = append(event.headers, headers)
	}
	for _, headers := range event.headers {
		if event.model == "" {
			event.model = firstJSONHeader(headers, "openai-model", "x-openai-model")
		}
		if event.requestID == "" {
			event.requestID = firstJSONHeader(headers, "x-request-id", "x-oai-request-id", "request-id", "openai-request-id")
		}
	}
	if event.model == "" {
		event.model, _ = event.response["model"].(string)
	}
	if value, ok := source["status"].(json.Number); ok {
		parsed, _ := strconv.Atoi(value.String())
		event.status = parsed
	}
	return event
}

func firstJSONHeader(headers map[string]any, names ...string) string {
	for headerName, raw := range headers {
		for _, name := range names {
			if !strings.EqualFold(headerName, name) {
				continue
			}
			switch value := raw.(type) {
			case string:
				return value
			case json.Number:
				return value.String()
			}
		}
	}
	return ""
}

func (relay *webSocketRelay) observeResponseEvent(active *webSocketActiveRound, event webSocketResponseEvent) {
	if !event.valid || event.typeName == "" {
		active.record.ErrorCode = "malformed_websocket_event"
		return
	}
	if event.requestID != "" {
		if active.requestID != "" && active.requestID != event.requestID {
			active.record.ErrorCode = "response_request_id_drift"
		}
		active.requestID = event.requestID
		active.record.UpstreamRequestIDHash = hashOpaque(event.requestID)
	}
	if event.model != "" {
		if active.record.WebSocketHandshakeModel != "" && active.record.WebSocketHandshakeModel != event.model {
			active.record.ErrorCode = "response_model_drift"
		}
		if active.record.ResponseCreatedModel != "" && active.record.ResponseCreatedModel != event.model {
			active.record.ErrorCode = "response_model_drift"
		}
		active.record.ResponseCreatedModel = event.model
	}
	if event.typeName == "response.completed" {
		if responseID, _ := event.response["id"].(string); responseID != "" {
			active.responseID = responseID
		}
	}
}

func (relay *webSocketRelay) finishRound(active *webSocketActiveRound, terminalError string) bool {
	active.collector.Close()
	record := &active.record
	record.FinishedAt = time.Now().UTC()
	if record.ResponseCreatedModel == "" {
		record.ResponseCreatedModel = record.WebSocketHandshakeModel
	}
	if record.ResponseModel == "" && record.ResponseCompleted {
		record.ResponseModel = record.ResponseCreatedModel
	}
	record.ResponseServiceTierCanonical = canonicalServiceTier(record.ResponseServiceTier)
	record.ServiceTierComparable = serviceTierComparable(*record)
	if terminalError != "" && record.ErrorCode == "" {
		record.ErrorCode = terminalError
	}
	if record.ErrorCode == "" && (!record.ResponseCompleted || record.ResponseStatus != "completed") {
		record.ErrorCode = "provider_response_not_completed"
	}
	if record.ErrorCode == "" && record.ProviderAttemptKind == "inference" && !validAtomicUsageReceipt(*record) {
		record.ErrorCode = "provider_usage_receipt_incomplete"
	}
	if (record.ResponseCreatedModel != "" && record.ResponseCreatedModel != relay.handler.expectedModel) ||
		(record.ResponseModel != "" && record.ResponseModel != relay.handler.expectedModel) {
		record.ErrorCode = "served_model_mismatch"
		record.Disposition = "experiment_invalid"
	}
	if !localToolCallsBoundToCatalog(record.ToolCalls, record.ToolDefinitions) {
		record.ErrorCode = "tool_call_outside_local_catalog"
		record.Disposition = "experiment_invalid"
	}
	if record.ErrorCode == "response_model_drift" {
		record.Disposition = "experiment_invalid"
	}
	if record.ErrorCode == "" && !record.ServiceTierComparable {
		if record.RequestedServiceTierCanonical != record.ResponseServiceTierCanonical {
			record.ErrorCode = "service_tier_mismatch"
		} else {
			record.ErrorCode = "service_tier_not_comparable"
		}
		record.Disposition = "experiment_invalid"
	}
	usageValid := record.ProviderAttemptKind == "prewarm" || validAtomicUsageReceipt(*record)
	record.ProtocolValid = record.ErrorCode == "" && (!relay.handler.requireTLSPeer || record.TLSVerified) && record.ResponseCompleted &&
		record.ResponseStatus == "completed" && record.ResponseIDHash != "" &&
		record.ResponseModel == relay.handler.expectedModel && usageValid &&
		record.WebSocketChainBound && active.replayValid
	if record.ProtocolValid && !relay.handler.commitContinuationReplay(
		active.replayPlan, record.ResponseOutputItemHashes,
		record.EncryptedReasoningHashes, record.ToolCalls,
	) {
		record.ProtocolValid = false
		record.ErrorCode = "continuation_lineage_commit_conflict"
	}
	if !record.ProtocolValid && record.ErrorCode == "" {
		record.ErrorCode = "incomplete_server_evidence"
	}
	if record.ProtocolValid {
		if record.ProviderAttemptKind == "prewarm" {
			record.Disposition = "prewarm_transport"
		} else {
			record.Disposition = "valid"
		}
		if active.responseID != "" {
			relay.lastResponseID = active.responseID
		}
	} else if record.Disposition == "" {
		record.Disposition = "provider_infra_exclusion"
	}
	if err := relay.handler.append(*record); err != nil {
		relay.handler.recordPersistenceError(err)
		return false
	}
	return true
}

func (relay *webSocketRelay) writeProtocolError(code string) {
	payload, _ := json.Marshal(map[string]any{
		"type": "error", "status": http.StatusBadRequest,
		"error": map[string]any{"code": code, "message": code},
	})
	_ = relay.downstream.WriteMessage(websocket.TextMessage, payload)
	_ = relay.downstream.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.ClosePolicyViolation, code),
		time.Now().Add(time.Second),
	)
	zero(payload)
}

func webSocketURL(target *url.URL, providerPath string) string {
	if target == nil {
		return ""
	}
	result := *target
	if result.Scheme == "https" {
		result.Scheme = "wss"
	}
	result.Path = joinURLPath(target.Path, providerPath)
	return result.String()
}
