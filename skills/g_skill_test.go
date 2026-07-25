package skills

import (
	"strings"
	"testing"
)

// -------- frontmatter strip --------

func TestPrepareSkillContent_StripsFrontmatter(t *testing.T) {
	raw := "---\ndescription: x\nmodel: sonnet\n---\nThe real body."
	parsed := parseFrontmatter(raw, "test.md")
	if strings.Contains(parsed.Content, "model: sonnet") {
		t.Errorf("parseFrontmatter must strip the YAML block; got: %q", parsed.Content)
	}
	if strings.Contains(parsed.Content, "---") {
		t.Errorf("parseFrontmatter must remove the --- delimiters; got: %q", parsed.Content)
	}
	if !strings.Contains(parsed.Content, "The real body.") {
		t.Errorf("body should remain; got: %q", parsed.Content)
	}
}
