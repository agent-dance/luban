package file

import (
	"encoding/json"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// MapToolResultToToolResultBlock keeps the complete EditResult in Data while
// presenting the same short success sentence as the TypeScript tool.
func (t *FileEditTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := data.(EditResult)
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   toolRuntimeText(i18n.KeyToolRuntimeEditInvalidData),
			IsError:   true,
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   result.String(),
		Data:      result,
	}
}

func editSuccessResponse(result EditResult) (types.ToolResult, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeResponseMarshalFailed, err), IsError: true}, nil
	}
	return types.ToolResult{
		Content: string(raw),
		Data:    result,
	}, nil
}

func decodeFileEditInput(input map[string]any) (FileEditInput, *types.ToolResult) {
	decoded, err := types.DecodeStrictToolInput[FileEditInput](input)
	if err != nil {
		result := errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, err))
		return FileEditInput{}, &result
	}
	return decoded, nil
}
