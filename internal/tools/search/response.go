package search

import (
	"fmt"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

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

func stringResponse(content string) (types.ToolResult, error) {
	return types.ToolResult{
		Content: content,
		Outcome: types.ToolOutcomeSucceeded,
	}, nil
}

func newTextBlock(text string) types.ContentBlock {
	return types.TextBlock{Type: types.ContentTypeText, Text: text}
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
