package remote

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func runtimeText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func runtimeFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func promptText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func runtimeError(key i18n.Key) types.ToolResult {
	return types.ToolResult{
		Content: runtimeText(key),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}

func runtimeErrorf(key i18n.Key, args ...any) types.ToolResult {
	return types.ToolResult{
		Content: runtimeFormat(key, args...),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}

func errorResult(err error) types.ToolResult {
	return types.ToolResult{
		Content: err.Error(),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}
