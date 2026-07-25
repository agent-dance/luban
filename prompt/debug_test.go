package prompt

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildPromptDumpJSONIncludesMetadataAndRedactsSecrets(t *testing.T) {
	blocks := ApplyCacheScopes([]SystemPromptBlock{{
		Text:   "Authorization: Bearer sk-secret1234567890\napi_key=abc123secret456",
		Source: "built_in",
		Name:   "static",
		Cache:  true,
	}}, CacheScopeOptions{GlobalSafe: true})
	userCtx := UserContext{
		Instructions: "password: hunter2",
		CurrentDate:  "Today's date is 2026-07-10.",
	}
	systemCtx := SystemContext{GitStatus: "clean"}

	dump := BuildPromptDump(blocks, userCtx, systemCtx)
	var buf bytes.Buffer
	if err := WritePromptDumpJSON(&buf, dump); err != nil {
		t.Fatalf("WritePromptDumpJSON: %v", err)
	}

	var decoded PromptDump
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("dump JSON did not parse: %v\n%s", err, buf.String())
	}
	if len(decoded.Blocks) != 1 {
		t.Fatalf("Blocks len = %d, want 1", len(decoded.Blocks))
	}
	block := decoded.Blocks[0]
	if block.ID == "" || block.Name != "static" || block.Source != "built_in" || block.CacheScope != CacheScopeGlobal || block.TextHash == "" || block.Text == "" {
		t.Fatalf("dump block missing metadata: %#v", block)
	}
	if strings.Contains(strings.ToLower(buf.String()), "hunter2") ||
		strings.Contains(buf.String(), "sk-secret1234567890") ||
		strings.Contains(buf.String(), "abc123secret456") {
		t.Fatalf("dump leaked secret material:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "[REDACTED]") {
		t.Fatalf("dump should contain redaction marker:\n%s", buf.String())
	}
	if len(decoded.Context) != 3 {
		t.Fatalf("Context len = %d, want instructions/currentDate/gitStatus", len(decoded.Context))
	}
}

func TestPromptDumpTextIsReadable(t *testing.T) {
	dump := BuildPromptDump(
		[]SystemPromptBlock{{Text: "hello", Source: "runtime", Name: "dynamic"}},
		UserContextBuilder{Date: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)}.Build(),
		SystemContext{},
	)
	var buf bytes.Buffer
	if err := WritePromptDumpText(&buf, dump); err != nil {
		t.Fatalf("WritePromptDumpText: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"## system.dynamic.0", "source: runtime", "text_hash:", "hello", "## context", "currentDate"} {
		if !strings.Contains(out, want) {
			t.Fatalf("text dump missing %q:\n%s", want, out)
		}
	}
}
