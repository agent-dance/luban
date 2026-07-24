package tui

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestAppStateWebSearchUsageAccumulatesAndResets(t *testing.T) {
	state := NewAppState()
	state.AccumulateSessionUsage(&types.Usage{ServerToolUse: types.ServerToolUsage{WebSearchRequests: 2}})
	state.AccumulateSessionUsage(&types.Usage{ServerToolUse: types.ServerToolUsage{WebSearchRequests: 1}})
	if got := state.SessionWebSearchRequests.Get(); got != 3 {
		t.Fatalf("web searches = %d, want 3", got)
	}
	state.ResetSessionUsage()
	if got := state.SessionWebSearchRequests.Get(); got != 0 {
		t.Fatalf("web searches after reset = %d", got)
	}
}
