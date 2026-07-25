// Package file — file_edit_conformance_test.go is the high-volume
// conformance suite for FileEditTool. It mirrors the describe-block
// structure of the TS reference (src/tools/FileEditTool/FileEditTool.ts +
// utils.ts) and pins the user-facing behaviour the runtime promises:
//
//   - identical-strings rejection
//   - missing-string rejection
//   - ambiguous-match guard (single-edit and replace_all paths)
//   - new-file creation via empty old_string
//   - line-ending preservation (LF / CRLF)
//   - Read-before-Edit gating, partial-view rejection, stale detection
//   - .ipynb hand-off rejection
//   - symlink rejection
//   - structured patch payload shape
//
// Every test case is self-contained: no shared helpers are imported from
// other test files except the standard library.
package file

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

// ----- helpers -----------------------------------------------------------

// editFixture creates a temp file with `content` and pre-records a Read
// entry so the Read-before-Edit guard passes for full-content edits. It
// returns the path, the ReadFileState, and the resolved abs path.
func editFixture(t *testing.T, name, content string) (string, *ReadFileState, string) {
	t.Helper()
	dir := t.TempDir()
	fp := filepath.Join(dir, name)
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	state := NewReadFileState()
	abs, _ := filepath.Abs(fp)
	recordStrongReadEvidenceForTest(t, state, abs)
	return fp, state, abs
}

// runEdit executes a FileEditTool against the supplied state and input.
func runEdit(t *testing.T, state *ReadFileState, input map[string]any) (*FileEditTool, types.ToolResult) {
	t.Helper()
	tool := &FileEditTool{ReadState: state}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned non-nil err: %v", err)
	}
	return tool, result
}

// decodeEditResult unmarshals the EditResult JSON inside a successful
// ToolResult. Fails the test if unmarshalling fails or the result is an
// error.
func decodeEditResult(t *testing.T, content string) EditResult {
	t.Helper()
	var r EditResult
	if err := json.Unmarshal([]byte(content), &r); err != nil {
		t.Fatalf("unmarshal EditResult: %v\nraw=%s", err, content)
	}
	return r
}

// ----- 1. apply-engine sentinel cases ------------------------------------

func TestEditConf_ApplyEdit_IdenticalStrings(t *testing.T) {
	_, _, err := ApplyEdit("hello", "x", "x", false)
	if !errors.Is(err, ErrEditIdenticalStrings) {
		t.Fatalf("expected ErrEditIdenticalStrings, got %v", err)
	}
}

func TestEditConf_ApplyEdit_OldStringMissing(t *testing.T) {
	_, _, err := ApplyEdit("hello world", "missing", "found", false)
	if !errors.Is(err, ErrEditOldStringMissing) {
		t.Fatalf("expected ErrEditOldStringMissing, got %v", err)
	}
}

func TestEditConf_ApplyEdit_EmptyOldStringOnNonEmpty(t *testing.T) {
	_, _, err := ApplyEdit("non-empty", "", "x", false)
	if !errors.Is(err, ErrEditEmptyOldString) {
		t.Fatalf("expected ErrEditEmptyOldString, got %v", err)
	}
}

func TestEditConf_ApplyEdit_EmptyOldStringOnEmpty(t *testing.T) {
	out, n, err := ApplyEdit("", "", "fresh", false)
	if err != nil {
		t.Fatalf("expected success on empty file with empty old_string, got %v", err)
	}
	if out != "fresh" || n != 1 {
		t.Fatalf("expected ('fresh', 1), got (%q, %d)", out, n)
	}
}

func TestEditConf_ApplyEdit_AmbiguousWithoutReplaceAll(t *testing.T) {
	_, n, err := ApplyEdit("aa bb aa", "aa", "zz", false)
	if err == nil {
		t.Fatal("expected ambiguous-match error, got nil")
	}
	if !errors.Is(err, ErrEditAmbiguousMatch) {
		t.Fatalf("expected ErrEditAmbiguousMatch, got %q", err.Error())
	}
	if n != 2 {
		t.Fatalf("expected occurrences=2 in error path, got %d", n)
	}
}

func TestEditConf_ApplyEdit_ReplaceAllAcceptsMany(t *testing.T) {
	out, n, err := ApplyEdit("aa bb aa cc aa", "aa", "ZZ", true)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 occurrences, got %d", n)
	}
	if out != "ZZ bb ZZ cc ZZ" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestEditConf_ApplyEdit_SingleMatch(t *testing.T) {
	out, n, err := ApplyEdit("foo bar baz", "bar", "QUX", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 || out != "foo QUX baz" {
		t.Fatalf("unexpected (%q, %d)", out, n)
	}
}

func TestEditConf_ApplyEdit_TrailingNewlineHack(t *testing.T) {
	// When new_string is "" and old_string lacks a trailing newline but the
	// file has the line followed by a newline, ApplyEdit consumes that
	// trailing newline so we don't leave a blank line behind.
	out, n, err := ApplyEdit("alpha\nbeta\ngamma\n", "beta", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 occurrence, got %d", n)
	}
	if out != "alpha\ngamma\n" {
		t.Fatalf("trailing-newline hack failed: %q", out)
	}
}

func TestEditConf_QuoteNormalization_MatchesCurlyQuotes(t *testing.T) {
	content := "const msg = “hello”\n"
	actual, ok := findActualString(content, `const msg = "hello"`)
	if !ok {
		t.Fatal("expected straight-quote search to match curly-quote file content")
	}
	if actual != "const msg = “hello”" {
		t.Fatalf("unexpected actual match: %q", actual)
	}
	replacement := preserveQuoteStyle(`const msg = "hello"`, actual, `const msg = "goodbye"`)
	if replacement != "const msg = “goodbye”" {
		t.Fatalf("expected replacement to preserve curly quote style, got %q", replacement)
	}
}

// ----- 2. line-ending detection ------------------------------------------

func TestEditConf_LineEnding_DetectLF(t *testing.T) {
	if got := detectLineEnding("a\nb\nc"); got != "\n" {
		t.Fatalf("expected LF, got %q", got)
	}
}

func TestEditConf_LineEnding_DetectCRLF(t *testing.T) {
	if got := detectLineEnding("a\r\nb\r\nc"); got != "\r\n" {
		t.Fatalf("expected CRLF, got %q", got)
	}
}

func TestEditConf_LineEnding_PredominantCRLFWins(t *testing.T) {
	if got := detectLineEnding("a\r\nb\r\nc\nd"); got != "\r\n" {
		t.Fatalf("expected CRLF (2 vs 1 bare LF), got %q", got)
	}
}

func TestEditConf_LineEnding_BareLFInMostlyLF(t *testing.T) {
	// One CRLF amongst many LFs — bare LFs outnumber CRLF, file is LF.
	if got := detectLineEnding("a\nb\nc\nd\r\ne\nf"); got != "\n" {
		t.Fatalf("expected LF, got %q", got)
	}
}

func TestEditConf_LineEnding_RoundtripCRLF(t *testing.T) {
	original := "line1\r\nline2\r\nline3\r\n"
	lf := normaliseToLF(original)
	if strings.Contains(lf, "\r") {
		t.Fatalf("normaliseToLF leaked CR: %q", lf)
	}
	back := restoreLineEnding(lf, "\r\n")
	if back != original {
		t.Fatalf("CRLF roundtrip failed: got %q want %q", back, original)
	}
}

func TestEditConf_LineEnding_RestoreToLFNoOp(t *testing.T) {
	if got := restoreLineEnding("a\nb\nc", "\n"); got != "a\nb\nc" {
		t.Fatalf("LF restore should be no-op, got %q", got)
	}
}

// ----- 4. tool-level: validation rejections ------------------------------

func TestEditConf_Tool_ReportsIdenticalStringsAsUnchanged(t *testing.T) {
	fp, state, _ := editFixture(t, "a.txt", "hello world")
	_, tr := runEdit(t, state, map[string]any{
		"file_path":  fp,
		"old_string": "hello",
		"new_string": "hello",
	})
	if tr.IsError || tr.Outcome != types.ToolOutcomeSucceeded || tr.Metadata["semanticCategory"] != "unchanged" {
		t.Fatalf("expected successful unchanged result, got %+v", tr)
	}
	if !strings.Contains(tr.Content, "exactly the same") {
		t.Fatalf("expected 'exactly the same' message, got %q", tr.Content)
	}
}

func TestEditConf_Tool_RejectsMissingFilePath(t *testing.T) {
	tool := &FileEditTool{ReadState: NewReadFileState()}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"old_string": "x",
		"new_string": "y",
	})
	if !tr.IsError {
		t.Fatal("expected error when file_path missing")
	}
}

func TestEditConf_Tool_RejectsMissingOldString(t *testing.T) {
	tool := &FileEditTool{ReadState: NewReadFileState()}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  filepath.Join(t.TempDir(), "x.txt"),
		"new_string": "y",
	})
	if !tr.IsError {
		t.Fatal("expected error when old_string missing")
	}
}

func TestEditConf_Tool_RejectsMissingNewString(t *testing.T) {
	tool := &FileEditTool{ReadState: NewReadFileState()}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  filepath.Join(t.TempDir(), "x.txt"),
		"old_string": "y",
	})
	if !tr.IsError {
		t.Fatal("expected error when new_string missing")
	}
}

func TestEditConf_Tool_RejectsIpynb(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "notebook.ipynb")
	os.WriteFile(fp, []byte("{}"), 0o644)
	tool := &FileEditTool{ReadState: NewReadFileState()}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "a",
		"new_string": "b",
	})
	if !tr.IsError || !strings.Contains(tr.Content, "NotebookEdit") {
		t.Fatalf("expected NotebookEdit hand-off message, got isErr=%v content=%q", tr.IsError, tr.Content)
	}
}

func TestEditConf_Tool_RejectsRelativePath(t *testing.T) {
	tool := &FileEditTool{ReadState: NewReadFileState()}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  "relative/path.txt",
		"old_string": "x",
		"new_string": "y",
	})
	if !tr.IsError {
		t.Fatal("expected error for relative path (allowed-dirs check)")
	}
}

// ----- 5. tool-level: Read-before-Edit gating ----------------------------

func TestEditConf_Tool_RejectsEditWithoutPriorRead(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "f.txt")
	os.WriteFile(fp, []byte("hello"), 0o644)
	tool := &FileEditTool{ReadState: NewReadFileState()}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello",
		"new_string": "world",
	})
	if !tr.IsError || !strings.Contains(tr.Content, "not been read yet") {
		t.Fatalf("expected 'not been read' guard, got isErr=%v content=%q", tr.IsError, tr.Content)
	}
}

func TestEditConf_Tool_RejectsEditWithPartialView(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "f.txt")
	os.WriteFile(fp, []byte("hello"), 0o644)
	state := NewReadFileState()
	abs, _ := filepath.Abs(fp)
	info, _ := os.Stat(abs)
	state.SetForContext(context.Background(), abs, ReadFileEntry{
		TimestampMs:   info.ModTime().UnixMilli(),
		IsPartialView: true,
	})
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello",
		"new_string": "world",
	})
	data, ok := tr.Data.(types.ToolErrorData)
	if !tr.IsError || !ok || data.Code != fileErrorViewTransformed {
		t.Fatalf("expected partial-view guard, got isErr=%v content=%q", tr.IsError, tr.Content)
	}
}

func TestEditConf_Tool_StaleEditRejected(t *testing.T) {
	fp, state, abs := editFixture(t, "f.txt", "hello world")
	// Backdate the recorded read so any modtime > entry.TimestampMs.
	info, _ := os.Stat(abs)
	state.SetForContext(context.Background(), abs, ReadFileEntry{
		TimestampMs:   info.ModTime().UnixMilli() - 5000,
		IsPartialView: false,
		// Empty content snapshot disables the content fallback.
		Content: "",
	})
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello",
		"new_string": "HELLO",
	})
	if !tr.IsError || !strings.Contains(tr.Content, "modified since read") {
		t.Fatalf("expected stale-read guard, got isErr=%v content=%q", tr.IsError, tr.Content)
	}
}

func TestEditConf_Tool_StaleButContentUnchangedAccepted(t *testing.T) {
	original := "hello world"
	fp, state, _ := editFixture(t, "f.txt", original)
	// A metadata-only timestamp change is accepted because descriptor identity
	// and complete raw digest, rather than mtime, are the authorization source.
	future := time.Now().Add(10 * time.Second)
	if err := os.Chtimes(fp, future, future); err != nil {
		t.Fatal(err)
	}
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello",
		"new_string": "HELLO",
	})
	if tr.IsError {
		t.Fatalf("expected accept (content fallback), got %q", tr.Content)
	}
}

// ----- 6. tool-level: replacement / ambiguity ----------------------------

func TestEditConf_Tool_SingleReplacementWritesFile(t *testing.T) {
	original := "alpha beta gamma"
	fp, state, _ := editFixture(t, "f.txt", original)
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "beta",
		"new_string": "BETA",
	})
	if tr.IsError {
		t.Fatalf("unexpected error: %s", tr.Content)
	}
	got, _ := os.ReadFile(fp)
	if string(got) != "alpha BETA gamma" {
		t.Fatalf("file content mismatch: %q", got)
	}
}

func TestEditConf_Tool_CurlyQuoteParity(t *testing.T) {
	fp, state, _ := editFixture(t, "quotes.txt", "title = “hello”\n")
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": `title = "hello"`,
		"new_string": `title = "goodbye"`,
	})
	if tr.IsError {
		t.Fatalf("expected straight quotes to match curly quotes, got %s", tr.Content)
	}
	got, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	if string(got) != "title = “goodbye”\n" {
		t.Fatalf("expected curly quote style preserved, got %q", string(got))
	}
	result := decodeEditResult(t, tr.Content)
	if result.OldString != "title = “hello”" {
		t.Fatalf("expected result oldString to report actual file text, got %q", result.OldString)
	}
}

func TestEditConf_Tool_UTF16LERoundTrip(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "utf16.txt")
	original := "alpha\n"
	if err := os.WriteFile(fp, encodeWriteBytes(original, EncodingUTF16LE, bomUTF16LE), 0o644); err != nil {
		t.Fatalf("write utf16 fixture: %v", err)
	}
	state := NewReadFileState()
	recordStrongReadEvidenceForTest(t, state, fp)
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "alpha",
		"new_string": "beta",
	})
	if tr.IsError {
		t.Fatalf("expected UTF-16LE edit to succeed, got %s", tr.Content)
	}
	raw, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	det := detectFileEncoding(raw)
	if det.Encoding != EncodingUTF16LE {
		t.Fatalf("expected UTF-16LE encoding preserved, got %s", det.Encoding)
	}
	if got := decodeFileBytes(raw, det); got != "beta\n" {
		t.Fatalf("expected decoded content %q, got %q", "beta\n", got)
	}
}

func TestEditConf_Tool_AmbiguousReplacementWithoutReplaceAll(t *testing.T) {
	original := "x x x"
	fp, state, _ := editFixture(t, "f.txt", original)
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "x",
		"new_string": "y",
	})
	if !tr.IsError || !strings.Contains(tr.Content, "Found 3 matches") {
		t.Fatalf("expected 'Found 3 matches' error, got isErr=%v content=%q", tr.IsError, tr.Content)
	}
}

func TestEditConf_Tool_ReplaceAllOccurrences(t *testing.T) {
	original := "x.x.x"
	fp, state, _ := editFixture(t, "f.txt", original)
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":   fp,
		"old_string":  "x",
		"new_string":  "Y",
		"replace_all": true,
	})
	if tr.IsError {
		t.Fatalf("unexpected error: %s", tr.Content)
	}
	got, _ := os.ReadFile(fp)
	if string(got) != "Y.Y.Y" {
		t.Fatalf("expected Y.Y.Y, got %q", got)
	}
	r := decodeEditResult(t, tr.Content)
	if r.Occurrences != 3 || !r.ReplaceAll {
		t.Fatalf("expected occurrences=3 replaceAll=true, got %+v", r)
	}
}

func TestEditConf_Tool_MissingOldStringSurfacesTSMessage(t *testing.T) {
	original := "alpha"
	fp, state, _ := editFixture(t, "f.txt", original)
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "missing",
		"new_string": "x",
	})
	if !tr.IsError || !strings.Contains(tr.Content, "String to replace not found") {
		t.Fatalf("expected 'String to replace not found', got %q", tr.Content)
	}
}

// ----- 7. tool-level: line-ending preservation on disk -------------------

func TestEditConf_Tool_PreservesCRLFOnDisk(t *testing.T) {
	original := "line1\r\nline2\r\nline3\r\n"
	fp, state, _ := editFixture(t, "f.txt", original)
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "line2",
		"new_string": "LINE2",
	})
	if tr.IsError {
		t.Fatalf("unexpected error: %s", tr.Content)
	}
	got, _ := os.ReadFile(fp)
	if !strings.Contains(string(got), "\r\n") {
		t.Fatalf("CRLF lost: %q", got)
	}
	if !strings.Contains(string(got), "LINE2\r\n") {
		t.Fatalf("expected CRLF after LINE2, got %q", got)
	}
}

func TestEditConf_Tool_PreservesLFOnDisk(t *testing.T) {
	original := "a\nb\nc\n"
	fp, state, _ := editFixture(t, "f.txt", original)
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "b",
		"new_string": "B",
	})
	if tr.IsError {
		t.Fatalf("unexpected error: %s", tr.Content)
	}
	got, _ := os.ReadFile(fp)
	if strings.Contains(string(got), "\r\n") {
		t.Fatalf("LF file gained CRLF: %q", got)
	}
}

// ----- 8. tool-level: structured patch & result shape --------------------

func TestEditConf_Tool_StructuredPatchPresent(t *testing.T) {
	fp, state, _ := editFixture(t, "f.txt", "alpha\nbeta\ngamma\n")
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "beta",
		"new_string": "BETA",
	})
	if tr.IsError {
		t.Fatalf("unexpected error: %s", tr.Content)
	}
	r := decodeEditResult(t, tr.Content)
	if len(r.StructuredPatch) == 0 {
		t.Fatal("expected non-empty structuredPatch")
	}
	if r.OriginalFile != "alpha\nbeta\ngamma\n" {
		t.Fatalf("originalFile not echoed back: %q", r.OriginalFile)
	}
	if r.OldString != "beta" || r.NewString != "BETA" {
		t.Fatalf("oldString/newString not echoed back: %+v", r)
	}
	if r.Status != "success" {
		t.Fatalf("expected status=success, got %q", r.Status)
	}
	if r.Occurrences != 1 {
		t.Fatalf("expected occurrences=1, got %d", r.Occurrences)
	}
}

func TestEditConf_Tool_DurationMsRecorded(t *testing.T) {
	fp, state, _ := editFixture(t, "f.txt", "old")
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "old",
		"new_string": "new",
	})
	r := decodeEditResult(t, tr.Content)
	if r.DurationMs < 0 {
		t.Fatalf("durationMs should be >=0, got %d", r.DurationMs)
	}
}

// ----- 9. tool-level: file creation / nonexistent ------------------------

func TestEditConf_Tool_CreatesNewFileOnEmptyOldString(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "fresh.txt")
	tool := &FileEditTool{ReadState: NewReadFileState()}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "",
		"new_string": "brand new content\n",
	})
	if tr.IsError {
		t.Fatalf("unexpected error: %s", tr.Content)
	}
	got, _ := os.ReadFile(fp)
	if string(got) != "brand new content\n" {
		t.Fatalf("file content mismatch: %q", got)
	}
	r := decodeEditResult(t, tr.Content)
	if r.OriginalFile != "" {
		t.Fatalf("originalFile should be empty for creation, got %q", r.OriginalFile)
	}
}

func TestEditConf_Tool_RejectsEditOnMissingFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "missing.txt")
	tool := &FileEditTool{ReadState: NewReadFileState()}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "anything",
		"new_string": "else",
	})
	if !tr.IsError || !strings.Contains(tr.Content, "does not exist") {
		t.Fatalf("expected 'does not exist', got %q", tr.Content)
	}
}

func TestEditConf_Tool_RejectsEditOnDirectory(t *testing.T) {
	dir := t.TempDir()
	state := NewReadFileState()
	abs, _ := filepath.Abs(dir)
	// Pre-record a fake entry so the Read guard is past — the directory
	// rejection path still fires after.
	state.SetForContext(context.Background(), abs, ReadFileEntry{TimestampMs: time.Now().UnixMilli()})
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  dir,
		"old_string": "x",
		"new_string": "y",
	})
	if !tr.IsError {
		t.Fatal("expected error when path is directory")
	}
}

// ----- 10. tool-level: symlink rejection ---------------------------------

func TestEditConf_Tool_RejectsSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows; covered by Linux/macOS runners")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	os.WriteFile(real, []byte("hello"), 0o644)
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}
	state := NewReadFileState()
	absLink, _ := filepath.Abs(link)
	state.SetForContext(context.Background(), absLink, ReadFileEntry{
		TimestampMs: time.Now().UnixMilli(),
		Content:     "hello",
	})
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  link,
		"old_string": "hello",
		"new_string": "world",
	})
	if !tr.IsError || !strings.Contains(tr.Content, "symlink") {
		t.Fatalf("expected symlink rejection, got isErr=%v content=%q", tr.IsError, tr.Content)
	}
}

// ----- 12. tool-level: plan-mode gating -----------------------------------

func TestEditConf_Tool_PlanModeBlocksEdit(t *testing.T) {
	fp, state, _ := editFixture(t, "f.txt", "hello")
	plan := testPlanMode{active: true}
	tool := &FileEditTool{ReadState: state, PlanState: plan}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello",
		"new_string": "world",
	})
	if !tr.IsError || !strings.Contains(tr.Content, "plan mode") {
		t.Fatalf("expected plan-mode rejection, got isErr=%v content=%q", tr.IsError, tr.Content)
	}
}

// ----- 15. tool-level: refreshes ReadState on success ---------------------

func TestEditConf_Tool_RefreshesReadStateAfterWrite(t *testing.T) {
	fp, state, abs := editFixture(t, "f.txt", "hello")
	beforeEntry, _ := state.GetForContext(context.Background(), abs)
	beforeTs := beforeEntry.TimestampMs
	// Sleep just enough so any second-level mtime change is observable on
	// FAT-style filesystems (1s); millisecond-precision systems (NTFS, ext4)
	// will see a delta within a single tick.
	time.Sleep(10 * time.Millisecond)
	tool := &FileEditTool{ReadState: state}
	tr, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello",
		"new_string": "world",
	})
	if tr.IsError {
		t.Fatalf("unexpected error: %s", tr.Content)
	}
	afterEntry, ok := state.GetForContext(context.Background(), abs)
	if !ok {
		t.Fatal("ReadState entry missing after edit")
	}
	if afterEntry.TimestampMs < beforeTs {
		t.Fatalf("ReadState mtime regressed: before=%d after=%d", beforeTs, afterEntry.TimestampMs)
	}
	if afterEntry.Content == "" {
		t.Fatal("ReadState content not refreshed after edit")
	}
	if afterEntry.IsPartialView {
		t.Fatal("Edit should leave IsPartialView=false on the refreshed entry")
	}
}

// ----- 16. tool-level: schema & metadata ---------------------------------

func TestEditConf_Tool_Name(t *testing.T) {
	tool := &FileEditTool{}
	if tool.Name() != "Edit" {
		t.Fatalf("expected Name=Edit, got %q", tool.Name())
	}
}

func TestEditConf_Tool_SchemaShape(t *testing.T) {
	tool := &FileEditTool{}
	schema := tool.Schema()
	if schema.Type != "object" {
		t.Fatalf("expected object schema, got %q", schema.Type)
	}
	required := schema.Required
	wanted := map[string]bool{"file_path": false, "old_string": false, "new_string": false}
	for _, r := range required {
		if _, ok := wanted[r]; ok {
			wanted[r] = true
		}
	}
	for k, ok := range wanted {
		if !ok {
			t.Fatalf("schema.Required missing %q (got %v)", k, required)
		}
	}
	if _, ok := schema.Properties["replace_all"]; !ok {
		t.Fatal("schema missing replace_all property")
	}
}

// ----- 17. result string rendering ---------------------------------------

func TestEditConf_Tool_StringSummarySingle(t *testing.T) {
	r := EditResult{FilePath: "/tmp/x.txt", Occurrences: 1}
	if !strings.Contains(r.String(), "/tmp/x.txt") {
		t.Fatalf("summary missing path: %q", r.String())
	}
	if strings.Contains(r.String(), "occurrences") {
		t.Fatalf("single edit summary should not mention occurrences: %q", r.String())
	}
}

func TestEditConf_Tool_StringSummaryReplaceAll(t *testing.T) {
	r := EditResult{FilePath: "/tmp/x.txt", Occurrences: 7, ReplaceAll: true}
	got := r.String()
	if !strings.Contains(got, "All 7 occurrences") {
		t.Fatalf("replace-all summary missing count: %q", got)
	}
}

// ----- 18. JSON marshalling parity ---------------------------------------

func TestEditConf_EditResult_JSONFields(t *testing.T) {
	r := EditResult{
		FilePath:        "/tmp/a.txt",
		OldString:       "x",
		NewString:       "y",
		OriginalFile:    "x",
		StructuredPatch: []DiffHunk{},
		ReplaceAll:      false,
		Occurrences:     1,
		Status:          "success",
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{
		`"filePath"`, `"oldString"`, `"newString"`, `"originalFile"`,
		`"structuredPatch"`, `"replaceAll"`, `"status"`,
	} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("expected field %s in JSON, got %s", field, data)
		}
	}
	for _, removed := range []string{`"userModified"`, `"warning"`} {
		if strings.Contains(string(data), removed) {
			t.Fatalf("removed compatibility field %s remains in JSON: %s", removed, data)
		}
	}
}
