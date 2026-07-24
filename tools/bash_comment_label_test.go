// Package tools — tests for bash_comment_label.go.
package tools

import "testing"

func TestExtractCommentLabel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no-comment", "ls -la", ""},
		{"plain-comment", "# list files\nls -la", "list files"},
		{"comment-no-space", "#run-build\nmake", "run-build"},
		{"shebang-then-comment", "#!/usr/bin/env bash\n# build\nmake", "build"},
		{"shebang-only", "#!/bin/sh\nls", ""},
		{"leading-blank-then-comment", "\n\n# step\nls", "step"},
		{"trailing-comment-only-no-leading", "ls\n# this is later", ""},
		{"comment-with-trailing-spaces", "#  spaced label   \nls", "spaced label"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractCommentLabel(c.in)
			if got != c.want {
				t.Fatalf("ExtractCommentLabel(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestStripEmptyLines(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single-line", "hello", "hello"},
		{"leading-blank", "\n\nhello", "hello"},
		{"trailing-blank", "hello\n\n", "hello"},
		{"both", "\n\nhello\n\n", "hello"},
		{"internal-blank-preserved", "a\n\nb", "a\n\nb"},
		{"whitespace-only-line-preserved", "a\n   \nb", "a\n   \nb"},
		{"all-blank", "\n\n\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := StripEmptyLines(c.in)
			if got != c.want {
				t.Fatalf("StripEmptyLines(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
