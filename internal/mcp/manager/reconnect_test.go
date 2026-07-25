package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

func fastReconnectPolicy() reconnectPolicy {
	return reconnectPolicy{
		RemoteMaxAttempts:       3,
		RemoteInitialDelay:      time.Millisecond,
		RemoteMaxDelay:          time.Millisecond,
		RemoteJitterFraction:    0,
		ConnectionLostThreshold: 3,
		StdioCooldowns:          []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
	}
}

func sessionExpiredErr() error {
	return &mcptransport.RemoteHTTPError{
		StatusCode: 404,
		Status:     "404 Not Found",
		Body:       `{"jsonrpc":"2.0","error":{"code":-32001,"message":"Session not found"},"id":1}`,
		RPCError:   &protocol.RPCError{Code: sessionNotFoundJSONRPCCode, Message: "Session not found"},
	}
}

func TestSessionExpiredDetectionRequires404AndRPCCode(t *testing.T) {
	if !IsSessionExpiredError(sessionExpiredErr()) {
		t.Fatalf("expected 404/-32001 to be treated as session expiry")
	}
	if IsSessionExpiredError(&mcptransport.RemoteHTTPError{StatusCode: 404, Body: "not json-rpc"}) {
		t.Fatalf("generic 404 must not be treated as session expiry")
	}
	if IsSessionExpiredError(&mcptransport.RemoteHTTPError{StatusCode: 500, RPCError: &protocol.RPCError{Code: sessionNotFoundJSONRPCCode}}) {
		t.Fatalf("-32001 without HTTP 404 must not be treated as session expiry")
	}
}

func TestSessionExpiryClearsCachesAndNextCallReconnects(t *testing.T) {
	var calls atomic.Int32
	manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
		generation := int(calls.Add(1))
		return newManagerTestTransport(name, generation, nil), nil
	}))
	manager.AddConfig("remote", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"})

	first, err := manager.GetOrConnect(context.Background(), "remote")
	if err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if first.Type != MCPStateConnected || first.Tools[0].Name != "tool-1" {
		t.Fatalf("first state = %#v", first)
	}
	if _, ok := manager.cache.toolsSnapshot("remote"); !ok {
		t.Fatalf("expected tools cache after connect")
	}

	if !manager.markSessionExpired("remote", sessionExpiredErr()) {
		t.Fatalf("markSessionExpired returned false")
	}
	state, ok := manager.State("remote")
	if !ok || state.Type != MCPStatePending || state.Client != nil {
		t.Fatalf("state after session expiry = %#v ok=%v", state, ok)
	}
	if _, ok := manager.cache.toolsSnapshot("remote"); ok {
		t.Fatalf("tools cache survived session expiry")
	}

	second, err := manager.GetOrConnect(context.Background(), "remote")
	if err != nil {
		t.Fatalf("GetOrConnect after session expiry: %v", err)
	}
	if second.Type != MCPStateConnected || second.Tools[0].Name != "tool-2" || calls.Load() != 2 {
		t.Fatalf("second state=%#v calls=%d", second, calls.Load())
	}
}

type blockingSessionTransport struct {
	recv    chan transportRead
	closed  chan struct{}
	sawSlow chan struct{}

	closeOnce sync.Once
	slowOnce  sync.Once
}

type transportRead struct {
	msg protocol.JSONRPCMessage
	err error
}

func newBlockingSessionTransport() *blockingSessionTransport {
	return &blockingSessionTransport{
		recv:    make(chan transportRead, 8),
		closed:  make(chan struct{}),
		sawSlow: make(chan struct{}),
	}
}

func (t *blockingSessionTransport) Send(ctx context.Context, msg protocol.JSONRPCMessage) error {
	if len(msg.ID) == 0 {
		return nil
	}
	var result any
	switch msg.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": testMCPProtocolVersion,
			"capabilities":    catalog.ServerCapabilities{},
			"serverInfo":      &catalog.ServerInfo{Name: "session-test"},
		}
	case "slow":
		t.slowOnce.Do(func() { close(t.sawSlow) })
		return nil
	default:
		return fmt.Errorf("unexpected method %s", msg.Method)
	}
	response, err := protocol.NewResultMessage(msg.ID, result)
	if err != nil {
		return err
	}
	select {
	case t.recv <- transportRead{msg: response}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closed:
		return mcptransport.NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, mcptransport.ErrTransportClosed, "blocking session")
	}
}

func (t *blockingSessionTransport) Receive(ctx context.Context) (protocol.JSONRPCMessage, error) {
	select {
	case result := <-t.recv:
		return result.msg, result.err
	case <-ctx.Done():
		return protocol.JSONRPCMessage{}, ctx.Err()
	case <-t.closed:
		return protocol.JSONRPCMessage{}, mcptransport.NewTransportClosedError(i18n.KeyServicesMCPTransportClosedStateReason, mcptransport.ErrTransportClosed, "blocking session")
	}
}

func (t *blockingSessionTransport) Close() error {
	t.closeOnce.Do(func() { close(t.closed) })
	return nil
}

func (t *blockingSessionTransport) triggerError(err error) {
	t.recv <- transportRead{err: err}
}

func TestSessionExpiryClosesPendingCalls(t *testing.T) {
	transport := newBlockingSessionTransport()
	manager := NewManager(withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
		return transport, nil
	}))
	manager.AddConfig("remote", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"})
	state, err := manager.GetOrConnect(context.Background(), "remote")
	if err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		var raw json.RawMessage
		done <- state.Client.CallRaw(context.Background(), "slow", nil, &raw)
	}()

	select {
	case <-transport.sawSlow:
	case <-time.After(time.Second):
		t.Fatalf("slow call was not sent")
	}
	transport.triggerError(sessionExpiredErr())

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("pending call returned nil error")
		}
		var expired *SessionExpiredError
		if !errors.As(err, &expired) {
			t.Fatalf("pending call error = %T %v, want SessionExpiredError in chain", err, err)
		}
	case <-time.After(time.Second):
		t.Fatalf("pending call hung after session expiry")
	}
	waitForMCPTest(t, time.Second, func() bool {
		state, ok := manager.State("remote")
		return ok && state.Type == MCPStatePending && state.Client == nil
	})
}

func TestRemoteDisconnectReconnectsAutomatically(t *testing.T) {
	var calls atomic.Int32
	manager := NewManager(
		withTestReconnectPolicy(fastReconnectPolicy()),
		withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
			generation := int(calls.Add(1))
			return newManagerTestTransport(name, generation, nil), nil
		}),
	)
	manager.AddConfig("remote", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"})
	first, err := manager.GetOrConnect(context.Background(), "remote")
	if err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if err := first.Client.Close(); err != nil {
		t.Fatalf("close first client: %v", err)
	}

	waitForMCPTest(t, time.Second, func() bool {
		state, ok := manager.State("remote")
		return ok && state.Type == MCPStateConnected && len(state.Tools) == 1 && state.Tools[0].Name == "tool-2"
	})
	if calls.Load() != 2 {
		t.Fatalf("transport calls = %d, want 2", calls.Load())
	}
}

func TestStdioExitReconnectsWithinCooldownPolicy(t *testing.T) {
	var calls atomic.Int32
	manager := NewManager(
		withTestReconnectPolicy(fastReconnectPolicy()),
		withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
			generation := int(calls.Add(1))
			return newManagerTestTransport(name, generation, nil), nil
		}),
	)
	manager.AddConfig("local", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})
	first, err := manager.GetOrConnect(context.Background(), "local")
	if err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if err := first.Client.Close(); err != nil {
		t.Fatalf("close first client: %v", err)
	}

	waitForMCPTest(t, time.Second, func() bool {
		state, ok := manager.State("local")
		return ok && state.Type == MCPStateConnected && len(state.Tools) == 1 && state.Tools[0].Name == "tool-2"
	})
}

func TestManualReconnectCancelsPendingAutomaticTimer(t *testing.T) {
	var calls atomic.Int32
	policy := fastReconnectPolicy()
	policy.RemoteInitialDelay = 100 * time.Millisecond
	policy.RemoteMaxDelay = 100 * time.Millisecond
	manager := NewManager(
		withTestReconnectPolicy(policy),
		withTestTransportFactory(func(ctx context.Context, name string, cfg catalog.MCPServerConfig, opts transportBuildOptions) (mcptransport.Transport, error) {
			generation := int(calls.Add(1))
			if generation == 2 {
				return nil, errors.New("transient reconnect failure")
			}
			return newManagerTestTransport(name, generation, nil), nil
		}),
	)
	manager.AddConfig("remote", catalog.MCPServerConfig{Type: catalog.TransportHTTP, URL: "http://example.test/mcp"})
	first, err := manager.GetOrConnect(context.Background(), "remote")
	if err != nil {
		t.Fatalf("GetOrConnect: %v", err)
	}
	if err := first.Client.Close(); err != nil {
		t.Fatalf("close first client: %v", err)
	}
	waitForMCPTest(t, time.Second, func() bool {
		state, ok := manager.State("remote")
		return calls.Load() == 2 && ok && state.Type == MCPStatePending && state.ReconnectAttempt == 1
	})

	manual, err := manager.Reconnect(context.Background(), "remote")
	if err != nil {
		t.Fatalf("manual Reconnect: %v", err)
	}
	if manual.Type != MCPStateConnected || calls.Load() != 3 {
		t.Fatalf("manual reconnect state=%#v calls=%d", manual, calls.Load())
	}
	time.Sleep(150 * time.Millisecond)
	if calls.Load() != 3 {
		t.Fatalf("automatic timer fired after manual reconnect; calls=%d", calls.Load())
	}
}

func waitForMCPTest(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if cond() {
		return
	}
	t.Fatalf("condition not met within %s", timeout)
}
