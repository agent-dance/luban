package agent

import (
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/types"
)

func cloneRuntimeTaskRunRecords(runs []agentcontract.RunRecord) []agentcontract.RunRecord {
	if len(runs) == 0 {
		return nil
	}
	out := make([]agentcontract.RunRecord, len(runs))
	for index, run := range runs {
		out[index] = run
		out[index].FinishedAt = cloneTimePointer(run.FinishedAt)
		out[index].DurationMs = cloneInt64Pointer(run.DurationMs)
		out[index].TotalTokens = cloneIntPointer(run.TotalTokens)
		out[index].Usage = cloneUsagePointer(run.Usage)
		out[index].ArtifactRefs = append([]string(nil), run.ArtifactRefs...)
		out[index].VerificationRefs = append([]string(nil), run.VerificationRefs...)
		out[index].LatestProgress = cloneAgentProgressEvent(run.LatestProgress)
	}
	return out
}

func cloneAgentProgressEvent(event *agentcontract.ProgressEvent) *agentcontract.ProgressEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	cloned.Usage = cloneUsagePointer(event.Usage)
	cloned.LastRequestUsage = cloneUsagePointer(event.LastRequestUsage)
	return &cloned
}

func cloneUsagePointer(usage *types.Usage) *types.Usage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
