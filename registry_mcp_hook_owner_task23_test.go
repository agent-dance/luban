package main

import (
	"sync/atomic"
	"testing"
)

func TestTask23RegistryDepsMCPTeardownOwnsInvalidatorExactlyOnce(t *testing.T) {
	var unregisterCalls atomic.Int32
	var skillBridgeUnregisterCalls atomic.Int32
	deps := &RegistryDeps{
		mcpListChangedUnregister: func() {
			unregisterCalls.Add(1)
		},
		mcpSkillRuntime: &mcpSkillRuntimeBridge{
			unregister: func() {
				skillBridgeUnregisterCalls.Add(1)
			},
		},
	}

	deps.StopMCPRuntimeBridge()
	deps.StopMCPRuntimeBridge()
	if got := unregisterCalls.Load(); got != 1 {
		t.Fatalf("MCP invalidator unregister calls = %d, want 1", got)
	}
	if got := skillBridgeUnregisterCalls.Load(); got != 1 {
		t.Fatalf("MCP skill bridge unregister calls = %d, want 1", got)
	}
}
