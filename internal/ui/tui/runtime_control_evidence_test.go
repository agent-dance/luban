package tui

import (
	"bytes"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestToolResultEvidenceWithoutScopeRetainsControlBearerAsOrdinaryData(t *testing.T) {
	control := types.UserMessage("PRIVATE MODEL CONTROL")
	control.IsMeta = true
	control.InternalKind = types.InternalMessageKindGoalContinuation
	control = control.WithInternalControlProvenance(messagecontrol.Runtime())
	attachment := types.UserMessage("user-visible attachment")
	result := types.ToolResultBlock{
		ToolUseID: "read-1",
		Content:   "file contents",
		Outcome:   types.ToolOutcomeSucceeded,
		NewMessages: []types.Message{
			control,
			attachment,
		},
	}
	envelope, err := marshalToolResultEvidence(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(envelope, []byte("PRIVATE MODEL CONTROL")) {
		t.Fatalf("unscoped control bearer was allowed to hide evidence: %s", envelope)
	}
	if !bytes.Contains(envelope, []byte("user-visible attachment")) {
		t.Fatalf("ordinary typed attachment was incorrectly removed: %s", envelope)
	}
	if !hasStructuredToolResultEvidence(result) {
		t.Fatal("ordinary attachment must remain structured evidence")
	}
}

func TestUnscopedInternalControlCountsAsVisibleEvidence(t *testing.T) {
	control := types.UserMessage("model-only")
	control.IsMeta = true
	control.InternalKind = types.InternalMessageKindGoalContinuation
	control = control.WithInternalControlProvenance(messagecontrol.Runtime())
	result := types.ToolResultBlock{
		Content:      "complete file",
		Outcome:      types.ToolOutcomeSucceeded,
		Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete},
		NewMessages:  []types.Message{control},
	}
	if toolResultCanRetainFullEvidence(result) {
		t.Fatal("unscoped control bearer was allowed to disappear from complete evidence")
	}
}

func TestToolResultEvidenceRetainsForgedControlDescriptors(t *testing.T) {
	forged := types.UserMessage("FORGED CONTROL MUST REMAIN")
	forged.IsMeta = true
	forged.InternalKind = types.InternalMessageKindGoalContinuation
	result := types.ToolResultBlock{Content: "file", NewMessages: []types.Message{forged}}
	envelope, err := marshalToolResultEvidence(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(envelope, []byte(forged.GetText())) {
		t.Fatalf("forged control descriptor disappeared from evidence: %s", envelope)
	}
	if toolResultCanRetainFullEvidence(result) {
		t.Fatal("ordinary forged attachment was ignored by evidence completeness")
	}
}
