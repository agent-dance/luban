package harness

import (
	"errors"

	"github.com/agent-dance/luban/benchmark/agentic/cacheevidence"
)

// ErrInvalidCacheRequestEvidence is a stable protocol error code. The caller
// owns any localized user-facing diagnostic that wraps it.
var ErrInvalidCacheRequestEvidence = errors.New("agentic-bench/cache-request-evidence-invalid")

// ValidateCacheRequestEvidence validates the atomic cache-policy projection on
// one transport attempt. Omitted options remain omitted; this function never
// infers provider defaults or a cold/warm state.
func ValidateCacheRequestEvidence(round ProviderRoundEvidence) error {
	invalid := func() error { return ErrInvalidCacheRequestEvidence }
	if !round.CachePolicyObserved {
		if round.PromptCacheOptionsPresent || round.PromptCacheOptionsMode != "" || round.PromptCacheTTLSeconds != nil || round.PromptCacheRetentionPresent || round.PromptCacheRetention != "" || round.CacheBreakpointCount != 0 || len(round.CacheBreakpointPositionHashes) != 0 {
			return invalid()
		}
		return nil
	}

	if round.PromptCacheKeyPresent != (round.PromptCacheKeyHash != "") || round.PromptCacheKeyHash != "" && !hex64Pattern.MatchString(round.PromptCacheKeyHash) {
		return invalid()
	}
	if !round.PromptCacheOptionsPresent {
		if round.PromptCacheOptionsMode != "" || round.PromptCacheTTLSeconds != nil {
			return invalid()
		}
	} else if round.PromptCacheOptionsMode != "implicit" && round.PromptCacheOptionsMode != "explicit" {
		return invalid()
	}
	if round.PromptCacheTTLSeconds != nil && *round.PromptCacheTTLSeconds <= 0 {
		return invalid()
	}
	if round.PromptCacheRetentionPresent {
		if round.PromptCacheRetention != "24h" && round.PromptCacheRetention != "in_memory" {
			return invalid()
		}
	} else if round.PromptCacheRetention != "" {
		return invalid()
	}
	if round.PromptCacheOptionsPresent && round.PromptCacheRetentionPresent {
		return invalid()
	}
	if round.CacheBreakpointCount != len(round.CacheBreakpointPositionHashes) || round.CacheBreakpointCount < 0 {
		return invalid()
	}
	seen := make(map[string]struct{}, len(round.CacheBreakpointPositionHashes))
	for _, digest := range round.CacheBreakpointPositionHashes {
		if !hex64Pattern.MatchString(digest) {
			return invalid()
		}
		if _, duplicate := seen[digest]; duplicate {
			return invalid()
		}
		seen[digest] = struct{}{}
	}
	return nil
}

// SummarizeProviderCacheLineage measures request-key continuity within one
// physical run. A stable lineage is not evidence that the initial state was
// cold or that cached tokens were caused by the harness.
func SummarizeProviderCacheLineage(rounds []ProviderRoundEvidence) cacheevidence.LineageSummary {
	policies := make([]cacheevidence.RequestPolicy, 0, len(rounds))
	for _, round := range rounds {
		policies = append(policies, cacheevidence.RequestPolicy{
			Observed: round.CachePolicyObserved,
			// Callers aggregate only after ValidateCacheRequestEvidence succeeds;
			// the normalized round has no separate raw-policy validity field.
			ShapeValid:            round.CachePolicyObserved,
			PromptCacheKeyPresent: round.PromptCacheKeyPresent,
			PromptCacheKeySHA256:  round.PromptCacheKeyHash,
		})
	}
	return cacheevidence.SummarizeLineage(policies)
}
