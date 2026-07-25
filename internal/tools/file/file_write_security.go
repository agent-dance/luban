// Package file — file_write_security.go implements security and
// encoding-preservation helpers for FileWriteTool. Mirrors fragments of
// src/tools/FileWriteTool/FileWriteTool.ts:
//   - checkTeamMemSecrets: refuse to write secret-shaped content into
//     team memory files (.luban-code/memory/team/* — committed to repo).
//   - encoding preservation: re-encode UTF-8 string bytes back into the
//     file's original encoding (UTF-16, Latin-1) on overwrite.
package file

import (
	"path/filepath"
	"regexp"
	"strings"
)

// isTeamMemoryFilePath reports whether path lies under a team-memory
// directory (.luban-code/memory/team/).
// Compares lower-cased + slash-normalised forms.
func isTeamMemoryFilePath(path string) bool {
	if path == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	uniform := strings.ToLower(filepath.ToSlash(filepath.Clean(abs)))
	return strings.Contains(uniform, "/.luban-code/memory/team/")
}

// secretPatterns are regex/string patterns matched against team-memory
// content. A hit returns a short label (matched first) so the error
// message can identify the kind of secret without echoing the value.
type secretPattern struct {
	Label string
	RE    *regexp.Regexp
}

var teamMemorySecretPatterns = []secretPattern{
	{"AWS access key", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"AWS secret key", regexp.MustCompile(`(?i)aws_secret_access_key\s*[:=]\s*["']?[A-Za-z0-9/+=]{40}["']?`)},
	{"Anthropic API key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_\-]{20,}\b`)},
	{"OpenAI API key", regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`)},
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`)},
	{"GitHub fine-grained token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{40,}\b`)},
	{"Slack token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{"Stripe key", regexp.MustCompile(`\b(?:sk|pk|rk)_(?:live|test)_[A-Za-z0-9]{20,}\b`)},
	{"Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z\-_]{35}\b`)},
	{"private key block", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`)},
	{"JWT token", regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b`)},
	{"generic api_key assignment", regexp.MustCompile(`(?i)\b(?:api[_-]?key|apikey|secret|password|passwd)\s*[:=]\s*["'][A-Za-z0-9_\-+/=]{16,}["']`)},
}

// scanForTeamMemorySecrets returns the label of the first matched secret
// pattern, or "" when none match. Inspecting the entire content is fine —
// team-memory files are typically tiny.
func scanForTeamMemorySecrets(content string) string {
	if content == "" {
		return ""
	}
	for _, p := range teamMemorySecretPatterns {
		if p.RE.MatchString(content) {
			return p.Label
		}
	}
	return ""
}

// ContainsCredential reports whether content matches a credential pattern
// without exposing the matched value or its user-facing category label.
func ContainsCredential(content string) bool {
	return scanForTeamMemorySecrets(content) != ""
}

// encodeWriteBytes re-encodes a UTF-8 content string back into the file's
// original encoding for round-trip preservation on overwrite. The BOM is
// reapplied when the original encoding included one. On unknown
// encodings, returns the UTF-8 bytes unchanged so the caller cannot fail
// closed on a typo.
func encodeWriteBytes(content string, enc FileEncoding, bom []byte) []byte {
	body := encodeStringForEncoding(content, enc)
	if body == nil {
		return []byte(content)
	}
	if len(bom) == 0 {
		return body
	}
	out := make([]byte, 0, len(bom)+len(body))
	out = append(out, bom...)
	out = append(out, body...)
	return out
}
