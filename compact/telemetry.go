package compact

import (
	"context"
	"fmt"

	"github.com/agent-dance/luban/types"
)

type CompactionTelemetryKind string

const (
	CompactionTelemetryStart       CompactionTelemetryKind = "compact_start"
	CompactionTelemetrySuccess     CompactionTelemetryKind = "compact_success"
	CompactionTelemetryFailure     CompactionTelemetryKind = "compact_failure"
	CompactionTelemetryPTLRetry    CompactionTelemetryKind = "compact_ptl_retry"
	CompactionTelemetryAutoAttempt CompactionTelemetryKind = "auto_compact_attempt"
	CompactionTelemetryAutoSuccess CompactionTelemetryKind = "auto_compact_success"
	CompactionTelemetryAutoFailure CompactionTelemetryKind = "auto_compact_failure"
)

// CompactionUsageMetrics remains as a compatibility alias for callers that
// referenced the old telemetry-only shape. Compaction telemetry now carries
// the complete provider-normalized usage so billing consumers do not lose
// server-tool charges or newly added usage buckets.
type CompactionUsageMetrics = types.Usage

type CompactionTelemetryEvent struct {
	Kind                       CompactionTelemetryKind
	Trigger                    string
	PreCompactTokenCount       int
	PostCompactTokenCount      int
	TruePostCompactTokenCount  int
	AutoCompactThreshold       int
	PostCompactWouldRetrigger  bool
	OriginalMessageCount       int
	CompactedMessageCount      int
	CompactionUsage            *types.Usage
	PTLAttempt                 int
	PTLDroppedMessages         int
	PTLRemainingMessages       int
	ConsecutiveFailureCount    int
	MaxConsecutiveFailureCount int
	ErrorType                  string
}

func UsageMetricsFromUsage(usage *types.Usage) *types.Usage {
	if usage == nil {
		return nil
	}
	copied := *usage
	return &copied
}

// mergeCompactUsageSnapshot combines partial stream updates for one provider
// request. Providers commonly report input/cache fields at message_start and
// output fields at message_delta; a later non-zero value supersedes an earlier
// value for the same bucket.
func mergeCompactUsageSnapshot(dst **types.Usage, src *types.Usage) {
	if src == nil {
		return
	}
	if *dst == nil {
		*dst = &types.Usage{}
	}
	if src.InputTokens != 0 {
		(*dst).InputTokens = src.InputTokens
	}
	if src.OutputTokens != 0 {
		(*dst).OutputTokens = src.OutputTokens
	}
	if src.CacheCreationInputTokens != 0 {
		(*dst).CacheCreationInputTokens = src.CacheCreationInputTokens
	}
	if src.CacheReadInputTokens != 0 {
		(*dst).CacheReadInputTokens = src.CacheReadInputTokens
	}
	if src.ServerToolUse.WebSearchRequests != 0 {
		(*dst).ServerToolUse.WebSearchRequests = src.ServerToolUse.WebSearchRequests
	}
	if src.ServerToolUse.WebFetchRequests != 0 {
		(*dst).ServerToolUse.WebFetchRequests = src.ServerToolUse.WebFetchRequests
	}
}

// addCompactUsage accumulates independently billed provider requests, such as
// a prompt-too-long request followed by a successful truncated retry.
func addCompactUsage(dst **types.Usage, src *types.Usage) {
	if src == nil {
		return
	}
	if *dst == nil {
		*dst = &types.Usage{}
	}
	(*dst).InputTokens += src.InputTokens
	(*dst).OutputTokens += src.OutputTokens
	(*dst).CacheCreationInputTokens += src.CacheCreationInputTokens
	(*dst).CacheReadInputTokens += src.CacheReadInputTokens
	(*dst).ServerToolUse.WebSearchRequests += src.ServerToolUse.WebSearchRequests
	(*dst).ServerToolUse.WebFetchRequests += src.ServerToolUse.WebFetchRequests
}

func isPostCompactLikelyToRetrigger(result *CompactionResult, threshold int) bool {
	return result != nil && threshold > 0 && result.TruePostCompactTokenCount >= threshold
}

type compactUsageRecorderKey struct{}

type compactUsageRecorder func(*types.Usage)

func withCompactUsageRecorder(ctx context.Context, recorder compactUsageRecorder) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, compactUsageRecorderKey{}, recorder)
}

func recordCompactUsage(ctx context.Context, usage *types.Usage) {
	if ctx == nil || usage == nil {
		return
	}
	recorder, ok := ctx.Value(compactUsageRecorderKey{}).(compactUsageRecorder)
	if !ok || recorder == nil {
		return
	}
	copied := *usage
	recorder(&copied)
}

func withCompactAttemptUsage(ctx context.Context) (context.Context, func()) {
	parent := ctx
	var usage *types.Usage
	attemptCtx := withCompactUsageRecorder(parent, func(update *types.Usage) {
		mergeCompactUsageSnapshot(&usage, update)
	})
	return attemptCtx, func() {
		recordCompactUsage(parent, usage)
	}
}

func errCompactTelemetryType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
