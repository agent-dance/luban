package permissions

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

func permissionText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func permissionFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func isLocalizedToolContractSource(source string) bool {
	for _, lang := range i18n.AllLanguages() {
		formatted := strings.TrimSpace(i18n.Format(lang, i18n.KeyRuntimePermissionRuleToolContract, ""))
		prefix := strings.TrimSpace(strings.TrimSuffix(formatted, ":"))
		if prefix != "" && strings.HasPrefix(source, prefix) {
			return true
		}
	}
	return false
}
