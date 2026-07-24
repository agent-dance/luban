package permissions

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

func sendMessageTarget(input map[string]any) string {
	target, _ := input["to"].(string)
	return strings.TrimSpace(target)
}

func sendMessagePreview(input map[string]any, max int) string {
	target := sendMessageTarget(input)
	message := ""
	if summary, ok := input["summary"].(string); ok && strings.TrimSpace(summary) != "" {
		message = strings.TrimSpace(summary)
	} else if msg, ok := input["message"].(string); ok {
		message = strings.TrimSpace(msg)
	}
	if max > 0 && len(message) > max {
		message = message[:max] + "…"
	}
	switch {
	case target != "" && message != "":
		return permissionFormat(i18n.KeyPermissionPreviewSendMessage, target, message)
	case target != "":
		return permissionFormat(i18n.KeyPermissionPreviewSendTarget, target)
	default:
		return message
	}
}
