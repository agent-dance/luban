package loop

import (
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/registry"
)

func TestP0ReadEvidenceScopeFollowsVisibleContextEpoch(t *testing.T) {
	query := New(nil, registry.New(), Config{})
	executioncontract.BeginRun(query.executionOwner, "active-run")
	defer executioncontract.EndRun(query.executionOwner, "active-run")
	execution := query.bindToolExecutionContext(
		executioncontract.ToolExecutionContext{SessionID: "session", ActorID: "assistant"},
		"active-run", query.skillLoadedLedgerCapability(nil), 0,
	)
	before, ok := execution.ActiveReadEvidenceScope()
	if !ok || before == "" {
		t.Fatal("active loop-owned execution did not receive an evidence scope")
	}
	query.installVisibleHistory(nil)
	if scope, ok := execution.ActiveReadEvidenceScope(); ok || scope != "" {
		t.Fatalf("compacted-away context retained evidence scope %q", scope)
	}
}

func TestP0ReadEvidenceScopeRejectsForgedActor(t *testing.T) {
	query := New(nil, registry.New(), Config{})
	executioncontract.BeginRun(query.executionOwner, "active-run")
	defer executioncontract.EndRun(query.executionOwner, "active-run")
	execution := query.bindToolExecutionContext(
		executioncontract.ToolExecutionContext{ActorID: "assistant"},
		"active-run", query.skillLoadedLedgerCapability(nil), 0,
	)
	execution.ActorID = "forged"
	if scope, ok := execution.ActiveReadEvidenceScope(); ok || scope != "" {
		t.Fatalf("forged actor received evidence scope %q", scope)
	}
}
