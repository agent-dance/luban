package tui

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/grindlemire/go-tui"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"

	"github.com/yuin/goldmark/extension"
)

// mdParser is a shared goldmark parser with GFM extensions enabled
// (table, strikethrough, task list).
var mdParser = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
	),
)

const (
	markdownIndentPrefix = "\u00a0\u00a0"
	markdownSpacerTag    = "markdown-spacer"
)

// --- Styles used for Markdown rendering ---
var (
	mdStyleNormal     = tui.NewStyle()
	mdStyleDim        = tui.NewStyle().Dim()
	mdStyleH1         = tui.NewStyle().Foreground(tui.Cyan).Bold()
	mdStyleH2         = tui.NewStyle().Foreground(tui.Cyan).Bold()
	mdStyleH3         = tui.NewStyle().Foreground(tui.Cyan)
	mdStyleH4         = tui.NewStyle().Foreground(tui.Cyan).Dim()
	mdStyleCode       = tui.NewStyle().Foreground(tui.BrightGreen)
	mdStyleBlockquote = tui.NewStyle().Dim().Italic()
	mdStyleLink       = tui.NewStyle().Foreground(tui.Blue).Underline()
	mdStyleListMarker = tui.NewStyle().Foreground(tui.Yellow)
	mdStyleHR         = tui.NewStyle().Dim()
)

type markdownRenderBlock struct {
	elements []*tui.Element
}

func newVerticalSpacer() *tui.Element {
	return tui.New(
		tui.WithTag(markdownSpacerTag),
		tui.WithText(""),
		tui.WithHeight(1),
		tui.WithWidthPercent(100),
	)
}

func appendVerticalSpacer(elements *[]*tui.Element) {
	if len(*elements) == 0 {
		return
	}
	last := (*elements)[len(*elements)-1]
	if last.Tag() == markdownSpacerTag {
		return
	}
	*elements = append(*elements, newVerticalSpacer())
}

func markdownIndent(level int) string {
	if level <= 0 {
		return ""
	}
	return strings.Repeat(markdownIndentPrefix, level)
}

func markdownIndentStepCells() int {
	return utf8.RuneCountInString(markdownIndentPrefix)
}

func markdownIndentCells(level int) int {
	if level <= 0 {
		return 0
	}
	return markdownIndentStepCells() * level
}

// renderMarkdown converts Markdown text into a slice of go-tui Elements.
//
// It uses goldmark to parse the Markdown into an AST, then walks the AST
// to produce styled Elements using go-tui's native WithTextStyle() API.
// This avoids ANSI escape codes entirely — go-tui's Cell-based renderer
// generates ANSI only at Flush() time.
func renderMarkdown(markdown string) []*tui.Element {
	return flattenMarkdownRenderBlocks(renderMarkdownBlocks(markdown))
}

func renderMarkdownBlocks(markdown string) []markdownRenderBlock {
	if strings.TrimSpace(markdown) == "" {
		return nil
	}

	src := []byte(markdown)
	reader := gmtext.NewReader(src)
	doc := mdParser.Parser().Parse(reader)
	return collectMarkdownRenderBlocks(doc, src, 0)
}

func collectMarkdownRenderBlocks(n ast.Node, src []byte, depth int) []markdownRenderBlock {
	var blocks []markdownRenderBlock
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		elements := renderMarkdownNode(child, src, depth)
		if len(elements) == 0 {
			continue
		}
		blocks = append(blocks, markdownRenderBlock{elements: elements})
	}
	return blocks
}

func flattenMarkdownRenderBlocks(blocks []markdownRenderBlock) []*tui.Element {
	var elements []*tui.Element
	for i, block := range blocks {
		elements = append(elements, block.elements...)
		if i < len(blocks)-1 {
			appendVerticalSpacer(&elements)
		}
	}
	return elements
}

// walkBlocks walks top-level block nodes and appends Elements.
func walkBlocks(n ast.Node, src []byte, depth int, elements *[]*tui.Element) {
	*elements = append(*elements, flattenMarkdownRenderBlocks(collectMarkdownRenderBlocks(n, src, depth))...)
}

func renderMarkdownNode(child ast.Node, src []byte, depth int) []*tui.Element {
	var elements []*tui.Element
	switch node := child.(type) {
	case *ast.Heading:
		renderHeading(node, src, &elements)
	case *ast.Paragraph:
		renderParagraph(node, src, depth, &elements)
	case *ast.TextBlock:
		// TextBlock is used in tight lists (no blank line between items).
		// It contains inline children just like Paragraph.
		renderParagraph(node, src, depth, &elements)
	case *ast.FencedCodeBlock:
		renderFencedCodeBlock(node, src, depth, &elements)
	case *ast.CodeBlock:
		renderCodeBlock(node, src, depth, &elements)
	case *ast.Blockquote:
		renderBlockquote(node, src, &elements)
	case *ast.List:
		renderList(node, src, depth, &elements)
	case *ast.ThematicBreak:
		elements = append(elements, tui.New(
			tui.WithHR(),
			tui.WithTextStyle(mdStyleHR),
			tui.WithWidthPercent(100),
		))
	case *ast.HTMLBlock:
		// Render HTML blocks as plain dim text
		text := extractLines(node, src)
		for _, line := range strings.Split(text, "\n") {
			elements = append(elements, tui.New(
				tui.WithText(line),
				tui.WithTextStyle(mdStyleDim),
				tui.WithWidthPercent(100),
			))
		}
	default:
		// Check for GFM Table
		if table, ok := child.(*east.Table); ok {
			renderTable(table, src, &elements)
		} else if child.HasChildren() {
			walkBlocks(child, src, depth, &elements)
		} else {
			text := markdownLeafText(child, src)
			if strings.TrimSpace(text) != "" {
				elements = append(elements, tui.New(
					tui.WithText(text),
					tui.WithWidthPercent(100),
				))
			}
		}
	}
	return elements
}

// renderHeading renders a heading with level-appropriate styling.
func renderHeading(node *ast.Heading, src []byte, elements *[]*tui.Element) {
	prefix := strings.Repeat("#", node.Level) + " "

	style := mdStyleH1
	switch {
	case node.Level == 2:
		style = mdStyleH2
	case node.Level == 3:
		style = mdStyleH3
	case node.Level >= 4:
		style = mdStyleH4
	}

	// Build spans: heading prefix + inline children (all in heading style)
	spans := []tui.StyledSpan{{Text: prefix, Style: style}}
	childSpans := extractInlineSpans(node, src, style)
	spans = append(spans, childSpans...)

	*elements = append(*elements, tui.New(
		tui.WithStyledSpans(spans),
		tui.WithWidthPercent(100),
	))
}

// renderParagraph renders a paragraph (or TextBlock in tight lists) using
// StyledSpans for rich inline formatting. Accepts ast.Node to handle both
// *ast.Paragraph and *ast.TextBlock uniformly.
func renderParagraph(node ast.Node, src []byte, depth int, elements *[]*tui.Element) {
	spans := extractInlineSpans(node, src, mdStyleNormal)
	if len(spans) == 0 {
		return
	}

	// Check if all text is whitespace
	allWhitespace := true
	for _, s := range spans {
		if strings.TrimSpace(s.Text) != "" {
			allWhitespace = false
			break
		}
	}
	if allWhitespace {
		return
	}

	opts := []tui.Option{
		tui.WithStyledSpans(spans),
		tui.WithWidthPercent(100),
	}
	if indent := markdownIndentCells(depth); indent > 0 {
		opts = append(opts, tui.WithPaddingTRBL(0, 0, 0, indent))
	}

	*elements = append(*elements, tui.New(opts...))
}

// renderFencedCodeBlock renders a fenced code block as a syntax-highlighted
// code panel with a header, line numbers, and full-width background.
func renderFencedCodeBlock(node *ast.FencedCodeBlock, src []byte, depth int, elements *[]*tui.Element) {
	lang := ""
	if node.Info != nil {
		lang = string(node.Info.Segment.Value(src))
		// Strip language tag from info string (e.g. "go" from "go\n")
		if idx := strings.IndexAny(lang, " \t\n"); idx >= 0 {
			lang = lang[:idx]
		}
	}

	code := extractLines(node, src)
	*elements = append(*elements, renderCodeBlockPanel(code, lang, depth)...)
}

// renderCodeBlock renders an indented code block. No language hint is
// available, so chroma falls back to content analysis / plaintext.
func renderCodeBlock(node *ast.CodeBlock, src []byte, depth int, elements *[]*tui.Element) {
	code := extractLines(node, src)
	*elements = append(*elements, renderCodeBlockPanel(code, "", depth)...)
}

// renderBlockquote renders a blockquote with "│ " prefix and dim styling.
func renderBlockquote(node *ast.Blockquote, src []byte, elements *[]*tui.Element) {
	// Recursively render children, then prefix each with "│ "
	var inner []*tui.Element
	walkBlocks(node, src, 0, &inner)

	prefix := markdownIndent(1) + "│ "
	for _, el := range inner {
		if el.HasStyledSpans() {
			// Rich text: prepend the blockquote prefix span and apply dim to all spans
			oldSpans := el.StyledSpans()
			newSpans := make([]tui.StyledSpan, 0, len(oldSpans)+1)
			newSpans = append(newSpans, tui.StyledSpan{Text: prefix, Style: mdStyleBlockquote})
			for _, s := range oldSpans {
				// Merge dim+italic into each span's style
				merged := s.Style.Dim().Italic()
				newSpans = append(newSpans, tui.StyledSpan{Text: s.Text, Style: merged})
			}
			*elements = append(*elements, tui.New(
				tui.WithStyledSpans(newSpans),
				tui.WithWidthPercent(100),
			))
		} else {
			text := el.Text()
			// If the inner element contains newlines, split into separate lines.
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				*elements = append(*elements, tui.New(
					tui.WithText(prefix+line),
					tui.WithTextStyle(mdStyleBlockquote),
					tui.WithWidthPercent(100),
				))
			}
		}
	}
}

// renderList renders ordered and unordered lists.
func renderList(node *ast.List, src []byte, depth int, elements *[]*tui.Element) {
	itemNum := node.Start
	if itemNum == 0 && node.IsOrdered() {
		itemNum = 1
	}

	indentPrefix := markdownIndentPrefix

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		listItem, ok := child.(*ast.ListItem)
		if !ok {
			continue
		}

		// Generate marker
		var marker string
		if node.IsOrdered() {
			marker = fmt.Sprintf("%d. ", itemNum)
			itemNum++
		} else {
			marker = "• "
		}

		// Render list item content
		var itemElements []*tui.Element
		walkBlocks(listItem, src, depth+1, &itemElements)

		if len(itemElements) > 0 {
			first := itemElements[0]
			outdentMarkdownElement(first, markdownIndentStepCells(), indentPrefix)

			if first.HasStyledSpans() {
				// Rich text: prepend marker span with yellow styling
				oldSpans := first.StyledSpans()
				newSpans := make([]tui.StyledSpan, 0, len(oldSpans)+1)
				newSpans = append(newSpans, tui.StyledSpan{Text: marker, Style: mdStyleListMarker})
				newSpans = append(newSpans, oldSpans...)
				first.SetStyledSpans(newSpans)
				*elements = append(*elements, first)
			} else {
				// Plain text: use TextPrefix for the marker
				firstText := strings.TrimPrefix(first.Text(), indentPrefix)
				first.SetText(marker + firstText)
				first.Apply(tui.WithTextPrefix(marker, mdStyleListMarker))
				*elements = append(*elements, first)
			}
			// Remaining elements (nested blocks)
			*elements = append(*elements, itemElements[1:]...)
			if len(itemElements) > 1 && child.NextSibling() != nil {
				appendVerticalSpacer(elements)
			}
		}
	}
}

func outdentMarkdownElement(el *tui.Element, cells int, textPrefix string) {
	if el == nil || cells <= 0 {
		return
	}

	style := el.Style()
	if style.Padding.Left >= cells {
		style.Padding.Left -= cells
		el.SetStyle(style)
	}

	if textPrefix == "" {
		return
	}

	if el.HasStyledSpans() {
		spans := append([]tui.StyledSpan(nil), el.StyledSpans()...)
		if len(spans) == 0 {
			return
		}
		trimmed := strings.TrimPrefix(spans[0].Text, textPrefix)
		if trimmed == spans[0].Text {
			return
		}
		if trimmed == "" {
			spans = spans[1:]
		} else {
			spans[0].Text = trimmed
		}
		el.SetStyledSpans(spans)
		return
	}

	trimmed := strings.TrimPrefix(el.Text(), textPrefix)
	if trimmed != el.Text() {
		el.SetText(trimmed)
	}
}

// renderTable renders a GFM table using native <table>/<tr>/<td>/<th> elements.
// The go-tui layout engine handles column sizing, and the render engine draws
// a complete grid (outer border + column separators + header separator).
// Columns are proportionally shrunk against the table's actual parent width
// during layout, including any space reserved for a vertical scrollbar.
func renderTable(table *east.Table, src []byte, elements *[]*tui.Element) {
	// Collect column alignments from the GFM Table node.
	alignments := table.Alignments

	// Collect all rows (header + body rows)
	type rowData struct {
		cells    [][]tui.StyledSpan
		isHeader bool
	}
	var rows []rowData

	for child := table.FirstChild(); child != nil; child = child.NextSibling() {
		switch section := child.(type) {
		case *east.TableHeader:
			var cells [][]tui.StyledSpan
			for cell := section.FirstChild(); cell != nil; cell = cell.NextSibling() {
				spans := extractInlineSpans(cell, src, mdStyleNormal)
				cells = append(cells, spans)
			}
			rows = append(rows, rowData{cells: cells, isHeader: true})
		case *east.TableRow:
			var cells [][]tui.StyledSpan
			for cell := section.FirstChild(); cell != nil; cell = cell.NextSibling() {
				spans := extractInlineSpans(cell, src, mdStyleNormal)
				cells = append(cells, spans)
			}
			rows = append(rows, rowData{cells: cells, isHeader: false})
		}
	}

	if len(rows) == 0 {
		return
	}

	// Compute number of columns
	numCols := 0
	for _, r := range rows {
		if len(r.cells) > numCols {
			numCols = len(r.cells)
		}
	}

	// Count header rows
	headerRows := 0
	for _, r := range rows {
		if r.isHeader {
			headerRows++
		} else {
			break // headers are always at the top
		}
	}

	// Map GFM alignment to tui.TextAlign
	colAlign := make([]tui.TextAlign, numCols)
	for i := 0; i < numCols; i++ {
		if i < len(alignments) {
			switch alignments[i] {
			case east.AlignCenter:
				colAlign[i] = tui.TextAlignCenter
			case east.AlignRight:
				colAlign[i] = tui.TextAlignRight
			default:
				colAlign[i] = tui.TextAlignLeft
			}
		}
	}

	// Build <tr> children
	var trElements []*tui.Element
	for _, r := range rows {
		var cellElements []*tui.Element
		for colIdx := 0; colIdx < numCols; colIdx++ {
			tag := "td"
			if r.isHeader {
				tag = "th"
			}

			var cellOpts []tui.Option
			cellOpts = append(cellOpts, tui.WithTag(tag))

			if colIdx < len(r.cells) && len(r.cells[colIdx]) > 0 {
				cellSpans := r.cells[colIdx]
				if r.isHeader {
					// Apply bold to header spans
					boldSpans := make([]tui.StyledSpan, len(cellSpans))
					for i, s := range cellSpans {
						boldSpans[i] = tui.StyledSpan{Text: s.Text, Style: s.Style.Bold()}
					}
					cellOpts = append(cellOpts, tui.WithStyledSpans(boldSpans))
				} else {
					cellOpts = append(cellOpts, tui.WithStyledSpans(cellSpans))
				}
			}

			// Apply column alignment
			if colIdx < len(colAlign) {
				cellOpts = append(cellOpts, tui.WithTextAlign(colAlign[colIdx]))
			}

			cellOpts = append(cellOpts, tui.WithWrap(false), tui.WithTruncate(true))

			cellElements = append(cellElements, tui.New(cellOpts...))
		}

		tr := tui.New(
			tui.WithTag("tr"),
			tui.WithDirection(tui.Row),
			tui.WithChildren(cellElements...),
		)
		trElements = append(trElements, tr)
	}

	// Build the <table> element with left margin for indent.
	// No wrapper is used — the table sizes itself via IntrinsicSize
	// (sum of column widths + inter-column gaps + border), preventing
	// flex stretch from bloating it to the full container width/height.
	indentCells := len([]rune(markdownIndent(1)))
	tableOpts := []tui.Option{
		tui.WithTag("table"),
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(mdStyleDim),
		tui.WithChildren(trElements...),
		tui.WithTableHeaderRows(headerRows),
		tui.WithTableRowSeparator(true),
		tui.WithMarginTRBL(0, 0, 0, indentCells),
		tui.WithAlignSelf(tui.AlignStart), // prevent flex stretch from bloating width
	}
	tableEl := tui.New(tableOpts...)

	*elements = append(*elements, tableEl)
}

// --- Inline text extraction ---

// --- Rich text (StyledSpan) extraction ---

// extractInlineSpans converts inline AST nodes into StyledSpan slices.
// This preserves formatting (bold, italic, inline code, links, strikethrough)
// as individual spans, each carrying the correct style.
func extractInlineSpans(node ast.Node, src []byte, baseStyle tui.Style) []tui.StyledSpan {
	var spans []tui.StyledSpan
	extractInlineSpansRec(node, src, baseStyle, &spans)
	// Merge adjacent spans with same style to reduce span count.
	return mergeAdjacentSpans(spans)
}

func extractInlineSpansRec(node ast.Node, src []byte, style tui.Style, spans *[]tui.StyledSpan) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			text := string(n.Segment.Value(src))
			if n.SoftLineBreak() {
				text += " "
			}
			if n.HardLineBreak() {
				text += "\n"
			}
			if text != "" {
				*spans = append(*spans, tui.StyledSpan{Text: text, Style: style})
			}
		case *ast.String:
			if len(n.Value) > 0 {
				*spans = append(*spans, tui.StyledSpan{Text: string(n.Value), Style: style})
			}
		case *ast.CodeSpan:
			// Inline code: `code` with bright green
			var buf bytes.Buffer
			extractCodeSpanText(n, src, &buf)
			codeText := "`" + buf.String() + "`"
			*spans = append(*spans, tui.StyledSpan{Text: codeText, Style: mdStyleCode})
		case *ast.Emphasis:
			// Emphasis: level 1 = italic, level 2 = bold
			childStyle := style
			if n.Level == 1 {
				childStyle = childStyle.Italic()
			} else if n.Level >= 2 {
				childStyle = childStyle.Bold()
			}
			extractInlineSpansRec(n, src, childStyle, spans)
		case *ast.Link:
			// Link text in blue+underline, then " (url)" in link style
			extractInlineSpansRec(n, src, mdStyleLink, spans)
			if len(n.Destination) > 0 {
				*spans = append(*spans, tui.StyledSpan{
					Text:  " (" + string(n.Destination) + ")",
					Style: mdStyleLink,
				})
			}
		case *ast.Image:
			*spans = append(*spans, tui.StyledSpan{Text: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyMarkdownImagePrefix), Style: style})
			extractInlineSpansRec(n, src, style, spans)
			*spans = append(*spans, tui.StyledSpan{Text: "]", Style: style})
		case *ast.AutoLink:
			url := string(n.URL(src))
			*spans = append(*spans, tui.StyledSpan{Text: url, Style: mdStyleLink})
		case *ast.RawHTML:
			var buf bytes.Buffer
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				buf.Write(seg.Value(src))
			}
			if buf.Len() > 0 {
				*spans = append(*spans, tui.StyledSpan{Text: buf.String(), Style: mdStyleDim})
			}
		default:
			// Check for GFM Strikethrough
			if _, ok := child.(*east.Strikethrough); ok {
				childStyle := style.Strikethrough()
				extractInlineSpansRec(child, src, childStyle, spans)
			} else if child.HasChildren() {
				extractInlineSpansRec(child, src, style, spans)
			} else {
				text := markdownLeafText(child, src)
				if text != "" {
					*spans = append(*spans, tui.StyledSpan{Text: text, Style: style})
				}
			}
		}
	}
}

// mergeAdjacentSpans merges consecutive spans with the same style to reduce
// the number of style transitions during rendering.
func mergeAdjacentSpans(spans []tui.StyledSpan) []tui.StyledSpan {
	if len(spans) <= 1 {
		return spans
	}
	result := make([]tui.StyledSpan, 0, len(spans))
	current := spans[0]
	for _, s := range spans[1:] {
		if current.Style.Equal(s.Style) {
			current.Text += s.Text
		} else {
			result = append(result, current)
			current = s
		}
	}
	result = append(result, current)
	return result
}

// extractCodeSpanText extracts the text content of a code span.
func extractCodeSpanText(node *ast.CodeSpan, src []byte, buf *bytes.Buffer) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			buf.Write(t.Segment.Value(src))
		}
	}
}

// extractLines extracts the raw line content from a block node (code blocks).
func extractLines(node ast.Node, src []byte) string {
	var buf bytes.Buffer
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(src))
	}
	return normalizeCodeBlockText(buf.String())
}

func markdownLeafText(node ast.Node, src []byte) string {
	switch typed := node.(type) {
	case *ast.Text:
		return string(typed.Value(src))
	case *ast.String:
		return string(typed.Value)
	default:
		return extractLines(node, src)
	}
}

const codeBlockTabWidth = 4

// normalizeCodeBlockText rewrites raw code-block text into a terminal-safe form.
// Markdown fenced blocks preserve bytes verbatim, which means control characters
// like CR and TAB can leak into the render stream. Real terminals execute those
// controls instead of painting glyphs, so the diff renderer's model of the
// screen diverges from what was actually drawn.
func normalizeCodeBlockText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var b strings.Builder
	b.Grow(len(text))

	col := 0
	for _, r := range text {
		switch {
		case r == '\n':
			b.WriteRune('\n')
			col = 0
		case r == '\t':
			spaces := codeBlockTabWidth - (col % codeBlockTabWidth)
			if spaces == 0 {
				spaces = codeBlockTabWidth
			}
			for range spaces {
				b.WriteByte(' ')
			}
			col += spaces
		case r < 0x20 || (r >= 0x7f && r < 0xa0):
			// Keep code blocks visually stable by neutralising remaining C0/C1
			// control characters before they reach the terminal output layer.
			b.WriteByte(' ')
			col++
		default:
			b.WriteRune(r)
			col += tui.RuneWidth(r)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// renderPlainText is a fallback when Markdown parsing fails.
// It renders text as plain unstyled lines.
func renderPlainText(text string) []*tui.Element {
	lines := strings.Split(text, "\n")
	elements := make([]*tui.Element, 0, len(lines))
	for _, line := range lines {
		elements = append(elements, tui.New(
			tui.WithText(line),
			tui.WithWidthPercent(100),
		))
	}
	return elements
}

// --- Diff rendering ---
//
// Diff content is NOT standard Markdown, so we handle it separately.
// These functions provide colored diff output (green for additions,
// red for deletions, cyan for hunk headers).

// renderDiffLines takes text that looks like a unified diff and renders it with
// green (additions) and red (deletions) coloring, plus dim context lines.
func renderDiffLines(text string) []*tui.Element {
	lines := strings.Split(text, "\n")
	elements := make([]*tui.Element, 0, len(lines))

	for _, line := range lines {
		style := tui.NewStyle().Dim()
		switch {
		case strings.HasPrefix(line, "+"):
			style = tui.NewStyle().Foreground(tui.Green)
		case strings.HasPrefix(line, "-"):
			style = tui.NewStyle().Foreground(tui.Red)
		case strings.HasPrefix(line, "@@"):
			style = tui.NewStyle().Foreground(tui.Cyan)
		}
		elements = append(elements, tui.New(
			tui.WithText(markdownIndent(1)+line),
			tui.WithTextStyle(style),
			tui.WithWidthPercent(100),
		))
	}
	return elements
}

// isDiffContent heuristically checks if text looks like a unified diff.
func isDiffContent(text string) bool {
	lines := strings.Split(text, "\n")
	diffIndicators := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "diff ") {
			diffIndicators++
		}
	}
	return diffIndicators >= 2
}
