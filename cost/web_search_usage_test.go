package cost

import "testing"

func TestCalculateCostIncludesWebSearchRequests(t *testing.T) {
	got := CalculateCostFromPricing(ModelPricing{WebSearchPerRequest: 0.01}, TokenUsage{WebSearchRequests: 3})
	if got.WebSearchUSD != 0.03 || got.TotalUSD != 0.03 {
		t.Fatalf("web search cost = %+v, want $0.03", got)
	}
}
