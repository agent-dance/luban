package approvalcommit

import (
	"context"

	"github.com/agent-dance/luban/types"
)

type contextKey struct{}

// Pending is an opaque, process-local approval capability. This package lives
// under internal/ so external Registry consumers cannot manufacture the commit
// context used by the trusted dispatch loop.
type Pending struct {
	Token      string
	Binding    types.ToolPermissionBinding
	PolicyCode string
}

// WithPending carries a registry-issued, not-yet-consumed capability from the
// permission phase to the registry execution phase.
func WithPending(ctx context.Context, pending Pending) context.Context {
	return context.WithValue(ctx, contextKey{}, pending)
}

// FromContext returns the pending capability, if any.
func FromContext(ctx context.Context) (Pending, bool) {
	pending, ok := ctx.Value(contextKey{}).(Pending)
	return pending, ok && pending.Token != ""
}
