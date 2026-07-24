package compact

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestCalculateTokenWarningStateAutoEnabledThresholds(t *testing.T) {
	state := CalculateTokenWarningState(TokenWarningOptions{
		MaxTokens:          100_000,
		MaxOutputTokens:    10_000,
		TokenUsage:         57_000,
		AutoCompactEnabled: true,
	})

	if state.EffectiveInputWindowTokens != 90_000 {
		t.Fatalf("effective window = %d, want 90000", state.EffectiveInputWindowTokens)
	}
	if state.AutoCompactThresholdTokens != 77_000 {
		t.Fatalf("auto threshold = %d, want 77000", state.AutoCompactThresholdTokens)
	}
	if !state.IsAboveWarningThreshold {
		t.Fatal("expected warning threshold at auto threshold minus warning buffer")
	}
	if state.IsAboveAutoCompactThreshold {
		t.Fatal("did not expect auto-compact threshold before 77000 tokens")
	}
	if state.PercentLeft != 26 {
		t.Fatalf("percent left = %d, want 26", state.PercentLeft)
	}
}

func TestCalculateTokenWarningStateAutoCompactAndError(t *testing.T) {
	state := CalculateTokenWarningState(TokenWarningOptions{
		MaxTokens:          100_000,
		MaxOutputTokens:    10_000,
		TokenUsage:         77_000,
		AutoCompactEnabled: true,
	})
	if !state.IsAboveAutoCompactThreshold {
		t.Fatal("expected auto-compact threshold")
	}
	if !state.IsAboveErrorThreshold {
		t.Fatal("expected error threshold")
	}
	if state.PercentLeft != 0 {
		t.Fatalf("percent left = %d, want 0", state.PercentLeft)
	}
}

func TestCalculateTokenWarningStateAutoDisabledUsesManualReserve(t *testing.T) {
	state := CalculateTokenWarningState(TokenWarningOptions{
		MaxTokens:          100_000,
		MaxOutputTokens:    10_000,
		TokenUsage:         87_000,
		AutoCompactEnabled: false,
	})
	if state.ThresholdTokens != 90_000 {
		t.Fatalf("threshold = %d, want effective input window 90000", state.ThresholdTokens)
	}
	if state.IsAboveAutoCompactThreshold {
		t.Fatal("auto-compact threshold should be false when auto compact is disabled")
	}
	if !state.IsAtBlockingLimit {
		t.Fatal("expected blocking at effective window minus manual compact buffer")
	}
}

func TestCalculateTokenWarningStateBlockingLimitOverride(t *testing.T) {
	t.Setenv("CLAUDE_CODE_BLOCKING_LIMIT_OVERRIDE", "42")
	state := CalculateTokenWarningState(TokenWarningOptions{
		MaxTokens:          100_000,
		TokenUsage:         42,
		AutoCompactEnabled: false,
	})
	if state.BlockingLimitTokens != 42 {
		t.Fatalf("blocking limit = %d, want override 42", state.BlockingLimitTokens)
	}
	if !state.IsAtBlockingLimit {
		t.Fatal("expected override blocking limit to apply")
	}
}

func TestCalculateTokenWarningStateSuppressionHidesWarningsButNotBlocking(t *testing.T) {
	state := CalculateTokenWarningState(TokenWarningOptions{
		MaxTokens:          100_000,
		MaxOutputTokens:    10_000,
		TokenUsage:         87_000,
		AutoCompactEnabled: false,
		SuppressWarning:    true,
	})
	if state.IsAboveWarningThreshold || state.IsAboveErrorThreshold || state.IsAboveAutoCompactThreshold {
		t.Fatalf("warnings were not suppressed: %+v", state)
	}
	if !state.IsAtBlockingLimit {
		t.Fatal("blocking must not be suppressed")
	}
}

func TestContextWindowPostCompactSuppressionClearsOnUsageUpdate(t *testing.T) {
	cw := NewContextWindow(100_000)
	cw.MaxOutputTokens = 10_000
	cw.RecordCompactSuccess()
	if state := cw.TokenWarningState(87_000, false); !state.WarningSuppressed || state.IsAboveWarningThreshold {
		t.Fatalf("post-compact warning not suppressed: %+v", state)
	}

	cw.UpdateUsage(&types.Usage{InputTokens: 87_000})
	if state := cw.TokenWarningState(-1, false); state.WarningSuppressed || !state.IsAboveWarningThreshold {
		t.Fatalf("usage update did not restore warnings: %+v", state)
	}
}

func TestContextWindowMicrocompactSuppression(t *testing.T) {
	cw := NewContextWindow(100_000)
	cw.MaxOutputTokens = 10_000
	cw.RecordMicrocompactSuccess()
	if state := cw.TokenWarningState(77_000, true); !state.WarningSuppressed || state.IsAboveAutoCompactThreshold {
		t.Fatalf("microcompact warning not suppressed: %+v", state)
	}
}
