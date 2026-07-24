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

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// ─── mock provider ────────────────────────────────────────────────────────────

type mockProvider struct {
	name    string
	modelID string
}

func (p *mockProvider) Name() string    { return p.name }
func (p *mockProvider) ModelID() string { return p.modelID }
func (p *mockProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	ch := make(chan types.StreamEvent)
	close(ch)
	return ch, nil
}

// ─── mock session manager ─────────────────────────────────────────────────────

type mockSessionManager struct{}

func (m *mockSessionManager) Save(_ string, _ []types.Message) error { return nil }
func (m *mockSessionManager) Load(_ string) ([]types.Message, error) {
	return nil, engine.ErrSessionNotFound
}
func (m *mockSessionManager) List() ([]engine.SessionInfo, error) { return nil, nil }
func (m *mockSessionManager) Latest() (string, error)             { return "", engine.ErrSessionNotFound }
func (m *mockSessionManager) Delete(_ string) error               { return nil }

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
	prov                *mockProvider
	queryCh             chan engine.Event
	lastQuery           engine.QueryRequest
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
	compactErr          error
	lastPermission      engine.PermissionHandler
}

func newMockEngine(tools []string, modelID string) *mockEngine {
	return &mockEngine{
		tools: tools,
		prov:  &mockProvider{name: "mock", modelID: modelID},
	}
}

func (e *mockEngine) Query(_ context.Context, req engine.QueryRequest) (<-chan engine.Event, error) {
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

func (e *mockEngine) Compact(_ context.Context, sessionID string, _ ...string) error {
	e.mu.Lock()
	e.compactedSessionIDs = append(e.compactedSessionIDs, sessionID)
	err := e.compactErr
	e.mu.Unlock()
	return err
}

func (e *mockEngine) Interrupt(sessionID string) {
	e.mu.Lock()
	e.interrupted = true
	e.interruptedID = sessionID
	e.mu.Unlock()
}

func (e *mockEngine) SetPermission(h engine.PermissionHandler) {
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

func (e *mockEngine) SetReasoningEffort(sessionID, effort string) error {
	return nil
}

func (e *mockEngine) SetThinkingConfig(sessionID string, enabled bool, budgetTokens int) error {
	e.mu.Lock()
	e.setThinkingCalls = append(e.setThinkingCalls, setThinkingCall{sessionID, enabled, budgetTokens})
	err := e.setThinkingErr
	e.mu.Unlock()
	return err
}

func (e *mockEngine) ContextUsage(_ string) (*engine.ContextUsageInfo, error) {
	return &engine.ContextUsageInfo{TotalTokens: 200000, UsedTokens: 50000, RemainingTokens: 150000}, nil
}

func (e *mockEngine) Tools() []string                         { return e.tools }
func (e *mockEngine) ToolDefinitions() []types.ToolDefinition { return nil }
func (e *mockEngine) Provider() provider.Provider             { return e.prov }
func (e *mockEngine) SetProvider(_ provider.Provider)         {} // stub for Engine interface
func (e *mockEngine) ProviderRef() *provider.ProviderRef      { return provider.NewProviderRef(e.prov) }
func (e *mockEngine) Sessions() engine.SessionManager         { return &mockSessionManager{} }
func (e *mockEngine) Shutdown(_ context.Context) error        { return nil }

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

// readLine reads one JSON line from the scanner.
func readLine(t *testing.T, r *bufio.Scanner) map[string]any {
	t.Helper()
	if !r.Scan() {
		t.Fatal("readLine: expected output line but got EOF")
	}
	var m map[string]any
	if err := json.Unmarshal(r.Bytes(), &m); err != nil {
		t.Fatalf("readLine: unmarshal %q: %v", r.Text(), err)
	}
	return m
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
func makeQueryCh(events ...engine.Event) chan engine.Event {
	ch := make(chan engine.Event, len(events))
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
// engine.Query to be called with the correct message text.
func TestServe_ReadUserMessage(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")
	eng.queryCh = makeQueryCh(engine.Event{
		SessionID: "s1",
		Final:     true,
	})

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	sendLine(t, inW, SDKUserMessage{
		Type:      "user",
		SessionID: "s1",
		Message:   json.RawMessage(`"hello world"`),
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
		t.Fatal("expected engine.Query to be called")
	}
	if gotMsg != "hello world" {
		t.Errorf("engine.Query message: got %q, want %q", gotMsg, "hello world")
	}
}

// TestServe_StreamEvents verifies that text events from the engine are emitted
// as streamlined_text NDJSON lines on stdout, followed by a result message.
func TestServe_StreamEvents(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")
	eng.queryCh = makeQueryCh(
		engine.Event{
			SessionID: "s1",
			Inner:     loop.Event{Type: loop.EventText, Text: "hello"},
		},
		engine.Event{
			SessionID: "s1",
			Inner:     loop.Event{Type: loop.EventText, Text: " world"},
		},
		engine.Event{
			SessionID: "s1",
			Final:     true,
		},
	)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { srv.Serve(ctx) }()

	scanCh := startScanner(outR)
	drainInit(t, scanCh)

	sendLine(t, inW, SDKUserMessage{
		Type:    "user",
		Message: json.RawMessage(`"hi"`),
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

// TestServe_Initialize sends an initialize control_request and verifies the
// response contains the tools list and model identifier.
func TestServe_Initialize(t *testing.T) {
	eng := newMockEngine([]string{"bash", "read_file"}, "claude-opus-4")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW)
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

	inW.Close()
}

// TestServe_Interrupt sends an interrupt control_request and verifies that
// engine.Interrupt was called with the correct session ID.
func TestServe_Interrupt(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW)
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
		t.Error("expected engine.Interrupt to be called")
	}
	if interruptedID != "sess-abc" {
		t.Errorf("interrupt session ID: got %q, want %q", interruptedID, "sess-abc")
	}
}

// TestServe_SetModel sends a set_model control_request and verifies that
// engine.SetModel was called with the correct session and model.
func TestServe_SetModel(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW)
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
		t.Fatal("expected engine.SetModel to be called")
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
	srv := NewSDKServer(eng, inR, io.Discard)
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

	srv := NewSDKServer(eng, inR, outW)
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
	srv := NewSDKServer(eng, inR, io.Discard)

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

	srv := NewSDKServer(eng, inR, outW)
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
	a := newEventAdapter("session-text-1")

	results := a.process(loop.Event{
		Type: loop.EventText,
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
	a := newEventAdapter("session-tool-1")

	toolUse := &types.ToolUseBlock{
		Type:  types.ContentTypeToolUse,
		ID:    "tu-abc-123",
		Name:  "bash",
		Input: map[string]any{"command": "ls -la"},
	}
	results := a.process(loop.Event{
		Type:    loop.EventToolUse,
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
	a := newEventAdapter("session-nil")
	results := a.process(loop.Event{Type: loop.EventToolUse, ToolUse: nil})
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil ToolUse, got %d", len(results))
	}
}

// ─── permissionBridge tests ───────────────────────────────────────────────────

// TestPermissionBridge_RegisterDeliver registers a request, delivers a result,
// and verifies the result arrives on the returned channel.
func TestPermissionBridge_RegisterDeliver(t *testing.T) {
	b := newPermissionBridge()

	ch := b.register("perm-req-42")

	want := permissionResult{behavior: "allow", message: ""}
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

	ch1 := b.register("req-1")
	ch2 := b.register("req-2")

	b.deliver("req-2", permissionResult{behavior: "deny", message: "not allowed"})
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
		if got.message != "not allowed" {
			t.Errorf("ch2 message: got %q, want %q", got.message, "not allowed")
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

	srv := NewSDKServer(eng, inR, outW)
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

	inW.Close()
}

func TestSDKLocalizedMessagesPreserveProtocolFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.SaveLanguage(i18n.LangEN) })

	eng := newMockEngine(nil, "model-id")
	var out bytes.Buffer
	srv := NewSDKServer(eng, bytes.NewReader(nil), &out)
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
		mode         string
		wantAllowAll bool
	}{
		{"full-auto uses AllowAllHandler", "full-auto", true},
		{"default uses SDK bridge handler", "default", false},
		{"auto-edit uses SDK bridge handler", "auto-edit", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eng := newMockEngine(nil, "claude-3")

			inR, inW := io.Pipe()
			outR, outW := io.Pipe()
			defer outW.Close()

			srv := NewSDKServer(eng, inR, outW)
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

			_, isAllowAll := perm.(engine.AllowAllHandler)
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
// engine.SetThinkingConfig with the correct parameters.
func TestServe_SetMaxThinkingTokens(t *testing.T) {
	t.Run("enable thinking", func(t *testing.T) {
		eng := newMockEngine(nil, "claude-3")

		inR, inW := io.Pipe()
		outR, outW := io.Pipe()
		defer outW.Close()

		srv := NewSDKServer(eng, inR, outW)
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

		srv := NewSDKServer(eng, inR, outW)
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

	srv := NewSDKServer(eng, inR, outW)
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

// TestServe_Compact verifies that a compact control_request calls engine.Compact
// with the correct session ID.
func TestServe_Compact(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW)
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

	eng.mu.Lock()
	ids := eng.compactedSessionIDs
	eng.mu.Unlock()

	if len(ids) == 0 || ids[0] != "sess-compact-1" {
		t.Errorf("compactedSessionIDs: got %v, want [sess-compact-1]", ids)
	}

	inW.Close()
}

// TestServe_GetContextUsage verifies that get_context_usage returns token
// statistics from the engine.
func TestServe_GetContextUsage(t *testing.T) {
	eng := newMockEngine(nil, "claude-3")

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := NewSDKServer(eng, inR, outW)
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

	var info engine.ContextUsageInfo
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
