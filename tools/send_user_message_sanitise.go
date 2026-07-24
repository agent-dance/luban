// Package tools — sanitise helper for send_user_message.go to strip
// terminal control sequences and NUL bytes from outbound user messages.
package tools

import (
	"regexp"
	"strings"
)

// ansiEscRE matches CSI sequences (ESC [ ... cmd) and a few common
// non-CSI ESC sequences (e.g. ESC ] for OSC; ESC P for DCS). The pattern
// purposely stays conservative — it only strips the bytes the user clearly
// did not type but that may have leaked from a tool wrapping the model
// output (e.g. a TUI screenshot pipeline).
var ansiEscRE = regexp.MustCompile(`\x1b(\[[0-?]*[ -/]*[@-~]|\][^\x07]*\x07|[PX^_][^\x1b]*\x1b\\)`)

// sanitiseUserMessageBody removes NULs and ANSI control sequences from m.
// Tabs / newlines / carriage returns are preserved — only the bytes that
// would mangle a transcript are dropped.
func sanitiseUserMessageBody(m string) string {
	if m == "" {
		return m
	}
	if strings.IndexByte(m, 0x00) >= 0 {
		m = strings.ReplaceAll(m, "\x00", "")
	}
	if strings.IndexByte(m, 0x1b) >= 0 {
		m = ansiEscRE.ReplaceAllString(m, "")
	}
	return m
}
