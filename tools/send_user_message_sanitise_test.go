// Package tools — tests for sanitiseUserMessageBody.
package tools

import "testing"

func TestSanitiseUserMessageBody(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello world", "hello world"},
		{"strip-NUL", "hello\x00world", "helloworld"},
		{"strip-CSI-color", "\x1b[31mred\x1b[0m text", "red text"},
		{"strip-OSC-title", "\x1b]0;title\x07after", "after"},
		{"preserve-newlines", "a\nb\tc", "a\nb\tc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitiseUserMessageBody(c.in)
			if got != c.want {
				t.Fatalf("sanitiseUserMessageBody(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
