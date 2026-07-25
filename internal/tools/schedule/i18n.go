package schedule

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func text(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func format(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func failedResult(key i18n.Key, code string, args ...any) types.ToolResult {
	metadata := map[string]string{"error_code": code}
	return types.ToolResult{
		Content:  format(key, args...),
		IsError:  true,
		Outcome:  types.ToolOutcomeFailed,
		Metadata: metadata,
	}
}

func failedWrappedResult(key i18n.Key, code string, cause error, args ...any) types.ToolResult {
	metadata := map[string]string{"error_code": code}
	return types.ToolResult{
		Content:  i18n.WrapError(key, cause, args...).Error(),
		IsError:  true,
		Outcome:  types.ToolOutcomeFailed,
		Metadata: metadata,
	}
}
