package sdk

import "github.com/agent-dance/luban/engine"

// ToolApprovalFunc is an optional callback that can be registered with an
// SDKServer to decide whether a tool call should be allowed.  It is called
// synchronously inside Check() before the normal bridge round-trip.
//
// Return values:
//
//	engine.PermissionAllow  – approve the tool call immediately
//	engine.PermissionDeny   – deny the tool call immediately
//	-1                      – abstain; fall through to the SDK client bridge
//
// The special abstain value (-1) lets the callback indicate "I don't have an
// opinion" so the normal can_use_tool round-trip still happens.
type ToolApprovalFunc func(toolName string, input map[string]any) engine.PermissionDecision

// PermissionAbstain is the sentinel returned by a ToolApprovalFunc to
// indicate that the callback did not make a decision and the default SDK
// bridge should be used instead.
const PermissionAbstain engine.PermissionDecision = -1

// SetToolApproval registers fn as the in-process tool-approval callback.
// Once registered, every permission Check calls fn first; only when fn returns
// PermissionAbstain does the server fall through to the client bridge.
// Pass nil to unregister.
func (s *SDKServer) SetToolApproval(fn ToolApprovalFunc) {
	s.toolApprovalMu.Lock()
	s.toolApproval = fn
	s.toolApprovalMu.Unlock()
}

// getToolApproval returns the current ToolApprovalFunc (may be nil).
func (s *SDKServer) getToolApproval() ToolApprovalFunc {
	s.toolApprovalMu.RLock()
	fn := s.toolApproval
	s.toolApprovalMu.RUnlock()
	return fn
}
