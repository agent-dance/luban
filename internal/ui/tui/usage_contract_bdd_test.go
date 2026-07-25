package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/presentation"
	ui "github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/types"
)

func compactedStatusFixture() *AppState {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.SessionUsageKnown.Set(true)
	state.SessionHasCompacted.Set(true)
	state.SessionCompactionBaselineKnown.Set(true)
	state.SessionTotalInputTokens.Set(4_100)
	state.SessionTotalOutputTokens.Set(310)
	state.SessionTotalCacheReadTokens.Set(1_650)
	state.SessionInputTokensAtCompact.Set(2_500)
	state.SessionCacheReadAtCompact.Set(1_000)
	state.SessionInputTokens.Set(900) // diagnostic only; never a status authority
	state.SessionOutputTokens.Set(70)
	state.CumulativeCost.Set(0.06)
	state.SessionCostKnown.Set(true)
	return state
}

func TestBDDResponsiveWidthsKeepOneAccountingScope(t *testing.T) {
	for _, width := range []int{48, 80, 120, 160, 220, 300} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			state := compactedStatusFixture()
			text := collectElementText(NewRootComponent(state, nil, nil).renderStatusBar(width))
			if strings.Contains(text, "900") || strings.Contains(text, "Req:") || strings.Contains(text, "Last request") {
				t.Fatalf("width %d changed scope to the last request: %q", width, text)
			}
			if strings.Contains(text, "Session:") && !strings.Contains(text, "in 1.6K (4.1K total)") {
				t.Fatalf("width %d partially changed the wide segment scope: %q", width, text)
			}
			if strings.Contains(text, "S:") && !strings.Contains(text, "in 1.6K/4.1K") {
				t.Fatalf("width %d partially changed the narrow segment scope: %q", width, text)
			}
		})
	}
}

func TestBDDAllLanguagesTranslateLabelsWithoutChangingValues(t *testing.T) {
	projection := presentation.SessionUsageProjection{
		Scope: presentation.UsageScopeCumulativeSession, Known: true,
		HasCompacted: true, BaselineKnown: true,
		InputTokens: 1_600, TotalInputTokens: 4_100, OutputTokens: 310,
		CacheReadTokens: 650, TotalCacheRead: 1_650, CacheHitKnown: true, CacheHitPercent: 41,
		CostKnown: true, CostUSD: 0.06,
	}
	for _, lang := range i18n.AllLanguages() {
		wide := ui.FormatSessionUsage(lang, projection)
		narrow := ui.FormatSessionUsageNarrow(lang, projection)
		for _, rendered := range []string{wide, narrow} {
			if !strings.Contains(rendered, "1.6K") || !strings.Contains(rendered, "4.1K") {
				t.Fatalf("language %s changed compact/session values: %q", lang, rendered)
			}
			if strings.Contains(rendered, "900") {
				t.Fatalf("language %s substituted the last request: %q", lang, rendered)
			}
		}
	}
}

func TestBDDLegacyCompactedTUIUsageShowsOnlySessionTotal(t *testing.T) {
	state := compactedStatusFixture()
	state.SessionCompactionBaselineKnown.Set(false)
	state.SessionInputTokensAtCompact.Set(0)
	state.SessionCacheReadAtCompact.Set(0)
	text := collectElementText(NewRootComponent(state, nil, nil).renderStatusBar(300))
	if !strings.Contains(text, "Session: in 4.1K") || strings.Contains(text, "0 (4.1K total)") || strings.Contains(text, "900") {
		t.Fatalf("Then an old session does not fabricate a compact segment: %q", text)
	}
}

func TestBDDContextUsesFullWindowAndMeasurementMarkers(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.UsedTokens.Set(100_000)
	state.MaxTokens.Set(200_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)
	root := NewRootComponent(state, nil, nil)
	text := collectElementText(root.renderStatusBar(300))
	if !strings.Contains(text, "Context: [█████░░░░░] 50% (100.0K/200.0K)") || strings.Contains(text, "Effective") {
		t.Fatalf("Then exact context uses the full model window: %q", text)
	}

	state.UsedTokens.Set(90_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementLocalEstimate)
	text = collectElementText(root.renderStatusBar(300))
	if !strings.Contains(text, "≈45%") {
		t.Fatalf("Then a complete local estimate is marked approximate: %q", text)
	}

	state.UsedTokens.Set(80_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementLocalLowerBound)
	text = collectElementText(root.renderStatusBar(300))
	if !strings.Contains(text, "≥40%") {
		t.Fatalf("Then an incomplete estimate is marked as a lower bound: %q", text)
	}

	state.UsedTokens.Set(64_000)
	state.MaxTokens.Set(128_000) // user override after catalog resolution
	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)
	text = collectElementText(root.renderStatusBar(300))
	if !strings.Contains(text, "50% (64.0K/128.0K)") {
		t.Fatalf("Then the user-overridden complete window is the denominator: %q", text)
	}
}

func TestBDDProviderContextSupersedesLocalEstimate(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.MaxTokens.Set(200_000)
	state.UsedTokens.Set(90_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementLocalEstimate)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	renderer.ModelContextAtEpoch(1, presentation.ModelContextProjection{
		Scope: presentation.UsageScopeModelContext, Known: true,
		UsedTokens: 92_000, CapacityTokens: 200_000, PercentUsed: 46,
		Measurement: presentation.ContextMeasurementProviderReported,
	})
	text := collectElementText(NewRootComponent(state, nil, nil).renderStatusBar(300))
	if !strings.Contains(text, "Context: [████▋░░░░░] 46%") || strings.Contains(text, "≈46%") {
		t.Fatalf("Then provider usage removes the estimate marker: %q", text)
	}
}

func TestBDDManualCompactionImmediatelyLowersContext(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(2)
	state.ContextGeneration.Set(3)
	state.ContextGenerationPersisted.Set(true)
	state.UsedTokens.Set(160_000)
	state.MaxTokens.Set(200_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	ctx := presentation.ToolEventContext{SessionID: "session", SessionEpoch: 2, ContextGeneration: 3, ContextGenerationPersisted: true, TurnID: "turn-4"}
	renderer.CompactionBoundaryAtEpoch(2, ctx, stream.CompactBoundaryEvent{
		Trigger: "manual", PreCompactTokenCount: 160_000,
		PostCompactTokenCount: 50_000, TruePostCompactTokenCount: 40_000,
	})
	text := collectElementText(NewRootComponent(state, nil, nil).renderStatusBar(300))
	if !strings.Contains(text, "≈20% (40.0K/200.0K)") || strings.Contains(text, "80%") {
		t.Fatalf("Then true post-compact tokens replace the old exact value immediately: %q", text)
	}
}

func TestBDDSuccessfulCompactionWithoutCountsDoesNotKeepStaleContext(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(2)
	state.ContextGeneration.Set(3)
	state.ContextGenerationPersisted.Set(true)
	state.UsedTokens.Set(160_000)
	state.MaxTokens.Set(200_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	ctx := presentation.ToolEventContext{SessionID: "session", SessionEpoch: 2, ContextGeneration: 3, ContextGenerationPersisted: true, TurnID: "turn-4"}
	renderer.CompactionBoundaryAtEpoch(2, ctx, stream.CompactBoundaryEvent{BoundaryID: "without-count", Trigger: "manual"})

	text := collectElementText(NewRootComponent(state, nil, nil).renderStatusBar(300))
	if strings.Contains(text, "80%") || state.ContextMeasurement.Get() != presentation.ContextMeasurementUnknown {
		t.Fatalf("Then unavailable post-compact usage is hidden instead of retaining stale exact context: %q", text)
	}
}

func TestBDDTUIDuplicateBoundaryDoesNotResetCurrentSegment(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(2)
	state.ContextGeneration.Set(4)
	state.ContextGenerationPersisted.Set(true)
	state.AccumulateSessionUsage(&types.Usage{InputTokens: 2_500, OutputTokens: 180, CacheReadInputTokens: 1_000})
	state.MaxTokens.Set(200_000)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	ctx := presentation.ToolEventContext{SessionID: "session", SessionEpoch: 2, ContextGeneration: 4, ContextGenerationPersisted: true, TurnID: "turn-3"}
	boundary := stream.CompactBoundaryEvent{Trigger: "manual", PreCompactTokenCount: 2_500, TruePostCompactTokenCount: 400}

	renderer.CompactionBoundaryAtEpoch(2, ctx, boundary)
	state.AccumulateSessionUsage(&types.Usage{InputTokens: 700, OutputTokens: 60, CacheReadInputTokens: 300})
	renderer.CompactionBoundaryAtEpoch(2, ctx, boundary)

	usage := state.ActiveSessionUsage()
	if usage.InputTokensAtCompact != 2_500 || usage.CompactionCount != 1 {
		t.Fatalf("Then duplicate boundary did not remain exactly once: %+v", usage)
	}
	projection, _ := sessionUsageProjection(usage, true)
	if projection.InputTokens != 700 {
		t.Fatalf("Then input accumulated after B remains visible: %+v", projection)
	}
}

func TestBDDDelayedUsageProjectionCannotOverwriteNewerSnapshot(t *testing.T) {
	state := NewAppState()
	newer := presentation.SessionUsageProjection{
		Revision: 2, Known: true, TotalInputTokens: 4_100, OutputTokens: 310,
		HasCompacted: true, BaselineKnown: true, InputAtCompact: 2_500,
		CostKnown: true, CostUSD: 0.06,
	}
	older := presentation.SessionUsageProjection{
		Revision: 1, Known: true, TotalInputTokens: 2_500, OutputTokens: 180,
		CostKnown: true, CostUSD: 0.03,
	}
	if !state.ApplySessionUsageProjection(newer) {
		t.Fatal("Given the newest atomic snapshot was rejected")
	}
	if state.ApplySessionUsageProjection(older) {
		t.Fatal("When a delayed older snapshot was accepted")
	}
	usage := state.ActiveSessionUsage()
	if usage.InputTokens != 4_100 || usage.OutputTokens != 310 || usage.InputTokensAtCompact != 2_500 || state.CumulativeCost.Get() != 0.06 {
		t.Fatalf("Then the UI did not retain one coherent newest revision: %+v cost=%v", usage, state.CumulativeCost.Get())
	}
}
