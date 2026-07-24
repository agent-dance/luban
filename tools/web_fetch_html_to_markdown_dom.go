// Package tools — DOM-aware HTML→Markdown converter.
//
// Mirrors the behaviour of turndown (used by the TS WebFetchTool) more
// closely than the regex pipeline in web_fetch_html_to_markdown.go. The
// regex converter remains the production default and is exposed via
// HTMLToMarkdown / HTMLToMarkdownWithOptions; the DOM-aware variant lives
// here as HTMLToMarkdownDOM and is used when:
//
//   - the host has set CLAUDE_WEBFETCH_DOM_MARKDOWN=1, or
//   - a caller wires WebFetchTool.UseDOMMarkdown via WithDOMMarkdown().
//
// The walker uses golang.org/x/net/html which is already in the dependency
// graph, so no new direct dependency is introduced. Edge cases the regex
// path mishandles (nested ul/ol indentation, tables with thead, code
// blocks with `language-foo` classes, escaped pre-content) are addressed
// by structural traversal.
package tools

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/net/html"
)

// HTMLToMarkdownDOM converts the supplied HTML fragment to Markdown via
// a structural DOM walk. The output is post-processed through the same
// truncation logic the regex converter uses so callers can interchange
// implementations.
func HTMLToMarkdownDOM(htmlInput string) string {
	if htmlInput == "" {
		return ""
	}
	root, err := html.Parse(strings.NewReader(htmlInput))
	if err != nil {
		// Fall back to regex pipeline rather than fail the request.
		return HTMLToMarkdown(htmlInput)
	}
	var buf strings.Builder
	walkDOMMarkdown(root, &buf, domWalkState{})
	out := buf.String()
	// Collapse 3+ newlines to 2 (paragraph break) and trim.
	out = htmlReBlankLines.ReplaceAllString(out, "\n\n")
	out = strings.TrimSpace(out)
	if len(out) > MaxMarkdownBytes {
		out = out[:MaxMarkdownBytes] + markdownTruncationMarker
	}
	return out
}

// domWalkState tracks indentation for nested lists and the active list
// type so li renders the right marker.
type domWalkState struct {
	listDepth int
	// ordered stack: each entry true=ol, false=ul
	listKinds []bool
	// per-ol counter stack
	olCounters []int
	inPre      bool
}

func (s domWalkState) clone() domWalkState {
	out := s
	out.listKinds = append([]bool(nil), s.listKinds...)
	out.olCounters = append([]int(nil), s.olCounters...)
	return out
}

// walkDOMMarkdown recursively converts the node tree to Markdown.
func walkDOMMarkdown(n *html.Node, buf *strings.Builder, state domWalkState) {
	if n == nil {
		return
	}
	switch n.Type {
	case html.TextNode:
		text := n.Data
		if !state.inPre {
			// Collapse runs of whitespace inside flow content.
			text = collapseDOMWhitespace(text)
		}
		buf.WriteString(text)
		return
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkDOMMarkdown(c, buf, state)
		}
		return
	case html.ElementNode:
		// fall through
	default:
		return
	}

	tag := strings.ToLower(n.Data)
	switch tag {
	case "script", "style", "head", "noscript", "nav", "footer", "svg":
		return
	case "br":
		buf.WriteString("  \n")
		return
	case "hr":
		buf.WriteString("\n\n---\n\n")
		return
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(tag[1] - '0')
		buf.WriteString("\n\n")
		buf.WriteString(strings.Repeat("#", level))
		buf.WriteString(" ")
		walkChildrenInline(n, buf, state)
		buf.WriteString("\n\n")
		return
	case "p":
		buf.WriteString("\n\n")
		walkChildrenInline(n, buf, state)
		buf.WriteString("\n\n")
		return
	case "blockquote":
		var inner strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkDOMMarkdown(c, &inner, state)
		}
		body := strings.TrimSpace(inner.String())
		if body == "" {
			return
		}
		buf.WriteString("\n\n")
		for _, line := range strings.Split(body, "\n") {
			buf.WriteString("> ")
			buf.WriteString(line)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
		return
	case "ul", "ol":
		newState := state.clone()
		newState.listDepth++
		newState.listKinds = append(newState.listKinds, tag == "ol")
		if tag == "ol" {
			newState.olCounters = append(newState.olCounters, 0)
		}
		buf.WriteString("\n")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkDOMMarkdown(c, buf, newState)
		}
		buf.WriteString("\n")
		return
	case "li":
		indent := strings.Repeat("  ", state.listDepth-1)
		marker := "- "
		if len(state.listKinds) > 0 && state.listKinds[len(state.listKinds)-1] {
			// Find the active counter for the topmost ol.
			if len(state.olCounters) > 0 {
				state.olCounters[len(state.olCounters)-1]++
				marker = fmt.Sprintf("%d. ", state.olCounters[len(state.olCounters)-1])
			}
		}
		buf.WriteString(indent)
		buf.WriteString(marker)
		var inner strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkDOMMarkdown(c, &inner, state)
		}
		// Strip leading paragraph break inside list items.
		body := strings.TrimSpace(inner.String())
		// Indent any continuation lines so nested content stays under
		// the same list item.
		body = strings.ReplaceAll(body, "\n", "\n"+indent+"  ")
		buf.WriteString(body)
		buf.WriteString("\n")
		return
	case "pre":
		// Collect language hint if present on a child <code>.
		lang := ""
		if c := firstElementChild(n, "code"); c != nil {
			lang = extractLangFromClass(getAttr(c, "class"))
		}
		var inner strings.Builder
		// Walk children but extract raw text rather than re-rendering.
		extractTextRaw(n, &inner)
		body := strings.TrimRight(inner.String(), "\n")
		buf.WriteString("\n\n```")
		if lang != "" {
			buf.WriteString(lang)
		}
		buf.WriteString("\n")
		buf.WriteString(body)
		buf.WriteString("\n```\n\n")
		return
	case "code":
		// Inline code (when not the only child of <pre>, which is handled above).
		if n.Parent != nil && strings.EqualFold(n.Parent.Data, "pre") {
			// Already captured by the <pre> branch; avoid double-rendering.
			return
		}
		buf.WriteString("`")
		extractTextRaw(n, buf)
		buf.WriteString("`")
		return
	case "b", "strong":
		buf.WriteString("**")
		walkChildrenInline(n, buf, state)
		buf.WriteString("**")
		return
	case "i", "em":
		buf.WriteString("*")
		walkChildrenInline(n, buf, state)
		buf.WriteString("*")
		return
	case "a":
		var inner strings.Builder
		walkChildrenInline(n, &inner, state)
		text := strings.TrimSpace(inner.String())
		href := getAttr(n, "href")
		switch {
		case text == "" && href == "":
			return
		case href == "":
			buf.WriteString(text)
		case text == "":
			buf.WriteString(href)
		default:
			fmt.Fprintf(buf, "[%s](%s)", text, href)
		}
		return
	case "img":
		alt := getAttr(n, "alt")
		src := getAttr(n, "src")
		if src == "" {
			return
		}
		fmt.Fprintf(buf, "![%s](%s)", alt, src)
		return
	case "table":
		renderDOMMarkdownTable(n, buf, state)
		return
	case "tr", "td", "th", "thead", "tbody", "tfoot", "caption":
		// Tables are rendered via renderDOMMarkdownTable above. Bare
		// trs that escape that path get their cells flattened.
		walkChildrenInline(n, buf, state)
		return
	default:
		// Generic block: recurse without adding markup so the children
		// surface unchanged.
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkDOMMarkdown(c, buf, state)
		}
	}
}

// walkChildrenInline walks the children of n preserving inline context (no
// paragraph wrapping). Used by elements whose semantics already imply a
// surrounding line break.
func walkChildrenInline(n *html.Node, buf *strings.Builder, state domWalkState) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkDOMMarkdown(c, buf, state)
	}
}

// renderDOMMarkdownTable emits a GitHub-flavoured Markdown table from a
// <table> node. Headers come from <thead>/<th>; rows from <tbody>/<tr>.
func renderDOMMarkdownTable(table *html.Node, buf *strings.Builder, state domWalkState) {
	var headers []string
	var rows [][]string
	var currentRow []string

	var visit func(*html.Node, *bool)
	inHeader := false
	visit = func(n *html.Node, _ *bool) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			switch tag {
			case "thead":
				inHeader = true
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					visit(c, nil)
				}
				inHeader = false
				return
			case "tr":
				currentRow = nil
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					visit(c, nil)
				}
				if inHeader || (len(headers) == 0 && hasOnlyTHCells(n)) {
					if len(headers) == 0 {
						headers = append([]string(nil), currentRow...)
					}
				} else if len(currentRow) > 0 {
					rows = append(rows, append([]string(nil), currentRow...))
				}
				currentRow = nil
				return
			case "th", "td":
				var cell strings.Builder
				walkChildrenInline(n, &cell, state)
				currentRow = append(currentRow, strings.TrimSpace(cell.String()))
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c, nil)
		}
	}
	visit(table, nil)

	if len(headers) == 0 && len(rows) == 0 {
		return
	}
	if len(headers) == 0 && len(rows) > 0 {
		// Promote the first row to header so we can emit a valid GFM table.
		headers = rows[0]
		rows = rows[1:]
	}
	buf.WriteString("\n\n")
	buf.WriteString("| ")
	buf.WriteString(strings.Join(headers, " | "))
	buf.WriteString(" |\n")
	buf.WriteString("|")
	for range headers {
		buf.WriteString("---|")
	}
	buf.WriteString("\n")
	for _, row := range rows {
		// Pad short rows so the column count matches.
		for len(row) < len(headers) {
			row = append(row, "")
		}
		buf.WriteString("| ")
		buf.WriteString(strings.Join(row, " | "))
		buf.WriteString(" |\n")
	}
	buf.WriteString("\n")
}

func hasOnlyTHCells(tr *html.Node) bool {
	hasCells := false
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		tag := strings.ToLower(c.Data)
		if tag == "td" {
			return false
		}
		if tag == "th" {
			hasCells = true
		}
	}
	return hasCells
}

func firstElementChild(n *html.Node, tag string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && strings.EqualFold(c.Data, tag) {
			return c
		}
	}
	return nil
}

func getAttr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func extractLangFromClass(class string) string {
	for _, tok := range strings.Fields(class) {
		if strings.HasPrefix(tok, "language-") {
			return strings.TrimPrefix(tok, "language-")
		}
		if strings.HasPrefix(tok, "lang-") {
			return strings.TrimPrefix(tok, "lang-")
		}
	}
	return ""
}

// extractTextRaw walks n and writes verbatim text content (used for <pre>
// blocks where whitespace must survive).
func extractTextRaw(n *html.Node, buf *strings.Builder) {
	if n == nil {
		return
	}
	if n.Type == html.TextNode {
		buf.WriteString(n.Data)
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractTextRaw(c, buf)
	}
}

func collapseDOMWhitespace(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

// useDOMMarkdownConverter reports whether the WebFetch local-fetch path
// should prefer the DOM-aware converter. Default OFF (regex pipeline)
// for backward-compat; opt-in via CLAUDE_WEBFETCH_DOM_MARKDOWN=1.
func useDOMMarkdownConverter() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_WEBFETCH_DOM_MARKDOWN"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
