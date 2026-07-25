package tasktools

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

func verificationNudgeText(agentType string) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolVerificationNudge, agentType)
}

func containsVerificationHint(value string) bool {
	return stringsContainsFold(value, "verif")
}

func stringsContainsFold(value, needle string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}
