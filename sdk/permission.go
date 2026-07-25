package sdk

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

// PermissionDecision is the result of an SDK permission challenge.
type PermissionDecision int

const (
	PermissionAllow PermissionDecision = iota
	PermissionDeny
	PermissionAllowOnce
)

// PermissionRequest is the SDK-owned authorization request projected by the
// application runtime adapter.
type PermissionRequest struct {
	SessionID          string             `json:"session_id"`
	ExecutionSessionID string             `json:"execution_session_id,omitempty"`
	TurnID             string             `json:"turn_id,omitempty"`
	DecisionID         string             `json:"decision_id,omitempty"`
	ToolUseID          string             `json:"tool_use_id,omitempty"`
	ToolName           string             `json:"tool_name"`
	Input              map[string]any     `json:"input"`
	ActorID            string             `json:"actor_id,omitempty"`
	ActorType          string             `json:"actor_type,omitempty"`
	WorkUnitID         string             `json:"work_unit_id,omitempty"`
	Kind               string             `json:"kind,omitempty"`
	Action             string             `json:"action,omitempty"`
	Target             string             `json:"target,omitempty"`
	Impact             string             `json:"impact,omitempty"`
	RiskReason         string             `json:"risk_reason,omitempty"`
	RuleSource         string             `json:"rule_source,omitempty"`
	ApprovalScope      string             `json:"approval_scope,omitempty"`
	Choices            []string           `json:"choices,omitempty"`
	Body               string             `json:"body,omitempty"`
	ReviewDetails      []string           `json:"review_details,omitempty"`
	PostMode           string             `json:"post_mode,omitempty"`
	Description        string             `json:"description,omitempty"`
	Mode               string             `json:"mode,omitempty"`
	AvoidPrompts       bool               `json:"avoid_prompts,omitempty"`
	Message            string             `json:"message,omitempty"`
	Suggestions        []PermissionUpdate `json:"suggestions,omitempty"`
	BlockedPath        string             `json:"blocked_path,omitempty"`
}

type PermissionRuleValue struct {
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent,omitempty"`
}

type PermissionUpdate struct {
	Type        string                `json:"type"`
	Destination string                `json:"destination"`
	Behavior    string                `json:"behavior,omitempty"`
	Rules       []PermissionRuleValue `json:"rules,omitempty"`
	Directories []string              `json:"directories,omitempty"`
	Mode        string                `json:"mode,omitempty"`
}

// PermissionHandler decides whether an SDK-projected tool call is allowed.
type PermissionHandler interface {
	Check(context.Context, PermissionRequest) (PermissionDecision, error)
}

// allowAllPermissionHandler approves every SDK permission request.
type allowAllPermissionHandler struct{}

func (allowAllPermissionHandler) Check(context.Context, PermissionRequest) (PermissionDecision, error) {
	return PermissionAllow, nil
}

// permissionResult is the resolved outcome of a permission challenge.
type permissionResult struct {
	behavior string // "allow" | "deny"
}

// permissionBridge manages the lifecycle of pending permission challenges.
// The SDKServer embeds one of these to bridge between the query goroutine
// (which calls Check) and the stdin-reading goroutine (which delivers replies).
type permissionBridge struct {
	mu      sync.Mutex
	pending map[string]chan permissionResult // keyed by requestID
	done    chan struct{}
	closed  bool
}

func newPermissionBridge() *permissionBridge {
	return &permissionBridge{
		pending: make(map[string]chan permissionResult),
		done:    make(chan struct{}),
	}
}

// register creates one waiter for requestID. Reusing an in-flight ID is
// rejected so a response can never be delivered to the wrong permission call.
func (b *permissionBridge) register(requestID string) (chan permissionResult, bool) {
	ch := make(chan permissionResult, 1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, false
	}
	if _, exists := b.pending[requestID]; exists {
		return nil, false
	}
	b.pending[requestID] = ch
	return ch, true
}

// deliver sends the result to the waiting Check call and cleans up.
func (b *permissionBridge) deliver(requestID string, result permissionResult) bool {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return false
	}
	ch, ok := b.pending[requestID]
	if ok {
		delete(b.pending, requestID)
	}
	b.mu.Unlock()
	if !ok {
		return false
	}
	ch <- result
	return true
}

// close fails every current and future permission waiter. Waiter channels are
// deliberately not closed: Check selects on done, so no zero-value result can
// ever be mistaken for a valid denial response.
func (b *permissionBridge) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	clear(b.pending)
	close(b.done)
	b.mu.Unlock()
}

func (b *permissionBridge) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// unregister removes only the exact waiter created by this Check call. The
// channel comparison prevents a cancelled request from deleting a newer waiter
// if a custom request-ID generator accidentally reuses an ID.
func (b *permissionBridge) unregister(requestID string, ch chan permissionResult) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	pending, ok := b.pending[requestID]
	if !ok || pending != ch {
		return false
	}
	delete(b.pending, requestID)
	return true
}

// sdkPermissionHandler implements PermissionHandler.
// It serialises the challenge to stdout via sendFn and blocks until the client
// replies or ctx is cancelled.
type sdkPermissionHandler struct {
	bridge      *permissionBridge
	sendFn      func(msg any) error
	newReqID    func() string
	getApproval func() ToolApprovalFunc // optional in-process callback; may be nil
}

// Check sends a can_use_tool challenge and waits for the client's response.
func (h *sdkPermissionHandler) Check(ctx context.Context, req PermissionRequest) (PermissionDecision, error) {
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
		Suggestions:        append([]PermissionUpdate(nil), req.Suggestions...),
		BlockedPath:        req.BlockedPath,
	})
	if err != nil {
		return PermissionDeny, i18n.WrapError(i18n.KeySDKPermissionMarshalRequest, err)
	}

	ch, registered := h.bridge.register(reqID)
	if !registered {
		if h.bridge.isClosed() {
			return PermissionDeny, context.Canceled
		}
		return PermissionDeny, i18n.NewError(i18n.KeySDKPermissionDuplicateRequestID, reqID)
	}

	if err := h.sendFn(SDKControlRequest{
		Type:      "control_request",
		RequestID: reqID,
		Request:   json.RawMessage(inner),
	}); err != nil {
		h.bridge.unregister(reqID, ch)
		return PermissionDeny, i18n.WrapError(i18n.KeySDKPermissionSendRequest, err)
	}

	select {
	case <-ctx.Done():
		h.bridge.unregister(reqID, ch)
		return PermissionDeny, ctx.Err()
	case <-h.bridge.done:
		h.bridge.unregister(reqID, ch)
		return PermissionDeny, context.Canceled
	case result := <-ch:
		if err := ctx.Err(); err != nil {
			return PermissionDeny, err
		}
		if h.bridge.isClosed() {
			return PermissionDeny, context.Canceled
		}
		if result.behavior == "allow" {
			return PermissionAllow, nil
		}
		return PermissionDeny, nil
	}
}
