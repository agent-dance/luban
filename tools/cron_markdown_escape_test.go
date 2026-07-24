// Package tools — tests for EscapeCronMarkdown.
package tools

import "testing"

func TestEscapeCronMarkdown(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"plain text", "plain text"},
		{"hello `code` world", "hello \\`code\\` world"},
		{"```\nblock\n```", "\\`\\`\\`\nblock\n\\`\\`\\`"},
		{"a `b` ```c``` d", "a \\`b\\` \\`\\`\\`c\\`\\`\\` d"},
	}
	for _, c := range cases {
		got := EscapeCronMarkdown(c.in)
		if got != c.want {
			t.Errorf("EscapeCronMarkdown(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
