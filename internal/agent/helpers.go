package agent

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const permissionModeDefault = "default"

func toolPermissionText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func toolRuntimeText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func toolRuntimeFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func ErrorResponse(err error) types.ToolResult {
	return types.ToolResult{
		Content: err.Error(),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}

func ErrorResponsef(format string, args ...any) types.ToolResult {
	return ErrorResponse(fmt.Errorf(format, args...))
}

func isTruthyEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func sanitizeAgentIdentifier(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case r == '_' || r == '-':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-_")
}
