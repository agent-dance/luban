package i18n

import (
	"fmt"
	"strings"
	"testing"
)

func TestAgenticReportSemanticCopyCoversEveryLanguage(t *testing.T) {
	const expectedKeys = 183
	if got := len(agenticReportKeys); got != expectedKeys {
		t.Fatalf("agentic report key count = %d, want %d", got, expectedKeys)
	}

	seen := make(map[Key]struct{}, len(agenticReportKeys))
	for _, key := range agenticReportKeys {
		if !strings.HasPrefix(string(key), "agentic.report.") {
			t.Errorf("agentic report key %q has an unexpected namespace", key)
		}
		if _, duplicate := seen[key]; duplicate {
			t.Errorf("agentic report key %q is duplicated", key)
		}
		seen[key] = struct{}{}

		translations, registered := semanticTranslations[key]
		if !registered {
			t.Errorf("agentic report key %q is not registered", key)
			continue
		}
		for _, lang := range AllLanguages() {
			got := translations[lang]
			if strings.TrimSpace(got) == "" || strings.HasPrefix(got, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}

	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("ValidateSemanticCatalog() error = %v", err)
	}
}

func TestAgenticReportFormattedCopyPreservesParameters(t *testing.T) {
	for _, lang := range AllLanguages() {
		path := "/tmp/agentic-report.html"
		if got := Format(lang, KeyAgenticReportCLISuccess, path); !strings.Contains(got, path) {
			t.Errorf("Format(%s, success) = %q; want path", lang.Code(), got)
		}

		cause := fmt.Errorf("sentinel-cause")
		if got := Format(lang, KeyAgenticReportCLIError, cause); !strings.Contains(got, cause.Error()) {
			t.Errorf("Format(%s, error) = %q; want cause", lang.Code(), got)
		}

		asOf := "2026-07-26T00:00:00Z"
		if got := Format(lang, KeyAgenticReportFooter, asOf); !strings.Contains(got, asOf) {
			t.Errorf("Format(%s, footer) = %q; want as-of timestamp", lang.Code(), got)
		}

		method := "paired-task-cluster-bootstrap-v1"
		confidence := "95%"
		if got := Format(lang, KeyAgenticReportStatisticsSummary, method, confidence, 10_000, 42); !strings.Contains(got, method) || !strings.Contains(got, confidence) || !strings.Contains(got, "10000") || !strings.Contains(got, "42") {
			t.Errorf("Format(%s, statistics summary) = %q; want every parameter", lang.Code(), got)
		}

		if got := Format(lang, KeyAgenticReportPairedSummary, 113, 226, 80, 31, 2); !strings.Contains(got, "113") || !strings.Contains(got, "226") || !strings.Contains(got, "80") || !strings.Contains(got, "31") || !strings.Contains(got, "2") {
			t.Errorf("Format(%s, paired summary) = %q; want every parameter", lang.Code(), got)
		}
		if got := Format(lang, KeyAgenticReportCostKnownLowerBound, "$1.25", 8, 9, 1, 7, 8); !strings.Contains(got, "$1.25") || !strings.Contains(got, "9") || !strings.Contains(got, "7") {
			t.Errorf("Format(%s, known cost lower bound) = %q; want every parameter", lang.Code(), got)
		}
		if got := Format(lang, KeyAgenticReportMethodStorageDeclaration, 20480); !strings.Contains(got, "20480") {
			t.Errorf("Format(%s, storage declaration) = %q; want storage", lang.Code(), got)
		}
	}
}

func TestAgenticReportProviderCostUnavailableCopy(t *testing.T) {
	const want = "N/A — gateway does not emit per-response cost; excluded from verdict"
	if got := Text(LangEN, KeyAgenticReportCostProviderNotAvailable); got != want {
		t.Fatalf("provider unavailable copy = %q, want %q", got, want)
	}
	for _, lang := range AllLanguages() {
		if got := strings.TrimSpace(Text(lang, KeyAgenticReportCostProviderNotAvailable)); got == "" {
			t.Errorf("provider unavailable copy is empty for %s", lang.Code())
		}
	}
}

func TestAgenticReportUsesTaskDurationTerminology(t *testing.T) {
	if got := Text(LangZH, KeyAgenticReportHeaderWallTime); got != "任务耗时" {
		t.Fatalf("task duration header = %q, want 任务耗时", got)
	}
	if got := Text(LangZH, KeyAgenticReportMetricWallTimeSeconds); got != "任务耗时" {
		t.Fatalf("task duration metric = %q, want 任务耗时", got)
	}
	for _, language := range AllLanguages() {
		if got := strings.ToLower(Text(language, KeyAgenticReportHeaderWallTime)); strings.Contains(got, "wall time") {
			t.Errorf("task duration header still uses wall-time terminology for %s: %q", language.Code(), got)
		}
	}
}
