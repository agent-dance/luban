package i18n

import "testing"

func TestConfigCacheRoutingModeKeyCoversAllLanguages(t *testing.T) {
	for _, language := range AllLanguages() {
		if got := Text(language, KeyConfigInvalidCacheRoutingMode); got == "" {
			t.Fatalf("missing cache routing config copy for %s", language)
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
