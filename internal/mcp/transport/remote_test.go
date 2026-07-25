package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

func TestHTTPTransportInitializesWithHeadersAcceptAndAuth(t *testing.T) {
	var gotAccept, gotStatic, gotAuth, gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotStatic = r.Header.Get("X-Static")
		gotAuth = r.Header.Get("Authorization")
		gotUserAgent = r.Header.Get("User-Agent")

		var msg protocol.JSONRPCMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch msg.Method {
		case "initialize":
			writeRPCResult(t, w, msg.ID, initializeResult())
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("unexpected method %q", msg.Method)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(HTTPTransportConfig{
		BaseURL: server.URL,
		Headers: map[string]string{"X-Static": "static"},
		Auth: mcpauth.TokenSourceFunc(func(context.Context, string) (string, error) {
			return "token-123", nil
		}),
	})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	client, err := NewClient(context.Background(), transport)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	if gotAccept != streamableHTTPAccept {
		t.Fatalf("Accept = %q, want %q", gotAccept, streamableHTTPAccept)
	}
	if gotStatic != "static" {
		t.Fatalf("X-Static = %q", gotStatic)
	}
	if gotAuth != "Bearer token-123" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotUserAgent != defaultMCPUserAgent {
		t.Fatalf("User-Agent = %q, want current product identity", gotUserAgent)
	}
}

func TestHTTPTransportPropagatesJSONRPCError(t *testing.T) {
	var initialized atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg protocol.JSONRPCMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch msg.Method {
		case "initialize":
			writeRPCResult(t, w, msg.ID, initializeResult())
		case "notifications/initialized":
			initialized.Store(true)
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeRPCError(t, w, msg.ID, -32000, "boom")
		default:
			t.Errorf("unexpected method %q", msg.Method)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	client, err := NewClient(context.Background(), transport)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	if !initialized.Load() {
		t.Fatalf("initialized notification was not sent")
	}

	_, err = client.ListTools(context.Background())
	var rpcErr *protocol.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("ListTools error = %T %[1]v, want RPCError", err)
	}
	if rpcErr.Code != -32000 || rpcErr.Message != "boom" {
		t.Fatalf("RPCError = %#v", rpcErr)
	}
}

func TestHTTPTransportUnauthorizedIsTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="mcp", as_uri="https://auth.example.test/oauth"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	_, err = NewClient(context.Background(), transport)
	var unauthorized *mcpauth.UnauthorizedError
	if !errors.As(err, &unauthorized) {
		t.Fatalf("NewClient error = %T %[1]v, want UnauthorizedError", err)
	}
	if unauthorized.Challenge == nil || unauthorized.Challenge.Realm != "mcp" || unauthorized.Challenge.ASURI != "https://auth.example.test/oauth" {
		t.Fatalf("challenge = %#v", unauthorized.Challenge)
	}
}

func TestSSETransportInitializesAndReceivesNotificationWithoutGETTimeout(t *testing.T) {
	notificationCh := make(chan protocol.JSONRPCMessage, 1)
	emitNotification := make(chan struct{})
	releaseStream := make(chan struct{})
	var sawGET atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sawGET.Store(true)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Errorf("response writer cannot flush")
				return
			}
			flusher.Flush()
			time.Sleep(75 * time.Millisecond)
			<-emitNotification
			fmt.Fprintf(w, "event: message\nid: note-1\nretry: 1000\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/tools/list_changed\"}\n\n")
			flusher.Flush()
			select {
			case <-releaseStream:
			case <-r.Context().Done():
			}
		case http.MethodPost:
			var msg protocol.JSONRPCMessage
			if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
				t.Errorf("decode request: %v", err)
				return
			}
			switch msg.Method {
			case "initialize":
				writeRPCResult(t, w, msg.ID, initializeResult())
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			default:
				t.Errorf("unexpected method %q", msg.Method)
				w.WriteHeader(http.StatusInternalServerError)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	defer close(releaseStream)

	transport, err := NewSSETransport(SSEConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	transport.requestTimeout = 20 * time.Millisecond
	client, err := NewClient(context.Background(), transport)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	client.SetNotificationHandler("notifications/tools/list_changed", func(_ context.Context, msg protocol.JSONRPCMessage) {
		notificationCh <- msg
	})
	close(emitNotification)

	select {
	case msg := <-notificationCh:
		if msg.Method != "notifications/tools/list_changed" {
			t.Fatalf("notification method = %q", msg.Method)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for SSE notification")
	}
	if !sawGET.Load() {
		t.Fatalf("SSE GET stream was not opened")
	}
}

func TestWebSocketTransportInitializesWithMCPSubprotocol(t *testing.T) {
	notificationCh := make(chan protocol.JSONRPCMessage, 1)
	emitNotification := make(chan struct{})
	var gotProtocol, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProtocol = r.Header.Get("Sec-WebSocket-Protocol")
		gotAuth = r.Header.Get("Authorization")
		conn, rw, err := hijackWebSocket(t, w, r)
		if err != nil {
			t.Errorf("hijack websocket: %v", err)
			return
		}
		defer conn.Close()

		msg := readWebSocketJSON(t, rw.Reader)
		if msg.Method != "initialize" {
			t.Errorf("first WS method = %q", msg.Method)
			return
		}
		writeWebSocketJSON(t, conn, mustRPCResult(t, msg.ID, initializeResult()))

		msg = readWebSocketJSON(t, rw.Reader)
		if msg.Method != "notifications/initialized" {
			t.Errorf("second WS method = %q", msg.Method)
			return
		}
		<-emitNotification
		writeWebSocketJSON(t, conn, protocol.JSONRPCMessage{JSONRPC: protocol.JSONRPCVersion, Method: "notifications/tools/list_changed"})
		<-r.Context().Done()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	transport, err := NewWebSocketTransport(context.Background(), WebSocketTransportConfig{
		URL: wsURL,
		Auth: mcpauth.TokenSourceFunc(func(context.Context, string) (string, error) {
			return "ws-token", nil
		}),
	})
	if err != nil {
		t.Fatalf("NewWebSocketTransport: %v", err)
	}
	client, err := NewClient(context.Background(), transport)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	client.SetNotificationHandler("notifications/tools/list_changed", func(_ context.Context, msg protocol.JSONRPCMessage) {
		notificationCh <- msg
	})
	close(emitNotification)

	if !headerContainsToken(gotProtocol, "mcp") {
		t.Fatalf("Sec-WebSocket-Protocol = %q, want mcp", gotProtocol)
	}
	if gotAuth != "Bearer ws-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	select {
	case <-notificationCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for WebSocket notification")
	}
}

func TestHTTPTransportSendHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be reached with canceled context")
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	msg, err := protocol.NewNotificationMessage("notifications/test", nil)
	if err != nil {
		t.Fatalf("NewNotificationMessage: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := transport.Send(ctx, msg); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %T %[1]v, want context.Canceled", err)
	}
}

func TestHTTPTransportSendAndReceiveCompleteWhilePOSTSSEStaysOpen(t *testing.T) {
	streamStopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg protocol.JSONRPCMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		response := mustRPCResult(t, msg.ID, map[string]any{"value": "ready"})
		payload, err := json.Marshal(response)
		if err != nil {
			t.Errorf("marshal response: %v", err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload); err != nil {
			t.Errorf("write response: %v", err)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer cannot flush")
			return
		}
		flusher.Flush()
		<-r.Context().Done()
		close(streamStopped)
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	transport.requestTimeout = time.Second
	defer transport.Close()

	request, err := protocol.NewRequestMessage(1, "fixture/echo", nil)
	if err != nil {
		t.Fatalf("NewRequestMessage: %v", err)
	}
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- transport.Send(context.Background(), request)
	}()
	select {
	case err := <-sendDone:
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Send waited for the SSE response body to close")
	}

	receiveCtx, cancelReceive := context.WithTimeout(context.Background(), time.Second)
	defer cancelReceive()
	response, err := transport.Receive(receiveCtx)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if string(response.ID) != "1" {
		t.Fatalf("response ID = %s, want 1", response.ID)
	}

	if err := transport.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-streamStopped:
	case <-time.After(time.Second):
		t.Fatal("POST SSE handler remained blocked after Close")
	}
}

func TestHTTPTransportCallRawCompletesWhilePOSTSSEStaysOpen(t *testing.T) {
	streamStopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg protocol.JSONRPCMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		switch msg.Method {
		case "initialize":
			writeRPCResult(t, w, msg.ID, initializeResult())
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "fixture/echo":
			response := mustRPCResult(t, msg.ID, map[string]any{"value": "ready"})
			payload, err := json.Marshal(response)
			if err != nil {
				t.Errorf("marshal response: %v", err)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				t.Errorf("write response: %v", err)
				return
			}
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Error("response writer cannot flush")
				return
			}
			flusher.Flush()
			<-r.Context().Done()
			close(streamStopped)
		default:
			t.Errorf("unexpected method %q", msg.Method)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(HTTPTransportConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	transport.requestTimeout = time.Second
	defer transport.Close()
	client, err := NewClient(context.Background(), transport)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	callCtx, cancelCall := context.WithTimeout(context.Background(), time.Second)
	defer cancelCall()
	var result struct {
		Value string `json:"value"`
	}
	if err := client.CallRaw(callCtx, "fixture/echo", nil, &result); err != nil {
		t.Fatalf("CallRaw: %v", err)
	}
	if result.Value != "ready" {
		t.Fatalf("result value = %q, want ready", result.Value)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-streamStopped:
	case <-time.After(time.Second):
		t.Fatal("POST SSE handler remained blocked after client Close")
	}
}

func writeRPCResult(t *testing.T, w http.ResponseWriter, id json.RawMessage, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	msg := mustRPCResult(t, id, result)
	if err := json.NewEncoder(w).Encode(msg); err != nil {
		t.Fatalf("encode result: %v", err)
	}
}

func writeRPCError(t *testing.T, w http.ResponseWriter, id json.RawMessage, code int, message string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	msg, err := protocol.NewErrorMessage(id, code, message, nil)
	if err != nil {
		t.Fatalf("NewErrorMessage: %v", err)
	}
	if err := json.NewEncoder(w).Encode(msg); err != nil {
		t.Fatalf("encode error: %v", err)
	}
}

func mustRPCResult(t *testing.T, id json.RawMessage, result any) protocol.JSONRPCMessage {
	t.Helper()
	msg, err := protocol.NewResultMessage(id, result)
	if err != nil {
		t.Fatalf("NewResultMessage: %v", err)
	}
	return msg
}

func initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "fixture", "version": "1.0.0"},
		"instructions":    "hello",
	}
}

func hijackWebSocket(t *testing.T, w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	t.Helper()
	if !headerContainsToken(r.Header.Get("Connection"), "upgrade") {
		return nil, nil, fmt.Errorf("Connection header = %q", r.Header.Get("Connection"))
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, nil, fmt.Errorf("Upgrade header = %q", r.Header.Get("Upgrade"))
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, nil, errors.New("missing Sec-WebSocket-Key")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer cannot hijack")
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\n")
	fmt.Fprintf(conn, "Upgrade: websocket\r\n")
	fmt.Fprintf(conn, "Connection: Upgrade\r\n")
	fmt.Fprintf(conn, "Sec-WebSocket-Accept: %s\r\n", webSocketAccept(key))
	fmt.Fprintf(conn, "Sec-WebSocket-Protocol: mcp\r\n")
	fmt.Fprintf(conn, "\r\n")
	return conn, rw, nil
}

func readWebSocketJSON(t *testing.T, r *bufio.Reader) protocol.JSONRPCMessage {
	t.Helper()
	payload, opcode, _, err := readWebSocketFrame(r)
	if err != nil {
		t.Fatalf("read websocket frame: %v", err)
	}
	if opcode != webSocketOpcodeText {
		t.Fatalf("opcode = %d, want text", opcode)
	}
	var msg protocol.JSONRPCMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("decode websocket JSON: %v", err)
	}
	return msg
}

func writeWebSocketJSON(t *testing.T, conn net.Conn, msg protocol.JSONRPCMessage) {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal websocket JSON: %v", err)
	}
	if _, err := conn.Write(buildWebSocketFrame(webSocketOpcodeText, data, false)); err != nil {
		t.Fatalf("write websocket frame: %v", err)
	}
}

func headerContainsToken(header, want string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), want) {
			return true
		}
	}
	return false
}
