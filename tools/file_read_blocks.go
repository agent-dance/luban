// Package tools — file_read_blocks.go provides typed helpers for building the
// discriminated-union output of the Read tool. Mirrors the TS outputSchema in
// src/tools/FileReadTool/FileReadTool.ts (text/image/notebook/pdf/parts/file_unchanged).
//
// These constructors return concrete types.ContentBlock values that are
// assigned to types.ToolResult.ContentBlocks, allowing the Read tool to
// return rich (image/document) content alongside plain text.
package tools

import (
	"encoding/base64"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func fileUnchangedStubText() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolReadFileUnchanged)
}

// newTextBlock wraps a plain-text payload as a TextBlock.
func newTextBlock(text string) types.ContentBlock {
	return types.TextBlock{
		Type: types.ContentTypeText,
		Text: text,
	}
}

// newImageBlock builds an ImageBlock from raw bytes and a media type
// (e.g. "image/png", "image/jpeg"). The bytes are base64-encoded inline.
func newImageBlock(data []byte, mediaType string) types.ContentBlock {
	if mediaType == "" {
		mediaType = "image/png"
	}
	return types.ImageBlock{
		Type: types.ContentTypeImage,
		Source: &types.ImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      base64.StdEncoding.EncodeToString(data),
		},
	}
}

// newImageBlockFromBase64 builds an ImageBlock from already-encoded base64
// data (avoids re-encoding when callers already have a base64 string).
func newImageBlockFromBase64(b64, mediaType string) types.ContentBlock {
	if mediaType == "" {
		mediaType = "image/png"
	}
	return types.ImageBlock{
		Type: types.ContentTypeImage,
		Source: &types.ImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      b64,
		},
	}
}

// newNotebookBlock builds the content blocks for a Jupyter notebook read.
// Each cell renders as a text block (and optional image blocks for outputs).
// This is a thin wrapper over notebookCellsToContentBlocks for callers that
// have parsed cells in hand.
func newNotebookBlock(cells []notebookReadCell) []types.ContentBlock {
	return notebookCellsToContentBlocks(cells)
}

// newPDFPagesBlock builds content blocks for a sequence of extracted PDF
// page images. Each page becomes one ImageBlock with media type image/jpeg.
func newPDFPagesBlock(pages []types.ImageBlock) []types.ContentBlock {
	blocks := make([]types.ContentBlock, 0, len(pages))
	for _, page := range pages {
		blocks = append(blocks, page)
	}
	return blocks
}

// newPDFDocumentBlock builds a single document content block for a whole PDF
// (used when the PDF fits inline without page extraction).
func newPDFDocumentBlock(data []byte) types.ContentBlock {
	return types.DocumentBlock{
		Type: types.ContentTypeDocument,
		Source: &types.DocumentSource{
			Type:      "base64",
			MediaType: "application/pdf",
			Data:      base64.StdEncoding.EncodeToString(data),
		},
	}
}

// newFileUnchangedBlock returns a text block representing a dedup-hit reply.
// Mirrors TS mapToolResultToToolResultBlockParam case 'file_unchanged'.
func newFileUnchangedBlock(filePath string) types.ContentBlock {
	_ = filePath // path is informational; the model only sees the stub message.
	return newTextBlock(fileUnchangedStubText())
}

// newFileUnchangedResult is the canonical helper used by FileReadTool when
// returning a dedup-hit result. It exposes the filename in the .Content
// summary while emitting the stub via ContentBlocks for model consumption.
func newFileUnchangedResult(filePath string) types.ToolResult {
	return types.ToolResult{
		Content:       fileUnchangedStubText(),
		ContentBlocks: []types.ContentBlock{newFileUnchangedBlock(filePath)},
		Data: FileReadOutput{
			Type: FileReadVariantFileUnchanged,
			File: FileReadOutputFile{FilePath: filePath},
		},
		Metadata: readVariantMetadata(FileReadVariantFileUnchanged),
	}
}
