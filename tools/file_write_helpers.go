// Package tools — file_write_helpers.go houses the small, single-purpose
// helpers used by FileWriteTool that do not warrant their own files.
//
//   - normalizeLineEndingsToLF: enforces the always-LF write policy
//     (mirrors TS FileWriteTool after the historical binary-preamble bug).
//   - isClaudeMemoryWritePath: detects CLAUDE.md / AGENTS.md targets so
//     dedicated analytics fire (tengu_write_claudemd parity).
package tools

import (
	"path/filepath"
	"strings"
)

// normalizeLineEndingsToLF replaces \r\n and bare \r with \n. Mirrors the
// TS post-bug-fix behaviour where Write always writes LF regardless of
// the file's prior convention. The encoding-preserve path runs AFTER this
// normalisation so reencoded UTF-16 bytes are also LF-only.
func normalizeLineEndingsToLF(content string) string {
	if !strings.ContainsAny(content, "\r") {
		return content
	}
	// Two-pass replacement keeps the result deterministic for inputs that
	// mix CR and CRLF.
	out := strings.ReplaceAll(content, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")
	return out
}

// claudeMemoryFilenames are the basenames TS treats as CLAUDE-memory
// targets for tengu_write_claudemd analytics. Case-insensitive match.
var claudeMemoryFilenames = []string{
	"claude.md",
	"agents.md",
	"agents.local.md",
	"claude.local.md",
}

// isClaudeMemoryWritePath reports whether a Write target path is a
// CLAUDE memory file (CLAUDE.md / AGENTS.md, plus their .local variants).
// Used to fire dedicated analytics events.
func isClaudeMemoryWritePath(absPath string) bool {
	if absPath == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(absPath))
	for _, name := range claudeMemoryFilenames {
		if base == name {
			return true
		}
	}
	return false
}
