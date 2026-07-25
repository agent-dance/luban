package ui

import (
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
	CostUSD                   float64 // legacy field name; amount uses CostCurrency
	CostCurrency              string
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

// UsageSnapshot is the single read-side authority for session accounting.
// Every field is copied while CostTracker's read lock is held, so renderers
// cannot combine totals, cost, and compaction baselines from different
// accounting revisions.
type UsageSnapshot struct {
	Revision                 uint64
	SessionInput             int
	SessionOutput            int
	SessionCacheRead         int
	SessionCacheCreate       int
	SessionWebSearchRequests int
	SessionCost              float64
	CostCurrency             string
	CostKnown                bool
	HasCompacted             bool
	CompactionBaselineKnown  bool
	InputAtCompact           int
	CacheReadAtCompact       int
	Conversation             ConversationUsage
}

// CostTracker accumulates per-turn and session-wide token usage and cost in
// the billing currency declared by the model catalog.
// It supports multi-model sessions: each turn records which model was used,
// and cost calculation prefers the ModelCatalog (supporting all providers)
// with fallback to the static USD pricing table. Costs in different currencies
// are never added together; such a session is marked as having unknown cost.
type CostTracker struct {
	model             string
	provider          string
	catalog           *provider.ModelCatalog // optional; nil = use static pricing
	mu                sync.RWMutex
	lastTurn          *TurnUsage
	totalInput        int
	totalOutput       int
	totalCacheRead    int
	totalCacheMake    int
	totalWebSearch    int
	totalCostUSD      float64
	costCurrency      string
	hasCompacted      bool
	baselineKnown     bool
	inputAtCompact    int
	cacheAtCompact    int
	costKnown         bool
	conversationUsage ConversationUsage
	accountedUsageIDs map[string]struct{}
	compactionIDs     map[string]struct{}
	revision          uint64
}

// NewCostTracker creates a CostTracker for the given model name.
func NewCostTracker(model string) *CostTracker {
	return &CostTracker{
		model:             model,
		costKnown:         true,
		conversationUsage: ConversationUsage{Known: true},
		accountedUsageIDs: make(map[string]struct{}),
		compactionIDs:     make(map[string]struct{}),
	}
}

// SetCatalog attaches a ModelCatalog for multi-provider pricing.
// When set, RecordTurn uses catalog pricing instead of the static table.
func (ct *CostTracker) SetCatalog(c *provider.ModelCatalog) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.catalog = c
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
// catalog. If model is non-empty, it becomes the active model for future usage.
func (ct *CostTracker) Reset(model string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if model != "" {
		ct.model = model
	}
	ct.lastTurn = nil
	ct.totalInput = 0
	ct.totalOutput = 0
	ct.totalCacheRead = 0
	ct.totalCacheMake = 0
	ct.totalWebSearch = 0
	ct.totalCostUSD = 0
	ct.costCurrency = ""
	ct.hasCompacted = false
	ct.baselineKnown = false
	ct.inputAtCompact = 0
	ct.cacheAtCompact = 0
	ct.costKnown = true
	ct.conversationUsage = ConversationUsage{Known: true}
	ct.accountedUsageIDs = make(map[string]struct{})
	ct.compactionIDs = make(map[string]struct{})
	ct.revision++
}

// RestoreSession seeds cumulative totals loaded from durable session metadata.
// Per-turn history is intentionally empty because historical turns are not
// reconstructed, but the next recorded turn continues from these totals
// instead of overwriting the resumed session with a fresh accumulator.
func (ct *CostTracker) RestoreSession(model string, input, output, cacheRead, cacheMake, webSearch int, totalCost float64, restoredCostKnown bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if model != "" {
		ct.model = model
	}
	ct.lastTurn = nil
	ct.totalInput = max(input, 0)
	ct.totalOutput = max(output, 0)
	ct.totalCacheRead = min(max(cacheRead, 0), ct.totalInput)
	ct.totalCacheMake = min(max(cacheMake, 0), ct.totalInput-ct.totalCacheRead)
	ct.totalWebSearch = max(webSearch, 0)
	ct.totalCostUSD = max(totalCost, 0)
	ct.costCurrency = ct.billingCurrencyLocked(ct.provider, ct.model)
	if ct.costCurrency == "" && ct.totalCostUSD > 0 {
		// Session metadata written before currency support stored USD amounts.
		ct.costCurrency = "USD"
	}
	ct.hasCompacted = false
	ct.baselineKnown = false
	ct.inputAtCompact = 0
	ct.cacheAtCompact = 0
	ct.costKnown = restoredCostKnown
	ct.conversationUsage = ConversationUsage{}
	ct.accountedUsageIDs = make(map[string]struct{})
	ct.compactionIDs = make(map[string]struct{})
	ct.revision++
}

// RestoreCompactionBaseline restores the usage snapshot captured at the last
// successful context compaction. The baseline is clamped to the restored
// session totals so corrupt or stale metadata cannot produce negative usage.
func (ct *CostTracker) RestoreCompactionBaseline(hasCompacted bool, input, cacheRead int) {
	ct.RestoreCompactionBaselineState(hasCompacted, hasCompacted, input, cacheRead)
}

// RestoreCompactionBaselineState preserves whether a compacted session's
// baseline was actually persisted. Legacy sessions may say HasCompacted while
// carrying zero-valued fields that were absent in the old schema; those
// sessions must remain session-total-only until the next successful compact.
func (ct *CostTracker) RestoreCompactionBaselineState(hasCompacted, baselineKnown bool, input, cacheRead int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	defer func() { ct.revision++ }()
	ct.hasCompacted = hasCompacted
	ct.baselineKnown = hasCompacted && baselineKnown
	if !hasCompacted {
		ct.inputAtCompact = 0
		ct.cacheAtCompact = 0
		return
	}
	if !ct.baselineKnown {
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
	ct.MarkCompactionBoundary("")
}

// MarkCompactionBoundary advances the baseline exactly once for a stable
// boundary identity. An empty identity is treated as an explicitly unique
// programmatic boundary for compatibility with direct callers.
func (ct *CostTracker) MarkCompactionBoundary(identity string) bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	identity = strings.TrimSpace(identity)
	if identity != "" {
		if _, exists := ct.compactionIDs[identity]; exists {
			return false
		}
		ct.compactionIDs[identity] = struct{}{}
	}
	ct.hasCompacted = true
	ct.baselineKnown = true
	ct.inputAtCompact = ct.totalInput
	ct.cacheAtCompact = ct.totalCacheRead
	ct.conversationUsage.CompactionCount++
	ct.conversationUsage.CompletedInputTokens += ct.conversationUsage.LastInputTokens
	ct.conversationUsage.CompletedOutputTokens += ct.conversationUsage.LastOutputTokens
	ct.conversationUsage.LastInputTokens = 0
	ct.conversationUsage.LastOutputTokens = 0
	ct.conversationUsage.LastCacheReadTokens = 0
	ct.conversationUsage.LastCacheMakeTokens = 0
	ct.revision++
	return true
}

// CompactionBaseline returns the usage snapshot at the most recent successful
// compaction boundary.
func (ct *CostTracker) CompactionBaseline() (hasCompacted bool, input, cacheRead int) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.hasCompacted, ct.inputAtCompact, ct.cacheAtCompact
}

// CompactionBaselineState returns both the boundary and its reliability bit.
func (ct *CostTracker) CompactionBaselineState() (hasCompacted, baselineKnown bool, input, cacheRead int) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.hasCompacted, ct.baselineKnown, ct.inputAtCompact, ct.cacheAtCompact
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
	ct.revision++
}

// ConversationUsage returns the current compact-segment endpoint ledger.
func (ct *CostTracker) ConversationUsage() ConversationUsage {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.conversationUsage
}

// RecordTurnUsageForProviderModel records a completed turn against the exact
// provider/model identity reported by the request. This is used when an
// automatic fallback serves the turn with a model other than the active one.
func (ct *CostTracker) RecordTurnUsageForProviderModel(providerName, model string, usage types.Usage, duration time.Duration) {
	ct.recordUsageForProviderModel("", providerName, model, usage, duration, true)
}

// RecordTurnUsageOnceForProviderModel records one billed provider call unless
// the same stable usage identity was already committed to this session.
func (ct *CostTracker) RecordTurnUsageOnceForProviderModel(usageID, providerName, model string, usage types.Usage, duration time.Duration) bool {
	return ct.recordUsageForProviderModel(usageID, providerName, model, usage, duration, true)
}

// RecordAuxiliaryUsageForProviderModel records non-turn usage against an
// explicit provider/model pair without changing the active conversation pair.
func (ct *CostTracker) RecordAuxiliaryUsageForProviderModel(providerName, model string, usage types.Usage) {
	ct.recordUsageForProviderModel("", providerName, model, usage, 0, false)
}

// RecordAuxiliaryUsageOnceForProviderModel is the exactly-once counterpart for
// retries, fallback attempts, compaction, and session-owned helper calls.
func (ct *CostTracker) RecordAuxiliaryUsageOnceForProviderModel(usageID, providerName, model string, usage types.Usage) bool {
	return ct.recordUsageForProviderModel(usageID, providerName, model, usage, 0, false)
}

func (ct *CostTracker) recordUsageForProviderModel(usageID, providerOverride, modelOverride string, usage types.Usage, duration time.Duration, recordTurn bool) bool {
	usage = normalizedUsage(usage)
	costUsage := cost.TokenUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		WebSearchRequests:        usage.ServerToolUse.WebSearchRequests,
	}

	ct.mu.Lock()
	defer ct.mu.Unlock()
	usageID = strings.TrimSpace(usageID)
	if usageID != "" {
		if _, exists := ct.accountedUsageIDs[usageID]; exists {
			return false
		}
		ct.accountedUsageIDs[usageID] = struct{}{}
	}

	var breakdown cost.CostBreakdown
	pricingKnown := false
	billingCurrency := ""
	model := modelOverride
	if model == "" {
		model = ct.model
	}
	providerName := providerOverride
	if providerName == "" {
		providerName = ct.provider
	}

	// Try catalog pricing first (covers all providers and native currencies).
	if ct.catalog != nil {
		if info, ok := resolveCatalogPricing(ct.catalog, providerName, model); ok {
			billingCurrency = normalizeBillingCurrency(info.BillingCurrency())
			pricing := pricingWithCacheFallback(cost.ModelPricing{
				InputPerMtok:         info.CostPer1MIn,
				OutputPerMtok:        info.CostPer1MOut,
				CacheReadPerMtok:     info.CacheReadPer1M,
				CacheCreationPerMtok: info.CacheCreatePer1M,
				WebSearchPerRequest:  webSearchRequestPrice(providerName, model, billingCurrency),
			})
			breakdown = cost.CalculateCostFromPricing(pricing, costUsage)
			pricingKnown = pricingSupportsUsage(pricing, costUsage)
		} else {
			// The static table is explicitly USD-denominated.
			breakdown, pricingKnown = calculateStaticCost(model, costUsage)
			billingCurrency = "USD"
		}
	} else {
		// No catalog — use the static USD table.
		breakdown, pricingKnown = calculateStaticCost(model, costUsage)
		billingCurrency = "USD"
	}
	hasBillableUsage := usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.CacheReadInputTokens != 0 || usage.CacheCreationInputTokens != 0 || usage.ServerToolUse.WebSearchRequests != 0
	if pricingKnown && hasBillableUsage {
		billingCurrency = normalizeBillingCurrency(billingCurrency)
		switch {
		case ct.costCurrency == "":
			ct.costCurrency = billingCurrency
		case ct.costCurrency != billingCurrency:
			// There is no exchange-rate source in the accounting ledger, so a
			// mixed-currency total would be misleading. Keep the known subtotal
			// and expose the complete session cost as unknown.
			ct.costKnown = false
			pricingKnown = false
			breakdown = cost.CostBreakdown{}
		}
	}
	if !pricingKnown && hasBillableUsage {
		ct.costKnown = false
	}

	if recordTurn {
		turn := TurnUsage{
			InputTokens:       usage.InputTokens,
			OutputTokens:      usage.OutputTokens,
			CacheRead:         usage.CacheReadInputTokens,
			CacheMake:         usage.CacheCreationInputTokens,
			WebSearchRequests: usage.ServerToolUse.WebSearchRequests,
			CostUSD:           breakdown.TotalUSD,
			CostCurrency:      billingCurrency,
			Duration:          duration,
			Model:             model,
		}
		ct.lastTurn = &turn
		// A fresh uncompressed session becomes exact as soon as it has a
		// conversation request. Restored compacted sessions remain governed by
		// their persisted conversation-usage ledger.
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
	ct.revision++
	return true
}

func normalizedUsage(usage types.Usage) types.Usage {
	usage.InputTokens = max(usage.TotalInputTokens(), 0)
	usage.OutputTokens = max(usage.OutputTokens, 0)
	usage.CacheReadInputTokens = min(max(usage.CacheReadInputTokens, 0), usage.InputTokens)
	usage.CacheCreationInputTokens = min(max(usage.CacheCreationInputTokens, 0), usage.InputTokens-usage.CacheReadInputTokens)
	usage.ServerToolUse.WebSearchRequests = max(usage.ServerToolUse.WebSearchRequests, 0)
	usage.ServerToolUse.WebFetchRequests = max(usage.ServerToolUse.WebFetchRequests, 0)
	return usage
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

func normalizeBillingCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return "USD"
	}
	return currency
}

func (ct *CostTracker) billingCurrencyLocked(providerName, model string) string {
	if ct.catalog != nil {
		if info, ok := resolveCatalogPricing(ct.catalog, providerName, model); ok {
			return normalizeBillingCurrency(info.BillingCurrency())
		}
	}
	if _, ok := cost.LookupPricing(model); ok {
		return "USD"
	}
	return ""
}

func calculateStaticCost(model string, usage cost.TokenUsage) (cost.CostBreakdown, bool) {
	pricing, ok := cost.LookupPricing(model)
	if !ok {
		return cost.CostBreakdown{}, false
	}
	pricing = pricingWithCacheFallback(pricing)
	return cost.CalculateCostFromPricing(pricing, usage), pricingSupportsUsage(pricing, usage)
}

func pricingSupportsUsage(pricing cost.ModelPricing, usage cost.TokenUsage) bool {
	if usage.UncachedInputTokens() > 0 && pricing.InputPerMtok <= 0 {
		return false
	}
	if usage.OutputTokens > 0 && pricing.OutputPerMtok <= 0 {
		return false
	}
	if usage.CacheReadInputTokens > 0 && pricing.CacheReadPerMtok <= 0 {
		return false
	}
	if usage.CacheCreationInputTokens > 0 && pricing.CacheCreationPerMtok <= 0 {
		return false
	}
	if usage.WebSearchRequests > 0 && pricing.WebSearchPerRequest <= 0 {
		return false
	}
	return true
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

func webSearchRequestPrice(providerName, model, currency string) float64 {
	if normalizeBillingCurrency(currency) != "USD" {
		return 0
	}
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

// TotalCost returns the cumulative cost in Currency().
func (ct *CostTracker) TotalCost() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.totalCostUSD
}

// Currency returns the ISO 4217 billing currency for TotalCost. An empty value
// means that no priced usage has established a currency yet.
func (ct *CostTracker) Currency() string {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.costCurrency
}

// CostKnown reports whether every non-zero usage record had a supported pricing
// rule in one consistent currency. It prevents unsupported pricing or mixed
// currencies from looking free.
func (ct *CostTracker) CostKnown() bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.costKnown
}

// LastTurn returns a copy of the most recently recorded turn, or nil if none.
func (ct *CostTracker) LastTurn() *TurnUsage {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if ct.lastTurn == nil {
		return nil
	}
	t := *ct.lastTurn
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

// Snapshot returns one internally consistent session ledger revision.
func (ct *CostTracker) Snapshot() UsageSnapshot {
	if ct == nil {
		return UsageSnapshot{}
	}
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return UsageSnapshot{
		Revision:                 ct.revision,
		SessionInput:             ct.totalInput,
		SessionOutput:            ct.totalOutput,
		SessionCacheRead:         ct.totalCacheRead,
		SessionCacheCreate:       ct.totalCacheMake,
		SessionWebSearchRequests: ct.totalWebSearch,
		SessionCost:              ct.totalCostUSD,
		CostCurrency:             ct.costCurrency,
		CostKnown:                ct.costKnown,
		HasCompacted:             ct.hasCompacted,
		CompactionBaselineKnown:  ct.baselineKnown,
		InputAtCompact:           ct.inputAtCompact,
		CacheReadAtCompact:       ct.cacheAtCompact,
		Conversation:             ct.conversationUsage,
	}
}
