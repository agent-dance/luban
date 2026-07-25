package i18n

import "testing"

func TestToolSendUserMessagePromptKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolSendUserMessagePromptKeys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
}
