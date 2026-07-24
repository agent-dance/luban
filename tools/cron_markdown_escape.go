// Package tools — cron_markdown_escape.go provides EscapeCronMarkdown to
// safely embed user-supplied job descriptions in markdown notification
// bodies. Backticks and triple-backtick fences are the only sequences that
// can break out of inline-code spans into surrounding markdown.
package tools

import "strings"

// EscapeCronMarkdown returns s with backticks and code-fence markers
// (```) escaped so the string is safe to embed in a markdown context.
// Pass the result through inline-code (`text`) or fenced (```text```)
// rendering at the call site — this helper only neutralises the
// fence-breakers; it does not add the surrounding decorators itself.
func EscapeCronMarkdown(s string) string {
	if s == "" {
		return ""
	}
	// Walk the string and escape every ` once. Replacing ``` first then `
	// would double-escape the survivors, so we apply a single rune-level pass.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '`' {
			b.WriteString("\\`")
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func fenceCronPrompt(prompt string) string {
	longest := 0
	current := 0
	for _, r := range prompt {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	width := longest + 1
	if width < 3 {
		width = 3
	}
	fence := strings.Repeat("`", width)
	return fence + "\n" + prompt + "\n" + fence
}
