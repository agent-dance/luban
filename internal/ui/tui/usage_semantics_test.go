package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
)

func TestInitialCatalogContextUsesCompleteModelWindow(t *testing.T) {
	info := provider.ModelInfo{
		Provider: "fixture", ID: "model", ContextWindow: 200_000, MaxOutput: 16_384,
	}
	if got, want := modelContextCapacity(info), 200_000; got != want {
		t.Fatalf("initial context capacity = %d, want %d", got, want)
	}
}

func TestIncidentFixtureLabelsLastSessionAndEffectiveContextWithoutMixing(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.SessionUsageKnown.Set(true)
	state.SessionRoundUsageKnown.Set(true)
	state.SessionInputTokens.Set(136_081)
	state.SessionOutputTokens.Set(146)
	state.SessionCacheReadTokens.Set(131_999)
	state.SessionTotalInputTokens.Set(136_081)
	state.SessionTotalOutputTokens.Set(146)
	state.SessionTotalCacheReadTokens.Set(131_999)
	state.CumulativeCost.Set(16.6019)
	state.UsedTokens.Set(560_000)
	state.MaxTokens.Set(1_000_000)
	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)

	text := collectElementText(NewRootComponent(state, nil, nil).renderStatusBar(420))
	for _, want := range []string{
		"Session total: in 136.1K · 97% cached · out 146 · $16.6019",
		"Context: [█████▋░░░░] 56% (560.0K/1000.0K)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("incident fixture missing %q in %q", want, text)
		}
	}
}

func TestContextStaysHiddenUntilProviderUsageIsKnown(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.UsedTokens.Set(56_000)
	state.MaxTokens.Set(100_000)
	text := collectElementText(NewRootComponent(state, nil, nil).renderStatusBar(300))
	if strings.Contains(text, "Context:") {
		t.Fatalf("unknown context was displayed: %q", text)
	}

	state.ContextMeasurement.Set(presentation.ContextMeasurementProviderReported)
	text = collectElementText(NewRootComponent(state, nil, nil).renderStatusBar(300))
	if !strings.Contains(text, "Context: [") || !strings.Contains(text, "56%") {
		t.Fatalf("provider context was not displayed: %q", text)
	}
}
