package ui

import (
	"math"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const UsageSemanticsSchemaVersion = "usage-semantics/v2"

// UsageScope is a non-overlapping accounting scope. Keeping the scopes typed
// prevents a renderer from placing a last-request token count next to a
// cumulative cache rate or cost without making the mismatch visible in code.
type UsageScope string

const (
	UsageScopeLastRequest       UsageScope = "last_request"
	UsageScopeCumulativeSession UsageScope = "cumulative_session"
	UsageScopeModelContext      UsageScope = "model_context"
	// UsageScopeEffectiveModelContext is retained as a source-compatible alias.
	// Its value now names the complete configured model context window.
	UsageScopeEffectiveModelContext = UsageScopeModelContext
)

type ContextMeasurement string

const (
	ContextMeasurementUnknown          ContextMeasurement = "unknown"
	ContextMeasurementLocalEstimate    ContextMeasurement = "local_estimate"
	ContextMeasurementProviderReported ContextMeasurement = "provider_reported"
)

// RequestUsageProjection describes one completed provider request.
type RequestUsageProjection struct {
	Scope             UsageScope `json:"scope"`
	Known             bool       `json:"known"`
	InputTokens       int        `json:"input_tokens,omitempty"`
	OutputTokens      int        `json:"output_tokens,omitempty"`
	CacheReadTokens   int        `json:"cache_read_tokens,omitempty"`
	CacheCreateTokens int        `json:"cache_create_tokens,omitempty"`
	CacheHitPercent   int        `json:"cache_hit_percent,omitempty"`
}

// SessionUsageProjection is the billing ledger accumulated across the whole
// session, including auxiliary model calls. CostKnown distinguishes a known
// zero from missing pricing information.
type SessionUsageProjection struct {
	Scope             UsageScope `json:"scope"`
	Known             bool       `json:"known"`
	HasCompacted      bool       `json:"has_compacted,omitempty"`
	InputTokens       int        `json:"input_tokens,omitempty"`
	TotalInputTokens  int        `json:"total_input_tokens,omitempty"`
	OutputTokens      int        `json:"output_tokens,omitempty"`
	CacheReadTokens   int        `json:"cache_read_tokens,omitempty"`
	TotalCacheRead    int        `json:"total_cache_read_tokens,omitempty"`
	CacheCreateTokens int        `json:"cache_create_tokens,omitempty"`
	CacheHitPercent   int        `json:"cache_hit_percent,omitempty"`
	CacheHitKnown     bool       `json:"cache_hit_known"`
	InputAtCompact    int        `json:"input_tokens_at_compact,omitempty"`
	CacheAtCompact    int        `json:"cache_read_at_compact,omitempty"`
	WebSearchRequests int        `json:"web_search_requests,omitempty"`
	CostUSD           float64    `json:"cost_usd"`
	CostKnown         bool       `json:"cost_known"`
}

// EffectiveContextProjection is retained as the public type name for source
// compatibility. CapacityTokens is the active model's complete configured
// context window; output reservation remains an internal compaction concern.
type EffectiveContextProjection struct {
	Scope            UsageScope         `json:"scope"`
	Known            bool               `json:"known"`
	UsedTokens       int                `json:"used_tokens,omitempty"`
	CapacityTokens   int                `json:"capacity_tokens,omitempty"`
	PercentUsed      int                `json:"percent_used,omitempty"`
	Measurement      ContextMeasurement `json:"measurement,omitempty"`
	EstimateComplete bool               `json:"estimate_complete"`
	UnknownOverheads []string           `json:"unknown_overheads,omitempty"`
}

// UsageSemanticsSnapshot is the common visual, screen-reader, and JSON
// projection. Each number belongs to exactly one named scope.
type UsageSemanticsSnapshot struct {
	SchemaVersion         string                     `json:"schema_version"`
	LastRequest           RequestUsageProjection     `json:"last_request"`
	CumulativeSession     SessionUsageProjection     `json:"cumulative_session"`
	EffectiveModelContext EffectiveContextProjection `json:"model_context"`
}

// StructuredUsageRenderer accepts the scope-safe usage projection.
type StructuredUsageRenderer interface {
	UsageSemantics(UsageSemanticsSnapshot)
}

type EffectiveContextRenderer interface {
	EffectiveContext(EffectiveContextProjection)
}

// BuildUsageSemanticsSnapshot builds a single accounting projection after a
// turn has been recorded in tracker. A nil tracker remains visibly unknown;
// it is never represented as a known zero-cost session.
func BuildUsageSemanticsSnapshot(last *types.Usage, tracker *CostTracker, usedTokens, capacityTokens int) UsageSemanticsSnapshot {
	snapshot := UsageSemanticsSnapshot{
		SchemaVersion:         UsageSemanticsSchemaVersion,
		LastRequest:           RequestUsageProjection{Scope: UsageScopeLastRequest},
		CumulativeSession:     SessionUsageProjection{Scope: UsageScopeCumulativeSession},
		EffectiveModelContext: EffectiveContextProjection{Scope: UsageScopeModelContext},
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
		snapshot.EffectiveModelContext.Known = true
		snapshot.EffectiveModelContext.UsedTokens = max(usedTokens, 0)
		snapshot.EffectiveModelContext.CapacityTokens = capacityTokens
		snapshot.EffectiveModelContext.PercentUsed = boundedUsagePercent(usedTokens, capacityTokens)
		// ContextUsage is updated from the provider's completed request usage.
		snapshot.EffectiveModelContext.Measurement = ContextMeasurementProviderReported
		snapshot.EffectiveModelContext.EstimateComplete = true
	}
	return snapshot
}

// BuildSessionUsageProjection aligns the displayed input and cache numerator
// to the current compaction segment while keeping output and cost session-wide.
func BuildSessionUsageProjection(tracker *CostTracker) SessionUsageProjection {
	projection := SessionUsageProjection{Scope: UsageScopeCumulativeSession}
	if tracker == nil {
		return projection
	}
	input, output, cacheRead, cacheCreate := tracker.TotalUsage()
	hasCompacted, inputAtCompact, cacheAtCompact := tracker.CompactionBaseline()
	input = max(input, 0)
	output = max(output, 0)
	cacheRead = max(cacheRead, 0)
	cacheCreate = max(cacheCreate, 0)
	inputAtCompact = min(max(inputAtCompact, 0), input)
	cacheAtCompact = min(max(cacheAtCompact, 0), cacheRead)
	displayInput := input
	displayCache := cacheRead
	if hasCompacted {
		displayInput -= inputAtCompact
		displayCache -= cacheAtCompact
	}
	projection.Known = true
	projection.HasCompacted = hasCompacted
	projection.InputTokens = displayInput
	projection.TotalInputTokens = input
	projection.OutputTokens = output
	projection.CacheReadTokens = displayCache
	projection.TotalCacheRead = cacheRead
	projection.CacheCreateTokens = cacheCreate
	projection.InputAtCompact = inputAtCompact
	projection.CacheAtCompact = cacheAtCompact
	projection.WebSearchRequests = tracker.TotalWebSearchRequests()
	projection.CacheHitKnown = displayInput > 0 && displayCache >= 0 && displayCache <= displayInput
	if projection.CacheHitKnown {
		projection.CacheHitPercent = usageCacheHitPercent(displayInput, displayCache)
	}
	projection.CostUSD = tracker.TotalCost()
	projection.CostKnown = tracker.CostKnown()
	return projection
}

// FormatSessionUsage is the shared visual and screen-reader projection for the
// session ledger. Input/cache use the current segment; output/cost use the
// complete session.
func FormatSessionUsage(lang i18n.Language, usage SessionUsageProjection) string {
	if usage.HasCompacted {
		if usage.CacheHitKnown {
			args := []any{fmtK(usage.InputTokens), fmtK(usage.TotalInputTokens), usage.CacheHitPercent, fmtK(usage.OutputTokens)}
			if usage.CostKnown {
				return i18n.Format(lang, i18n.KeyUsageSessionCompacted, append(args, usage.CostUSD)...)
			}
			return i18n.Format(lang, i18n.KeyUsageSessionCompactedUnknownCost, args...)
		}
		args := []any{fmtK(usage.InputTokens), fmtK(usage.TotalInputTokens), fmtK(usage.OutputTokens)}
		if usage.CostKnown {
			return i18n.Format(lang, i18n.KeyUsageSessionCompactedNoCache, append(args, usage.CostUSD)...)
		}
		return i18n.Format(lang, i18n.KeyUsageSessionCompactedNoCacheUnknownCost, args...)
	}
	if usage.CacheHitKnown {
		args := []any{fmtK(usage.InputTokens), usage.CacheHitPercent, fmtK(usage.OutputTokens)}
		if usage.CostKnown {
			return i18n.Format(lang, i18n.KeyUsageSession, append(args, usage.CostUSD)...)
		}
		return i18n.Format(lang, i18n.KeyUsageSessionUnknownCost, args...)
	}
	args := []any{fmtK(usage.InputTokens), fmtK(usage.OutputTokens)}
	if usage.CostKnown {
		return i18n.Format(lang, i18n.KeyUsageSessionNoCache, append(args, usage.CostUSD)...)
	}
	return i18n.Format(lang, i18n.KeyUsageSessionNoCacheUnknownCost, args...)
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
