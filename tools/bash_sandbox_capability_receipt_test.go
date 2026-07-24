package tools

import (
	"context"
	"os/exec"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

type mutableCapabilitySandbox struct {
	mu    sync.Mutex
	cap   sandbox.Capability
	calls int
}

func (b *mutableCapabilitySandbox) Name() string    { return "mutable-capability" }
func (b *mutableCapabilitySandbox) Available() bool { _, ok := b.SandboxCapability(); return ok }
func (b *mutableCapabilitySandbox) SandboxCapability() (sandbox.Capability, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cap, b.cap.ID() != ""
}
func (b *mutableCapabilitySandbox) Command(ctx context.Context, _ sandbox.Config, name string, args ...string) (*exec.Cmd, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	return exec.CommandContext(ctx, name, args...), nil
}
func (b *mutableCapabilitySandbox) setIdentity(identity string) {
	b.mu.Lock()
	b.cap = sandbox.Capability{
		Backend: "mutable-capability", ExecutablePath: "/usr/bin/mutable-capability", ExecutableIdentity: identity,
	}
	b.mu.Unlock()
}
func (b *mutableCapabilitySandbox) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func TestBashPermissionReceiptBindsSandboxExecutableCapability(t *testing.T) {
	backend := &mutableCapabilitySandbox{}
	backend.setIdentity("inode-a")
	tool := &BashTool{CWD: t.TempDir(), Sandbox: backend}
	reg := registry.New()
	reg.Register(tool)
	input := map[string]any{"command": "mkdir build"}
	request := types.ToolPermissionRequest{
		SessionID: "session", TurnID: "turn", ToolUseID: "tool-use", ApprovalEpoch: "epoch",
	}
	preflight, err := reg.CheckToolPermissions(context.Background(), tool.Name(), input, request)
	if err != nil || preflight.PermissionGrant == "" || preflight.PermissionBinding.SandboxCapability == "" {
		t.Fatalf("preflight=%#v err=%v", preflight, err)
	}
	executionGrant := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, tool.Name(), input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	if executionGrant == "" {
		t.Fatal("failed to promote capability-bound permission grant")
	}

	backend.setIdentity("inode-b")
	approved := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: executionGrant, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})
	result, err := reg.ExecuteToolWithError(approved, tool.Name(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || backend.callCount() != 0 {
		t.Fatalf("stale sandbox receipt executed: result=%#v commandCalls=%d", result, backend.callCount())
	}
}
