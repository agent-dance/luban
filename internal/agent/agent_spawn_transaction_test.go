package agent

import (
	"context"
	"sync/atomic"
	"testing"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
)

func TestRollbackRegisteredAgentSessionIsRetryableAndCleansOnce(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	var cleanups atomic.Int32
	session, _, err := manager.RegisterAgentSession(
		"agent-rollback", "alias", "prompt", "description", agentcontract.Input{Prompt: "prompt"}, nil,
		agentcontract.SessionMetadata{CWD: root}, func() { cleanups.Add(1) }, nil, context.Background(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.rollbackRegisteredAgentSession("agent-rollback", session); err != nil {
		t.Fatal(err)
	}
	if err := manager.rollbackRegisteredAgentSession("agent-rollback", session); err != nil {
		t.Fatal(err)
	}
	if cleanups.Load() != 1 {
		t.Fatalf("cleanup count=%d", cleanups.Load())
	}
	if _, ok := manager.ResolveAgentTarget("agent-rollback"); ok {
		t.Fatal("rolled back agent remains addressable")
	}
	if _, ok := manager.ResolveAgentTarget("alias"); ok {
		t.Fatal("rolled back alias remains addressable")
	}
}
