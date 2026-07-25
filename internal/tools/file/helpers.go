package file

import (
	"fmt"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// PlanMode is the narrow read-only capability needed by mutating file tools.
// The plan lifecycle remains owned by the session layer.
type PlanMode interface {
	IsActive() bool
}

func cloneToolInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func errorResponse(err error) types.ToolResult {
	return types.ToolResult{
		Content: err.Error(),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}

func errorResponsef(format string, args ...any) types.ToolResult {
	return errorResponse(fmt.Errorf(format, args...))
}

func toolRuntimeErrorf(key i18n.Key, args ...any) types.ToolResult {
	return types.ToolResult{
		Content: toolRuntimeFormat(key, args...),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}

// isUNCPath recognizes both Windows separator forms before any filesystem
// probing so permission checks cannot leak network credentials.
func isUNCPath(value string) bool {
	return len(value) >= 2 &&
		((value[0] == '\\' && value[1] == '\\') || (value[0] == '/' && value[1] == '/'))
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
