package inspect

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/internal/tools/file"
	"github.com/agent-dance/luban/types"
)

func TestInspectCursorContinuationUsesItsServerSnapshot(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha.txt", "beta.txt", "gamma.txt"} {
		writeInspectFixture(t, filepath.Join(root, name), "value\n")
	}

	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, file.NewReadFileState())
	first, err := executeInspectTest(tool, context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "files", "kind": KindGlob, "path": ".", "pattern": "**/*.txt",
		}},
		"max_chars": minimumMaxChars,
		"max_files": 1,
	})
	if err != nil || first.IsError {
		t.Fatalf("first page failed: err=%v result=%+v", err, first)
	}
	firstPage := first.Data.(Result)
	if firstPage.Cursor == "" {
		t.Fatalf("first page did not paginate: %+v", firstPage)
	}

	continuation, err := executeInspectTest(tool, context.Background(), map[string]any{"cursor": firstPage.Cursor})
	if err != nil || continuation.IsError {
		t.Fatalf("model-shaped continuation failed: err=%v result=%+v", err, continuation)
	}
	page := continuation.Data.(Result)
	if got := len(page.Requests[0].Files); got != 1 {
		t.Fatalf("cursor snapshot limit was replaced by placeholder: files=%d page=%+v", got, page)
	}
}

func TestInspectDiscriminatedContinuationRejectsNewFieldsAndUnknownFields(t *testing.T) {
	tool := New(nil, nil)

	if _, err := tool.validateInput(map[string]any{
		"operation": map[string]any{
			"mode": ModeContinue, "cursor": "opaque-cursor",
			"requests": []any{map[string]any{"id": "mixed", "kind": KindGlob}},
		},
	}); err == nil {
		t.Fatal("cursor continuation accepted a non-empty request batch")
	}

	_, err := tool.validateInput(map[string]any{
		"operation": map[string]any{
			"mode": ModeContinue, "cursor": "opaque-cursor", "unsupported": true,
		},
	})
	var validationErr *types.ToolInputValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("cursor continuation accepted or misclassified an unknown field: %T %v", err, err)
	}
}
