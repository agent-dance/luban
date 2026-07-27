package i18n

import "testing"

func TestProviderResponsesWebSocketKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range []Key{
		KeyProviderResponsesWebSocketProtocolInvalid,
		KeyProviderResponsesWebSocketCapacity,
		KeyProviderResponsesWebSocketEndpointInvalid,
		KeyProviderResponsesWebSocketProfileUnsupported,
	} {
		for _, language := range AllLanguages() {
			if text := Text(language, key); text == "" || text == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", language.Code(), key, text)
			}
		}
	}
}
