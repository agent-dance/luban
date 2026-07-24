// Package tools — file_read_variant.go declares the typed variant union for
// FileReadTool results. FileReadOutput mirrors the exact TS discriminated union
// (text|image|notebook|pdf|parts|file_unchanged). Large-pdf and oversize remain
// legacy error metadata only and never appear as successful typed output data.
//
// FileReadResult is retained as a legacy summary helper. Production typed data
// uses FileReadOutput and ToolResultMapper.
package tools

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
	// FileReadVariantLargePDF — PDF too large to inline; pages must be read
	// explicitly via the `pages` selector.
	FileReadVariantLargePDF FileReadVariant = "large-pdf"
	// FileReadVariantOversize — file exceeds the inline size guard.
	FileReadVariantOversize FileReadVariant = "oversize"
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
	switch output := data.(type) {
	case FileReadOutput:
		return output, true
	case *FileReadOutput:
		if output != nil {
			return *output, true
		}
	}
	return FileReadOutput{}, false
}

// MapToolResultToToolResultBlock owns the model-visible rendering of typed Read
// data, matching the switch in FileReadTool.ts.
func (t *FileReadTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := asFileReadOutput(data)
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   toolRuntimeText(i18n.KeyToolLegacyAFileReadInvalidTyped),
			IsError:   true,
		}
	}
	block := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Data:      output,
		Metadata:  readVariantMetadata(output.Type),
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
	return toolRuntimeFormat(i18n.KeyToolLegacyAFilePDFRead, path, formatReadSize(size))
}

func fmtPDFPartsSummary(path string, size int64, count int) string {
	return toolRuntimeFormat(i18n.KeyToolLegacyAFilePDFPagesExtracted, count, path, formatReadSize(size))
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
	result := types.ToolResult{
		Content:  t.renderTextReadOutput(output),
		Data:     output,
		Metadata: readVariantMetadata(FileReadVariantText),
	}
	if file.Content != "" && t.shouldAppendCyberReminder() {
		result.NewMessages = []types.Message{fileReadSecurityMessage()}
	}
	return result
}

func (t *FileReadTool) ToAutoClassifierInput(input map[string]any) string {
	path, _ := input["file_path"].(string)
	return strings.TrimSpace(path)
}

func (t *FileReadTool) SearchReadClassification(map[string]any) types.ToolSearchReadClassification {
	return types.ToolSearchReadClassification{IsRead: true}
}

func fileReadOutputSchema() types.JSONSchema {
	stringSchema := map[string]any{"type": "string"}
	numberSchema := map[string]any{"type": "number"}
	fileObject := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		}
	}
	variant := func(name string, file map[string]any) map[string]any {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{"const": name},
				"file": file,
			},
			"required":             []string{"type", "file"},
			"additionalProperties": false,
		}
	}
	dimensions := fileObject(map[string]any{
		"originalWidth":  numberSchema,
		"originalHeight": numberSchema,
		"displayWidth":   numberSchema,
		"displayHeight":  numberSchema,
	})
	return types.JSONSchema{OneOf: []any{
		variant("text", fileObject(map[string]any{
			"filePath": stringSchema, "content": stringSchema, "numLines": numberSchema,
			"startLine": numberSchema, "totalLines": numberSchema,
		}, "filePath", "content", "numLines", "startLine", "totalLines")),
		variant("image", fileObject(map[string]any{
			"base64":       stringSchema,
			"type":         map[string]any{"type": "string", "enum": []string{"image/jpeg", "image/png", "image/gif", "image/webp"}},
			"originalSize": numberSchema,
			"dimensions":   dimensions,
		}, "base64", "type", "originalSize")),
		variant("notebook", fileObject(map[string]any{
			"filePath": stringSchema, "cells": map[string]any{"type": "array"},
		}, "filePath", "cells")),
		variant("pdf", fileObject(map[string]any{
			"filePath": stringSchema, "base64": stringSchema, "originalSize": numberSchema,
		}, "filePath", "base64", "originalSize")),
		variant("parts", fileObject(map[string]any{
			"filePath": stringSchema, "originalSize": numberSchema, "count": numberSchema, "outputDir": stringSchema,
		}, "filePath", "originalSize", "count", "outputDir")),
		variant("file_unchanged", fileObject(map[string]any{"filePath": stringSchema}, "filePath")),
	}}
}

// FileReadResult is the structured representation of a Read result. The
// production tool path returns types.ToolResult; this type lets callers (and
// alignment probes) reflect on a stable Go-side shape that captures the
// discriminator and the most useful per-variant fields.
//
// Most fields are optional; only Variant + FilePath are guaranteed. Consumers
// should treat empty fields as "not applicable for this variant".
type FileReadResult struct {
	// Variant is the discriminator or legacy error classification.
	Variant FileReadVariant `json:"variant"`

	// FilePath is the absolute path that was read.
	FilePath string `json:"file_path"`

	// Content is the rendered text payload (numbered lines for text, summary
	// for rich variants, stub for file_unchanged).
	Content string `json:"content,omitempty"`

	// LineCount is the total number of lines in the underlying file.
	LineCount int `json:"line_count,omitempty"`

	// ByteCount is the size of the rendered/read payload in bytes.
	ByteCount int64 `json:"byte_count,omitempty"`

	// Partial reports whether the read covered only a portion of the file.
	Partial bool `json:"partial,omitempty"`

	// Ext is the lowercased file extension, including the dot (e.g. ".txt").
	Ext string `json:"ext,omitempty"`
}

// readVariantMetadata is the canonical per-variant metadata stamp written to
// types.ToolResult.Metadata. Mirrors TS where the variant is the result's
// discriminator. Returned map values are always strings so the result aligns
// with types.ToolResult.Metadata's string-valued shape.
func readVariantMetadata(variant FileReadVariant) map[string]string {
	return map[string]string{
		"variant": string(variant),
		"type":    string(variant),
	}
}

// deriveReadVariantForExt is a best-effort mapping from a file extension to
// the read variant that would be reported. Used by the dedup code path which
// returns a stub without re-running the rich-read pipeline.
func deriveReadVariantForExt(ext string) FileReadVariant {
	switch ext {
	case ".ipynb":
		return FileReadVariantNotebook
	case ".pdf":
		return FileReadVariantPDF
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return FileReadVariantImage
	default:
		return FileReadVariantText
	}
}

// deriveRichReadVariant inspects a rich-read result by extension. PDFs that
// triggered the "too many pages" gate (i.e. surfaced as an error) are tagged
// large-pdf so callers can branch without re-stating the threshold.
func deriveRichReadVariant(ext string, res richReadVariantHints) FileReadVariant {
	switch ext {
	case ".ipynb":
		return FileReadVariantNotebook
	case ".pdf":
		if res.IsError {
			return FileReadVariantLargePDF
		}
		return FileReadVariantPDF
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return FileReadVariantImage
	}
	return FileReadVariantText
}

// richReadVariantHints carries the small slice of result data the variant
// derivation actually needs. We avoid importing types.ToolResult in this
// helper to keep the dependency graph minimal and the helper trivially
// testable.
type richReadVariantHints struct {
	IsError bool
}
