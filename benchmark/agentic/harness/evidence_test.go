package harness

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestValidateAndAggregateEvidenceSeparatesRoundsToolsCacheAndCost(t *testing.T) {
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	providerCost := 1.25
	rounds := []ProviderRoundEvidence{
		{
			SchemaVersion: "agentic-bench/provider-round-v2", Round: 0, Outcome: "success", RequestID: strings.Repeat("a", 64), ResponseIDHash: strings.Repeat("b", 64),
			StartedAt: now, UpstreamHeadersAt: now.Add(100 * time.Millisecond), FirstResponseByteAt: now.Add(200 * time.Millisecond), FinishedAt: now.Add(time.Second),
			Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", StoreSpecified: true,
			EncryptedReasoningRequested: true,
			HTTPStatus:                  200, RequestBytes: 1000, ResponseBytes: 100,
			UsagePresent: true, InputTokens: int64TestPointer(1000), CachedInputTokens: int64TestPointer(800), CacheWriteInputTokens: int64TestPointer(0), OutputTokens: int64TestPointer(100),
			ProviderReportedCost:   &providerCost,
			PhysicalToolOperations: intTestPointer(1), ToolCriticalPathMS: int64TestPointer(41), ToolTotalLatencyMS: int64TestPointer(19), ToolQueueMS: int64TestPointer(30),
			ToolCalls: []ToolCallEvidence{
				{ID: "tool-0", Name: "Inspect", DurationMS: int64TestPointer(10), Error: boolTestPointer(false), InputBytes: 20, OutputBytes: int64TestPointer(30), TraceMatch: "id"},
				{ID: "tool-1", Name: "Run", DurationMS: int64TestPointer(5), Error: boolTestPointer(true), OutputBytes: int64TestPointer(0), TraceMatch: "ordered_kind"},
			},
		},
		{
			SchemaVersion: "agentic-bench/provider-round-v2", Round: 1, Outcome: "success", RequestID: strings.Repeat("c", 64), ResponseIDHash: strings.Repeat("d", 64),
			StartedAt: now.Add(2 * time.Second), UpstreamHeadersAt: now.Add(2100 * time.Millisecond), FirstResponseByteAt: now.Add(2200 * time.Millisecond), FinishedAt: now.Add(3 * time.Second),
			Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", StoreSpecified: true,
			EncryptedReasoningRequested: true,
			HTTPStatus:                  200, RequestBytes: 2000, ResponseBytes: 200,
			UsagePresent: true, InputTokens: int64TestPointer(2000), CachedInputTokens: int64TestPointer(1600), CacheWriteInputTokens: int64TestPointer(0), OutputTokens: int64TestPointer(200),
		},
	}
	completeEvidenceTestRounds(rounds)
	catalog := PricingCatalog{UnitTokens: 1000, Rates: []PricingRate{{Provider: "openai", Model: "gpt-5.6-sol", Input: 5, CachedInput: .5, Output: 30, CacheWriteInputMultiplier: 1.25}}}
	metrics, err := ValidateAndAggregateEvidence(rounds, formalEvidenceTestModel("luban", TransportRequirementHTTPInference), catalog)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.ProviderRequests != 2 || metrics.ProviderRounds != 2 || metrics.ProviderErrors != 0 || metrics.ToolBearingRounds != 1 || metrics.ToolInvocations != 2 || metrics.ToolErrors != 1 {
		t.Fatalf("layered counters are wrong: %#v", metrics)
	}
	if metrics.ToolErrorObservations != 2 || metrics.ToolDurationObservations != 2 || metrics.ToolOutputObservations != 2 || metrics.ToolTraceIDMatches != 1 || metrics.ToolTraceOrderedMatches != 1 || metrics.ToolTraceUnmatched != 0 {
		t.Fatalf("observability counters are wrong: %#v", metrics)
	}
	if metrics.CacheHitRate == nil || math.Abs(*metrics.CacheHitRate-.8) > 1e-9 {
		t.Fatalf("cache hit rate = %v, want .8", metrics.CacheHitRate)
	}
	wantCost := .6*5 + 2.4*.5 + .3*30
	if metrics.CatalogCost == nil || math.Abs(*metrics.CatalogCost-wantCost) > 1e-9 {
		t.Fatalf("catalog cost = %v, want %f", metrics.CatalogCost, wantCost)
	}
	if metrics.ProviderReportedCost != nil || metrics.ProviderReportedCostPartial == nil || *metrics.ProviderReportedCostPartial != providerCost {
		t.Fatalf("partial provider cost coverage not preserved: %#v %#v", metrics.ProviderReportedCost, metrics.ProviderReportedCostPartial)
	}
	if metrics.PhysicalToolObservations != 1 || metrics.PhysicalToolOperations != 1 || metrics.ToolCriticalPathMS != 41 || metrics.ToolTotalLatencyMS != 19 {
		t.Fatalf("independent operational counters are wrong: %#v", metrics)
	}
}

func TestCatalogCostAppliesLongContextTierPerRequestAtStrictBoundary(t *testing.T) {
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	round := func(index int, input, cached, cacheWrite, output int64) ProviderRoundEvidence {
		result := ProviderRoundEvidence{
			SchemaVersion: "agentic-bench/provider-round-v2", Round: index, Outcome: "success",
			RequestID: strings.Repeat(string(rune('a'+index*2)), 64), ResponseIDHash: strings.Repeat(string(rune('b'+index*2)), 64),
			StartedAt: now.Add(time.Duration(index) * time.Second), UpstreamHeadersAt: now.Add(time.Duration(index)*time.Second + time.Millisecond),
			FirstResponseByteAt: now.Add(time.Duration(index)*time.Second + 2*time.Millisecond), FinishedAt: now.Add(time.Duration(index)*time.Second + 3*time.Millisecond),
			Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", StoreSpecified: true, EncryptedReasoningRequested: true,
			HTTPStatus: 200, RequestBytes: 1, ResponseBytes: 1, UsagePresent: true,
			InputTokens: int64TestPointer(input), CachedInputTokens: int64TestPointer(cached), CacheWriteInputTokens: int64TestPointer(cacheWrite), OutputTokens: int64TestPointer(output),
		}
		completeEvidenceTestRound(&result)
		return result
	}
	rounds := []ProviderRoundEvidence{
		round(0, 272000, 100000, 20000, 1000),
		round(1, 272001, 200000, 72001, 2000),
	}
	catalog := PricingCatalog{UnitTokens: 1_000_000, Rates: []PricingRate{{
		Provider: "openai", Model: "gpt-5.6-sol", Input: 5, CachedInput: .5, Output: 30, CacheWriteInputMultiplier: 1.25,
		RequestTiers: []PricingTier{{Name: "long-context", ThresholdInputTokens: 272000, InputMultiplier: 2, CachedInputMultiplier: 2, OutputMultiplier: 1.5}},
	}}}
	metrics, err := ValidateAndAggregateEvidence(rounds, formalEvidenceTestModel("luban", TransportRequirementHTTPInference), catalog)
	if err != nil {
		t.Fatal(err)
	}
	const want = 2.1550125
	if metrics.CatalogCost == nil || math.Abs(*metrics.CatalogCost-want) > 1e-12 {
		t.Fatalf("per-request tiered catalog cost = %v, want %.7f", metrics.CatalogCost, want)
	}
	if metrics.CacheWriteTokenObservations != 2 || metrics.CacheWriteInputTokens != 92001 || metrics.UnreportedCacheWriteRounds != 0 || math.Abs(metrics.KnownCacheWriteSurcharge-.2050025) > 1e-12 {
		t.Fatalf("cache-write accounting = %#v", metrics)
	}
}

func TestCatalogCostIsPartialWhenCacheWriteUsageIsUnreported(t *testing.T) {
	now := time.Now().UTC()
	round := ProviderRoundEvidence{
		SchemaVersion: "agentic-bench/provider-round-v2", Round: 0, Outcome: "success", RequestID: strings.Repeat("a", 64), ResponseIDHash: strings.Repeat("b", 64),
		StartedAt: now, UpstreamHeadersAt: now, FirstResponseByteAt: now, FinishedAt: now,
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", StoreSpecified: true, EncryptedReasoningRequested: true,
		HTTPStatus: 200, RequestBytes: 1, ResponseBytes: 1, UsagePresent: true,
		InputTokens: int64TestPointer(100), CachedInputTokens: int64TestPointer(0), OutputTokens: int64TestPointer(10),
	}
	completeEvidenceTestRound(&round)
	catalog := PricingCatalog{UnitTokens: 1_000_000, Rates: []PricingRate{{Provider: "openai", Model: "gpt-5.6-sol", Input: 5, CachedInput: .5, Output: 30, CacheWriteInputMultiplier: 1.25}}}
	metrics, err := ValidateAndAggregateEvidence([]ProviderRoundEvidence{round}, formalEvidenceTestModel("luban", TransportRequirementHTTPInference), catalog)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.CatalogCost != nil || metrics.CatalogCostPartial == nil || metrics.UnreportedCacheWriteRounds != 1 {
		t.Fatalf("unreported cache-write cost was presented as complete: %#v", metrics)
	}
}

func TestEvidenceFailsClosedOnRequestedModelMismatch(t *testing.T) {
	now := time.Now()
	rounds := []ProviderRoundEvidence{{
		SchemaVersion: "agentic-bench/provider-round-v2", Round: 0, Outcome: "success", RequestID: strings.Repeat("a", 64), ResponseIDHash: strings.Repeat("b", 64),
		StartedAt: now, UpstreamHeadersAt: now, FirstResponseByteAt: now, FinishedAt: now,
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "high", StoreSpecified: true,
		EncryptedReasoningRequested: true,
		HTTPStatus:                  200, RequestBytes: 1, ResponseBytes: 1,
		UsagePresent: true, InputTokens: int64TestPointer(0), CachedInputTokens: int64TestPointer(0), OutputTokens: int64TestPointer(0),
	}}
	completeEvidenceTestRounds(rounds)
	catalog := PricingCatalog{UnitTokens: 1_000_000, Rates: []PricingRate{{Provider: "openai", Model: "gpt-5.6-sol"}}}
	_, err := ValidateAndAggregateEvidence(rounds, formalEvidenceTestModel("luban", TransportRequirementHTTPInference), catalog)
	if err == nil {
		t.Fatal("reasoning effort mismatch was accepted")
	}
}

func TestValidateAndAggregateEvidenceAcceptsOnlySealedContextFailureDisposition(t *testing.T) {
	now := time.Now().UTC()
	round := ProviderRoundEvidence{
		SchemaVersion: "agentic-bench/provider-round-v2", Round: 0, Outcome: "error",
		RequestID: strings.Repeat("a", 64), ResponseIDHash: strings.Repeat("b", 64),
		StartedAt: now, UpstreamHeadersAt: now, FirstResponseByteAt: now, FinishedAt: now,
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", StoreSpecified: true,
		EncryptedReasoningRequested: true, HTTPStatus: 200, RequestBytes: 1, ResponseBytes: 1,
	}
	completeEvidenceTestRound(&round)
	round.Outcome = "error"
	round.TransportDisposition = "agent_context_failure"
	round.ErrorCode = "provider_context_failure"
	round.ResponseCompleted = false
	round.ResponseStatus = "failed"
	round.ResponseFailureCode = "context_length_exceeded"
	round.ResponseFailureEventSHA256 = strings.Repeat("d", 64)

	metrics, err := ValidateAndAggregateEvidence(
		[]ProviderRoundEvidence{round},
		formalEvidenceTestModel("luban", TransportRequirementHTTPInference),
		PricingCatalog{UnitTokens: 1_000_000, Rates: []PricingRate{{Provider: "openai", Model: "gpt-5.6-sol"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.ProviderRequests != 1 || metrics.ProviderRounds != 0 || metrics.ProviderErrors != 1 {
		t.Fatalf("context failure metrics = %#v", metrics)
	}

	for name, mutate := range map[string]func(*ProviderRoundEvidence){
		"unknown code":      func(value *ProviderRoundEvidence) { value.ResponseFailureCode = "future_failure" },
		"missing digest":    func(value *ProviderRoundEvidence) { value.ResponseFailureEventSHA256 = "" },
		"wrong disposition": func(value *ProviderRoundEvidence) { value.TransportDisposition = "provider_infra_exclusion" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := round
			mutate(&invalid)
			if _, err := ValidateAndAggregateEvidence(
				[]ProviderRoundEvidence{invalid},
				formalEvidenceTestModel("luban", TransportRequirementHTTPInference),
				PricingCatalog{UnitTokens: 1_000_000, Rates: []PricingRate{{Provider: "openai", Model: "gpt-5.6-sol"}}},
			); err == nil {
				t.Fatal("invalid context failure evidence was accepted")
			}
		})
	}
}

func TestEvidenceLocksExactAgentCatalogAndPreservesUnexecutedTerminalCalls(t *testing.T) {
	now := time.Now().UTC()
	base := ProviderRoundEvidence{
		SchemaVersion: "agentic-bench/provider-round-v2", Round: 0, Outcome: "success",
		RequestID: strings.Repeat("a", 64), ResponseIDHash: strings.Repeat("b", 64),
		StartedAt: now, UpstreamHeadersAt: now, FirstResponseByteAt: now, FinishedAt: now,
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", StoreSpecified: true,
		HTTPStatus: 200, RequestBytes: 1, ResponseBytes: 1, UsagePresent: true,
		InputTokens: int64TestPointer(1), CachedInputTokens: int64TestPointer(0), CacheWriteInputTokens: int64TestPointer(0), OutputTokens: int64TestPointer(1),
		ToolCalls: []ToolCallEvidence{{ID: "unexecuted-terminal-call", Kind: "function_call", Name: "Run", InputBytes: 7}},
	}
	completeEvidenceTestRound(&base)
	expected := formalEvidenceTestModel("luban", TransportRequirementHTTPInference)
	catalog := PricingCatalog{UnitTokens: 1_000_000, Rates: []PricingRate{{Provider: "openai", Model: "gpt-5.6-sol"}}}
	metrics, err := ValidateAndAggregateEvidence([]ProviderRoundEvidence{base}, expected, catalog)
	if err != nil {
		t.Fatalf("unexecuted terminal tool call should remain a scored failure observation: %v", err)
	}
	if metrics.ToolInvocations != 1 || metrics.ToolTraceUnmatched != 1 {
		t.Fatalf("unexecuted terminal tool metrics = %#v", metrics)
	}

	for name, mutate := range map[string]func(*ProviderRoundEvidence){
		"extra tool": func(round *ProviderRoundEvidence) {
			round.ToolDefinitions = append(round.ToolDefinitions, ToolDefinitionEvidence{Type: "function", Name: "Extra", BillingOwner: "client", SchemaHash: strings.Repeat("1", 64), SchemaSHA256: strings.Repeat("2", 64), SchemaBytes: 1, DefinitionSHA256: strings.Repeat("3", 64), DefinitionBytes: 1})
			refreshEvidenceTestCatalog(round)
		},
		"reordered tool": func(round *ProviderRoundEvidence) {
			round.ToolDefinitions[0], round.ToolDefinitions[1] = round.ToolDefinitions[1], round.ToolDefinitions[0]
			refreshEvidenceTestCatalog(round)
		},
		"hosted owner": func(round *ProviderRoundEvidence) {
			round.ToolDefinitions[0].BillingOwner = "provider"
			refreshEvidenceTestCatalog(round)
		},
		"changed definition": func(round *ProviderRoundEvidence) {
			round.ToolDefinitions[0].DefinitionSHA256 = strings.Repeat("9", 64)
			refreshEvidenceTestCatalog(round)
		},
		"call outside catalog":    func(round *ProviderRoundEvidence) { round.ToolCalls[0].Name = "Bash" },
		"partial execution claim": func(round *ProviderRoundEvidence) { round.ToolCalls[0].TraceMatch = "id" },
		"wrong transport lane":    func(round *ProviderRoundEvidence) { round.Transport = "websocket" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := base
			invalid.ToolDefinitions = append([]ToolDefinitionEvidence(nil), base.ToolDefinitions...)
			invalid.ToolCalls = append([]ToolCallEvidence(nil), base.ToolCalls...)
			mutate(&invalid)
			if _, err := ValidateAndAggregateEvidence([]ProviderRoundEvidence{invalid}, expected, catalog); err == nil {
				t.Fatal("invalid formal local-tool evidence was accepted")
			}
		})
	}
}

func TestEvidenceRejectsIncompleteOrUnstablePerRunCacheLineage(t *testing.T) {
	now := time.Now().UTC()
	makeRound := func(index int) ProviderRoundEvidence {
		round := ProviderRoundEvidence{
			SchemaVersion: "agentic-bench/provider-round-v2", Round: index, Outcome: "success",
			RequestID: strings.Repeat(string(rune('a'+index*2)), 64), ResponseIDHash: strings.Repeat(string(rune('b'+index*2)), 64),
			StartedAt: now.Add(time.Duration(index) * time.Second), UpstreamHeadersAt: now.Add(time.Duration(index)*time.Second + time.Millisecond),
			FirstResponseByteAt: now.Add(time.Duration(index)*time.Second + 2*time.Millisecond), FinishedAt: now.Add(time.Duration(index)*time.Second + 3*time.Millisecond),
			Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", StoreSpecified: true,
			HTTPStatus: 200, RequestBytes: 1, ResponseBytes: 1,
			CachePolicyObserved: true, PromptCacheKeyPresent: true, PromptCacheKeyHash: strings.Repeat("7", 64),
		}
		completeEvidenceTestRound(&round)
		return round
	}
	rounds := []ProviderRoundEvidence{makeRound(0), makeRound(1)}
	expected := formalEvidenceTestModel("luban", TransportRequirementHTTPInference)
	catalog := PricingCatalog{UnitTokens: 1_000_000, Rates: []PricingRate{{Provider: "openai", Model: "gpt-5.6-sol"}}}
	if _, err := ValidateAndAggregateEvidence(rounds, expected, catalog); err != nil {
		t.Fatalf("stable complete cache lineage was rejected: %v", err)
	}

	for name, mutate := range map[string]func([]ProviderRoundEvidence){
		"missing policy and key coverage": func(candidate []ProviderRoundEvidence) {
			candidate[1].CachePolicyObserved = false
			candidate[1].PromptCacheKeyPresent = false
			candidate[1].PromptCacheKeyHash = ""
		},
		"observed request missing key": func(candidate []ProviderRoundEvidence) {
			candidate[1].PromptCacheKeyPresent = false
			candidate[1].PromptCacheKeyHash = ""
		},
		"key changes within run": func(candidate []ProviderRoundEvidence) {
			candidate[1].PromptCacheKeyHash = strings.Repeat("8", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := append([]ProviderRoundEvidence(nil), rounds...)
			mutate(candidate)
			if _, err := ValidateAndAggregateEvidence(candidate, expected, catalog); err == nil {
				t.Fatal("invalid per-run cache lineage was accepted")
			}
		})
	}
}

func TestEvidenceAcceptsCodexExactCatalogAndFinalTierBinding(t *testing.T) {
	now := time.Now().UTC()
	round := ProviderRoundEvidence{
		SchemaVersion: "agentic-bench/provider-round-v2", Round: 0, Outcome: "success",
		RequestID: strings.Repeat("a", 64), ResponseIDHash: strings.Repeat("b", 64),
		StartedAt: now, UpstreamHeadersAt: now, FirstResponseByteAt: now, FinishedAt: now,
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", StoreSpecified: true,
		HTTPStatus: 200, RequestBytes: 10, ResponseBytes: 1, UsagePresent: true,
		InputTokens: int64TestPointer(1), CachedInputTokens: int64TestPointer(0), CacheWriteInputTokens: int64TestPointer(0), OutputTokens: int64TestPointer(1),
		ToolCalls: []ToolCallEvidence{{ID: "codex-exec", Kind: "custom_tool_call", Name: "exec", InputBytes: 7, OutputBytes: int64TestPointer(0), TraceMatch: "id"}},
	}
	completeEvidenceTestRound(&round)
	round.ClientAgentID = "codex"
	round.RequestedServiceTierRaw = ""
	round.RequestedServiceTierPresent = false
	round.RequestedServiceTierRepresentation = ServiceTierEncodingClientCanonical
	round.ClientCanonicalizationProofSHA256 = strings.Repeat("e", 64)
	round.OriginalRequestBodySHA256 = strings.Repeat("1", 64)
	round.ForwardedRequestBodySHA256 = strings.Repeat("2", 64)
	round.OriginalRequestCanonicalSHA256 = strings.Repeat("3", 64)
	round.ForwardedRequestCanonicalSHA256 = strings.Repeat("4", 64)
	round.OriginalServiceTierPresent = false
	round.OriginalServiceTier = ""
	round.ForwardedRequestBytes = 20
	round.ServiceTierTransformation = "inject_explicit_default"
	round.ToolDefinitions = formalEvidenceTestDefinitions("codex")
	refreshEvidenceTestCatalog(&round)
	_, err := ValidateAndAggregateEvidence(
		[]ProviderRoundEvidence{round},
		formalEvidenceTestModel("codex", TransportRequirementHTTPInference),
		PricingCatalog{UnitTokens: 1_000_000, Rates: []PricingRate{{Provider: "openai", Model: "gpt-5.6-sol"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrewarmSpendAndLLMCallsCoverAllStartedAttemptsWithoutInventingRetryAmplification(t *testing.T) {
	now := time.Now().UTC()
	providerCost := 0.25
	makeRound := func(roundNumber int) ProviderRoundEvidence {
		round := ProviderRoundEvidence{
			SchemaVersion: "agentic-bench/provider-round-v2", Round: roundNumber, Outcome: "success", ResponseIDHash: strings.Repeat(string(rune('a'+roundNumber)), 64),
			StartedAt: now.Add(time.Duration(roundNumber) * time.Second), FirstResponseByteAt: now.Add(time.Duration(roundNumber)*time.Second + time.Millisecond), FinishedAt: now.Add(time.Duration(roundNumber)*time.Second + 2*time.Millisecond),
			Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", StoreSpecified: true,
			RequestBytes: 10, ResponseBytes: 1, UsagePresent: true,
			InputTokens: int64TestPointer(10), CachedInputTokens: int64TestPointer(5), CacheWriteInputTokens: int64TestPointer(0), OutputTokens: int64TestPointer(1),
			ProviderReportedCost: &providerCost,
			CachePolicyObserved:  true, PromptCacheKeyPresent: true, PromptCacheKeyHash: strings.Repeat("7", 64),
		}
		completeEvidenceTestRound(&round)
		round.Transport = "websocket"
		round.RequestID = ""
		round.HTTPStatus = 0
		round.UpstreamHeadersAt = time.Time{}
		round.WebSocketConnectionHash = strings.Repeat("f", 64)
		round.WebSocketRequestSequence = uint64(roundNumber)
		round.WebSocketConnectionReused = roundNumber > 0
		round.WebSocketHandshakeStatus = http.StatusSwitchingProtocols
		round.WebSocketHandshakeModel = "gpt-5.6-sol"
		round.WebSocketChainBound = true
		return round
	}
	prewarm, inference := makeRound(0), makeRound(1)
	prewarm.ProviderAttemptKind = "prewarm"
	prewarm.GenerateSpecified = true
	prewarm.Generate = false
	prewarm.Outcome = "prewarm"
	prewarm.TransportDisposition = "prewarm_transport"
	inference.GenerateSpecified = false
	inference.Generate = false
	metrics, err := ValidateAndAggregateEvidence(
		[]ProviderRoundEvidence{prewarm, inference},
		formalEvidenceTestModel("luban", TransportRequirementWebSocket),
		PricingCatalog{UnitTokens: 1_000_000, Rates: []PricingRate{{Provider: "openai", Model: "gpt-5.6-sol"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.TransportAttempts != 2 || metrics.PrewarmAttempts != 1 || metrics.LLMCallsStarted != 1 || metrics.CompletedLLMResponses != 1 || metrics.RetryAmplification != nil ||
		metrics.AllExecutedUsageObservations != 2 || metrics.PrewarmUsageObservations != 1 || metrics.ProviderCostObservations != 2 ||
		metrics.ProviderReportedCost == nil || *metrics.ProviderReportedCost != 0.5 || metrics.CachePolicyObservedRequests != 2 || !metrics.CacheLineageStable {
		t.Fatalf("all-started/prewarm accounting = %#v", metrics)
	}
}

func int64TestPointer(value int64) *int64 { return &value }

func intTestPointer(value int) *int { return &value }

func boolTestPointer(value bool) *bool { return &value }

func formalEvidenceTestDefinitions(agentID string) []ToolDefinitionEvidence {
	identities := formalToolCatalog(agentID)
	definitions := make([]ToolDefinitionEvidence, 0, len(identities))
	for index, identity := range identities {
		definitions = append(definitions, ToolDefinitionEvidence{
			Type: identity.Type, Name: identity.Name, BillingOwner: "client",
			SchemaHash: strings.Repeat(string(rune('1'+index)), 64), SchemaSHA256: strings.Repeat(string(rune('4'+index)), 64), SchemaBytes: 1,
			DefinitionSHA256: strings.Repeat(string(rune('a'+index)), 64), DefinitionBytes: 1,
		})
	}
	return definitions
}

func formalEvidenceTestModel(agentID, transportRequirement string) ModelRequestSpec {
	definitions := formalEvidenceTestDefinitions(agentID)
	tools := make([]ToolIdentitySpec, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, ToolIdentitySpec{
			Type: definition.Type, Name: definition.Name, DefinitionSHA256: definition.DefinitionSHA256,
		})
	}
	encoding := ServiceTierEncodingExplicitDefault
	if agentID == "codex" {
		encoding = ServiceTierEncodingClientCanonical
	}
	return ModelRequestSpec{
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: FormalServiceTier,
		ServiceTierRequestEncoding: encoding, TransportRequirement: transportRequirement,
		ToolCatalog: ToolCatalogSpec{
			SchemaVersion: FormalToolCatalogSchemaVersion, SemanticSHA256: stableToolCatalogSHA256(definitions), Tools: tools,
		},
	}
}

func refreshEvidenceTestCatalog(round *ProviderRoundEvidence) {
	round.ToolDefinitionCount = len(round.ToolDefinitions)
	round.ToolCatalogCanonicalBytes = 0
	for _, definition := range round.ToolDefinitions {
		round.ToolCatalogCanonicalBytes += definition.DefinitionBytes
	}
	round.ToolCatalogSemanticSHA256 = stableToolCatalogSHA256(round.ToolDefinitions)
}

func completeEvidenceTestRounds(rounds []ProviderRoundEvidence) {
	for index := range rounds {
		completeEvidenceTestRound(&rounds[index])
	}
}

func completeEvidenceTestRound(round *ProviderRoundEvidence) {
	agentID := round.ClientAgentID
	if agentID == "" {
		agentID = "luban"
	}
	round.EvidenceSequence = uint64(round.Round)
	round.EvidenceHash = fmt.Sprintf("%064x", round.Round+1)
	if round.Round > 0 {
		round.PreviousEvidenceHash = fmt.Sprintf("%064x", round.Round)
	}
	round.RunIdentity = strings.Repeat("f", 64)
	round.ProviderAttemptStarted = true
	if round.Transport == "" {
		round.Transport = "http_sse"
	}
	if round.ProviderAttemptKind == "" {
		round.ProviderAttemptKind = "inference"
	}
	if round.Transport == "http_sse" {
		round.WebSocketChainBound = true
	}
	round.TransportDisposition = "valid"
	round.RequestedReasoningContext = "all_turns"
	round.RequestedReasoningModeCanonical = "standard"
	round.ContinuationLineagePresent = true
	round.ContinuationLineageHash = strings.Repeat("e", 64)
	round.ContinuationLineageSource = "agent_header"
	round.ContinuationEpoch = 1
	round.RequestedServiceTierCanonical = FormalServiceTier
	round.ClientAgentID = agentID
	round.OriginalRequestBodySHA256 = strings.Repeat("1", 64)
	round.OriginalRequestCanonicalSHA256 = strings.Repeat("2", 64)
	round.OriginalRequestWithoutServiceTierSHA256 = strings.Repeat("3", 64)
	round.ForwardedRequestWithoutServiceTierSHA256 = round.OriginalRequestWithoutServiceTierSHA256
	round.ForwardedServiceTierPresent = true
	round.ForwardedServiceTier = FormalServiceTier
	if agentID == "codex" {
		round.RequestedServiceTierRaw = ""
		round.RequestedServiceTierPresent = false
		round.RequestedServiceTierRepresentation = ServiceTierEncodingClientCanonical
		round.ClientCanonicalizationProofSHA256 = strings.Repeat("e", 64)
		round.OriginalServiceTierPresent = false
		round.OriginalServiceTier = ""
		round.ForwardedRequestBodySHA256 = strings.Repeat("4", 64)
		round.ForwardedRequestCanonicalSHA256 = strings.Repeat("5", 64)
		round.ForwardedRequestBytes = round.RequestBytes + 1
		round.ServiceTierTransformation = "inject_explicit_default"
	} else {
		round.RequestedServiceTierRaw = FormalServiceTier
		round.RequestedServiceTierPresent = true
		round.RequestedServiceTierRepresentation = ServiceTierEncodingExplicitDefault
		round.ClientCanonicalizationProofSHA256 = ""
		round.OriginalServiceTierPresent = true
		round.OriginalServiceTier = FormalServiceTier
		round.ForwardedRequestBodySHA256 = round.OriginalRequestBodySHA256
		round.ForwardedRequestCanonicalSHA256 = round.OriginalRequestCanonicalSHA256
		round.ForwardedRequestBytes = round.RequestBytes
		round.ServiceTierTransformation = "none"
	}
	round.ServiceTierTransformationExactDiff = true
	round.ServiceTierTransformationProofSHA256 = strings.Repeat("4", 64)
	round.ResponseServiceTierCanonical = FormalServiceTier
	round.ResponseServiceTierRaw = FormalServiceTier
	round.ServiceTierComparable = true
	round.ToolCatalogHash = strings.Repeat("c", 64)
	round.ToolDefinitions = formalEvidenceTestDefinitions(agentID)
	round.ToolDefinitionCount = len(round.ToolDefinitions)
	round.ToolCatalogCanonicalBytes = 0
	for index := range round.ToolDefinitions {
		definition := &round.ToolDefinitions[index]
		if definition.DefinitionBytes == 0 {
			definition.DefinitionBytes = 1
		}
		if definition.DefinitionSHA256 == "" {
			definition.DefinitionSHA256 = fmt.Sprintf("%064x", index+100)
		}
		if definition.SchemaBytes > 0 && definition.SchemaSHA256 == "" {
			definition.SchemaSHA256 = fmt.Sprintf("%064x", index+200)
		}
		if definition.DescriptionBytes > 0 && definition.DescriptionSHA256 == "" {
			definition.DescriptionSHA256 = fmt.Sprintf("%064x", index+300)
		}
		round.ToolCatalogCanonicalBytes += definition.DefinitionBytes
	}
	round.ToolCatalogSemanticSHA256 = stableToolCatalogSHA256(round.ToolDefinitions)
	round.ToolCatalogStable = true
	round.ToolResultHistoryValid = true
	round.ResponseModel = round.Model
	round.ResponseCompleted = true
	round.ResponseStatus = "completed"
	for index := range round.ToolCalls {
		if round.ToolCalls[index].Kind == "" {
			round.ToolCalls[index].Kind = "function_call"
		}
	}
}
