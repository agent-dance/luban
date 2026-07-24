package i18n

import (
	"strings"
	"testing"
)

func TestPermissionPromptSemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyPermissionPromptInline, KeyPermissionPromptTool, KeyPermissionPromptCall,
		KeyPermissionPromptInfo, KeyPermissionPromptRisk, KeyPermissionPromptAllow,
		KeyPermissionPromptRiskLow, KeyPermissionPromptRiskMedium, KeyPermissionPromptRiskHigh,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || strings.HasPrefix(got, "[") {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
