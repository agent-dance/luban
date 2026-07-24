package ui

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/cost"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// TurnUsage holds per-turn token and cost information.
type TurnUsage struct {
	InputTokens, OutputTokens int
	CacheRead, CacheMake      int
	WebSearchRequests         int
	CostUSD                   float64
	Duration                  time.Duration
	Model                     string // model used for this turn
}

// ConversationUsage tracks the request that defines the visible context for
// the current conversation segment and the final requests of segments closed
// by successful compactions. Auxiliary provider calls never update this view.
type ConversationUsage struct {
	Known                 bool
	CompactionCount       int
	CompletedInputTokens  int
	CompletedOutputTokens int
	LastInputTokens       int
	LastOutputTokens      int
	LastCacheReadTokens   int
	LastCacheMakeTokens   int
}

// CostTracker accumulates per-turn and session-wide token usage and USD cost.
// It supports multi-model sessions: each turn records which model was used,
// and cost calculation prefers the ModelCatalog (supporting all providers)
// with fallback to the static USD pricing table.
type CostTracker struct {
	model          string
	provider       string
	catalog        *provider.ModelCatalog // optional; nil = use static pricing
	mu             sync.RWMutex
	turns          []TurnUsage
	totalInput     int
	totalOutput    int
	totalCacheRead int
	totalCacheMake int
	totalWebSearch int
	totalCostUSD   float64
	hasCompacted   bool
	inputAtCompact int
	cacheAtCompact int
	totalBreakdown cost.CostBreakdown
	// breakdownComplete is false after restoring a legacy aggregate cost,
	// because that persisted value has no per-bucket history.
	breakdownComplete bool
	costKnown         bool
	conversationUsage ConversationUsage
	// perModel tracks cumulative cost per model for /cost grouped output.
	perModel map[string]*modelCostAccum
}

// modelCostAccum accumulates per-model costs.
type modelCostAccum struct {
	InputTokens       int
	OutputTokens      int
	WebSearchRequests int
	CostUSD           float64
	TurnCount         int
}

// NewCostTracker creates a CostTracker for the given model name.
func NewCostTracker(model string) *CostTracker {
	return &CostTracker{
		model:             model,
		perModel:          make(map[string]*modelCostAccum),
		breakdownComplete: true,
		costKnown:         true,
		conversationUsage: ConversationUsage{Known: true},
	}
}

// SetCatalog attaches a ModelCatalog for multi-provider pricing.
// When set, RecordTurn uses catalog pricing instead of the static table.
func (ct *CostTracker) SetCatalog(c *provider.ModelCatalog) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.catalog = c
}

// SetModel updates the current model (used for subsequent RecordTurn calls).
func (ct *CostTracker) SetModel(model string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.model = model
}

// SetProvider updates the provider used to resolve catalog pricing for
// subsequent usage. Supplying the provider separately is necessary for model
// IDs that are shared across providers or contain a slash themselves.
func (ct *CostTracker) SetProvider(providerName string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.provider = providerName
}

// SetProviderAndModel atomically updates the provider/model pricing identity.
// It avoids charging an in-flight usage record against a half-updated identity
// while switching providers.
func (ct *CostTracker) SetProviderAndModel(providerName, model string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.provider = providerName
	ct.model = model
}

// Reset clears accumulated turn/session totals while preserving the attached
// catalog. If model is non-empty, it becomes the active model for future turns.
func (ct *CostTracker) Reset(model string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if model != "" {
		ct.model = model
	}
	ct.turns = nil
	ct.totalInput = 0
	ct.totalOutput = 0
	ct.totalCacheRead = 0
	ct.totalCacheMake = 0
	ct.totalWebSearch = 0
	ct.totalCostUSD = 0
	ct.hasCompacted = false
	ct.inputAtCompact = 0
	ct.cacheAtCompact = 0
	ct.totalBreakdown = cost.CostBreakdown{}
	ct.breakdownComplete = true
	ct.costKnown = true
	ct.conversationUsage = ConversationUsage{Known: true}
	ct.perModel = make(map[string]*modelCostAccum)
}

// RestoreSession seeds cumulative totals loaded from durable session metadata.
// Per-turn history is intentionally empty because historical turns are not
// reconstructed, but the next RecordTurn continues from these totals instead
// of overwriting the resumed session with a fresh accumulator.
func (ct *CostTracker) RestoreSession(model string, input, output, cacheRead, cacheMake, webSearch int, totalCost float64, restoredCostKnown ...bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if model != "" {
		ct.model = model
	}
	ct.turns = nil
	ct.totalInput = input
	ct.totalOutput = output
	ct.totalCacheRead = cacheRead
	ct.totalCacheMake = cacheMake
	ct.totalWebSearch = webSearch
	ct.totalCostUSD = totalCost
	ct.hasCompacted = false
	ct.inputAtCompact = 0
	ct.cacheAtCompact = 0
	ct.totalBreakdown = cost.CostBreakdown{}
	hasHistoricalUsage := input != 0 || output != 0 || cacheRead != 0 || cacheMake != 0 || webSearch != 0
	ct.breakdownComplete = !hasHistoricalUsage
	ct.costKnown = totalCost != 0 || !hasHistoricalUsage
	if len(restoredCostKnown) > 0 {
		ct.costKnown = restoredCostKnown[0]
	}
	ct.conversationUsage = ConversationUsage{}
	ct.perModel = make(map[string]*modelCostAccum)
	if ct.model != "" && (input != 0 || output != 0 || webSearch != 0 || totalCost != 0) {
		ct.perModel[ct.model] = &modelCostAccum{
			InputTokens: input, OutputTokens: output, WebSearchRequests: webSearch, CostUSD: totalCost,
		}
	}
}

// RestoreCompactionBaseline restores the usage snapshot captured at the last
// successful context compaction. The baseline is clamped to the restored
// session totals so corrupt or stale metadata cannot produce negative usage.
func (ct *CostTracker) RestoreCompactionBaseline(hasCompacted bool, input, cacheRead int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.hasCompacted = hasCompacted
	if !hasCompacted {
		ct.inputAtCompact = 0
		ct.cacheAtCompact = 0
		return
	}
	ct.inputAtCompact = min(max(input, 0), ct.totalInput)
	ct.cacheAtCompact = min(max(cacheRead, 0), ct.totalCacheRead)
}

// MarkCompaction captures the current session totals after a successful
// compaction. Later callers can subtract this snapshot from TotalUsage to get
// usage accumulated since the most recent compaction boundary.
func (ct *CostTracker) MarkCompaction() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.hasCompacted = true
	ct.inputAtCompact = ct.totalInput
	ct.cacheAtCompact = ct.totalCacheRead
	ct.conversationUsage.CompactionCount++
	ct.conversationUsage.CompletedInputTokens += ct.conversationUsage.LastInputTokens
	ct.conversationUsage.CompletedOutputTokens += ct.conversationUsage.LastOutputTokens
	ct.conversationUsage.LastInputTokens = 0
	ct.conversationUsage.LastOutputTokens = 0
	ct.conversationUsage.LastCacheReadTokens = 0
	ct.conversationUsage.LastCacheMakeTokens = 0
}

// CompactionBaseline returns the usage snapshot at the most recent successful
// compaction boundary.
func (ct *CostTracker) CompactionBaseline() (hasCompacted bool, input, cacheRead int) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.hasCompacted, ct.inputAtCompact, ct.cacheAtCompact
}

// RestoreConversationUsage restores the compact-segment endpoint ledger. The
// full billing ledger is restored separately by RestoreSession.
func (ct *CostTracker) RestoreConversationUsage(usage ConversationUsage) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	usage.CompactionCount = max(usage.CompactionCount, 0)
	usage.CompletedInputTokens = min(max(usage.CompletedInputTokens, 0), ct.totalInput)
	usage.CompletedOutputTokens = min(max(usage.CompletedOutputTokens, 0), ct.totalOutput)
	usage.LastInputTokens = min(max(usage.LastInputTokens, 0), ct.totalInput)
	usage.LastOutputTokens = min(max(usage.LastOutputTokens, 0), ct.totalOutput)
	usage.LastCacheReadTokens = min(max(usage.LastCacheReadTokens, 0), usage.LastInputTokens)
	usage.LastCacheMakeTokens = min(max(usage.LastCacheMakeTokens, 0), usage.LastInputTokens)
	ct.conversationUsage = usage
}

// ConversationUsage returns the current compact-segment endpoint ledger.
func (ct *CostTracker) ConversationUsage() ConversationUsage {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.conversationUsage
}

// RecordTurn records a completed turn's token usage and duration, computing
// the USD cost. Prefers ModelCatalog pricing (multi-provider) if available,
// falling back to cost.CalculateCost (Anthropic-only static table).
func (ct *CostTracker) RecordTurn(inputTok, outputTok, cacheRead, cacheMake int, duration time.Duration) {
	ct.recordUsage(types.Usage{
		InputTokens:              inputTok,
		OutputTokens:             outputTok,
		CacheReadInputTokens:     cacheRead,
		CacheCreationInputTokens: cacheMake,
	}, duration, true)
}

// RecordTurnUsage records every usage bucket, including provider-hosted web
// search requests. Legacy RecordTurn callers retain their token-only behavior.
func (ct *CostTracker) RecordTurnUsage(usage types.Usage, duration time.Duration) {
	ct.recordUsage(usage, duration, true)
}

// RecordTurnUsageForProvider records a turn against an explicit provider while
// retaining the tracker's current model.
func (ct *CostTracker) RecordTurnUsageForProvider(providerName string, usage types.Usage, duration time.Duration) {
	ct.recordUsageForProviderModel(providerName, "", usage, duration, true)
}

// RecordTurnUsageForProviderModel records a completed turn against the exact
// provider/model identity reported by the request. This is used when an
// automatic fallback serves the turn with a model other than the active one.
func (ct *CostTracker) RecordTurnUsageForProviderModel(providerName, model string, usage types.Usage, duration time.Duration) {
	ct.recordUsageForProviderModel(providerName, model, usage, duration, true)
}

// RecordAuxiliaryUsage adds non-turn model calls, such as goal evaluation, to
// session totals without changing LastTurn or TurnCount.
func (ct *CostTracker) RecordAuxiliaryUsage(usage types.Usage) {
	ct.recordUsage(usage, 0, false)
}

// RecordAuxiliaryUsageForModel adds non-turn usage under an explicit model
// without changing the model used by subsequent conversation turns.
func (ct *CostTracker) RecordAuxiliaryUsageForModel(model string, usage types.Usage) {
	ct.recordUsageForModel(model, usage, 0, false)
}

// RecordAuxiliaryUsageForProviderModel records non-turn usage against an
// explicit provider/model pair without changing the active conversation pair.
func (ct *CostTracker) RecordAuxiliaryUsageForProviderModel(providerName, model string, usage types.Usage) {
	ct.recordUsageForProviderModel(providerName, model, usage, 0, false)
}

func (ct *CostTracker) recordUsage(usage types.Usage, duration time.Duration, recordTurn bool) {
	ct.recordUsageForModel("", usage, duration, recordTurn)
}

func (ct *CostTracker) recordUsageForModel(modelOverride string, usage types.Usage, duration time.Duration, recordTurn bool) {
	ct.recordUsageForProviderModel("", modelOverride, usage, duration, recordTurn)
}

func (ct *CostTracker) recordUsageForProviderModel(providerOverride, modelOverride string, usage types.Usage, duration time.Duration, recordTurn bool) {
	costUsage := cost.TokenUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		WebSearchRequests:        usage.ServerToolUse.WebSearchRequests,
	}

	ct.mu.Lock()
	defer ct.mu.Unlock()

	var breakdown cost.CostBreakdown
	pricingKnown := false
	model := modelOverride
	if model == "" {
		model = ct.model
	}
	providerName := providerOverride
	if providerName == "" {
		providerName = ct.provider
	}

	// Try catalog pricing first (covers all providers)
	if ct.catalog != nil {
		if info, ok := resolveCatalogPricing(ct.catalog, providerName, model); ok && info.BillingCurrency() == "USD" && (info.CostPer1MIn > 0 || info.CostPer1MOut > 0) {
			pricing := pricingWithCacheFallback(cost.ModelPricing{
				InputPerMtok:         info.CostPer1MIn,
				OutputPerMtok:        info.CostPer1MOut,
				CacheReadPerMtok:     info.CacheReadPer1M,
				CacheCreationPerMtok: info.CacheCreatePer1M,
				WebSearchPerRequest:  webSearchRequestPriceUSD(providerName, model),
			})
			breakdown = cost.CalculateCostFromPricing(pricing, costUsage)
			pricingKnown = true
		} else {
			// Catalog doesn't have USD pricing — fall back to static USD table.
			breakdown, pricingKnown = calculateStaticCost(model, costUsage)
		}
	} else {
		// No catalog — use static table
		breakdown, pricingKnown = calculateStaticCost(model, costUsage)
	}
	if !pricingKnown && (usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.CacheReadInputTokens != 0 || usage.CacheCreationInputTokens != 0 || usage.ServerToolUse.WebSearchRequests != 0) {
		ct.costKnown = false
	}

	if recordTurn {
		ct.turns = append(ct.turns, TurnUsage{
			InputTokens:       usage.InputTokens,
			OutputTokens:      usage.OutputTokens,
			CacheRead:         usage.CacheReadInputTokens,
			CacheMake:         usage.CacheCreationInputTokens,
			WebSearchRequests: usage.ServerToolUse.WebSearchRequests,
			CostUSD:           breakdown.TotalUSD,
			Duration:          duration,
			Model:             model,
		})
		// A fresh uncompressed session becomes exact as soon as it has a
		// conversation request. Legacy compacted sessions stay unknown because
		// their pre-upgrade segment endpoints cannot be reconstructed.
		if !ct.hasCompacted && ct.conversationUsage.CompactionCount == 0 {
			ct.conversationUsage.Known = true
		}
		ct.conversationUsage.LastInputTokens = usage.TotalInputTokens()
		ct.conversationUsage.LastOutputTokens = max(usage.OutputTokens, 0)
		ct.conversationUsage.LastCacheReadTokens = max(usage.CacheReadInputTokens, 0)
		ct.conversationUsage.LastCacheMakeTokens = max(usage.CacheCreationInputTokens, 0)
	}

	ct.totalInput += usage.InputTokens
	ct.totalOutput += usage.OutputTokens
	ct.totalCacheRead += usage.CacheReadInputTokens
	ct.totalCacheMake += usage.CacheCreationInputTokens
	ct.totalWebSearch += usage.ServerToolUse.WebSearchRequests
	ct.totalCostUSD += breakdown.TotalUSD
	ct.totalBreakdown.InputUSD += breakdown.InputUSD
	ct.totalBreakdown.OutputUSD += breakdown.OutputUSD
	ct.totalBreakdown.CacheReadUSD += breakdown.CacheReadUSD
	ct.totalBreakdown.CacheCreationUSD += breakdown.CacheCreationUSD
	ct.totalBreakdown.WebSearchUSD += breakdown.WebSearchUSD
	ct.totalBreakdown.TotalUSD += breakdown.TotalUSD

	// Accumulate per-model stats
	accum := ct.perModel[model]
	if accum == nil {
		accum = &modelCostAccum{}
		ct.perModel[model] = accum
	}
	accum.InputTokens += usage.InputTokens
	accum.OutputTokens += usage.OutputTokens
	accum.WebSearchRequests += usage.ServerToolUse.WebSearchRequests
	accum.CostUSD += breakdown.TotalUSD
	if recordTurn {
		accum.TurnCount++
	}
}

func resolveCatalogPricing(catalog *provider.ModelCatalog, providerName, model string) (provider.ModelInfo, bool) {
	if providerName == "" {
		return catalog.Resolve(model)
	}
	if info, ok := catalog.ResolveForProvider(providerName, model); ok {
		return info, true
	}
	// Also accept an explicitly qualified provider/model value. Do not split a
	// raw model ID at an arbitrary slash: IDs such as openai/gpt-oss-120b are
	// valid Groq model IDs and must reach ResolveForProvider intact first.
	if qualifiedPrefix := providerName + "/"; strings.HasPrefix(model, qualifiedPrefix) {
		return catalog.ResolveForProvider(providerName, strings.TrimPrefix(model, qualifiedPrefix))
	}
	return provider.ModelInfo{}, false
}

func calculateStaticCost(model string, usage cost.TokenUsage) (cost.CostBreakdown, bool) {
	pricing, ok := cost.LookupPricing(model)
	if !ok {
		return cost.CostBreakdown{}, false
	}
	return cost.CalculateCostFromPricing(pricingWithCacheFallback(pricing), usage), true
}

func pricingWithCacheFallback(pricing cost.ModelPricing) cost.ModelPricing {
	if pricing.CacheReadPerMtok <= 0 {
		pricing.CacheReadPerMtok = pricing.InputPerMtok
	}
	if pricing.CacheCreationPerMtok <= 0 {
		pricing.CacheCreationPerMtok = pricing.InputPerMtok
	}
	return pricing
}

func webSearchRequestPriceUSD(providerName, model string) float64 {
	switch provider.CanonicalProviderName(providerName) {
	case "anthropic", "bedrock", "vertex", "oauth":
		return cost.WebSearchRequestPriceUSD
	}
	if providerName == "" && strings.HasPrefix(strings.ToLower(model), "claude") {
		return cost.WebSearchRequestPriceUSD
	}
	return 0
}

func (ct *CostTracker) TotalWebSearchRequests() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalWebSearch
}

// TotalCost returns the cumulative USD cost across all recorded turns.
func (ct *CostTracker) TotalCost() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalCostUSD
}

// CostKnown reports whether every non-zero usage record had a supported USD
// pricing rule. It prevents an unsupported provider from looking free.
func (ct *CostTracker) CostKnown() bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.costKnown
}

// TotalCostBreakdown returns the exact sum of recorded bucket costs. complete
// is false when the tracker was restored from legacy metadata that persisted
// only the aggregate total.
func (ct *CostTracker) TotalCostBreakdown() (breakdown cost.CostBreakdown, complete bool) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalBreakdown, ct.breakdownComplete && ct.costKnown
}

// TurnCount returns the number of recorded turns.
func (ct *CostTracker) TurnCount() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return len(ct.turns)
}

// LastTurn returns a copy of the most recently recorded turn, or nil if none.
func (ct *CostTracker) LastTurn() *TurnUsage {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if len(ct.turns) == 0 {
		return nil
	}
	t := ct.turns[len(ct.turns)-1]
	return &t
}

// TotalTokens returns the cumulative input and output token counts.
func (ct *CostTracker) TotalTokens() (input, output int) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalInput, ct.totalOutput
}

// TotalCacheTokens returns the cumulative cache read and cache creation token counts.
func (ct *CostTracker) TotalCacheTokens() (cacheRead, cacheMake int) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalCacheRead, ct.totalCacheMake
}

// TotalUsage returns the cumulative usage buckets tracked for the session.
func (ct *CostTracker) TotalUsage() (input, output, cacheRead, cacheMake int) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalInput, ct.totalOutput, ct.totalCacheRead, ct.totalCacheMake
}

// Model returns the current model name.
func (ct *CostTracker) Model() string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.model
}

// Provider returns the provider used for subsequent catalog pricing lookups.
func (ct *CostTracker) Provider() string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.provider
}

// ModelCostEntry represents per-model cost summary for display.
type ModelCostEntry struct {
	Model             string
	InputTokens       int
	OutputTokens      int
	WebSearchRequests int
	CostUSD           float64
	TurnCount         int
}

// PerModelCosts returns a snapshot of per-model cost accumulations, sorted by
// model name for deterministic display order. Useful for /cost command to show
// grouped costs.
func (ct *CostTracker) PerModelCosts() []ModelCostEntry {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	result := make([]ModelCostEntry, 0, len(ct.perModel))
	for model, accum := range ct.perModel {
		result = append(result, ModelCostEntry{
			Model:             model,
			InputTokens:       accum.InputTokens,
			OutputTokens:      accum.OutputTokens,
			WebSearchRequests: accum.WebSearchRequests,
			CostUSD:           accum.CostUSD,
			TurnCount:         accum.TurnCount,
		})
	}
	// Sort by model name for stable, deterministic output across calls.
	sort.Slice(result, func(i, j int) bool {
		return result[i].Model < result[j].Model
	})
	return result
}
