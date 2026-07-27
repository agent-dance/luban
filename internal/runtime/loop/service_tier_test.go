package loop

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestEnvelopeFingerprintChangesWithServiceTier(t *testing.T) {
	base := provider.Params{
		Model: "gpt-5.6-sol", MaxTokens: 1024, ReasoningEffort: "xhigh",
		Messages: []types.Message{types.UserMessage("same input")},
	}
	defaultTier := base
	defaultTier.ServiceTier = provider.ServiceTierDefault
	if envelopeFingerprint(base) == envelopeFingerprint(defaultTier) {
		t.Fatal("service-tier drift did not invalidate the response-chain envelope")
	}
	copyOfDefault := defaultTier
	if envelopeFingerprint(defaultTier) != envelopeFingerprint(copyOfDefault) {
		t.Fatal("identical default service tiers produced different envelope fingerprints")
	}
}

func TestProviderGoalEvaluatorInheritsServiceTier(t *testing.T) {
	fake := &providerGoalEvaluatorFake{}
	evaluator := NewProviderGoalEvaluatorWithModelAndServiceTier(
		fake,
		"gpt-5.6-sol",
		provider.ServiceTierDefault,
	)
	if _, err := evaluator.Evaluate(context.Background(), GoalEvaluationRequest{Objective: "finish"}); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := fake.lastParams(t).ServiceTier; got != provider.ServiceTierDefault {
		t.Fatalf("goal evaluator ServiceTier = %q, want default", got)
	}
}
