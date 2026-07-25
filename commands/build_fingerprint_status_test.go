package commands

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/i18n"
)

func TestStatusIncludesLocalizedCompleteBuildDiagnostic(t *testing.T) {
	dirty := false
	buildTime := time.Date(2026, 7, 17, 1, 2, 3, 0, time.UTC)
	var output string
	ctx := &Context{
		Language:  i18n.LangZH,
		QueryLoop: commandI18NQueryLoop{},
		OnEvent:   func(value string) { output += value },
		BuildDiagnostic: buildinfo.Diagnostic{
			Fingerprint: buildinfo.Fingerprint{
				Version: "v9.8.7", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Dirty: &dirty, BuildTime: &buildTime,
				ProcessStart: time.Date(2026, 7, 18, 4, 5, 6, 0, time.UTC),
				Executable:   "/opt/luban-code",
			},
			RepositoryRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RevisionStatus:     buildinfo.RevisionMatch,
		},
	}
	if err := (&statusCmd{}).Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"构建标识", "v9.8.7", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "干净构建",
		"2026-07-17T01:02:03Z", "2026-07-18T04:05:06Z", "/opt/luban-code", "与 HEAD 匹配",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("status omitted %q:\n%s", want, output)
		}
	}
}

func TestStatusBuildDiagnosticKeepsUnknownDistinctFromMatch(t *testing.T) {
	var output string
	ctx := &Context{
		Language: i18n.LangEN, QueryLoop: commandI18NQueryLoop{},
		OnEvent: func(value string) { output += value },
		BuildDiagnostic: buildinfo.Diagnostic{
			Fingerprint:    buildinfo.Fingerprint{ProcessStart: time.Unix(1, 0)},
			RevisionStatus: buildinfo.RevisionUnknown,
		},
	}
	if err := (&statusCmd{}).Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "matches HEAD") || !strings.Contains(output, "HEAD comparison unknown") {
		t.Fatalf("unknown comparison presented as match:\n%s", output)
	}
}

func TestStatusUsesSessionBillingCurrency(t *testing.T) {
	var output string
	ctx := &Context{
		Language: i18n.LangZH, QueryLoop: commandI18NQueryLoop{},
		TotalCostUSD: 0.125, CostCurrency: "CNY",
		OnEvent: func(value string) { output += value },
	}
	if err := (&statusCmd{}).Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "¥0.1250") || strings.Contains(output, "$0.1250") {
		t.Fatalf("status cost did not use session currency:\n%s", output)
	}
}
