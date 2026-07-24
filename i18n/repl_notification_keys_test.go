package i18n

import "testing"

func TestREPLNotificationSessionChangedCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		if got := Text(lang, KeyREPLNotificationSessionChanged); got == "" || got == string(KeyREPLNotificationSessionChanged) {
			t.Fatalf("missing notification session copy for %s", lang)
		}
	}
}
