package tools

import (
	"encoding/json"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// FileWriteResult is the complete TS FileWriteTool output schema. Operational
// details such as diagnostics and timing stay on lifecycle hooks instead of
// leaking into the model-visible or SDK result contract.
type FileWriteResult struct {
	Type            string       `json:"type"`
	FilePath        string       `json:"filePath"`
	Content         string       `json:"content"`
	StructuredPatch []DiffHunk   `json:"structuredPatch"`
	OriginalFile    *string      `json:"originalFile"`
	GitDiff         *EditGitDiff `json:"gitDiff,omitempty"`
}

func (t *FileWriteTool) ToolContract() types.ToolContract {
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
		"required":             []string{"filename", "status", "additions", "deletions", "changes", "patch", "repository"},
		"additionalProperties": false,
	}
	output := types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"type":            map[string]any{"type": "string", "enum": []any{"create", "update"}},
			"filePath":        map[string]any{"type": "string"},
			"content":         map[string]any{"type": "string"},
			"structuredPatch": map[string]any{"type": "array", "items": hunkSchema},
			"originalFile":    map[string]any{"type": []any{"string", "null"}},
			"gitDiff":         gitDiffSchema,
		},
		Required:             []string{"type", "filePath", "content", "structuredPatch", "originalFile"},
		AdditionalProperties: false,
	}
	return types.ToolContract{
		OutputSchema:       &output,
		Strict:             true,
		ReadOnly:           false,
		ConcurrencySafe:    false,
		MaxResultSizeChars: 100_000,
	}
}

func (t *FileWriteTool) ToAutoClassifierInput(input map[string]any) string {
	path, _ := input["file_path"].(string)
	content, _ := input["content"].(string)
	return path + ": " + content
}

func (t *FileWriteTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := coerceFileWriteResult(data)
	if !ok {
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

func coerceFileWriteResult(data any) (FileWriteResult, bool) {
	switch value := data.(type) {
	case FileWriteResult:
		return value, value.FilePath != "" && (value.Type == "create" || value.Type == "update")
	case *FileWriteResult:
		if value != nil {
			return *value, value.FilePath != "" && (value.Type == "create" || value.Type == "update")
		}
		return FileWriteResult{}, false
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return FileWriteResult{}, false
		}
		var result FileWriteResult
		if err := json.Unmarshal(raw, &result); err != nil {
			return FileWriteResult{}, false
		}
		return result, result.FilePath != "" && (result.Type == "create" || result.Type == "update")
	}
}

func fileWriteSuccessResponse(result FileWriteResult) (types.ToolResult, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return types.ToolResult{}, err
	}
	return types.ToolResult{Content: string(raw), Data: result}, nil
}
