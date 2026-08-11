package compact

import "testing"

func TestProgressiveConfigFailClosedControls(t *testing.T) {
	config := DefaultProgressiveConfig()
	config.Enabled = true
	config.ProviderAllowlist = []string{"openai-responses"}
	config.ModelAllowlist = []string{"gpt-5.6-sol"}
	if !ProgressiveEnabledForSession(config, "openai-responses", "gpt-5.6-sol-20260801", "session") {
		t.Fatal("reviewed provider/model was not enabled")
	}
	if ProgressiveEnabledForSession(config, "anthropic", "gpt-5.6-sol", "session") ||
		ProgressiveEnabledForSession(config, "openai-responses", "unknown", "session") {
		t.Fatal("allowlist admitted an unreviewed provider/model")
	}
	config.KillSwitch = true
	if ProgressiveEnabledForSession(config, "openai-responses", "gpt-5.6-sol", "session") {
		t.Fatal("kill switch did not fail closed")
	}
}

func TestProgressiveRolloutAssignmentIsStable(t *testing.T) {
	config := DefaultProgressiveConfig()
	config.Enabled = true
	config.RolloutPercent = 37
	first := ProgressiveEnabledForSession(config, "provider", "model", "stable-session")
	for range 20 {
		if got := ProgressiveEnabledForSession(config, "provider", "model", "stable-session"); got != first {
			t.Fatal("rollout assignment changed for the same session")
		}
	}
}

func TestProgressiveConfigDefaultsTwoRequestCacheRecovery(t *testing.T) {
	config := NormalizeProgressiveConfig(ProgressiveConfig{})
	if config.CacheRecoveryRequests != 2 || config.ReuseHorizon != 3 || config.MinTokenSavings != 2_000 {
		t.Fatalf("cost gate defaults = %+v", config)
	}
}

func TestProductionProgressiveConfigAdmitsOnlyReviewedScope(t *testing.T) {
	config := ProductionProgressiveConfig()
	if !ProgressiveEnabledForSession(config, "openai", "gpt-5.6-sol-20260801", "session") ||
		!ProgressiveEnabledForSession(config, "deepseek", "deepseek-v4-flash", "session") ||
		!ProgressiveToolEnabled(config, "Inspect") {
		t.Fatalf("reviewed production scope is disabled: %+v", config)
	}
	if ProgressiveEnabledForSession(config, "benchmark-meter", "gpt-5.6-sol", "session") ||
		ProgressiveEnabledForSession(config, "openai", "gpt-5.7", "session") ||
		ProgressiveEnabledForSession(config, "openai", "deepseek-v4-flash", "session") ||
		ProgressiveEnabledForSession(config, "deepseek", "gpt-5.6-sol", "session") ||
		ProgressiveImminentCompactCounterfactualEnabled(config, "openai") ||
		ProgressiveToolEnabled(config, "Run") || ProgressiveToolEnabled(config, "ApplyPatch") {
		t.Fatalf("production policy admitted an unreviewed scope: %+v", config)
	}
	if !ProgressiveImminentCompactCounterfactualEnabled(config, "deepseek") || config.AutoCompactKeepRecent != 1 ||
		config.AutoCompactMaxGrowthTokens != 4_000 || config.AutoCompactMinThresholdPercent != 100 ||
		!config.RequireConsumedMutation || !config.FlattenCompactInput || !config.ConciseCompactSummary || config.CompactMaxOutputTokens != 4_000 {
		t.Fatalf("DeepSeek production policy is incomplete: %+v", config)
	}
}

func TestProgressiveImminentCompactCounterfactualIsExplicitAndProviderScoped(t *testing.T) {
	config := DefaultProgressiveConfig()
	if ProgressiveImminentCompactCounterfactualEnabled(config, "deepseek") {
		t.Fatal("empty counterfactual allowlist must fail closed")
	}
	config.ImminentCompactProviderAllowlist = []string{" DeepSeek ", "deepseek"}
	config.AutoCompactKeepRecent = 8
	config.AutoCompactMaxGrowthTokens = 8_000
	config.AutoCompactMinThresholdPercent = 90
	config = NormalizeProgressiveConfig(config)
	if len(config.ImminentCompactProviderAllowlist) != 1 ||
		!ProgressiveImminentCompactCounterfactualEnabled(config, "deepseek") ||
		ProgressiveImminentCompactCounterfactualEnabled(config, "openai") {
		t.Fatalf("provider-scoped counterfactual = %+v", config.ImminentCompactProviderAllowlist)
	}
	if config.AutoCompactKeepRecent != 8 {
		t.Fatalf("auto compact keep recent = %d, want 8", config.AutoCompactKeepRecent)
	}
	if config.AutoCompactMaxGrowthTokens != 8_000 || config.AutoCompactMinThresholdPercent != 90 {
		t.Fatalf("provider-scoped auto compact policy = %+v", config)
	}
}

func TestProgressiveAutoCompactPolicyNormalizesFailClosed(t *testing.T) {
	config := NormalizeProgressiveConfig(ProgressiveConfig{
		AutoCompactKeepRecent:          -1,
		AutoCompactMaxGrowthTokens:     -1,
		AutoCompactMinThresholdPercent: 101,
		CompactMaxOutputTokens:         CompactMaxOutputTokens + 1,
	})
	if config.AutoCompactKeepRecent != 0 || config.AutoCompactMaxGrowthTokens != 0 || config.AutoCompactMinThresholdPercent != 100 || config.CompactMaxOutputTokens != CompactMaxOutputTokens {
		t.Fatalf("normalized auto compact policy = %+v", config)
	}
	production := ProductionProgressiveConfig()
	if ProgressiveImminentCompactCounterfactualEnabled(production, "openai") {
		t.Fatalf("GPT production policy unexpectedly enabled DeepSeek compact options: %+v", production)
	}
}

func TestProgressiveProviderCompactPolicyRequiresFullPairedScope(t *testing.T) {
	config := ProductionProgressiveConfig()
	if !ProgressiveProviderCompactPolicyEnabled(config, "deepseek", "deepseek-v4-flash", "session") {
		t.Fatal("reviewed DeepSeek compact policy is disabled")
	}
	if ProgressiveProviderCompactPolicyEnabled(config, "openai", "gpt-5.6-sol", "session") ||
		ProgressiveProviderCompactPolicyEnabled(config, "deepseek", "gpt-5.6-sol", "session") {
		t.Fatal("provider compact policy escaped its paired scope")
	}
	config.Enabled = false
	if ProgressiveProviderCompactPolicyEnabled(config, "deepseek", "deepseek-v4-flash", "session") {
		t.Fatal("explicit progressive disable retained provider compact behavior")
	}
}
