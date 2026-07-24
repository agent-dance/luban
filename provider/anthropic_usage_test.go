package provider

import "testing"

func TestAnthropicTotalInputTokensIncludesCacheBuckets(t *testing.T) {
	if got := anthropicTotalInputTokens(86, 128, 1920); got != 2134 {
		t.Fatalf("anthropicTotalInputTokens() = %d, want 2134", got)
	}
}
