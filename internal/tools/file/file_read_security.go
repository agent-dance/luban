// Package file — file_read_security.go implements path normalisation helpers
// for the Read tool.
package file

import (
	"os"
	"strings"
)

// normalizeReadFilePath canonicalises the requested path before we open
// it. macOS screenshot filenames embed a U+202F NARROW NO-BREAK SPACE
// between the date and time (e.g. "Screenshot 2024-05-01 at 9.30.00 AM.png").
// Some shells/copy-paste paths convert the U+202F to a regular space,
// which then fails to open. We try both forms.
func normalizeReadFilePath(path string) string {
	if path == "" {
		return path
	}
	// Try the path as-is first; if it doesn't exist, try toggling the
	// thin-space ↔ regular-space variants.
	if _, err := os.Stat(path); err == nil {
		return path
	}
	const thinSpace = " "
	if strings.Contains(path, thinSpace) {
		alt := strings.ReplaceAll(path, thinSpace, " ")
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	if strings.Contains(path, " ") {
		alt := strings.ReplaceAll(path, " ", thinSpace)
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	return path
}
