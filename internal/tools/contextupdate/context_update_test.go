package contextupdate

import (
	"context"
	"testing"

	contextcontract "github.com/agent-dance/luban/internal/contracts/contextupdate"
)

func TestContextUpdateToolProducesShadowDecision(t *testing.T) {
	tool := New()
	result, err := tool.Execute(context.Background(), map[string]any{
		"target_index": 0, "target_tool": "functions.Inspect", "action": "REWRITE", "reason_code": "source_consumed", "confidence": .8,
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute = %+v, %v", result, err)
	}
	provider, ok := result.Data.(contextcontract.Provider)
	if !ok {
		t.Fatalf("result data = %T", result.Data)
	}
	decision := provider.ContextUpdateDecision()
	if decision.Schema != contextcontract.SchemaVersion || decision.TargetIndex != 0 || decision.TargetTool != "Inspect" || decision.Action != contextcontract.ActionRewrite {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestContextUpdateToolRejectsInvalidTargetIndex(t *testing.T) {
	for _, index := range []any{-1, .5} {
		result, err := New().Execute(context.Background(), map[string]any{
			"target_index": index, "target_tool": "Inspect", "action": "REWRITE", "reason_code": "source_consumed", "confidence": 1,
		})
		if err != nil || !result.IsError {
			t.Fatalf("invalid target index %v = %+v, %v", index, result, err)
		}
	}
}

func TestContextUpdateToolRejectsFreeFormReasonCode(t *testing.T) {
	result, err := New().Execute(context.Background(), map[string]any{
		"target_index": 0, "target_tool": "Inspect", "action": "KEEP", "reason_code": "still needed because this is prose", "confidence": 1,
	})
	if err != nil || !result.IsError {
		t.Fatalf("free-form reason code = %+v, %v", result, err)
	}
}

func TestContextUpdateToolRejectsFreeFormTargetTool(t *testing.T) {
	result, err := New().Execute(context.Background(), map[string]any{
		"target_index": 0, "target_tool": "Inspect result please", "action": "KEEP", "reason_code": "still_needed", "confidence": 1,
	})
	if err != nil || !result.IsError {
		t.Fatalf("free-form target tool = %+v, %v", result, err)
	}
}
