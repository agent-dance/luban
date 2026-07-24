package tools

import (
	"strings"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// CreateMCPContentSummary takes a structured ContentBlock array as returned by
// MCP-style bash wrappers and produces a scannable summary line plus per-block
// previews:
//
//	MCP Result: 2 images, 3 text blocks
//
//	[image #0: image/png]
//	[image #1: image/jpeg]
//	[text #0]: first 200 chars …
//	[text #1]: …
//
// Each text block is truncated to 200 characters with an ellipsis. Images
// produce a one-line marker carrying the media type. Mirrors TS
// createContentSummary.
const mcpTextPreviewMax = 200

// CreateMCPContentSummary returns a string suitable for ToolResult.Content
// when the underlying response carries a multi-block payload that the caller
// wants to render in plain text alongside the structured ContentBlocks.
func CreateMCPContentSummary(blocks []types.ContentBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	imageCount := 0
	textCount := 0
	otherCount := 0
	for _, block := range blocks {
		switch block.(type) {
		case types.ImageBlock:
			imageCount++
		case types.TextBlock:
			textCount++
		default:
			otherCount++
		}
	}

	parts := make([]string, 0, 3)
	if imageCount > 0 {
		key := i18n.KeyToolRuntimeMCPSummaryImages
		if imageCount == 1 {
			key = i18n.KeyToolRuntimeMCPSummaryImage
		}
		parts = append(parts, toolRuntimeFormat(key, imageCount))
	}
	if textCount > 0 {
		key := i18n.KeyToolRuntimeMCPSummaryTextBlocks
		if textCount == 1 {
			key = i18n.KeyToolRuntimeMCPSummaryTextBlock
		}
		parts = append(parts, toolRuntimeFormat(key, textCount))
	}
	if otherCount > 0 {
		key := i18n.KeyToolRuntimeMCPSummaryOtherBlocks
		if otherCount == 1 {
			key = i18n.KeyToolRuntimeMCPSummaryOtherBlock
		}
		parts = append(parts, toolRuntimeFormat(key, otherCount))
	}

	var lines []string
	lines = append(lines, toolRuntimeFormat(i18n.KeyToolRuntimeMCPSummaryHeader, strings.Join(parts, ", ")))

	imgIdx := 0
	txtIdx := 0
	otherIdx := 0
	for _, block := range blocks {
		switch typed := block.(type) {
		case types.ImageBlock:
			mt := toolRuntimeText(i18n.KeyToolRuntimeMCPSummaryUnknownMedia)
			if typed.Source != nil {
				mt = typed.Source.MediaType
			}
			lines = append(lines, toolRuntimeFormat(i18n.KeyToolRuntimeMCPSummaryImagePreview, imgIdx, mt))
			imgIdx++
		case types.TextBlock:
			lines = append(lines, toolRuntimeFormat(i18n.KeyToolRuntimeMCPSummaryTextPreview, txtIdx, truncatePreview(typed.Text, mcpTextPreviewMax)))
			txtIdx++
		default:
			lines = append(lines, toolRuntimeFormat(i18n.KeyToolRuntimeMCPSummaryBlockPreview, otherIdx, block.GetType()))
			otherIdx++
		}
	}
	return strings.Join(lines, "\n")
}

// truncatePreview returns the first `max` runes of `s` followed by "…" when
// the original was longer. We count runes rather than bytes so multi-byte
// characters aren't sliced through their middle.
func truncatePreview(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}
