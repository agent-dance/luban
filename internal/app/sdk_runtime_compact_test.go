package app

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-dance/luban/internal/runtime/engine"
)

type sdkCompactProjectionEngine struct {
	engine.Engine
	result engine.CompactResult
	err    error
}

func (e *sdkCompactProjectionEngine) Compact(context.Context, string, ...string) (engine.CompactResult, error) {
	return e.result, e.err
}

func TestSDKRuntimeCompactProjectsAuthoritativeEngineResult(t *testing.T) {
	engineResult := engine.CompactResult{
		Compacted: true, BeforeMessageCount: 28, AfterMessageCount: 6, ContextGeneration: 9,
	}
	runtime := newSDKRuntime(&sdkCompactProjectionEngine{result: engineResult})
	result, err := runtime.Compact(context.Background(), "compact-session")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compacted || result.BeforeMessageCount != engineResult.BeforeMessageCount ||
		result.AfterMessageCount != engineResult.AfterMessageCount || result.ContextGeneration != engineResult.ContextGeneration {
		t.Fatalf("SDK compact projection = %+v, engine result = %+v", result, engineResult)
	}
}

func TestSDKRuntimeCompactErrorCannotExposeSpeculativeResult(t *testing.T) {
	cause := errors.New("private compact failure")
	runtime := newSDKRuntime(&sdkCompactProjectionEngine{
		result: engine.CompactResult{
			Compacted: true, BeforeMessageCount: 28, AfterMessageCount: 6, ContextGeneration: 9,
		},
		err: cause,
	})
	result, err := runtime.Compact(context.Background(), "compact-session")
	if err == nil {
		t.Fatal("Compact unexpectedly succeeded")
	}
	if result.Compacted || result.BeforeMessageCount != 0 || result.AfterMessageCount != 0 || result.ContextGeneration != 0 {
		t.Fatalf("failed compact exposed speculative result: %+v", result)
	}
}
