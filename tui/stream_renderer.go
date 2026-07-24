package tui

import (
	"bytes"
	"strings"
	"sync"

	"github.com/grindlemire/go-tui"
)

// StreamRenderer provides incremental (block-level) Markdown rendering.
// Instead of re-rendering the entire accumulated text on every token, it:
//
//  1. Splits input into Markdown blocks (paragraphs, fenced code blocks,
//     headings, etc.) using lightweight boundary detection.
//  2. Renders each completed block once via goldmark/go-tui and caches the
//     resulting Elements.
//  3. For Elements(), renders the pending (incomplete) block through goldmark
//     together with cached block sources, so styles appear incrementally
//     during streaming. The pending portion is typically small (one paragraph
//     or partial table), so re-rendering on each debounce tick is cheap.
//  4. On Finalize() (stream end), re-renders the entire text as a single
//     Markdown document to fix fragmentation artifacts from streaming.
//
// This brings the rendering cost from O(n²) (full re-render per token) down to
// roughly O(n) — each completed block is rendered exactly once and cached,
// and the pending tail is re-rendered cheaply (~0.1ms for 100 lines) at ~20Hz.
//
// StreamRenderer is protected by an internal RWMutex: Feed/Finalize take a
// write lock, Lines/Elements takes a read lock. This ensures the render goroutine
// can safely read while a debounce timer goroutine calls Feed().
type StreamRenderer struct {
	mu sync.RWMutex

	// blocks holds the go-tui Elements for each completed block.
	blocks []renderedBlock

	// pending accumulates text that hasn't yet formed a complete block.
	// Uses bytes.Buffer to avoid O(n²) string concatenation.
	pending bytes.Buffer

	// finalized is set to true after Finalize() is called; further Feed()
	// calls are no-ops.
	finalized bool
}

// renderedBlock pairs a source Markdown block with its rendered Elements.
type renderedBlock struct {
	source string                // original Markdown source chunk, preserved verbatim
	blocks []markdownRenderBlock // rendered top-level Markdown blocks for this chunk
}

func trimMarkdownEdges(text string) string {
	return strings.Trim(text, "\r\n")
}

// NewStreamRenderer creates a StreamRenderer.
func NewStreamRenderer() *StreamRenderer {
	return &StreamRenderer{}
}

// Feed appends a streaming token to the renderer and processes any newly
// completed blocks. Returns true if the output changed (caller should redraw).
func (sr *StreamRenderer) Feed(token string) bool {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.finalized {
		return false
	}
	sr.pending.WriteString(token)
	return sr.processBlocks()
}

// processBlocks scans sr.pending for complete block boundaries. Each complete
// block is rendered via renderMarkdown and moved to sr.blocks. Returns true if any
// new blocks were committed or if pending text changed (new token appended).
func (sr *StreamRenderer) processBlocks() bool {
	changed := false
	for {
		boundary := sr.findBlockBoundary()
		if boundary < 0 {
			break
		}
		// Extract the completed block from pending
		data := sr.pending.Bytes()
		block := string(data[:boundary])

		// Rebuild pending with remaining data.
		remaining := make([]byte, len(data)-boundary)
		copy(remaining, data[boundary:])
		sr.pending.Reset()
		sr.pending.Write(remaining)

		sr.renderAndCache(block)
		changed = true
	}
	// Even if no block was committed, the pending text grew (new token),
	// so the output of Lines() will differ — signal a change.
	return changed
}

// countLeadingBackticks returns the number of leading backtick characters in s.
func countLeadingBackticks(s string) int {
	n := 0
	for _, c := range s {
		if c == '`' {
			n++
		} else {
			break
		}
	}
	return n
}

// findBlockBoundary scans sr.pending for the first complete block boundary.
// Returns the byte offset just past the boundary (so pending[:offset] is the
// complete block including its trailing delimiter), or -1 if no complete block
// is found.
//
// This always scans from the beginning of pending with fresh fence state.
// While this is technically O(n) per Feed() call on the pending buffer, the
// pending buffer is typically small (one paragraph at most before a block
// boundary is found), and the simplicity avoids subtle bugs with state
// tracking across incremental feeds. The real performance win is in the
// block-level caching (rendering each block only once), not in skipping a
// few string scans.
//
// Block boundary rules:
//   - Fenced code block: starts with a line beginning with N backticks (N >= 3,
//     with optional language tag), ends with a line that starts with >= N
//     backticks (plus optional whitespace). This correctly handles nested fences
//     (e.g. ```` containing ``` inside).
//   - Outside a fence, a blank line (\n\n) separates blocks.
//   - A heading line (# at start of line) following a \n also starts a new
//     block (the previous content becomes one block).
func (sr *StreamRenderer) findBlockBoundary() int {
	data := sr.pending.Bytes()
	if len(data) == 0 {
		return -1
	}

	// Always start with fresh fence state — we scan the entire pending
	// from scratch each time. This avoids cross-call state bugs where
	// a fence-opening line is re-interpreted as a close.
	inFence := false
	fenceLen := 0

	i := 0
	for i < len(data) {
		lineStart := i

		// Find end of current line
		nl := bytes.IndexByte(data[i:], '\n')
		if nl < 0 {
			// No newline yet — line is incomplete, can't determine boundary
			break
		}
		lineEnd := i + nl      // index of the \n
		line := data[i : i+nl] // line content without \n
		nextLineStart := lineEnd + 1

		trimmedLine := bytes.TrimSpace(line)

		// --- Fenced code block handling ---
		backtickCount := countLeadingBackticks(string(trimmedLine))
		if backtickCount >= 3 {
			if inFence {
				// Check if this closes the fence
				if backtickCount >= fenceLen {
					rest := bytes.TrimSpace(trimmedLine[backtickCount:])
					if len(rest) == 0 {
						// Closes the fence
						inFence = false
						fenceLen = 0
						end := nextLineStart
						if end < len(data) && data[end] == '\n' {
							end++
						}
						return end
					}
				}
				i = nextLineStart
				continue
			}
			// Opens a new fence. If there's content before this line,
			// that content is a complete block.
			if lineStart > 0 {
				return lineStart
			}
			inFence = true
			fenceLen = backtickCount
			i = nextLineStart
			continue
		}

		if inFence {
			i = nextLineStart
			continue
		}

		// --- Double newline (blank line) → block boundary ---
		if nextLineStart < len(data) {
			if data[nextLineStart] == '\n' {
				end := nextLineStart + 1
				for end < len(data) && data[end] == '\n' {
					end++
				}
				return end
			}

			// --- Heading after newline starts a new block ---
			if data[nextLineStart] == '#' && lineStart > 0 {
				return nextLineStart
			}
		}

		i = nextLineStart
	}

	return -1
}

// renderAndCache renders a Markdown block via renderMarkdown and appends to sr.blocks.
func (sr *StreamRenderer) renderAndCache(block string) {
	if strings.TrimSpace(block) == "" {
		return
	}

	renderBlocks := renderMarkdownBlocks(block)
	if len(renderBlocks) == 0 {
		trimmed := trimMarkdownEdges(block)
		renderBlocks = []markdownRenderBlock{{
			elements: renderPlainText(trimmed),
		}}
	}

	sr.blocks = append(sr.blocks, renderedBlock{
		source: block,
		blocks: renderBlocks,
	})
}

// Finalize renders any remaining pending text. Call this when the
// stream ends to ensure the last (possibly incomplete) block gets full
// Markdown rendering.
//
// It also re-renders the entire accumulated text as a single Markdown document.
// During streaming, blocks are committed individually which can cause tiny
// fragments (e.g. a lone "h" token separated by blank lines) to become
// permanent single-character blocks.  Re-rendering the full text lets goldmark
// merge such fragments with their surrounding context, producing the correct
// final output.
func (sr *StreamRenderer) Finalize() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.finalized {
		return
	}
	sr.finalized = true

	fullText := sr.fullSourceLocked(sr.pending.String())
	trimmedFull := trimMarkdownEdges(fullText)
	if strings.TrimSpace(trimmedFull) == "" {
		return
	}

	renderBlocks := renderMarkdownBlocks(fullText)
	if len(renderBlocks) == 0 {
		renderBlocks = []markdownRenderBlock{{
			elements: renderPlainText(trimmedFull),
		}}
	}

	// Replace all cached blocks with the fresh render.
	sr.blocks = []renderedBlock{{
		source: fullText,
		blocks: renderBlocks,
	}}
	sr.pending.Reset()
}

// Lines returns the current rendered output as a slice of display lines.
// Each line is a plain text string (no ANSI escape codes).
//
// Cached (completed) blocks use renderMarkdown output; the pending tail uses
// plain text. Uses RLock so multiple render goroutine reads don't block each other.
func (sr *StreamRenderer) Lines() []string {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	var lines []string

	// Emit cached blocks — extract text from cached Elements
	appendElementLines(&lines, flattenMarkdownRenderBlocks(sr.cachedRenderBlocksLocked()))

	// Emit pending text as plain text (lightweight, no rendering)
	if pending := strings.TrimRight(sr.pending.String(), "\n"); pending != "" {
		pendingLines := strings.Split(pending, "\n")
		lines = append(lines, pendingLines...)
	}

	return lines
}

func appendElementLines(lines *[]string, elements []*tui.Element) {
	for _, el := range elements {
		if el == nil {
			continue
		}
		*lines = append(*lines, el.Text())
		if len(el.Children()) > 0 {
			appendElementLines(lines, el.Children())
		}
	}
}

// Elements returns the current rendered output as go-tui Elements.
// This is the preferred method for rendering in the TUI, as it preserves
// styling information.
//
// Cached (completed) blocks use full Markdown rendering. The pending tail
// (text that hasn't formed a complete block yet) is also rendered through
// goldmark so that styles (headings, bold, tables, etc.) appear incrementally
// during streaming rather than "popping" in only after a block boundary.
//
// When there is pending text, we re-render the *entire* accumulated document
// (cached block sources + pending) through goldmark. This sounds expensive
// but is safe because:
//   - The pending buffer is typically small (one paragraph or partial table).
//   - goldmark parsing is ~0.1ms for 100 lines of Markdown.
//   - The 50ms debounce interval limits this to ~20 calls/s.
//   - Once a block boundary is found, the completed block is cached and the
//     pending buffer shrinks back to near-zero, so the re-rendered portion
//     never grows unboundedly.
//
// When there is NO pending text (all blocks are committed), we simply
// concatenate the cached Elements — zero goldmark overhead.
func (sr *StreamRenderer) Elements() []*tui.Element {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	pending := sr.pending.String()

	// Fast path: no pending text — return cached block Elements directly.
	if strings.TrimSpace(pending) == "" {
		return flattenMarkdownRenderBlocks(sr.cachedRenderBlocksLocked())
	}

	// Slow path: pending text exists — re-render full document via goldmark
	// so the pending portion gets proper Markdown styling (headings, bold,
	// tables, code fences, etc.) instead of appearing as raw plain text.
	fullText := sr.fullSourceLocked(pending)
	trimmedFull := trimMarkdownEdges(fullText)
	if strings.TrimSpace(trimmedFull) == "" {
		return nil
	}

	renderBlocks := renderMarkdownBlocks(fullText)
	if len(renderBlocks) > 0 {
		return flattenMarkdownRenderBlocks(renderBlocks)
	}

	// Fallback: goldmark produced nothing — emit cached + plain text pending.
	fallback := flattenMarkdownRenderBlocks(sr.cachedRenderBlocksLocked())
	pendingLines := strings.Split(strings.TrimRight(pending, "\n"), "\n")
	for _, line := range pendingLines {
		fallback = append(fallback, tui.New(
			tui.WithText(line),
			tui.WithWidthPercent(100),
		))
	}
	return fallback
}

func (sr *StreamRenderer) cachedRenderBlocksLocked() []markdownRenderBlock {
	var blocks []markdownRenderBlock
	for _, block := range sr.blocks {
		blocks = append(blocks, block.blocks...)
	}
	return blocks
}

func (sr *StreamRenderer) fullSourceLocked(pending string) string {
	var full strings.Builder
	for _, block := range sr.blocks {
		full.WriteString(block.source)
	}
	full.WriteString(pending)
	return full.String()
}

// Reset clears all state, allowing the StreamRenderer to be reused.
func (sr *StreamRenderer) Reset() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.blocks = sr.blocks[:0]
	sr.pending.Reset()
	sr.finalized = false
}

// IsFinalized returns whether Finalize() has been called.
func (sr *StreamRenderer) IsFinalized() bool {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.finalized
}
