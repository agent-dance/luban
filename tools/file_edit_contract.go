package tools

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ToolContract publishes the typed Edit result while keeping the concise
// transcript rendering owned by MapToolResultToToolResultBlock.
func (t *FileEditTool) ToolContract() types.ToolContract {
	hunkSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"oldStart": map[string]any{"type": "number"},
			"oldLines": map[string]any{"type": "number"},
			"newStart": map[string]any{"type": "number"},
			"newLines": map[string]any{"type": "number"},
			"lines":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"oldStart", "oldLines", "newStart", "newLines", "lines"},
		"additionalProperties": false,
	}
	gitDiffSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filename":   map[string]any{"type": "string"},
			"status":     map[string]any{"type": "string", "enum": []any{"modified", "added"}},
			"additions":  map[string]any{"type": "number"},
			"deletions":  map[string]any{"type": "number"},
			"changes":    map[string]any{"type": "number"},
			"patch":      map[string]any{"type": "string"},
			"repository": map[string]any{"type": []any{"string", "null"}},
		},
		"required": []string{"filename", "status", "additions", "deletions", "changes", "patch"},
	}
	output := types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"filePath":        map[string]any{"type": "string"},
			"oldString":       map[string]any{"type": "string"},
			"newString":       map[string]any{"type": "string"},
			"originalFile":    map[string]any{"type": "string"},
			"structuredPatch": map[string]any{"type": "array", "items": hunkSchema},
			"userModified":    map[string]any{"type": "boolean"},
			"replaceAll":      map[string]any{"type": "boolean"},
			"gitDiff":         gitDiffSchema,
			"metadata": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"occurrences": map[string]any{"type": "number"},
					"durationMs":  map[string]any{"type": "number"},
				},
				"required": []string{"occurrences", "durationMs"},
			},
			"status":      map[string]any{"type": "string"},
			"diagnostics": map[string]any{"type": "array"},
			"warning":     map[string]any{"type": "string"},
		},
		Required: []string{"filePath", "oldString", "newString", "originalFile", "structuredPatch", "userModified", "replaceAll"},
	}
	return types.ToolContract{
		OutputSchema:       &output,
		Strict:             true,
		ReadOnly:           false,
		ConcurrencySafe:    false,
		MaxResultSizeChars: 100_000,
	}
}

// ToAutoClassifierInput matches the TS permission classifier projection.
func (t *FileEditTool) ToAutoClassifierInput(input map[string]any) string {
	path, _ := input["file_path"].(string)
	newString, _ := input["new_string"].(string)
	return path + ": " + newString
}

// MapToolResultToToolResultBlock keeps the complete EditResult in Data while
// presenting the same short success sentence as the TypeScript tool.
func (t *FileEditTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := coerceEditResult(data)
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   fmt.Sprint(data),
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   result.String(),
		Data:      result,
	}
}

func coerceEditResult(data any) (EditResult, bool) {
	switch value := data.(type) {
	case EditResult:
		return value, true
	case *EditResult:
		if value != nil {
			return *value, true
		}
		return EditResult{}, false
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return EditResult{}, false
		}
		var result EditResult
		if err := json.Unmarshal(raw, &result); err != nil || result.FilePath == "" {
			return EditResult{}, false
		}
		return result, true
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
	normalized := make(map[string]any, len(input))
	for key, value := range input {
		normalized[key] = value
	}
	if raw, ok := normalized["replace_all"].(string); ok {
		parsed, err := strconv.ParseBool(raw)
		if err != nil || (raw != "true" && raw != "false") {
			result := ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeEditInvalidReplaceAll))
			return FileEditInput{}, &result
		}
		normalized["replace_all"] = parsed
	}
	decoded, err := types.DecodeStrictToolInput[FileEditInput](normalized)
	if err != nil {
		result := ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, err))
		return FileEditInput{}, &result
	}
	return decoded, nil
}
