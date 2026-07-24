package types

import "testing"

func TestUsageTotalInputTokensIncludesAllPromptBuckets(t *testing.T) {
	usage := Usage{
		InputTokens:              2134,
		CacheReadInputTokens:     1920,
		CacheCreationInputTokens: 128,
	}

	if got := usage.TotalInputTokens(); got != 2134 {
		t.Fatalf("TotalInputTokens() = %d, want 2134", got)
	}
	if got := usage.UncachedInputTokens(); got != 86 {
		t.Fatalf("UncachedInputTokens() = %d, want 86", got)
	}
}
