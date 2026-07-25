package mcp

import (
	"context"
	"testing"
	"time"
)

// TestMCPCallTimeoutPreservesEarlierDeadline ensures we don't extend an
// already-tight deadline supplied by the caller.
func TestMCPCallTimeoutPreservesEarlierDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	ctx, c := withMCPCallTimeout(parent)
	defer c()
	d, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected deadline")
	}
	if time.Until(d) > 2*time.Second {
		t.Fatalf("MCP timeout should not extend caller deadline; got %v remaining", time.Until(d))
	}
}
