package shell

import (
	"fmt"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func errorResponse(err error) types.ToolResult {
	return types.ToolResult{Content: err.Error(), IsError: true, Outcome: types.ToolOutcomeFailed}
}

func errorResponsef(format string, args ...any) types.ToolResult {
	return errorResponse(fmt.Errorf(format, args...))
}

func stringResponse(content string) (types.ToolResult, error) {
	return types.ToolResult{Content: content, Outcome: types.ToolOutcomeSucceeded}, nil
}

func inputInt(input map[string]any, field string, fallback int) int {
	switch value := input[field].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return fallback
	}
}

func requiredString(input map[string]any, field string) (string, error) {
	value, ok := input[field].(string)
	if !ok {
		return "", i18n.NewError(i18n.KeyToolRuntimeFieldStringRequired, field)
	}
	return value, nil
}

func toolPermissionText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func toolPermissionFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func toolRuntimeText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func toolRuntimeFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func toolPromptText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}
