package mcp

import (
	"context"
	"time"
)

const defaultMCPCallTimeout = 60 * time.Second

func withMCPCallTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < defaultMCPCallTimeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultMCPCallTimeout)
}
