package permissions

import (
	"context"
	"errors"

	"github.com/agent-dance/luban/internal/contracts/permission"
)

// CLIPermissionHandler adapts Checker to the runtime permission contract.
type CLIPermissionHandler struct {
	checker *Checker
}

// NewCLIPermissionHandler creates a handler that uses the given Checker.
func NewCLIPermissionHandler(checker *Checker) *CLIPermissionHandler {
	return &CLIPermissionHandler{checker: checker}
}

// Check implements permission.PermissionHandler.
// DecisionAllowOnce is passed through so the engine skips session-level caching.
// DecisionAsk (no promptFunc configured) and any unknown value fall through to Deny.
func (h *CLIPermissionHandler) Check(ctx context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	opts := CheckOptions{
		AvoidPrompts:           req.AvoidPrompts,
		Required:               req.Required,
		policyRequiredAsk:      req.PolicyDecision != nil && req.PolicyDecision.IsRequiredAsk(),
		toolLocalReadOnlyAllow: req.ToolLocalReadOnlyAllow,
	}
	if req.PermissionSnapshot != nil {
		snapshot := clonePermissionRuntimeContext(*req.PermissionSnapshot)
		opts.runtimeSnapshot = &snapshot
	}
	// An explicit request mode is an execution-scope snapshot. It must win
	// even when the foreground checker has since entered AllowAll; otherwise a
	// retained background agent silently inherits a later session's bypass.
	modeValue := req.Mode
	if modeValue == "" && req.PermissionSnapshot != nil {
		modeValue = req.PermissionSnapshot.PermissionMode
	}
	if mode, ok := permissionModeOverride(modeValue); ok {
		opts.ModeOverride = &mode
	}
	promptRequest := PromptRequest{
		DecisionID:         req.DecisionID,
		SessionID:          req.SessionID,
		ExecutionSessionID: req.ExecutionSessionID,
		TurnID:             req.TurnID,
		ToolUseID:          req.ToolUseID,
		ToolName:           req.ToolName,
		Input:              req.Input,
		ActorID:            req.ActorID,
		ActorType:          req.ActorType,
		WorkUnitID:         req.WorkUnitID,
		Kind:               PromptKind(req.Kind),
		Action:             req.Action,
		Target:             req.Target,
		Impact:             req.Impact,
		RiskLevel:          promptRiskLevel(req.ToolName, req.Input),
		RiskReason:         req.RiskReason,
		RuleSource:         req.RuleSource,
		ApprovalScope:      req.ApprovalScope,
		Choices:            append([]string(nil), req.Choices...),
		Body:               req.Body,
		ReviewDetails:      append([]string(nil), req.ReviewDetails...),
		PostMode:           req.PostMode,
		Message:            req.Message,
	}
	if req.Required {
		// First-party mandatory prompts are invocation-scoped. Even a custom
		// caller-provided choice list cannot reintroduce a persistent approval.
		promptRequest.Choices = []string{"allow_once", "reject"}
	}
	response := h.checker.CheckPrompt(ctx, promptRequest, opts)
	if response.Outcome == PromptOutcomeTimedOut {
		return permission.PermissionDeny, context.DeadlineExceeded
	}
	if response.Outcome == PromptOutcomeCancelled || response.Outcome == PromptOutcomeShutdown {
		if err := ctx.Err(); err != nil {
			return permission.PermissionDeny, err
		}
		return permission.PermissionDeny, context.Canceled
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return permission.PermissionDeny, err
	}
	switch response.Decision {
	case DecisionAllow:
		return permission.PermissionAllow, nil
	case DecisionAllowOnce:
		return permission.PermissionAllowOnce, nil
	case DecisionDeny:
		return permission.PermissionDeny, nil
	default:
		// DecisionAsk with no promptFunc configured — deny for safety.
		return permission.PermissionDeny, nil
	}
}

func promptRiskLevel(toolName string, input map[string]any) int {
	switch classifyRisk(toolName, input) {
	case RiskHigh:
		return 3
	case RiskMedium:
		return 2
	default:
		return 1
	}
}

func permissionModeOverride(mode string) (Mode, bool) {
	switch mode {
	case "default":
		return ModeRuleBased, true
	case "bypassPermissions":
		return ModeAllowAll, true
	case "acceptEdits":
		return ModeAllowAll, true
	case "bubble", "dontAsk":
		return ModeAskAlways, true
	case "plan":
		return ModeAskAlways, true
	default:
		return 0, false
	}
}
