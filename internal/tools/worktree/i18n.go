package worktree

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

func errorResponse(err error) types.ToolResult {
	return types.ToolResult{Content: err.Error(), IsError: true, Outcome: types.ToolOutcomeFailed}
}
