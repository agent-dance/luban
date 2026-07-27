package i18n

import "testing"

func TestCLIResponsesWebSocketKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range []Key{
		KeyCLIFlagResponsesWebSocket,
		KeyCLIResponsesWebSocketRequiresResponses,
	} {
		for _, language := range AllLanguages() {
			if text := Text(language, key); text == "" || text == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", language.Code(), key, text)
			}
		}
	}
}
