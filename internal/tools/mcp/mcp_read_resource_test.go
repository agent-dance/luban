package mcp

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renderReadResourceFixture(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	result := renderMCPReadResourceToolResult(raw, "docs")
	if result.IsError {
		t.Fatalf("render fixture: %s", result.Content)
	}
	return result.Content
}

func TestReadMcpResourceOutputDropsRawMetaAndUnknownFields(t *testing.T) {
	content := renderReadResourceFixture(t, map[string]any{
		"contents": []map[string]any{
			{
				"uri":         "memo://alpha",
				"mimeType":    "text/markdown",
				"text":        "# Alpha",
				"blob":        "must-not-win-over-text",
				"type":        "text",
				"annotations": map[string]any{"audience": []string{"assistant"}},
				"_meta":       map[string]any{"source": "fixture"},
				"unknown":     "drop-me",
			},
			{
				"uri":      "memo://empty",
				"mimeType": "application/octet-stream",
				"data":     "raw-data-is-not-a-resource-blob",
				"type":     "blob",
			},
		},
		"_meta": map[string]any{"request": "read"},
	})
	want := `{"contents":[{"uri":"memo://alpha","mimeType":"text/markdown","text":"# Alpha"},{"uri":"memo://empty","mimeType":"application/octet-stream"}]}`
	if content != want {
		t.Fatalf("normalized output:\n got: %s\nwant: %s", content, want)
	}
	for _, forbidden := range []string{"_meta", "annotations", "unknown", "blob", "data", `"type"`} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("raw MCP field %q leaked into output: %s", forbidden, content)
		}
	}
}

func TestReadMcpResourceOutputPreservesTextAndEmptyArray(t *testing.T) {
	text := " {\"b\":2,\"a\":1} \n"
	content := renderReadResourceFixture(t, map[string]any{"contents": []map[string]any{{
		"uri": "memo://json", "mimeType": "application/json", "text": text,
	}}})
	var output ReadMcpResourceOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output.Contents) != 1 || output.Contents[0].Text == nil || *output.Contents[0].Text != text {
		t.Fatalf("text changed: %#v", output)
	}
	if got := renderReadResourceFixture(t, map[string]any{"contents": []any{}}); got != `{"contents":[]}` {
		t.Fatalf("empty output = %q", got)
	}
}

func TestReadMcpResourceBinaryPersistsWithoutInliningBase64(t *testing.T) {
	for _, size := range []int{17, 256*1024 + 31} {
		size := size
		t.Run(map[bool]string{true: "large", false: "small"}[size > 256*1024], func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(mcpToolResultsDirEnv, dir)
			payload := make([]byte, size)
			for i := range payload {
				payload[i] = byte(i % 251)
			}
			encoded := base64.StdEncoding.EncodeToString(payload)
			content := renderReadResourceFixture(t, map[string]any{"contents": []map[string]any{{
				"uri": "file://report.pdf", "mimeType": "application/pdf", "blob": encoded,
			}}})
			if strings.Contains(content, encoded) || strings.Contains(content, `"blob"`) || strings.Contains(content, `"data"`) {
				t.Fatalf("raw binary leaked: %s", content)
			}
			var output ReadMcpResourceOutput
			if err := json.Unmarshal([]byte(content), &output); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if len(output.Contents) != 1 || output.Contents[0].BlobSavedTo == "" || output.Contents[0].Text == nil {
				t.Fatalf("binary output shape: %#v", output)
			}
			path := output.Contents[0].BlobSavedTo
			if filepath.Dir(path) != dir || filepath.Ext(path) != ".pdf" {
				t.Fatalf("persist path = %q", path)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read persisted bytes: %v", err)
			}
			if string(got) != string(payload) {
				t.Fatalf("persisted bytes = %d, want %d", len(got), len(payload))
			}
			if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("persisted file mode = %v, %v", info, err)
			}
		})
	}
}

func TestReadMcpResourceBinaryPersistFailureDoesNotLeakBlob(t *testing.T) {
	parent := t.TempDir()
	notDir := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(notDir, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(mcpToolResultsDirEnv, notDir)
	encoded := base64.StdEncoding.EncodeToString([]byte("binary"))
	content := renderReadResourceFixture(t, map[string]any{"contents": []map[string]any{{
		"uri": "memo://blob", "mimeType": "application/octet-stream", "blob": encoded,
	}}})
	var output ReadMcpResourceOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(output.Contents) != 1 || output.Contents[0].Text == nil || !strings.HasPrefix(*output.Contents[0].Text, "Binary content could not be saved to disk: ") {
		t.Fatalf("persist failure output = %#v", output)
	}
	if output.Contents[0].BlobSavedTo != "" || strings.Contains(content, encoded) {
		t.Fatalf("failed persistence leaked binary: %s", content)
	}
}

func TestReadMcpResourceOutputRejectsMalformedEnvelope(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"contents":[{"text":"missing uri"}]}`),
		json.RawMessage(`{"contents":[`),
	} {
		if result := renderMCPReadResourceToolResult(raw, "docs"); !result.IsError {
			t.Fatalf("malformed result accepted: %s", raw)
		}
	}
}
