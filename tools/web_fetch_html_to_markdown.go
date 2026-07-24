// Package tools — HTML → Markdown converter for the legacy WebFetch path.
//
// Mirrors the role of turndown in src/tools/WebFetchTool/utils.ts:
// when the server-side fetch isn't available we still want to feed the
// summariser readable markdown rather than raw HTML. This is intentionally
// a small, dependency-light converter — it does not need to be perfect,
// just good enough that paragraphs, headings, lists, code blocks, and
// links survive in a form Haiku-class models can read.
package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MaxMarkdownBytes mirrors TS MAX_MARKDOWN_LENGTH (100KB).
const MaxMarkdownBytes = 100 * 1024

const markdownTruncationMarker = "\n\n[Content truncated due to length...]"

var (
	htmlReComment       = regexp.MustCompile(`(?is)<!--.*?-->`)
	htmlReScript        = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	htmlReStyle         = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlReNav           = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	htmlReFooter        = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	htmlReNoscript      = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	htmlRePre           = regexp.MustCompile(`(?is)<pre([^>]*)>(.*?)</pre>`)
	htmlReCodeBlock     = regexp.MustCompile(`(?is)<code([^>]*)>(.*?)</code>`)
	htmlReHeading       = regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]>`)
	htmlReParagraph     = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	htmlReBr            = regexp.MustCompile(`(?is)<br\s*/?>`)
	htmlReHr            = regexp.MustCompile(`(?is)<hr\s*/?>`)
	htmlReBoldB         = regexp.MustCompile(`(?is)<b[^>]*>(.*?)</b>`)
	htmlReBoldStrong    = regexp.MustCompile(`(?is)<strong[^>]*>(.*?)</strong>`)
	htmlReItalicI       = regexp.MustCompile(`(?is)<i[^>]*>(.*?)</i>`)
	htmlReItalicEm      = regexp.MustCompile(`(?is)<em[^>]*>(.*?)</em>`)
	htmlReListItem      = regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
	htmlReUnorderedList = regexp.MustCompile(`(?is)<ul[^>]*>(.*?)</ul>`)
	htmlReOrderedList   = regexp.MustCompile(`(?is)<ol[^>]*>(.*?)</ol>`)
	htmlReBlockquote    = regexp.MustCompile(`(?is)<blockquote[^>]*>(.*?)</blockquote>`)
	htmlReAnchorMD      = regexp.MustCompile(`(?is)<a[^>]*\bhref\s*=\s*"([^"]*)"[^>]*>(.*?)</a>`)
	htmlReImage         = regexp.MustCompile(`(?is)<img[^>]*\balt\s*=\s*"([^"]*)"[^>]*\bsrc\s*=\s*"([^"]*)"[^>]*/?>`)
	htmlReImageNoAlt    = regexp.MustCompile(`(?is)<img[^>]*\bsrc\s*=\s*"([^"]*)"[^>]*/?>`)
	htmlReHeadBlock     = regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	htmlReSvg           = regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`)
	htmlReGenericTag    = regexp.MustCompile(`</?(?:div|span|section|article|main|aside|header|figure|figcaption|table|thead|tbody|tfoot|tr|td|th|caption|form|input|label|button|select|option|small|sub|sup|abbr|kbd|cite|time|mark|details|summary)[^>]*>`)
	htmlReAnyTag        = regexp.MustCompile(`<[^>]+>`)
	htmlReWhitespaceRun = regexp.MustCompile(`[ \t]+`)
	htmlReBlankLines    = regexp.MustCompile(`\n{3,}`)
	htmlReLangClass     = regexp.MustCompile(`(?i)(?:^|\s)(?:class\s*=\s*"[^"]*\blanguage-([a-zA-Z0-9_+\-]+)\b[^"]*"|class\s*=\s*'[^']*\blanguage-([a-zA-Z0-9_+\-]+)\b[^']*')`)
	htmlReTable         = regexp.MustCompile(`(?is)<table[^>]*>(.*?)</table>`)
	htmlReTableRow      = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	htmlReTableHeader   = regexp.MustCompile(`(?is)<th[^>]*>(.*?)</th>`)
	htmlReTableCell     = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
)

// htmlEntities lists the entity replacements we apply *after* tag stripping.
// We deliberately keep the set short — anything we miss simply remains as
// literal text, which is far better than mangling the page.
var htmlEntities = map[string]string{
	"&amp;":   "&",
	"&lt;":    "<",
	"&gt;":    ">",
	"&quot;":  "\"",
	"&#39;":   "'",
	"&apos;":  "'",
	"&nbsp;":  " ",
	"&hellip": "…",
	"&mdash;": "—",
	"&ndash;": "–",
	"&copy;":  "©",
	"&reg;":   "®",
	"&trade;": "™",
}

var htmlReNumericEntity = regexp.MustCompile(`&#(x?)([0-9a-fA-F]+);`)

// HTMLToMarkdownOptions tunes the converter for tests and special cases.
type HTMLToMarkdownOptions struct {
	// MaxBytes overrides MaxMarkdownBytes when > 0.
	MaxBytes int
	// Strip is a list of additional case-insensitive tag names to remove
	// outright (along with their content). Used in tests to confirm that
	// the user-visible markdown contains nothing from a hidden region.
	Strip []string
}

// HTMLToMarkdown converts a snippet of HTML to a markdown approximation,
// stripping <script>/<style>/<nav>/<footer>/<noscript>/<head> first,
// preserving fenced code blocks (with `language-foo` class hint), and
// finally truncating to MaxMarkdownBytes with a marker.
func HTMLToMarkdown(html string) string {
	return HTMLToMarkdownWithOptions(html, HTMLToMarkdownOptions{})
}

// HTMLToMarkdownWithOptions is the variant accepting tunables.
func HTMLToMarkdownWithOptions(html string, opts HTMLToMarkdownOptions) string {
	if html == "" {
		return ""
	}
	max := opts.MaxBytes
	if max <= 0 {
		max = MaxMarkdownBytes
	}

	// 1. Drop comments and structural noise that should never reach the model.
	html = htmlReComment.ReplaceAllString(html, "")
	html = htmlReHeadBlock.ReplaceAllString(html, "")
	html = htmlReScript.ReplaceAllString(html, "")
	html = htmlReStyle.ReplaceAllString(html, "")
	html = htmlReNav.ReplaceAllString(html, "")
	html = htmlReFooter.ReplaceAllString(html, "")
	html = htmlReNoscript.ReplaceAllString(html, "")
	html = htmlReSvg.ReplaceAllString(html, "")

	for _, name := range opts.Strip {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		re := regexp.MustCompile(`(?is)<` + regexp.QuoteMeta(name) + `[^>]*>.*?</` + regexp.QuoteMeta(name) + `>`)
		html = re.ReplaceAllString(html, "")
	}

	// 2. Preserve <pre><code> blocks with optional language hint *before*
	//    we touch <code> in inline contexts.
	html = htmlRePre.ReplaceAllStringFunc(html, func(match string) string {
		groups := htmlRePre.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		preAttrs := groups[1]
		inner := groups[2]
		lang := pickLanguageHint(preAttrs, inner)
		// If the inner content is a single <code> block, unwrap it; otherwise
		// keep its text verbatim so multi-line snippets survive.
		codeText := inner
		if codeMatch := htmlReCodeBlock.FindStringSubmatch(inner); len(codeMatch) >= 3 {
			if lang == "" {
				lang = pickLanguageHint(codeMatch[1], codeMatch[2])
			}
			codeText = codeMatch[2]
		}
		codeText = decodeHTMLEntities(stripInnerTags(codeText))
		codeText = strings.TrimRight(codeText, " \t\n")
		return "\n\n```" + lang + "\n" + codeText + "\n```\n\n"
	})

	// 3. Inline code: preserve as backticks but strip nested tags.
	html = htmlReCodeBlock.ReplaceAllStringFunc(html, func(match string) string {
		groups := htmlReCodeBlock.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		text := decodeHTMLEntities(stripInnerTags(groups[2]))
		if text == "" {
			return ""
		}
		if strings.Contains(text, "\n") {
			return "\n\n```\n" + strings.TrimSpace(text) + "\n```\n\n"
		}
		return "`" + text + "`"
	})

	// 4. Inline structural elements.
	html = htmlReBoldStrong.ReplaceAllString(html, "**$1**")
	html = htmlReBoldB.ReplaceAllString(html, "**$1**")
	html = htmlReItalicEm.ReplaceAllString(html, "*$1*")
	html = htmlReItalicI.ReplaceAllString(html, "*$1*")

	// 5. Anchors → markdown links. Drop empty/anchor-only links.
	html = htmlReAnchorMD.ReplaceAllStringFunc(html, func(match string) string {
		groups := htmlReAnchorMD.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		href := strings.TrimSpace(groups[1])
		text := strings.TrimSpace(stripInnerTags(groups[2]))
		switch {
		case href == "" && text == "":
			return ""
		case strings.HasPrefix(href, "#") || href == "":
			return text
		case text == "" || text == href:
			return href
		default:
			return "[" + text + "](" + href + ")"
		}
	})

	// 6. Images: alt-only, then src.
	html = htmlReImage.ReplaceAllString(html, "![${1}](${2})")
	html = htmlReImageNoAlt.ReplaceAllString(html, "![]($1)")

	// 7. Headings → "## Title".
	html = htmlReHeading.ReplaceAllStringFunc(html, func(match string) string {
		groups := htmlReHeading.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		level, err := strconv.Atoi(groups[1])
		if err != nil || level < 1 || level > 6 {
			return strings.TrimSpace(stripInnerTags(groups[2]))
		}
		text := strings.TrimSpace(stripInnerTags(groups[2]))
		return "\n\n" + strings.Repeat("#", level) + " " + text + "\n\n"
	})

	// 8. Lists: process inner list items inside <ul>/<ol> wrappers so each
	//    item gets the right marker, then fall through for stragglers.
	listIndex := 0
	html = htmlReOrderedList.ReplaceAllStringFunc(html, func(match string) string {
		groups := htmlReOrderedList.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		listIndex = 1
		items := htmlReListItem.ReplaceAllStringFunc(groups[1], func(li string) string {
			liGroups := htmlReListItem.FindStringSubmatch(li)
			if len(liGroups) < 2 {
				return li
			}
			text := strings.TrimSpace(stripInnerTags(liGroups[1]))
			line := fmt.Sprintf("%d. %s", listIndex, text)
			listIndex++
			return "\n" + line
		})
		return "\n" + items + "\n"
	})
	html = htmlReUnorderedList.ReplaceAllStringFunc(html, func(match string) string {
		groups := htmlReUnorderedList.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		items := htmlReListItem.ReplaceAllStringFunc(groups[1], func(li string) string {
			liGroups := htmlReListItem.FindStringSubmatch(li)
			if len(liGroups) < 2 {
				return li
			}
			text := strings.TrimSpace(stripInnerTags(liGroups[1]))
			return "\n- " + text
		})
		return "\n" + items + "\n"
	})
	html = htmlReListItem.ReplaceAllString(html, "\n- $1")

	// 9. Blockquotes prefix each line with `> `.
	html = htmlReBlockquote.ReplaceAllStringFunc(html, func(match string) string {
		groups := htmlReBlockquote.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		text := strings.TrimSpace(stripInnerTags(groups[1]))
		var sb strings.Builder
		for _, line := range strings.Split(text, "\n") {
			sb.WriteString("\n> " + line)
		}
		sb.WriteString("\n")
		return sb.String()
	})

	// 10. Paragraphs and breaks.
	html = htmlReParagraph.ReplaceAllString(html, "\n\n$1\n\n")
	html = htmlReBr.ReplaceAllString(html, "  \n")
	html = htmlReHr.ReplaceAllString(html, "\n\n---\n\n")

	// 11. Tables → markdown pipe tables. Must run BEFORE the generic-tag
	//     stripper, which otherwise drops the table structure entirely.
	html = htmlReTable.ReplaceAllStringFunc(html, func(match string) string {
		groups := htmlReTable.FindStringSubmatch(match)
		if len(groups) < 2 {
			return ""
		}
		return convertTableToMarkdown(groups[1])
	})

	// 12. Generic block wrappers can be discarded — content survives.
	html = htmlReGenericTag.ReplaceAllString(html, "")

	// 13. Last-resort tag drop.
	html = htmlReAnyTag.ReplaceAllString(html, "")

	// 14. Decode entities.
	html = decodeHTMLEntities(html)

	// 15. Whitespace tidy.
	html = strings.ReplaceAll(html, "\r\n", "\n")
	html = htmlReWhitespaceRun.ReplaceAllString(html, " ")
	html = htmlReBlankLines.ReplaceAllString(html, "\n\n")
	html = strings.TrimSpace(html)

	// 16. Truncation. We cut on bytes, not runes, because the cap exists
	//     to bound the *bytes* sent to the model.
	if len(html) > max {
		html = html[:max] + markdownTruncationMarker
	}
	return html
}

func stripInnerTags(s string) string {
	return htmlReAnyTag.ReplaceAllString(s, "")
}

// pickLanguageHint pulls a language identifier from class="language-foo"
// attributes on either the <pre> or its <code> descendant. Returns "" if
// none is present.
func pickLanguageHint(attrs, inner string) string {
	if m := htmlReLangClass.FindStringSubmatch(attrs); len(m) >= 3 {
		if m[1] != "" {
			return strings.ToLower(m[1])
		}
		return strings.ToLower(m[2])
	}
	if m := htmlReLangClass.FindStringSubmatch(inner); len(m) >= 3 {
		if m[1] != "" {
			return strings.ToLower(m[1])
		}
		return strings.ToLower(m[2])
	}
	return ""
}

func decodeHTMLEntities(s string) string {
	for k, v := range htmlEntities {
		if strings.Contains(s, k) {
			s = strings.ReplaceAll(s, k, v)
		}
	}
	return htmlReNumericEntity.ReplaceAllStringFunc(s, func(match string) string {
		groups := htmlReNumericEntity.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		base := 10
		if groups[1] == "x" {
			base = 16
		}
		n, err := strconv.ParseInt(groups[2], base, 32)
		if err != nil || n <= 0 {
			return ""
		}
		return string(rune(n))
	})
}

// IsHTMLContentType reports whether the Content-Type header looks like HTML.
// Re-exported so the converter file is self-contained for tests.
func IsHTMLContentType(contentType string) bool {
	return isHTML(contentType)
}

// convertTableToMarkdown turns the inner HTML of a <table> into a markdown
// pipe table. The first row containing <th> elements is treated as the
// header; if no <th> row exists, the first row of cells is used and a
// synthetic separator is inserted to keep the output a valid pipe-table.
func convertTableToMarkdown(inner string) string {
	rowMatches := htmlReTableRow.FindAllStringSubmatch(inner, -1)
	if len(rowMatches) == 0 {
		return ""
	}

	type tableRow struct {
		cells    []string
		isHeader bool
	}

	rows := make([]tableRow, 0, len(rowMatches))
	for _, rm := range rowMatches {
		if len(rm) < 2 {
			continue
		}
		rowHTML := rm[1]
		var cells []string
		isHeader := false

		if hMatches := htmlReTableHeader.FindAllStringSubmatch(rowHTML, -1); len(hMatches) > 0 {
			isHeader = true
			for _, hm := range hMatches {
				if len(hm) >= 2 {
					cells = append(cells, normaliseTableCell(hm[1]))
				}
			}
		} else if cMatches := htmlReTableCell.FindAllStringSubmatch(rowHTML, -1); len(cMatches) > 0 {
			for _, cm := range cMatches {
				if len(cm) >= 2 {
					cells = append(cells, normaliseTableCell(cm[1]))
				}
			}
		}

		if len(cells) > 0 {
			rows = append(rows, tableRow{cells: cells, isHeader: isHeader})
		}
	}

	if len(rows) == 0 {
		return ""
	}

	// Locate the header row (first row containing <th>) — fall back to row 0.
	headerIdx := 0
	for i, r := range rows {
		if r.isHeader {
			headerIdx = i
			break
		}
	}

	width := 0
	for _, r := range rows {
		if len(r.cells) > width {
			width = len(r.cells)
		}
	}

	pad := func(cells []string) []string {
		if len(cells) >= width {
			return cells[:width]
		}
		out := make([]string, width)
		copy(out, cells)
		return out
	}

	formatRow := func(cells []string) string {
		c := pad(cells)
		var sb strings.Builder
		sb.WriteString("|")
		for _, cell := range c {
			sb.WriteString(" ")
			sb.WriteString(cell)
			sb.WriteString(" |")
		}
		return sb.String()
	}

	var sb strings.Builder
	sb.WriteString("\n\n")
	sb.WriteString(formatRow(rows[headerIdx].cells))
	sb.WriteString("\n|")
	for i := 0; i < width; i++ {
		sb.WriteString(" --- |")
	}
	sb.WriteString("\n")
	for i, r := range rows {
		if i == headerIdx {
			continue
		}
		sb.WriteString(formatRow(r.cells))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// normaliseTableCell strips inner tags, decodes entities, and collapses
// whitespace inside a single table cell.
func normaliseTableCell(s string) string {
	s = stripInnerTags(s)
	s = decodeHTMLEntities(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	s = htmlReWhitespaceRun.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	// Pipe characters inside cells must be escaped to avoid breaking the
	// surrounding markdown table.
	s = strings.ReplaceAll(s, "|", `\|`)
	return s
}
