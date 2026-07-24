// Package tools contains TS-alignment tests for GlobTool.
package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

// TestGlobAlignment_StructuredContentBlocks asserts that GlobTool.Execute
// emits structured ContentBlocks (matched-files JSON or similar), not just a
// plain newline-joined string in result.Content. Audit gap P2-1.
func TestGlobAlignment_StructuredContentBlocks(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), "package x")

	tool := &GlobTool{}
	res, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("Execute returned infra error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected non-error result, got: %s", res.Content)
	}
	if len(res.ContentBlocks) == 0 {
		t.Fatalf("expected structured ContentBlocks (TS multi-block contract), got plain Content=%q with no blocks", res.Content)
	}
	output, ok := res.Data.(GlobOutput)
	if !ok {
		t.Fatalf("expected GlobOutput typed data, got %T", res.Data)
	}
	if output.NumFiles != 1 || len(output.Filenames) != 1 || output.Filenames[0] == "" {
		t.Fatalf("unexpected typed output: %#v", output)
	}
}

// TestGlobAlignment_MetadataMatchedCount pins the legacy metadata mirror kept
// alongside the typed TS-compatible output.
func TestGlobAlignment_MetadataMatchedCount(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		mustWrite(t, filepath.Join(dir, name), "")
	}

	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if v := res.Metadata["matched_count"]; v != "3" {
		t.Fatalf("metadata.matched_count: want %q, got %q (full metadata=%v)", "3", v, res.Metadata)
	}
	if _, ok := res.Metadata["truncated"]; !ok {
		t.Fatalf("metadata.truncated must always be present, got map=%v", res.Metadata)
	}
}

// TestGlobAlignment_BracketCharClassNegation checks that minimatch-style
// negation `[!abc].txt` matches files that do NOT start with a/b/c. The TS
// reference (minimatch) accepts this; doublestar/v4 used by the Go side
// expects `[^abc]` instead.
func TestGlobAlignment_BracketCharClassNegation(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "d.txt"} {
		mustWrite(t, filepath.Join(dir, name), "")
	}

	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "[!abc].txt",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "d.txt") {
		t.Fatalf("expected d.txt to match [!abc].txt (TS minimatch negation), got: %s", res.Content)
	}
	if strings.Contains(res.Content, "a.txt") {
		t.Fatalf("a.txt must NOT match [!abc].txt, got: %s", res.Content)
	}
}

// TestGlobAlignment_ExtGlobAlternation tests that minimatch-style extglob
// alternation `+(foo|bar).go` matches foo.go and bar.go but not baz.go.
// doublestar accepts {foo,bar} brace expansion but not the +(…) extglob form
// the TS minimatch reference understands.
func TestGlobAlignment_ExtGlobAlternation(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"foo.go", "bar.go", "baz.go"} {
		mustWrite(t, filepath.Join(dir, name), "")
	}

	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "+(foo|bar).go",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "foo.go") || !strings.Contains(res.Content, "bar.go") {
		t.Fatalf("expected +(foo|bar).go to match foo.go and bar.go (TS minimatch parity), got: %s", res.Content)
	}
	if strings.Contains(res.Content, "baz.go") {
		t.Fatalf("baz.go must not match, got: %s", res.Content)
	}
}

// TestGlobAlignment_ModtimeSortAscending asserts TS ripgrep --sort=modified
// order: oldest first.
func TestGlobAlignment_ModtimeSortAscending(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "older.txt")
	newer := filepath.Join(dir, "newer.txt")
	mustWrite(t, older, "")
	mustWrite(t, newer, "")

	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	olderIdx := strings.Index(res.Content, "older.txt")
	newerIdx := strings.Index(res.Content, "newer.txt")
	if olderIdx < 0 || newerIdx < 0 {
		t.Fatalf("missing files in output: %s", res.Content)
	}
	if olderIdx > newerIdx {
		t.Fatalf("expected oldest-first ordering (older.txt before newer.txt), got older@%d newer@%d in:\n%s",
			olderIdx, newerIdx, res.Content)
	}
}

func TestGlobAlignment_OutputContractText(t *testing.T) {
	tool := &GlobTool{}
	empty := globModelContent(GlobOutput{})
	if empty != "No files found" {
		t.Fatalf("empty text = %q", empty)
	}
	mapped := tool.MapToolResultToToolResultBlock(GlobOutput{
		Filenames: []string{"a.go", "b.go"},
		Truncated: true,
	}, "toolu_1")
	want := "a.go\nb.go\n(Results are truncated. Consider using a more specific path or pattern.)"
	if mapped.Content != want {
		t.Fatalf("mapped content = %q want %q", mapped.Content, want)
	}
	if mapped.ToolUseID != "toolu_1" || mapped.Type != types.ContentTypeToolResult {
		t.Fatalf("mapped identity/type mismatch: %#v", mapped)
	}
}

func TestGlobAlignment_StrictUnknownFieldRejected(t *testing.T) {
	res, err := (&GlobTool{}).Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    t.TempDir(),
		"limit":   10,
	})
	if err != nil {
		t.Fatalf("Execute returned infra error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "unexpected parameter `limit`") {
		t.Fatalf("expected strict unknown-field error, got error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGlobAlignment_MetadataAndClassifierContract(t *testing.T) {
	tool := &GlobTool{}
	metadata := tool.ToolMetadata(map[string]any{"pattern": "**/*.go"})
	if !metadata.ReadOnly || !metadata.Search || !metadata.ConcurrencySafe || metadata.MaxResultSizeChars != 100_000 {
		t.Fatalf("Glob metadata mismatch: %+v", metadata)
	}
	if got := types.ToolAutoClassifierInput(tool, map[string]any{"pattern": " **/*.go "}); got != "**/*.go" {
		t.Fatalf("auto classifier input = %q", got)
	}
	classification := types.ToolSearchRead(tool, map[string]any{"pattern": "**/*.go"})
	if !classification.IsSearch || classification.IsRead {
		t.Fatalf("search/read classification mismatch: %+v", classification)
	}
}

// TestGlobAlignment_UpperBoundLimitDocumented asserts that the result metadata
// exposes the configured upper bound (defaultGlobLimit=100).
func TestGlobAlignment_UpperBoundLimitDocumented(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "x.go"), "")

	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    dir,
	})
	if v := res.Metadata["max_results"]; v == "" {
		t.Fatalf("metadata.max_results must surface upper bound; got map=%v", res.Metadata)
	}
}

// TestGlobAlignment_TruncationSentinelInMetadata asserts that when the
// result count exceeds defaultGlobLimit, the truncated flag is set in
// metadata (not embedded as a free-text suffix in Content). This pins the
// machine-readable contract.
func TestGlobAlignment_TruncationSentinelInMetadata(t *testing.T) {
	dir := t.TempDir()
	// Create defaultGlobLimit+1 files so truncation kicks in.
	for i := 0; i < defaultGlobLimit+1; i++ {
		mustWrite(t, filepath.Join(dir, "f"+pad(i)+".go"), "")
	}

	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if res.Metadata["truncated"] != "true" {
		t.Fatalf("metadata.truncated should be %q when over limit, got %q (map=%v)",
			"true", res.Metadata["truncated"], res.Metadata)
	}
	if !strings.Contains(res.Content, "(Results are truncated. Consider using a more specific path or pattern.)") {
		t.Fatalf("expected TS truncation suffix, got %q", res.Content)
	}
	output, ok := res.Data.(GlobOutput)
	if !ok || !output.Truncated || output.NumFiles != defaultGlobLimit {
		t.Fatalf("unexpected typed output: %#v (ok=%v)", res.Data, ok)
	}
}

func TestGlobAlignment_IgnoreDoesNotConsumeLimitSlots(t *testing.T) {
	dir := t.TempDir()
	ignored := filepath.Join(dir, "ignored")
	if err := os.MkdirAll(ignored, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	for i := 0; i < defaultGlobLimit+20; i++ {
		path := filepath.Join(ignored, "old-"+pad(i)+".go")
		mustWrite(t, path, "")
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes ignored: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		mustWrite(t, filepath.Join(dir, "keep-"+pad(i)+".go"), "")
	}

	SetFileReadIgnorePatterns([]string{"ignored/**"})
	t.Cleanup(func() { SetFileReadIgnorePatterns(nil) })

	res, err := (&GlobTool{}).Execute(context.Background(), map[string]any{
		"pattern": "**/*.go",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("Execute returned infra error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(res.Content, "keep-"+pad(i)+".go") {
			t.Fatalf("kept file missing after ignored over-limit set: %q", res.Content)
		}
	}
	if strings.Contains(res.Content, "ignored/") {
		t.Fatalf("ignored path leaked into output: %q", res.Content)
	}
}

func pad(n int) string {
	s := ""
	switch {
	case n < 10:
		s = "00"
	case n < 100:
		s = "0"
	}
	return s + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}
