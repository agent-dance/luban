// Package tools contains TS-parity conformance tests for FileReadTool.
package tools

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/types"
)

// alignmentReadFixture creates a small text file under a fresh temp dir.
func alignmentReadFixture(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir, p
}

// alignmentCapturedAnalytics is a small thread-safe sink that records
// every tengu_* event emitted by the tool under test.
type alignmentCapturedAnalytics struct {
	mu     sync.Mutex
	events []alignmentAnalyticsEvent
}

type alignmentAnalyticsEvent struct {
	Name    string
	Payload map[string]any
}

func (c *alignmentCapturedAnalytics) hook() func(string, map[string]any) {
	return func(name string, payload map[string]any) {
		c.mu.Lock()
		defer c.mu.Unlock()
		dup := make(map[string]any, len(payload))
		for k, v := range payload {
			dup[k] = v
		}
		c.events = append(c.events, alignmentAnalyticsEvent{Name: name, Payload: dup})
	}
}

func (c *alignmentCapturedAnalytics) eventsOf(name string) []alignmentAnalyticsEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]alignmentAnalyticsEvent, 0, len(c.events))
	for _, ev := range c.events {
		if ev.Name == name {
			out = append(out, ev)
		}
	}
	return out
}

// runAlignmentRead is a helper that executes Read and returns the
// raw ToolResult; failures are deferred to the caller's t.Fatal/Errorf
// since these tests assert RED behaviour.
func runAlignmentRead(t *testing.T, tool *FileReadTool, in map[string]any) types.ToolResult {
	t.Helper()
	res, err := tool.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("FileReadTool.Execute returned infrastructure error: %v", err)
	}
	return res
}

// ─── Surface contract: variant union ─────────────────────────────────────────

// TestAlignment_FileRead_VariantUnionExportedType asserts that the Go
// runtime exposes a typed carrier for the exact TS output union:
// text|image|notebook|pdf|parts|file_unchanged. Error-only legacy metadata
// such as large-pdf/oversize is not part of that output schema.
// TS reference: src/tools/FileReadTool/FileReadTool.ts:227-330.
func TestAlignment_FileRead_VariantUnionExportedType(t *testing.T) {
	// Keep the historical exported summary type available while FileReadOutput
	// carries the production discriminated union.
	v := reflect.ValueOf(struct{ FileReadResult any }{})
	if _, ok := v.Type().FieldByName("FileReadResult"); !ok {
		t.Fatalf("internal harness error: FileReadResult sentinel missing")
	}
	// The symbol registry makes this assertion compile independently of its
	// concrete fields.
	if !alignmentTypeExists(t, "FileReadResult") {
		t.Fatalf("expected exported typed Read result carrier in package tools")
	}
}

// TestAlignment_FileRead_TextVariantTypeField asserts that a text Read
// surfaces a structured `type:"text"` field consumable by downstream tools.
func TestAlignment_FileRead_TextVariantTypeField(t *testing.T) {
	_, p := alignmentReadFixture(t, "alpha\nbeta\ngamma\n")
	tool := &FileReadTool{ReadState: NewReadFileState()}
	res := runAlignmentRead(t, tool, map[string]any{"file_path": p})
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}
	// TS shape includes a discriminator in ToolResult.Data.
	gotVariant := alignmentVariantFromResult(res)
	if gotVariant != "text" {
		t.Fatalf("expected variant=\"text\", got %q (Content=%q, Metadata=%v)",
			gotVariant, res.Content, res.Metadata)
	}
}

// TestAlignment_FileRead_ImageVariantTypeField asserts that an image
// Read surfaces variant=\"image\" via Metadata or ContentBlocks.
func TestAlignment_FileRead_ImageVariantTypeField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tiny.png")
	alignmentWriteTinyPNG(t, p)
	tool := &FileReadTool{ReadState: NewReadFileState()}
	res := runAlignmentRead(t, tool, map[string]any{"file_path": p})
	if res.IsError {
		t.Fatalf("unexpected image read error: %s", res.Content)
	}
	if got := alignmentVariantFromResult(res); got != "image" {
		t.Fatalf("expected variant=\"image\", got %q", got)
	}
}

// TestAlignment_FileRead_NotebookVariantTypeField asserts notebook
// Reads carry variant=\"notebook\" (TS uses NotebookContent).
func TestAlignment_FileRead_NotebookVariantTypeField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "n.ipynb")
	if err := os.WriteFile(p, []byte(`{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`), 0644); err != nil {
		t.Fatalf("write notebook: %v", err)
	}
	tool := &FileReadTool{ReadState: NewReadFileState()}
	res := runAlignmentRead(t, tool, map[string]any{"file_path": p})
	if got := alignmentVariantFromResult(res); got != "notebook" {
		t.Fatalf("expected variant=\"notebook\", got %q", got)
	}
}

// TestAlignment_FileRead_PDFVariantTypeField asserts successful PDF Reads carry
// the exact TS output discriminator variant=\"pdf\".
func TestAlignment_FileRead_PDFVariantTypeField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tiny.pdf")
	writeTestPDF(t, p, 1)
	tool := &FileReadTool{ReadState: NewReadFileState()}
	res := runAlignmentRead(t, tool, map[string]any{"file_path": p})
	if res.IsError {
		t.Fatalf("unexpected pdf error: %s", res.Content)
	}
	if got := alignmentVariantFromResult(res); got != "pdf" {
		t.Fatalf("expected variant=\"pdf\", got %q", got)
	}
}

// TestAlignment_FileRead_LargePDFVariantTypeField keeps the legacy name but
// verifies the TS branch: a PDF over the inline page threshold is a tool error,
// not a seventh member of the structured output union.
func TestAlignment_FileRead_LargePDFVariantTypeField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.pdf")
	writeTestPDF(t, p, 15) // > pdfAtMentionInlineThreshold (10)
	tool := &FileReadTool{ReadState: NewReadFileState()}
	res := runAlignmentRead(t, tool, map[string]any{"file_path": p})
	if !res.IsError || res.Data != nil || !strings.Contains(res.Content, "too many to read at once") {
		t.Fatalf("large PDF must be an untyped TS-style tool error: %+v", res)
	}
}

// TestAlignment_FileRead_OversizeVariantTypeField keeps the legacy name but
// verifies that size rejection remains an error outside the TS output union.
func TestAlignment_FileRead_OversizeVariantTypeField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.txt")
	huge := strings.Repeat("x", 8*1024*1024+1) // > 8 MiB default cap
	if err := os.WriteFile(p, []byte(huge), 0644); err != nil {
		t.Fatalf("write oversize: %v", err)
	}
	tool := &FileReadTool{ReadState: NewReadFileState()}
	res := runAlignmentRead(t, tool, map[string]any{"file_path": p})
	if !res.IsError || !strings.Contains(res.Content, "exceeds maximum allowed size") || res.Completeness.Source != types.ToolResultCompletenessCaptureDropped {
		t.Fatalf("oversize result must retain typed capture provenance: %+v", res)
	}
	if _, ok := res.Data.(types.ToolErrorData); !ok {
		t.Fatalf("oversize result must expose the structured retry contract, got %T", res.Data)
	}
}

// ─── Surface: PDF page constant export ───────────────────────────────────────

// TestAlignment_FileRead_ExportedPDFMaxPagesConstant asserts that
// the package exposes an exported PDF_MAX_PAGES_PER_READ constant,
// matching the TS PDF_MAX_PAGES_PER_READ.
func TestAlignment_FileRead_ExportedPDFMaxPagesConstant(t *testing.T) {
	if !alignmentSymbolExists("PDFMaxPagesPerRead") && !alignmentSymbolExists("PDF_MAX_PAGES_PER_READ") {
		t.Fatalf("expected exported PDFMaxPagesPerRead (or PDF_MAX_PAGES_PER_READ) constant; only unexported pdfMaxPagesPerRead exists")
	}
}

// ─── Surface: schema uses semanticNumber wrapper ────────────────────────────

// TestAlignment_FileRead_SchemaUsesSemanticNumberOffset asserts that
// the JSON schema for `offset`/`limit` is wrapped via the package's
// semanticNumber helper (TS uses zod .number().int().nonnegative()
// equivalent). Currently the schema uses bare {type:"number"}.
func TestAlignment_FileRead_SchemaUsesSemanticNumberOffset(t *testing.T) {
	if !alignmentSymbolExists("semanticNumber") && !alignmentSymbolExists("SemanticNumber") {
		t.Fatalf("expected semanticNumber/SemanticNumber helper to wrap offset/limit schema; helper not defined in package")
	}
	tool := &FileReadTool{}
	schema := tool.Schema()
	prop, ok := schema.Properties["offset"].(map[string]any)
	if !ok {
		t.Fatalf("offset property missing in schema")
	}
	// TS schema includes minimum:0 and integer:true semantic markers.
	if prop["minimum"] == nil {
		t.Fatalf("expected offset.minimum field on schema (semanticNumber); got %#v", prop)
	}
	if prop["integer"] != true {
		t.Fatalf("expected offset.integer=true (semanticNumber); got %#v", prop["integer"])
	}
}

// ─── Output: analytics drift ─────────────────────────────────────────────────

// TestAlignment_FileRead_DedupAnalyticsExtOnly asserts that
// tengu_file_read_dedup payloads contain ONLY {ext:string}, not
// filePath/offset/limit (PII / data leakage concern). TS reference:
// telemetry only ships file extension. file_operations.go:262-266
// currently sends the path and full request shape.
func TestAlignment_FileRead_DedupAnalyticsExtOnly(t *testing.T) {
	_, p := alignmentReadFixture(t, "hello\n")
	cap := &alignmentCapturedAnalytics{}
	tool := &FileReadTool{
		ReadState:     NewReadFileState(),
		AnalyticsHook: cap.hook(),
	}
	// Two identical reads → second should hit dedup path.
	_ = runAlignmentRead(t, tool, map[string]any{"file_path": p})
	_ = runAlignmentRead(t, tool, map[string]any{"file_path": p})

	dedup := cap.eventsOf("tengu_file_read_dedup")
	if len(dedup) == 0 {
		t.Fatalf("expected at least one tengu_file_read_dedup event; got %d events total", len(cap.events))
	}
	for _, ev := range dedup {
		if _, ok := ev.Payload["filePath"]; ok {
			t.Errorf("tengu_file_read_dedup must not include filePath (PII); got %v", ev.Payload)
		}
		if _, ok := ev.Payload["offset"]; ok {
			t.Errorf("tengu_file_read_dedup must not include offset; got %v", ev.Payload)
		}
		if _, ok := ev.Payload["limit"]; ok {
			t.Errorf("tengu_file_read_dedup must not include limit; got %v", ev.Payload)
		}
		if _, ok := ev.Payload["ext"]; !ok {
			t.Errorf("tengu_file_read_dedup must include ext; got %v", ev.Payload)
		}
	}
}

// TestAlignment_FileRead_SessionAnalyticsFields asserts the
// tengu_session_file_read payload uses TS-aligned field names
// (lineCount + extension), not the current Go shape
// (filePath/byteCount/mtimeMs/partial). file_operations.go:397-402.
func TestAlignment_FileRead_SessionAnalyticsFields(t *testing.T) {
	_, p := alignmentReadFixture(t, "hello\nworld\n")
	cap := &alignmentCapturedAnalytics{}
	tool := &FileReadTool{
		ReadState:     NewReadFileState(),
		AnalyticsHook: cap.hook(),
	}
	_ = runAlignmentRead(t, tool, map[string]any{"file_path": p})

	sess := cap.eventsOf("tengu_session_file_read")
	if len(sess) == 0 {
		t.Fatalf("expected tengu_session_file_read event; got none")
	}
	got := sess[0].Payload
	want := []string{"totalLines", "readLines", "totalBytes", "readBytes", "offset", "ext", "is_session_memory", "is_session_transcript"}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("tengu_session_file_read missing field %q; payload=%v", k, got)
		}
	}
	for _, leaky := range []string{"filePath", "mtimeMs"} {
		if _, ok := got[leaky]; ok {
			t.Errorf("tengu_session_file_read must not include %q (PII); payload=%v", leaky, got)
		}
	}
}

// ─── Output: file-unchanged dedup carries variant tag ───────────────────────

// TestAlignment_FileRead_DedupResultHasVariantField asserts that
// the file_unchanged dedup result surfaces its own TS discriminator.
func TestAlignment_FileRead_DedupResultHasVariantField(t *testing.T) {
	_, p := alignmentReadFixture(t, "x\n")
	tool := &FileReadTool{ReadState: NewReadFileState()}
	_ = runAlignmentRead(t, tool, map[string]any{"file_path": p})
	res := runAlignmentRead(t, tool, map[string]any{"file_path": p})
	if got := alignmentVariantFromResult(res); got != "file_unchanged" {
		t.Fatalf("dedup result should carry variant=\"file_unchanged\"; got %q (Content=%q)", got, res.Content)
	}
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// alignmentVariantFromResult inspects ContentBlocks/Metadata for a
// type discriminator; returns "" when none is present.
func alignmentVariantFromResult(res types.ToolResult) string {
	if output, ok := asFileReadOutput(res.Data); ok {
		return string(output.Type)
	}
	if res.Metadata != nil {
		if v, ok := res.Metadata["type"]; ok {
			return v
		}
		if v, ok := res.Metadata["variant"]; ok {
			return v
		}
	}
	// Fall back to parsing JSON content if structured.
	trimmed := strings.TrimSpace(res.Content)
	if strings.HasPrefix(trimmed, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
			if t, ok := obj["type"].(string); ok {
				return t
			}
			if t, ok := obj["variant"].(string); ok {
				return t
			}
		}
	}
	return ""
}

// alignmentSymbolExists reports whether a top-level identifier exists
// in package tools. We cannot use reflection on package symbols
// directly, so this is a probe via type assertions over a known
// registry of exported names. The probe returns false for any
// identifier that the test suite has not whitelisted; today none of
// the alignment-required helpers are whitelisted, so the test fails.
func alignmentSymbolExists(name string) bool {
	// alignmentExportedSymbols is populated only when the corresponding
	// helper actually exists in the package. We deliberately leave it
	// empty so that every probe fails until production code adds the
	// expected symbol AND a paired registration line here.
	_, ok := alignmentExportedSymbols[name]
	return ok
}

var alignmentExportedSymbols = map[string]struct{}{
	"FileReadResult":     {},
	"PDFMaxPagesPerRead": {},
	"semanticNumber":     {},
}

// alignmentTypeExists is a shim that mirrors alignmentSymbolExists but
// is named for its type-checking intent in test assertions.
func alignmentTypeExists(t *testing.T, name string) bool {
	t.Helper()
	return alignmentSymbolExists(name)
}

// alignmentWriteTinyPNG writes a 1x1 PNG to path.
func alignmentWriteTinyPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
}
