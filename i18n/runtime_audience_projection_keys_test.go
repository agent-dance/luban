package i18n

import "testing"

func TestRuntimeAudienceProjectionCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range runtimeAudienceProjectionKeys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == string(key) {
				t.Fatalf("%s missing %s translation", key, lang.Code())
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
