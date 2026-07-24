package cost

// TokenUsage holds normalized per-turn counts. InputTokens is the total prompt;
// cache fields are details within it.
type TokenUsage struct {
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int
	WebSearchRequests        int
}

func (u TokenUsage) UncachedInputTokens() int {
	uncached := max(u.InputTokens, 0) - max(u.CacheReadInputTokens, 0) - max(u.CacheCreationInputTokens, 0)
	return max(uncached, 0)
}

// CostBreakdown holds the USD cost broken down by token category, plus the total.
type CostBreakdown struct {
	InputUSD         float64
	OutputUSD        float64
	CacheReadUSD     float64
	CacheCreationUSD float64
	WebSearchUSD     float64
	TotalUSD         float64
}
