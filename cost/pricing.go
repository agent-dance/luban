package cost

// WebSearchRequestPriceUSD matches the TS model cost table for supported
// Anthropic models.
const WebSearchRequestPriceUSD = 0.01

// ModelPricing holds per-million-token prices (USD) for a single model.
// All prices are per 1,000,000 tokens.
type ModelPricing struct {
	InputPerMtok         float64
	OutputPerMtok        float64
	CacheReadPerMtok     float64
	CacheCreationPerMtok float64
	WebSearchPerRequest  float64
}

// modelPricingTable maps model name prefixes to their pricing.
// Prefix matching is used: the longest matching prefix wins.
// Prices are sourced from provider public pricing pages and are matched by
// model prefix. Re-check prices before release because provider pricing changes.
//
// To add a new model: append an entry with its canonical name prefix.
var modelPricingTable = []modelPricingEntry{
	// DeepSeek's global API and the provider catalog use these USD rates.
	{prefix: "deepseek-v4-flash", pricing: ModelPricing{
		InputPerMtok:     0.14,
		OutputPerMtok:    0.28,
		CacheReadPerMtok: 0.0028,
	}},
	{prefix: "deepseek-v4-pro", pricing: ModelPricing{
		InputPerMtok:     0.435,
		OutputPerMtok:    0.87,
		CacheReadPerMtok: 0.003625,
	}},

	// ── OpenAI GPT-5 generation ─────────────────────────────────────────────
	// Prices are USD per Mtok from OpenAI API model pages.
	{prefix: "gpt-5.6-terra", pricing: ModelPricing{
		InputPerMtok:         2.50,
		OutputPerMtok:        15.00,
		CacheReadPerMtok:     0.25,
		CacheCreationPerMtok: 3.125,
	}},
	{prefix: "gpt-5.6-luna", pricing: ModelPricing{
		InputPerMtok:         1.00,
		OutputPerMtok:        6.00,
		CacheReadPerMtok:     0.10,
		CacheCreationPerMtok: 1.25,
	}},
	{prefix: "gpt-5.6-sol", pricing: ModelPricing{
		InputPerMtok:         5.00,
		OutputPerMtok:        30.00,
		CacheReadPerMtok:     0.50,
		CacheCreationPerMtok: 6.25,
	}},
	// gpt-5.6 is the official alias for GPT-5.6 Sol.
	{prefix: "gpt-5.6", pricing: ModelPricing{
		InputPerMtok:         5.00,
		OutputPerMtok:        30.00,
		CacheReadPerMtok:     0.50,
		CacheCreationPerMtok: 6.25,
	}},
	{prefix: "gpt-5.5", pricing: ModelPricing{
		InputPerMtok:     5.00,
		OutputPerMtok:    30.00,
		CacheReadPerMtok: 0.50,
	}},
	{prefix: "gpt-5.4-mini", pricing: ModelPricing{
		InputPerMtok:     0.75,
		OutputPerMtok:    4.50,
		CacheReadPerMtok: 0.075,
	}},
	{prefix: "gpt-5.4-nano", pricing: ModelPricing{
		InputPerMtok:     0.20,
		OutputPerMtok:    1.25,
		CacheReadPerMtok: 0.02,
	}},
	{prefix: "gpt-5.4", pricing: ModelPricing{
		InputPerMtok:     2.50,
		OutputPerMtok:    15.00,
		CacheReadPerMtok: 0.25,
	}},
	{prefix: "gpt-5.3-codex", pricing: ModelPricing{
		InputPerMtok:     1.75,
		OutputPerMtok:    14.00,
		CacheReadPerMtok: 0.175,
	}},
	{prefix: "gpt-5.2", pricing: ModelPricing{
		InputPerMtok:     1.75,
		OutputPerMtok:    14.00,
		CacheReadPerMtok: 0.175,
	}},
	{prefix: "gpt-5-mini", pricing: ModelPricing{
		InputPerMtok:     0.25,
		OutputPerMtok:    2.00,
		CacheReadPerMtok: 0.025,
	}},

	// ── Claude Fable / Opus ──────────────────────────────────────────────────
	// Claude Opus 4.5+ is priced at $5 input / $25 output per Mtok.
	{prefix: "claude-fable-5", pricing: ModelPricing{
		InputPerMtok:         10.00,
		OutputPerMtok:        50.00,
		CacheReadPerMtok:     1.00,
		CacheCreationPerMtok: 12.50,
	}},
	{prefix: "claude-opus-4-8", pricing: ModelPricing{
		InputPerMtok:         5.00,
		OutputPerMtok:        25.00,
		CacheReadPerMtok:     0.50,
		CacheCreationPerMtok: 6.25,
	}},
	{prefix: "claude-opus-4-7", pricing: ModelPricing{
		InputPerMtok:         5.00,
		OutputPerMtok:        25.00,
		CacheReadPerMtok:     0.50,
		CacheCreationPerMtok: 6.25,
	}},
	{prefix: "claude-opus-4-6", pricing: ModelPricing{
		InputPerMtok:         5.00,
		OutputPerMtok:        25.00,
		CacheReadPerMtok:     0.50,
		CacheCreationPerMtok: 6.25,
	}},
	{prefix: "claude-opus-4-5", pricing: ModelPricing{
		InputPerMtok:         5.00,
		OutputPerMtok:        25.00,
		CacheReadPerMtok:     0.50,
		CacheCreationPerMtok: 6.25,
	}},
	// $15 input / $75 output per Mtok
	{prefix: "claude-opus-4", pricing: ModelPricing{
		InputPerMtok:         15.00,
		OutputPerMtok:        75.00,
		CacheReadPerMtok:     1.50,
		CacheCreationPerMtok: 18.75,
	}},
	{prefix: "claude-4-opus", pricing: ModelPricing{
		InputPerMtok:         15.00,
		OutputPerMtok:        75.00,
		CacheReadPerMtok:     1.50,
		CacheCreationPerMtok: 18.75,
	}},

	// ── Claude 4 Sonnet ──────────────────────────────────────────────────────
	// $3 input / $15 output per Mtok
	// Claude Sonnet 5 introductory pricing expires after 2026-08-31; update to
	// $3/$15 with $0.30 cache reads and $3.75 cache writes on 2026-09-01.
	{prefix: "claude-sonnet-5", pricing: ModelPricing{
		InputPerMtok:         2.00,
		OutputPerMtok:        10.00,
		CacheReadPerMtok:     0.20,
		CacheCreationPerMtok: 2.50,
	}},
	{prefix: "claude-sonnet-4", pricing: ModelPricing{
		InputPerMtok:         3.00,
		OutputPerMtok:        15.00,
		CacheReadPerMtok:     0.30,
		CacheCreationPerMtok: 3.75,
	}},
	{prefix: "claude-4-sonnet", pricing: ModelPricing{
		InputPerMtok:         3.00,
		OutputPerMtok:        15.00,
		CacheReadPerMtok:     0.30,
		CacheCreationPerMtok: 3.75,
	}},

	// ── Claude 3.7 Sonnet ────────────────────────────────────────────────────
	// $3 input / $15 output per Mtok
	{prefix: "claude-3-7-sonnet", pricing: ModelPricing{
		InputPerMtok:         3.00,
		OutputPerMtok:        15.00,
		CacheReadPerMtok:     0.30,
		CacheCreationPerMtok: 3.75,
	}},

	// ── Claude 3.5 Sonnet ────────────────────────────────────────────────────
	// $3 input / $15 output per Mtok
	{prefix: "claude-3-5-sonnet", pricing: ModelPricing{
		InputPerMtok:         3.00,
		OutputPerMtok:        15.00,
		CacheReadPerMtok:     0.30,
		CacheCreationPerMtok: 3.75,
	}},

	// ── Claude Haiku 4.5 ─────────────────────────────────────────────────────
	// $1 input / $5 output per Mtok
	{prefix: "claude-haiku-4-5", pricing: ModelPricing{
		InputPerMtok:         1.00,
		OutputPerMtok:        5.00,
		CacheReadPerMtok:     0.10,
		CacheCreationPerMtok: 1.25,
	}},
	{prefix: "claude-4-5-haiku", pricing: ModelPricing{
		InputPerMtok:         1.00,
		OutputPerMtok:        5.00,
		CacheReadPerMtok:     0.10,
		CacheCreationPerMtok: 1.25,
	}},

	// ── Claude Haiku 4 ───────────────────────────────────────────────────────
	// $1 input / $5 output per Mtok (same tier as 4.5)
	{prefix: "claude-haiku-4", pricing: ModelPricing{
		InputPerMtok:         1.00,
		OutputPerMtok:        5.00,
		CacheReadPerMtok:     0.10,
		CacheCreationPerMtok: 1.25,
	}},
	{prefix: "claude-4-haiku", pricing: ModelPricing{
		InputPerMtok:         1.00,
		OutputPerMtok:        5.00,
		CacheReadPerMtok:     0.10,
		CacheCreationPerMtok: 1.25,
	}},

	// ── Claude 3.5 Haiku ─────────────────────────────────────────────────────
	// $0.80 input / $4 output per Mtok
	{prefix: "claude-3-5-haiku", pricing: ModelPricing{
		InputPerMtok:         0.80,
		OutputPerMtok:        4.00,
		CacheReadPerMtok:     0.08,
		CacheCreationPerMtok: 1.00,
	}},

	// ── Claude 3 Opus ────────────────────────────────────────────────────────
	// $15 input / $75 output per Mtok
	{prefix: "claude-3-opus", pricing: ModelPricing{
		InputPerMtok:         15.00,
		OutputPerMtok:        75.00,
		CacheReadPerMtok:     1.50,
		CacheCreationPerMtok: 18.75,
	}},

	// ── Claude 3 Sonnet ──────────────────────────────────────────────────────
	// $3 input / $15 output per Mtok
	{prefix: "claude-3-sonnet", pricing: ModelPricing{
		InputPerMtok:         3.00,
		OutputPerMtok:        15.00,
		CacheReadPerMtok:     0.30,
		CacheCreationPerMtok: 3.75,
	}},

	// ── Claude 3 Haiku ───────────────────────────────────────────────────────
	// $0.25 input / $1.25 output per Mtok
	{prefix: "claude-3-haiku", pricing: ModelPricing{
		InputPerMtok:         0.25,
		OutputPerMtok:        1.25,
		CacheReadPerMtok:     0.03,
		CacheCreationPerMtok: 0.30,
	}},
}

// modelPricingEntry pairs a name prefix with a ModelPricing.
type modelPricingEntry struct {
	prefix  string
	pricing ModelPricing
}

// LookupPricing returns the ModelPricing for model, matched by longest prefix.
// The second return value is false when the model is not recognised.
func LookupPricing(model string) (ModelPricing, bool) {
	bestLen := 0
	var best *ModelPricing
	for i := range modelPricingTable {
		e := &modelPricingTable[i]
		if len(e.prefix) > bestLen && hasPrefix(model, e.prefix) {
			bestLen = len(e.prefix)
			best = &e.pricing
		}
	}
	if best == nil {
		return ModelPricing{}, false
	}
	pricing := *best
	if pricing.WebSearchPerRequest == 0 && hasPrefix(model, "claude") {
		pricing.WebSearchPerRequest = WebSearchRequestPriceUSD
	}
	return pricing, true
}

// hasPrefix is a simple ASCII-lowercase prefix check.
func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		sc := s[i]
		if sc >= 'A' && sc <= 'Z' {
			sc += 'a' - 'A'
		}
		pc := prefix[i]
		if pc >= 'A' && pc <= 'Z' {
			pc += 'a' - 'A'
		}
		if sc != pc {
			return false
		}
	}
	return true
}
