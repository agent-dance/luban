package i18n

import (
	"strings"
	"testing"
)

func TestToolShellImagePlaceholderCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyToolShellImagePlaceholder, "image/png", 12)
		if strings.TrimSpace(got) == "" || !strings.Contains(got, "image/png") || !strings.Contains(got, "12") {
			t.Fatalf("image placeholder for %s lost typed parameters: %q", lang.Code(), got)
		}
	}
}
