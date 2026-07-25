package sdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// ─── mock engine ──────────────────────────────────────────────────────────────

type setModelCall struct {
	sessionID string
	model     string
}

type setThinkingCall struct {
	sessionID    string
	enabled      bool
	budgetTokens int
}

type mockEngine struct {
	mu                  sync.Mutex
	tools               []string
	modelID             string
	queryCh             chan QueryEvent
	lastQuery           QueryRequest
	queryCalled         bool
	interrupted         bool
	interruptedID       string
	setModelCalls       []setModelCall
	setModelErr         error
	setThinkingCalls    []setThinkingCall
	setThinkingErr      error
	resumedSessionIDs   []string
	resumeCount         int
	resumeErr           error
	compactedSessionIDs []string
	compactResult       CompactResult
	compactErr          error
	lastPermission      PermissionHandler
	systemPrompt        string
}

func newMockEngine(tools []string, modelID string) *mockEngine {
	return &mockEngine{
		tools:   tools,
		modelID: modelID,
	}
}

func (e *mockEngine) Query(_ context.Context, req QueryRequest) (<-chan QueryEvent, error) {
	e.mu.Lock()
	e.lastQuery = req
	e.queryCalled = true
	ch := e.queryCh
	e.mu.Unlock()
	return ch, nil
}

func (e *mockEngine) Resume(_ context.Context, sessionID string) (int, error) {
	e.mu.Lock()
	e.resumedSessionIDs = append(e.resumedSessionIDs, sessionID)
	count := e.resumeCount
	err := e.resumeErr
	e.mu.Unlock()
	return count, err
}

func (e *mockEngine) Compact(_ context.Context, sessionID string) (CompactResult, error) {
	e.mu.Lock()
	e.compactedSessionIDs = append(e.compactedSessionIDs, sessionID)
	result := e.compactResult
	err := e.compactErr
	e.mu.Unlock()
	return result, err
}

func (e *mockEngine) Interrupt(sessionID string) {
	e.mu.Lock()
	e.interrupted = true
	e.interruptedID = sessionID
	e.mu.Unlock()
}

func (e *mockEngine) SetPermission(h PermissionHandler) {
	e.mu.Lock()
	e.lastPermission = h
	e.mu.Unlock()
}

func (e *mockEngine) SetModel(sessionID, model string) error {
	e.mu.Lock()
	e.setModelCalls = append(e.setModelCalls, setModelCall{sessionID, model})
	err := e.setModelErr
	e.mu.Unlock()
	return err
}

func (e *mockEngine) SetThinkingConfig(sessionID string, enabled bool, budgetTokens int) error {
	e.mu.Lock()
	e.setThinkingCalls = append(e.setThinkingCalls, setThinkingCall{sessionID, enabled, budgetTokens})
	err := e.setThinkingErr
	e.mu.Unlock()
	return err
}

func (e *mockEngine) ContextUsage(_ string) (*ContextUsageInfo, error) {
	return &ContextUsageInfo{TotalTokens: 200000, UsedTokens: 50000, RemainingTokens: 150000}, nil
}

func (e *mockEngine) Tools() []string { return e.tools }
func (e *mockEngine) ModelID() string { return e.modelID }
func (e *mockEngine) SetSystemPrompt(systemPrompt string) {
	e.mu.Lock()
	e.systemPrompt = systemPrompt
	e.mu.Unlock()
}

// ─── test helpers ─────────────────────────────────────────────────────────────

// sendLine writes a JSON line to the writer.
func sendLine(t *testing.T, w io.Writer, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("sendLine: marshal: %v", err)
	}
	if _, err := fmt.Fprintf(w, "%s\n", data); err != nil {
		t.Fatalf("sendLine: write: %v", err)
	}
}

func textUserMessage(text string) json.RawMessage {
	payload, err := json.Marshal(struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: text})
	if err != nil {
		panic(err)
	}
	return payload
}

// startScanner runs a bufio.Scanner in a goroutine, sending each parsed JSON
// line to the returned buffered channel.
func startScanner(r io.Reader) chan map[string]any {
	ch := make(chan map[string]any, 64)
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			var m map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &m); err == nil {
				ch <- m
			}
		}
		close(ch)
	}()
	return ch
}

// makeQueryCh creates a buffered channel pre-loaded with events (then closed).
func makeQueryCh(events ...QueryEvent) chan QueryEvent {
	ch := make(chan QueryEvent, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch
}

// receiveWithTimeout waits for a value on ch or fails after timeout.
func receiveWithTimeout(t *testing.T, ch chan map[string]any, d time.Duration) map[string]any {
	t.Helper()
	select {
	case m, ok := <-ch:
		if !ok {
			t.Fatal("receiveWithTimeout: channel closed unexpectedly")
		}
		return m
	case <-time.After(d):
		t.Fatalf("receiveWithTimeout: timed out after %s", d)
		return nil
	}
}

const testTimeout = 3 * time.Second

// drainInit reads and discards the system/init "ready" message emitted at Serve startup.
func drainInit(t *testing.T, ch chan map[string]any) {
	t.Helper()
	m := receiveWithTimeout(t, ch, testTimeout)
	if m["type"] != "system" || m["subtype"] != "init" {
		t.Fatalf("drainInit: expected system/init, got type=%v subtype=%v", m["type"], m["subtype"])
	}
}

// ─── Serve tests ──────────────────────────────────────────────────────────────

// TestServe_ReadUserMessage verifies that a user message sent over stdin causes
// Runtime.Query to be called with the correct message text.
func TestServe_ReadUserMessage(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")
	eng.queryCh = makeQueryCh(QueryEvent{
		SessionID: "s1",
		Final:     true,
	})

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	sendLine(t, inW, SDKUserMessage{
		Type:      "user",
		SessionID: "s1",
		UUID:      "query-read-user",
		Message:   textUserMessage("hello world"),
	})

	// Drain the result message so the pipe write unblocks.
	receiveWithTimeout(t, scanCh, testTimeout)

	inW.Close()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for Serve to exit")
	}

	eng.mu.Lock()
	gotMsg := eng.lastQuery.Message
	wasCalled := eng.queryCalled
	eng.mu.Unlock()

	if !wasCalled {
		t.Fatal("expected Runtime.Query to be called")
	}
	if gotMsg != "hello world" {
		t.Errorf("Runtime.Query message: got %q, want %q", gotMsg, "hello world")
	}
}

// TestServe_StreamEvents verifies that text events from the runtime are emitted
// as streamlined_text NDJSON lines on stdout, followed by a result message.
func TestServe_StreamEvents(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")
	eng.queryCh = makeQueryCh(
		QueryEvent{
			SessionID: "s1",
			Event:     Event{Type: EventText, Text: "hello"},
		},
		QueryEvent{
			SessionID: "s1",
			Event:     Event{Type: EventText, Text: " world"},
		},
		QueryEvent{
			SessionID: "s1",
			Final:     true,
		},
	)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	sendLine(t, inW, SDKUserMessage{
		Type: "user", UUID: "query-stream-events", Message: textUserMessage("hi"),
	})

	// Expect 2 streamlined_text lines + 1 result line.
	var lines []map[string]any
	timeout := time.After(testTimeout)
	for len(lines) < 3 {
		select {
		case m, ok := <-scanCh:
			if !ok {
				t.Fatalf("scanner closed early after %d lines", len(lines))
			}
			lines = append(lines, m)
		case <-timeout:
			t.Fatalf("timed out: received %d lines, expected 3", len(lines))
		}
	}

	if got := lines[0]["type"]; got != "streamlined_text" {
		t.Errorf("line[0] type: got %q, want %q", got, "streamlined_text")
	}
	if got := lines[0]["text"]; got != "hello" {
		t.Errorf("line[0] text: got %q, want %q", got, "hello")
	}
	if got := lines[1]["type"]; got != "streamlined_text" {
		t.Errorf("line[1] type: got %q, want %q", got, "streamlined_text")
	}
	if got := lines[1]["text"]; got != " world" {
		t.Errorf("line[1] text: got %q, want %q", got, " world")
	}
	if got := lines[2]["type"]; got != "result" {
		t.Errorf("line[2] type: got %q, want %q", got, "result")
	}

	inW.Close()
}

func TestRunUserQueryForwardsProviderRequestLifecycle(t *testing.T) {
	const secret = "upstream token=sk-private-transport"
	runtime := newMockEngine(nil, "model-request-status")
	eventTypes := []EventType{
		EventRequestStart,
		EventRequestRetry,
		EventRequestFirstToken,
		EventRequestEnd,
		EventRequestFailed,
	}
	events := make([]QueryEvent, 0, len(eventTypes)+1)
	for _, eventType := range eventTypes {
		errorCode := "provider_request_retry"
		if eventType == EventRequestFailed {
			errorCode = "provider_request_failed"
		}
		events = append(events, QueryEvent{
			SessionID: "session-request-status",
			Event: Event{Type: eventType, RequestStatus: &RequestStatusEvent{
				RequestID: "request-transport", Phase: "untrusted_phase", Status: "untrusted_status",
				Attempt: 2, MaxAttempts: 3, RetryDelayMilliseconds: 500,
				ErrorCode: errorCode, ErrorMessage: secret,
			}},
		})
	}
	events = append(events, QueryEvent{SessionID: "session-request-status", Final: true})
	runtime.queryCh = makeQueryCh(events...)
	var output bytes.Buffer
	server := NewSDKServer(runtime, bytes.NewReader(nil), &output, InitialPermissionBridge)
	line, err := json.Marshal(SDKUserMessage{
		Type: "user", SessionID: "session-request-status", UUID: "query-request-status",
		Message: textUserMessage("hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	query, err := parseSDKUserQuery(line, i18n.LangZH)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.runUserQuery(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	wantStatuses := []string{"started", "retrying", "streaming", "completed", "failed"}
	decoder := json.NewDecoder(&output)
	for index, eventType := range eventTypes {
		var statusMessage struct {
			Type  string `json:"type"`
			Event struct {
				Type          EventType           `json:"type"`
				RequestStatus *RequestStatusEvent `json:"request_status"`
			} `json:"event"`
		}
		if err := decoder.Decode(&statusMessage); err != nil {
			t.Fatal(err)
		}
		status := statusMessage.Event.RequestStatus
		if statusMessage.Type != "stream_event" || statusMessage.Event.Type != eventType || status == nil ||
			status.RequestID != "request-transport" || status.Phase != string(eventType) || status.Status != wantStatuses[index] ||
			status.Attempt != 2 || status.MaxAttempts != 3 {
			t.Fatalf("request lifecycle message[%d] = %+v", index, statusMessage)
		}
		switch eventType {
		case EventRequestRetry:
			if status.ErrorMessage != i18n.Format(i18n.LangZH, i18n.KeyRuntimeTransientAPIError, 2, 3) {
				t.Fatalf("retry error message = %q", status.ErrorMessage)
			}
		case EventRequestFailed:
			if status.ErrorMessage != i18n.Text(i18n.LangZH, i18n.KeyRuntimeErrorPublicSummary) {
				t.Fatalf("failed error message = %q", status.ErrorMessage)
			}
		default:
			if status.ErrorCode != "" || status.ErrorMessage != "" {
				t.Fatalf("non-error request status retained error authority: %+v", status)
			}
		}
	}
	if strings.Contains(output.String(), secret) || strings.Contains(output.String(), "sk-private-transport") {
		t.Fatalf("request lifecycle leaked raw provider error: %s", output.String())
	}
}

// TestServe_Initialize sends an initialize control_request and verifies the
// response contains the tools list and model identifier.
func TestServe_Initialize(t *testing.T) {
	eng := newMockEngine([]string{"bash", "read_file"}, "claude-opus-4")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	reqPayload, _ := json.Marshal(InitializeRequest{Subtype: "initialize"})
	sendLine(t, inW, SDKControlRequest{
		Type:      "control_request",
		RequestID: "req-init-1",
		Request:   json.RawMessage(reqPayload),
	})

	m := receiveWithTimeout(t, scanCh, testTimeout)

	if m["type"] != "control_response" {
		t.Fatalf("type: got %q, want %q", m["type"], "control_response")
	}

	// Re-encode the "response" value to JSON so we can unmarshal into ControlSuccess.
	respRaw, err := json.Marshal(m["response"])
	if err != nil {
		t.Fatalf("re-marshal response: %v", err)
	}
	var cs ControlSuccess
	if err := json.Unmarshal(respRaw, &cs); err != nil {
		t.Fatalf("unmarshal ControlSuccess: %v", err)
	}
	if cs.Subtype != "success" {
		t.Errorf("subtype: got %q, want %q", cs.Subtype, "success")
	}
	if cs.RequestID != "req-init-1" {
		t.Errorf("request_id: got %q, want %q", cs.RequestID, "req-init-1")
	}

	var initResp InitializeResponse
	if err := json.Unmarshal(cs.Response, &initResp); err != nil {
		t.Fatalf("unmarshal InitializeResponse: %v", err)
	}
	if initResp.Model != "claude-opus-4" {
		t.Errorf("model: got %q, want %q", initResp.Model, "claude-opus-4")
	}
	if len(initResp.Tools) != 2 {
		t.Fatalf("tools len: got %d, want 2", len(initResp.Tools))
	}
	if initResp.Tools[0] != "bash" || initResp.Tools[1] != "read_file" {
		t.Errorf("tools: got %v", initResp.Tools)
	}
	if initResp.OutputStyle != "streamlined" || len(initResp.AvailableStyles) != 1 || initResp.AvailableStyles[0] != "streamlined" {
		t.Errorf("output styles: active=%q available=%v", initResp.OutputStyle, initResp.AvailableStyles)
	}

	inW.Close()
}

// TestServe_Interrupt sends an interrupt control_request and verifies that
// Runtime.Interrupt was called with the correct session ID.
func TestServe_Interrupt(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	reqPayload, _ := json.Marshal(InterruptRequest{Subtype: "interrupt", SessionID: "sess-abc"})
	sendLine(t, inW, SDKControlRequest{
		Type:      "control_request",
		RequestID: "req-interrupt-1",
		Request:   json.RawMessage(reqPayload),
	})

	m := receiveWithTimeout(t, scanCh, testTimeout)

	if m["type"] != "control_response" {
		t.Errorf("type: got %q, want %q", m["type"], "control_response")
	}

	inW.Close()

	eng.mu.Lock()
	interrupted := eng.interrupted
	interruptedID := eng.interruptedID
	eng.mu.Unlock()

	if !interrupted {
		t.Error("expected Runtime.Interrupt to be called")
	}
	if interruptedID != "sess-abc" {
		t.Errorf("interrupt session ID: got %q, want %q", interruptedID, "sess-abc")
	}
}

// TestServe_SetModel sends a set_model control_request and verifies that
// Runtime.SetModel was called with the correct session and model.
func TestServe_SetModel(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	reqPayload, _ := json.Marshal(SetModelRequest{
		Subtype:   "set_model",
		Model:     "claude-3-5-sonnet",
		SessionID: "sess-x",
	})
	sendLine(t, inW, SDKControlRequest{
		Type:      "control_request",
		RequestID: "req-setmodel-1",
		Request:   json.RawMessage(reqPayload),
	})

	m := receiveWithTimeout(t, scanCh, testTimeout)

	if m["type"] != "control_response" {
		t.Fatalf("type: got %q, want %q", m["type"], "control_response")
	}
	respRaw, _ := json.Marshal(m["response"])
	var cs ControlSuccess
	if err := json.Unmarshal(respRaw, &cs); err != nil {
		t.Fatalf("unmarshal ControlSuccess: %v", err)
	}
	if cs.Subtype != "success" {
		t.Errorf("subtype: got %q, want %q", cs.Subtype, "success")
	}

	inW.Close()

	eng.mu.Lock()
	calls := eng.setModelCalls
	eng.mu.Unlock()

	if len(calls) == 0 {
		t.Fatal("expected Runtime.SetModel to be called")
	}
	if calls[0].model != "claude-3-5-sonnet" {
		t.Errorf("SetModel model: got %q, want %q", calls[0].model, "claude-3-5-sonnet")
	}
	if calls[0].sessionID != "sess-x" {
		t.Errorf("SetModel sessionID: got %q, want %q", calls[0].sessionID, "sess-x")
	}
}

// TestServe_KeepAlive sends a keep_alive heartbeat and verifies that Serve
// does not crash and produces no output.
func TestServe_KeepAlive(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()

	// Use io.Discard for stdout — keep_alive must produce no output.
	srv := NewSDKServer(eng, inR, io.Discard, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(ctx) }()

	sendLine(t, inW, SDKKeepAlive{Type: "keep_alive"})
	inW.Close()

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for Serve to exit after keep_alive + EOF")
	}
}

// TestServe_UnknownControlSubtype sends a control_request with an unrecognised
// subtype and verifies that the server returns a control_response error.
func TestServe_UnknownControlSubtype(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	reqPayload, _ := json.Marshal(map[string]string{"subtype": "totally_unknown_op"})
	sendLine(t, inW, SDKControlRequest{
		Type:      "control_request",
		RequestID: "req-unk-1",
		Request:   json.RawMessage(reqPayload),
	})

	m := receiveWithTimeout(t, scanCh, testTimeout)

	if m["type"] != "control_response" {
		t.Fatalf("type: got %q, want %q", m["type"], "control_response")
	}
	respRaw, _ := json.Marshal(m["response"])
	var ce ControlError
	if err := json.Unmarshal(respRaw, &ce); err != nil {
		t.Fatalf("unmarshal ControlError: %v", err)
	}
	if ce.Subtype != "error" {
		t.Errorf("subtype: got %q, want %q", ce.Subtype, "error")
	}
	if ce.RequestID != "req-unk-1" {
		t.Errorf("request_id: got %q, want %q", ce.RequestID, "req-unk-1")
	}
	if ce.Error == "" {
		t.Error("expected non-empty error message in ControlError")
	}

	inW.Close()
}

// TestServe_EOF closes stdin immediately and verifies that Serve returns nil.
func TestServe_EOF(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	srv := NewSDKServer(eng, inR, io.Discard, InitialPermissionBridge)

	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(context.Background()) }()

	inW.Close() // immediate EOF

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("expected nil on clean EOF, got: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for Serve to return on EOF")
	}
}

// TestServe_InvalidJSON sends a malformed JSON line and verifies that Serve
// emits a system error message on stdout rather than crashing.
func TestServe_InvalidJSON(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	// Write a line that is not valid JSON.
	fmt.Fprintf(inW, "{this is: not, valid json}\n")

	m := receiveWithTimeout(t, scanCh, testTimeout)

	if m["type"] != "system" {
		t.Errorf("type: got %v, want %q", m["type"], "system")
	}
	if m["subtype"] != "error" {
		t.Errorf("subtype: got %v, want %q", m["subtype"], "error")
	}
	if msg, _ := m["message"].(string); msg == "" {
		t.Error("expected non-empty error message in system message")
	}

	inW.Close()
}

// ─── eventAdapter tests ───────────────────────────────────────────────────────

// TestEventAdapter_TextEvent verifies that a text loop event is converted to
// a StreamlinedTextMsg SDK message.
func TestEventAdapter_TextEvent(t *testing.T) {
	a := newEventAdapter("session-text-1", i18n.LangEN)

	results := a.process(Event{
		Type: EventText,
		Text: "streaming content",
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	msg, ok := results[0].(StreamlinedTextMsg)
	if !ok {
		t.Fatalf("expected StreamlinedTextMsg, got %T", results[0])
	}
	if msg.Type != "streamlined_text" {
		t.Errorf("Type: got %q, want %q", msg.Type, "streamlined_text")
	}
	if msg.Text != "streaming content" {
		t.Errorf("Text: got %q, want %q", msg.Text, "streaming content")
	}
	if msg.SessionID != "session-text-1" {
		t.Errorf("SessionID: got %q, want %q", msg.SessionID, "session-text-1")
	}

	// Text should accumulate in the adapter's buffer for the final result.
	result := a.resultMessage(i18n.LangEN, "session-text-1", "uuid-1", nil)
	if result.Result != "streaming content" {
		t.Errorf("result text buffer: got %q, want %q", result.Result, "streaming content")
	}
}

// TestEventAdapter_ToolUseEvent verifies that a tool_use loop event is converted
// to a StreamlinedToolUseSummaryMsg SDK message.
func TestEventAdapter_ToolUseEvent(t *testing.T) {
	a := newEventAdapter("session-tool-1", i18n.LangEN)

	toolUse := &ToolUse{
		ID:    "tu-abc-123",
		Name:  "bash",
		Input: map[string]any{"command": "ls -la"},
	}
	results := a.process(Event{
		Type:    EventToolUse,
		ToolUse: toolUse,
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	msg, ok := results[0].(StreamlinedToolUseSummaryMsg)
	if !ok {
		t.Fatalf("expected StreamlinedToolUseSummaryMsg, got %T", results[0])
	}
	if msg.Type != "streamlined_tool_use_summary" {
		t.Errorf("Type: got %q, want %q", msg.Type, "streamlined_tool_use_summary")
	}
	if msg.ToolUseID != "tu-abc-123" {
		t.Errorf("ToolUseID: got %q, want %q", msg.ToolUseID, "tu-abc-123")
	}
	if msg.ToolName != "bash" {
		t.Errorf("ToolName: got %q, want %q", msg.ToolName, "bash")
	}
	if msg.Status != "started" {
		t.Errorf("Status: got %q, want %q", msg.Status, "started")
	}
	if msg.SessionID != "session-tool-1" {
		t.Errorf("SessionID: got %q, want %q", msg.SessionID, "session-tool-1")
	}
}

// TestEventAdapter_NilToolUse verifies that a tool_use event with nil ToolUse
// produces no output.
func TestEventAdapter_NilToolUse(t *testing.T) {
	a := newEventAdapter("session-nil", i18n.LangEN)
	results := a.process(Event{Type: EventToolUse, ToolUse: nil})
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil ToolUse, got %d", len(results))
	}
}

// ─── permissionBridge tests ───────────────────────────────────────────────────

// TestPermissionBridge_RegisterDeliver registers a request, delivers a result,
// and verifies the result arrives on the returned channel.
func TestPermissionBridge_RegisterDeliver(t *testing.T) {
	b := newPermissionBridge()

	ch, ok := b.register("perm-req-42")
	if !ok {
		t.Fatal("register unexpectedly failed")
	}

	want := permissionResult{behavior: "allow"}
	delivered := b.deliver("perm-req-42", want)
	if !delivered {
		t.Fatal("expected deliver to return true for a registered ID")
	}

	select {
	case got := <-ch:
		if got.behavior != "allow" {
			t.Errorf("behavior: got %q, want %q", got.behavior, "allow")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel result after deliver")
	}

	// After delivery the entry must be cleaned up — a second deliver should fail.
	if b.deliver("perm-req-42", want) {
		t.Error("expected second deliver to return false (already cleaned up)")
	}
}

// TestPermissionBridge_DeliverUnknown verifies that delivering to an
// unregistered ID returns false without panicking.
func TestPermissionBridge_DeliverUnknown(t *testing.T) {
	b := newPermissionBridge()

	result := b.deliver("does-not-exist", permissionResult{behavior: "deny"})
	if result {
		t.Error("expected deliver to return false for an unknown request ID")
	}
}

// TestPermissionBridge_MultipleRequests verifies that multiple in-flight
// requests are tracked independently.
func TestPermissionBridge_MultipleRequests(t *testing.T) {
	b := newPermissionBridge()

	ch1, ok := b.register("req-1")
	if !ok {
		t.Fatal("register req-1 unexpectedly failed")
	}
	ch2, ok := b.register("req-2")
	if !ok {
		t.Fatal("register req-2 unexpectedly failed")
	}

	b.deliver("req-2", permissionResult{behavior: "deny"})
	b.deliver("req-1", permissionResult{behavior: "allow"})

	select {
	case got := <-ch1:
		if got.behavior != "allow" {
			t.Errorf("ch1 behavior: got %q, want allow", got.behavior)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading ch1")
	}

	select {
	case got := <-ch2:
		if got.behavior != "deny" {
			t.Errorf("ch2 behavior: got %q, want deny", got.behavior)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading ch2")
	}
}

// ─── New subtype tests ────────────────────────────────────────────────────────

// TestServe_SystemInit verifies that Serve emits a system/init "ready" message
// immediately on startup before any client messages are sent.
func TestServe_SystemInit(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	m := receiveWithTimeout(t, scanCh, testTimeout)

	if m["type"] != "system" {
		t.Errorf("type: got %v, want %q", m["type"], "system")
	}
	if m["subtype"] != "init" {
		t.Errorf("subtype: got %v, want %q", m["subtype"], "init")
	}
	wantMessage := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeySDKReady)
	if m["message"] != wantMessage {
		t.Errorf("message: got %v, want %q", m["message"], wantMessage)
	}
	if len(m) != 3 {
		t.Fatalf("system/init exposed non-canonical fields: %#v", m)
	}

	inW.Close()
}

func TestCanonicalSDKInputNDJSONAndCompactResponse(t *testing.T) {
	tests := []struct {
		name string
		msg  any
		want string
	}{
		{
			name: "initialize",
			msg: SDKControlRequest{
				Type: "control_request", RequestID: "init-1",
				Request: json.RawMessage(`{"subtype":"initialize"}`),
			},
			want: `{"type":"control_request","request_id":"init-1","request":{"subtype":"initialize"}}`,
		},
		{
			name: "user",
			msg: SDKUserMessage{
				Type: "user", Message: textUserMessage("hello"), SessionID: "session-1", UUID: "query-1",
			},
			want: `{"type":"user","message":{"role":"user","content":"hello"},"session_id":"session-1","uuid":"query-1"}`,
		},
		{
			name: "compact",
			msg: SDKControlRequest{
				Type: "control_request", RequestID: "compact-1",
				Request: json.RawMessage(`{"subtype":"compact","session_id":"session-1"}`),
			},
			want: `{"type":"control_request","request_id":"compact-1","request":{"subtype":"compact","session_id":"session-1"}}`,
		},
	}
	for _, tt := range tests {
		encoded, err := json.Marshal(tt.msg)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got := string(encoded); got != tt.want {
			t.Errorf("%s NDJSON payload = %s, want %s", tt.name, got, tt.want)
		}
	}

	runtime := newMockEngine(nil, "model")
	runtime.compactResult = CompactResult{
		Compacted: true, BeforeMessageCount: 24, AfterMessageCount: 5, ContextGeneration: 7,
	}
	var output bytes.Buffer
	server := NewSDKServer(runtime, strings.NewReader(""), &output, InitialPermissionBridge)
	compactLine := []byte(`{"type":"control_request","request_id":"compact-1","request":{"subtype":"compact","session_id":"session-1"}}`)
	if err := server.handleControlRequest(context.Background(), compactLine, i18n.LangEN); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"compact-1\",\"response\":{\"session_id\":\"session-1\",\"compacted\":true,\"before_message_count\":24,\"after_message_count\":5,\"context_generation\":7}}}\n"; got != want {
		t.Fatalf("compact response NDJSON = %q, want %q", got, want)
	}
}

func TestSDKLocalizedMessagesPreserveProtocolFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.SaveLanguage(i18n.LangEN) })

	eng := newMockEngine(nil, "model-id")
	var out bytes.Buffer
	srv := NewSDKServer(eng, bytes.NewReader(nil), &out, InitialPermissionBridge)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var initMessage SDKSystemMessage
	if err := json.NewDecoder(&out).Decode(&initMessage); err != nil {
		t.Fatalf("decode init message: %v", err)
	}
	if initMessage.Type != "system" || initMessage.Subtype != "init" {
		t.Fatalf("init protocol fields changed: type=%q subtype=%q", initMessage.Type, initMessage.Subtype)
	}
	if initMessage.Message != i18n.Text(i18n.LangZH, i18n.KeySDKReady) {
		t.Fatalf("localized init message = %q", initMessage.Message)
	}

	out.Reset()
	rawRequest, err := json.Marshal(map[string]string{"subtype": "vendor_extension"})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.handleControlRequest(context.Background(), []byte(fmt.Sprintf(
		`{"type":"control_request","request_id":"req-raw","request":%s}`,
		rawRequest,
	)), i18n.LangZH); err != nil {
		t.Fatalf("handleControlRequest: %v", err)
	}
	var response SDKControlResponse
	if err := json.NewDecoder(&out).Decode(&response); err != nil {
		t.Fatalf("decode control response: %v", err)
	}
	if response.Type != "control_response" {
		t.Fatalf("control response type changed: %q", response.Type)
	}
	var controlError ControlError
	if err := json.Unmarshal(response.Response, &controlError); err != nil {
		t.Fatalf("decode control error: %v", err)
	}
	if controlError.Subtype != "error" || controlError.RequestID != "req-raw" {
		t.Fatalf("control error protocol fields changed: subtype=%q request_id=%q", controlError.Subtype, controlError.RequestID)
	}
	if !strings.Contains(controlError.Error, "vendor_extension") ||
		controlError.Error == `unsupported control subtype "vendor_extension"` {
		t.Fatalf("control error was not localized or lost its raw subtype: %q", controlError.Error)
	}
}

// TestServe_SetPermissionMode verifies that set_permission_mode changes the
// permission handler on the engine appropriately.
func TestServe_SetPermissionMode(t *testing.T) {
	tests := []struct {
		name         string
		initialMode  InitialPermissionMode
		mode         string
		wantAllowAll bool
	}{
		{name: "full-auto uses AllowAllHandler", initialMode: InitialPermissionBridge, mode: "full-auto", wantAllowAll: true},
		{name: "default uses SDK bridge handler", initialMode: InitialPermissionBridge, mode: "default"},
		{name: "auto-edit uses SDK bridge handler", initialMode: InitialPermissionBridge, mode: "auto-edit"},
		{name: "full-auto initial mode can switch to bridge", initialMode: InitialPermissionFullAuto, mode: "default"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eng := newMockEngine(nil, "claude-3")

			inR, inW := io.Pipe()
			outR, outW := io.Pipe()
			defer outW.Close()

			srv := NewSDKServer(eng, inR, outW, tc.initialMode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() { srv.Serve(ctx) }()

			scanCh := startScanner(outR)
			drainInit(t, scanCh)

			reqPayload, _ := json.Marshal(SetPermissionModeRequest{
				Subtype: "set_permission_mode",
				Mode:    tc.mode,
			})
			sendLine(t, inW, SDKControlRequest{
				Type:      "control_request",
				RequestID: "req-pm-1",
				Request:   json.RawMessage(reqPayload),
			})

			m := receiveWithTimeout(t, scanCh, testTimeout)
			if m["type"] != "control_response" {
				t.Fatalf("type: got %v, want control_response", m["type"])
			}
			respRaw, _ := json.Marshal(m["response"])
			var cs ControlSuccess
			if err := json.Unmarshal(respRaw, &cs); err != nil {
				t.Fatalf("unmarshal ControlSuccess: %v", err)
			}
			if cs.Subtype != "success" {
				t.Errorf("subtype: got %q, want success", cs.Subtype)
			}

			eng.mu.Lock()
			perm := eng.lastPermission
			eng.mu.Unlock()

			_, isAllowAll := perm.(allowAllPermissionHandler)
			if tc.wantAllowAll && !isAllowAll {
				t.Errorf("expected AllowAllHandler for mode %q, got %T", tc.mode, perm)
			}
			if !tc.wantAllowAll && isAllowAll {
				t.Errorf("expected SDK bridge handler for mode %q, got AllowAllHandler", tc.mode)
			}

			inW.Close()
		})
	}
}

// TestServe_SetMaxThinkingTokens verifies that set_max_thinking_tokens calls
// Runtime.SetThinkingConfig with the correct parameters.
func TestServe_SetMaxThinkingTokens(t *testing.T) {
	t.Run("enable thinking", func(t *testing.T) {
		eng := newMockEngine(nil, "claude-3")

		inR, inW := io.Pipe()
		outR, outW := io.Pipe()
		defer outW.Close()

		srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() { srv.Serve(ctx) }()

		scanCh := startScanner(outR)
		drainInit(t, scanCh)

		budget := 10000
		reqPayload, _ := json.Marshal(SetMaxThinkingTokensRequest{
			Subtype:           "set_max_thinking_tokens",
			MaxThinkingTokens: &budget,
			SessionID:         "sess-think",
		})
		sendLine(t, inW, SDKControlRequest{
			Type:      "control_request",
			RequestID: "req-think-1",
			Request:   json.RawMessage(reqPayload),
		})

		m := receiveWithTimeout(t, scanCh, testTimeout)
		if m["type"] != "control_response" {
			t.Fatalf("type: got %v, want control_response", m["type"])
		}
		respRaw, _ := json.Marshal(m["response"])
		var cs ControlSuccess
		if err := json.Unmarshal(respRaw, &cs); err != nil {
			t.Fatalf("unmarshal ControlSuccess: %v", err)
		}
		if cs.Subtype != "success" {
			t.Errorf("subtype: got %q, want success", cs.Subtype)
		}

		eng.mu.Lock()
		calls := eng.setThinkingCalls
		eng.mu.Unlock()

		if len(calls) == 0 {
			t.Fatal("expected SetThinkingConfig to be called")
		}
		if calls[0].sessionID != "sess-think" {
			t.Errorf("sessionID: got %q, want sess-think", calls[0].sessionID)
		}
		if !calls[0].enabled {
			t.Error("expected enabled=true")
		}
		if calls[0].budgetTokens != 10000 {
			t.Errorf("budgetTokens: got %d, want 10000", calls[0].budgetTokens)
		}

		inW.Close()
	})

	t.Run("disable thinking", func(t *testing.T) {
		eng := newMockEngine(nil, "claude-3")

		inR, inW := io.Pipe()
		outR, outW := io.Pipe()
		defer outW.Close()

		srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() { srv.Serve(ctx) }()

		scanCh := startScanner(outR)
		drainInit(t, scanCh)

		reqPayload, _ := json.Marshal(SetMaxThinkingTokensRequest{
			Subtype:           "set_max_thinking_tokens",
			MaxThinkingTokens: nil,
			SessionID:         "sess-think",
		})
		sendLine(t, inW, SDKControlRequest{
			Type:      "control_request",
			RequestID: "req-think-2",
			Request:   json.RawMessage(reqPayload),
		})

		m := receiveWithTimeout(t, scanCh, testTimeout)
		if m["type"] != "control_response" {
			t.Fatalf("type: got %v, want control_response", m["type"])
		}

		eng.mu.Lock()
		calls := eng.setThinkingCalls
		eng.mu.Unlock()

		if len(calls) == 0 {
			t.Fatal("expected SetThinkingConfig to be called")
		}
		if calls[0].enabled {
			t.Error("expected enabled=false when MaxThinkingTokens is nil")
		}
		if calls[0].budgetTokens != 0 {
			t.Errorf("budgetTokens: got %d, want 0", calls[0].budgetTokens)
		}

		inW.Close()
	})
}

// TestServe_Resume verifies that a resume control_request loads the session
// and returns the message count in the success response.
func TestServe_Resume(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")
	eng.resumeCount = 42

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	reqPayload, _ := json.Marshal(ResumeRequest{
		Subtype:   "resume",
		SessionID: "sess-resume-1",
	})
	sendLine(t, inW, SDKControlRequest{
		Type:      "control_request",
		RequestID: "req-resume-1",
		Request:   json.RawMessage(reqPayload),
	})

	m := receiveWithTimeout(t, scanCh, testTimeout)
	if m["type"] != "control_response" {
		t.Fatalf("type: got %v, want control_response", m["type"])
	}

	respRaw, _ := json.Marshal(m["response"])
	var cs ControlSuccess
	if err := json.Unmarshal(respRaw, &cs); err != nil {
		t.Fatalf("unmarshal ControlSuccess: %v", err)
	}
	if cs.Subtype != "success" {
		t.Errorf("subtype: got %q, want success", cs.Subtype)
	}
	if cs.RequestID != "req-resume-1" {
		t.Errorf("request_id: got %q, want req-resume-1", cs.RequestID)
	}

	var resumeResp ResumeResponse
	if err := json.Unmarshal(cs.Response, &resumeResp); err != nil {
		t.Fatalf("unmarshal ResumeResponse: %v", err)
	}
	if resumeResp.SessionID != "sess-resume-1" {
		t.Errorf("session_id: got %q, want sess-resume-1", resumeResp.SessionID)
	}
	if resumeResp.MessageCount != 42 {
		t.Errorf("message_count: got %d, want 42", resumeResp.MessageCount)
	}

	eng.mu.Lock()
	ids := eng.resumedSessionIDs
	eng.mu.Unlock()

	if len(ids) == 0 || ids[0] != "sess-resume-1" {
		t.Errorf("resumedSessionIDs: got %v, want [sess-resume-1]", ids)
	}

	inW.Close()
}

// TestServe_Compact verifies that a compact control_request calls Runtime.Compact
// with the correct session ID.
func TestServe_Compact(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")
	eng.compactResult = CompactResult{
		Compacted: true, BeforeMessageCount: 31, AfterMessageCount: 6, ContextGeneration: 4,
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	reqPayload, _ := json.Marshal(CompactRequest{
		Subtype:   "compact",
		SessionID: "sess-compact-1",
	})
	sendLine(t, inW, SDKControlRequest{
		Type:      "control_request",
		RequestID: "req-compact-1",
		Request:   json.RawMessage(reqPayload),
	})

	m := receiveWithTimeout(t, scanCh, testTimeout)
	if m["type"] != "control_response" {
		t.Fatalf("type: got %v, want control_response", m["type"])
	}

	respRaw, _ := json.Marshal(m["response"])
	var cs ControlSuccess
	if err := json.Unmarshal(respRaw, &cs); err != nil {
		t.Fatalf("unmarshal ControlSuccess: %v", err)
	}
	if cs.Subtype != "success" {
		t.Errorf("subtype: got %q, want success", cs.Subtype)
	}
	var compactResponse CompactResponse
	if err := json.Unmarshal(cs.Response, &compactResponse); err != nil {
		t.Fatalf("unmarshal CompactResponse: %v", err)
	}
	if compactResponse.SessionID != "sess-compact-1" || !compactResponse.Compacted ||
		compactResponse.BeforeMessageCount != 31 || compactResponse.AfterMessageCount != 6 ||
		compactResponse.ContextGeneration != 4 {
		t.Fatalf("compact response = %+v", compactResponse)
	}

	eng.mu.Lock()
	ids := eng.compactedSessionIDs
	eng.mu.Unlock()

	if len(ids) == 0 || ids[0] != "sess-compact-1" {
		t.Errorf("compactedSessionIDs: got %v, want [sess-compact-1]", ids)
	}

	inW.Close()
}

func TestServe_CompactSemanticNoopReturnsTypedResult(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")
	eng.compactResult = CompactResult{
		Compacted: false, BeforeMessageCount: 3, AfterMessageCount: 3, ContextGeneration: 2,
	}
	var output bytes.Buffer
	srv := NewSDKServer(eng, bytes.NewReader(nil), &output, InitialPermissionBridge)
	payload, err := json.Marshal(CompactRequest{Subtype: "compact", SessionID: "noop-session"})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.handleCompact(context.Background(), SDKControlRequest{
		Type: "control_request", RequestID: "compact-noop", Request: payload,
	}, i18n.LangEN); err != nil {
		t.Fatal(err)
	}
	var envelope SDKControlResponse
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var success ControlSuccess
	if err := json.Unmarshal(envelope.Response, &success); err != nil {
		t.Fatal(err)
	}
	var response CompactResponse
	if err := json.Unmarshal(success.Response, &response); err != nil {
		t.Fatal(err)
	}
	if response.SessionID != "noop-session" || response.Compacted || response.BeforeMessageCount != 3 ||
		response.AfterMessageCount != 3 || response.ContextGeneration != 2 {
		t.Fatalf("no-op compact response = %+v", response)
	}
}

// TestServe_GetContextUsage verifies that get_context_usage returns token
// statistics from the runtime.
func TestServe_GetContextUsage(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW, InitialPermissionBridge)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	reqPayload, _ := json.Marshal(GetContextUsageRequest{
		Subtype:   "get_context_usage",
		SessionID: "sess-ctx-1",
	})
	sendLine(t, inW, SDKControlRequest{
		Type:      "control_request",
		RequestID: "req-ctx-1",
		Request:   json.RawMessage(reqPayload),
	})

	m := receiveWithTimeout(t, scanCh, testTimeout)
	if m["type"] != "control_response" {
		t.Fatalf("type: got %v, want control_response", m["type"])
	}

	respRaw, _ := json.Marshal(m["response"])
	var cs ControlSuccess
	if err := json.Unmarshal(respRaw, &cs); err != nil {
		t.Fatalf("unmarshal ControlSuccess: %v", err)
	}
	if cs.Subtype != "success" {
		t.Errorf("subtype: got %q, want success", cs.Subtype)
	}

	var info ContextUsageInfo
	if err := json.Unmarshal(cs.Response, &info); err != nil {
		t.Fatalf("unmarshal ContextUsageInfo: %v", err)
	}
	if info.TotalTokens != 200000 {
		t.Errorf("TotalTokens: got %d, want 200000", info.TotalTokens)
	}
	if info.UsedTokens != 50000 {
		t.Errorf("UsedTokens: got %d, want 50000", info.UsedTokens)
	}
	if info.RemainingTokens != 150000 {
		t.Errorf("RemainingTokens: got %d, want 150000", info.RemainingTokens)
	}

	inW.Close()
}
