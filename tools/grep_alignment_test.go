// Package tools contains TS-alignment tests for GrepTool.
package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

// TestGrepAlignment_StructuredContentBlocks asserts that GrepTool.Execute
// emits structured ContentBlocks rather than just newline-joined text in
// result.Content. Audit gap P2-2 (output contract).
func TestGrepAlignment_StructuredContentBlocks(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "needle in a haystack\n")

	tool := &GrepTool{}
	res, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        dir,
		"output_mode": "content",
	})
	if err != nil {
		t.Fatalf("infra error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if len(res.ContentBlocks) == 0 {
		t.Fatalf("expected structured ContentBlocks (TS contract), got plain Content=%q with no blocks", res.Content)
	}
}

// TestGrepAlignment_MetadataMatchCount asserts that the result Metadata map
// surfaces match_count so callers can paginate and report.
func TestGrepAlignment_MetadataMatchCount(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "needle\nneedle\nhaystack\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        dir,
		"output_mode": "content",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if v := res.Metadata["match_count"]; v != "2" {
		t.Fatalf("metadata.match_count: want %q, got %q (full metadata=%v)", "2", v, res.Metadata)
	}
}

// TestGrepAlignment_GlobStartingWithDashIsForwardedAsGlob confirms that
// leading-dash glob values are treated as rg --glob values, matching TS.
func TestGrepAlignment_GlobStartingWithDashIsForwardedAsGlob(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "secret token\n")
	mustWrite(t, filepath.Join(dir, "b.go"), "secret token\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "secret",
		"path":        dir,
		"glob":        "--include=*.go",
		"output_mode": "files_with_matches",
	})
	if res.IsError {
		t.Fatalf("leading-dash glob should be forwarded as a glob value, got error: %s", res.Content)
	}
	if res.Content != "No files found" {
		t.Fatalf("leading-dash glob should not be interpreted as an rg flag, got %q", res.Content)
	}
}

// TestGrepAlignment_MetadataOutputMode asserts that the resolved output_mode
// (after defaulting) is surfaced in Metadata.
func TestGrepAlignment_MetadataOutputMode(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "x\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "missing",
		"path":    dir,
	})
	if res.Metadata["output_mode"] != "files_with_matches" {
		t.Fatalf("metadata.output_mode: want %q, got %q (map=%v)",
			"files_with_matches", res.Metadata["output_mode"], res.Metadata)
	}
}

// TestGrepAlignment_HeadLimitTruncationFlag asserts truncated flag is set in
// metadata when more matches exist than the head_limit.
func TestGrepAlignment_HeadLimitTruncationFlag(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		mustWrite(t, filepath.Join(dir, "f"+pad(i)+".txt"), "match\n")
	}

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "match",
		"path":        dir,
		"output_mode": "files_with_matches",
		"head_limit":  float64(2),
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if res.Metadata["truncated"] != "true" {
		t.Fatalf("metadata.truncated should be %q after head_limit clipping, got %q (map=%v)",
			"true", res.Metadata["truncated"], res.Metadata)
	}
}

// TestGrepAlignment_MultilineDotallStructuredOutput asserts that even in
// multiline mode the structured contract holds (ContentBlocks present and
// metadata reports the multiline flag).
func TestGrepAlignment_MultilineDotallStructuredOutput(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "alpha\nbravo\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     `alpha\sbravo`,
		"path":        dir,
		"output_mode": "content",
		"multiline":   true,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if len(res.ContentBlocks) == 0 {
		t.Fatalf("expected ContentBlocks for multiline match (structured contract), got Content=%q", res.Content)
	}
	if res.Metadata["multiline"] != "true" {
		t.Fatalf("metadata.multiline should mirror input flag; want %q, got %q (map=%v)",
			"true", res.Metadata["multiline"], res.Metadata)
	}
}

// TestGrepAlignment_NoMatchesMetadataPresent asserts that even the empty
// result path still emits Metadata (callers shouldn't have to special-case
// nil maps based on hit count).
func TestGrepAlignment_NoMatchesMetadataPresent(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "alpha\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "zzz_no_hit_zzz",
		"path":        dir,
		"output_mode": "files_with_matches",
	})
	if res.Metadata == nil {
		t.Fatalf("Metadata must always be present, got nil (Content=%q)", res.Content)
	}
	if v := res.Metadata["match_count"]; v != "0" {
		t.Fatalf("metadata.match_count on no-match path: want %q, got %q (map=%v)",
			"0", v, res.Metadata)
	}
}

// TestGrepAlignment_TypeFilterMetadata asserts that the resolved --type
// filter is surfaced in Metadata so callers can audit what filtered the
// results.
func TestGrepAlignment_TypeFilterMetadata(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), "needle\n")
	mustWrite(t, filepath.Join(dir, "b.txt"), "needle\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        dir,
		"type":        "go",
		"output_mode": "files_with_matches",
	})
	if res.Metadata["type"] != "go" {
		t.Fatalf("metadata.type should mirror input filter; want %q, got %q (map=%v)",
			"go", res.Metadata["type"], res.Metadata)
	}
}

// TestGrepAlignment_CountModeNumericMetadata asserts that count mode emits
// numeric structured payload in metadata (TS reference exposes `numMatches`
// in the JSON block, not a free-text "path:N" line).
func TestGrepAlignment_CountModeNumericMetadata(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "needle\nneedle\nhaystack\n")

	tool := &GrepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        dir,
		"output_mode": "count",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if res.Metadata["num_matches"] != "2" {
		t.Fatalf("metadata.num_matches in count mode: want %q, got %q (map=%v)",
			"2", res.Metadata["num_matches"], res.Metadata)
	}
}

// TestGrepAlignment_DurationMetadata asserts that GrepTool surfaces a
// duration_ms field in Metadata so the renderer can display search latency.
// The production path emits duration metadata for UI/result summaries.
func TestGrepAlignment_DurationMetadata(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "needle\n")

	tool := &GrepTool{}
	res, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        dir,
		"output_mode": "files_with_matches",
	})
	if err != nil {
		t.Fatalf("infra error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	v := res.Metadata["duration_ms"]
	if v == "" {
		t.Fatalf("metadata.duration_ms must be present (TS GrepTool emits durationMs); got map=%v", res.Metadata)
	}
	// Must parse as an integer ≥ 0 — pin the contract shape.
	if _, err := strconv.Atoi(v); err != nil {
		t.Fatalf("metadata.duration_ms must parse as int; got %q (err=%v)", v, err)
	}
}

func TestGrepAlignment_TypedDataAndFilesText(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "needle\n")

	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
	})
	output, ok := res.Data.(GrepOutput)
	if !ok {
		t.Fatalf("expected GrepOutput data, got %T", res.Data)
	}
	if output.Mode != "files_with_matches" || output.NumFiles != 1 || len(output.Filenames) != 1 {
		t.Fatalf("unexpected typed output: %#v", output)
	}
	if !strings.HasPrefix(res.Content, "Found 1 file\n") {
		t.Fatalf("expected TS files_with_matches header, got %q", res.Content)
	}
}

func TestGrepAlignment_CountModeSummaryText(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "needle\nneedle\n")

	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        dir,
		"output_mode": "count",
	})
	if !strings.Contains(res.Content, "Found 2 total occurrences across 1 file.") {
		t.Fatalf("expected TS count summary, got %q", res.Content)
	}
	output := res.Data.(GrepOutput)
	if output.NumMatches != 2 || output.NumFiles != 1 {
		t.Fatalf("unexpected count output: %#v", output)
	}
}

func TestGrepAlignment_FileReadIgnoreDoesNotConsumeLimitSlots(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ignored"), 0o755); err != nil {
		t.Fatalf("mkdir ignored: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "visible"), 0o755); err != nil {
		t.Fatalf("mkdir visible: %v", err)
	}
	mustWrite(t, filepath.Join(dir, "ignored", "a.txt"), "needle\n")
	mustWrite(t, filepath.Join(dir, "visible", "b.txt"), "needle\n")
	SetFileReadIgnorePatterns([]string{"ignored/**"})
	t.Cleanup(func() { SetFileReadIgnorePatterns(nil) })

	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern":    "needle",
		"path":       dir,
		"head_limit": float64(1),
	})
	if strings.Contains(res.Content, "ignored") {
		t.Fatalf("ignored path leaked into grep output: %q", res.Content)
	}
	if !strings.Contains(res.Content, "visible") {
		t.Fatalf("visible path should remain after ignored path is filtered: %q", res.Content)
	}
}

func TestGrepAlignment_FilesWithMatchesNewestFirst(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	mustWrite(t, oldPath, "needle\n")
	mustWrite(t, newPath, "needle\n")
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now()
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtime old: %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("chtime new: %v", err)
	}

	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern":    "needle",
		"path":       dir,
		"head_limit": float64(1),
	})
	lines := grepResultPayloadLines(res.Content)
	if len(lines) != 1 || !strings.Contains(lines[0], "new.txt") {
		t.Fatalf("expected newest file first, got %q", res.Content)
	}
}

func TestGrepAlignment_ScopedDenyRulesDoNotConsumeLimitSlots(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	SetFileReadIgnorePatterns(nil)
	t.Cleanup(func() { SetFileReadIgnorePatterns(nil) })
	dir := t.TempDir()
	denied := filepath.Join(dir, "denied", "new.txt")
	visible := filepath.Join(dir, "visible", "old.txt")
	if err := os.MkdirAll(filepath.Dir(denied), 0o755); err != nil {
		t.Fatalf("mkdir denied: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(visible), 0o755); err != nil {
		t.Fatalf("mkdir visible: %v", err)
	}
	mustWrite(t, denied, "needle\n")
	mustWrite(t, visible, "needle\n")
	now := time.Now()
	if err := os.Chtimes(denied, now, now); err != nil {
		t.Fatalf("chtime denied: %v", err)
	}
	if err := os.Chtimes(visible, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("chtime visible: %v", err)
	}

	scope := NewRuntimeScope(dir, true)
	scope.SetAllowedDirs([]string{dir})
	scope.SetDeniedTools([]string{"Read(denied/**)"})
	res, _ := NewGrepTool(scope).Execute(context.Background(), map[string]any{
		"pattern": "needle", "head_limit": float64(1),
	})
	if res.IsError || strings.Contains(res.Content, "denied") || !strings.Contains(res.Content, "visible") {
		t.Fatalf("runtime deny rule must prefilter before pagination: error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGrepAlignment_ScopedPluginExclusionDoesNotConsumeLimitSlots(t *testing.T) {
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	pluginsDir := t.TempDir()
	t.Setenv("CLAUDE_CODE_PLUGIN_CACHE_DIR", pluginsDir)
	cacheRoot := filepath.Join(pluginsDir, "cache")
	orphaned := filepath.Join(cacheRoot, "market", "plugin", "1.0.0")
	current := filepath.Join(cacheRoot, "market", "plugin", "2.0.0")
	if err := os.MkdirAll(orphaned, 0o755); err != nil {
		t.Fatalf("mkdir orphaned: %v", err)
	}
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatalf("mkdir current: %v", err)
	}
	mustWrite(t, filepath.Join(orphaned, orphanedPluginMarker), "marked\n")
	orphanedFile := filepath.Join(orphaned, "new.txt")
	currentFile := filepath.Join(current, "old.txt")
	mustWrite(t, orphanedFile, "needle\n")
	mustWrite(t, currentFile, "needle\n")
	now := time.Now()
	if err := os.Chtimes(orphanedFile, now, now); err != nil {
		t.Fatalf("chtime orphaned: %v", err)
	}
	if err := os.Chtimes(currentFile, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("chtime current: %v", err)
	}

	scope := NewRuntimeScope(cacheRoot, true)
	scope.SetAllowedDirs([]string{cacheRoot})
	res, _ := NewGrepTool(scope).Execute(context.Background(), map[string]any{
		"pattern": "needle", "head_limit": float64(1),
	})
	if res.IsError || strings.Contains(res.Content, "1.0.0") || !strings.Contains(res.Content, "2.0.0") {
		t.Fatalf("orphaned plugin must be excluded before pagination: error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGrepAlignment_MetadataDescriptionAndClassifiers(t *testing.T) {
	tool := &GrepTool{}
	description := tool.Description()
	for _, required := range []string{"ALWAYS use Grep", "NEVER invoke `grep` or `rg`", "full regex syntax", "glob", "type", "files_with_matches", "Agent", "literal braces", "multiline"} {
		if !strings.Contains(description, required) {
			t.Fatalf("Grep description missing %q: %q", required, description)
		}
	}
	metadata := tool.ToolMetadata(nil)
	if !metadata.ReadOnly || !metadata.Search || !metadata.ConcurrencySafe || metadata.MaxResultSizeChars != 20_000 {
		t.Fatalf("Grep metadata mismatch: %+v", metadata)
	}
	input := map[string]any{"pattern": "needle", "path": "src"}
	if got := types.ToolAutoClassifierInput(tool, input); got != "needle in src" {
		t.Fatalf("auto-classifier input = %q", got)
	}
	if got := types.ToolSearchRead(tool, input); !got.IsSearch || got.IsRead {
		t.Fatalf("search/read classification mismatch: %+v", got)
	}
}

func TestGrepAlignment_ExactModelTextGolden(t *testing.T) {
	tests := []struct {
		name string
		data GrepOutput
		want string
	}{
		{name: "content empty", data: GrepOutput{Mode: "content", Filenames: []string{}}, want: "No matches found"},
		{name: "content pagination", data: GrepOutput{Mode: "content", Filenames: []string{}, Content: "a.go:1:needle", NumLines: 1, AppliedLimit: 1, AppliedOffset: 2}, want: "a.go:1:needle\n\n[Showing results with pagination = limit: 1, offset: 2]"},
		{name: "count empty", data: GrepOutput{Mode: "count", Filenames: []string{}}, want: "No matches found\n\nFound 0 total occurrences across 0 files."},
		{name: "count singular pagination", data: GrepOutput{Mode: "count", Filenames: []string{}, Content: "a.go:1", NumFiles: 1, NumMatches: 1, AppliedLimit: 1, AppliedOffset: 3}, want: "a.go:1\n\nFound 1 total occurrence across 1 file. with pagination = limit: 1, offset: 3"},
		{name: "files empty", data: GrepOutput{Mode: "files_with_matches", Filenames: []string{}}, want: "No files found"},
		{name: "files pagination", data: GrepOutput{Mode: "files_with_matches", NumFiles: 2, Filenames: []string{"a.go", "b.go"}, AppliedLimit: 2, AppliedOffset: 4}, want: "Found 2 files limit: 2, offset: 4\na.go\nb.go"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := grepModelContent(test.data); got != test.want {
				t.Fatalf("model text mismatch\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

func TestGrepAlignment_SingleFileCountUsesUsefulStructuredSummary(t *testing.T) {
	// TS currently parses only filename:count lines, so rg's single-file "N"
	// output produces a misleading zero summary. The Go clone deliberately
	// fixes that source bug while retaining the same structured fields/text.
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	file := filepath.Join(t.TempDir(), "single.txt")
	mustWrite(t, file, "needle\nneedle\n")
	res, _ := (&GrepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": file, "output_mode": "count",
	})
	output, ok := res.Data.(GrepOutput)
	if res.IsError || !ok || output.NumFiles != 1 || output.NumMatches != 2 {
		t.Fatalf("single-file count structured result mismatch: error=%v data=%#v content=%q", res.IsError, res.Data, res.Content)
	}
	if res.Content != "2\n\nFound 2 total occurrences across 1 file." {
		t.Fatalf("single-file count model text mismatch: %q", res.Content)
	}
}

func TestGrepAlignment_ResultBlockCarries20KPersistenceThreshold(t *testing.T) {
	tool := &GrepTool{}
	block := types.MapToolResult(tool, types.ToolResult{Data: GrepOutput{
		Mode: "files_with_matches", NumFiles: 1, Filenames: []string{"a.go"},
	}}, "toolu_grep")
	if block.Metadata["maxResultSizeChars"] != "20000" {
		t.Fatalf("Grep result block threshold mismatch: %#v", block.Metadata)
	}
	if block.Content != "Found 1 file\na.go" {
		t.Fatalf("mapped Grep model content mismatch: %q", block.Content)
	}
}

func TestGrepAlignment_RuntimeDenyRulesBecomeRipgrepGlobs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell ripgrep fixture requires a POSIX shell")
	}
	root := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	fake := writeFakeRipgrep(t, "printf '%s\\n' \"$@\" > \"$CLAUDE_FAKE_RG_ARGS\"")
	withFakeRipgrep(t, fake)
	t.Setenv("CLAUDE_FAKE_RG_ARGS", argsFile)
	scope := NewRuntimeScope(root, true)
	scope.SetAllowedDirs([]string{root})
	scope.SetDeniedTools([]string{"Read(secret/**)"})
	res, _ := NewGrepTool(scope).Execute(context.Background(), map[string]any{"pattern": "needle"})
	if res.IsError {
		t.Fatalf("grep fake invocation: %s", res.Content)
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read fake rg args: %v", err)
	}
	if !strings.Contains(string(data), "!secret/**") {
		t.Fatalf("runtime Read deny rule was not sent as negative --glob: %q", data)
	}
}

func TestGrepAlignment_MtimeNativeAndFallbackSelectSameFirstPage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell ripgrep fixture requires a POSIX shell")
	}
	t.Setenv("NODE_ENV", "")
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.txt")
	midPath := filepath.Join(root, "mid.txt")
	newPath := filepath.Join(root, "new.txt")
	for _, file := range []string{oldPath, midPath, newPath} {
		mustWrite(t, file, "needle\n")
	}
	now := time.Now()
	for file, modTime := range map[string]time.Time{
		oldPath: now.Add(-2 * time.Hour), midPath: now.Add(-time.Hour), newPath: now,
	} {
		if err := os.Chtimes(file, modTime, modTime); err != nil {
			t.Fatalf("chtime %s: %v", file, err)
		}
	}
	fake := writeFakeRipgrep(t, "printf '%s\\n%s\\n%s\\n' \"$CLAUDE_FAKE_OLD\" \"$CLAUDE_FAKE_NEW\" \"$CLAUDE_FAKE_MID\"")
	withFakeRipgrep(t, fake)
	t.Setenv("CLAUDE_FAKE_OLD", oldPath)
	t.Setenv("CLAUDE_FAKE_MID", midPath)
	t.Setenv("CLAUDE_FAKE_NEW", newPath)
	input := map[string]any{"pattern": "needle", "path": root, "head_limit": float64(2)}
	native, _ := (&GrepTool{}).Execute(context.Background(), input)
	if native.IsError {
		t.Fatalf("native Grep: %s", native.Content)
	}
	t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	fallback, _ := (&GrepTool{}).Execute(context.Background(), input)
	if fallback.IsError {
		t.Fatalf("fallback Grep: %s", fallback.Content)
	}
	nativeFiles := native.Data.(GrepOutput).Filenames
	fallbackFiles := fallback.Data.(GrepOutput).Filenames
	want := []string{newPath, midPath}
	if strings.Join(nativeFiles, "|") != strings.Join(want, "|") || strings.Join(fallbackFiles, "|") != strings.Join(want, "|") {
		t.Fatalf("native/fallback first page mismatch: native=%v fallback=%v want=%v", nativeFiles, fallbackFiles, want)
	}
}
