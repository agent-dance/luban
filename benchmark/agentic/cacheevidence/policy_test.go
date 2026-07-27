package cacheevidence

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectRequestProjectsCachePolicyWithoutContent(t *testing.T) {
	body := []byte(`{
  "model":"gpt-5.6-sol",
  "prompt_cache_key":"secret-cache-key",
  "prompt_cache_options":{"mode":"implicit","ttl":"30m"},
  "input":[{"type":"message","role":"developer","content":[
    {"type":"input_text","text":"secret system prompt","prompt_cache_breakpoint":{"mode":"explicit"}},
    {"type":"input_text","text":"secret dynamic prompt"}
  ]}]
}`)
	policy, ok := InspectRequest(body)
	if !ok || !policy.Observed || !policy.ShapeValid {
		t.Fatalf("policy was not observed as valid: %#v, ok=%v", policy, ok)
	}
	if !policy.PromptCacheKeyPresent || len(policy.PromptCacheKeySHA256) != 64 {
		t.Fatalf("cache key projection = %#v", policy)
	}
	if !policy.PromptCacheOptionsPresent || policy.PromptCacheOptionsMode != "implicit" || !policy.PromptCacheOptionsTTLPresent || policy.PromptCacheOptionsTTL != "30m" || policy.PromptCacheOptionsTTLSeconds == nil || *policy.PromptCacheOptionsTTLSeconds != 1800 {
		t.Fatalf("cache options projection = %#v", policy)
	}
	if policy.PromptCacheRetentionPresent || policy.PromptCacheBreakpointCount != 1 || len(policy.PromptCacheBreakpointPositions) != 1 || len(policy.PromptCacheBreakpointPositions[0]) != 64 {
		t.Fatalf("cache retention/breakpoint projection = %#v", policy)
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-cache-key", "secret system prompt", "secret dynamic prompt", "/input/0/content/0"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("projection retained request content or raw position %q: %s", forbidden, encoded)
		}
	}
}

func TestInspectRequestPreservesOmittedPolicyAsOmitted(t *testing.T) {
	policy, ok := InspectRequest([]byte(`{"model":"gpt-5.6-sol","input":[]}`))
	if !ok || !policy.Observed || !policy.ShapeValid {
		t.Fatalf("omitted policy request was invalid: %#v, ok=%v", policy, ok)
	}
	if policy.PromptCacheKeyPresent || policy.PromptCacheOptionsPresent || policy.PromptCacheOptionsTTLPresent || policy.PromptCacheOptionsTTLSeconds != nil || policy.PromptCacheRetentionPresent || policy.PromptCacheBreakpointCount != 0 || len(policy.PromptCacheBreakpointPositions) != 0 {
		t.Fatalf("omitted policy was synthesized: %#v", policy)
	}
}

func TestInspectRequestMarksMalformedPolicyShapeInvalid(t *testing.T) {
	for _, body := range []string{
		`{"prompt_cache_key":7}`,
		`{"prompt_cache_options":"implicit"}`,
		`{"prompt_cache_options":{"ttl":"forever"}}`,
		`{"prompt_cache_retention":24}`,
		`{"input":[{"prompt_cache_breakpoint":{"mode":7}}]}`,
		`{"input":[{"type":"message","content":[{"type":"input_text","prompt_cache_breakpoint":{"mode":"implicit"}}]}]}`,
	} {
		policy, ok := InspectRequest([]byte(body))
		if !ok || !policy.Observed || policy.ShapeValid {
			t.Fatalf("malformed policy accepted for %s: %#v, ok=%v", body, policy, ok)
		}
	}
	if _, ok := InspectRequest([]byte(`{"model":`)); ok {
		t.Fatal("malformed JSON was observed as a request")
	}
	if _, ok := InspectRequest([]byte(`{} trailing`)); ok {
		t.Fatal("trailing malformed input was observed as a request")
	}
	if _, ok := InspectRequest([]byte(`{"prompt_cache_key":"first","prompt_cache_key":"second"}`)); ok {
		t.Fatal("duplicate JSON key was observed as a request")
	}
	outside, ok := InspectRequest([]byte(`{"tools":[{"prompt_cache_breakpoint":{"mode":"explicit"}}]}`))
	if !ok || outside.ShapeValid {
		t.Fatalf("breakpoint outside a cacheable content block was accepted: %#v, ok=%v", outside, ok)
	}
}

func TestSummarizeLineageCountsUniqueKeysAndTransitions(t *testing.T) {
	first, ok := InspectRequest([]byte(`{"prompt_cache_key":"lineage-a"}`))
	if !ok {
		t.Fatal("first request did not decode")
	}
	second, ok := InspectRequest([]byte(`{"prompt_cache_key":"lineage-a"}`))
	if !ok {
		t.Fatal("second request did not decode")
	}
	changed, ok := InspectRequest([]byte(`{"prompt_cache_key":"lineage-b"}`))
	if !ok {
		t.Fatal("changed request did not decode")
	}

	stable := SummarizeLineage([]RequestPolicy{first, second})
	if !stable.Stable || stable.ObservedRequests != 2 || stable.InvalidRequests != 0 || stable.KeyPresentRequests != 2 || stable.UniqueKeyCount != 1 || stable.KeyTransitions != 0 || stable.FirstKeySHA256 == "" {
		t.Fatalf("stable lineage summary = %#v", stable)
	}
	unstable := SummarizeLineage([]RequestPolicy{first, second, changed, {Observed: true, ShapeValid: true}})
	if unstable.Stable || unstable.ObservedRequests != 4 || unstable.KeyPresentRequests != 3 || unstable.UniqueKeyCount != 2 || unstable.KeyTransitions != 2 {
		t.Fatalf("unstable lineage summary = %#v", unstable)
	}
	invalid := first
	invalid.ShapeValid = false
	if summary := SummarizeLineage([]RequestPolicy{first, invalid}); summary.Stable || summary.InvalidRequests != 1 {
		t.Fatalf("invalid request was hidden by the lineage summary: %#v", summary)
	}
}
