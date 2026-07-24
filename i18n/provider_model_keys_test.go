package i18n

import (
	"strings"
	"testing"
)

func TestProviderModelCatalogCoversEverySupportedLanguage(t *testing.T) {
	seen := 0
	for key, translations := range semanticTranslations {
		name := string(key)
		if !strings.HasPrefix(name, "provider.") && !strings.HasPrefix(name, "model.") && !strings.HasPrefix(name, "reasoning.") {
			continue
		}
		seen++
		for _, lang := range AllLanguages() {
			if got := translations[lang]; got == "" {
				t.Errorf("%s is missing a %s translation", key, lang.Code())
			}
		}
	}
	if seen < 88 {
		t.Fatalf("checked %d Provider/model keys, want at least 88", seen)
	}
}

func TestProviderModelCopyLocalizesWhilePreservingTechnicalValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		detail := Format(lang, KeyProviderConnectionConnectedEnv, "CUSTOM_API_KEY")
		if !strings.Contains(detail, "CUSTOM_API_KEY") || strings.Contains(detail, "%!") {
			t.Errorf("%s connection detail lost its environment variable: %q", lang.Code(), detail)
		}
		hint := Format(lang, KeyProviderSetupEnvOrConnect, "CUSTOM_API_KEY", "provider-id")
		for _, value := range []string{"CUSTOM_API_KEY", "provider-id", "/connect"} {
			if !strings.Contains(hint, value) {
				t.Errorf("%s setup hint lost %q: %q", lang.Code(), value, hint)
			}
		}
	}

	for _, key := range []Key{KeyReasoningDescriptionMedium, KeyModelDescriptionGPT54, KeyProviderConnectionNotConnected} {
		english := Text(LangEN, key)
		for _, lang := range AllLanguages()[1:] {
			if got := Text(lang, key); got == english {
				t.Errorf("%s still uses English for %s: %q", lang.Code(), key, got)
			}
		}
	}
}
