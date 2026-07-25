package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestTermRendererBannerUsesLUBANCodeBrand(t *testing.T) {
	var buf bytes.Buffer
	r := NewTermRenderer(&buf)

	r.Banner("deepseek", "deepseek-v4-flash")

	out := buf.String()
	for _, want := range []string{"█▀█", "LUBAN Code", i18n.Text(i18n.LangEN, i18n.KeyBrandTagline), "deepseek", "deepseek-v4-flash"} {
		if !strings.Contains(out, want) {
			t.Fatalf("banner missing %q:\n%s", want, out)
		}
	}
}

func TestTermRendererBannerUsesActiveLanguageAndPreservesIdentifiers(t *testing.T) {
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.SaveLanguage(i18n.LangEN) })

	var buf bytes.Buffer
	NewTermRenderer(&buf).Banner("provider-id", "model-id")
	out := buf.String()
	for _, want := range []string{i18n.Text(i18n.LangZH, i18n.KeyBrandTagline), "provider-id", "model-id"} {
		if !strings.Contains(out, want) {
			t.Fatalf("localized banner missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, i18n.Text(i18n.LangEN, i18n.KeyBrandTagline)) {
		t.Fatalf("localized banner retained the English tagline:\n%s", out)
	}
}

func TestTermRendererRuntimeErrorUsesStrictUserProjection(t *testing.T) {
	var output bytes.Buffer
	renderer := NewTermRenderer(&output)
	secret := "/Users/private/.aws/credentials token=sk-live-terminal-secret\x1b[2J"
	renderer.RuntimeErrorEvent(presentation.ToolEventContext{
		SessionID: "private-session", ProjectRoot: "/private/project", TurnID: "private-turn",
		ActorID: "private-actor", WorkUnitID: "private-work",
	}, "private-tool", secret, &types.APIError{Type: "private-provider-code", Message: secret}, map[string]any{"authorization": "Bearer private-token"})

	got := output.String()
	if !strings.Contains(got, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary)) {
		t.Fatalf("terminal runtime error omitted strict user projection: %q", got)
	}
	for _, private := range []string{secret, "sk-live-terminal-secret", "private-provider-code", "private-token", "private-session", "private-tool", "private-project", "\x1b[2J"} {
		if strings.Contains(got, private) {
			t.Fatalf("terminal runtime-error projection leaked %q: %q", private, got)
		}
	}
}

func TestTermRendererToolResultRejectsMissingOutcome(t *testing.T) {
	var output bytes.Buffer
	renderer := NewTermRenderer(&output)
	renderer.RenderToolResult(presentation.ToolEventContext{}, types.ToolResultBlock{
		ToolUseID: "tool-missing-outcome", Content: "private untyped result",
	})

	got := output.String()
	if !strings.Contains(got, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary)) {
		t.Fatalf("terminal omitted semantic invariant failure: %q", got)
	}
	if strings.Contains(got, "private untyped result") {
		t.Fatalf("terminal emitted missing-outcome result: %q", got)
	}
}

func TestTermRendererCacheBreakDebugDefaultSilent(t *testing.T) {
	t.Setenv("LUBAN_CODE_CACHE_BREAK_DEBUG", "")
	t.Setenv("PROMPT_CACHE_BREAK_DEBUG", "")
	var buf bytes.Buffer
	r := NewTermRenderer(&buf)

	r.Usage(&types.Usage{InputTokens: 120000, CacheReadInputTokens: 100000})
	r.Usage(&types.Usage{InputTokens: 120000, CacheReadInputTokens: 1000, CacheCreationInputTokens: 90000})

	out := buf.String()
	if strings.Contains(out, "cache debug") || strings.Contains(out, "cache break") {
		t.Fatalf("cache debug should be silent by default:\n%s", out)
	}
	if !strings.Contains(out, "[cache:") {
		t.Fatalf("existing cache usage line should still render:\n%s", out)
	}
}

func TestTermRendererCostSummaryUsesBillingCurrency(t *testing.T) {
	var output bytes.Buffer
	NewTermRenderer(&output).CostSummaryInCurrency(0.003, 0.125, "CNY", 1200, 450)
	got := output.String()
	if !strings.Contains(got, "¥0.0030") || !strings.Contains(got, "¥0.1250") || strings.Contains(got, "$0.1250") {
		t.Fatalf("terminal cost summary did not use billing currency: %q", got)
	}
}

func TestTermRendererCacheBreakDebugEnabled(t *testing.T) {
	t.Setenv("LUBAN_CODE_CACHE_BREAK_DEBUG", "1")
	var buf bytes.Buffer
	r := NewTermRenderer(&buf)

	r.Usage(&types.Usage{InputTokens: 120000, CacheReadInputTokens: 100000})
	r.Usage(&types.Usage{InputTokens: 120000, CacheReadInputTokens: 1000, CacheCreationInputTokens: 90000})

	out := buf.String()
	if !strings.Contains(out, "cache debug: possible cache break") {
		t.Fatalf("expected opt-in cache debug line:\n%s", out)
	}
	if !strings.Contains(out, "[cache:") {
		t.Fatalf("existing cache usage line should still render:\n%s", out)
	}
}
