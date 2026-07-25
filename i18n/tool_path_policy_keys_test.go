package i18n

import (
	"strings"
	"testing"
)

func TestToolPathOutsideAllowedCoversEveryLanguage(t *testing.T) {
	const rawPath = "/outside/raw-value.txt"
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyToolPathOutsideAllowed, rawPath)
		if strings.TrimSpace(got) == "" || !strings.Contains(got, rawPath) {
			t.Fatalf("path policy copy for %s lost raw path: %q", lang.Code(), got)
		}
	}
}
