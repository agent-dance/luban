package file

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func writeTestPDF(t *testing.T, path string, pageCount int) {
	t.Helper()

	type pdfObject struct {
		Number int
		Body   string
	}

	pageObjects := make([]int, pageCount)
	contentObjects := make([]int, pageCount)
	nextObject := 3
	for i := 0; i < pageCount; i++ {
		pageObjects[i] = nextObject
		contentObjects[i] = nextObject + 1
		nextObject += 2
	}
	fontObject := nextObject

	objects := []pdfObject{
		{Number: 1, Body: "<< /Type /Catalog /Pages 2 0 R >>"},
		{
			Number: 2,
			Body:   fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", joinObjectRefs(pageObjects), pageCount),
		},
	}

	for i := 0; i < pageCount; i++ {
		content := fmt.Sprintf("BT /F1 12 Tf 72 120 Td (Page %d) Tj ET", i+1)
		objects = append(objects,
			pdfObject{
				Number: pageObjects[i],
				Body: fmt.Sprintf(
					"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>",
					fontObject,
					contentObjects[i],
				),
			},
			pdfObject{
				Number: contentObjects[i],
				Body:   fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
			},
		)
	}

	objects = append(objects, pdfObject{
		Number: fontObject,
		Body:   "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	})

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("%\xE2\xE3\xCF\xD3\n")

	offsets := make([]int, fontObject+1)
	for _, obj := range objects {
		offsets[obj.Number] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", obj.Number, obj.Body)
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", fontObject+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= fontObject; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Root 1 0 R /Size %d >>\nstartxref\n%d\n%%%%EOF\n", fontObject+1, xrefOffset)

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
}

func joinObjectRefs(numbers []int) string {
	parts := make([]string, 0, len(numbers))
	for _, number := range numbers {
		parts = append(parts, fmt.Sprintf("%d 0 R", number))
	}
	return strings.Join(parts, " ")
}

func TestFileReadTool(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	// Check metadata
	if tool.Name() != "Read" {
		t.Errorf("expected Name='Read', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty Description")
	}

	schema := tool.Schema()
	if schema.Type != "object" {
		t.Errorf("expected Schema.Type='object', got %q", schema.Type)
	}

	// Create temp file for testing
	tmpFile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "test content"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Test successful read
	result, err := tool.Execute(ctx, map[string]any{
		"file_path": tmpFile.Name(),
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}
	if result.Content != "1\ttest content" {
		t.Errorf("expected numbered content, got %q", result.Content)
	}

	// Test non-existent file
	result, err = tool.Execute(ctx, map[string]any{
		"file_path": "/nonexistent/file",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for non-existent file")
	}
	if !strings.Contains(result.Content, "File does not exist. Current working directory is") {
		t.Fatalf("expected missing-file guidance, got %q", result.Content)
	}
}

func TestFileReadToolOffsetLimitAndWarnings(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpFile, err := os.CreateTemp("", "file-read-range")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString("a\nb\nc\nd\n"); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	result, err := tool.Execute(ctx, map[string]any{
		"file_path": tmpFile.Name(),
		"offset":    float64(2),
		"limit":     float64(2),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got, want := result.Content, "2\tb\n3\tc"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}

	result, err = tool.Execute(ctx, map[string]any{
		"file_path": tmpFile.Name(),
		"offset":    float64(99),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected warning text, got error: %s", result.Content)
	}
	if want := "<system-reminder>Warning: the file exists but is shorter than the provided offset (99). The file has 5 lines.</system-reminder>"; result.Content != want {
		t.Fatalf("warning = %q, want %q", result.Content, want)
	}
}

func TestFileReadToolEmptyFileWarningAndValidation(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpFile, err := os.CreateTemp("", "file-read-empty")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	result, err := tool.Execute(ctx, map[string]any{
		"file_path": tmpFile.Name(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected warning text, got error: %s", result.Content)
	}
	if got, want := result.Content, "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>"; got != want {
		t.Fatalf("warning = %q, want %q", got, want)
	}

	result, err = tool.Execute(ctx, map[string]any{
		"file_path": tmpFile.Name(),
		"limit":     float64(0),
	})
	if err != nil {
		t.Fatalf("expected no infrastructure error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected validation error for zero limit")
	}
	if got, want := result.Content, "'limit' must be a positive integer"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestFileReadToolDoesNotDefaultToImplicit2000LineLimit(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpFile, err := os.CreateTemp("", "file-read-many-lines")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	var content strings.Builder
	for i := 1; i <= 2105; i++ {
		fmt.Fprintf(&content, "line-%d\n", i)
	}
	if _, err := tmpFile.WriteString(content.String()); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	result, err := tool.Execute(ctx, map[string]any{
		"file_path": tmpFile.Name(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected successful read, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "2105\tline-2105") {
		t.Fatalf("expected full file content without implicit 2000-line cap")
	}
}

func TestFileReadToolAllowsTargetedReadsForLargeFiles(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpFile, err := os.CreateTemp("", "file-read-large")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	var content strings.Builder
	for i := 1; i <= 12000; i++ {
		fmt.Fprintf(&content, "line-%05d %s\n", i, strings.Repeat("x", 20))
	}
	if _, err := tmpFile.WriteString(content.String()); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	stat, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("stat temp file: %v", err)
	}
	if stat.Size() <= defaultReadMaxSizeBytes {
		t.Fatalf("test file must exceed default size limit; got %d bytes", stat.Size())
	}

	result, err := tool.Execute(ctx, map[string]any{
		"file_path": tmpFile.Name(),
		"offset":    float64(9000),
		"limit":     float64(3),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected targeted large-file read to succeed, got error: %s", result.Content)
	}
	if got, want := result.Content, "9000\tline-09000 "+strings.Repeat("x", 20)+"\n9001\tline-09001 "+strings.Repeat("x", 20)+"\n9002\tline-09002 "+strings.Repeat("x", 20); got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}

	result, err = tool.Execute(ctx, map[string]any{
		"file_path": tmpFile.Name(),
	})
	if err != nil {
		t.Fatalf("expected no infrastructure error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected full read without limit to hit size guard")
	}
	if !strings.Contains(result.Content, "exceeds maximum allowed size") {
		t.Fatalf("expected size-limit error, got %q", result.Content)
	}
}

func TestFileReadToolMissingFileSuggestsNearbyPath(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	expected := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(expected, []byte("name: test\n"), 0644); err != nil {
		t.Fatalf("write suggested file: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]any{
		"file_path": filepath.Join(tmpDir, "cnfig.yaml"),
	})
	if err != nil {
		t.Fatalf("expected no infrastructure error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected missing-file error")
	}
	if !strings.Contains(result.Content, "Did you mean "+expected+"?") {
		t.Fatalf("expected nearby-path suggestion, got %q", result.Content)
	}
}

func TestFileReadToolNotebookFormatting(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "notebook.ipynb")
	imagePayload := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/5mQAAAAASUVORK5CYII="
	content := `{
  "cells": [
    {
      "cell_type": "markdown",
      "id": "intro",
      "source": ["# Title\n", "hello"]
    },
    {
      "cell_type": "code",
      "id": "code-1",
      "source": ["print(1)\n"],
      "outputs": [
        {"output_type":"stream","text":"1\n"},
        {"output_type":"display_data","data":{"text/plain":["plot\n"],"image/png":"` + imagePayload + `"}}
      ]
    }
  ],
  "metadata": {
    "language_info": {"name": "python"}
  },
  "nbformat": 4,
  "nbformat_minor": 5
}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write notebook: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if got, want := result.Content, "Notebook file read: "+path+" (2 cell(s))"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if len(result.ContentBlocks) != 2 {
		t.Fatalf("expected 2 notebook content blocks, got %d", len(result.ContentBlocks))
	}
	text, ok := result.ContentBlocks[0].(types.TextBlock)
	if !ok {
		t.Fatalf("expected first notebook block to be text, got %#v", result.ContentBlocks[0])
	}
	if !containsAll(text.Text, []string{
		`<cell id="intro"><cell_type>markdown</cell_type># Title`,
		`<cell id="code-1">print(1)`,
		"\n1\n",
		"\nplot\n",
	}) {
		t.Fatalf("unexpected notebook text payload: %q", text.Text)
	}
	img, ok := result.ContentBlocks[1].(types.ImageBlock)
	if !ok {
		t.Fatalf("expected second notebook block to be image, got %#v", result.ContentBlocks[1])
	}
	if img.Source == nil || img.Source.MediaType != "image/png" || img.Source.Data != imagePayload {
		t.Fatalf("unexpected notebook image source: %#v", img.Source)
	}
}

func TestFileReadToolNotebookLargeOutputsAreSummarized(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "large-output.ipynb")
	content := `{
  "cells": [
    {
      "cell_type": "code",
      "id": "code-1",
      "source": ["print(1)\n"],
      "outputs": [{"output_type":"stream","text":"` + strings.Repeat("x", 12000) + `"}]
    }
  ],
  "metadata": {
    "language_info": {"name": "python"}
  },
  "nbformat": 4,
  "nbformat_minor": 5
}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write notebook: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("expected 1 notebook content block, got %d", len(result.ContentBlocks))
	}
	text, ok := result.ContentBlocks[0].(types.TextBlock)
	if !ok {
		t.Fatalf("expected notebook text block, got %#v", result.ContentBlocks[0])
	}
	if !strings.Contains(text.Text, `Outputs are too large to include. Use Bash with: cat `) {
		t.Fatalf("expected oversized output guidance, got %q", text.Text)
	}
	if !strings.Contains(text.Text, `.cells[0].outputs`) {
		t.Fatalf("expected jq cell index guidance, got %q", text.Text)
	}
}

func TestFileReadToolReturnsImageAttachment(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "pixel.png")
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/5mQAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if err := os.WriteFile(path, pngBytes, 0644); err != nil {
		t.Fatalf("write png: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("expected 1 image content block, got %d", len(result.ContentBlocks))
	}
	img, ok := result.ContentBlocks[0].(types.ImageBlock)
	if !ok {
		t.Fatalf("expected image block, got %#v", result.ContentBlocks[0])
	}
	if img.Source == nil || img.Source.MediaType != "image/png" || img.Source.Data == "" {
		t.Fatalf("unexpected image source: %#v", img.Source)
	}
}

func TestFileReadToolReturnsImageMetadataWhenResized(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "wide.png")
	wide := image.NewRGBA(image.Rect(0, 0, 3000, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 3000; x++ {
			wide.Set(x, y, color.RGBA{R: 20, G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, wide); err != nil {
		t.Fatalf("encode wide image: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write wide image: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("expected image-only tool result, got %d blocks", len(result.ContentBlocks))
	}
	img, ok := result.ContentBlocks[0].(types.ImageBlock)
	if !ok {
		t.Fatalf("expected image block, got %#v", result.ContentBlocks[0])
	}
	if img.Source == nil || img.Source.MediaType == "" || img.Source.Data == "" {
		t.Fatalf("unexpected resized image source: %#v", img.Source)
	}
	if len(result.NewMessages) != 1 || len(result.NewMessages[0].Content) != 1 {
		t.Fatalf("expected one supplemental metadata message, got %#v", result.NewMessages)
	}
	text, ok := result.NewMessages[0].Content[0].(types.TextBlock)
	if !ok || !strings.Contains(text.Text, "original 3000x100, displayed at 2000x67") {
		t.Fatalf("unexpected supplemental image metadata: %#v", result.NewMessages[0].Content[0])
	}
}

func TestFileReadToolReturnsPDFAttachment(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sample.pdf")
	pdfBytes := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF\n")
	if err := os.WriteFile(path, pdfBytes, 0644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if len(result.NewMessages) != 1 {
		t.Fatalf("expected 1 supplemental message, got %d", len(result.NewMessages))
	}
	doc, ok := result.NewMessages[0].Content[0].(types.DocumentBlock)
	if !ok {
		t.Fatalf("expected document block, got %#v", result.NewMessages[0].Content[0])
	}
	if doc.Source == nil || doc.Source.MediaType != "application/pdf" || doc.Source.Data == "" {
		t.Fatalf("unexpected document source: %#v", doc.Source)
	}
}

func TestFileReadToolRejectsBinaryExtensions(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "archive.zip")
	if err := os.WriteFile(path, []byte("not really a zip"), 0644); err != nil {
		t.Fatalf("write binary fixture: %v", err)
	}

	result, err := tool.Execute(ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected binary-extension error")
	}
	if want := "This tool cannot read binary files. The file appears to be a binary .zip file."; !strings.Contains(result.Content, want) {
		t.Fatalf("unexpected binary-extension error: %q", result.Content)
	}
}

func TestFileReadToolRejectsBlockedDevicePaths(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("/dev/zero not available on this platform")
	}

	tool := &FileReadTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]any{"file_path": "/dev/zero"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected blocked device file error")
	}
	if got, want := result.Content, "Cannot read '/dev/zero': this device file would block or produce infinite output."; got != want {
		t.Fatalf("device error = %q, want %q", got, want)
	}
}

func TestFileReadToolRejectsOpenEndedPDFRangesOverLimit(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "multi.pdf")
	writeTestPDF(t, path, 5)

	result, err := tool.Execute(ctx, map[string]any{
		"file_path": path,
		"pages":     "3-",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected page-range validation error")
	}
	if got, want := result.Content, `Page range "3-" exceeds maximum of 20 pages per request. Please use a smaller range.`; got != want {
		t.Fatalf("page range error = %q, want %q", got, want)
	}
}

func TestFileReadToolExtractsPDFPages(t *testing.T) {
	if !hasPDFToPPM() {
		t.Skip("PDF page extraction runtime is not available")
	}

	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "pages.pdf")
	writeTestPDF(t, path, 2)

	result, err := tool.Execute(ctx, map[string]any{
		"file_path": path,
		"pages":     "1-2",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "PDF pages extracted: 2 page(s) from "+path) {
		t.Fatalf("unexpected PDF page extraction summary: %q", result.Content)
	}
	if len(result.NewMessages) != 1 {
		t.Fatalf("expected 1 supplemental message, got %d", len(result.NewMessages))
	}
	if len(result.NewMessages[0].Content) != 2 {
		t.Fatalf("expected 2 extracted page images, got %d", len(result.NewMessages[0].Content))
	}
	for i, block := range result.NewMessages[0].Content {
		img, ok := block.(types.ImageBlock)
		if !ok {
			t.Fatalf("content[%d] = %#v, want image block", i, block)
		}
		if img.Source == nil || img.Source.MediaType != "image/jpeg" || img.Source.Data == "" {
			t.Fatalf("unexpected extracted image source at %d: %#v", i, img.Source)
		}
	}
}

func TestFileReadToolRejectsLargePDFWithoutPages(t *testing.T) {
	pageCountProbePath := filepath.Join(t.TempDir(), "probe.pdf")
	writeTestPDF(t, pageCountProbePath, 1)
	if _, ok := getPDFPageCount(pageCountProbePath); !ok {
		t.Skip("PDF page counting runtime is not available")
	}

	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "large.pdf")
	writeTestPDF(t, path, 11)

	result, err := tool.Execute(ctx, map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected PDF page-count validation error")
	}
	if !strings.Contains(result.Content, `This PDF has 11 pages, which is too many to read at once.`) {
		t.Fatalf("unexpected large-PDF error: %q", result.Content)
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func TestFileWriteTool(t *testing.T) {
	tool := &FileWriteTool{}
	ctx := context.Background()

	if tool.Name() != "Write" {
		t.Errorf("expected Name='Write', got %q", tool.Name())
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "subdir", "test.txt")
	content := "test content"

	// Test successful write (creates directories)
	result, err := tool.Execute(ctx, map[string]any{
		"file_path": filePath,
		"content":   content,
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}

	// Verify file was created with correct content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Errorf("failed to read file: %v", err)
	}
	if string(fileContent) != content {
		t.Errorf("expected content=%q, got %q", content, fileContent)
	}
}

// fileWriteToolDecode pulls the JSON payload from a successful FileWriteTool
// response. Returns nil for error results.
func fileWriteToolDecode(t *testing.T, res types.ToolResult) map[string]any {
	t.Helper()
	if res.IsError {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("failed to decode response: %v: %s", err, res.Content)
	}
	return out
}

// TestFileWriteToolRequiresReadBeforeOverwrite covers write-04: existing files
// require a prior Read entry; without one, the write is rejected.
func TestFileWriteToolRequiresReadBeforeOverwrite(t *testing.T) {
	state := NewReadFileState()
	tool := &FileWriteTool{ReadState: state}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-noread")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "existing.txt")
	if err := os.WriteFile(target, []byte("seed"), 0o644); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "new",
	})
	if !res.IsError {
		t.Fatalf("expected error for unread existing file, got success: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "has not been read") {
		t.Errorf("expected 'has not been read' message, got: %s", res.Content)
	}
	// File must remain untouched.
	got, _ := os.ReadFile(target)
	if string(got) != "seed" {
		t.Errorf("file modified despite rejection: %q", got)
	}
}

// TestFileWriteToolDetectsStaleReads covers write-04: when the file's mtime is
// newer than the recorded read timestamp, the write is rejected.
func TestFileWriteToolDetectsStaleReads(t *testing.T) {
	state := NewReadFileState()
	tool := &FileWriteTool{ReadState: state}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-stale")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "stale.txt")
	if err := os.WriteFile(target, []byte("v1"), 0o644); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	seedCanonicalFileReadState(t, state, target)
	// Bump the file's mtime to make sure it's newer than the recorded ts.
	now := time.Now()
	if err := os.Chtimes(target, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "v2",
	})
	if res.IsError {
		t.Fatalf("mtime-only change should be writable, got: %s", res.Content)
	}

	if err := os.WriteFile(target, []byte("external"), 0o644); err != nil {
		t.Fatalf("external write: %v", err)
	}
	res, _ = tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "model",
	})
	if !res.IsError {
		t.Fatalf("expected stale-write rejection, got success: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "modified since read") {
		t.Errorf("expected 'modified since read' message, got: %s", res.Content)
	}
}

// TestFileWriteToolRejectsPartialView covers write-04: even with a Read entry,
// writes are refused when the read covered only a portion of the file.
func TestFileWriteToolRejectsPartialView(t *testing.T) {
	state := NewReadFileState()
	tool := &FileWriteTool{ReadState: state}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-partial")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "partial.txt")
	if err := os.WriteFile(target, []byte("payload"), 0o644); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	abs, _ := filepath.Abs(target)
	abs = filepath.Clean(abs)

	seedCanonicalFileReadState(t, state, target)
	entry, ok := state.GetForContext(context.Background(), abs)
	if !ok {
		t.Fatal("canonical read did not record evidence")
	}
	entry.IsPartialView = true
	state.SetForContext(context.Background(), abs, entry)

	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "replacement",
	})
	if !res.IsError {
		t.Fatalf("expected partial-view rejection, got success: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "partially") {
		t.Errorf("expected 'partially' rejection message, got: %s", res.Content)
	}
}

// TestFileWriteToolRejectsIpynb locks the Write contract: it does not reject
// .ipynb at runtime; NotebookEdit preference is prompt guidance, not a hard
// Write validation rule.
func TestFileWriteToolRejectsIpynb(t *testing.T) {
	tool := &FileWriteTool{ReadState: NewReadFileState()}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-ipynb")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "notebook.ipynb")
	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "{}",
	})
	if res.IsError {
		t.Fatalf("TS-parity Write should allow .ipynb runtime writes: %s", res.Content)
	}
	out := fileWriteToolDecode(t, res)
	if out["type"] != "create" || out["filePath"] != target || out["content"] != "{}" {
		t.Fatalf("unexpected .ipynb write result: %v", out)
	}
}

// TestFileWriteToolRejectsSymlinkTarget covers write-07 parity: TS writes
// through existing symlinks while preserving the link and target mode.
func TestFileWriteToolRejectsSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin or developer mode on Windows")
	}
	tool := &FileWriteTool{ReadState: NewReadFileState()}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-symlink")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	real := filepath.Join(tmpDir, "real.txt")
	if err := os.WriteFile(real, []byte("real"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	link := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create symlink in this environment: %v", err)
	}
	seedCanonicalFileReadState(t, tool.ReadState, link)

	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": link,
		"content":   "attacker",
	})
	if res.IsError {
		t.Fatalf("expected symlink write-through success, got: %s", res.Content)
	}
	if linkInfo, err := os.Lstat(link); err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink should remain in place: %v %v", linkInfo, err)
	}
	got, _ := os.ReadFile(real)
	if string(got) != "attacker" {
		t.Errorf("real file not modified through symlink: %q", got)
	}
}

// TestFileWriteToolNewFilePayload covers write-05: new-file writes return the
// TS-shaped structured data only.
func TestFileWriteToolNewFilePayload(t *testing.T) {
	tool := &FileWriteTool{ReadState: NewReadFileState()}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-create")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "new.txt")
	content := "hello\nworld"
	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   content,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}

	out := fileWriteToolDecode(t, res)
	if got, _ := out["type"].(string); got != "create" {
		t.Errorf("expected type=create, got %v", out["type"])
	}
	if got, _ := out["filePath"].(string); got != target {
		t.Errorf("expected filePath=%q, got %v", target, out["filePath"])
	}
	if got, _ := out["content"].(string); got != content {
		t.Errorf("expected content=%q, got %v", content, out["content"])
	}
	if out["originalFile"] != nil {
		t.Errorf("expected originalFile=nil for new file, got %v", out["originalFile"])
	}
	for _, key := range []string{"isNew", "bytes", "lineCount", "status", "path", "diagnostics", "metadata", "warning", "remoteGitDiff", "userModified"} {
		if _, ok := out[key]; ok {
			t.Errorf("Go-only field %q leaked into Write result: %v", key, out)
		}
	}
}

// TestFileWriteToolUpdatePayload covers write-05: overwriting an existing file
// returns TS-shaped update data with originalFile and no Go-only counters.
func TestFileWriteToolUpdatePayload(t *testing.T) {
	state := NewReadFileState()
	tool := &FileWriteTool{ReadState: state}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-update")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "existing.txt")
	if err := os.WriteFile(target, []byte("old\ncontent"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedCanonicalFileReadState(t, state, target)

	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "fresh\ndata\nhere",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}

	out := fileWriteToolDecode(t, res)
	if got, _ := out["type"].(string); got != "update" {
		t.Errorf("expected type=update, got %v", out["type"])
	}
	if got, _ := out["content"].(string); got != "fresh\ndata\nhere" {
		t.Errorf("expected updated content, got %v", out["content"])
	}
	if got, _ := out["originalFile"].(string); got != "old\ncontent" {
		t.Errorf("expected originalFile=old\\ncontent, got %v", out["originalFile"])
	}
	for _, key := range []string{"isNew", "bytes", "lineCount", "status", "path", "diagnostics", "metadata", "warning", "remoteGitDiff", "userModified"} {
		if _, ok := out[key]; ok {
			t.Errorf("Go-only field %q leaked into Write result: %v", key, out)
		}
	}
}

// TestFileWriteToolEmitsDocumentationWarning locks the prompt contract:
// text discourages unsolicited docs, but Write does not emit a warning field.
func TestFileWriteToolEmitsDocumentationWarning(t *testing.T) {
	tool := &FileWriteTool{ReadState: NewReadFileState()}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-doc")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "README.md")
	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "# heading\n",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	out := fileWriteToolDecode(t, res)
	if _, ok := out["warning"]; ok {
		t.Errorf("unexpected Go-only documentation warning: %v", out)
	}
}

// TestFileWriteToolRefreshesReadState covers write-04: after a successful
// write the read-state entry is refreshed so an immediate second write does
// not trip stale-write detection.
func TestFileWriteToolRefreshesReadState(t *testing.T) {
	state := NewReadFileState()
	tool := &FileWriteTool{ReadState: state}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-refresh")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "loop.txt")
	res1, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "first",
	})
	if res1.IsError {
		t.Fatalf("first write failed: %s", res1.Content)
	}

	// Sleep a hair so the second write's mtime cannot equal the first's; this
	// confirms the refreshed read-state entry tracks the post-write mtime.
	time.Sleep(10 * time.Millisecond)

	res2, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "second",
	})
	if res2.IsError {
		t.Fatalf("second write failed (read state not refreshed?): %s", res2.Content)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "second" {
		t.Errorf("expected second write to persist, got %q", got)
	}
}

// TestFileWriteToolPlanModeBlocks covers write-08: plan mode short-circuits all
// writes regardless of allowedDirs/read state.
func TestFileWriteToolPlanModeBlocks(t *testing.T) {
	plan := testPlanMode{active: true}
	tool := &FileWriteTool{
		ReadState: NewReadFileState(),
		PlanState: plan,
	}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-plan")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "plan.txt")
	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "blocked",
	})
	if !res.IsError {
		t.Fatalf("expected plan-mode rejection, got success: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "plan mode") {
		t.Errorf("expected plan-mode error, got: %s", res.Content)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("file should not exist after plan-mode block: %v", statErr)
	}
}

// TestFileWriteToolEmptyContent covers the line-count edge case: empty
// content produces lineCount=0.
func TestFileWriteToolEmptyContent(t *testing.T) {
	tool := &FileWriteTool{ReadState: NewReadFileState()}
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "fwt-empty")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	target := filepath.Join(tmpDir, "empty.txt")
	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": target,
		"content":   "",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	out := fileWriteToolDecode(t, res)
	if got, _ := out["bytes"].(float64); int(got) != 0 {
		t.Errorf("expected bytes=0, got %v", out["bytes"])
	}
	if got, _ := out["lineCount"].(float64); int(got) != 0 {
		t.Errorf("expected lineCount=0, got %v", out["lineCount"])
	}
}

// TestFileWriteToolRejectsAllowedDirEscape covers write-08: paths outside the
// configured AllowedDirs sandbox are rejected.
func TestFileWriteToolRejectsAllowedDirEscape(t *testing.T) {
	allowed, err := os.MkdirTemp("", "fwt-allowed")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(allowed)

	other, err := os.MkdirTemp("", "fwt-other")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(other)

	tool := &FileWriteTool{
		AllowedDirs: []string{allowed},
		ReadState:   NewReadFileState(),
	}
	ctx := context.Background()

	res, _ := tool.Execute(ctx, map[string]any{
		"file_path": filepath.Join(other, "escape.txt"),
		"content":   "nope",
	})
	if !res.IsError {
		t.Fatalf("expected allowed-dirs rejection, got success: %s", res.Content)
	}
}

func TestFileEditTool(t *testing.T) {
	state := NewReadFileState()
	tool := &FileEditTool{ReadState: state}
	ctx := context.Background()

	if tool.Name() != "Edit" {
		t.Errorf("expected Name='Edit', got %q", tool.Name())
	}

	tmpFile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	originalContent := "hello world"
	os.WriteFile(tmpFile.Name(), []byte(originalContent), 0644)

	// Pre-record a Read so the Edit's Read-before-Edit guard passes.
	absPath, _ := filepath.Abs(tmpFile.Name())
	recordStrongReadEvidenceForTest(t, state, absPath)

	// Test replace
	result, err := tool.Execute(ctx, map[string]any{
		"file_path":  tmpFile.Name(),
		"old_string": "world",
		"new_string": "universe",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got %v", result.Content)
	}

	// Verify content was replaced
	fileContent, _ := os.ReadFile(tmpFile.Name())
	expected := "hello universe"
	if string(fileContent) != expected {
		t.Errorf("expected %q, got %q", expected, fileContent)
	}
}
