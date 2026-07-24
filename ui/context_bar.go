package ui

import (
	"fmt"
	"strings"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/terminaltheme"
	"github.com/charmbracelet/lipgloss"
)

func contextBarStyles() (success, warning, danger lipgloss.Style) {
	p := terminaltheme.Current()
	return lipgloss.NewStyle().Foreground(lipgloss.Color(p.Success)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(p.Warning)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(p.Danger))
}

// FormatContextBar returns a visual bar representing context-window utilisation.
//
// Example output:
//
//	[████████████░░░░░░░░ 60% (120K/200K)]
//
// The bar is 20 cells wide. The colour reflects remaining context buffer.
//
// Token counts are formatted as whole-number K values (e.g. "134K").
func FormatContextBar(usedTokens, maxTokens int) string {
	state := compact.TokenWarningState{
		UsedTokens:                 usedTokens,
		EffectiveInputWindowTokens: maxTokens,
		ThresholdTokens:            maxTokens,
		PercentLeft:                percentLeft(usedTokens, maxTokens),
		IsAboveWarningThreshold:    maxTokens > 0 && usedTokens >= maxTokens-compact.WarningThresholdBufferTokens,
		IsAboveErrorThreshold:      maxTokens > 0 && usedTokens >= maxTokens-compact.ManualCompactBufferTokens,
		IsAtBlockingLimit:          maxTokens > 0 && usedTokens >= maxTokens-compact.ManualCompactBufferTokens,
	}
	return FormatContextBarState(state)
}

// FormatContextBarState returns a context bar whose severity is driven by the
// compact warning state rather than fixed percentage cutoffs.
func FormatContextBarState(state compact.TokenWarningState) string {
	const barWidth = 20

	pct := 0.0
	maxTokens := state.EffectiveInputWindowTokens
	usedTokens := state.UsedTokens
	if maxTokens > 0 {
		pct = float64(usedTokens) / float64(maxTokens)
		if pct > 1.0 {
			pct = 1.0
		}
	}

	filled := int(pct * barWidth)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	success, warning, danger := contextBarStyles()
	var style lipgloss.Style
	switch {
	case state.IsAboveErrorThreshold || state.IsAtBlockingLimit:
		style = danger
	case state.IsAboveWarningThreshold || state.IsAboveAutoCompactThreshold:
		style = warning
	default:
		style = success
	}

	coloredBar := style.Render(bar)
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyPresentationContextBar,
		coloredBar,
		int(pct*100),
		fmtTokensK(usedTokens),
		fmtTokensK(maxTokens),
	)
}

func percentLeft(usedTokens, maxTokens int) int {
	if maxTokens <= 0 {
		return 0
	}
	left := maxTokens - usedTokens
	if left < 0 {
		left = 0
	}
	return int(float64(left) / float64(maxTokens) * 100)
}

// fmtTokensK formats a token count as a whole-number K string (e.g. "134K").
// Values below 1000 are rendered as plain integers.
func fmtTokensK(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%dK", n/1000)
	}
	return fmt.Sprintf("%d", n)
}
