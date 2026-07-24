package i18n

import (
	"strings"
	"testing"
)

func TestToolPromptSemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyAskUserPermission, KeyAskUserPreviewSideBySide, KeyAskUserMultiPrompt,
		KeyAskUserSinglePrompt, KeyAskUserCustomPrompt,
		KeyAskUserOtherOption, KeyAskUserTUISingleHint, KeyAskUserTUIMultiHint,
		KeyAskUserTUICustomHint, KeyAskUserProgress,
		KeyAskUserTUINotesPrompt, KeyAskUserTUINotesHint, KeyAskUserTUINotesAvailable,
		KeyBashSandboxBuildError, KeyBashSandboxFallback,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || strings.HasPrefix(got, "[") {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
