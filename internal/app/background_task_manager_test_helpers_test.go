package app

import (
	"context"
	"testing"

	agentruntime "github.com/agent-dance/luban/internal/agent"
)

func cleanupBackgroundTaskManager(t *testing.T, manager *agentruntime.BackgroundTaskManager) {
	t.Helper()
	t.Cleanup(func() {
		if err := manager.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown background task manager: %v", err)
		}
	})
}
