package swarm

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// UserFacingError maps team persistence and mailbox diagnostics to stable
// localized copy while callers retain the original error for diagnostics.
func UserFacingError(lang i18n.Language, err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "team") && strings.Contains(message, "not found"):
		return i18n.Text(lang, i18n.KeyAuxSwarmTeamNotFound)
	case strings.Contains(message, "must not be empty"),
		strings.Contains(message, "too long"),
		strings.Contains(message, "invalid characters"),
		strings.Contains(message, "escapes base directory"):
		return i18n.Text(lang, i18n.KeyAuxSwarmInvalidName)
	case strings.Contains(message, "mailbox"),
		strings.Contains(message, "inbox"),
		strings.Contains(message, "lockfile"):
		return i18n.Text(lang, i18n.KeyAuxSwarmMailboxFailed)
	default:
		return i18n.Text(lang, i18n.KeyAuxSwarmFailed)
	}
}
