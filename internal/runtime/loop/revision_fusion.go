package loop

import (
	"context"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// revisionBarrierSkip is local typed scheduler evidence. It remains outside
// provider serialization while distinguishing a dependency skip from a tool
// process that actually ran and failed.
type revisionBarrierSkip struct {
	Status            string `json:"status"`
	Reason            string `json:"reason"`
	UpstreamToolUseID string `json:"upstream_tool_use_id"`
}

func isAdjacentRevisionFusion(reg *registry.Registry, mutation, verification types.ToolUseBlock) bool {
	if reg == nil || mutation.Name != "ApplyPatch" || verification.Name != "Run" {
		return false
	}
	producer, producerOK := reg.Get(mutation.Name).(workspacerevision.MutationTool)
	consumer, consumerOK := reg.Get(verification.Name).(workspacerevision.VerificationTool)
	dependent, dependencyOK := reg.Get(verification.Name).(workspacerevision.PatchCommitDependentTool)
	if !producerOK || !consumerOK || !dependencyOK || !producer.ProvidesWorkspaceRevisionBarrier() ||
		!consumer.ConsumesWorkspaceRevisionBarrier() {
		return false
	}
	// An immediately adjacent ApplyPatch is the only scheduler-authored source
	// of revision authority in this assistant response, so bind it by default.
	// Omission outside this adjacency creates no scheduler dependency. The model
	// can set requires_patch_commit=false for a deliberately independent Run;
	// explicit true documents the fail-closed dependency contract.
	if _, explicit := verification.Input["requires_patch_commit"]; !explicit {
		return true
	}
	return dependent.RequiresPatchCommit(verification.Input)
}

func revisionReceiptFromResult(result types.ToolResultBlock) (workspacerevision.Receipt, bool) {
	if result.IsError || result.Outcome != types.ToolOutcomeSucceeded {
		return workspacerevision.Receipt{}, false
	}
	committed, ok := result.Data.(workspacerevision.MutationResult)
	if !ok {
		return workspacerevision.Receipt{}, false
	}
	return committed.WorkspaceRevisionReceipt()
}

func workspaceRevisionReceiptFromData(result types.ToolResultBlock) (workspacerevision.Receipt, bool) {
	committed, ok := result.Data.(workspacerevision.MutationResult)
	if !ok {
		return workspacerevision.Receipt{}, false
	}
	return committed.WorkspaceRevisionReceipt()
}

func revisionBarrierExecutionContext(ctx context.Context, upstream types.ToolResultBlock, verification types.ToolUseBlock) (context.Context, *types.ToolResultBlock) {
	receipt, ok := revisionReceiptFromResult(upstream)
	if ok {
		return workspacerevision.WithReceipt(ctx, receipt), nil
	}
	result := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: verification.ID,
		Content:   i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolRunSkippedAfterPatch),
		Data: revisionBarrierSkip{
			Status: "skipped", Reason: "upstream_mutation_uncommitted", UpstreamToolUseID: upstream.ToolUseID,
		},
		Metadata: map[string]string{
			"schedule.status": "skipped", "schedule.reason": "upstream_mutation_uncommitted",
		},
		IsError:      true,
		Outcome:      types.ToolOutcomeFailed,
		Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete},
	}
	return ctx, &result
}

func isRevisionMismatchResult(result types.ToolResultBlock) bool {
	return result.Metadata["verification.status"] == "revision_mismatch"
}
