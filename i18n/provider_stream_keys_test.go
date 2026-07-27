package i18n

import (
	"strings"
	"testing"
	"time"
)

func TestProviderStreamKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range []Key{KeyProviderStreamInitialIdleTimeout, KeyProviderStreamActiveIdleTimeout} {
		for _, lang := range AllLanguages() {
			got := Format(lang, key, 90*time.Second)
			if got == "" || got == "["+string(key)+"]" || !strings.Contains(got, "1m30s") {
				t.Fatalf("Format(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
