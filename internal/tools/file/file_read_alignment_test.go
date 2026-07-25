// Package file contains TS-parity conformance tests for FileReadTool.
package file

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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

// runAlignmentRead is a helper that executes Read and returns the
// raw ToolResult; failures are deferred to the caller's t.Fatal/Errorf
// so each contract test can report the relevant result details.
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
		t.Fatalf("expected variant=\"text\", got %q (Content=%q)", gotVariant, res.Content)
	}
	if res.Metadata["type"] != "" || res.Metadata["variant"] != "" {
		t.Fatalf("typed Read discriminator must not be duplicated in metadata: %#v", res.Metadata)
	}
}

func TestAlignment_FileRead_ResultMappingRejectsPointerCompatibilityPayload(t *testing.T) {
	tool := &FileReadTool{}
	output := FileReadOutput{Type: FileReadVariantText, File: FileReadOutputFile{Content: "alpha", StartLine: 1}}
	block := tool.MapToolResultToToolResultBlock(&output, "toolu_invalid")
	if !block.IsError || block.ToolUseID != "toolu_invalid" {
		t.Fatalf("pointer compatibility payload was accepted: %+v", block)
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

// ─── Surface: schema uses semanticNumber wrapper ────────────────────────────

// TestAlignment_FileRead_SchemaUsesSemanticNumberOffset asserts that
// the JSON schema for `offset`/`limit` is wrapped via the package's
// semanticNumber helper, carrying integer and nonnegative constraints.
func TestAlignment_FileRead_SchemaUsesSemanticNumberOffset(t *testing.T) {
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

// alignmentVariantFromResult reads the typed result discriminator.
func alignmentVariantFromResult(res types.ToolResult) string {
	if output, ok := asFileReadOutput(res.Data); ok {
		return string(output.Type)
	}
	return ""
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
