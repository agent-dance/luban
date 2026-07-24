package tools

import (
	"strings"
)

// stripClaudeMdSection removes the "# User Instructions" section from a
// system-prompt envelope so Explore/Plan agents (which set OmitClaudeMd)
// don't carry project-specific guidance into a focused subagent. The
// section is bounded by the next H1 header (line beginning with "# ") or
// end of input. Mirrors the TS stripBaseSystemForAgent helper.
func stripClaudeMdSection(prompt string) string {
	if prompt == "" {
		return prompt
	}
	const header = "# User Instructions"
	idx := strings.Index(prompt, header)
	if idx < 0 {
		return prompt
	}
	// Find next H1 starting AFTER the header line itself.
	rest := prompt[idx+len(header):]
	nextH1 := -1
	for off, line := range strings.SplitAfter(rest, "\n") {
		// We need the absolute offset within `rest`.
		_ = off
		if strings.HasPrefix(line, "# ") {
			// Compute absolute offset.
			nextH1 = strings.Index(rest, line)
			break
		}
	}
	head := strings.TrimRight(prompt[:idx], " \t\r\n")
	var tail string
	if nextH1 >= 0 {
		tail = strings.TrimLeft(rest[nextH1:], " \t\r\n")
	}
	if tail == "" {
		return strings.TrimSpace(head)
	}
	if head == "" {
		return strings.TrimSpace(tail)
	}
	return head + "\n\n" + tail
}
