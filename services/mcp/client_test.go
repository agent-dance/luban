package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type testTransport struct {
	sent   chan JSONRPCMessage
	recv   chan JSONRPCMessage
	closed chan struct{}
	once   sync.Once
}

func newTestTransport() *testTransport {
	return &testTransport{
		sent:   make(chan JSONRPCMessage, 32),
		recv:   make(chan JSONRPCMessage, 32),
		closed: make(chan struct{}),
	}
}

func (t *testTransport) Send(ctx context.Context, msg JSONRPCMessage) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closed:
		return &TransportClosedError{Reason: "test transport closed", Err: ErrTransportClosed}
	case t.sent <- msg:
		return nil
	}
}

func (t *testTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	select {
	case <-ctx.Done():
		return JSONRPCMessage{}, ctx.Err()
	case <-t.closed:
		return JSONRPCMessage{}, &TransportClosedError{Reason: "test transport closed", Err: ErrTransportClosed}
	case msg := <-t.recv:
		return msg, nil
	}
}

func (t *testTransport) Close() error {
	t.once.Do(func() {
		close(t.closed)
	})
	return nil
}

func (t *testTransport) nextSent(tb testing.TB) JSONRPCMessage {
	tb.Helper()
	select {
	case msg := <-t.sent:
		return msg
	case <-time.After(2 * time.Second):
		tb.Fatal("timed out waiting for sent JSON-RPC message")
		return JSONRPCMessage{}
	}
}

func (t *testTransport) push(tb testing.TB, msg JSONRPCMessage) {
	tb.Helper()
	select {
	case t.recv <- msg:
	case <-time.After(2 * time.Second):
		tb.Fatal("timed out pushing JSON-RPC message")
	}
}

func startInitializedClient(tb testing.TB, options ClientOptions) (*Client, *testTransport) {
	tb.Helper()
	transport := newTestTransport()
	clientCh := make(chan *Client, 1)
	errCh := make(chan error, 1)
	go func() {
		client, err := NewProtocolClient(context.Background(), transport, options)
		if err != nil {
			errCh <- err
			return
		}
		clientCh <- client
	}()

	initReq := transport.nextSent(tb)
	if initReq.Method != "initialize" {
		tb.Fatalf("first request method = %q, want initialize", initReq.Method)
	}
	transport.push(tb, mustResultMessage(tb, initReq.ID, map[string]any{
		"protocolVersion": MCPProtocolVersion,
		"capabilities": map[string]any{
			"tools":     map[string]any{"listChanged": true},
			"resources": map[string]any{"subscribe": true},
			"prompts":   map[string]any{},
		},
		"serverInfo": map[string]any{"name": "fixture", "version": "1.0.0"},
	}))

	initialized := transport.nextSent(tb)
	if initialized.Method != "notifications/initialized" {
		tb.Fatalf("second message method = %q, want notifications/initialized", initialized.Method)
	}

	select {
	case err := <-errCh:
		tb.Fatalf("NewProtocolClient: %v", err)
		return nil, nil
	case client := <-clientCh:
		tb.Cleanup(func() { _ = client.Close() })
		return client, transport
	case <-time.After(2 * time.Second):
		tb.Fatal("timed out waiting for initialized client")
		return nil, nil
	}
}

func mustResultMessage(tb testing.TB, id json.RawMessage, result any) JSONRPCMessage {
	tb.Helper()
	msg, err := NewResultMessage(id, result)
	if err != nil {
		tb.Fatalf("NewResultMessage: %v", err)
	}
	return msg
}

func TestClientInitializeStoresCapabilitiesServerInfoAndInstructions(t *testing.T) {
	transport := newTestTransport()
	longInstructions := strings.Repeat("x", MaxMCPDescriptionLength+12)
	clientCh := make(chan *Client, 1)
	errCh := make(chan error, 1)
	go func() {
		client, err := NewProtocolClient(context.Background(), transport, ClientOptions{
			ClientInfo: ClientInfo{Version: "9.8.7"},
		})
		if err != nil {
			errCh <- err
			return
		}
		clientCh <- client
	}()

	initReq := transport.nextSent(t)
	if initReq.Method != "initialize" {
		t.Fatalf("method = %q, want initialize", initReq.Method)
	}
	var params struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ClientInfo      ClientInfo     `json:"clientInfo"`
	}
	if err := json.Unmarshal(initReq.Params, &params); err != nil {
		t.Fatalf("decode initialize params: %v", err)
	}
	if params.ProtocolVersion != MCPProtocolVersion {
		t.Fatalf("protocolVersion = %q, want %q", params.ProtocolVersion, MCPProtocolVersion)
	}
	if params.ClientInfo.Name != "claude-code" || params.ClientInfo.Title != "Claude Code" || params.ClientInfo.Version != "9.8.7" {
		t.Fatalf("clientInfo mismatch: %#v", params.ClientInfo)
	}
	if _, ok := params.Capabilities["roots"].(map[string]any); !ok {
		t.Fatalf("initialize capabilities missing roots: %#v", params.Capabilities)
	}
	if _, ok := params.Capabilities["elicitation"].(map[string]any); !ok {
		t.Fatalf("initialize capabilities missing empty elicitation object: %#v", params.Capabilities)
	}

	transport.push(t, mustResultMessage(t, initReq.ID, map[string]any{
		"protocolVersion": MCPProtocolVersion,
		"capabilities": map[string]any{
			"tools":     map[string]any{"listChanged": true},
			"resources": map[string]any{"subscribe": true},
			"prompts":   map[string]any{},
		},
		"serverInfo":   map[string]any{"name": "fixture", "version": "2.3.4"},
		"instructions": longInstructions,
	}))

	initialized := transport.nextSent(t)
	if initialized.Method != "notifications/initialized" || len(initialized.ID) != 0 {
		t.Fatalf("initialized notification mismatch: %#v", initialized)
	}

	var client *Client
	select {
	case err := <-errCh:
		t.Fatalf("NewProtocolClient: %v", err)
	case client = <-clientCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for client")
	}
	defer client.Close() //nolint:errcheck

	caps := client.GetServerCapabilities()
	if caps["tools"] == nil || caps["resources"] == nil || caps["prompts"] == nil {
		t.Fatalf("capabilities not stored: %#v", caps)
	}
	info := client.GetServerInfo()
	if info == nil || info.Name != "fixture" || info.Version != "2.3.4" {
		t.Fatalf("serverInfo not stored: %#v", info)
	}
	if got := client.GetInstructions(); len(got) != MaxMCPDescriptionLength+len(mcpTruncationSuffix) || !strings.HasSuffix(got, mcpTruncationSuffix) {
		t.Fatalf("instructions were not TS-truncated: len=%d suffix=%q", len(got), got[len(got)-len(mcpTruncationSuffix):])
	}
	if len(client.InitializeRaw()) == 0 {
		t.Fatalf("initialize raw result was not preserved")
	}
}

func TestClientConcurrentCallsCorrelateResponsesByID(t *testing.T) {
	client, transport := startInitializedClient(t, ClientOptions{})

	type callResult struct {
		name string
		got  map[string]any
		err  error
	}
	results := make(chan callResult, 2)
	for _, name := range []string{"first", "second"} {
		name := name
		go func() {
			var out map[string]any
			err := client.CallRaw(context.Background(), "fixture/echo", map[string]any{"name": name}, &out)
			results <- callResult{name: name, got: out, err: err}
		}()
	}

	reqA := transport.nextSent(t)
	reqB := transport.nextSent(t)
	var paramsA, paramsB map[string]string
	if err := json.Unmarshal(reqA.Params, &paramsA); err != nil {
		t.Fatalf("decode params A: %v", err)
	}
	if err := json.Unmarshal(reqB.Params, &paramsB); err != nil {
		t.Fatalf("decode params B: %v", err)
	}

	transport.push(t, mustResultMessage(t, reqB.ID, map[string]any{"name": paramsB["name"]}))
	transport.push(t, mustResultMessage(t, reqA.ID, map[string]any{"name": paramsA["name"]}))

	got := map[string]string{}
	for i := 0; i < 2; i++ {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("CallRaw(%s): %v", result.name, result.err)
			}
			got[result.name] = result.got["name"].(string)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for concurrent calls")
		}
	}
	if got["first"] != "first" || got["second"] != "second" {
		t.Fatalf("responses were not correlated by id: %#v", got)
	}
}

func TestClientNotificationHandlerDoesNotBlockResponses(t *testing.T) {
	client, transport := startInitializedClient(t, ClientOptions{})
	started := make(chan struct{})
	release := make(chan struct{})
	client.SetNotificationHandler("notifications/tools/list_changed", func(context.Context, JSONRPCMessage) {
		close(started)
		<-release
	})

	errCh := make(chan error, 1)
	go func() {
		var out map[string]any
		errCh <- client.CallRaw(context.Background(), "tools/list", nil, &out)
	}()
	req := transport.nextSent(t)
	transport.push(t, JSONRPCMessage{JSONRPC: JSONRPCVersion, Method: "notifications/tools/list_changed"})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("notification handler did not start")
	}

	transport.push(t, mustResultMessage(t, req.ID, map[string]any{"tools": []any{}}))
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("CallRaw while notification blocked: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked notification prevented response processing")
	}
	close(release)
}

func TestClientDefaultRequestHandlersRespondToRootsAndElicitation(t *testing.T) {
	client, transport := startInitializedClient(t, ClientOptions{
		Roots: []Root{{URI: "file:///tmp/project", Name: "project"}},
	})

	rootsReq := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`77`),
		Method:  "roots/list",
	}
	transport.push(t, rootsReq)
	rootsResp := transport.nextSent(t)
	if string(rootsResp.ID) != "77" || rootsResp.Error != nil {
		t.Fatalf("roots/list response mismatch: %#v", rootsResp)
	}
	var roots struct {
		Roots []Root `json:"roots"`
	}
	if err := json.Unmarshal(rootsResp.Result, &roots); err != nil {
		t.Fatalf("decode roots response: %v", err)
	}
	if len(roots.Roots) != 1 || roots.Roots[0].URI != "file:///tmp/project" {
		t.Fatalf("roots/list result mismatch: %#v", roots)
	}

	elicitationReq := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`78`),
		Method:  "elicitation/create",
	}
	transport.push(t, elicitationReq)
	elicitationResp := transport.nextSent(t)
	var elicitation map[string]string
	if err := json.Unmarshal(elicitationResp.Result, &elicitation); err != nil {
		t.Fatalf("decode elicitation response: %v", err)
	}
	if elicitation["action"] != "cancel" {
		t.Fatalf("elicitation default action = %#v, want cancel", elicitation)
	}

	_ = client.Close()
}

func TestClientCloseRejectsInflightRequests(t *testing.T) {
	client, transport := startInitializedClient(t, ClientOptions{})

	errCh := make(chan error, 1)
	go func() {
		var out map[string]any
		errCh <- client.CallRaw(context.Background(), "tools/list", nil, &out)
	}()
	_ = transport.nextSent(t)

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrTransportClosed) {
			t.Fatalf("CallRaw error = %T %[1]v, want ErrTransportClosed", err)
		}
		var closed *TransportClosedError
		if !errors.As(err, &closed) {
			t.Fatalf("CallRaw error = %T, want *TransportClosedError", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("inflight request did not reject after Close")
	}
}

func TestClientRawCallPreservesStructuredToolEnvelope(t *testing.T) {
	client, transport := startInitializedClient(t, ClientOptions{})

	errCh := make(chan error, 1)
	var raw json.RawMessage
	go func() {
		errCh <- client.CallRaw(context.Background(), "tools/call", map[string]any{"name": "rich"}, &raw)
	}()
	req := transport.nextSent(t)
	transport.push(t, mustResultMessage(t, req.ID, map[string]any{
		"content": []map[string]any{{
			"type":        "text",
			"text":        "hello",
			"uri":         "memo://tool",
			"mimeType":    "text/plain",
			"annotations": map[string]any{"audience": "assistant"},
		}},
		"structuredContent": map[string]any{"answer": 42},
		"_meta":             map[string]any{"trace": "abc"},
	}))
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("CallRaw: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CallRaw")
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("raw result is not JSON: %v", err)
	}
	if parsed["structuredContent"] == nil || parsed["_meta"] == nil {
		t.Fatalf("raw envelope dropped structuredContent/_meta: %s", raw)
	}
	content := parsed["content"].([]any)[0].(map[string]any)
	if content["uri"] != "memo://tool" || content["mimeType"] != "text/plain" || content["annotations"] == nil {
		t.Fatalf("raw content envelope dropped fields: %#v", content)
	}
}
