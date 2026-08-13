package compact

import (
	"hash/fnv"
	"strings"
)

const (
	DefaultProgressiveMinTokenSavings         = 2_000
	DefaultProgressiveReuseHorizon            = 3
	DefaultProgressiveCacheRecoveryRequests   = 2
	DefaultProgressiveMaxProjectedTools       = 24
	DefaultProgressiveMaxProjectedTokens      = 48_000
	DefaultProgressiveMaxConsecutiveAnomalies = 3
)

// ProgressiveConfig is the rollout and safety control plane for progressive
// provider-view projection. The zero value is disabled. All allowlists are
// fail-closed when present; model entries use canonical prefix matching so a
// dated model revision can inherit an explicitly reviewed family policy.
type ProgressiveConfig struct {
	Enabled           bool     `json:"enabled"`
	Shadow            bool     `json:"shadow,omitempty"`
	KillSwitch        bool     `json:"killSwitch,omitempty"`
	RolloutPercent    int      `json:"rolloutPercent,omitempty"`
	ProviderAllowlist []string `json:"providerAllowlist,omitempty"`
	ModelAllowlist    []string `json:"modelAllowlist,omitempty"`
	// ProviderModelAllowlist prevents a multi-provider production policy from
	// admitting the cross-product of independently reviewed provider and model
	// families. Entries use "provider/model-prefix".
	ProviderModelAllowlist []string `json:"providerModelAllowlist,omitempty"`
	ToolAllowlist          []string `json:"toolAllowlist,omitempty"`
	// ImminentCompactProviderAllowlist enables a counterfactual correction for
	// providers whose next action would be semantic compaction. When a
	// projection gets the same request back below the hard threshold, both
	// branches reset the prompt cache, so the projection is charged only its
	// incremental cache cost instead of a second, duplicate reset penalty.
	ImminentCompactProviderAllowlist []string `json:"imminentCompactProviderAllowlist,omitempty"`
	// AutoCompactKeepRecent optionally lowers the semantic-compaction verbatim
	// tail for providers in ImminentCompactProviderAllowlist. Zero preserves the
	// compactor's established default.
	AutoCompactKeepRecent int `json:"autoCompactKeepRecent,omitempty"`
	// AutoCompactMaxGrowthTokens bounds an uncalibrated local request delta
	// above the last provider-reported input for reviewed providers. It prevents
	// tokenizer/schema representation bias from causing an early semantic
	// compact while still allowing the authoritative provider total to cross
	// the threshold. Zero preserves the established estimator.
	AutoCompactMaxGrowthTokens int `json:"autoCompactMaxGrowthTokens,omitempty"`
	// AutoCompactMinThresholdPercent may postpone semantic compaction until at
	// least this percentage of the effective input window is occupied. The
	// ordinary fixed-buffer threshold remains a floor, so this option can never
	// make compaction earlier. Zero preserves the established threshold.
	AutoCompactMinThresholdPercent int `json:"autoCompactMinThresholdPercent,omitempty"`
	// RequireConsumedMutation prevents pressure-only projection while exact
	// source reads may still be needed to construct the first mutation. It is a
	// provider-scoped quality guard for models that do not reliably recover
	// indexed evidence. False preserves the established GPT strategy.
	RequireConsumedMutation bool `json:"requireConsumedMutation,omitempty"`
	// BenefitTrigger admits the smallest cost-positive batch as soon as its
	// results have been consumed by a later assistant decision. The ordinary
	// path keeps the recent working set and rich rewrites; recoverable indexes
	// remain reserved for actual context pressure. Runtime admission also
	// requires the first changed byte to be beyond the last provider-reported
	// cache frontier and permits at most one early reset per session.
	BenefitTrigger                  bool     `json:"benefitTrigger,omitempty"`
	BenefitTriggerProviderAllowlist []string `json:"benefitTriggerProviderAllowlist,omitempty"`
	// FlattenCompactInput serializes the history as one explicitly untrusted
	// transcript for the semantic summarizer. This prevents reviewed providers
	// from continuing an in-progress tool loop instead of obeying the compact
	// request. False preserves the established structured GPT input.
	FlattenCompactInput bool `json:"flattenCompactInput,omitempty"`
	// ConciseCompactSummary selects a smaller coding handoff prompt and
	// CompactMaxOutputTokens optionally caps that provider-scoped response.
	// Zero/false preserve the established GPT nine-section, 20k-token contract.
	ConciseCompactSummary   bool    `json:"conciseCompactSummary,omitempty"`
	CompactMaxOutputTokens  int     `json:"compactMaxOutputTokens,omitempty"`
	MinTokenSavings         int     `json:"minTokenSavings,omitempty"`
	BenefitMinTokenSavings  int     `json:"benefitMinTokenSavings,omitempty"`
	ReuseHorizon            int     `json:"reuseHorizon,omitempty"`
	CacheRecoveryRequests   int     `json:"cacheRecoveryRequests,omitempty"`
	MinNetSavingsUSD        float64 `json:"minNetSavingsUsd,omitempty"`
	MaxProjectedTools       int     `json:"maxProjectedTools,omitempty"`
	MaxProjectedTokens      int     `json:"maxProjectedTokens,omitempty"`
	MaxConsecutiveAnomalies int     `json:"maxConsecutiveAnomalies,omitempty"`
}

// DefaultProgressiveConfig returns production-safe values while leaving the
// feature disabled until a caller deliberately enables it.
func DefaultProgressiveConfig() ProgressiveConfig {
	return ProgressiveConfig{
		RolloutPercent:          100,
		ToolAllowlist:           []string{"Inspect"},
		MinTokenSavings:         DefaultProgressiveMinTokenSavings,
		ReuseHorizon:            DefaultProgressiveReuseHorizon,
		CacheRecoveryRequests:   DefaultProgressiveCacheRecoveryRequests,
		MaxProjectedTools:       DefaultProgressiveMaxProjectedTools,
		MaxProjectedTokens:      DefaultProgressiveMaxProjectedTokens,
		MaxConsecutiveAnomalies: DefaultProgressiveMaxConsecutiveAnomalies,
	}
}

// ProductionProgressiveConfig enables only the provider, model family, and
// tool strategy that have passed a real provider A/B and the frozen quality
// evaluator. Callers may still override or disable this policy explicitly.
func ProductionProgressiveConfig() ProgressiveConfig {
	config := DefaultProgressiveConfig()
	config.Enabled = true
	config.ProviderAllowlist = []string{"openai", "deepseek"}
	config.ModelAllowlist = []string{"gpt-5.6-sol", "deepseek-v4-flash"}
	config.ProviderModelAllowlist = []string{"openai/gpt-5.6-sol", "deepseek/deepseek-v4-flash"}
	config.ToolAllowlist = []string{"Inspect"}
	config.ImminentCompactProviderAllowlist = []string{"deepseek"}
	config.AutoCompactKeepRecent = 1
	config.AutoCompactMaxGrowthTokens = 4_000
	config.AutoCompactMinThresholdPercent = 100
	config.RequireConsumedMutation = true
	config.FlattenCompactInput = true
	config.ConciseCompactSummary = true
	config.CompactMaxOutputTokens = 4_000
	config.BenefitTrigger = true
	config.BenefitTriggerProviderAllowlist = []string{"openai"}
	// A 6k threshold was the smallest tested OpenAI setting whose repeated real
	// traces were jointly positive on input, output, cost, turns, and median
	// provider time without invalidating an already-hit cached suffix.
	config.BenefitMinTokenSavings = 6_000
	return config
}

// NormalizeProgressiveConfig applies bounded defaults without turning the
// feature on. Invalid limits fail toward less projection.
func NormalizeProgressiveConfig(config ProgressiveConfig) ProgressiveConfig {
	defaults := DefaultProgressiveConfig()
	if config.RolloutPercent <= 0 {
		config.RolloutPercent = defaults.RolloutPercent
	}
	if config.RolloutPercent > 100 {
		config.RolloutPercent = 100
	}
	if config.MinTokenSavings <= 0 {
		config.MinTokenSavings = defaults.MinTokenSavings
	}
	if config.BenefitMinTokenSavings <= 0 {
		config.BenefitMinTokenSavings = config.MinTokenSavings
	}
	if config.AutoCompactKeepRecent < 0 {
		config.AutoCompactKeepRecent = 0
	}
	if config.AutoCompactMaxGrowthTokens < 0 {
		config.AutoCompactMaxGrowthTokens = 0
	}
	if config.AutoCompactMinThresholdPercent < 0 {
		config.AutoCompactMinThresholdPercent = 0
	} else if config.AutoCompactMinThresholdPercent > 100 {
		config.AutoCompactMinThresholdPercent = 100
	}
	if config.CompactMaxOutputTokens < 0 {
		config.CompactMaxOutputTokens = 0
	} else if config.CompactMaxOutputTokens > CompactMaxOutputTokens {
		config.CompactMaxOutputTokens = CompactMaxOutputTokens
	}
	if config.ReuseHorizon < 0 {
		config.ReuseHorizon = 0
	} else if config.ReuseHorizon == 0 {
		config.ReuseHorizon = defaults.ReuseHorizon
	}
	if config.CacheRecoveryRequests <= 0 {
		config.CacheRecoveryRequests = defaults.CacheRecoveryRequests
	}
	if config.MinNetSavingsUSD < 0 {
		config.MinNetSavingsUSD = 0
	}
	if config.MaxProjectedTools <= 0 {
		config.MaxProjectedTools = defaults.MaxProjectedTools
	}
	if config.MaxProjectedTokens <= 0 {
		config.MaxProjectedTokens = defaults.MaxProjectedTokens
	}
	if config.MaxConsecutiveAnomalies <= 0 {
		config.MaxConsecutiveAnomalies = defaults.MaxConsecutiveAnomalies
	}
	if len(config.ToolAllowlist) == 0 {
		config.ToolAllowlist = append([]string(nil), defaults.ToolAllowlist...)
	}
	config.ProviderAllowlist = normalizedProgressiveValues(config.ProviderAllowlist)
	config.ModelAllowlist = normalizedProgressiveValues(config.ModelAllowlist)
	config.ProviderModelAllowlist = normalizedProgressiveValues(config.ProviderModelAllowlist)
	config.ToolAllowlist = normalizedProgressiveValues(config.ToolAllowlist)
	config.ImminentCompactProviderAllowlist = normalizedProgressiveValues(config.ImminentCompactProviderAllowlist)
	config.BenefitTriggerProviderAllowlist = normalizedProgressiveValues(config.BenefitTriggerProviderAllowlist)
	return config
}

// ProgressiveEnabledForSession evaluates the dynamic safety controls. A
// stable session hash keeps rollout assignment unchanged across resume, fork
// preparation, and process restart.
func ProgressiveEnabledForSession(config ProgressiveConfig, providerName, model, sessionID string) bool {
	config = NormalizeProgressiveConfig(config)
	if !config.Enabled || config.KillSwitch || !progressiveAllowed(config.ProviderAllowlist, providerName, false) ||
		!progressiveAllowed(config.ModelAllowlist, model, true) || !progressiveProviderModelAllowed(config.ProviderModelAllowlist, providerName, model) {
		return false
	}
	if config.RolloutPercent >= 100 {
		return true
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.TrimSpace(sessionID)))
	return int(hash.Sum32()%100) < config.RolloutPercent
}

func progressiveProviderModelAllowed(allowlist []string, providerName, model string) bool {
	if len(allowlist) == 0 {
		return true
	}
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	model = strings.ToLower(strings.TrimSpace(model))
	for _, allowed := range allowlist {
		provider, modelPrefix, ok := strings.Cut(allowed, "/")
		if ok && providerName == provider && modelPrefix != "" && strings.HasPrefix(model, modelPrefix) {
			return true
		}
	}
	return false
}

// ProgressiveToolEnabled reports whether one reviewed tool strategy is in the
// current allowlist. Tool names are protocol identifiers and compare
// case-insensitively.
func ProgressiveToolEnabled(config ProgressiveConfig, toolName string) bool {
	config = NormalizeProgressiveConfig(config)
	return progressiveAllowed(config.ToolAllowlist, toolName, false)
}

// ProgressiveBenefitTriggerEnabled reports whether the early cost-positive
// trigger is enabled for this provider. An empty allowlist preserves explicit
// experiment configurations; production supplies a reviewed provider scope.
func ProgressiveBenefitTriggerEnabled(config ProgressiveConfig, providerName string) bool {
	config = NormalizeProgressiveConfig(config)
	return config.BenefitTrigger && progressiveAllowed(config.BenefitTriggerProviderAllowlist, providerName, false)
}

// ProgressiveImminentCompactCounterfactualEnabled reports whether a reviewed
// provider may compare projection against the cache reset that semantic
// compaction would otherwise perform immediately. An empty allowlist is
// deliberately disabled rather than wildcarded.
func ProgressiveImminentCompactCounterfactualEnabled(config ProgressiveConfig, providerName string) bool {
	config = NormalizeProgressiveConfig(config)
	return len(config.ImminentCompactProviderAllowlist) > 0 &&
		progressiveAllowed(config.ImminentCompactProviderAllowlist, providerName, false)
}

// ProgressiveProviderCompactPolicyEnabled applies the complete rollout and
// paired provider/model scope before any provider-specific semantic compact
// behavior is activated. Disabling progressive context therefore restores the
// legacy compactor in full, not only tool-result projection.
func ProgressiveProviderCompactPolicyEnabled(config ProgressiveConfig, providerName, model, sessionID string) bool {
	return ProgressiveEnabledForSession(config, providerName, model, sessionID) &&
		ProgressiveImminentCompactCounterfactualEnabled(config, providerName)
}

func progressiveAllowed(allowlist []string, value string, prefix bool) bool {
	if len(allowlist) == 0 {
		return true
	}
	value = strings.ToLower(strings.TrimSpace(value))
	for _, allowed := range allowlist {
		if prefix && strings.HasPrefix(value, allowed) || !prefix && value == allowed {
			return true
		}
	}
	return false
}

func normalizedProgressiveValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
