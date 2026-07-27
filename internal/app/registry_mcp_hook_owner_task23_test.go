package app

import (
	"sync/atomic"
	"testing"
	"time"

	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
)

func TestTask23RegistryDepsMCPTeardownOwnsInvalidatorExactlyOnce(t *testing.T) {
	var unregisterCalls atomic.Int32
	var skillBridgeUnregisterCalls atomic.Int32
	var shutdownObserved atomic.Bool
	manager := mcpmanager.NewManager()
	ownedUnregister := manager.RegisterCatalogChangeHook(func() {
		t.Error("unregistered MCP observer ran during shutdown")
	})
	shutdownSignal := make(chan struct{}, 1)
	probeUnregister := manager.RegisterCatalogChangeHook(func() {
		if unregisterCalls.Load() == 1 && skillBridgeUnregisterCalls.Load() == 1 {
			shutdownObserved.Store(true)
		}
		select {
		case shutdownSignal <- struct{}{}:
		default:
		}
	})
	t.Cleanup(probeUnregister)
	deps := &RegistryDeps{
		mcpListChangedUnregister: func() {
			unregisterCalls.Add(1)
			ownedUnregister()
		},
		mcpSkillRuntime: &mcpSkillRuntimeBridge{
			unregister: func() {
				skillBridgeUnregisterCalls.Add(1)
			},
		},
		ServiceMCP: manager,
	}

	stopMCPRuntimeBridgeForTest(t, deps)
	stopMCPRuntimeBridgeForTest(t, deps)
	if got := unregisterCalls.Load(); got != 1 {
		t.Fatalf("MCP invalidator unregister calls = %d, want 1", got)
	}
	if got := skillBridgeUnregisterCalls.Load(); got != 1 {
		t.Fatalf("MCP skill bridge unregister calls = %d, want 1", got)
	}
	select {
	case <-shutdownSignal:
	case <-time.After(time.Second):
		t.Fatal("MCP manager shutdown was not observed")
	}
	if !shutdownObserved.Load() {
		t.Fatal("MCP manager shut down before its observers were unregistered")
	}
}
