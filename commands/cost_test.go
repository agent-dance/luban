package commands

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/cost"
	"github.com/agent-dance/luban/provider"
)

func TestCostCmdDoesNotTreatCatalogCNYPricesAsUSD(t *testing.T) {
	var output string
	ctx := &Context{
		OnEvent:           func(event string) { output += event },
		CurrentProvider:   "kimi",
		CurrentModel:      "kimi-k2.6",
		ProviderRegistry:  provider.DefaultRegistry(),
		TotalInputTokens:  1_000_000,
		TotalOutputTokens: 1_000_000,
	}

	if err := (&costCmd{}).Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "$") {
		t.Fatalf("CNY catalog price was displayed as USD: %q", output)
	}
	if !strings.Contains(output, "cost unknown") {
		t.Fatalf("expected token-only fallback for a model without USD pricing, got %q", output)
	}
}

func TestCostCmdUsesRecordedSessionBreakdownInsteadOfRepricingAsCurrentModel(t *testing.T) {
	var output string
	recorded := cost.CostBreakdown{
		InputUSD: 1, OutputUSD: 2, CacheReadUSD: 0.10, CacheCreationUSD: 0.30, TotalUSD: 3.40,
	}
	ctx := &Context{
		OnEvent:                  func(event string) { output += event },
		CurrentProvider:          "openai",
		CurrentModel:             "gpt-5.5",
		TotalInputTokens:         1_000_000,
		TotalOutputTokens:        1_000_000,
		TotalCacheReadTokens:     200_000,
		TotalCacheCreationTokens: 100_000,
		TotalCostUSD:             3.40,
		SessionCostBreakdown:     &recorded,
	}

	if err := (&costCmd{}).Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Session cost: $3.40", "Input:          1,000,000 tokens  ($1.00)", "Output:           1,000,000 tokens  ($2.00)", "Cache read:     200,000 tokens  ($0.10)", "Cache creation: 100,000 tokens  ($0.30)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("recorded session breakdown missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "$30.00") || strings.Contains(output, "$5.00") {
		t.Fatalf("session was repriced as current gpt-5.5 model:\n%s", output)
	}
}
