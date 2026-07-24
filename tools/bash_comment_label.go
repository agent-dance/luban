// Package tools — bash_comment_label.go ports two small helpers from
// src/tools/BashTool/commentLabel.ts and utils.ts:
//
//   - ExtractCommentLabel: scans the first non-shebang line of a bash command
//     for a leading "# ..." comment and returns its trimmed text. Used by the
//     UI to surface the model's intent label instead of the raw pipeline.
//
//   - StripEmptyLines: trims fully-empty leading/trailing lines while
//     preserving internal whitespace. Mirrors the lightweight cleanup TS
//     applies to multi-line stdout/stderr blocks before formatting them for
//     the transcript.
//
// Both helpers are pure / allocation-bounded and have no external state, so
// they can be called from any of the bash_*.go files (e.g. when emitting a
// tool-use envelope or post-processing captured output).
package tools

import "strings"

// ExtractCommentLabel returns the comment label of a bash command, or "" if
// none is present. It mirrors src/tools/BashTool/commentLabel.ts:1-13:
//
//   - the input is split on newlines
//   - a leading "#!" shebang line is skipped
//   - the first remaining line is examined; if it starts with "#" (after
//     trimming whitespace), the rest of the line is returned with leading "#"
//     and surrounding whitespace stripped
//
// Anything else returns the empty string. The function makes no judgement
// about the command itself; it is a pure label extractor.
func ExtractCommentLabel(command string) string {
	if command == "" {
		return ""
	}
	lines := strings.Split(command, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// Skip shebang.
		if strings.HasPrefix(line, "#!") {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			return ""
		}
		// Strip leading "#" then trim.
		return strings.TrimSpace(strings.TrimPrefix(line, "#"))
	}
	return ""
}

// StripEmptyLines trims fully-empty leading/trailing lines while preserving
// internal whitespace. Lines that contain whitespace-only characters are NOT
// considered empty (mirrors the TS conservative trim — internal blank lines
// in code blocks must survive).
func StripEmptyLines(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && lines[start] == "" {
		start++
	}
	end := len(lines)
	for end > start && lines[end-1] == "" {
		end--
	}
	if start == 0 && end == len(lines) {
		return s
	}
	return strings.Join(lines[start:end], "\n")
}
