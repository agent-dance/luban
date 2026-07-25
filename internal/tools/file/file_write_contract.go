package file

import (
	"encoding/json"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// FileWriteResult is the complete FileWriteTool output schema.
type FileWriteResult struct {
	Type            string     `json:"type"`
	FilePath        string     `json:"filePath"`
	Content         string     `json:"content"`
	StructuredPatch []DiffHunk `json:"structuredPatch"`
	OriginalFile    *string    `json:"originalFile"`
}

func (t *FileWriteTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := data.(FileWriteResult)
	if !ok || result.FilePath == "" || (result.Type != "create" && result.Type != "update") {
		return types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
			Content: toolRuntimeText(i18n.KeyToolRuntimeWriteInvalidData), IsError: true,
		}
	}
	content := toolRuntimeFormat(i18n.KeyToolRuntimeFileUpdated, result.FilePath)
	if result.Type == "create" {
		content = toolRuntimeFormat(i18n.KeyToolRuntimeFileCreated, result.FilePath)
	}
	return types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
		Content: content, Data: result,
	}
}

func fileWriteSuccessResponse(result FileWriteResult) (types.ToolResult, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return types.ToolResult{}, err
	}
	return types.ToolResult{Content: string(raw), Data: result}, nil
}
