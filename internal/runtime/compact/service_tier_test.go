package compact

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestStructuredSummarizerInheritsServiceTier(t *testing.T) {
	fake := &recordingSummaryProvider{}
	summarize := NewLLMStructuredSummarizeFuncWithServiceTier(fake, provider.ServiceTierDefault)
	if _, err := summarize(context.Background(), []types.Message{types.UserMessage("summarize")}, ""); err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0].ServiceTier != provider.ServiceTierDefault {
		t.Fatalf("summary calls = %#v, want one default-tier request", fake.calls)
	}
}
