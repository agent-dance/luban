package tools

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="

func TestMCPRenderToolResultPreservesStructuredContentMetaAndError(t *testing.T) {
	raw := mustJSONRaw(t, map[string]any{
		"isError": true,
		"_meta": map[string]any{
			"requestId": "req-1",
		},
		"structuredContent": map[string]any{
			"items": []map[string]any{{"id": 1, "name": "alpha"}},
		},
		"content": []map[string]any{{
			"type": "text",
			"text": "human readable fallback",
		}},
	})

	result := renderMCPCallToolResult(raw, "srv", "lookup")
	if !result.IsError {
		t.Fatalf("expected isError to be preserved")
	}
	if got := result.Metadata["mcp._meta"]; !strings.Contains(got, `"requestId":"req-1"`) {
		t.Fatalf("_meta not preserved in metadata: %q", got)
	}
	if got := result.Metadata["mcp.structuredContent"]; !strings.Contains(got, `"items"`) {
		t.Fatalf("structuredContent not preserved in metadata: %q", got)
	}
	if len(result.ContentBlocks) < 2 {
		t.Fatalf("expected structured content plus content[] blocks, got %#v", result.ContentBlocks)
	}
	first, ok := result.ContentBlocks[0].(types.TextBlock)
	if !ok || !strings.Contains(first.Text, `"items"`) {
		t.Fatalf("first block should carry structuredContent JSON, got %#v", result.ContentBlocks[0])
	}
}

func TestMCPRenderImageContentBlockUnderBudget(t *testing.T) {
	raw := mustJSONRaw(t, map[string]any{
		"content": []map[string]any{{
			"type":     "image",
			"mimeType": "image/png",
			"data":     tinyPNGBase64,
		}},
	})

	result := renderMCPCallToolResult(raw, "vision", "chart")
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("expected one image block, got %#v", result.ContentBlocks)
	}
	image, ok := result.ContentBlocks[0].(types.ImageBlock)
	if !ok {
		t.Fatalf("expected ImageBlock, got %T", result.ContentBlocks[0])
	}
	if image.Source == nil || image.Source.MediaType != "image/png" || image.Source.Data == "" {
		t.Fatalf("bad image source: %#v", image.Source)
	}
	if strings.Contains(result.Content, tinyPNGBase64) {
		t.Fatalf("summary should not inline image base64")
	}
}

func TestMCPOutputBinaryPersistsRawBytes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(mcpToolResultsDirEnv, dir)
	payload := []byte("hello audio")
	raw := mustJSONRaw(t, map[string]any{
		"content": []map[string]any{{
			"type":     "audio",
			"mimeType": "audio/wav",
			"data":     base64.StdEncoding.EncodeToString(payload),
		}},
	})

	result := renderMCPCallToolResult(raw, "media", "play")
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("expected one text block, got %#v", result.ContentBlocks)
	}
	text, ok := result.ContentBlocks[0].(types.TextBlock)
	if !ok || !strings.Contains(text.Text, "saved to") {
		t.Fatalf("expected saved-path text block, got %#v", result.ContentBlocks[0])
	}
	files := mustGlobOne(t, filepath.Join(dir, "*.wav"))
	got, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("binary file should contain raw bytes, got %q", string(got))
	}
	if strings.Contains(result.Content, base64.StdEncoding.EncodeToString(payload)) {
		t.Fatalf("binary base64 leaked into model content")
	}
}

func TestMCPRenderResourceTextBlobAndResourceLink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(mcpToolResultsDirEnv, dir)
	pdfBytes := []byte("%PDF-1.7\nbody")
	raw := mustJSONRaw(t, map[string]any{
		"content": []map[string]any{
			{
				"type": "resource",
				"resource": map[string]any{
					"uri":      "memo://alpha",
					"mimeType": "text/markdown",
					"text":     "# Alpha",
				},
			},
			{
				"type":        "resource_link",
				"name":        "Spec",
				"uri":         "memo://spec",
				"description": "canonical spec",
			},
			{
				"type": "resource",
				"resource": map[string]any{
					"uri":      "file://report.pdf",
					"mimeType": "application/pdf",
					"blob":     base64.StdEncoding.EncodeToString(pdfBytes),
				},
			},
		},
	})

	result := renderMCPCallToolResult(raw, "docs", "read")
	text := result.TextContent()
	if !strings.Contains(text, "[Resource from docs at memo://alpha] # Alpha") {
		t.Fatalf("resource text missing from result: %q", text)
	}
	if !strings.Contains(text, "[Resource link: Spec] memo://spec (canonical spec)") {
		t.Fatalf("resource_link missing from result: %q", text)
	}
	files := mustGlobOne(t, filepath.Join(dir, "*.pdf"))
	got, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read persisted pdf: %v", err)
	}
	if string(got) != string(pdfBytes) {
		t.Fatalf("pdf persisted bytes mismatch: %q", string(got))
	}
}

func TestMCPLargeOutputPersistsToFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(mcpToolResultsDirEnv, dir)
	t.Setenv("MAX_MCP_OUTPUT_TOKENS", "5")
	large := strings.Repeat("x", 200)
	raw := mustJSONRaw(t, map[string]any{
		"content": []map[string]any{{
			"type": "text",
			"text": large,
		}},
	})

	result := renderMCPCallToolResult(raw, "big", "dump")
	if !strings.Contains(result.Content, "Output has been saved to") {
		t.Fatalf("expected large-output instructions, got %q", result.Content)
	}
	if strings.Contains(result.Content, large) {
		t.Fatalf("large raw output leaked into model content")
	}
	path := result.Metadata["mcp.largeOutputPath"]
	if path == "" {
		t.Fatalf("large output path missing from metadata: %#v", result.Metadata)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read large output: %v", err)
	}
	if !strings.Contains(string(got), large) || !strings.Contains(string(got), `"type": "text"`) {
		t.Fatalf("large content[] output should persist as JSON blocks, got %q", string(got))
	}
}

func TestMCPRenderUnknownBinaryDoesNotInlineBase64(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(mcpToolResultsDirEnv, dir)
	payload := []byte{0, 1, 2, 3, 4}
	encoded := base64.StdEncoding.EncodeToString(payload)
	raw := mustJSONRaw(t, map[string]any{
		"content": []map[string]any{{
			"type":     "blob",
			"mimeType": "application/octet-stream",
			"data":     encoded,
		}},
	})

	result := renderMCPCallToolResult(raw, "bin", "get")
	if strings.Contains(result.Content, encoded) {
		t.Fatalf("unknown binary base64 leaked into model content")
	}
	files := mustGlobOne(t, filepath.Join(dir, "*.bin"))
	got, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read persisted binary: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("persisted binary mismatch: %#v", got)
	}
}

func mustJSONRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal test JSON: %v", err)
	}
	return data
}

func mustGlobOne(t *testing.T, pattern string) []string {
	t.Helper()
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %q: %v", pattern, err)
	}
	if len(files) != 1 {
		t.Fatalf("glob %q matched %d files: %v", pattern, len(files), files)
	}
	return files
}
