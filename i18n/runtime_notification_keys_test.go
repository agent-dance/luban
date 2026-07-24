package i18n

import "testing"

func TestRuntimeNotificationPersistenceCopyCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		if got := Text(lang, KeyToolNotificationPersistenceFailed); got == "" || got == string(KeyToolNotificationPersistenceFailed) {
			t.Fatalf("missing runtime notification persistence copy for %s", lang)
		}
	}
}
