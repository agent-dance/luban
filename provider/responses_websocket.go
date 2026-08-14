package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	maxResponsesWebSocketSessions = 32
	maxResponsesWebSocketFrame    = 32 << 20
	responsesWebSocketRotateAfter = 55 * time.Minute
	responsesWebSocketConnectWait = 15 * time.Second
	responsesWebSocketWriteWait   = 30 * time.Second
)

var _ CloseProvider = (*ResponsesProvider)(nil)

// responsesWebSocketSession is one loop-private affinity lane. The token makes
// the official one-in-flight-per-connection constraint explicit without
// serializing unrelated main, fork, or subagent sessions.
type responsesWebSocketSession struct {
	turn chan struct{}

	mu              sync.Mutex
	wire            *responsesWebSocketWire
	retired         bool
	model           string
	continuationAge uint64
	credentialAge   uint64
	lastResponseID  string
	envelopeDigest  [sha256.Size]byte
	lastUsed        time.Time
}

func newResponsesWebSocketSession() *responsesWebSocketSession {
	session := &responsesWebSocketSession{turn: make(chan struct{}, 1)}
	session.turn <- struct{}{}
	return session
}

func (session *responsesWebSocketSession) acquire(ctx context.Context) error {
	select {
	case <-session.turn:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (session *responsesWebSocketSession) release() {
	session.turn <- struct{}{}
}

func (session *responsesWebSocketSession) close(retire bool) {
	session.mu.Lock()
	if retire {
		session.retired = true
	}
	wire := session.wire
	session.wire = nil
	session.lastResponseID = ""
	session.envelopeDigest = [sha256.Size]byte{}
	session.mu.Unlock()
	if wire != nil {
		wire.close()
	}
}

func (session *responsesWebSocketSession) invalidateWire(wire *responsesWebSocketWire) {
	session.mu.Lock()
	if session.wire == wire {
		session.wire = nil
		session.lastResponseID = ""
		session.envelopeDigest = [sha256.Size]byte{}
	}
	session.mu.Unlock()
	if wire != nil {
		wire.close()
	}
}

func (session *responsesWebSocketSession) clearChain(wire *responsesWebSocketWire) {
	session.mu.Lock()
	if session.wire == wire {
		session.lastResponseID = ""
		session.envelopeDigest = [sha256.Size]byte{}
	}
	session.mu.Unlock()
}

func (session *responsesWebSocketSession) commit(
	wire *responsesWebSocketWire,
	responseID string,
	model string,
	continuationAge uint64,
	credentialAge uint64,
	envelopeDigest [sha256.Size]byte,
) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.retired || session.wire != wire {
		return
	}
	session.model = model
	session.continuationAge = continuationAge
	session.credentialAge = credentialAge
	session.lastResponseID = responseID
	session.envelopeDigest = envelopeDigest
	session.lastUsed = time.Now()
}

// responsesWebSocketWire owns the sole reader goroutine for one physical
// socket. It keeps reading between turns so an idle server close is observable
// before the next response.create instead of being mistaken for a healthy
// reusable connection.
type responsesWebSocketWire struct {
	conn        *websocket.Conn
	incoming    chan sseEvent
	done        chan struct{}
	closing     chan struct{}
	closeOnce   sync.Once
	connectedAt time.Time

	activityMu sync.Mutex
	inFlight   bool
	active     bool
	watchdog   streamWatchdogConfig
}

func newResponsesWebSocketWire(conn *websocket.Conn) *responsesWebSocketWire {
	wire := &responsesWebSocketWire{
		conn:        conn,
		incoming:    make(chan sseEvent, 512),
		done:        make(chan struct{}),
		closing:     make(chan struct{}),
		connectedAt: time.Now(),
		watchdog:    normalizeStreamWatchdogConfig(responsesStreamWatchdogConfig()),
	}
	conn.SetReadLimit(maxResponsesWebSocketFrame)
	defaultPingHandler := conn.PingHandler()
	conn.SetPingHandler(func(payload string) error {
		wire.markActivity()
		return defaultPingHandler(payload)
	})
	conn.SetPongHandler(func(string) error {
		wire.markActivity()
		return nil
	})
	go wire.readPump()
	return wire
}

func (wire *responsesWebSocketWire) close() {
	if wire == nil {
		return
	}
	wire.closeOnce.Do(func() {
		close(wire.closing)
		_ = wire.conn.Close()
	})
}

func (wire *responsesWebSocketWire) closed() bool {
	if wire == nil {
		return true
	}
	select {
	case <-wire.done:
		return true
	default:
		return false
	}
}

func (wire *responsesWebSocketWire) reusable() bool {
	if wire == nil || wire.closed() || time.Since(wire.connectedAt) >= responsesWebSocketRotateAfter {
		return false
	}
	// Any frame outside an active request means the connection's sequential
	// protocol state is no longer one we can prove. Discard the connection and
	// recover from full local history on a fresh socket.
	select {
	case <-wire.incoming:
		return false
	default:
		return true
	}
}

func (wire *responsesWebSocketWire) beginResponse() error {
	if wire.closed() {
		return i18n.NewError(i18n.KeyProviderResponsesWebSocketProtocolInvalid)
	}
	wire.activityMu.Lock()
	wire.inFlight = true
	wire.active = false
	deadline := time.Now().Add(wire.watchdog.initialIdle)
	wire.activityMu.Unlock()
	return wire.conn.SetReadDeadline(deadline)
}

func (wire *responsesWebSocketWire) endResponse() {
	wire.activityMu.Lock()
	wire.inFlight = false
	wire.active = false
	wire.activityMu.Unlock()
	_ = wire.conn.SetReadDeadline(time.Time{})
}

func (wire *responsesWebSocketWire) markActivity() {
	wire.activityMu.Lock()
	defer wire.activityMu.Unlock()
	if !wire.inFlight {
		return
	}
	wire.active = true
	_ = wire.conn.SetReadDeadline(time.Now().Add(wire.watchdog.activeIdle))
}

func (wire *responsesWebSocketWire) timeoutError() error {
	wire.activityMu.Lock()
	defer wire.activityMu.Unlock()
	phase := streamWatchdogAwaitingOutput
	idleFor := wire.watchdog.initialIdle
	if wire.active {
		phase = streamWatchdogActive
		idleFor = wire.watchdog.activeIdle
	}
	return &StreamIdleTimeoutError{Phase: phase, IdleFor: idleFor}
}

func (wire *responsesWebSocketWire) readPump() {
	defer close(wire.incoming)
	defer close(wire.done)
	for {
		messageType, payload, err := wire.conn.ReadMessage()
		if err != nil {
			select {
			case <-wire.closing:
				return
			default:
			}
			var netError net.Error
			if ok := errors.As(err, &netError); ok && netError.Timeout() {
				err = wire.timeoutError()
			}
			wire.sendReadError(err)
			return
		}
		if messageType != websocket.TextMessage {
			wire.sendReadError(i18n.NewError(i18n.KeyProviderResponsesWebSocketProtocolInvalid))
			return
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if len(payload) == 0 || json.Unmarshal(payload, &envelope) != nil || strings.TrimSpace(envelope.Type) == "" {
			wire.sendReadError(i18n.NewError(i18n.KeyProviderResponsesWebSocketProtocolInvalid))
			return
		}
		wire.markActivity()
		select {
		case wire.incoming <- sseEvent{Type: envelope.Type, Data: string(payload)}:
		case <-wire.closing:
			return
		}
	}
}

func (wire *responsesWebSocketWire) sendReadError(err error) {
	select {
	case wire.incoming <- sseEvent{Type: "error", Err: err}:
	case <-wire.closing:
	}
}

func (p *ResponsesProvider) resetResponsesWebSocketSessions() {
	p.wsMu.Lock()
	sessions := make([]*responsesWebSocketSession, 0, len(p.wsSessions))
	for _, session := range p.wsSessions {
		sessions = append(sessions, session)
	}
	p.wsSessions = nil
	p.wsMu.Unlock()
	for _, session := range sessions {
		session.close(true)
	}
}

// Close releases persistent Responses sockets. It is intentionally optional on
// the Provider interface so existing providers remain source-compatible.
func (p *ResponsesProvider) Close() error {
	p.resetResponsesWebSocketSessions()
	return nil
}

func (p *ResponsesProvider) responsesWebSocketSession(lineage string) *responsesWebSocketSession {
	digest := sha256.Sum256([]byte(strings.TrimSpace(lineage)))
	key := string(digest[:])
	p.wsMu.Lock()
	defer p.wsMu.Unlock()
	if session := p.wsSessions[key]; session != nil {
		return session
	}
	if len(p.wsSessions) >= maxResponsesWebSocketSessions {
		return nil
	}
	if p.wsSessions == nil {
		p.wsSessions = make(map[string]*responsesWebSocketSession)
	}
	session := newResponsesWebSocketSession()
	p.wsSessions[key] = session
	return session
}

func responsesWebSocketEnvelopeDigest(params Params, model string) [sha256.Size]byte {
	payload := map[string]any{
		"model":                      model,
		"max_tokens":                 params.MaxTokens,
		"max_output_tokens_override": params.MaxOutputTokensOverride,
		"system_blocks":              params.SystemTextBlocks(),
		"tools":                      params.Tools,
		"extra_tool_schemas":         params.ExtraToolSchemas,
		"tool_choice":                params.ToolChoice,
		"conversation":               params.Conversation,
		"truncation":                 params.Truncation,
		"prompt_cache_key":           params.PromptCacheKey,
		"use_prompt_cache":           params.UsePromptCache,
		"reasoning_effort":           params.ReasoningEffort,
		"service_tier":               params.ServiceTier,
		"thinking":                   params.Thinking,
		"task_budget":                params.TaskBudget,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return [sha256.Size]byte{}
	}
	return sha256.Sum256(encoded)
}

// createResponsesWebSocketStream returns safeHTTPFallback only when no
// response.create was written. After a write, failure is ambiguous and must
// consume the caller's normal replay budget rather than silently duplicating a
// potentially billable generation over HTTP.
func (p *ResponsesProvider) createResponsesWebSocketStream(
	ctx context.Context,
	params Params,
	profile responsesRequestProfile,
) (<-chan types.StreamEvent, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session := p.responsesWebSocketSession(params.ContinuationLineage)
	if session == nil {
		return nil, true, i18n.NewError(i18n.KeyProviderResponsesWebSocketCapacity)
	}
	if err := session.acquire(ctx); err != nil {
		return nil, false, err
	}
	release := true
	defer func() {
		if release {
			session.release()
		}
	}()

	model := profile.modelFor(params)
	envelopeDigest := responsesWebSocketEnvelopeDigest(params, model)
	var wire *responsesWebSocketWire
	prevID := ""

	session.mu.Lock()
	if session.retired {
		session.mu.Unlock()
		return nil, false, i18n.NewError(i18n.KeyProviderResponsesWebSocketProtocolInvalid)
	}
	if session.model != "" && (session.model != model ||
		session.continuationAge != params.ContinuationEpoch ||
		session.credentialAge != profile.credentialEpoch || params.ContinuationReset) {
		wire = session.wire
		session.wire = nil
		session.lastResponseID = ""
		session.envelopeDigest = [sha256.Size]byte{}
	}
	if wire != nil {
		wire.close()
		wire = nil
	}
	wire = session.wire
	if wire != nil && !wire.reusable() {
		session.wire = nil
		session.lastResponseID = ""
		session.envelopeDigest = [sha256.Size]byte{}
		wire.close()
		wire = nil
	}
	if wire != nil && params.PreviousResponseID != "" &&
		params.PreviousResponseID == session.lastResponseID &&
		session.envelopeDigest == envelopeDigest {
		prevID = params.PreviousResponseID
	} else {
		session.lastResponseID = ""
		session.envelopeDigest = [sha256.Size]byte{}
	}
	session.mu.Unlock()

	body, requestModel, responsesLite, err := p.buildResponsesRequestBody(params, profile, prevID, responsesTransportWebSocket)
	if err != nil {
		return nil, false, err
	}
	if responsesLite {
		return nil, false, i18n.NewError(i18n.KeyProviderResponsesWebSocketProtocolInvalid)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, false, i18n.WrapError(i18n.KeyProviderRequestEncodeFailed, err)
	}

	if wire == nil {
		endpoint, endpointErr := responsesWebSocketURL(profile.baseURL)
		if endpointErr != nil {
			return nil, false, endpointErr
		}
		wire, err = dialResponsesWebSocket(ctx, endpoint, profile)
		if err != nil {
			return nil, true, err
		}
		session.mu.Lock()
		if session.retired {
			session.mu.Unlock()
			wire.close()
			return nil, false, i18n.NewError(i18n.KeyProviderResponsesWebSocketProtocolInvalid)
		}
		session.wire = wire
		session.model = model
		session.continuationAge = params.ContinuationEpoch
		session.credentialAge = profile.credentialEpoch
		session.mu.Unlock()
	}

	if err := wire.beginResponse(); err != nil {
		session.invalidateWire(wire)
		// No response.create frame has been written, so replaying the complete
		// local history over HTTP cannot duplicate a generation.
		return nil, true, err
	}
	_ = wire.conn.SetWriteDeadline(responsesWebSocketWriteDeadline(ctx))
	err = wire.conn.WriteMessage(websocket.TextMessage, encoded)
	_ = wire.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		wire.endResponse()
		session.invalidateWire(wire)
		return nil, false, i18n.WrapError(i18n.KeyProviderRequestFailed, err, "Responses WebSocket")
	}

	out := make(chan types.StreamEvent)
	release = false
	go p.forwardResponsesWebSocketStream(
		ctx, out, session, wire, requestModel, profile.semantics,
		params.ContinuationEpoch, profile.credentialEpoch, envelopeDigest, params.ServiceTier,
	)
	return out, false, nil
}

func responsesWebSocketURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSuffix(strings.TrimSpace(baseURL), "/") + "/responses")
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", i18n.NewError(i18n.KeyProviderResponsesWebSocketEndpointInvalid)
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", i18n.NewError(i18n.KeyProviderResponsesWebSocketEndpointInvalid)
	}
	return parsed.String(), nil
}

func dialResponsesWebSocket(ctx context.Context, endpoint string, profile responsesRequestProfile) (*responsesWebSocketWire, error) {
	headers := make(http.Header, len(profile.headers)+1)
	for key, value := range profile.headers {
		lower := strings.ToLower(strings.TrimSpace(key))
		if lower == "authorization" || lower == "connection" || lower == "upgrade" ||
			strings.HasPrefix(lower, "sec-websocket-") {
			continue
		}
		headers.Set(key, value)
	}
	if profile.apiKey != "" {
		headers.Set("Authorization", "Bearer "+profile.apiKey)
	}
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = responsesWebSocketConnectWait
	dialer.EnableCompression = true
	connection, response, err := dialer.DialContext(ctx, endpoint, headers)
	if response != nil && response.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderRequestFailed, err, "Responses WebSocket")
	}
	return newResponsesWebSocketWire(connection), nil
}

func responsesWebSocketWriteDeadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(responsesWebSocketWriteWait)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		return contextDeadline
	}
	return deadline
}

type responsesWebSocketTerminal struct {
	eventType string
	err       error
}

func feedResponsesWebSocketEvents(
	ctx context.Context,
	wire *responsesWebSocketWire,
	destination chan<- sseEvent,
) responsesWebSocketTerminal {
	defer close(destination)
	for {
		select {
		case <-ctx.Done():
			return responsesWebSocketTerminal{err: ctx.Err()}
		case event, ok := <-wire.incoming:
			if !ok {
				event = sseEvent{Type: "error", Err: i18n.NewError(i18n.KeyRuntimeResponsesStreamIncomplete)}
				select {
				case destination <- event:
				case <-ctx.Done():
				}
				return responsesWebSocketTerminal{err: event.Err}
			}
			select {
			case destination <- event:
			case <-ctx.Done():
				return responsesWebSocketTerminal{err: ctx.Err()}
			}
			switch event.Type {
			case "response.completed", "response.failed", "response.incomplete", "error":
				return responsesWebSocketTerminal{eventType: event.Type, err: event.Err}
			}
		}
	}
}

func (p *ResponsesProvider) forwardResponsesWebSocketStream(
	ctx context.Context,
	out chan<- types.StreamEvent,
	session *responsesWebSocketSession,
	wire *responsesWebSocketWire,
	requestModel string,
	semantics ResponsesSemantics,
	continuationAge uint64,
	credentialAge uint64,
	envelopeDigest [sha256.Size]byte,
	expectedServiceTier ServiceTier,
) {
	defer close(out)
	defer session.release()

	wireEvents := make(chan sseEvent, 64)
	terminalResult := make(chan responsesWebSocketTerminal, 1)
	go func() {
		terminalResult <- feedResponsesWebSocketEvents(ctx, wire, wireEvents)
	}()
	parsed := make(chan types.StreamEvent, 64)
	go func() {
		defer close(parsed)
		processResponsesEventsForRequest(ctx, nil, wireEvents, parsed, requestModel, semantics, false, true, expectedServiceTier)
	}()

	committed := false
	for event := range parsed {
		select {
		case out <- event:
		case <-ctx.Done():
			wire.endResponse()
			session.invalidateWire(wire)
			return
		}
		if event.Type == types.EventMessageStop {
			receipt := event.ProviderCommitReceipt
			committed = receipt != nil && receipt.ResponseStatus == "completed" && receipt.ToolsAuthorized
			if committed && event.ResponseID != "" {
				session.commit(wire, event.ResponseID, requestModel, continuationAge, credentialAge, envelopeDigest)
			} else {
				session.clearChain(wire)
			}
		}
	}
	terminal := <-terminalResult
	wire.endResponse()
	if !committed || terminal.eventType != "response.completed" || terminal.err != nil {
		session.invalidateWire(wire)
	}
}
