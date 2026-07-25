package ui

import (
	"math"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/presentation"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// BuildUsageSemanticsSnapshot builds a single accounting projection after a
// turn has been recorded in tracker. A nil tracker remains visibly unknown;
// it is never represented as a known zero-cost session.
func BuildUsageSemanticsSnapshot(last *types.Usage, tracker *CostTracker, usedTokens, capacityTokens int) presentation.UsageSemanticsSnapshot {
	snapshot := presentation.UsageSemanticsSnapshot{
		SchemaVersion:     presentation.UsageSemanticsSchemaVersion,
		LastRequest:       presentation.RequestUsageProjection{Scope: presentation.UsageScopeLastRequest},
		CumulativeSession: presentation.SessionUsageProjection{Scope: presentation.UsageScopeCumulativeSession},
		ModelContext:      presentation.ModelContextProjection{Scope: presentation.UsageScopeModelContext},
	}
	if last != nil {
		snapshot.LastRequest.Known = true
		snapshot.LastRequest.InputTokens = max(last.TotalInputTokens(), 0)
		snapshot.LastRequest.OutputTokens = max(last.OutputTokens, 0)
		snapshot.LastRequest.CacheReadTokens = max(last.CacheReadInputTokens, 0)
		snapshot.LastRequest.CacheCreateTokens = max(last.CacheCreationInputTokens, 0)
		snapshot.LastRequest.CacheHitPercent = usageCacheHitPercent(snapshot.LastRequest.InputTokens, snapshot.LastRequest.CacheReadTokens)
	}
	if tracker != nil {
		snapshot.CumulativeSession = BuildSessionUsageProjection(tracker)
	}
	if capacityTokens > 0 {
		snapshot.ModelContext.Known = true
		snapshot.ModelContext.UsedTokens = max(usedTokens, 0)
		snapshot.ModelContext.CapacityTokens = capacityTokens
		snapshot.ModelContext.PercentUsed = boundedUsagePercent(usedTokens, capacityTokens)
		// ContextUsage is updated from the provider's completed request usage.
		snapshot.ModelContext.Measurement = presentation.ContextMeasurementProviderReported
	}
	return snapshot
}

// BuildSessionUsageProjection aligns the displayed input and cache numerator
// to the current compaction segment while keeping output and cost session-wide.
func BuildSessionUsageProjection(tracker *CostTracker) presentation.SessionUsageProjection {
	projection := presentation.SessionUsageProjection{Scope: presentation.UsageScopeCumulativeSession}
	if tracker == nil {
		return projection
	}
	return BuildSessionUsageProjectionFromSnapshot(tracker.Snapshot())
}

// BuildSessionUsageProjectionFromSnapshot derives every visible statistic
// from one atomic ledger revision.
func BuildSessionUsageProjectionFromSnapshot(snapshot UsageSnapshot) presentation.SessionUsageProjection {
	projection := presentation.SessionUsageProjection{Scope: presentation.UsageScopeCumulativeSession, Revision: snapshot.Revision}
	input := max(snapshot.SessionInput, 0)
	output := max(snapshot.SessionOutput, 0)
	cacheRead := min(max(snapshot.SessionCacheRead, 0), input)
	cacheCreate := min(max(snapshot.SessionCacheCreate, 0), input-cacheRead)
	inputAtCompact := snapshot.InputAtCompact
	cacheAtCompact := snapshot.CacheReadAtCompact
	inputAtCompact = min(max(inputAtCompact, 0), input)
	cacheAtCompact = min(max(cacheAtCompact, 0), cacheRead)
	displayInput := input
	displayCache := cacheRead
	if snapshot.HasCompacted && snapshot.CompactionBaselineKnown {
		displayInput -= inputAtCompact
		displayCache -= cacheAtCompact
	}
	projection.Known = true
	projection.HasCompacted = snapshot.HasCompacted
	projection.BaselineKnown = snapshot.CompactionBaselineKnown
	projection.InputTokens = displayInput
	projection.TotalInputTokens = input
	projection.OutputTokens = output
	projection.CacheReadTokens = displayCache
	projection.TotalCacheRead = cacheRead
	projection.CacheCreateTokens = cacheCreate
	projection.InputAtCompact = inputAtCompact
	projection.CacheAtCompact = cacheAtCompact
	projection.WebSearchRequests = max(snapshot.SessionWebSearchRequests, 0)
	projection.CacheHitKnown = displayInput > 0 && displayCache >= 0 && displayCache <= displayInput
	if projection.CacheHitKnown {
		projection.CacheHitPercent = usageCacheHitPercent(displayInput, displayCache)
	}
	projection.CostUSD = max(snapshot.SessionCost, 0)
	projection.CostCurrency = snapshot.CostCurrency
	projection.CostKnown = snapshot.CostKnown
	return projection
}

// FormatSessionUsage is the shared visual and screen-reader projection for the
// session ledger. Input/cache use the current segment; output/cost use the
// complete session.
func FormatSessionUsage(lang i18n.Language, usage presentation.SessionUsageProjection) string {
	if usage.HasCompacted && usage.BaselineKnown {
		if usage.CacheHitKnown {
			args := []any{fmtK(usage.InputTokens), fmtK(usage.TotalInputTokens), usage.CacheHitPercent, fmtK(usage.OutputTokens)}
			if usage.CostKnown {
				return i18n.Format(lang, i18n.KeyUsageSessionCompacted, appendCostArgs(args, usage)...)
			}
			return i18n.Format(lang, i18n.KeyUsageSessionCompactedUnknownCost, args...)
		}
		args := []any{fmtK(usage.InputTokens), fmtK(usage.TotalInputTokens), fmtK(usage.OutputTokens)}
		if usage.CostKnown {
			return i18n.Format(lang, i18n.KeyUsageSessionCompactedNoCache, appendCostArgs(args, usage)...)
		}
		return i18n.Format(lang, i18n.KeyUsageSessionCompactedNoCacheUnknownCost, args...)
	}
	if usage.CacheHitKnown {
		args := []any{fmtK(usage.InputTokens), usage.CacheHitPercent, fmtK(usage.OutputTokens)}
		if usage.CostKnown {
			return i18n.Format(lang, i18n.KeyUsageSession, appendCostArgs(args, usage)...)
		}
		return i18n.Format(lang, i18n.KeyUsageSessionUnknownCost, args...)
	}
	args := []any{fmtK(usage.InputTokens), fmtK(usage.OutputTokens)}
	if usage.CostKnown {
		return i18n.Format(lang, i18n.KeyUsageSessionNoCache, appendCostArgs(args, usage)...)
	}
	return i18n.Format(lang, i18n.KeyUsageSessionNoCacheUnknownCost, args...)
}

// FormatSessionUsageNarrow shortens labels and precision without changing any
// accounting scope. Cache hit rate stays aligned with the displayed input and
// is omitted only when that rate is unavailable.
func FormatSessionUsageNarrow(lang i18n.Language, usage presentation.SessionUsageProjection) string {
	if usage.HasCompacted && usage.BaselineKnown {
		if usage.CacheHitKnown {
			args := []any{fmtK(usage.InputTokens), fmtK(usage.TotalInputTokens), usage.CacheHitPercent, fmtK(usage.OutputTokens)}
			if usage.CostKnown {
				return i18n.Format(lang, i18n.KeyUsageSessionCompactedNarrow, appendCostArgs(args, usage)...)
			}
			return i18n.Format(lang, i18n.KeyUsageSessionCompactedNarrowUnknownCost, args...)
		}
		args := []any{fmtK(usage.InputTokens), fmtK(usage.TotalInputTokens), fmtK(usage.OutputTokens)}
		if usage.CostKnown {
			return i18n.Format(lang, i18n.KeyUsageSessionCompactedNarrowNoCache, appendCostArgs(args, usage)...)
		}
		return i18n.Format(lang, i18n.KeyUsageSessionCompactedNarrowNoCacheUnknownCost, args...)
	}
	if usage.CacheHitKnown {
		args := []any{fmtK(usage.InputTokens), usage.CacheHitPercent, fmtK(usage.OutputTokens)}
		if usage.CostKnown {
			return i18n.Format(lang, i18n.KeyUsageSessionNarrow, appendCostArgs(args, usage)...)
		}
		return i18n.Format(lang, i18n.KeyUsageSessionNarrowUnknownCost, args...)
	}
	args := []any{fmtK(usage.InputTokens), fmtK(usage.OutputTokens)}
	if usage.CostKnown {
		return i18n.Format(lang, i18n.KeyUsageSessionNarrowNoCache, appendCostArgs(args, usage)...)
	}
	return i18n.Format(lang, i18n.KeyUsageSessionNarrowNoCacheUnknownCost, args...)
}

func appendCostArgs(args []any, usage presentation.SessionUsageProjection) []any {
	currency := usage.CostCurrency
	if currency == "" {
		currency = "USD"
	}
	return append(args, provider.CostCurrencySymbol(currency), usage.CostUSD)
}

func usageCacheHitPercent(inputTokens, cacheReadTokens int) int {
	if inputTokens <= 0 || cacheReadTokens <= 0 || cacheReadTokens > inputTokens {
		return 0
	}
	if cacheReadTokens == inputTokens {
		return 100
	}
	percent := int(math.Round(float64(cacheReadTokens) / float64(inputTokens) * 100))
	if percent >= 100 {
		return 99
	}
	return max(percent, 0)
}

func boundedUsagePercent(usedTokens, capacityTokens int) int {
	if capacityTokens <= 0 {
		return 0
	}
	percent := int(math.Round(float64(max(usedTokens, 0)) / float64(capacityTokens) * 100))
	return min(max(percent, 0), 100)
}
