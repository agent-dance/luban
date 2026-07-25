package manager

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
)

func TestTask23ReplaceWorkspaceConfigsDropsLateInflightConnectionWithoutCatalogResurrection(t *testing.T) {
	startedA := make(chan struct{})
	releaseA := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var oldTransport *managerTestTransport
	var builds []string
	manager := NewManager(withTestTransportFactory(func(_ context.Context, name string, config catalog.MCPServerConfig, _ transportBuildOptions) (mcptransport.Transport, error) {
		mu.Lock()
		builds = append(builds, config.Command)
		generation := len(builds)
		mu.Unlock()
		if config.Command == "workspace-a" {
			once.Do(func() { close(startedA) })
			<-releaseA // deliberately ignore cancellation to exercise the final fence
		}
		transport := newManagerTestTransport(name, generation, nil)
		if config.Command == "workspace-a" {
			mu.Lock()
			oldTransport = transport
			mu.Unlock()
		}
		return transport, nil
	}))
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	configA := catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-a", Scope: catalog.ScopeProject}
	configB := catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-b", Scope: catalog.ScopeProject}
	normalizedB := normalizeManagerConfig("shared", configB)
	expectedBHash := catalog.HashMCPConfig(normalizedB)
	manager.AddConfig("shared", configA)
	connectA := make(chan MCPServerConnection, 1)
	connectErr := make(chan error, 1)
	go func() {
		state, err := manager.GetOrConnect(context.Background(), "shared")
		connectA <- state
		connectErr <- err
	}()
	select {
	case <-startedA:
	case <-time.After(3 * time.Second):
		t.Fatal("workspace A connection did not start")
	}

	manager.SetWorkingDirectory("/workspace-b")
	manager.ReplaceWorkspaceConfigs(map[string]catalog.MCPServerConfig{"shared": configB})
	stateB, ok := manager.State("shared")
	if !ok || stateB.Type != MCPStatePending || stateB.ConfigHash != expectedBHash || stateB.Client != nil ||
		len(stateB.Tools) != 0 || len(stateB.Resources) != 0 || len(stateB.Prompts) != 0 {
		t.Fatalf("target state before stale completion = %#v ok=%v", stateB, ok)
	}

	close(releaseA)
	select {
	case err := <-connectErr:
		if err != nil {
			t.Fatalf("stale owner returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("workspace A connection did not finish")
	}
	staleOwnerView := <-connectA
	if staleOwnerView.ConfigHash != expectedBHash || staleOwnerView.Type != MCPStatePending || staleOwnerView.Client != nil {
		t.Fatalf("stale owner observed resurrected A state: %#v", staleOwnerView)
	}

	stateB, ok = manager.State("shared")
	if !ok || stateB.ConfigHash != expectedBHash || stateB.Type != MCPStatePending || stateB.Client != nil ||
		len(stateB.Tools) != 0 || len(stateB.Resources) != 0 || len(stateB.Prompts) != 0 {
		t.Fatalf("late A result resurrected service catalog: %#v ok=%v", stateB, ok)
	}
	if _, ok := manager.cache.toolsSnapshot("shared"); ok {
		t.Fatal("late A tools remained cached")
	}
	if _, ok := manager.cache.resourcesSnapshot("shared"); ok {
		t.Fatal("late A resources remained cached")
	}
	if _, ok := manager.cache.promptsSnapshot("shared"); ok {
		t.Fatal("late A prompts remained cached")
	}
	mu.Lock()
	transportA := oldTransport
	mu.Unlock()
	closedA := false
	if transportA != nil {
		transportA.closeMu.Lock()
		closedA = transportA.isClosed
		transportA.closeMu.Unlock()
	}
	if !closedA {
		t.Fatal("late A transport was not closed")
	}

	connectedB, err := manager.GetOrConnect(context.Background(), "shared")
	if err != nil {
		t.Fatal(err)
	}
	if connectedB.Type != MCPStateConnected || connectedB.ConfigHash != expectedBHash || connectedB.Client == nil ||
		len(connectedB.Tools) != 1 || len(connectedB.Resources) != 1 || len(connectedB.Prompts) != 1 {
		t.Fatalf("target B connection = %#v", connectedB)
	}
	mu.Lock()
	gotBuilds := append([]string(nil), builds...)
	mu.Unlock()
	if !reflect.DeepEqual(gotBuilds, []string{"workspace-a", "workspace-b"}) {
		t.Fatalf("transport builds = %v", gotBuilds)
	}
}

func TestTask23LateReconnectCannotPublishWorkspaceACatalogIntoWorkspaceB(t *testing.T) {
	manager := NewManager()
	configA := normalizeManagerConfig("shared", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-a", Scope: catalog.ScopeProject})
	configB := normalizeManagerConfig("shared", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-b", Scope: catalog.ScopeProject})
	manager.AddConfig("shared", configB)

	transport := newManagerTestTransport("shared", 1, nil)
	client, err := mcptransport.NewClient(context.Background(), transport)
	if err != nil {
		t.Fatal(err)
	}
	resultA, catalogA, err := manager.connectedStateFromClient(context.Background(), "shared", configA, client)
	if err != nil {
		t.Fatal(err)
	}
	if len(resultA.Tools) != 1 || len(resultA.Resources) != 1 || len(resultA.Prompts) != 1 {
		t.Fatalf("test A catalog was not populated: %#v", resultA)
	}
	if done := manager.finishReconnectAttempt(context.Background(), "shared", configA, resultA, catalogA, 1, 1, nil); !done {
		t.Fatal("stale reconnect loop was not terminated")
	}

	stateB, ok := manager.State("shared")
	if !ok || stateB.Type != MCPStatePending || stateB.ConfigHash != catalog.HashMCPConfig(configB) || stateB.Client != nil ||
		len(stateB.Tools) != 0 || len(stateB.Resources) != 0 || len(stateB.Prompts) != 0 {
		t.Fatalf("late reconnect A changed B state: %#v ok=%v", stateB, ok)
	}
	if _, ok := manager.cache.toolsSnapshot("shared"); ok {
		t.Fatal("late reconnect A published tools")
	}
	if _, ok := manager.cache.resourcesSnapshot("shared"); ok {
		t.Fatal("late reconnect A published resources")
	}
	if _, ok := manager.cache.promptsSnapshot("shared"); ok {
		t.Fatal("late reconnect A published prompts")
	}
	transport.closeMu.Lock()
	closed := transport.isClosed
	transport.closeMu.Unlock()
	if !closed {
		t.Fatal("late reconnect A client was not closed")
	}
}

func TestTask23OldClientTeardownCannotDemoteTargetWorkspaceState(t *testing.T) {
	var mu sync.Mutex
	var transports []*managerTestTransport
	manager := NewManager(withTestTransportFactory(func(_ context.Context, name string, config catalog.MCPServerConfig, _ transportBuildOptions) (mcptransport.Transport, error) {
		mu.Lock()
		defer mu.Unlock()
		transport := newManagerTestTransport(name, len(transports)+1, nil)
		transports = append(transports, transport)
		return transport, nil
	}))
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	configA := catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-a", Scope: catalog.ScopeProject}
	configB := catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "workspace-b", Scope: catalog.ScopeProject}
	manager.AddConfig("shared", configA)
	stateA, err := manager.GetOrConnect(context.Background(), "shared")
	if err != nil {
		t.Fatal(err)
	}
	if stateA.Client == nil {
		t.Fatal("workspace A did not install a live client")
	}

	manager.ReplaceWorkspaceConfigs(map[string]catalog.MCPServerConfig{"shared": configB})
	deadline := time.Now().Add(3 * time.Second)
	for !stateA.Client.IsClosed() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !stateA.Client.IsClosed() {
		t.Fatal("workspace A client was not closed during retarget")
	}
	// Let the already-running watchClientClosed goroutine consume the old
	// client's teardown. It must reject that callback by exact client identity.
	time.Sleep(20 * time.Millisecond)
	stateB, ok := manager.State("shared")
	if !ok || stateB.Type != MCPStatePending || stateB.Client != nil || stateB.Config.Scope != catalog.ScopeProject || stateB.Config.Command != "workspace-b" {
		t.Fatalf("old client teardown demoted target state: %#v ok=%v", stateB, ok)
	}

	connectedB, err := manager.GetOrConnect(context.Background(), "shared")
	if err != nil {
		t.Fatal(err)
	}
	if connectedB.Type != MCPStateConnected || connectedB.Client == nil || connectedB.Config.Command != "workspace-b" || connectedB.Tools[0].Name != "tool-2" {
		t.Fatalf("workspace B connection = %#v", connectedB)
	}
}

func TestTask23CatalogChangeHookUnregisterRemovesOnlyItsCallback(t *testing.T) {
	manager := NewManager()
	var removedCalls atomic.Int32
	var retainedCalls atomic.Int32
	unregisterRemoved := manager.RegisterCatalogChangeHook(func() { removedCalls.Add(1) })
	unregisterRetained := manager.RegisterCatalogChangeHook(func() { retainedCalls.Add(1) })
	t.Cleanup(unregisterRetained)

	manager.mu.Lock()
	if got := len(manager.catalogHooks); got != 2 {
		manager.mu.Unlock()
		t.Fatalf("registered hook count = %d, want 2", got)
	}
	manager.mu.Unlock()
	unregisterRemoved()
	unregisterRemoved() // idempotent
	manager.mu.Lock()
	if got := len(manager.catalogHooks); got != 1 {
		manager.mu.Unlock()
		t.Fatalf("hook count after unregister = %d, want 1", got)
	}
	manager.mu.Unlock()

	manager.AddConfig("probe", catalog.MCPServerConfig{Type: catalog.TransportStdio, Command: "probe", Scope: catalog.ScopeProject})
	waitForMCPTest(t, time.Second, func() bool { return retainedCalls.Load() > 0 })
	if got := removedCalls.Load(); got != 0 {
		t.Fatalf("unregistered hook was invoked %d times", got)
	}
}
