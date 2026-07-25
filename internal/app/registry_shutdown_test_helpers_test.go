package app

import (
	"context"
	"testing"
	"time"
)

const registryShutdownTestTimeout = 5 * time.Second

func stopScheduleForTest(t testing.TB, deps *RegistryDeps) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), registryShutdownTestTimeout)
	defer cancel()
	if err := deps.StopSchedule(ctx); err != nil {
		t.Errorf("StopSchedule: %v", err)
	}
}

func stopMCPRuntimeBridgeForTest(t testing.TB, deps *RegistryDeps) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), registryShutdownTestTimeout)
	defer cancel()
	if err := deps.StopMCPRuntimeBridge(ctx); err != nil {
		t.Errorf("StopMCPRuntimeBridge: %v", err)
	}
}
