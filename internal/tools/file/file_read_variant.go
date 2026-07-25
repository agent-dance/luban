// Package file — file_read_variant.go declares the typed variant union for
// FileReadTool results. FileReadOutput mirrors the exact TS discriminated union
// (text|image|notebook|pdf|parts|file_unchanged). Oversize remains error
// metadata and never appears as successful typed output data.
package file

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// FileReadVariant identifies which read path produced a ToolResult.
type FileReadVariant string

const (
	// FileReadVariantText — plain text file, possibly truncated by offset/limit.
	FileReadVariantText FileReadVariant = "text"
	// FileReadVariantImage — image file rendered via the image content block path.
	FileReadVariantImage FileReadVariant = "image"
	// FileReadVariantNotebook — Jupyter notebook (.ipynb) rendered via cells.
	FileReadVariantNotebook FileReadVariant = "notebook"
	// FileReadVariantPDF — PDF file inlined as a document content block.
	FileReadVariantPDF FileReadVariant = "pdf"
	// FileReadVariantParts identifies PDF page extraction results.
	FileReadVariantParts FileReadVariant = "parts"
	// FileReadVariantFileUnchanged identifies an eligible ReadState dedup hit.
	FileReadVariantFileUnchanged FileReadVariant = "file_unchanged"
)

// FileReadOutput is the Go carrier for the TS Read output discriminated union.
// Only fields applicable to Type are populated. The JSON names intentionally
// match FileReadTool.ts so SDK/runtime consumers can inspect the same data that
// the model-facing mapper renders.
type FileReadOutput struct {
	Type FileReadVariant    `json:"type"`
	File FileReadOutputFile `json:"file"`
}

type FileReadOutputFile struct {
	FilePath     string                   `json:"filePath,omitempty"`
	Content      string                   `json:"content,omitempty"`
	NumLines     int                      `json:"numLines,omitempty"`
	StartLine    int                      `json:"startLine,omitempty"`
	TotalLines   int                      `json:"totalLines,omitempty"`
	Base64       string                   `json:"base64,omitempty"`
	MediaType    string                   `json:"type,omitempty"`
	OriginalSize int64                    `json:"originalSize,omitempty"`
	Dimensions   *FileReadImageDimensions `json:"dimensions,omitempty"`
	Cells        []notebookReadCell       `json:"cells,omitempty"`
	Count        int                      `json:"count,omitempty"`
	OutputDir    string                   `json:"outputDir,omitempty"`
}

type FileReadImageDimensions struct {
	OriginalWidth  int `json:"originalWidth,omitempty"`
	OriginalHeight int `json:"originalHeight,omitempty"`
	DisplayWidth   int `json:"displayWidth,omitempty"`
	DisplayHeight  int `json:"displayHeight,omitempty"`
}

// MarshalJSON emits the exact per-variant file object from the TS output
// union. The shared Go carrier uses optional fields, but fields such as
// numLines and originalSize remain required even when their value is zero.
func (o FileReadOutput) MarshalJSON() ([]byte, error) {
	type envelope struct {
		Type FileReadVariant `json:"type"`
		File any             `json:"file"`
	}
	file := o.File
	switch o.Type {
	case FileReadVariantText:
		return json.Marshal(envelope{Type: o.Type, File: struct {
			FilePath   string `json:"filePath"`
			Content    string `json:"content"`
			NumLines   int    `json:"numLines"`
			StartLine  int    `json:"startLine"`
			TotalLines int    `json:"totalLines"`
		}{file.FilePath, file.Content, file.NumLines, file.StartLine, file.TotalLines}})
	case FileReadVariantImage:
		return json.Marshal(envelope{Type: o.Type, File: struct {
			Base64       string                   `json:"base64"`
			MediaType    string                   `json:"type"`
			OriginalSize int64                    `json:"originalSize"`
			Dimensions   *FileReadImageDimensions `json:"dimensions,omitempty"`
		}{file.Base64, file.MediaType, file.OriginalSize, file.Dimensions}})
	case FileReadVariantNotebook:
		cells := file.Cells
		if cells == nil {
			cells = []notebookReadCell{}
		}
		return json.Marshal(envelope{Type: o.Type, File: struct {
			FilePath string             `json:"filePath"`
			Cells    []notebookReadCell `json:"cells"`
		}{file.FilePath, cells}})
	case FileReadVariantPDF:
		return json.Marshal(envelope{Type: o.Type, File: struct {
			FilePath     string `json:"filePath"`
			Base64       string `json:"base64"`
			OriginalSize int64  `json:"originalSize"`
		}{file.FilePath, file.Base64, file.OriginalSize}})
	case FileReadVariantParts:
		return json.Marshal(envelope{Type: o.Type, File: struct {
			FilePath     string `json:"filePath"`
			OriginalSize int64  `json:"originalSize"`
			Count        int    `json:"count"`
			OutputDir    string `json:"outputDir"`
		}{file.FilePath, file.OriginalSize, file.Count, file.OutputDir}})
	case FileReadVariantFileUnchanged:
		return json.Marshal(envelope{Type: o.Type, File: struct {
			FilePath string `json:"filePath"`
		}{file.FilePath}})
	default:
		return json.Marshal(envelope{Type: o.Type, File: file})
	}
}

func fileReadImageDimensions(in *readImageDimensions) *FileReadImageDimensions {
	if in == nil {
		return nil
	}
	return &FileReadImageDimensions{
		OriginalWidth:  in.OriginalWidth,
		OriginalHeight: in.OriginalHeight,
		DisplayWidth:   in.DisplayWidth,
		DisplayHeight:  in.DisplayHeight,
	}
}

func asFileReadOutput(data any) (FileReadOutput, bool) {
	output, ok := data.(FileReadOutput)
	return output, ok
}

// MapToolResultToToolResultBlock owns the model-visible rendering of typed Read
// data, matching the switch in FileReadTool.ts.
func (t *FileReadTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := asFileReadOutput(data)
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   toolRuntimeText(i18n.KeyToolFileReadInvalidTyped),
			IsError:   true,
		}
	}
	block := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Data:      output,
	}
	switch output.Type {
	case FileReadVariantImage:
		block.ContentBlocks = []types.ContentBlock{newImageBlockFromBase64(output.File.Base64, output.File.MediaType)}
	case FileReadVariantNotebook:
		block.ContentBlocks = newNotebookBlock(output.File.Cells)
	case FileReadVariantPDF:
		block.Content = fmtPDFReadSummary(output.File.FilePath, output.File.OriginalSize)
	case FileReadVariantParts:
		block.Content = fmtPDFPartsSummary(output.File.FilePath, output.File.OriginalSize, output.File.Count)
	case FileReadVariantFileUnchanged:
		block.Content = fileUnchangedStubText()
	case FileReadVariantText:
		block.Content = t.renderTextReadOutput(output)
	default:
		block.Content = output.File.Content
	}
	return block
}

func fmtPDFReadSummary(path string, size int64) string {
	return toolRuntimeFormat(i18n.KeyToolFilePDFRead, path, formatReadSize(size))
}

func fmtPDFPartsSummary(path string, size int64, count int) string {
	return toolRuntimeFormat(i18n.KeyToolFilePDFPagesExtracted, count, path, formatReadSize(size))
}

func (t *FileReadTool) renderTextReadOutput(output FileReadOutput) string {
	file := output.File
	if file.Content == "" {
		if file.TotalLines == 0 {
			return toolRuntimeText(i18n.KeyToolReadEmptyFileWarning)
		}
		return toolRuntimeFormat(i18n.KeyToolReadOffsetBeyondEndWarning, file.StartLine, file.TotalLines)
	}
	var builder strings.Builder
	for i, line := range strings.Split(file.Content, "\n") {
		fmt.Fprintf(&builder, "%d\t%s\n", file.StartLine+i, line)
	}
	return strings.TrimRight(builder.String(), "\n")
}

func (t *FileReadTool) newTextReadResult(file FileReadOutputFile) types.ToolResult {
	output := FileReadOutput{Type: FileReadVariantText, File: file}
	return types.ToolResult{
		Content:  t.renderTextReadOutput(output),
		Data:     output,
		Metadata: make(map[string]string),
	}
}
