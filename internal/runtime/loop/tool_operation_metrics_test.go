package loop

import (
	"context"
	"testing"
	"time"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

type compoundOperationMetricsTool struct{}

func (*compoundOperationMetricsTool) Name() string        { return "CompoundMetrics" }
func (*compoundOperationMetricsTool) Description() string { return "" }
func (*compoundOperationMetricsTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object", Properties: map[string]any{}, AdditionalProperties: false}
}
func (*compoundOperationMetricsTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}
func (*compoundOperationMetricsTool) ReportsPhysicalChildOperations() bool { return true }

type operationMetricsEvidence struct {
	committed bool
}

func (e operationMetricsEvidence) ToolExecutionEvidence() runtimeevent.ToolExecutionEvidence {
	return runtimeevent.ToolExecutionEvidence{
		LogicalExecutionCommitted: e.committed,
		PhysicalSteps: []runtimeevent.PhysicalToolStepEvidence{
			{Ordinal: 0, StartedOffsetMS: 0, EndedOffsetMS: 4, DurationMS: 4, Outcome: "succeeded"},
			{Ordinal: 2, StartedOffsetMS: 1, EndedOffsetMS: 8, DurationMS: 7, Outcome: "failed"},
		},
	}
}

func TestMeasuredToolOperationFactsCountsCommittedCompoundChildren(t *testing.T) {
	reg := registry.New()
	reg.Register(&compoundOperationMetricsTool{})
	result := types.ToolResultBlock{ToolUseID: "toolu-compound", Data: operationMetricsEvidence{committed: true}}
	operations, latency := measuredToolOperationFacts(reg, "CompoundMetrics", result, 99*time.Second)
	if operations != 2 || latency != 11*time.Millisecond {
		t.Fatalf("compound operation facts = %d/%s", operations, latency)
	}

	result.Data = operationMetricsEvidence{}
	operations, latency = measuredToolOperationFacts(reg, "CompoundMetrics", result, 99*time.Second)
	if operations != 0 || latency != 0 {
		t.Fatalf("uncommitted compound operation facts = %d/%s", operations, latency)
	}
}

func TestMeasuredToolOperationFactsPreservesOrdinaryToolDefault(t *testing.T) {
	operations, latency := measuredToolOperationFacts(nil, "Read", types.ToolResultBlock{}, 13*time.Millisecond)
	if operations != 1 || latency != 13*time.Millisecond {
		t.Fatalf("ordinary operation facts = %d/%s", operations, latency)
	}
}
