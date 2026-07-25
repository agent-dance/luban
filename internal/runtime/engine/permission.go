package engine

import (
	"context"
	"sync"

	"github.com/agent-dance/luban/internal/contracts/permission"
)

// permissionHandlerRef is the live authority shared by parent and child query
// loops. Updating its target changes subsequent checks atomically.
type permissionHandlerRef struct {
	mu      sync.RWMutex
	handler permission.PermissionHandler
}

func newPermissionHandlerRef(handler permission.PermissionHandler) *permissionHandlerRef {
	return &permissionHandlerRef{handler: handler}
}

func (r *permissionHandlerRef) Set(handler permission.PermissionHandler) {
	r.mu.Lock()
	r.handler = handler
	r.mu.Unlock()
}

func (r *permissionHandlerRef) Check(ctx context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	r.mu.RLock()
	handler := r.handler
	r.mu.RUnlock()
	if handler == nil {
		return permission.PermissionAllow, nil
	}
	return handler.Check(ctx, req)
}
