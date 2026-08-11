package compact

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/compactproof"
	"github.com/agent-dance/luban/types"
)

func TestProgressiveRunRewriteKeepsProofAndBoundedEvidence(t *testing.T) {
	original := "begin-marker\n" + strings.Repeat("successful test output\n", 600) + "end-marker"
	result := types.ToolResultBlock{
		ToolUseID: "run", Content: original, Outcome: types.ToolOutcomeSucceeded,
		Data: progressiveProofFixture{proof: compactproof.Proof{Run: &compactproof.RunProof{
			LogicalExecutionCommitted: true,
			Steps:                     []compactproof.RunStepProof{{Ordinal: 1, Status: "succeeded", ExitCode: 0, Invoked: true}},
			VerificationStatus:        "passed",
		}}},
	}
	rewrite, ok := progressiveRunRewriteContent(result, original)
	if !ok || len(rewrite) >= len(original) || !strings.Contains(rewrite, progressiveRunRewriteSchema) ||
		!strings.Contains(rewrite, "begin-marker") || !strings.Contains(rewrite, "end-marker") || !strings.Contains(rewrite, compactproof.SchemaVersion) {
		t.Fatalf("run rewrite = ok:%v len:%d/%d %q", ok, len(rewrite), len(original), rewrite)
	}
}

func TestProgressiveRunRewriteFailsClosedForDiagnostics(t *testing.T) {
	base := types.ToolResultBlock{
		ToolUseID: "run", Content: strings.Repeat("diagnostic output\n", 600), Outcome: types.ToolOutcomeSucceeded,
		Data: progressiveProofFixture{proof: compactproof.Proof{Run: &compactproof.RunProof{
			LogicalExecutionCommitted: true,
			Steps:                     []compactproof.RunStepProof{{Ordinal: 1, Status: "succeeded", ExitCode: 0, Invoked: true}},
		}}},
	}
	failed := base
	failed.Outcome = types.ToolOutcomeFailed
	failed.IsError = true
	if _, ok := progressiveRunRewriteContent(failed, failed.Content); ok {
		t.Fatal("failed Run was rewritten")
	}
	mismatch := base
	mismatch.Metadata = map[string]string{"schedule.reason": "revision_mismatch"}
	if _, ok := progressiveRunRewriteContent(mismatch, mismatch.Content); ok {
		t.Fatal("revision-mismatched Run was rewritten")
	}
}
