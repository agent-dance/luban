package manager

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

func TestToggleEnabledTrueKeepsAuthoritativeLiveClient(t *testing.T) {
	var builds atomic.Int32
	manager := NewManager(withTestTransportFactory(func(context.Context, string, catalog.MCPServerConfig, transportBuildOptions) (mcptransport.Transport, error) {
		generation := int(builds.Add(1))
		return newManagerTestTransport("alpha", generation, nil), nil
	}))
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	manager.AddConfig("alpha", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})

	first, err := manager.GetOrConnect(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("first connection: %v", err)
	}
	second, err := manager.ToggleEnabled(context.Background(), "alpha", true)
	if err != nil {
		t.Fatalf("idempotent enable: %v", err)
	}
	if builds.Load() != 1 {
		t.Fatalf("transport builds = %d, want 1", builds.Load())
	}
	if first.Client == nil || second.Client != first.Client {
		t.Fatalf("live client changed across idempotent enable: first=%p second=%p", first.Client, second.Client)
	}
}

func TestPublishedLiveClientHandlesListChangedByServerName(t *testing.T) {
	var transport *managerTestTransport
	manager := NewManager(withTestTransportFactory(func(context.Context, string, catalog.MCPServerConfig, transportBuildOptions) (mcptransport.Transport, error) {
		transport = newManagerTestTransport("alpha-team", 1, nil)
		return transport, nil
	}))
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	manager.AddConfig("alpha-team", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "fake"})
	if _, err := manager.GetOrConnect(context.Background(), "alpha-team"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	transport.tools = newManagerTestTransport("alpha-team", 2, nil).tools
	notification, err := protocol.NewNotificationMessage(notificationToolsListChanged, nil)
	if err != nil {
		t.Fatalf("build notification: %v", err)
	}
	transport.recv <- notification

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, ok := manager.State("alpha-team")
		cached, cachedOK := manager.cache.toolsSnapshot("alpha-team")
		if ok && len(state.Tools) == 1 && state.Tools[0].Name == "tool-2" &&
			cachedOK && len(cached.Tools) == 1 && cached.Tools[0].Name == "tool-2" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	state, _ := manager.State("alpha-team")
	cached, cachedOK := manager.cache.toolsSnapshot("alpha-team")
	t.Fatalf("list_changed did not refresh live state/cache: state=%#v cache=%#v/%v", state.Tools, cached.Tools, cachedOK)
}
