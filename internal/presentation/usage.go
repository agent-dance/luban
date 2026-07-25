package presentation

import (
	"fmt"
	"strings"

	"github.com/agent-dance/luban/internal/contracts/stream"
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
)

type ContextMeasurement string

const (
	ContextMeasurementUnknown          ContextMeasurement = "unknown"
	ContextMeasurementProviderReported ContextMeasurement = "provider_reported"
	ContextMeasurementLocalEstimate    ContextMeasurement = "local_estimate"
	ContextMeasurementLocalLowerBound  ContextMeasurement = "local_lower_bound"
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
	Revision          uint64     `json:"revision,omitempty"`
	Known             bool       `json:"known"`
	HasCompacted      bool       `json:"has_compacted,omitempty"`
	BaselineKnown     bool       `json:"compaction_baseline_known"`
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
	CostUSD           float64    `json:"cost_usd"` // legacy field name; amount uses CostCurrency
	CostCurrency      string     `json:"cost_currency,omitempty"`
	CostKnown         bool       `json:"cost_known"`
}

// ModelContextProjection reports the active model's complete configured
// context window. Output reservation remains an internal compaction concern.
type ModelContextProjection struct {
	Scope          UsageScope         `json:"scope"`
	Known          bool               `json:"known"`
	UsedTokens     int                `json:"used_tokens,omitempty"`
	CapacityTokens int                `json:"capacity_tokens,omitempty"`
	PercentUsed    int                `json:"percent_used,omitempty"`
	Measurement    ContextMeasurement `json:"measurement,omitempty"`
}

// UsageSemanticsSnapshot is the common visual, screen-reader, and JSON
// projection. Each number belongs to exactly one named scope.
type UsageSemanticsSnapshot struct {
	SchemaVersion     string                 `json:"schema_version"`
	LastRequest       RequestUsageProjection `json:"last_request"`
	CumulativeSession SessionUsageProjection `json:"cumulative_session"`
	ModelContext      ModelContextProjection `json:"model_context"`
}

// StructuredUsageRenderer accepts the scope-safe usage projection.
type StructuredUsageRenderer interface {
	UsageSemantics(UsageSemanticsSnapshot)
}

type ModelContextRenderer interface {
	ModelContext(ModelContextProjection)
}

// CompactionBoundaryIdentity is stable across duplicate deliveries of the
// same successful boundary. It deliberately uses semantic lineage rather than
// delivery order, so a late duplicate cannot open a new accounting segment.
func CompactionBoundaryIdentity(ctx ToolEventContext, boundary stream.CompactBoundaryEvent) string {
	if identity := strings.TrimSpace(boundary.BoundaryID); identity != "" {
		return identity
	}
	trigger := strings.ToLower(strings.TrimSpace(boundary.Trigger))
	return fmt.Sprintf("%s:%d:%d:%s:%s:%s:%d:%d:%d",
		strings.TrimSpace(ctx.SessionID), ctx.SessionEpoch, ctx.ContextGeneration,
		strings.TrimSpace(ctx.TurnID), trigger, strings.TrimSpace(boundary.PreviousTailIdentifier),
		boundary.PreCompactTokenCount, boundary.PostCompactTokenCount, boundary.TruePostCompactTokenCount)
}
