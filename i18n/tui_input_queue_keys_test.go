package i18n

import (
	"strings"
	"testing"
)

func TestTUIInputQueueSemanticCatalog(t *testing.T) {
	keys := []Key{KeyTUIInputQueuedStatus, KeyTUIInputQueuedAsGuidance, KeyTUIExitConfirm}
	for _, lang := range AllLanguages() {
		for _, key := range keys {
			if got := strings.TrimSpace(Text(lang, key)); got == "" || got == string(key) {
				t.Fatalf("%s translation for %s = %q", lang.Code(), key, got)
			}
		}
	}
	if got := Format(LangZH, KeyTUIInputQueuedStatus, 2); !strings.Contains(got, "2") || !strings.Contains(got, "Esc") {
		t.Fatalf("Chinese queued-input status = %q", got)
	}
}
