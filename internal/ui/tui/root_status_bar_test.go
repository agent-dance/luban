package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
	gtui "github.com/grindlemire/go-tui"
)

func renderElementText(e *gtui.Element, width, height int) string {
	buf := gtui.NewBuffer(width, height)
	e.Render(buf, width, height)

	var rendered strings.Builder
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := buf.Cell(x, y).Rune
			if r == 0 {
				r = ' '
			}
			rendered.WriteRune(r)
		}
		rendered.WriteByte('\n')
	}
	return rendered.String()
}

func collectElementText(e *gtui.Element) string {
	if e == nil {
		return ""
	}
	var parts []string
	if txt := e.Text(); txt != "" {
		parts = append(parts, txt)
	}
	for _, child := range e.Children() {
		if txt := collectElementText(child); txt != "" {
			parts = append(parts, txt)
		}
	}
	return strings.Join(parts, "\n")
}

func TestFormatSessionUsageSummary(t *testing.T) {
	got := formatSessionUsageSummary(SessionUsage{
		InputTokens: 2500, OutputTokens: 180, CacheReadTokens: 1000, CumulativeCost: 0.0834, Known: true,
		RoundUsageKnown: true, LastInputTokens: 1500, LastOutputTokens: 80, LastCacheReadTokens: 600,
	}, true, i18n.LangEN)
	want := "Session: in 2.5K · 40% cached · out 180 · $0.0834"
	if got != want {
		t.Fatalf("formatSessionUsageSummary() = %q, want %q", got, want)
	}
}

func TestFormatSessionUsageSummaryUsesModelBillingCurrency(t *testing.T) {
	got := formatSessionUsageCompactSummary(SessionUsage{
		InputTokens: 2500, OutputTokens: 180, CacheReadTokens: 1000, CumulativeCost: 0.0834, Known: true,
	}, true, i18n.LangZH, "CNY")
	want := "会话：输入 2.5K · 缓存 40% · 输出 180 · ¥0.08"
	if got != want {
		t.Fatalf("formatSessionUsageCompactSummary() = %q, want %q", got, want)
	}
}

func TestFormatSessionUsageSummary_DoesNotRoundPartialCacheHitToOneHundred(t *testing.T) {
	got := formatSessionUsageSummary(SessionUsage{InputTokens: 2001, OutputTokens: 250, CacheReadTokens: 2000, CumulativeCost: 0.0834, Known: true}, true, i18n.LangEN)
	want := "Session: in 2.0K · 99% cached · out 250 · $0.0834"
	if got != want {
		t.Fatalf("formatSessionUsageSummary() = %q, want %q", got, want)
	}
}

func TestFormatSessionUsageSummary_ShowsOneHundredForExactFullCacheHit(t *testing.T) {
	got := formatSessionUsageSummary(SessionUsage{InputTokens: 2000, OutputTokens: 250, CacheReadTokens: 2000, CumulativeCost: 0.0834, Known: true}, true, i18n.LangEN)
	want := "Session: in 2.0K · 100% cached · out 250 · $0.0834"
	if got != want {
		t.Fatalf("formatSessionUsageSummary() = %q, want %q", got, want)
	}
}

func TestFormatSessionUsageSummaryDoesNotPresentUnknownCostAsZero(t *testing.T) {
	got := formatSessionUsageSummary(SessionUsage{InputTokens: 2000, OutputTokens: 250, CacheReadTokens: 1000, Known: true}, false, i18n.LangEN)
	want := "Session: in 2.0K · 50% cached · out 250 · cost unknown"
	if got != want {
		t.Fatalf("formatSessionUsageSummary() = %q, want %q", got, want)
	}
}

func TestFormatSessionUsageSummaryUsesSessionAverageCacheRateAcrossCompactions(t *testing.T) {
	got := formatSessionUsageSummary(SessionUsage{
		InputTokens: 4700, OutputTokens: 370, CacheReadTokens: 1950, CumulativeCost: 0.12,
		RoundUsageKnown: true, HasCompacted: true, CompactionBaselineKnown: true, CompactionCount: 2,
		InputTokensAtCompact: 4100, CacheReadAtCompact: 1650,
		CompletedRoundInputTokens: 2400, CompletedRoundOutputTokens: 150,
		LastInputTokens: 600, LastOutputTokens: 40, LastCacheReadTokens: 300,
	}, true, i18n.LangEN)
	want := "Session: in 600 (4.7K total) · 50% cached · out 370 · $0.1200"
	if got != want {
		t.Fatalf("formatSessionUsageSummary() = %q, want %q", got, want)
	}
}

func TestFormatSessionUsageSummaryUsesCompactionBaselines(t *testing.T) {
	got := formatSessionUsageSummary(SessionUsage{
		InputTokens: 2500, OutputTokens: 200, CacheReadTokens: 1000, CumulativeCost: 1.25,
		HasCompacted: true, CompactionBaselineKnown: true, InputTokensAtCompact: 1800, CacheReadAtCompact: 700,
	}, true, i18n.LangEN)
	want := "Session: in 700 (2.5K total) · 43% cached · out 200 · $1.2500"
	if got != want {
		t.Fatalf("formatSessionUsageSummary() = %q, want %q", got, want)
	}
}

func TestBDDStatusBarCacheRateUsesDisplayedInputScope(t *testing.T) {
	t.Run("uncompacted session uses cumulative rate", func(t *testing.T) {
		got := formatSessionUsageCompactSummary(SessionUsage{
			InputTokens: 2_500, OutputTokens: 180, CacheReadTokens: 1_000,
			RoundUsageKnown: true, LastInputTokens: 400, LastCacheReadTokens: 100,
		}, true, i18n.LangEN)
		want := "S: in 2.5K · 40% cached · out 180 · $0.00"
		if got != want {
			t.Fatalf("Then session cache rate uses 1000/2500: got %q, want %q", got, want)
		}
	})

	t.Run("compacted session uses current segment rate", func(t *testing.T) {
		got := formatSessionUsageCompactSummary(SessionUsage{
			InputTokens: 4_100, OutputTokens: 310, CacheReadTokens: 1_650,
			HasCompacted: true, CompactionBaselineKnown: true,
			InputTokensAtCompact: 2_500, CacheReadAtCompact: 1_000,
			RoundUsageKnown: true, LastInputTokens: 900, LastCacheReadTokens: 450,
		}, true, i18n.LangEN)
		want := "S: in 1.6K/4.1K · 41% cached · out 310 · $0.00"
		if got != want {
			t.Fatalf("Then compact-segment cache rate uses 650/1600: got %q, want %q", got, want)
		}
	})

	t.Run("new compact segment has no available rate", func(t *testing.T) {
		got := formatSessionUsageCompactSummary(SessionUsage{
			InputTokens: 2_500, OutputTokens: 180, CacheReadTokens: 1_000,
			HasCompacted: true, CompactionBaselineKnown: true,
			InputTokensAtCompact: 2_500, CacheReadAtCompact: 1_000,
		}, true, i18n.LangEN)
		want := "S: in 0/2.5K · out 180 · $0.00"
		if got != want {
			t.Fatalf("Then an empty compact segment omits cache rate: got %q, want %q", got, want)
		}
		if strings.Contains(got, "% cached") {
			t.Fatalf("Then an empty compact segment does not fabricate 0%% cached: %q", got)
		}
	})
}

func TestRenderStatusBar_DefaultStartupUsesAutoWithoutUnknownUsage(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	root := NewRootComponent(state, nil, nil)

	text := collectElementText(root.renderStatusBar(120))
	if !strings.Contains(text, "Auto mode") {
		t.Fatalf("default status bar missing Auto mode: %q", text)
	}
	if strings.Contains(strings.ToLower(text), "usage unknown") {
		t.Fatalf("default status bar exposed unknown usage: %q", text)
	}
	if strings.Contains(text, "Session: in 0") {
		t.Fatalf("default status bar fabricated zero usage: %q", text)
	}
}

func TestRenderStatusBar_OmitsReasoningEffortMovedToBanner(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.ReasoningEffort.Set("high")
	root := NewRootComponent(state, nil, nil)

	text := collectElementText(root.renderStatusBar(120))
	if strings.Contains(text, "🧠") || strings.Contains(text, "High") {
		t.Fatalf("status bar retained reasoning effort after it moved to the banner: %q", text)
	}
}

func TestRenderStatusBar_ShowsSessionSummaryWithoutPaused(t *testing.T) {
	state := NewAppState()
	state.Mode.Set(ModeAskEdit)
	state.ProvStatus.Set(StatusConnected)
	state.CumulativeCost.Set(0.0834)
	state.SessionInputTokens.Set(1000)
	state.SessionOutputTokens.Set(250)
	state.SessionCacheReadTokens.Set(400)
	state.SessionTotalInputTokens.Set(1000)
	state.SessionTotalOutputTokens.Set(250)
	state.SessionTotalCacheReadTokens.Set(400)
	state.SessionTotalCacheCreateTokens.Set(100)
	state.UsedTokens.Set(7100)
	state.MaxTokens.Set(1_050_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)

	root := NewRootComponent(state, nil, nil)
	bar := root.renderStatusBar(120)
	allText := collectElementText(bar)

	if !strings.Contains(allText, "S: in 1.0K · 40% cached · out 250 · $0.08") {
		t.Fatalf("expected session summary in status bar, got %q", allText)
	}
	if !strings.Contains(allText, "●") {
		t.Fatalf("expected connected provider dot in status bar, got %q", allText)
	}
	if strings.Contains(allText, "connected") {
		t.Fatalf("expected connected text to be removed from status bar, got %q", allText)
	}
	if !strings.Contains(allText, "Context: [▏░░░░░░░░░] 1%") {
		t.Fatalf("expected precise context meter in status bar, got %q", allText)
	}
	if strings.ContainsAny(allText, "○◷◶◵◴◎") {
		t.Fatalf("expected context meter instead of obsolete context ring, got %q", allText)
	}
	if strings.Contains(allText, "PAUSED") {
		t.Fatalf("expected PAUSED to be removed from status bar, got %q", allText)
	}
}

func TestRenderStatusBarCostMatchesModelBillingCurrency(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	state.ModelCostCurrency.Set("CNY")
	state.CumulativeCost.Set(0.0834)
	state.SessionTotalInputTokens.Set(1000)
	state.SessionTotalOutputTokens.Set(250)
	state.SessionTotalCacheReadTokens.Set(400)

	text := collectElementText(NewRootComponent(state, nil, nil).renderStatusBar(120))
	if !strings.Contains(text, "会话：输入 1.0K · 缓存 40% · 输出 250 · ¥0.08") || strings.Contains(text, "$0.08") {
		t.Fatalf("status bar cost did not match model currency: %q", text)
	}
}

func TestRenderStatusBar_HidesWholeLowPrioritySegmentsOnNarrowTerminals(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.Mode.Set(ModeAskEdit)
	state.ProvStatus.Set(StatusConnected)
	state.CumulativeCost.Set(0.0834)
	state.SessionTotalInputTokens.Set(10_600)
	state.SessionOutputTokens.Set(250)
	state.SessionTotalCacheReadTokens.Set(10_500)
	state.UsedTokens.Set(7100)
	state.MaxTokens.Set(1_050_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)

	root := NewRootComponent(state, nil, nil)

	wide := root.renderStatusBar(120)
	wide.Render(gtui.NewBuffer(120, 4), 120, 4)
	if got := wide.Rect().Height; got != 1 {
		t.Fatalf("wide status bar height = %d, want 1", got)
	}

	narrow := root.renderStatusBar(48)
	rendered := renderElementText(narrow, 48, 2)
	if got := narrow.Rect().Height; got != 1 {
		t.Fatalf("narrow status bar height = %d, want one priority-budgeted row; rendered:\n%s", got, rendered)
	}
	for _, want := range []string{"Ask mode", "1%"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("narrow status bar missing %q; rendered:\n%s", want, rendered)
		}
	}
	for _, hidden := range []string{"Session:", "cached", "out 250", "$0.0834"} {
		if strings.Contains(rendered, hidden) {
			t.Errorf("narrow status bar partially rendered low-priority segment %q:\n%s", hidden, rendered)
		}
	}
}

func TestRenderStatusBar_NarrowCopyKeepsSessionTotals(t *testing.T) {
	state := NewAppState()
	state.AccumulateSessionUsage(&types.Usage{
		InputTokens:              1_000,
		OutputTokens:             100,
		CacheCreationInputTokens: 1_000,
	})
	state.AccumulateSessionUsage(&types.Usage{
		InputTokens:          99_000,
		OutputTokens:         200,
		CacheReadInputTokens: 99_000,
	})
	state.UsedTokens.Set(20_000)
	state.MaxTokens.Set(100_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)

	root := NewRootComponent(state, nil, nil)
	text := collectElementText(root.renderStatusBar(160))
	for _, want := range []string{
		"20%",
		"S: in 100.0K · 99% cached · out 300 · $0.00",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status bar missing %q: %s", want, text)
		}
	}
}

func TestContextMeter(t *testing.T) {
	tests := []struct {
		pct  float64
		want string
	}{
		{-1, "░░░░░░░░░░"},
		{0, "░░░░░░░░░░"},
		{2, "▎░░░░░░░░░"},
		{25, "██▌░░░░░░░"},
		{47, "████▊░░░░░"},
		{83, "████████▎░"},
		{99.9, "█████████▉"},
		{100, "██████████"},
		{101, "██████████"},
	}

	for _, tt := range tests {
		got := contextMeter(tt.pct, 10)
		if got != tt.want {
			t.Fatalf("contextMeter(%v, 10) = %q, want %q", tt.pct, got, tt.want)
		}
		if width := terminalCellWidth(got); width != 10 {
			t.Fatalf("contextMeter(%v, 10) width = %d, want 10", tt.pct, width)
		}
	}

	if got := contextMeter(50, 0); got != "" {
		t.Fatalf("contextMeter(50, 0) = %q, want empty string", got)
	}
}
