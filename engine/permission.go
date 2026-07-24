package engine

import (
	"context"
	"sync"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/types"
)

// PermissionDecision is the outcome of a permission check.
type PermissionDecision int

const (
	// PermissionAllow permits the tool call and caches the decision for the session.
	PermissionAllow PermissionDecision = iota
	// PermissionDeny blocks the tool call.
	PermissionDeny
	// PermissionAllowOnce permits this single invocation without caching.
	PermissionAllowOnce
)

// PermissionRequest describes a tool call that requires authorisation.
type PermissionRequest struct {
	SessionID          string                    `json:"session_id"`
	ExecutionSessionID string                    `json:"execution_session_id,omitempty"`
	TurnID             string                    `json:"turn_id,omitempty"`
	DecisionID         string                    `json:"decision_id,omitempty"`
	ToolUseID          string                    `json:"tool_use_id,omitempty"`
	ToolName           string                    `json:"tool_name"`
	Input              map[string]any            `json:"input"`
	ActorID            string                    `json:"actor_id,omitempty"`
	ActorType          string                    `json:"actor_type,omitempty"`
	WorkUnitID         string                    `json:"work_unit_id,omitempty"`
	Kind               string                    `json:"kind,omitempty"`
	Action             string                    `json:"action,omitempty"`
	Target             string                    `json:"target,omitempty"`
	Impact             string                    `json:"impact,omitempty"`
	RiskReason         string                    `json:"risk_reason,omitempty"`
	RuleSource         string                    `json:"rule_source,omitempty"`
	ApprovalScope      string                    `json:"approval_scope,omitempty"`
	Choices            []string                  `json:"choices,omitempty"`
	Body               string                    `json:"body,omitempty"`
	ReviewDetails      []string                  `json:"review_details,omitempty"`
	PostMode           string                    `json:"post_mode,omitempty"`
	Description        string                    `json:"description"`
	Mode               string                    `json:"mode,omitempty"`
	AvoidPrompts       bool                      `json:"avoid_prompts,omitempty"`
	Required           bool                      `json:"required,omitempty"`
	Sandboxed          bool                      `json:"sandboxed,omitempty"`
	SandboxCapability  string                    `json:"sandbox_capability,omitempty"`
	Message            string                    `json:"message,omitempty"`
	Suggestions        []types.PermissionUpdate  `json:"suggestions,omitempty"`
	BlockedPath        string                    `json:"blocked_path,omitempty"`
	PolicyDecision     *types.PolicyDecision     `json:"policy_decision,omitempty"`
	PermissionSnapshot *types.ToolRuntimeContext `json:"permission_snapshot,omitempty"`
}

// PermissionHandler decides whether a tool call should be allowed.
//
// Trust boundary: custom implementations are trusted embedders. When
// req.Required is true, an Allow response normally attests that the embedder
// obtained an explicit decision for this exact invocation. First-party
// handlers make one deliberate exception: the user-selected automatic mode
// may consume PolicyRequiredAsk without prompting. Other Required contracts
// remain non-cacheable and interactive.
type PermissionHandler interface {
	Check(ctx context.Context, req PermissionRequest) (PermissionDecision, error)
}

// permissionHandlerRef is the shared authority used by parent and child query
// loops. Updating its target changes subsequent checks everywhere atomically,
// including conversations that already exist and subagents spawned later.
type permissionHandlerRef struct {
	mu      sync.RWMutex
	handler PermissionHandler
}

func newPermissionHandlerRef(handler PermissionHandler) *permissionHandlerRef {
	return &permissionHandlerRef{handler: handler}
}

func (r *permissionHandlerRef) Set(handler PermissionHandler) {
	r.mu.Lock()
	r.handler = handler
	r.mu.Unlock()
}

func (r *permissionHandlerRef) Check(ctx context.Context, req PermissionRequest) (PermissionDecision, error) {
	r.mu.RLock()
	handler := r.handler
	r.mu.RUnlock()
	if handler == nil {
		return PermissionAllow, nil
	}
	return handler.Check(ctx, req)
}

// AllowAllHandler is a PermissionHandler that unconditionally allows every call.
type AllowAllHandler struct{}

// Check always returns PermissionAllow.
func (AllowAllHandler) Check(_ context.Context, _ PermissionRequest) (PermissionDecision, error) {
	return PermissionAllow, nil
}

// loopPermissionAdapter bridges engine.PermissionHandler → loop.PermissionHandler.
// loop/ cannot import engine/ (import cycle), so this adapter lives in engine/ and is
// passed to loop.Config.PermissionHandler when building a QueryLoop.
type loopPermissionAdapter struct {
	handler PermissionHandler
}

// toLoopDecision converts an engine decision to a loop decision.
func toLoopDecision(d PermissionDecision) loop.PermissionDecision {
	switch d {
	case PermissionDeny:
		return loop.PermissionDeny
	case PermissionAllowOnce:
		return loop.PermissionAllowOnce
	default:
		return loop.PermissionAllow
	}
}

// Check implements loop.PermissionHandler by delegating to the engine handler.
func (a *loopPermissionAdapter) Check(ctx context.Context, req loop.PermissionRequest) (loop.PermissionDecision, error) {
	decision, err := a.handler.Check(ctx, enginePermissionRequestFromLoop(req, nil))
	return toLoopDecision(decision), err
}

// CheckWithPermissionSnapshot lets subagent wrappers pass a trusted,
// spawn-time runtime policy through the loop adapter without widening the
// general loop.PermissionRequest surface.
func (a *loopPermissionAdapter) CheckWithPermissionSnapshot(ctx context.Context, req loop.PermissionRequest, snapshot types.ToolRuntimeContext) (loop.PermissionDecision, error) {
	cloned := clonePermissionRuntimeSnapshot(snapshot)
	decision, err := a.handler.Check(ctx, enginePermissionRequestFromLoop(req, &cloned))
	return toLoopDecision(decision), err
}

func enginePermissionRequestFromLoop(req loop.PermissionRequest, snapshot *types.ToolRuntimeContext) PermissionRequest {
	return PermissionRequest{
		SessionID:          req.SessionID,
		ExecutionSessionID: req.ExecutionSessionID,
		TurnID:             req.TurnID,
		DecisionID:         req.DecisionID,
		ToolUseID:          req.ToolUseID,
		ToolName:           req.ToolName,
		Input:              req.Input,
		ActorID:            req.ActorID,
		ActorType:          req.ActorType,
		WorkUnitID:         req.WorkUnitID,
		Kind:               req.Kind,
		Action:             req.Action,
		Target:             req.Target,
		Impact:             req.Impact,
		RiskReason:         req.RiskReason,
		RuleSource:         req.RuleSource,
		ApprovalScope:      req.ApprovalScope,
		Choices:            append([]string(nil), req.Choices...),
		Body:               req.Body,
		ReviewDetails:      append([]string(nil), req.ReviewDetails...),
		PostMode:           req.PostMode,
		Description:        req.Action,
		Mode:               req.Mode,
		AvoidPrompts:       req.AvoidPrompts,
		Required:           req.Required,
		Sandboxed:          req.Sandboxed,
		SandboxCapability:  req.SandboxCapability,
		Message:            req.Message,
		Suggestions:        append([]types.PermissionUpdate(nil), req.Suggestions...),
		BlockedPath:        req.BlockedPath,
		PolicyDecision:     clonePolicyDecision(req.PolicyDecision),
		PermissionSnapshot: snapshot,
	}
}

func clonePolicyDecision(source *types.PolicyDecision) *types.PolicyDecision {
	if source == nil {
		return nil
	}
	cloned := source.Clone()
	return &cloned
}

func clonePermissionRuntimeSnapshot(snapshot types.ToolRuntimeContext) types.ToolRuntimeContext {
	cloned := snapshot
	cloned.AllowedDirs = append([]string(nil), snapshot.AllowedDirs...)
	cloned.Features = cloneBoolMap(snapshot.Features)
	cloned.AllowedTools = cloneBoolMap(snapshot.AllowedTools)
	cloned.DeniedTools = cloneBoolMap(snapshot.DeniedTools)
	cloned.AllowedRules = append([]types.PermissionRuleValue(nil), snapshot.AllowedRules...)
	cloned.DeniedRules = append([]types.PermissionRuleValue(nil), snapshot.DeniedRules...)
	cloned.AskRules = append([]types.PermissionRuleValue(nil), snapshot.AskRules...)
	return cloned
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	if source == nil {
		return nil
	}
	cloned := make(map[string]bool, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

// asLoopPermissionHandler returns a loop.PermissionHandler adapter for h.
// Returns nil when h is nil so that the loop fast-path (nil check) is preserved.
func asLoopPermissionHandler(h PermissionHandler) loop.PermissionHandler {
	if h == nil {
		return nil
	}
	// AllowAllHandler → nil so the loop nil-fast-path applies (zero overhead).
	if _, ok := h.(AllowAllHandler); ok {
		return nil
	}
	return &loopPermissionAdapter{handler: h}
}

// AsLoopPermissionHandler exposes the engine-to-loop permission adapter for
// tools that create child QueryLoops outside CoreEngine.
func AsLoopPermissionHandler(h PermissionHandler) loop.PermissionHandler {
	return asLoopPermissionHandler(h)
}
