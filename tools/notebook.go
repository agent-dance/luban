package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ─── NotebookEditTool ────────────────────────────────────────────────────────

// NotebookEditInput is the typed input for NotebookEditTool
type NotebookEditInput struct {
	NotebookPath string `json:"notebook_path"`
	CellID       string `json:"cell_id,omitempty"`
	NewSource    string `json:"new_source"`
	CellType     string `json:"cell_type,omitempty"` // "code" or "markdown"
	EditMode     string `json:"edit_mode,omitempty"` // "replace", "insert", "delete"
}

// NotebookEditResult mirrors the structured shape returned by the TS
// NotebookEdit tool.
type NotebookEditResult struct {
	NewSource    string `json:"new_source"`
	CellID       string `json:"cell_id,omitempty"`
	CellType     string `json:"cell_type"`
	Language     string `json:"language"`
	EditMode     string `json:"edit_mode"`
	Error        string `json:"error"`
	NotebookPath string `json:"notebook_path"`
	OriginalFile string `json:"original_file"`
	UpdatedFile  string `json:"updated_file"`
}

// NotebookEditTool edits Jupyter notebook (.ipynb) cells. Edits require a
// prior read entry for the target notebook; a nil ReadState falls back to the
// package default so the gate remains active for bare tool instances.
type NotebookEditTool struct {
	AllowedDirs []string
	PlanState   *PlanState

	// ReadState — when set, Edit refuses to operate on a notebook that has
	// no prior read entry. Registry wiring supplies this in normal sessions.
	ReadState *ReadFileState

	HistoryStore *FileHistoryStore
}

// readState resolves the ReadFileState NotebookEdit should check before edit
// and refresh after a successful write.
func (t *NotebookEditTool) readState() *ReadFileState {
	if t != nil && t.ReadState != nil {
		return t.ReadState
	}
	return DefaultReadFileState()
}

func (t *NotebookEditTool) Name() string { return "NotebookEdit" }

func (t *NotebookEditTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *NotebookEditTool) Description() string {
	return "Edit cells in a Jupyter notebook (.ipynb file). Supports replace, insert, and delete operations."
}

func (t *NotebookEditTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true}
}

func (t *NotebookEditTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	if t != nil && t.PlanState != nil && t.PlanState.IsActive() {
		return types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny,
			Message:  toolPermissionText(i18n.KeyToolPermissionNotebookPlanMode),
			Required: true,
		}, nil
	}
	return checkFileWritePermission(notebookPermissionPath(input), input, request.Runtime.AllowedDirs, t.allowedDirs(), "NotebookEdit"), nil
}

func (t *NotebookEditTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"notebook_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the .ipynb file",
			},
			"cell_id": map[string]any{
				"type":        "string",
				"description": "Cell ID to edit. For insert, new cell is added after this ID.",
			},
			"new_source": map[string]any{
				"type":        "string",
				"description": "New source content for the cell",
			},
			"cell_type": map[string]any{
				"type":        "string",
				"description": "Cell type: code or markdown",
				"enum":        []string{"code", "markdown"},
			},
			"edit_mode": map[string]any{
				"type":        "string",
				"description": "Edit mode: replace (default), insert, or delete",
				"enum":        []string{"replace", "insert", "delete"},
			},
		},
		"notebook_path",
		"new_source",
	)
}

func (t *NotebookEditTool) ToolContract() types.ToolContract {
	outputSchema := types.StrictObjectSchema(map[string]any{
		"new_source":    map[string]any{"type": "string"},
		"cell_id":       map[string]any{"type": "string"},
		"cell_type":     map[string]any{"type": "string", "enum": []string{"code", "markdown"}},
		"language":      map[string]any{"type": "string"},
		"edit_mode":     map[string]any{"type": "string"},
		"error":         map[string]any{"type": "string"},
		"notebook_path": map[string]any{"type": "string"},
		"original_file": map[string]any{"type": "string"},
		"updated_file":  map[string]any{"type": "string"},
	}, "new_source", "cell_type", "language", "edit_mode", "notebook_path", "original_file", "updated_file")
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		ReadOnly:           false,
		ConcurrencySafe:    false,
		MaxResultSizeChars: 100000,
	}
}

func (t *NotebookEditTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	block := types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID}
	out, ok := data.(NotebookEditResult)
	if !ok {
		if ptr, ptrOK := data.(*NotebookEditResult); ptrOK && ptr != nil {
			out = *ptr
			ok = true
		}
	}
	if !ok {
		block.Content = toolRuntimeText(i18n.KeyToolRuntimeNotebookUnknownEditMode)
		return block
	}
	if out.Error != "" {
		block.Content = out.Error
		block.IsError = true
		return block
	}
	switch out.EditMode {
	case "replace":
		block.Content = toolRuntimeFormat(i18n.KeyToolRuntimeNotebookCellUpdated, out.CellID, out.NewSource)
	case "insert":
		block.Content = toolRuntimeFormat(i18n.KeyToolRuntimeNotebookCellInserted, out.CellID, out.NewSource)
	case "delete":
		block.Content = toolRuntimeFormat(i18n.KeyToolRuntimeNotebookCellDeleted, out.CellID)
	default:
		block.Content = toolRuntimeText(i18n.KeyToolRuntimeNotebookUnknownEditMode)
	}
	return block
}

func (t *NotebookEditTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if validationErr := validateNotebookEditStringInputs(input); validationErr != nil {
		return notebookEditResultResponse(NotebookEditResult{
			CellType: "code",
			Language: "python",
			EditMode: "replace",
			Error:    validationErr.Error(),
		}, true), nil
	}
	in, decodeErr := types.DecodeStrictToolInput[NotebookEditInput](input)
	if decodeErr != nil {
		return notebookEditResultResponse(NotebookEditResult{
			CellType: "code",
			Language: "python",
			EditMode: "replace",
			Error:    toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, decodeErr),
		}, true), nil
	}

	if in.NotebookPath == "" {
		return notebookEditErrorResult(in, "", toolRuntimeFormat(i18n.KeyToolRuntimeRequiredFieldMissing, "notebook_path")), nil
	}
	absPath, absErr := filepath.Abs(in.NotebookPath)
	if absErr != nil {
		return notebookEditErrorResult(in, "", toolRuntimeFormat(i18n.KeyToolRuntimeNotebookResolvePathFailed, absErr)), nil
	}
	absPath = filepath.Clean(absPath)
	if !strings.HasSuffix(absPath, ".ipynb") {
		return notebookEditErrorResult(in, absPath, toolRuntimeText(i18n.KeyToolRuntimeNotebookFileRequired)), nil
	}
	if err := checkAllowedPath(absPath, t.allowedDirs()); err != nil {
		return notebookEditErrorResult(in, absPath, err.Error()), nil
	}
	if in.CellType != "" && in.CellType != "code" && in.CellType != "markdown" {
		return notebookEditErrorResult(in, absPath, toolRuntimeFormat(i18n.KeyToolRuntimeNotebookCellTypeInvalid, in.CellType)), nil
	}

	editMode := in.EditMode
	if editMode == "" {
		editMode = "replace"
	}
	if editMode != "replace" && editMode != "insert" && editMode != "delete" {
		return notebookEditErrorResult(in, absPath, toolRuntimeText(i18n.KeyToolRuntimeNotebookEditModeInvalid)), nil
	}

	if editMode == "insert" && in.CellType == "" {
		return notebookEditErrorResult(in, absPath, toolRuntimeText(i18n.KeyToolRuntimeNotebookInsertCellTypeRequired)), nil
	}
	if editMode != "insert" && in.CellID == "" {
		return notebookEditErrorResult(in, absPath, toolRuntimeText(i18n.KeyToolRuntimeNotebookCellIDRequired)), nil
	}

	state := t.readState()
	readEntry, ok := state.GetForContext(ctx, absPath)
	if !ok {
		return notebookEditErrorResult(in, absPath, toolRuntimeText(i18n.KeyToolRuntimeNotebookNotRead)), nil
	}
	info, statErr := os.Stat(absPath)
	if statErr != nil {
		return notebookEditErrorResult(in, absPath, toolRuntimeFormat(i18n.KeyToolRuntimeNotebookNotFound, statErr)), nil
	}
	if isFileStale(info.ModTime(), readEntry) {
		return notebookEditErrorResult(in, absPath, toolRuntimeText(i18n.KeyToolRuntimeNotebookChangedSinceRead)), nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return notebookEditErrorResult(in, absPath, toolRuntimeFormat(i18n.KeyToolRuntimeNotebookReadFailed, err)), nil
	}
	encoding := detectFileEncoding(data)
	originalContent := decodeFileBytes(data, encoding)
	ending := detectNotebookLineEnding(originalContent)
	normalisedContent := normaliseToLF(originalContent)
	nb, err := ParseNotebook([]byte(normalisedContent))
	if err != nil {
		return notebookEditErrorResult(in, absPath, toolRuntimeText(i18n.KeyToolRuntimeNotebookInvalidJSON)), nil
	}
	if _, _, targetErr := validateNotebookTarget(nb, in.CellID, editMode); targetErr != nil {
		return notebookEditErrorResult(in, absPath, targetErr.Error()), nil
	}

	outcome, err := applyNotebookEdit(nb, NotebookEditOp{
		CellID:    in.CellID,
		NewSource: in.NewSource,
		CellType:  in.CellType,
		EditMode:  editMode,
	})
	if err != nil {
		return notebookEditErrorResult(in, absPath, err.Error()), nil
	}

	out, err := SerializeNotebook(nb)
	if err != nil {
		return notebookEditErrorResult(in, absPath, toolRuntimeFormat(i18n.KeyToolRuntimeNotebookSerializeFailed, err)), nil
	}
	updatedContent := string(out)
	finalContent := restoreLineEnding(updatedContent, ending)
	mode := info.Mode().Perm()
	finalBytes := encodeWriteBytes(finalContent, encoding.Encoding, encoding.BOM)
	if err := atomicWriteFile(absPath, finalBytes, mode); err != nil {
		return notebookEditErrorResult(in, absPath, toolRuntimeFormat(i18n.KeyToolRuntimeNotebookWriteFailed, err)), nil
	}

	if t != nil && t.HistoryStore != nil {
		_ = t.HistoryStore.TrackEdit(FileHistoryEntry{
			Path:   absPath,
			Before: originalContent,
			After:  finalContent,
			Tool:   "NotebookEdit",
			Ts:     time.Now().UnixMilli(),
		})
	}

	if newInfo, statErr := os.Stat(absPath); statErr == nil {
		totalLines := readStateTotalLines(updatedContent)
		coverage, _ := readObservationCoverage(1, totalLines, totalLines)
		state.SetForContext(ctx, absPath, ReadFileEntry{
			TimestampMs:   newInfo.ModTime().UnixMilli(),
			MtimeNs:       newInfo.ModTime().UnixNano(),
			TotalBytes:    newInfo.Size(),
			TotalLines:    totalLines,
			Coverage:      coverage,
			CoverageKnown: true, CoverageComplete: true, FullSnapshot: true,
			Content:       updatedContent,
			IsPartialView: false,
			LastTool:      "NotebookEdit",
			Encoding:      encoding.Encoding,
			BOM:           append([]byte(nil), encoding.BOM...),
		})
	} else {
		totalLines := readStateTotalLines(updatedContent)
		coverage, _ := readObservationCoverage(1, totalLines, totalLines)
		state.SetForContext(ctx, absPath, ReadFileEntry{
			TimestampMs:   time.Now().UnixMilli(),
			TotalBytes:    int64(len(finalBytes)),
			TotalLines:    totalLines,
			Coverage:      coverage,
			CoverageKnown: true, CoverageComplete: true, FullSnapshot: true,
			Content:       updatedContent,
			IsPartialView: false,
			LastTool:      "NotebookEdit",
			Encoding:      encoding.Encoding,
			BOM:           append([]byte(nil), encoding.BOM...),
		})
	}

	result := NotebookEditResult{
		NewSource:    in.NewSource,
		CellID:       notebookEditResultCellID(nb, in, outcome),
		CellType:     notebookEditResultCellType(in.CellType),
		Language:     notebookLanguage(nb),
		EditMode:     outcome.EditMode,
		Error:        "",
		NotebookPath: absPath,
		OriginalFile: normalisedContent,
		UpdatedFile:  updatedContent,
	}
	return notebookEditResultResponse(result, false), nil
}

// MarshalNotebookEditResult exposes the structured result for callers that
// want JSON (e.g. tests / SDK consumers).
func MarshalNotebookEditResult(r NotebookEditResult) ([]byte, error) {
	return json.Marshal(r)
}

func (t *NotebookEditTool) allowedDirs() []string {
	if t == nil {
		return nil
	}
	return t.AllowedDirs
}

func notebookPermissionPath(input map[string]any) string {
	if input == nil {
		return ""
	}
	path, _ := input["notebook_path"].(string)
	return strings.TrimSpace(path)
}

func validateNotebookEditStringInputs(input map[string]any) error {
	for _, field := range []string{"notebook_path", "new_source"} {
		value, ok := input[field]
		if !ok {
			return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeRequiredFieldMissing, field))
		}
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeFieldStringRequired, field))
		}
	}
	for _, field := range []string{"cell_id", "cell_type", "edit_mode"} {
		if value, ok := input[field]; ok {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeFieldStringRequired, field))
			}
		}
	}
	return nil
}

func detectNotebookLineEnding(content string) string {
	crlfCount := strings.Count(content, "\r\n")
	bareLFCount := strings.Count(content, "\n") - crlfCount
	if crlfCount > bareLFCount {
		return "\r\n"
	}
	return "\n"
}

func notebookEditResultResponse(result NotebookEditResult, isError bool) types.ToolResult {
	body, err := json.Marshal(result)
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeResponseMarshalFailed, err), IsError: true}
	}
	return types.ToolResult{Content: string(body), Data: result, IsError: isError}
}

func notebookEditErrorResult(in NotebookEditInput, absPath, message string) types.ToolResult {
	return notebookEditResultResponse(NotebookEditResult{
		NewSource:    in.NewSource,
		CellID:       in.CellID,
		CellType:     notebookEditResultCellType(in.CellType),
		Language:     "python",
		EditMode:     "replace",
		Error:        message,
		NotebookPath: absPath,
		OriginalFile: "",
		UpdatedFile:  "",
	}, true)
}

func notebookEditResultCellType(cellType string) string {
	if cellType == "markdown" {
		return "markdown"
	}
	return "code"
}

func notebookEditResultCellID(nb *Notebook, in NotebookEditInput, outcome *NotebookEditOutcome) string {
	if outcome == nil {
		return ""
	}
	if outcome.EditMode == "insert" {
		return outcome.CellID
	}
	if notebookSupportsCellID(nb.NBFormat, nb.NBFormatMinor) {
		return in.CellID
	}
	return ""
}

func notebookLanguage(nb *Notebook) string {
	if nb == nil || nb.Metadata == nil {
		return "python"
	}
	info, _ := nb.Metadata["language_info"].(map[string]any)
	if name, _ := info["name"].(string); name != "" {
		return name
	}
	return "python"
}

func validateNotebookTarget(nb *Notebook, cellID, editMode string) (*Cell, int, error) {
	if cellID == "" {
		if editMode == "insert" {
			return nil, 0, nil
		}
		return nil, -1, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeNotebookCellIDRequired))
	}
	cell, idx, err := ResolveCell(nb, cellID)
	if err != nil {
		return nil, -1, err
	}
	return cell, idx, nil
}

// splitSourceLines splits source text into lines as stored in .ipynb format.
// Each line except the last gets a trailing newline.
func splitSourceLines(source string) []string {
	if source == "" {
		return []string{}
	}
	lines := strings.Split(source, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		if i < len(lines)-1 {
			result[i] = line + "\n"
		} else {
			result[i] = line
		}
	}
	return result
}
