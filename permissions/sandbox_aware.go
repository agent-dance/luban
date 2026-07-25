package permissions

import (
	"context"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/sandbox"
)

// SandboxAwarePermissionHandler auto-approves only sandboxed tools whose
// immutable capability proves every required protection, and delegates
// everything else to a fallback handler (typically CLIPermissionHandler).
type SandboxAwarePermissionHandler struct {
	sandbox  sandbox.Backend
	fallback permission.PermissionHandler
}

// NewSandboxAwarePermissionHandler creates a handler that auto-approves
// tools protected by the sandbox and falls back to the given handler for others.
func NewSandboxAwarePermissionHandler(sb sandbox.Backend, fallback permission.PermissionHandler) *SandboxAwarePermissionHandler {
	return &SandboxAwarePermissionHandler{sandbox: sb, fallback: fallback}
}

// Check implements permission.PermissionHandler.
// Even when the sandbox auto-approves a tool, the bypass-immune SafetyCheck
// is always evaluated first (C1 fix). If the sandbox is active and the
// requested tool is sandboxed, it returns PermissionAllow after the safety
// check passes. Otherwise it delegates to the fallback (e.g. the interactive
// CLI prompt).
func (h *SandboxAwarePermissionHandler) Check(ctx context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	if req.PolicyDecision != nil && req.PolicyDecision.IsBlock() {
		return permission.PermissionDeny, nil
	}
	// Bypass-immune safety check — always runs, even in sandbox mode.
	// This prevents sandbox auto-approval from bypassing protected-path
	// and dangerous-command checks.
	if d, _ := SafetyCheck(req.ToolName, req.Input); d == DecisionDeny {
		return permission.PermissionDeny, nil
	}
	// Tool-specific mandatory asks (including explicit Ask rules) have already
	// been classified before the general handler. Delegate them so the selected
	// permission mode can either prompt or explicitly consume PolicyRequiredAsk.
	if req.Required {
		return h.fallback.Check(ctx, req)
	}
	// Unknown/dynamic shell structure is a mandatory prompt even with a real
	// sandbox. A sandbox limits effects; it does not prove the runtime target.
	if d, _ := MandatoryApprovalCheck(req.ToolName, req.Input); d == DecisionAsk {
		return h.fallback.Check(ctx, req)
	}
	// Subagents carry a complete spawn-time policy. The mutable foreground
	// checker must evaluate that snapshot before any sandbox auto-approval.
	if req.PermissionSnapshot != nil {
		return h.fallback.Check(ctx, req)
	}

	// If sandbox is active and this tool is sandboxed, auto-approve only when
	// the permission request and this handler prove the exact same immutable
	// executable authority. Name/Available parity is insufficient: two backend
	// instances must not be spliced across approval and execution.
	capability, capabilityOK := sandbox.Snapshot(h.sandbox)
	disabled, _ := req.Input["dangerouslyDisableSandbox"].(bool)
	if req.Sandboxed && capabilityOK && req.SandboxCapability != "" &&
		capability.ID() == req.SandboxCapability && capability.Enforces(sandbox.ProtectionProtectedPaths) &&
		req.ToolName == "Bash" && !disabled {
		if cli, ok := h.fallback.(*CLIPermissionHandler); ok && cli != nil && cli.checker != nil {
			if decision, handled := cli.checker.sandboxAutoApproveDecision(req.ToolName, req.Input); handled {
				switch decision {
				case DecisionDeny:
					return permission.PermissionDeny, nil
				case DecisionAsk:
					return h.fallback.Check(ctx, req)
				default:
					return permission.PermissionAllow, nil
				}
			}
		}
		return permission.PermissionAllow, nil
	}
	// Otherwise delegate to the fallback (interactive prompt, etc.).
	return h.fallback.Check(ctx, req)
}
