package cost_test

import (
	"math"
	"testing"

	"github.com/agent-dance/luban/cost"
)

// within asserts that got is within tol of want.
func within(t *testing.T, label string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.8f, want %.8f (tol %.8f)", label, got, want, tol)
	}
}

// ── CalculateCost ────────────────────────────────────────────────────────────

func TestCalculateCost_Claude35Sonnet(t *testing.T) {
	// $3 input / $15 output / $0.30 cache_read / $3.75 cache_creation per Mtok
	usage := cost.TokenUsage{
		InputTokens:              3_000_000,
		OutputTokens:             1_000_000,
		CacheReadInputTokens:     1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	breakdown, ok := cost.CalculateCost("claude-3-5-sonnet-20241022", usage)
	if !ok {
		t.Fatal("expected model to be recognised")
	}
	within(t, "InputUSD", breakdown.InputUSD, 3.00, 1e-9)
	within(t, "OutputUSD", breakdown.OutputUSD, 15.00, 1e-9)
	within(t, "CacheReadUSD", breakdown.CacheReadUSD, 0.30, 1e-9)
	within(t, "CacheCreationUSD", breakdown.CacheCreationUSD, 3.75, 1e-9)
	within(t, "TotalUSD", breakdown.TotalUSD, 22.05, 1e-9)
}

func TestCalculateCost_DeepSeekV4Flash(t *testing.T) {
	usage := cost.TokenUsage{
		InputTokens:              3_000_000,
		OutputTokens:             1_000_000,
		CacheReadInputTokens:     1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	breakdown, ok := cost.CalculateCost("deepseek-v4-flash", usage)
	if !ok {
		t.Fatal("expected deepseek-v4-flash pricing to be recognised")
	}
	within(t, "InputUSD", breakdown.InputUSD, 0.14, 1e-9)
	within(t, "OutputUSD", breakdown.OutputUSD, 0.28, 1e-9)
	within(t, "CacheReadUSD", breakdown.CacheReadUSD, 0.0028, 1e-9)
	within(t, "CacheCreationUSD", breakdown.CacheCreationUSD, 0, 1e-9)
}

func TestCalculateCost_OpenAIGPT55(t *testing.T) {
	usage := cost.TokenUsage{
		InputTokens:          2_000_000,
		OutputTokens:         1_000_000,
		CacheReadInputTokens: 1_000_000,
	}
	breakdown, ok := cost.CalculateCost("gpt-5.5", usage)
	if !ok {
		t.Fatal("expected gpt-5.5 pricing to be recognised")
	}
	within(t, "InputUSD", breakdown.InputUSD, 5.00, 1e-9)
	within(t, "OutputUSD", breakdown.OutputUSD, 30.00, 1e-9)
	within(t, "CacheReadUSD", breakdown.CacheReadUSD, 0.50, 1e-9)
}

func TestCalculateCost_OpenAIGPT56Family(t *testing.T) {
	usage := cost.TokenUsage{
		InputTokens:              3_000_000,
		OutputTokens:             1_000_000,
		CacheReadInputTokens:     1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	tests := []struct {
		model                                string
		input, cacheRead, cacheWrite, output float64
	}{
		{"gpt-5.6", 5.0, 0.5, 6.25, 30.0},
		{"gpt-5.6-sol", 5.0, 0.5, 6.25, 30.0},
		{"gpt-5.6-terra", 2.5, 0.25, 3.125, 15.0},
		{"gpt-5.6-luna", 1.0, 0.1, 1.25, 6.0},
	}
	for _, tt := range tests {
		breakdown, ok := cost.CalculateCost(tt.model, usage)
		if !ok {
			t.Fatalf("expected %s pricing to be recognised", tt.model)
		}
		within(t, tt.model+" InputUSD", breakdown.InputUSD, tt.input, 1e-9)
		within(t, tt.model+" CacheReadUSD", breakdown.CacheReadUSD, tt.cacheRead, 1e-9)
		within(t, tt.model+" CacheCreationUSD", breakdown.CacheCreationUSD, tt.cacheWrite, 1e-9)
		within(t, tt.model+" OutputUSD", breakdown.OutputUSD, tt.output, 1e-9)
	}
}

func TestCalculateCost_Claude3Opus(t *testing.T) {
	// $15 input / $75 output per Mtok
	usage := cost.TokenUsage{
		InputTokens:  500_000,
		OutputTokens: 500_000,
	}
	breakdown, ok := cost.CalculateCost("claude-3-opus-20240229", usage)
	if !ok {
		t.Fatal("expected model to be recognised")
	}
	within(t, "InputUSD", breakdown.InputUSD, 7.50, 1e-9)
	within(t, "OutputUSD", breakdown.OutputUSD, 37.50, 1e-9)
	within(t, "TotalUSD", breakdown.TotalUSD, 45.00, 1e-9)
}

func TestCalculateCost_Claude3Haiku(t *testing.T) {
	// $0.25 input / $1.25 output per Mtok
	usage := cost.TokenUsage{
		InputTokens:  100_000,
		OutputTokens: 100_000,
	}
	breakdown, ok := cost.CalculateCost("claude-3-haiku-20240307", usage)
	if !ok {
		t.Fatal("expected model to be recognised")
	}
	within(t, "InputUSD", breakdown.InputUSD, 0.025, 1e-9)
	within(t, "OutputUSD", breakdown.OutputUSD, 0.125, 1e-9)
	within(t, "TotalUSD", breakdown.TotalUSD, 0.15, 1e-9)
}

func TestCalculateCost_Claude35Haiku(t *testing.T) {
	// $0.80 input / $4.00 output per Mtok
	usage := cost.TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	breakdown, ok := cost.CalculateCost("claude-3-5-haiku-20241022", usage)
	if !ok {
		t.Fatal("expected model to be recognised")
	}
	within(t, "InputUSD", breakdown.InputUSD, 0.80, 1e-9)
	within(t, "OutputUSD", breakdown.OutputUSD, 4.00, 1e-9)
	within(t, "TotalUSD", breakdown.TotalUSD, 4.80, 1e-9)
}

func TestCalculateCost_ClaudeOpus4(t *testing.T) {
	// $15 input / $75 output per Mtok
	usage := cost.TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	breakdown, ok := cost.CalculateCost("claude-opus-4-20250514", usage)
	if !ok {
		t.Fatal("expected model to be recognised")
	}
	within(t, "InputUSD", breakdown.InputUSD, 15.00, 1e-9)
	within(t, "OutputUSD", breakdown.OutputUSD, 75.00, 1e-9)
	within(t, "TotalUSD", breakdown.TotalUSD, 90.00, 1e-9)
}

func TestCalculateCost_ClaudeOpus47(t *testing.T) {
	usage := cost.TokenUsage{
		InputTokens:              3_000_000,
		OutputTokens:             1_000_000,
		CacheReadInputTokens:     1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	breakdown, ok := cost.CalculateCost("claude-opus-4-7", usage)
	if !ok {
		t.Fatal("expected claude-opus-4-7 pricing to be recognised")
	}
	within(t, "InputUSD", breakdown.InputUSD, 5.00, 1e-9)
	within(t, "OutputUSD", breakdown.OutputUSD, 25.00, 1e-9)
	within(t, "CacheReadUSD", breakdown.CacheReadUSD, 0.50, 1e-9)
	within(t, "CacheCreationUSD", breakdown.CacheCreationUSD, 6.25, 1e-9)
}

func TestCalculateCost_ClaudeLatestModels(t *testing.T) {
	usage := cost.TokenUsage{
		InputTokens:              3_000_000,
		OutputTokens:             1_000_000,
		CacheReadInputTokens:     1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	tests := []struct {
		model                                string
		input, cacheRead, cacheWrite, output float64
	}{
		{"claude-fable-5", 10.0, 1.0, 12.5, 50.0},
		{"claude-opus-4-8", 5.0, 0.5, 6.25, 25.0},
		{"claude-sonnet-5", 2.0, 0.2, 2.5, 10.0},
	}
	for _, tt := range tests {
		breakdown, ok := cost.CalculateCost(tt.model, usage)
		if !ok {
			t.Fatalf("expected %s pricing to be recognised", tt.model)
		}
		within(t, tt.model+" InputUSD", breakdown.InputUSD, tt.input, 1e-9)
		within(t, tt.model+" CacheReadUSD", breakdown.CacheReadUSD, tt.cacheRead, 1e-9)
		within(t, tt.model+" CacheCreationUSD", breakdown.CacheCreationUSD, tt.cacheWrite, 1e-9)
		within(t, tt.model+" OutputUSD", breakdown.OutputUSD, tt.output, 1e-9)
	}
}

func TestCalculateCost_ClaudeSonnet4(t *testing.T) {
	// $3 input / $15 output per Mtok
	usage := cost.TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	}
	breakdown, ok := cost.CalculateCost("claude-sonnet-4-20250514", usage)
	if !ok {
		t.Fatal("expected model to be recognised")
	}
	within(t, "InputUSD", breakdown.InputUSD, 3.00, 1e-9)
	within(t, "OutputUSD", breakdown.OutputUSD, 15.00, 1e-9)
	within(t, "TotalUSD", breakdown.TotalUSD, 18.00, 1e-9)
}

func TestCalculateCost_SmallUsage(t *testing.T) {
	// Realistic small query: 1k input, 200 output on Sonnet 3.5
	usage := cost.TokenUsage{
		InputTokens:  1_000,
		OutputTokens: 200,
	}
	breakdown, ok := cost.CalculateCost("claude-3-5-sonnet-20241022", usage)
	if !ok {
		t.Fatal("expected model to be recognised")
	}
	// 1000/1M * $3 = $0.003; 200/1M * $15 = $0.003; total = $0.006
	within(t, "InputUSD", breakdown.InputUSD, 0.003, 1e-12)
	within(t, "OutputUSD", breakdown.OutputUSD, 0.003, 1e-12)
	within(t, "TotalUSD", breakdown.TotalUSD, 0.006, 1e-12)
}

func TestCalculateCost_CacheTokens(t *testing.T) {
	// Cache read is much cheaper than input; cache creation is more expensive
	usage := cost.TokenUsage{
		InputTokens:              2_000_000,
		OutputTokens:             0,
		CacheReadInputTokens:     1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	breakdown, ok := cost.CalculateCost("claude-3-5-sonnet-20241022", usage)
	if !ok {
		t.Fatal("expected model to be recognised")
	}
	within(t, "CacheReadUSD", breakdown.CacheReadUSD, 0.30, 1e-9)
	within(t, "CacheCreationUSD", breakdown.CacheCreationUSD, 3.75, 1e-9)
	within(t, "TotalUSD", breakdown.TotalUSD, 4.05, 1e-9)
}

func TestCalculateCost_UnknownModel(t *testing.T) {
	usage := cost.TokenUsage{InputTokens: 1000, OutputTokens: 200}
	_, ok := cost.CalculateCost("gpt-4o", usage)
	if ok {
		t.Error("expected unknown model to return ok=false")
	}
}

func TestCalculateCost_ZeroUsage(t *testing.T) {
	usage := cost.TokenUsage{}
	breakdown, ok := cost.CalculateCost("claude-3-5-sonnet-20241022", usage)
	if !ok {
		t.Fatal("expected model to be recognised")
	}
	within(t, "TotalUSD", breakdown.TotalUSD, 0.0, 1e-12)
}

func TestCalculateCost_DeepSeekCachedInputIsNotDoubleCharged(t *testing.T) {
	usage := cost.TokenUsage{
		InputTokens:          10_600,
		OutputTokens:         31,
		CacheReadInputTokens: 9_222,
	}
	breakdown, ok := cost.CalculateCost("deepseek-v4-flash", usage)
	if !ok {
		t.Fatal("expected deepseek-v4-flash pricing to be recognised")
	}

	within(t, "InputUSD", breakdown.InputUSD, 0.00019292, 1e-12)
	within(t, "CacheReadUSD", breakdown.CacheReadUSD, 0.0000258216, 1e-12)
	within(t, "OutputUSD", breakdown.OutputUSD, 0.00000868, 1e-12)
	within(t, "TotalUSD", breakdown.TotalUSD, 0.0002274216, 1e-12)
}

// ── LookupPricing (prefix matching) ─────────────────────────────────────────

func TestLookupPricing_ExactAndVersioned(t *testing.T) {
	// Both bare prefix and versioned suffix should match
	models := []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-5-sonnet-20240620",
		"claude-3-5-sonnet",
	}
	for _, m := range models {
		p, ok := cost.LookupPricing(m)
		if !ok {
			t.Errorf("LookupPricing(%q): expected ok=true", m)
			continue
		}
		if p.InputPerMtok != 3.0 {
			t.Errorf("LookupPricing(%q).InputPerMtok = %v, want 3.0", m, p.InputPerMtok)
		}
	}
}

func TestLookupPricing_Haiku45VsHaiku35(t *testing.T) {
	// claude-haiku-4-5 should NOT match claude-haiku-4 prefix if haiku-4-5 entry exists
	p45, ok45 := cost.LookupPricing("claude-haiku-4-5-20250514")
	if !ok45 {
		t.Fatal("expected claude-haiku-4-5 to be recognised")
	}
	if p45.InputPerMtok != 1.00 {
		t.Errorf("claude-haiku-4-5 input: got %v, want 1.00", p45.InputPerMtok)
	}
}
