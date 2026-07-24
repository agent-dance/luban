// Package tools — tests for M_search_web_ask wave.
package tools

import (
	"strings"
	"testing"
)

// TestAskUserPreviewFormat_HTMLValidationGate confirms that wiring
// SetAskUserPreviewFormat("html") routes ValidateAskUserQuestions through
// ValidateHtmlPreview so previews containing <script> are rejected.
func TestAskUserPreviewFormat_HTMLValidationGate(t *testing.T) {
	defer SetAskUserPreviewFormat("")
	SetAskUserPreviewFormat("html")
	q := QuestionSpec{
		Question: "pick one?",
		Header:   "pick",
		Options: []OptionSpec{
			{Label: "ok", Description: "one", Preview: "<div>ok</div>"},
			{Label: "evil", Description: "two", Preview: "<div><script>x</script></div>"},
		},
	}
	err := ValidateAskUserQuestions([]QuestionSpec{q})
	if err == nil {
		t.Fatalf("expected validation error for <script> preview, got nil")
	}
	if !strings.Contains(err.Error(), "<script>") {
		t.Fatalf("expected error to mention <script>, got %q", err)
	}
}

// TestAskUserPreviewFormat_PassthroughWhenUnset confirms that without
// SetAskUserPreviewFormat the validator stays permissive (no regression).
func TestAskUserPreviewFormat_PassthroughWhenUnset(t *testing.T) {
	q := QuestionSpec{
		Question: "pick one?",
		Header:   "pick",
		Options: []OptionSpec{
			{Label: "a", Description: "one"},
			{Label: "b", Description: "two"},
		},
	}
	if err := ValidateAskUserQuestions([]QuestionSpec{q}); err != nil {
		t.Fatalf("unexpected validation error with unset preview format: %v", err)
	}
}

// TestAskUserChannelsActive_EnvVarDriven verifies the KAIROS_CHANNELS env
// gate. Uses SetAskUserChannelsActiveForTest as the test override.
func TestAskUserChannelsActive_EnvVarDriven(t *testing.T) {
	on := true
	SetAskUserChannelsActiveForTest(&on)
	defer SetAskUserChannelsActiveForTest(nil)
	if !AskUserChannelsActive() {
		t.Fatalf("expected AskUserChannelsActive=true with override")
	}
	off := false
	SetAskUserChannelsActiveForTest(&off)
	if AskUserChannelsActive() {
		t.Fatalf("expected AskUserChannelsActive=false with override")
	}
}

// TestFileReadIgnore_MatchesAndPassthrough confirms IsFileReadIgnored
// matches both pattern and basename forms.
func TestFileReadIgnore_MatchesAndPassthrough(t *testing.T) {
	defer SetFileReadIgnorePatterns(nil)
	SetFileReadIgnorePatterns([]string{".env", "secrets/**"})
	cases := []struct {
		path    string
		ignored bool
	}{
		{".env", true},
		{"foo/.env", true},
		{"secrets/api_key.txt", true},
		{"src/main.go", false},
		{".envoy/config", false},
	}
	for _, c := range cases {
		if got := IsFileReadIgnored(c.path); got != c.ignored {
			t.Errorf("IsFileReadIgnored(%q) = %v, want %v", c.path, got, c.ignored)
		}
	}
}

// TestFilterFileReadIgnored_PreservesOrder confirms that the filter
// preserves the input order and removes only denied entries.
func TestFilterFileReadIgnored_PreservesOrder(t *testing.T) {
	defer SetFileReadIgnorePatterns(nil)
	SetFileReadIgnorePatterns([]string{"*.lock"})
	in := []string{"a.go", "b.lock", "c.go"}
	out := FilterFileReadIgnored(in)
	want := []string{"a.go", "c.go"}
	if len(out) != len(want) {
		t.Fatalf("len(out)=%d, want %d (out=%v)", len(out), len(want), out)
	}
	for i, p := range want {
		if out[i] != p {
			t.Errorf("out[%d]=%q want %q", i, out[i], p)
		}
	}
}

// TestRipgrepTimeoutError_Render confirms RipgrepTimeoutError carries the
// partial slice and reports the canonical error string.
func TestRipgrepTimeoutError_Render(t *testing.T) {
	err := &RipgrepTimeoutError{Partial: []string{"foo:1:bar"}}
	if err.Error() != "Ripgrep search timed out" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if len(err.Partial) != 1 {
		t.Fatalf("len(Partial) = %d, want 1", len(err.Partial))
	}
}

// TestSplitTrailingNotes confirms that the "n:<text>" suffix is parsed
// off without disturbing the answer body.
func TestSplitTrailingNotes(t *testing.T) {
	cases := []struct {
		in, body, notes string
	}{
		{"1", "1", ""},
		{"1 n:hello world", "1", "hello world"},
		{"3 n:use the green variant", "3", "use the green variant"},
		{"o:bespoke n:why not", "o:bespoke", "why not"},
		{"foo n: ", "foo", ""},
	}
	for _, c := range cases {
		body, notes := splitTrailingNotes(c.in)
		if body != c.body || notes != c.notes {
			t.Errorf("splitTrailingNotes(%q) = (%q,%q), want (%q,%q)", c.in, body, notes, c.body, c.notes)
		}
	}
}

// TestIsBinaryContentType covers the canonical PDF/zip/etc detection.
func TestIsBinaryContentType(t *testing.T) {
	for _, ct := range []string{
		"application/pdf",
		"application/pdf; charset=utf-8",
		"application/zip",
		"image/png",
		"video/mp4",
	} {
		if !isBinaryContentType(ct) {
			t.Errorf("isBinaryContentType(%q) = false, want true", ct)
		}
	}
	for _, ct := range []string{
		"text/html",
		"text/markdown",
		"application/json",
	} {
		if isBinaryContentType(ct) {
			t.Errorf("isBinaryContentType(%q) = true, want false", ct)
		}
	}
}

// TestIsTransientSinkError covers the retry-classification heuristic.
func TestIsTransientSinkError(t *testing.T) {
	transient := []string{
		"HTTP 429 Too Many Requests",
		"channel closed",
		"i/o timeout",
		"deadline exceeded",
	}
	for _, msg := range transient {
		if !isTransientSinkError(testErr(msg)) {
			t.Errorf("isTransientSinkError(%q) = false, want true", msg)
		}
	}
	permanent := []string{
		"validation failed: missing required field",
		"forbidden",
	}
	for _, msg := range permanent {
		if isTransientSinkError(testErr(msg)) {
			t.Errorf("isTransientSinkError(%q) = true, want false", msg)
		}
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }
