package sdk

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// permissionResult is the resolved outcome of a permission challenge.
type permissionResult struct {
	behavior string // "allow" | "deny"
	message  string
}

// pendingPermission is a single in-flight permission challenge.
type pendingPermission struct {
	ch chan permissionResult
}

// permissionBridge manages the lifecycle of pending permission challenges.
// The SDKServer embeds one of these to bridge between the query goroutine
// (which calls Check) and the stdin-reading goroutine (which delivers replies).
type permissionBridge struct {
	mu      sync.Mutex
	pending map[string]*pendingPermission // keyed by requestID
}

func newPermissionBridge() *permissionBridge {
	return &permissionBridge{
		pending: make(map[string]*pendingPermission),
	}
}

// register creates a channel for the given requestID and returns it.
func (b *permissionBridge) register(requestID string) chan permissionResult {
	ch := make(chan permissionResult, 1)
	b.mu.Lock()
	b.pending[requestID] = &pendingPermission{ch: ch}
	b.mu.Unlock()
	return ch
}

// deliver sends the result to the waiting Check call and cleans up.
func (b *permissionBridge) deliver(requestID string, result permissionResult) bool {
	b.mu.Lock()
	pp, ok := b.pending[requestID]
	if ok {
		delete(b.pending, requestID)
	}
	b.mu.Unlock()
	if !ok {
		return false
	}
	pp.ch <- result
	return true
}

// unregister removes only the exact waiter created by this Check call. The
// channel comparison prevents a cancelled request from deleting a newer waiter
// if a custom request-ID generator accidentally reuses an ID.
func (b *permissionBridge) unregister(requestID string, ch chan permissionResult) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	pp, ok := b.pending[requestID]
	if !ok || pp.ch != ch {
		return false
	}
	delete(b.pending, requestID)
	return true
}

// SDKPermissionHandler implements engine.PermissionHandler.
// It serialises the challenge to stdout via sendFn and blocks until the client
// replies or ctx is cancelled.
type SDKPermissionHandler struct {
	bridge      *permissionBridge
	sendFn      func(msg any) error
	newReqID    func() string
	getApproval func() ToolApprovalFunc // optional in-process callback; may be nil
}

// Check sends a can_use_tool challenge and waits for the client's response.
func (h *SDKPermissionHandler) Check(ctx context.Context, req engine.PermissionRequest) (engine.PermissionDecision, error) {
	// Fast path: if an in-process approval callback is registered, ask it first.
	if h.getApproval != nil {
		if fn := h.getApproval(); fn != nil {
			decision := fn(req.ToolName, req.Input)
			if decision != PermissionAbstain {
				return decision, nil
			}
		}
	}

	reqID := h.newReqID()

	// Marshal the challenge.
	inner, err := json.Marshal(PermissionRequestMsg{
		Subtype:            "can_use_tool",
		SessionID:          req.SessionID,
		ExecutionSessionID: req.ExecutionSessionID,
		TurnID:             req.TurnID,
		DecisionID:         req.DecisionID,
		RequestID:          reqID,
		ToolName:           req.ToolName,
		Input:              req.Input,
		ToolUseID:          req.ToolUseID,
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
		Description:        req.Description,
		Mode:               req.Mode,
		AvoidPrompts:       req.AvoidPrompts,
		Message:            req.Message,
		Suggestions:        append([]types.PermissionUpdate(nil), req.Suggestions...),
		BlockedPath:        req.BlockedPath,
	})
	if err != nil {
		return engine.PermissionDeny, i18n.WrapError(i18n.KeySDKPermissionMarshalRequest, err)
	}

	ch := h.bridge.register(reqID)

	if err := h.sendFn(SDKControlRequestOut{
		Type:      "control_request",
		RequestID: reqID,
		Request:   json.RawMessage(inner),
	}); err != nil {
		h.bridge.unregister(reqID, ch)
		return engine.PermissionDeny, i18n.WrapError(i18n.KeySDKPermissionSendRequest, err)
	}

	select {
	case <-ctx.Done():
		h.bridge.unregister(reqID, ch)
		return engine.PermissionDeny, ctx.Err()
	case result := <-ch:
		if result.behavior == "allow" {
			return engine.PermissionAllow, nil
		}
		return engine.PermissionDeny, nil
	}
}
