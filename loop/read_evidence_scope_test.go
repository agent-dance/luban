package loop

import (
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
)

func TestP0ReadEvidenceScopeFollowsVisibleContextEpoch(t *testing.T) {
	query := New(nil, registry.New(), Config{})
	query.activeRunToken = "active-run"
	execution := ToolExecutionContext{
		SessionID:              "session",
		ActorID:                "assistant",
		loadedSkillLedger:      func(skills.SkillID) SkillLoadedLedgerState { return SkillLoadedLedgerState{} },
		owner:                  query,
		runToken:               "active-run",
		readEvidenceOwnerID:    query.readEvidenceOwnerID,
		readEvidenceEpoch:      query.currentReadEvidenceEpoch(),
		ownedReadEvidenceActor: "assistant",
	}
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
	query.activeRunToken = "active-run"
	execution := ToolExecutionContext{
		ActorID:                "forged",
		loadedSkillLedger:      func(skills.SkillID) SkillLoadedLedgerState { return SkillLoadedLedgerState{} },
		owner:                  query,
		runToken:               "active-run",
		readEvidenceOwnerID:    query.readEvidenceOwnerID,
		readEvidenceEpoch:      query.currentReadEvidenceEpoch(),
		ownedReadEvidenceActor: "assistant",
	}
	if scope, ok := execution.ActiveReadEvidenceScope(); ok || scope != "" {
		t.Fatalf("forged actor received evidence scope %q", scope)
	}
}
