package loop

import (
	"testing"

	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/types"
)

func TestContextUsageReportsCompleteWindowWhileWarningsUseEffectiveCapacity(t *testing.T) {
	q := New(nil, nil, Config{MaxContextTokens: 200_000, MaxOutputTokens: 20_000})
	q.ctxWindow.UpdateUsage(&types.Usage{InputTokens: 100_000})

	capacity, usage := q.ContextUsageDetail()
	if capacity != 200_000 || usage.UsedTokens != 100_000 || usage.Measurement != compact.ContextUsageProviderReported {
		t.Fatalf("display context = %d/%+v, want 100000/200000 provider-reported", capacity, usage)
	}
	warning := q.ContextWarningState()
	if warning.EffectiveInputWindowTokens != 180_000 {
		t.Fatalf("auto-compact capacity = %d, want 180000", warning.EffectiveInputWindowTokens)
	}
}
