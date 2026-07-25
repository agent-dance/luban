package cost

// CalculateCost computes the USD cost for a single API response given the
// model name and token usage counts.
//
// If the model is not recognised in the pricing table the returned
// CostBreakdown will have all-zero USD fields and the caller should treat
// the cost as unknown.
func CalculateCost(model string, usage TokenUsage) (CostBreakdown, bool) {
	pricing, ok := LookupPricing(model)
	if !ok {
		return CostBreakdown{}, false
	}
	return CalculateCostFromPricing(pricing, usage), true
}

// CalculateCostFromPricing computes the USD cost breakdown using explicit pricing.
// This is the core calculation function used by both CalculateCost (static table)
// and catalog-based callers (ModelCatalog → ModelPricing → CostBreakdown).
func CalculateCostFromPricing(pricing ModelPricing, usage TokenUsage) CostBreakdown {
	const perMillion = 1_000_000.0

	inputUSD := float64(usage.UncachedInputTokens()) / perMillion * pricing.InputPerMtok
	outputUSD := float64(usage.OutputTokens) / perMillion * pricing.OutputPerMtok
	cacheReadUSD := float64(usage.CacheReadInputTokens) / perMillion * pricing.CacheReadPerMtok
	cacheCreationUSD := float64(usage.CacheCreationInputTokens) / perMillion * pricing.CacheCreationPerMtok
	webSearchUSD := float64(usage.WebSearchRequests) * pricing.WebSearchPerRequest

	return CostBreakdown{
		InputUSD:         inputUSD,
		OutputUSD:        outputUSD,
		CacheReadUSD:     cacheReadUSD,
		CacheCreationUSD: cacheCreationUSD,
		WebSearchUSD:     webSearchUSD,
		TotalUSD:         inputUSD + outputUSD + cacheReadUSD + cacheCreationUSD + webSearchUSD,
	}
}
