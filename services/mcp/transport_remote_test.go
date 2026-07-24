package mcp

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
)

func TestHTTPTransportInitializesWithHeadersAcceptAndAuth(t *testing.T) {
	var gotAccept, gotStatic, gotDynamic, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotStatic = r.Header.Get("X-Static")
		gotDynamic = r.Header.Get("X-Dynamic")
		gotAuth = r.Header.Get("Authorization")

		var msg JSONRPCMessage
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
		HeaderProvider: HeaderProviderFunc(func(context.Context, string, string) (map[string]string, error) {
			return map[string]string{"X-Dynamic": "dynamic"}, nil
		}),
		Auth: TokenSourceFunc(func(context.Context, string) (string, error) {
			return "token-123", nil
		}),
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	client, err := NewProtocolClient(context.Background(), transport, ClientOptions{})
	if err != nil {
		t.Fatalf("NewProtocolClient: %v", err)
	}
	defer client.Close()

	if gotAccept != MCPStreamableHTTPAccept {
		t.Fatalf("Accept = %q, want %q", gotAccept, MCPStreamableHTTPAccept)
	}
	if gotStatic != "static" || gotDynamic != "dynamic" {
		t.Fatalf("headers = static %q dynamic %q", gotStatic, gotDynamic)
	}
	if gotAuth != "Bearer token-123" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !client.IsInitialized() {
		t.Fatalf("client was not initialized")
	}
}

func TestHTTPTransportPropagatesJSONRPCError(t *testing.T) {
	var initialized atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg JSONRPCMessage
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
	client, err := NewProtocolClient(context.Background(), transport, ClientOptions{})
	if err != nil {
		t.Fatalf("NewProtocolClient: %v", err)
	}
	defer client.Close()
	if !initialized.Load() {
		t.Fatalf("initialized notification was not sent")
	}

	_, err = client.ListTools(context.Background())
	var rpcErr *RPCError
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
	_, err = NewProtocolClient(context.Background(), transport, ClientOptions{InitializeTimeout: time.Second})
	var unauthorized *UnauthorizedError
	if !errors.As(err, &unauthorized) {
		t.Fatalf("NewProtocolClient error = %T %[1]v, want UnauthorizedError", err)
	}
	if unauthorized.Challenge == nil || unauthorized.Challenge.Realm != "mcp" || unauthorized.Challenge.ASURI != "https://auth.example.test/oauth" {
		t.Fatalf("challenge = %#v", unauthorized.Challenge)
	}
}

func TestSSETransportInitializesAndReceivesNotificationWithoutGETTimeout(t *testing.T) {
	notificationCh := make(chan JSONRPCMessage, 1)
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
			fmt.Fprintf(w, "event: message\nid: note-1\nretry: 1000\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/tools/list_changed\"}\n\n")
			flusher.Flush()
			select {
			case <-releaseStream:
			case <-r.Context().Done():
			}
		case http.MethodPost:
			var msg JSONRPCMessage
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

	transport, err := NewSSETransport(SSEConfig{
		BaseURL:        server.URL,
		RequestTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSSETransport: %v", err)
	}
	client, err := NewProtocolClient(context.Background(), transport, ClientOptions{
		NotificationHandlers: map[string]NotificationHandler{
			"notifications/tools/list_changed": func(_ context.Context, msg JSONRPCMessage) {
				notificationCh <- msg
			},
		},
	})
	if err != nil {
		t.Fatalf("NewProtocolClient: %v", err)
	}
	defer client.Close()

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
	notificationCh := make(chan JSONRPCMessage, 1)
	var gotProtocol, gotAuth, gotDynamic string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProtocol = r.Header.Get("Sec-WebSocket-Protocol")
		gotAuth = r.Header.Get("Authorization")
		gotDynamic = r.Header.Get("X-Dynamic")
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
		writeWebSocketJSON(t, conn, JSONRPCMessage{JSONRPC: JSONRPCVersion, Method: "notifications/tools/list_changed"})
		<-r.Context().Done()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	transport, err := NewWebSocketTransport(context.Background(), WebSocketTransportConfig{
		URL: wsURL,
		Auth: TokenSourceFunc(func(context.Context, string) (string, error) {
			return "ws-token", nil
		}),
		HeaderProvider: HeaderProviderFunc(func(context.Context, string, string) (map[string]string, error) {
			return map[string]string{"X-Dynamic": "ws-dynamic"}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewWebSocketTransport: %v", err)
	}
	client, err := NewProtocolClient(context.Background(), transport, ClientOptions{
		NotificationHandlers: map[string]NotificationHandler{
			"notifications/tools/list_changed": func(_ context.Context, msg JSONRPCMessage) {
				notificationCh <- msg
			},
		},
	})
	if err != nil {
		t.Fatalf("NewProtocolClient: %v", err)
	}
	defer client.Close()

	if !headerContainsToken(gotProtocol, "mcp") {
		t.Fatalf("Sec-WebSocket-Protocol = %q, want mcp", gotProtocol)
	}
	if gotAuth != "Bearer ws-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotDynamic != "ws-dynamic" {
		t.Fatalf("X-Dynamic = %q", gotDynamic)
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
	msg, err := NewNotificationMessage("notifications/test", nil)
	if err != nil {
		t.Fatalf("NewNotificationMessage: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := transport.Send(ctx, msg); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %T %[1]v, want context.Canceled", err)
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
	msg, err := NewErrorMessage(id, code, message, nil)
	if err != nil {
		t.Fatalf("NewErrorMessage: %v", err)
	}
	if err := json.NewEncoder(w).Encode(msg); err != nil {
		t.Fatalf("encode error: %v", err)
	}
}

func mustRPCResult(t *testing.T, id json.RawMessage, result any) JSONRPCMessage {
	t.Helper()
	msg, err := NewResultMessage(id, result)
	if err != nil {
		t.Fatalf("NewResultMessage: %v", err)
	}
	return msg
}

func initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": MCPProtocolVersion,
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

func readWebSocketJSON(t *testing.T, r *bufio.Reader) JSONRPCMessage {
	t.Helper()
	payload, opcode, _, err := readWebSocketFrame(r)
	if err != nil {
		t.Fatalf("read websocket frame: %v", err)
	}
	if opcode != webSocketOpcodeText {
		t.Fatalf("opcode = %d, want text", opcode)
	}
	var msg JSONRPCMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("decode websocket JSON: %v", err)
	}
	return msg
}

func writeWebSocketJSON(t *testing.T, conn net.Conn, msg JSONRPCMessage) {
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
