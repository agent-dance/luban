// Package permission defines the runtime-neutral authorization contract shared
// by execution loops, engine orchestration, and first-party permission policy.
package permission

import (
	"context"

	"github.com/agent-dance/luban/types"
)

// PermissionDecision is the outcome of a permission check.
type PermissionDecision int

const (
	// PermissionAllow permits the tool call and may be cached for the session.
	PermissionAllow PermissionDecision = iota
	// PermissionDeny blocks the tool call.
	PermissionDeny
	// PermissionAllowOnce permits only this invocation.
	PermissionAllowOnce
)

// PermissionRequest describes one tool invocation awaiting authorization.
type PermissionRequest struct {
	SessionID              string                    `json:"session_id"`
	ExecutionSessionID     string                    `json:"execution_session_id,omitempty"`
	TurnID                 string                    `json:"turn_id,omitempty"`
	DecisionID             string                    `json:"decision_id,omitempty"`
	ToolUseID              string                    `json:"tool_use_id,omitempty"`
	ToolName               string                    `json:"tool_name"`
	Input                  map[string]any            `json:"input"`
	ActorID                string                    `json:"actor_id,omitempty"`
	ActorType              string                    `json:"actor_type,omitempty"`
	WorkUnitID             string                    `json:"work_unit_id,omitempty"`
	Kind                   string                    `json:"kind,omitempty"`
	Action                 string                    `json:"action,omitempty"`
	Target                 string                    `json:"target,omitempty"`
	Impact                 string                    `json:"impact,omitempty"`
	RiskReason             string                    `json:"risk_reason,omitempty"`
	RuleSource             string                    `json:"rule_source,omitempty"`
	ApprovalScope          string                    `json:"approval_scope,omitempty"`
	Choices                []string                  `json:"choices,omitempty"`
	Body                   string                    `json:"body,omitempty"`
	ReviewDetails          []string                  `json:"review_details,omitempty"`
	PostMode               string                    `json:"post_mode,omitempty"`
	Description            string                    `json:"description,omitempty"`
	Mode                   string                    `json:"mode,omitempty"`
	AvoidPrompts           bool                      `json:"avoid_prompts,omitempty"`
	Required               bool                      `json:"required,omitempty"`
	ToolLocalReadOnlyAllow bool                      `json:"tool_local_read_only_allow,omitempty"`
	Sandboxed              bool                      `json:"sandboxed,omitempty"`
	SandboxCapability      string                    `json:"sandbox_capability,omitempty"`
	Message                string                    `json:"message,omitempty"`
	Suggestions            []types.PermissionUpdate  `json:"suggestions,omitempty"`
	BlockedPath            string                    `json:"blocked_path,omitempty"`
	PolicyDecision         *types.PolicyDecision     `json:"policy_decision,omitempty"`
	PermissionSnapshot     *types.ToolRuntimeContext `json:"permission_snapshot,omitempty"`
}

// PermissionHandler decides whether a tool call is allowed. Implementations
// must respect context cancellation. A PermissionAllow response for a required
// request attests that the configured authority approved that exact call.
type PermissionHandler interface {
	Check(context.Context, PermissionRequest) (PermissionDecision, error)
}

// AllowAllHandler approves every permission request.
type AllowAllHandler struct{}

// Check always permits the invocation.
func (AllowAllHandler) Check(context.Context, PermissionRequest) (PermissionDecision, error) {
	return PermissionAllow, nil
}
