package i18n

import (
	"strings"
	"testing"
)

func TestActivityResultsPendingViewCoversAllLanguages(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyActivityResultsPendingView, 2)
		if got == "" || !strings.Contains(got, "2") || strings.Contains(got, "%!") {
			t.Errorf("Format(%s, KeyActivityResultsPendingView) = %q", lang, got)
		}
	}
}
