package transport

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/mcp/protocol"
)

func TestWebSocketReceiveCancellationClosesConnAndReleasesReadLock(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	observed := newObservedWebSocketConn(clientConn)
	transport := newPipeWebSocketTransport(observed)

	ctx, cancel := context.WithCancel(context.Background())
	receiveDone := make(chan error, 1)
	go func() {
		_, err := transport.Receive(ctx)
		receiveDone <- err
	}()
	waitForSignal(t, observed.readStarted, "WebSocket read")
	cancel()

	if err := waitForError(t, receiveDone, "Receive cancellation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Receive error = %T %[1]v, want context.Canceled", err)
	}
	assertMutexReleased(t, &transport.readMu, "read")
	assertPipePeerClosed(t, serverConn)
}

func TestWebSocketSendCancellationClosesConnAndReleasesWriteLock(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	observed := newObservedWebSocketConn(clientConn)
	transport := newPipeWebSocketTransport(observed)
	message, err := protocol.NewNotificationMessage("notifications/test", nil)
	if err != nil {
		t.Fatalf("NewNotificationMessage: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sendDone := make(chan error, 1)
	go func() { sendDone <- transport.Send(ctx, message) }()
	waitForSignal(t, observed.writeStarted, "WebSocket write")
	cancel()

	if err := waitForError(t, sendDone, "Send cancellation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %T %[1]v, want context.Canceled", err)
	}
	assertMutexReleased(t, &transport.writeMu, "write")
	assertPipePeerClosed(t, serverConn)
}

func TestWebSocketPreCanceledOperationsDoNotCloseHealthyConn(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	observed := newObservedWebSocketConn(clientConn)
	transport := newPipeWebSocketTransport(observed)
	defer transport.Close()
	message, err := protocol.NewNotificationMessage("notifications/test", nil)
	if err != nil {
		t.Fatalf("NewNotificationMessage: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := transport.Receive(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Receive error = %T %[1]v, want context.Canceled", err)
	}
	if err := transport.Send(ctx, message); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %T %[1]v, want context.Canceled", err)
	}
	assertSignalNotClosed(t, observed.readStarted, "WebSocket read")
	assertSignalNotClosed(t, observed.writeStarted, "WebSocket write")
	if err := serverConn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("peer connection was closed by a pre-canceled operation: %v", err)
	}
	if err := serverConn.SetDeadline(time.Time{}); err != nil {
		t.Fatalf("clear peer deadline: %v", err)
	}
}

func TestWebSocketHandshakeCancellationClosesRealConnection(t *testing.T) {
	requestStarted := make(chan struct{})
	handlerDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
		close(handlerDone)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	dialDone := make(chan error, 1)
	go func() {
		_, err := NewWebSocketTransport(ctx, WebSocketTransportConfig{
			URL: "ws" + strings.TrimPrefix(server.URL, "http"),
		})
		dialDone <- err
	}()
	waitForSignal(t, requestStarted, "WebSocket handshake request")
	cancel()

	if err := waitForError(t, dialDone, "handshake cancellation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("NewWebSocketTransport error = %T %[1]v, want context.Canceled", err)
	}
	waitForSignal(t, handlerDone, "handshake connection close")
}

func TestWebSocketHandshakeRejectsInvalidHeaders(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{name: "carriage return in name", headers: map[string]string{"X-Safe\rX-Injected": "value"}},
		{name: "newline in name", headers: map[string]string{"X-Safe\nX-Injected": "value"}},
		{name: "carriage return in value", headers: map[string]string{"X-Safe": "value\rX-Injected: true"}},
		{name: "newline in value", headers: map[string]string{"X-Safe": "value\nX-Injected: true"}},
		{name: "colon in name", headers: map[string]string{"X-Safe: X-Injected": "value"}},
		{name: "control byte in value", headers: map[string]string{"X-Safe": "value\x00suffix"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport, err := NewWebSocketTransport(context.Background(), WebSocketTransportConfig{
				URL:     wsURL,
				Headers: test.headers,
			})
			if transport != nil || err == nil {
				t.Fatalf("NewWebSocketTransport = (%#v, %v), want header validation error", transport, err)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("server handled %d requests for rejected handshake headers", got)
	}
}

type observedWebSocketConn struct {
	net.Conn
	readStarted  chan struct{}
	writeStarted chan struct{}
	readOnce     sync.Once
	writeOnce    sync.Once
}

func newObservedWebSocketConn(conn net.Conn) *observedWebSocketConn {
	return &observedWebSocketConn{
		Conn:         conn,
		readStarted:  make(chan struct{}),
		writeStarted: make(chan struct{}),
	}
}

func (c *observedWebSocketConn) Read(p []byte) (int, error) {
	c.readOnce.Do(func() { close(c.readStarted) })
	return c.Conn.Read(p)
}

func (c *observedWebSocketConn) Write(p []byte) (int, error) {
	c.writeOnce.Do(func() { close(c.writeStarted) })
	return c.Conn.Write(p)
}

func newPipeWebSocketTransport(conn net.Conn) *webSocketTransport {
	return &webSocketTransport{
		conn: conn,
		br:   bufio.NewReader(conn),
	}
}

func waitForError(t *testing.T, result <-chan error, operation string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", operation)
		return nil
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", operation)
	}
}

func assertSignalNotClosed(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatalf("%s started for a pre-canceled operation", operation)
	default:
	}
}

func assertMutexReleased(t *testing.T, mutex *sync.Mutex, name string) {
	t.Helper()
	locked := make(chan struct{})
	go func() {
		mutex.Lock()
		close(locked)
		mutex.Unlock()
	}()
	waitForSignal(t, locked, name+" mutex release")
}

func assertPipePeerClosed(t *testing.T, conn net.Conn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
			return
		}
		t.Fatalf("SetReadDeadline: %v", err)
	}
	var one [1]byte
	_, err := conn.Read(one[:])
	if err == nil {
		t.Fatal("peer connection remained readable after transport cancellation")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		t.Fatal("peer connection remained open until the read deadline")
	}
}
