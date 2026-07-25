package execution

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestRuntimeAuthorityExpiresAndRejectsForgedIdentity(t *testing.T) {
	owner := NewOwner()
	other := NewOwner()
	BeginRun(owner, "run-1")
	exec := Bind(owner, ToolExecutionContext{
		SessionID: "session", SessionProjectDir: "project-dir", ProjectRoot: "/workspace", CWD: "/workspace",
		ActorID: "assistant",
	}, BindSpec{
		RunToken: "run-1",
		Identity: RuntimeOwnerIdentity{
			SessionID: "session", SessionProjectDir: "project-dir", ProjectRoot: "/workspace", CWD: "/workspace",
		},
		SkillProjectGeneration: 7,
		ResolveSkillLedger: func(id string) SkillLoadedLedgerState {
			return SkillLoadedLedgerState{ContextEpoch: 3, LoadedContextEpoch: 3, ContentDigest: id}
		},
		ReadEvidenceOwnerID: "evidence", ReadEvidenceEpoch: 3, ReadEvidenceActorID: "assistant",
		CurrentEvidenceEpoch: func() uint64 { return 3 },
	})

	if !exec.IsRuntimeOwned() || !exec.OwnedBy(owner) || exec.OwnedBy(other) || !exec.RuntimeIdentityMatches() {
		t.Fatalf("active authority mismatch: %#v", exec)
	}
	if _, _, _, _, ok := exec.ActiveRuntimeOwnerIdentity(); !ok {
		t.Fatal("active identity missing")
	}
	if scope, ok := exec.ActiveReadEvidenceScope(); !ok || scope == "" {
		t.Fatalf("read evidence scope = %q, %t", scope, ok)
	}
	if state, ok := exec.ResolveSkillLoadedLedger("skill-id"); !ok || state.ContentDigest != "skill-id" {
		t.Fatalf("skill evidence = %#v, %t", state, ok)
	}
	if generation, ok := exec.SkillProjectGeneration(); !ok || generation != 7 {
		t.Fatalf("skill generation = %d, %t", generation, ok)
	}

	forged := exec
	forged.ProjectRoot = "/forged"
	if forged.RuntimeIdentityMatches() {
		t.Fatal("forged public identity retained authority")
	}

	EndRun(owner, "run-1")
	if !exec.HasRuntimeOwner() || exec.IsRuntimeOwned() || exec.ApprovalEpoch() != "" {
		t.Fatalf("expired authority remained active: %#v", exec)
	}
	if _, ok := exec.ResolveSkillLoadedLedger("skill-id"); ok {
		t.Fatal("expired context resolved skill evidence")
	}
	if _, ok := exec.SkillProjectGeneration(); ok {
		t.Fatal("expired context exposed skill generation")
	}

	BeginRun(owner, "run-2")
	defer EndRun(owner, "run-2")
	if exec.IsRuntimeOwned() {
		t.Fatal("previous run token was accepted by a later run")
	}
}

func TestContextStorageDeepClonesMutableToolData(t *testing.T) {
	original := ToolExecutionContext{
		Messages: []types.Message{{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, Input: map[string]any{"nested": map[string]any{"value": "before"}}},
		}}},
		ToolUse: types.ToolUseBlock{Input: map[string]any{"items": []any{"before"}}},
	}
	ctx := WithToolExecutionContext(context.Background(), original)
	original.ToolUse.Input["items"].([]any)[0] = "mutated"
	first, ok := ToolExecutionContextFromContext(ctx)
	if !ok || first.ToolUse.Input["items"].([]any)[0] != "before" {
		t.Fatalf("stored execution = %#v, %t", first, ok)
	}
	first.ToolUse.Input["items"].([]any)[0] = "returned mutation"
	second, _ := ToolExecutionContextFromContext(ctx)
	if second.ToolUse.Input["items"].([]any)[0] != "before" {
		t.Fatalf("context retained returned mutable data: %#v", second.ToolUse.Input)
	}
}

func TestJSONRoundTripDropsRuntimeAuthority(t *testing.T) {
	owner := NewOwner()
	BeginRun(owner, "run")
	defer EndRun(owner, "run")
	bound := Bind(owner, ToolExecutionContext{SessionID: "session"}, BindSpec{
		RunToken: "run", Identity: RuntimeOwnerIdentity{SessionID: "session"},
	})
	encoded, err := json.Marshal(bound)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ToolExecutionContext
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.HasRuntimeOwner() || decoded.IsRuntimeOwned() {
		t.Fatal("serialized context retained private runtime authority")
	}
}
