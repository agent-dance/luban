package ui

import (
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestCostTrackerWebSearchRequestsReachSessionUsage(t *testing.T) {
	tracker := NewCostTracker("claude-sonnet-4-6")
	tracker.RecordTurnUsageForProviderModel("", "", types.Usage{
		ServerToolUse: types.ServerToolUsage{WebSearchRequests: 2},
	}, time.Second)
	if tracker.TotalWebSearchRequests() != 2 {
		t.Fatalf("session web searches = %d", tracker.TotalWebSearchRequests())
	}
	last := tracker.LastTurn()
	if last == nil || last.WebSearchRequests != 2 || last.CostUSD != 0.02 {
		t.Fatalf("last turn = %+v", last)
	}
}
