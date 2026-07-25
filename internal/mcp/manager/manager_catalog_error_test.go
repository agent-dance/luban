package manager

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

type catalogFailureTransport struct {
	*managerTestTransport
	method string
}

func (t *catalogFailureTransport) Send(ctx context.Context, msg protocol.JSONRPCMessage) error {
	if len(msg.ID) != 0 && msg.Method == t.method {
		response, err := protocol.NewErrorMessage(msg.ID, -32001, "catalog unavailable", nil)
		if err != nil {
			return err
		}
		select {
		case t.recv <- response:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-t.closed:
			return mcptransport.ErrTransportClosed
		}
	}
	return t.managerTestTransport.Send(ctx, msg)
}

func TestManagerCatalogFetchFailureCreatesFailedStateOnConnectAndReconnect(t *testing.T) {
	var builds atomic.Int32
	var closes atomic.Int32
	manager := NewManager(withTestTransportFactory(func(context.Context, string, catalog.MCPServerConfig, transportBuildOptions) (mcptransport.Transport, error) {
		generation := int(builds.Add(1))
		return &catalogFailureTransport{
			managerTestTransport: newManagerTestTransport("catalog-failure", generation, func() { closes.Add(1) }),
			method:               "tools/list",
		}, nil
	}))
	manager.AddConfig("catalog-failure", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fixture"})

	for attempt, connect := range []func() (MCPServerConnection, error){
		func() (MCPServerConnection, error) {
			return manager.GetOrConnect(context.Background(), "catalog-failure")
		},
		func() (MCPServerConnection, error) {
			return manager.Reconnect(context.Background(), "catalog-failure")
		},
	} {
		state, err := connect()
		if err != nil {
			t.Fatalf("attempt %d returned manager error: %v", attempt+1, err)
		}
		if state.Type != MCPStateFailed || state.Client != nil || !strings.Contains(state.Error, "tools/list") || !strings.Contains(state.Error, "catalog unavailable") {
			t.Fatalf("attempt %d state = %#v", attempt+1, state)
		}
		if got := closes.Load(); got != int32(attempt+1) {
			t.Fatalf("attempt %d closed clients = %d, want %d", attempt+1, got, attempt+1)
		}
		if _, ok := manager.cache.toolsSnapshot("catalog-failure"); ok {
			t.Fatalf("attempt %d published a partial tools catalog", attempt+1)
		}
	}
	if builds.Load() != 2 {
		t.Fatalf("transport builds = %d, want 2", builds.Load())
	}
}
