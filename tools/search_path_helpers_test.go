// Package tools — tests for path helpers used by Glob/Grep.
package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandTildePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"~", home},
		{"~/", home},
		{"/abs/path", "/abs/path"},
		{"relative/path", "relative/path"},
	}
	for _, c := range cases {
		got := expandTildePath(c.in)
		// Normalise Windows separators for comparison.
		if filepath.Clean(got) != filepath.Clean(c.want) {
			t.Errorf("expandTildePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// "~/foo"
	if got := expandTildePath("~/foo"); got != filepath.Join(home, "foo") {
		t.Errorf("~/foo expansion = %q", got)
	}
}

func TestIsUNCPath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"a", false},
		{"/abs", false},
		{"\\\\server\\share", true},
		{"//server/share", true},
		{"\\\\?\\C:\\path", true},
	}
	for _, c := range cases {
		if got := isUNCPath(c.in); got != c.want {
			t.Errorf("isUNCPath(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEnvFlagDefaultTrue(t *testing.T) {
	t.Setenv("CLAUDE_TEST_FLAG", "")
	if !envFlagDefaultTrue("CLAUDE_TEST_FLAG") {
		t.Errorf("empty env should be true")
	}
	for _, v := range []string{"0", "false", "no", "OFF"} {
		t.Setenv("CLAUDE_TEST_FLAG", v)
		if envFlagDefaultTrue("CLAUDE_TEST_FLAG") {
			t.Errorf("value %q should be false", v)
		}
	}
	t.Setenv("CLAUDE_TEST_FLAG", "1")
	if !envFlagDefaultTrue("CLAUDE_TEST_FLAG") {
		t.Errorf("'1' should be true")
	}
}
