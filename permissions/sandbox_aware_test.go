package permissions

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/sandbox"
)

// mockBackend is a test-only sandbox.Backend.
type mockBackend struct {
	available bool
}

func (m *mockBackend) Name() string    { return "mock" }
func (m *mockBackend) Available() bool { return m.available }
func (m *mockBackend) SandboxCapability() (sandbox.Capability, bool) {
	if !m.available {
		return sandbox.Capability{}, false
	}
	return sandbox.Capability{
		Backend: "mock", ExecutablePath: "/usr/bin/mock-sandbox", ExecutableIdentity: "mock-v1",
	}, true
}
func (m *mockBackend) Command(_ context.Context, _ sandbox.Config, name string, args ...string) (*exec.Cmd, error) {
	return nil, errors.New("mockBackend.Command not implemented")
}

// mockFallback records whether it was called and what decision to return.
type mockFallback struct {
	called   bool
	decision engine.PermissionDecision
	err      error
}

func (f *mockFallback) Check(_ context.Context, _ engine.PermissionRequest) (engine.PermissionDecision, error) {
	f.called = true
	return f.decision, f.err
}

func bashReq() engine.PermissionRequest {
	capability, _ := (&mockBackend{available: true}).SandboxCapability()
	return engine.PermissionRequest{
		ToolName: "Bash", Input: map[string]any{"command": "ls"}, Sandboxed: true,
		SandboxCapability: capability.ID(),
	}
}

func fileReadReq() engine.PermissionRequest {
	return engine.PermissionRequest{ToolName: "FileRead", Input: map[string]any{"path": "/tmp/x"}}
}

// TestSandboxAwareHandler_AutoApprovesBash: sandbox available + Bash tool →
// PermissionAllow without calling fallback.
func TestSandboxAwareHandlerDoesNotAutoApproveIdentityOnlyBash(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &mockBackend{available: true}
	fb := &mockFallback{decision: engine.PermissionDeny}
	h := NewSandboxAwarePermissionHandler(sb, fb)

	decision, err := h.Check(context.Background(), bashReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != engine.PermissionDeny {
		t.Errorf("got %v, want fallback PermissionDeny", decision)
	}
	if !fb.called {
		t.Error("identity-only sandbox should delegate Bash approval")
	}
}

func TestSandboxAwareHandlerRejectsBackendCapabilitySplicing(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &mockBackend{available: true}
	fb := &mockFallback{decision: engine.PermissionDeny}
	h := NewSandboxAwarePermissionHandler(sb, fb)
	req := bashReq()
	req.SandboxCapability = sandbox.Capability{
		Backend: "mock", ExecutablePath: "/usr/bin/other-sandbox", ExecutableIdentity: "other-v1",
	}.ID()

	decision, err := h.Check(context.Background(), req)
	if err != nil || decision != engine.PermissionDeny || !fb.called {
		t.Fatalf("mismatched capability auto-approved: decision=%v err=%v fallback=%v", decision, err, fb.called)
	}
}

type unprotectedMockBackend struct{ mockBackend }

func (m *unprotectedMockBackend) SandboxCapability() (sandbox.Capability, bool) {
	if !m.available {
		return sandbox.Capability{}, false
	}
	return sandbox.Capability{
		Backend: "mock", ExecutablePath: "/usr/bin/mock-sandbox", ExecutableIdentity: "mock-v1",
	}, true
}

func TestSandboxAwareHandlerDoesNotAutoApproveWithoutProtectedPathEnforcement(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &unprotectedMockBackend{mockBackend{available: true}}
	fb := &mockFallback{decision: engine.PermissionDeny}
	h := NewSandboxAwarePermissionHandler(sb, fb)
	req := bashReq()
	capability, _ := sb.SandboxCapability()
	req.SandboxCapability = capability.ID()

	decision, err := h.Check(context.Background(), req)
	if err != nil || decision != engine.PermissionDeny || !fb.called {
		t.Fatalf("unprotected sandbox auto-approved: decision=%v err=%v fallback=%v", decision, err, fb.called)
	}
}

func TestSandboxAwareHandler_RequiredAskDelegatesEvenForSafeBash(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &mockBackend{available: true}
	fb := &mockFallback{decision: engine.PermissionDeny}
	h := NewSandboxAwarePermissionHandler(sb, fb)
	req := bashReq()
	req.Required = true

	decision, err := h.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb.called || decision != engine.PermissionDeny {
		t.Fatalf("required ask was sandbox-auto-approved: called=%v decision=%v", fb.called, decision)
	}
}

// TestSandboxAwareHandler_DelegatesNonBash: sandbox available + FileRead tool →
// calls fallback.
func TestSandboxAwareHandler_DelegatesNonBash(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &mockBackend{available: true}
	fb := &mockFallback{decision: engine.PermissionDeny}
	h := NewSandboxAwarePermissionHandler(sb, fb)

	decision, err := h.Check(context.Background(), fileReadReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb.called {
		t.Error("fallback should have been called for non-sandboxed tool")
	}
	if decision != engine.PermissionDeny {
		t.Errorf("got %v, want PermissionDeny (from fallback)", decision)
	}
}

// TestSandboxAwareHandler_NoSandbox: sandbox nil → always delegates to fallback.
func TestSandboxAwareHandler_NoSandbox(t *testing.T) {
	installNoopSafetyChecks(t)
	fb := &mockFallback{decision: engine.PermissionAllow}
	h := NewSandboxAwarePermissionHandler(nil, fb)

	decision, err := h.Check(context.Background(), bashReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb.called {
		t.Error("fallback should have been called when sandbox is nil")
	}
	if decision != engine.PermissionAllow {
		t.Errorf("got %v, want PermissionAllow (from fallback)", decision)
	}
}

func TestSandboxAwareHandler_NoopBackendDelegates(t *testing.T) {
	installNoopSafetyChecks(t)
	fb := &mockFallback{decision: engine.PermissionDeny}
	h := NewSandboxAwarePermissionHandler(sandbox.NoopBackend{}, fb)

	decision, err := h.Check(context.Background(), bashReq())
	if err != nil || !fb.called || decision != engine.PermissionDeny {
		t.Fatalf("NoopBackend auto-approved Bash: called=%v decision=%v err=%v", fb.called, decision, err)
	}
}

func TestSandboxAwareHandler_DoesNotAutoApproveReverseABACapability(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &mockBackend{available: false}
	fb := &mockFallback{decision: engine.PermissionDeny}
	h := NewSandboxAwarePermissionHandler(sb, fb)
	req := bashReq()
	req.Sandboxed = false // exact Bash preflight snapshot
	sb.available = true   // capability appears only during handler evaluation

	decision, err := h.Check(context.Background(), req)
	if err != nil || !fb.called || decision != engine.PermissionDeny {
		t.Fatalf("transient sandbox auto-approved unsandboxed preflight: called=%v decision=%v err=%v", fb.called, decision, err)
	}
}

// TestSandboxAwareHandler_SandboxUnavailable: sandbox.Available() returns false →
// delegates to fallback.
func TestSandboxAwareHandler_SandboxUnavailable(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &mockBackend{available: false}
	fb := &mockFallback{decision: engine.PermissionDeny}
	h := NewSandboxAwarePermissionHandler(sb, fb)

	decision, err := h.Check(context.Background(), bashReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb.called {
		t.Error("fallback should have been called when sandbox is unavailable")
	}
	if decision != engine.PermissionDeny {
		t.Errorf("got %v, want PermissionDeny (from fallback)", decision)
	}
}

func TestSandboxAwareHandler_DoesNotAutoApproveDangerouslyDisabledBash(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &mockBackend{available: true}
	fb := &mockFallback{decision: engine.PermissionDeny}
	h := NewSandboxAwarePermissionHandler(sb, fb)

	req := bashReq()
	req.Input["dangerouslyDisableSandbox"] = true

	decision, err := h.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fb.called {
		t.Error("fallback should have been called when sandbox is explicitly disabled")
	}
	if decision != engine.PermissionDeny {
		t.Errorf("got %v, want PermissionDeny (from fallback)", decision)
	}
}

func TestSandboxAwareHandler_RespectsToolBlacklist(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &mockBackend{available: true}
	checker := NewChecker(ModeAskAlways, nil)
	checker.SetPromptFunc(func(string, map[string]any) Decision { return DecisionAllow })
	checker.SetDisallowedTools([]string{"Bash"})
	h := NewSandboxAwarePermissionHandler(sb, NewCLIPermissionHandler(checker))

	decision, err := h.Check(context.Background(), bashReq())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != engine.PermissionDeny {
		t.Errorf("got %v, want PermissionDeny", decision)
	}
}

func TestSandboxAwareHandler_RespectsRuleBasedAskForSandboxedBash(t *testing.T) {
	installNoopSafetyChecks(t)
	sb := &mockBackend{available: true}
	checker := NewChecker(ModeRuleBased, []Rule{
		{Tool: "Bash", Pattern: "echo", Decision: DecisionAsk},
	})
	promptCount := 0
	checker.SetPromptFunc(func(string, map[string]any) Decision {
		promptCount++
		return DecisionAllowOnce
	})
	h := NewSandboxAwarePermissionHandler(sb, NewCLIPermissionHandler(checker))

	req := bashReq()
	req.Input["command"] = "echo hi"

	decision, err := h.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != engine.PermissionAllow {
		t.Errorf("got %v, want PermissionAllow", decision)
	}
	if promptCount != 1 {
		t.Fatalf("expected prompt to be called once, got %d", promptCount)
	}
}
