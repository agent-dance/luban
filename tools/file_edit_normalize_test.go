package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/types"
)

type p0EditRuntime struct{ root string }

func (runtime p0EditRuntime) ToolRuntimeContext() types.ToolRuntimeContext {
	return types.ToolRuntimeContext{ProjectRoot: runtime.root, AllowedDirs: []string{runtime.root}}
}

func TestP0FileEditNormalizerDesanitizesAndPreservesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.txt")
	if err := os.WriteFile(path, []byte("<function_results>\n<name>x</name>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &FileEditTool{Runtime: p0EditRuntime{root: dir}, ReadState: NewReadFileState()}
	input := map[string]any{
		"file_path": "tokens.txt", "old_string": "<fnr>\n<n>x</n>",
		"new_string": "<fnr>\n<n>y</n>   ", "replace_all": false,
		"unknown": "kept-for-strict-validation",
	}
	normalized, err := tool.NormalizeToolInput(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if normalized["file_path"] != path || normalized["old_string"] != "<fnr>\n<n>x</n>" ||
		normalized["new_string"] != "<fnr>\n<n>y</n>" {
		t.Fatalf("unexpected normalized input: %#v", normalized)
	}
	if normalized["unknown"] != input["unknown"] {
		t.Fatal("normalization swallowed an unknown field")
	}
	if input["file_path"] != "tokens.txt" || input["old_string"] != "<fnr>\n<n>x</n>" {
		t.Fatal("normalization mutated the caller's input")
	}
	if tool.ReadState.Len() != 0 {
		t.Fatal("input normalization incorrectly granted Read evidence")
	}
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: tool.ReadState}
	if result := p0Read(t, read, map[string]any{"file_path": path}); result.IsError {
		t.Fatal(result.Content)
	}
	delete(normalized, "unknown")
	result, err := tool.Execute(context.Background(), normalized)
	if err != nil || result.IsError {
		t.Fatalf("permission-safe desanitized Edit failed: result=%+v err=%v", result, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "<function_results>\n<name>y</name>\n" {
		t.Fatalf("desanitized edit result = %q err=%v", got, err)
	}
}

func TestP0FileEditNormalizerPreservesMarkdownTrailingSpaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &FileEditTool{Runtime: p0EditRuntime{root: dir}}
	normalized, err := tool.BackfillObservableInput(map[string]any{
		"file_path": path, "old_string": "old", "new_string": "new  \nnext  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if normalized["new_string"] != "new  \nnext  " {
		t.Fatalf("Markdown hard-break spaces were stripped: %q", normalized["new_string"])
	}
}
