package loop

import (
	"time"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

type physicalChildOperationReporter interface {
	ReportsPhysicalChildOperations() bool
}

// measuredToolOperationFacts preserves the legacy one-operation default for
// ordinary tools. Compound tools opt in and are counted exclusively from
// committed, exec.Start-backed child evidence; dispatcher attempts, denied
// calls, preflight failures, and skipped children contribute zero.
func measuredToolOperationFacts(reg *registry.Registry, toolName string, result types.ToolResultBlock, fallbackLatency time.Duration) (int, time.Duration) {
	if reg == nil {
		return 1, fallbackLatency
	}
	reporter, compound := reg.Get(toolName).(physicalChildOperationReporter)
	if !compound || !reporter.ReportsPhysicalChildOperations() {
		return 1, fallbackLatency
	}
	metrics := runtimeevent.ToolEventMetrics{}
	runtimeevent.AttachToolExecutionEvidence(&metrics, result.ToolUseID, result.Data)
	if !metrics.LogicalExecutionCommitted {
		return 0, 0
	}
	var latency time.Duration
	for _, step := range metrics.PhysicalSteps {
		latency += time.Duration(step.DurationMS) * time.Millisecond
	}
	return metrics.PhysicalChildOperations, latency
}
