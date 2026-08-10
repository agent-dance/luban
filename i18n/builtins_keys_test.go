package i18n

import "testing"

func TestBuiltinSessionCurrentErrorCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		if got := Text(lang, KeyBuiltinSessionCurrentError); got == "" || got == "["+string(KeyBuiltinSessionCurrentError)+"]" {
			t.Fatalf("Text(%s, %q) = %q", lang.Code(), KeyBuiltinSessionCurrentError, got)
		}
	}
}
