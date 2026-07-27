package harness

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCacheRequestEvidencePreservesMissingPolicy(t *testing.T) {
	round := ProviderRoundEvidence{
		PromptCacheKeyPresent: true,
		PromptCacheKeyHash:    strings.Repeat("a", 64),
	}
	if err := ValidateCacheRequestEvidence(round); err != nil {
		t.Fatalf("legacy key-only evidence was rejected: %v", err)
	}
	round.PromptCacheOptionsMode = "implicit"
	if err := ValidateCacheRequestEvidence(round); !errors.Is(err, ErrInvalidCacheRequestEvidence) {
		t.Fatalf("unobserved policy fields were accepted: %v", err)
	}
}

func TestValidateCacheRequestEvidenceAcceptsCodexAndLubanPolicies(t *testing.T) {
	codex := ProviderRoundEvidence{
		CachePolicyObserved:   true,
		PromptCacheKeyPresent: true,
		PromptCacheKeyHash:    strings.Repeat("a", 64),
	}
	if err := ValidateCacheRequestEvidence(codex); err != nil {
		t.Fatalf("Codex native omitted-options policy was rejected: %v", err)
	}
	ttl := int64(1800)
	luban := ProviderRoundEvidence{
		CachePolicyObserved:           true,
		PromptCacheKeyPresent:         true,
		PromptCacheKeyHash:            strings.Repeat("b", 64),
		PromptCacheOptionsPresent:     true,
		PromptCacheOptionsMode:        "implicit",
		PromptCacheTTLSeconds:         &ttl,
		CacheBreakpointCount:          1,
		CacheBreakpointPositionHashes: []string{strings.Repeat("c", 64)},
	}
	if err := ValidateCacheRequestEvidence(luban); err != nil {
		t.Fatalf("Luban GPT-5.6 cache policy was rejected: %v", err)
	}
}

func TestValidateCacheRequestEvidenceRejectsInconsistentProjection(t *testing.T) {
	ttl := int64(1800)
	valid := ProviderRoundEvidence{
		CachePolicyObserved:           true,
		PromptCacheKeyPresent:         true,
		PromptCacheKeyHash:            strings.Repeat("a", 64),
		PromptCacheOptionsPresent:     true,
		PromptCacheOptionsMode:        "implicit",
		PromptCacheTTLSeconds:         &ttl,
		CacheBreakpointCount:          1,
		CacheBreakpointPositionHashes: []string{strings.Repeat("b", 64)},
	}
	mutations := []func(*ProviderRoundEvidence){
		func(round *ProviderRoundEvidence) { round.PromptCacheKeyHash = "" },
		func(round *ProviderRoundEvidence) { round.PromptCacheOptionsPresent = false },
		func(round *ProviderRoundEvidence) { round.PromptCacheOptionsMode = "automatic" },
		func(round *ProviderRoundEvidence) { zero := int64(0); round.PromptCacheTTLSeconds = &zero },
		func(round *ProviderRoundEvidence) {
			round.PromptCacheRetentionPresent = true
			round.PromptCacheRetention = "24h"
		},
		func(round *ProviderRoundEvidence) { round.CacheBreakpointCount = 2 },
		func(round *ProviderRoundEvidence) { round.CacheBreakpointPositionHashes[0] = "invalid" },
		func(round *ProviderRoundEvidence) {
			round.CacheBreakpointCount = 2
			round.CacheBreakpointPositionHashes = []string{strings.Repeat("b", 64), strings.Repeat("b", 64)}
		},
	}
	for index, mutate := range mutations {
		candidate := valid
		candidate.CacheBreakpointPositionHashes = append([]string(nil), valid.CacheBreakpointPositionHashes...)
		mutate(&candidate)
		if err := ValidateCacheRequestEvidence(candidate); !errors.Is(err, ErrInvalidCacheRequestEvidence) {
			t.Fatalf("mutation %d was accepted: %#v, err=%v", index, candidate, err)
		}
	}
}

func TestSummarizeProviderCacheLineage(t *testing.T) {
	rounds := []ProviderRoundEvidence{
		{CachePolicyObserved: true, PromptCacheKeyPresent: true, PromptCacheKeyHash: strings.Repeat("a", 64)},
		{CachePolicyObserved: true, PromptCacheKeyPresent: true, PromptCacheKeyHash: strings.Repeat("a", 64)},
	}
	stable := SummarizeProviderCacheLineage(rounds)
	if !stable.Stable || stable.ObservedRequests != 2 || stable.KeyPresentRequests != 2 || stable.UniqueKeyCount != 1 || stable.KeyTransitions != 0 {
		t.Fatalf("stable provider cache lineage = %#v", stable)
	}
	rounds = append(rounds,
		ProviderRoundEvidence{CachePolicyObserved: true, PromptCacheKeyPresent: true, PromptCacheKeyHash: strings.Repeat("b", 64)},
		ProviderRoundEvidence{CachePolicyObserved: true},
	)
	changed := SummarizeProviderCacheLineage(rounds)
	if changed.Stable || changed.UniqueKeyCount != 2 || changed.KeyTransitions != 2 {
		t.Fatalf("changed provider cache lineage = %#v", changed)
	}
}
