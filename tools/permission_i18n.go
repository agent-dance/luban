package tools

import "github.com/agent-dance/luban/i18n"

func toolPermissionText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func toolPermissionFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

// toolRuntimeText renders first-party copy that is exposed through tool
// results, progress events, notifications, or other user-visible tool
// surfaces. Raw tool output and protocol values must remain outside this
// helper and be passed as format arguments instead.
func toolRuntimeText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func toolRuntimeFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

// toolPromptText renders first-party copy sent to the active model through
// tools[].description and JSON Schema field descriptions.
func toolPromptText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}
