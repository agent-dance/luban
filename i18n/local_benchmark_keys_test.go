package i18n

import (
	"strings"
	"testing"
)

func TestLocalBenchmarkKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range localBenchmarkKeys {
		for _, language := range AllLanguages() {
			if got := Text(language, key); got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", language.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestLocalBenchmarkAdjudicationCopyRemainsExplicit(t *testing.T) {
	for _, language := range AllLanguages() {
		measured := Format(language, KeyLocalBenchmarkReportMeasuredAdjudicated, 3, 5, 3, 5, 127, 181, "1s", "2s", "$1", "$2")
		if strings.Contains(measured, "%!") || !strings.Contains(measured, "3/5") {
			t.Errorf("Format(%s, adjudicated measured) = %q", language.Code(), measured)
		}
		if got := Text(language, KeyLocalBenchmarkReportStatusAdjudicatedPass); strings.TrimSpace(got) == "" {
			t.Errorf("Text(%s, adjudicated pass) is empty", language.Code())
		}
	}
	english := Text(LangEN, KeyLocalBenchmarkReportAdjudicationNotice)
	for _, fragment := range []string{"skim-rs__skim-1044", "587", "original evaluator failure", "unchanged"} {
		if !strings.Contains(strings.ToLower(english), strings.ToLower(fragment)) {
			t.Errorf("adjudication notice = %q; want fragment %q", english, fragment)
		}
	}
}
