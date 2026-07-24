package ui

import (
	"strings"
	"testing"
)

func TestFormatContextBar_Zero(t *testing.T) {
	bar := FormatContextBar(0, 200_000)

	// 0% filled — all empty blocks
	if strings.Contains(bar, "█") {
		t.Errorf("0%% bar should have no filled blocks, got: %q", bar)
	}
	if !strings.Contains(bar, "░") {
		t.Errorf("0%% bar should contain empty blocks, got: %q", bar)
	}
	if !strings.Contains(bar, "0%") {
		t.Errorf("0%% bar should show 0%%, got: %q", bar)
	}
}

func TestFormatContextBar_Fifty(t *testing.T) {
	bar := FormatContextBar(100_000, 200_000)

	if !strings.Contains(bar, "50%") {
		t.Errorf("50%% bar should show 50%%, got: %q", bar)
	}
	if !strings.Contains(bar, "100K") {
		t.Errorf("50%% bar should show '100K' used, got: %q", bar)
	}
	if !strings.Contains(bar, "200K") {
		t.Errorf("50%% bar should show '200K' max, got: %q", bar)
	}
	// At exactly 50% the bar switches to yellow — verify no red style codes
	// by checking the yellow ANSI color (code 3) is present.
	// Lipgloss emits ANSI when output is a terminal; in tests it may render
	// plain text. We only check structural content.
	if !strings.Contains(bar, "█") {
		t.Errorf("50%% bar should have some filled blocks, got: %q", bar)
	}
}

func TestFormatContextBar_NinetyFive(t *testing.T) {
	bar := FormatContextBar(190_000, 200_000)

	if !strings.Contains(bar, "95%") {
		t.Errorf("95%% bar should show 95%%, got: %q", bar)
	}
	// Nearly full — all 20 cells should be filled blocks
	filledCount := strings.Count(bar, "█")
	if filledCount < 18 {
		t.Errorf("95%% bar: expected ≥18 filled blocks, got %d in: %q", filledCount, bar)
	}
}

func TestFormatContextBar_Full(t *testing.T) {
	bar := FormatContextBar(200_000, 200_000)

	if !strings.Contains(bar, "100%") {
		t.Errorf("100%% bar should show 100%%, got: %q", bar)
	}
	// Should have no empty blocks
	if strings.Contains(bar, "░") {
		t.Errorf("100%% bar should have no empty blocks, got: %q", bar)
	}
}

func TestFormatContextBar_ZeroMax(t *testing.T) {
	// Zero max should not panic and should render 0%
	bar := FormatContextBar(0, 0)
	if !strings.Contains(bar, "0%") {
		t.Errorf("zero-max bar should show 0%%, got: %q", bar)
	}
}

func TestFmtTokensK(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1K"},
		{1500, "1K"},
		{134000, "134K"},
		{200000, "200K"},
	}
	for _, tc := range cases {
		got := fmtTokensK(tc.n)
		if got != tc.want {
			t.Errorf("fmtTokensK(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
