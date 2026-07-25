package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/i18n"
)

func TestBannerShowsReasoningEffortWithoutBuildFingerprint(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	state.Provider.Set("provider-id")
	state.Model.Set("model-id")
	state.ReasoningEffort.Set("high")
	dirty := true
	root := NewRootComponent(state, nil, nil)
	root.build = buildinfo.Diagnostic{
		Fingerprint: buildinfo.Fingerprint{
			Version: "v9.8.7", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Dirty: &dirty, ProcessStart: time.Date(2026, 7, 18, 8, 9, 10, 0, time.Local),
		},
		RepositoryRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		RevisionStatus:     buildinfo.RevisionStale,
	}

	text := collectElementText(root.renderBanner())
	for _, want := range []string{"LUBAN Code", "provider-id/model-id", "[🧠 高]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("banner omitted %q: %q", want, text)
		}
	}
	for _, removed := range []string{"aaaaaaaaaaaa", "HEAD", "脏", "08:09:10", "修订", "推理："} {
		if strings.Contains(text, removed) {
			t.Fatalf("banner retained removed build or reasoning copy %q: %q", removed, text)
		}
	}

	state.Language.Set(i18n.LangEN)
	text = collectElementText(root.renderBanner())
	if !strings.Contains(text, "[🧠 High]") {
		t.Fatalf("banner did not relocalize the reasoning label in the active runtime language: %q", text)
	}
	for _, removed := range []string{"aaaaaaaaaaaa", "HEAD", "dirty", "08:09:10", "Reasoning:"} {
		if strings.Contains(text, removed) {
			t.Fatalf("banner retained removed build or reasoning copy %q: %q", removed, text)
		}
	}
}

func TestBannerOmitsCustomProviderIdentifierPrefix(t *testing.T) {
	state := NewAppState()
	state.Provider.Set("custom-my-gateway")
	state.Model.Set("model-id")

	text := collectElementText(NewRootComponent(state, nil, nil).renderBanner())
	if !strings.Contains(text, "my-gateway/model-id") || strings.Contains(text, "custom-my-gateway") {
		t.Fatalf("banner provider name = %q", text)
	}
}
