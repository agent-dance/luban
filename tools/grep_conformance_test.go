// Package tools — grep_conformance_test.go pins the GrepTool surface against
// TS GrepTool parity (output modes, head_limit/offset, multiline, type
// filters, fallback behaviour). Cross-platform tolerant: tests that depend on
// rg-specific output use the fallback path so they run on systems without
// ripgrep installed.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestValidateGrepOutputMode(t *testing.T) {
	cases := []struct {
		in      string
		want    GrepOutputMode
		wantErr bool
	}{
		{"", GrepModeFilesWithMatches, false},
		{"content", GrepModeContent, false},
		{"files_with_matches", GrepModeFilesWithMatches, false},
		{"count", GrepModeCount, false},
		{"summary", "", true},
		{"  files_with_matches  ", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ValidateGrepOutputMode(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Validate(%q)=%v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRipgrepFlagsForMode(t *testing.T) {
	cases := []struct {
		mode     GrepOutputMode
		opts     grepRipgrepOptions
		mustHave []string
		mustMiss []string
	}{
		{
			mode:     GrepModeFilesWithMatches,
			opts:     grepRipgrepOptions{},
			mustHave: []string{"-l"},
			mustMiss: []string{"-n", "-c"},
		},
		{
			mode:     GrepModeCount,
			opts:     grepRipgrepOptions{},
			mustHave: []string{"-c"},
			mustMiss: []string{"-l", "-n"},
		},
		{
			mode:     GrepModeContent,
			opts:     grepRipgrepOptions{ShowLineNumbers: true, ContextBefore: 1, ContextAfter: 2, ContextBeforeSet: true, ContextAfterSet: true},
			mustHave: []string{"-n", "-B", "1", "-A", "2"},
			mustMiss: []string{"-l", "-c"},
		},
		{
			mode:     GrepModeContent,
			opts:     grepRipgrepOptions{Context: 0, ContextSet: true, ContextBefore: 1, ContextAfter: 2, ContextBeforeSet: true, ContextAfterSet: true},
			mustHave: []string{"-C", "0"},
			mustMiss: []string{"-B", "-A", "-l", "-c"},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			flags := RipgrepFlagsForMode(tc.mode, tc.opts)
			joined := strings.Join(flags, " ")
			for _, want := range tc.mustHave {
				if !containsFlag(flags, want) {
					t.Errorf("expected %q in %q", want, joined)
				}
			}
			for _, must := range tc.mustMiss {
				if containsFlag(flags, must) {
					t.Errorf("did not expect %q in %q", must, joined)
				}
			}
		})
	}
}

func containsFlag(flags []string, target string) bool {
	for _, f := range flags {
		if f == target {
			return true
		}
	}
	return false
}

func TestValidateContextFlagsIgnoresOutsideContent(t *testing.T) {
	if err := ValidateContextFlags(GrepModeFilesWithMatches, grepRipgrepOptions{ContextAfter: 1, ContextAfterSet: true}); err != nil {
		t.Errorf("TS ignores -A in files_with_matches; got %v", err)
	}
	if err := ValidateContextFlags(GrepModeCount, grepRipgrepOptions{ContextBefore: 1, ContextBeforeSet: true}); err != nil {
		t.Errorf("TS ignores -B in count; got %v", err)
	}
	if err := ValidateContextFlags(GrepModeContent, grepRipgrepOptions{ContextBefore: 1, ContextAfter: 1, ContextBeforeSet: true, ContextAfterSet: true}); err != nil {
		t.Errorf("content mode should accept context flags: %v", err)
	}
}

func TestApplyHeadLimitOffset(t *testing.T) {
	in := []string{"a", "b", "c", "d", "e"}

	// head_limit only
	got := ApplyHeadLimitOffset(in, 0, 3, false)
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("limit only failed: %v", got)
	}

	// offset only
	got = ApplyHeadLimitOffset(in, 2, 100, true)
	if len(got) != 3 || got[0] != "c" {
		t.Errorf("offset+unlimited failed: %v", got)
	}

	// both
	got = ApplyHeadLimitOffset(in, 1, 2, false)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Errorf("offset+limit window failed: %v", got)
	}

	// offset past end
	got = ApplyHeadLimitOffset(in, 100, 2, false)
	if len(got) != 0 {
		t.Errorf("expected empty slice when offset >= len, got %v", got)
	}

	// unlimited
	got = ApplyHeadLimitOffset(in, 0, 0, true)
	if len(got) != len(in) {
		t.Errorf("unlimited should return all: %v", got)
	}
}

func TestResolveHeadLimitDefault(t *testing.T) {
	parsed := &GrepInput{}
	limit, unlimited := ResolveHeadLimit(map[string]any{}, parsed)
	if limit != defaultGrepHeadLimit {
		t.Errorf("expected default %d, got %d", defaultGrepHeadLimit, limit)
	}
	if unlimited {
		t.Error("default should not be unlimited")
	}
}

func TestResolveHeadLimitExplicitZero(t *testing.T) {
	zero := float64(0)
	parsed := &GrepInput{HeadLimit: &zero}
	limit, unlimited := ResolveHeadLimit(map[string]any{"head_limit": float64(0)}, parsed)
	if !unlimited {
		t.Error("explicit zero should signal unlimited")
	}
	if limit != 0 {
		t.Errorf("expected 0 limit, got %d", limit)
	}
}

func TestResolveHeadLimitExplicitValue(t *testing.T) {
	seven := float64(7)
	parsed := &GrepInput{HeadLimit: &seven}
	limit, unlimited := ResolveHeadLimit(map[string]any{"head_limit": float64(7)}, parsed)
	if unlimited {
		t.Error("positive limit should not be unlimited")
	}
	if limit != 7 {
		t.Errorf("expected 7, got %d", limit)
	}
}

func TestParseTypeListBasic(t *testing.T) {
	raw := "go: *.go\nrust: *.rs, *.toml\n  : ignore-empty\nnoglob"
	got := parseTypeList(raw)
	if len(got) != 2 || got[0] != "go" || got[1] != "rust" {
		t.Errorf("unexpected types: %v", got)
	}
}

func TestSimpleEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"go", "go", 0},
		{"go", "got", 1},
		{"py", "python", 4},
		{"", "abc", 3},
		{"abc", "", 3},
	}
	for _, tc := range cases {
		got := simpleEditDistance(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("dist(%q,%q)=%d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSuggestGrepTypesNearMatch(t *testing.T) {
	names := []string{"go", "rust", "python", "java"}
	got := suggestGrepTypes("rsut", names)
	found := false
	for _, s := range got {
		if s == "rust" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected suggestion to include rust, got %v", got)
	}
}

func TestValidateGrepTypeAcceptsKnown(t *testing.T) {
	// Use a stub by populating the cache directly.
	grepKnownTypesOnce = forceOnce()
	grepKnownTypesValue = map[string]struct{}{"go": {}, "rust": {}}
	grepKnownTypesNames = []string{"go", "rust"}
	t.Cleanup(ResetGrepKnownTypesCache)

	if err := ValidateGrepType("go"); err != nil {
		t.Errorf("expected go to be valid: %v", err)
	}
	if err := ValidateGrepType(""); err != nil {
		t.Errorf("empty should be valid: %v", err)
	}
}

func TestValidateGrepTypeRejectsUnknown(t *testing.T) {
	grepKnownTypesOnce = forceOnce()
	grepKnownTypesValue = map[string]struct{}{"go": {}, "rust": {}}
	grepKnownTypesNames = []string{"go", "rust"}
	t.Cleanup(ResetGrepKnownTypesCache)

	if err := ValidateGrepType("foobar"); err == nil {
		t.Error("expected error for unknown type")
	}
}

// forceOnce returns a sync.Once that is already done so subsequent
// loadGrepKnownTypes calls don't overwrite the test stubs.
func forceOnce() (o sync.Once) {
	o.Do(func() {})
	return
}

type syncOnceLike struct {
	done bool
}

func (s *syncOnceLike) Do(f func()) {
	if !s.done {
		s.done = true
		f()
	}
}

func TestGrepToolFallbackOutputModeFiles(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "match\n")
	mustWrite(t, filepath.Join(dir, "b.txt"), "skip\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "match",
		"path":        dir,
		"output_mode": "files_with_matches",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.txt") {
		t.Errorf("expected a.txt in result, got %q", res.Content)
	}
	if strings.Contains(res.Content, "b.txt") {
		t.Errorf("did not expect b.txt: %q", res.Content)
	}
}

func TestGrepToolFallbackCountMode(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	fp := filepath.Join(dir, "data.txt")
	mustWrite(t, fp, "match\nother\nmatch\nmatch\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "match",
		"path":        fp,
		"output_mode": "count",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "3") {
		t.Errorf("expected count 3, got %q", res.Content)
	}
}

func TestGrepToolFallbackContentMode(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	fp := filepath.Join(dir, "x.txt")
	mustWrite(t, fp, "alpha\nbeta\ngamma\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "beta",
		"path":        fp,
		"output_mode": "content",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "beta") {
		t.Errorf("expected beta in content, got %q", res.Content)
	}
}

func TestGrepToolFallbackOffset(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		mustWrite(t, filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), "needle\n")
	}

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":    "needle",
		"path":       dir,
		"head_limit": float64(2),
		"offset":     float64(1),
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	lines := grepResultPayloadLines(res.Content)
	if len(lines) != 2 {
		t.Errorf("expected 2 lines after offset+limit, got %d (%q)", len(lines), res.Content)
	}
}

func TestGrepToolFallbackHeadLimitDefault(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	for i := 0; i < defaultGrepHeadLimit+10; i++ {
		mustWrite(t, filepath.Join(dir, fmt.Sprintf("f%04d.txt", i)), "needle\n")
	}

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	lines := grepResultPayloadLines(res.Content)
	if len(lines) != defaultGrepHeadLimit {
		t.Errorf("expected default cap %d, got %d", defaultGrepHeadLimit, len(lines))
	}
}

func TestGrepToolFallbackUnlimited(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		mustWrite(t, filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), "needle\n")
	}

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":    "needle",
		"path":       dir,
		"head_limit": float64(0),
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	lines := grepResultPayloadLines(res.Content)
	if len(lines) != 5 {
		t.Errorf("expected all 5 results, got %d (%q)", len(lines), res.Content)
	}
}

func TestGrepToolFallbackInvalidOutputMode(t *testing.T) {
	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "x",
		"output_mode": "html",
	})
	if !res.IsError {
		t.Fatalf("expected error for invalid output_mode")
	}
	if !strings.Contains(res.Content, "output_mode") {
		t.Errorf("expected output_mode in error, got %q", res.Content)
	}
}

func TestGrepToolFallbackTypeFilter(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "main.go"), "needle\n")
	mustWrite(t, filepath.Join(dir, "main.py"), "needle\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
		"type":    "go",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "main.go") {
		t.Errorf("expected main.go: %q", res.Content)
	}
	if strings.Contains(res.Content, "main.py") {
		t.Errorf("type filter should exclude py: %q", res.Content)
	}
}

func TestGrepToolFallbackGlobFilter(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), "needle\n")
	mustWrite(t, filepath.Join(dir, "b.txt"), "needle\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
		"glob":    "*.go",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.go") {
		t.Errorf("expected a.go: %q", res.Content)
	}
	if strings.Contains(res.Content, "b.txt") {
		t.Errorf("glob should exclude txt: %q", res.Content)
	}
}

func TestGrepToolFallbackContextLines(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	fp := filepath.Join(dir, "log.txt")
	mustWrite(t, fp, "line1\nline2\ntarget\nline4\nline5\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "target",
		"path":        fp,
		"output_mode": "content",
		"-B":          float64(1),
		"-A":          float64(1),
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "line2") || !strings.Contains(res.Content, "line4") {
		t.Errorf("expected -B/-A context lines, got %q", res.Content)
	}
}

func TestGrepToolFallbackCaseInsensitive(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	fp := filepath.Join(dir, "log.txt")
	mustWrite(t, fp, "HELLO\nhello\nHeLLo\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "hello",
		"path":        fp,
		"output_mode": "count",
		"-i":          true,
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "3") {
		t.Errorf("expected count 3, got %q", res.Content)
	}
}

func TestGrepToolFallbackShowsLineNumbers(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	fp := filepath.Join(dir, "log.txt")
	mustWrite(t, fp, "first\nneedle\nthird\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        fp,
		"output_mode": "content",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "2:needle") {
		t.Errorf("expected '2:needle' (default line numbers), got %q", res.Content)
	}
}

func TestGrepToolNoMatchSimple(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "alpha\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
	})
	if res.IsError {
		t.Errorf("no match should not be an error: %q", res.Content)
	}
	if !strings.Contains(res.Content, "No files") && !strings.Contains(res.Content, "No matches") {
		t.Errorf("expected no-match message, got %q", res.Content)
	}
}

func TestGrepToolPathOutsideAllowed(t *testing.T) {
	scope := t.TempDir()
	other := t.TempDir()
	mustWrite(t, filepath.Join(other, "x.txt"), "needle\n")
	SetAllowedSearchDirs([]string{scope})
	t.Cleanup(func() { SetAllowedSearchDirs(nil) })

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    other,
	})
	if !res.IsError {
		t.Fatalf("expected error for out-of-scope path, got %q", res.Content)
	}
	if !strings.Contains(res.Content, "outside allowed directories") {
		t.Errorf("expected scope error, got %q", res.Content)
	}
}

func TestGrepToolMultilineFallbackSingleLine(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	fp := filepath.Join(dir, "x.txt")
	mustWrite(t, fp, "alpha\nbeta\ngamma\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "alpha",
		"path":        fp,
		"output_mode": "content",
		"multiline":   true,
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "alpha") {
		t.Errorf("expected alpha match in multiline mode, got %q", res.Content)
	}
}

func TestGrepToolRejectsUnknownSchemaKeys(t *testing.T) {
	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern":          "needle",
		"case_insensitive": true,
	})
	if !res.IsError || !strings.Contains(res.Content, "unexpected parameter `case_insensitive`") {
		t.Fatalf("expected strict schema unknown-field error, got %#v", res)
	}
}

func TestGrepToolSemanticQuotedNumbersAndBooleans(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "HELLO\nhello\n")

	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern":     "hello",
		"path":        dir,
		"output_mode": "count",
		"-i":          "true",
		"head_limit":  "1",
	})
	if res.IsError {
		t.Fatalf("quoted semantic values should parse: %s", res.Content)
	}
	if res.Metadata["num_matches"] != "2" {
		t.Fatalf("expected case-insensitive count through quoted bool, got %v / %q", res.Metadata, res.Content)
	}
}

func TestGrepToolPatternWhitespacePreserved(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "needle\n needle \n")

	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern":     " needle ",
		"path":        dir,
		"output_mode": "content",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if strings.Contains(res.Content, "1:needle") || !strings.Contains(res.Content, "2: needle ") {
		t.Fatalf("pattern whitespace should be preserved, got %q", res.Content)
	}
}

func TestGrepToolEmptyPatternIsForwarded(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	file := filepath.Join(t.TempDir(), "a.txt")
	mustWrite(t, file, "first\nsecond\n")
	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "", "path": file, "output_mode": "content",
	})
	if res.IsError || !strings.Contains(res.Content, "first") || !strings.Contains(res.Content, "second") {
		t.Fatalf("TS accepts an explicitly supplied empty regex pattern: error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGrepToolContextZeroOverridesBeforeAfter(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "before\nneedle\nafter\n")

	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        dir,
		"output_mode": "content",
		"context":     float64(0),
		"-A":          float64(1),
		"-B":          float64(1),
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if strings.Contains(res.Content, "before") || strings.Contains(res.Content, "after") {
		t.Fatalf("context:0 should override -A/-B and suppress context lines, got %q", res.Content)
	}
}

func TestGrepRunRipgrepUsesClaudeRGPath(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "rg")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'ripgrep 99.0.0'; exit 0; fi\necho \"$CLAUDE_FAKE_RG_OUTPUT\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake rg: %v", err)
	}
	t.Setenv("CLAUDE_RG_PATH", fake)
	t.Setenv("CLAUDE_FAKE_RG_OUTPUT", "from-override.txt")
	ResetRipgrepLocation()
	t.Cleanup(ResetRipgrepLocation)

	out, err := runRipgrep(context.Background(), []string{"-l", "needle"}, dir)
	if err != nil {
		t.Fatalf("runRipgrep with CLAUDE_RG_PATH: %v", err)
	}
	if len(out) != 1 || out[0] != "from-override.txt" {
		t.Fatalf("expected fake rg output, got %#v", out)
	}
}

func TestGrepSchemaAllowsRipgrepToValidateContextEdges(t *testing.T) {
	properties := (&GrepTool{}).Schema().Properties
	for _, key := range []string{"-B", "-A", "-C", "context", "head_limit", "offset"} {
		property, ok := properties[key].(map[string]any)
		if !ok {
			t.Fatalf("schema property %s has type %T", key, properties[key])
		}
		if minimum, exists := property["minimum"]; exists {
			t.Fatalf("%s must not advertise a minimum; rg owns edge validation (got %v)", key, minimum)
		}
	}
}

func TestGrepSemanticNumberOnlyCoercesTSDecimalLiterals(t *testing.T) {
	for _, raw := range []string{"1e3", "+1", ".5", "1.", "NaN", "Infinity", " 1"} {
		got, err := coerceGrepSemanticNumber(raw)
		if err != nil || got != raw {
			t.Fatalf("invalid TS decimal literal %q must pass through for schema rejection: got=%#v err=%v", raw, got, err)
		}
	}
	for _, raw := range []string{"0", "-5", "3.14", "01"} {
		got, err := coerceGrepSemanticNumber(raw)
		if err != nil {
			t.Fatalf("valid TS decimal literal %q: %v", raw, err)
		}
		if _, ok := got.(float64); !ok {
			t.Fatalf("valid TS decimal literal %q was not coerced: %#v", raw, got)
		}
	}
}

func TestGrepContextUsesSingleCFlagAndPreservesDecimal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell ripgrep fixture requires a POSIX shell")
	}
	argsFile := filepath.Join(t.TempDir(), "args")
	fake := writeFakeRipgrep(t, "printf '%s\\n' \"$@\" > \"$CLAUDE_FAKE_RG_ARGS\"; exit 2")
	withFakeRipgrep(t, fake)
	t.Setenv("CLAUDE_FAKE_RG_ARGS", argsFile)
	_, _ = (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": t.TempDir(), "output_mode": "content", "context": 1.5,
	})
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read rg args: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-C 1.5") || strings.Contains(joined, "-B") || strings.Contains(joined, "-A") {
		t.Fatalf("context must be passed as exact rg -C value, args=%q", joined)
	}
}

func TestGrepTypeUsageErrorComesFromSelectedRipgrep(t *testing.T) {
	fake := writeFakeRipgrep(t, "if [ \"$1\" = \"--type-list\" ]; then echo 'go: *.go'; exit 0; fi; echo 'custom rg type usage error' >&2; exit 2")
	withFakeRipgrep(t, fake)
	ResetGrepKnownTypesCache()
	t.Cleanup(ResetGrepKnownTypesCache)
	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": t.TempDir(), "type": "not-a-real-type",
	})
	if !res.IsError || !strings.Contains(res.Content, "custom rg type usage error") {
		t.Fatalf("type must be forwarded to rg without Go prevalidation: error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGrepMissingRipgrepDoesNotSilentlyFallback(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "")
	t.Setenv("CLAUDE_CODE_ALLOW_SEARCH_FALLBACK", "")
	t.Setenv("CLAUDE_RG_PATH", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	ResetRipgrepLocation()
	t.Cleanup(ResetRipgrepLocation)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "would-match.txt"), "needle\n")
	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{"pattern": "needle", "path": dir})
	if !res.IsError || !strings.Contains(strings.ToLower(res.Content), "ripgrep") {
		t.Fatalf("production missing-rg must surface locator failure, not fallback: error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGrepUNCPathRequiresPermissionAskWithoutProbe(t *testing.T) {
	tool := &GrepTool{}
	input := map[string]any{"pattern": "needle", "path": `\\\\server\\share`}
	decision, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("permission check: %v", err)
	}
	if decision.Behavior != types.PermissionBehaviorAsk || !decision.Required || !strings.Contains(decision.Message, "UNC") {
		t.Fatalf("UNC path must take required ask path without filesystem probing: %+v", decision)
	}
}

func TestRipgrepLocatorRejectsNonRipgrepOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell ripgrep fixture requires a POSIX shell")
	}
	fake := filepath.Join(t.TempDir(), "rg")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'not ripgrep'\n"), 0o755); err != nil {
		t.Fatalf("write fake rg: %v", err)
	}
	t.Setenv("CLAUDE_RG_PATH", fake)
	ResetRipgrepLocation()
	t.Cleanup(ResetRipgrepLocation)
	if _, err := LocateRipgrep(); err == nil || !strings.Contains(err.Error(), "failed --version sanity check") {
		t.Fatalf("non-ripgrep override must fail verification, got %v", err)
	}
}

func TestRipgrepLocatorFindsBundledHomeCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell ripgrep fixture requires a POSIX shell")
	}
	home := t.TempDir()
	embedded := filepath.Join(home, ".claude", "bin", "rg")
	if err := os.MkdirAll(filepath.Dir(embedded), 0o755); err != nil {
		t.Fatalf("mkdir embedded rg: %v", err)
	}
	if err := os.WriteFile(embedded, []byte("#!/bin/sh\necho 'ripgrep 99.0.0'\n"), 0o755); err != nil {
		t.Fatalf("write embedded rg: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CLAUDE_RG_PATH", "")
	t.Setenv("USE_BUILTIN_RIPGREP", "1")
	ResetRipgrepLocation()
	t.Cleanup(ResetRipgrepLocation)
	got, err := LocateRipgrep()
	if err != nil || filepath.Clean(got) != filepath.Clean(embedded) {
		t.Fatalf("embedded locator mismatch: path=%q err=%v", got, err)
	}
}

func TestRipgrepLocatorCacheResetSelectsNewOverride(t *testing.T) {
	first := writeFakeRipgrep(t, "exit 0")
	second := writeFakeRipgrep(t, "exit 0")
	t.Setenv("CLAUDE_RG_PATH", first)
	ResetRipgrepLocation()
	t.Cleanup(ResetRipgrepLocation)
	got, err := LocateRipgrep()
	if err != nil || got != first {
		t.Fatalf("first locator: path=%q err=%v", got, err)
	}
	t.Setenv("CLAUDE_RG_PATH", second)
	if cached, err := LocateRipgrep(); err != nil || cached != first {
		t.Fatalf("location must remain cached before reset: path=%q err=%v", cached, err)
	}
	ResetRipgrepLocation()
	if refreshed, err := LocateRipgrep(); err != nil || refreshed != second {
		t.Fatalf("location must refresh after reset: path=%q err=%v", refreshed, err)
	}
}

func TestGrepFilesWithMatchesUsesFilenameSortOnlyInNodeTestMode(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	t.Setenv("NODE_ENV", "test")
	dir := t.TempDir()
	newer := filepath.Join(dir, "z-newer.txt")
	older := filepath.Join(dir, "a-older.txt")
	mustWrite(t, newer, "needle\n")
	mustWrite(t, older, "needle\n")
	now := time.Now()
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatalf("chtime newer: %v", err)
	}
	if err := os.Chtimes(older, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("chtime older: %v", err)
	}
	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": dir, "head_limit": float64(1),
	})
	if res.IsError || !strings.Contains(res.Content, "a-older.txt") || strings.Contains(res.Content, "z-newer.txt") {
		t.Fatalf("NODE_ENV=test must use filename ordering before pagination: %q", res.Content)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
