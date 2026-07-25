package app

import (
	"testing"

	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/engine"
)

type contextUsageProjectionEngine struct {
	engine.Engine
	info *engine.ContextUsageInfo
}

func (e *contextUsageProjectionEngine) ContextUsage(string) (*engine.ContextUsageInfo, error) {
	return e.info, nil
}

func TestEngineQueryLooperHidesContextUntilProviderReportsUsage(t *testing.T) {
	eng := &contextUsageProjectionEngine{info: &engine.ContextUsageInfo{
		TotalTokens: 353400,
		Measurement: string(compact.ContextUsageUnknown),
	}}
	looper := &engineQueryLooper{eng: eng, sessionID: func() string { return "fresh-session" }}

	maxTokens, usage := looper.ContextUsageDetail()
	if maxTokens != 353400 || usage.Measurement != compact.ContextUsageUnknown {
		t.Fatalf("detail = max %d, usage %+v", maxTokens, usage)
	}
	if maxTokens, usedTokens := looper.ContextUsage(); maxTokens != 0 || usedTokens != 0 {
		t.Fatalf("display usage before provider report = %d/%d, want unavailable", usedTokens, maxTokens)
	}
}

func TestEngineQueryLooperProjectsProviderReportedContextExactly(t *testing.T) {
	eng := &contextUsageProjectionEngine{info: &engine.ContextUsageInfo{
		TotalTokens: 353400,
		UsedTokens:  5797,
		Measurement: string(compact.ContextUsageProviderReported),
	}}
	looper := &engineQueryLooper{eng: eng, sessionID: func() string { return "reported-session" }}

	maxTokens, usage := looper.ContextUsageDetail()
	if maxTokens != 353400 || usage.UsedTokens != 5797 || usage.Measurement != compact.ContextUsageProviderReported {
		t.Fatalf("detail = max %d, usage %+v", maxTokens, usage)
	}
	if maxTokens, usedTokens := looper.ContextUsage(); maxTokens != 353400 || usedTokens != 5797 {
		t.Fatalf("display usage = %d/%d, want 5797/353400", usedTokens, maxTokens)
	}
}
