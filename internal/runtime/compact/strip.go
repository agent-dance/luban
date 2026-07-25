package compact

import (
	"sort"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// StripImagesFromMessages replaces image and document blocks with text markers
// before sending messages to the summarization LLM. This prevents base64
// image/document data from consuming the compaction API's own context window.
// Matches the original TS stripImagesFromMessages() in compact.ts, which
// handles both top-level image/document blocks AND image/document blocks
// nested inside tool_result content arrays.
func StripImagesFromMessages(messages []types.Message) []types.Message {
	result := make([]types.Message, len(messages))
	for i, msg := range messages {
		if msg.Role != types.RoleUser {
			result[i] = msg
			continue
		}

		hasMedia := false
		newContent := make([]types.ContentBlock, 0, len(msg.Content))

		for _, block := range msg.Content {
			switch typed := block.(type) {
			case types.ImageBlock:
				hasMedia = true
				newContent = append(newContent, types.TextBlock{
					Type: types.ContentTypeText,
					Text: "[image]",
				})
			case types.DocumentBlock:
				hasMedia = true
				newContent = append(newContent, types.TextBlock{
					Type: types.ContentTypeText,
					Text: "[document]",
				})
			case types.ToolResultBlock:
				if !typed.HasStructuredContent() {
					newContent = append(newContent, block)
					continue
				}
				replaced, nestedMedia := stripMediaFromToolResultContent(typed)
				hasMedia = hasMedia || nestedMedia
				newContent = append(newContent, replaced)
			default:
				newContent = append(newContent, block)
			}
		}

		if hasMedia {
			transformed := msg
			transformed.Content = newContent
			result[i] = preserveInternalControlAfterTransform(msg, transformed)
		} else {
			result[i] = msg
		}
	}
	return result
}

// Per-message aggregate budget constants.
const (
	// MaxToolResultsPerMessageChars is the maximum aggregate size in characters
	// for all tool_result blocks within a single user message. When exceeded,
	// the largest blocks are replaced with truncated previews.
	// Matches MAX_TOOL_RESULTS_PER_MESSAGE_CHARS in the original TS.
	MaxToolResultsPerMessageChars = 200_000

	// perMessagePreviewSize is how many characters of content to keep when
	// a tool result is truncated due to the per-message aggregate budget.
	perMessagePreviewSize = 2000
)

// EnforcePerMessageBudget checks each user message and, when the aggregate
// tool_result content exceeds MaxToolResultsPerMessageChars, truncates the
// largest results until under budget. This prevents N parallel tool calls
// from collectively producing e.g. 10 × 40K = 400K in one turn.
//
// NOTE: This is a Go-specific defensive measure for the compact path. The
// original TS applies per-message budget enforcement in the main query loop
// (query.ts) via enforceToolResultBudget / ContentReplacementState, NOT in
// the compact path. The TS version is stateful (persists across turns for
// prompt-cache stability), checks isContentAlreadyCompacted, and handles
// array-typed tool_result content — this simplified variant is intentionally
// scoped to the compact path only and operates statelessly.
//
// Returns a new message slice — does not modify the input.
func EnforcePerMessageBudget(messages []types.Message) []types.Message {
	lang := i18n.DetectOrLoadLanguage()
	result := make([]types.Message, len(messages))
	for i, msg := range messages {
		if msg.Role != types.RoleUser {
			result[i] = msg
			continue
		}

		// Measure aggregate tool result size in this message.
		type trPos struct {
			blockIdx int
			size     int
		}
		var positions []trPos
		totalChars := 0

		for j, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok {
				size := len(tr.TextContent())
				positions = append(positions, trPos{j, size})
				totalChars += size
			}
		}

		if totalChars <= MaxToolResultsPerMessageChars || len(positions) == 0 {
			result[i] = msg
			continue
		}

		// Sort by size descending — truncate the biggest first.
		sort.Slice(positions, func(a, b int) bool {
			return positions[a].size > positions[b].size
		})

		// Build a set of block indices to truncate.
		truncateSet := make(map[int]bool)
		excess := totalChars - MaxToolResultsPerMessageChars

		for _, pos := range positions {
			if excess <= 0 {
				break
			}
			// Truncating this block saves (size - previewSize) chars.
			savings := pos.size - perMessagePreviewSize
			if savings <= 0 {
				continue
			}
			truncateSet[pos.blockIdx] = true
			excess -= savings
		}

		if len(truncateSet) == 0 {
			result[i] = msg
			continue
		}

		// Build new content with truncated blocks.
		newContent := make([]types.ContentBlock, len(msg.Content))
		for j, block := range msg.Content {
			if truncateSet[j] {
				tr := block.(types.ToolResultBlock)
				preview := tr.TextContent()
				if len(preview) > perMessagePreviewSize {
					preview = preview[:perMessagePreviewSize]
				}
				tr.Content = i18n.Format(lang, i18n.KeyAuxCompactBudgetTruncated, preview, len(tr.TextContent()))
				tr.ContentBlocks = nil
				newContent[j] = tr
			} else {
				newContent[j] = block
			}
		}
		transformed := msg
		transformed.Content = newContent
		result[i] = preserveInternalControlAfterTransform(msg, transformed)
	}
	return result
}

func stripMediaFromToolResultContent(tr types.ToolResultBlock) (types.ToolResultBlock, bool) {
	if !tr.HasStructuredContent() {
		return tr, false
	}
	updated := tr
	updated.ContentBlocks = make([]types.ContentBlock, 0, len(tr.ContentBlocks))
	hasMedia := false
	for _, block := range tr.ContentBlocks {
		switch block.GetType() {
		case types.ContentTypeImage:
			hasMedia = true
			updated.ContentBlocks = append(updated.ContentBlocks, types.TextBlock{
				Type: types.ContentTypeText,
				Text: "[image]",
			})
		case types.ContentTypeDocument:
			hasMedia = true
			updated.ContentBlocks = append(updated.ContentBlocks, types.TextBlock{
				Type: types.ContentTypeText,
				Text: "[document]",
			})
		default:
			updated.ContentBlocks = append(updated.ContentBlocks, block)
		}
	}
	return updated, hasMedia
}
