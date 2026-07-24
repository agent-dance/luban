// Package tools contains NotebookEditTool alignment tests against the TS
// contract. Successful edit helpers seed ReadState; guard tests deliberately
// omit or stale it.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ─── Fixtures ───────────────────────────────────────────────────────────────

const alignmentNotebookFixture = `{
  "cells":[
    {"cell_type":"code","id":"c1","metadata":{},"outputs":[{"output_type":"stream","name":"stdout","text":["hello"]}],"source":["print('hello')"],"execution_count":1},
    {"cell_type":"markdown","id":"m1","metadata":{},"source":["# heading"]}
  ],
  "metadata":{},
  "nbformat":4,
  "nbformat_minor":5
}`

func alignmentWriteNotebook(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "n.ipynb")
	if err := os.WriteFile(p, []byte(alignmentNotebookFixture), 0644); err != nil {
		t.Fatalf("seed notebook: %v", err)
	}
	return p
}

// alignmentRunNotebookEdit calls NotebookEditTool.Execute, returning the
// raw ToolResult so each test can probe whichever facet it asserts on.
func alignmentRunNotebookEdit(t *testing.T, in map[string]any) (*NotebookEditTool, ToolResultWrap) {
	t.Helper()
	tool := &NotebookEditTool{ReadState: NewReadFileState()}
	if path, _ := in["notebook_path"].(string); path != "" {
		alignmentSeedReadState(t, tool.ReadState, path)
	}
	res, err := tool.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("infrastructure error: %v", err)
	}
	return tool, ToolResultWrap{
		Content: res.Content,
		IsError: res.IsError,
		Meta:    res.Metadata,
	}
}

func alignmentSeedReadState(t *testing.T, state *ReadFileState, path string) {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		t.Fatalf("stat seed: %v", err)
	}
	state.Set(filepath.Clean(abs), ReadFileEntry{
		TimestampMs:   info.ModTime().UnixMilli(),
		Content:       string(data),
		IsPartialView: false,
		LastTool:      "Read",
	})
}

// ToolResultWrap shadows types.ToolResult for terse assertion.
type ToolResultWrap struct {
	Content string
	IsError bool
	Meta    map[string]string
}

// ─── Surface: Read-before-Edit gate ─────────────────────────────────────────

// TestAlignment_NotebookEdit_HasReadStateField locks the Read/Edit
// coordination dependency exposed by the TS tool-use context.
func TestAlignment_NotebookEdit_HasReadStateField(t *testing.T) {
	tool := &NotebookEditTool{}
	v := reflect.TypeOf(tool).Elem()
	if _, ok := v.FieldByName("ReadState"); !ok {
		t.Fatalf("expected NotebookEditTool.ReadState field; got empty struct")
	}
}

// TestAlignment_NotebookEdit_RequiresPriorRead asserts an edit on a
// notebook that has not been read is rejected.
func TestAlignment_NotebookEdit_RequiresPriorRead(t *testing.T) {
	p := alignmentWriteNotebook(t)
	tool := &NotebookEditTool{ReadState: NewReadFileState()}
	raw, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": p,
		"cell_id":       "c1",
		"new_source":    "print('updated')",
		"edit_mode":     "replace",
	})
	if err != nil {
		t.Fatalf("infrastructure error: %v", err)
	}
	res := ToolResultWrap{Content: raw.Content, IsError: raw.IsError, Meta: raw.Metadata}
	if !res.IsError {
		t.Fatalf("expected edit to be rejected without prior Read; succeeded with: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "read") {
		t.Errorf("expected error message to mention required Read; got %q", res.Content)
	}
}

// ─── Output: structured JSON result shape ───────────────────────────────────

// TestAlignment_NotebookEdit_ResultIsJSON asserts that ToolResult.Content
// carries the typed JSON data while the mapper owns concise model text.
func TestAlignment_NotebookEdit_ResultIsJSON(t *testing.T) {
	p := alignmentWriteNotebook(t)
	_, res := alignmentRunNotebookEdit(t, map[string]any{
		"notebook_path": p,
		"cell_id":       "c1",
		"new_source":    "print('after')",
		"edit_mode":     "replace",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	trimmed := strings.TrimSpace(res.Content)
	if !strings.HasPrefix(trimmed, "{") {
		t.Fatalf("expected JSON-object result; got %q", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
}

// TestAlignment_NotebookEdit_ResultUsesSnakeCaseKeys asserts that the
// result keys mirror the TS output schema.
func TestAlignment_NotebookEdit_ResultUsesSnakeCaseKeys(t *testing.T) {
	p := alignmentWriteNotebook(t)
	_, res := alignmentRunNotebookEdit(t, map[string]any{
		"notebook_path": p,
		"cell_id":       "c1",
		"new_source":    "print('after')",
		"edit_mode":     "replace",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	for _, k := range []string{"new_source", "cell_id", "cell_type", "language", "edit_mode", "error", "notebook_path", "original_file", "updated_file"} {
		if _, ok := obj[k]; !ok {
			t.Errorf("expected snake_case key %q in result; got %v", k, obj)
		}
	}
}

func TestAlignment_NotebookEdit_ResultOmitsGoOnlyFields(t *testing.T) {
	p := alignmentWriteNotebook(t)
	_, res := alignmentRunNotebookEdit(t, map[string]any{
		"notebook_path": p,
		"cell_id":       "c1",
		"new_source":    "print('after')",
		"edit_mode":     "replace",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	var obj map[string]any
	_ = json.Unmarshal([]byte(res.Content), &obj)
	for _, key := range []string{"file_path", "before", "after", "edit_id", "cell_index", "execution_count_reset", "editMode"} {
		if _, ok := obj[key]; ok {
			t.Fatalf("TS NotebookEdit result must not expose Go-only %q: %v", key, obj)
		}
	}
}

func TestAlignment_NotebookEdit_DeleteResultShape(t *testing.T) {
	p := alignmentWriteNotebook(t)
	_, res := alignmentRunNotebookEdit(t, map[string]any{
		"notebook_path": p,
		"cell_id":       "c1",
		"edit_mode":     "delete",
		"new_source":    "", // ignored by delete
	})
	if res.IsError {
		t.Fatalf("delete failed: %s", res.Content)
	}
	var obj map[string]any
	_ = json.Unmarshal([]byte(res.Content), &obj)
	if obj["edit_mode"] != "delete" || obj["cell_id"] != "c1" {
		t.Fatalf("unexpected delete result: %v", obj)
	}
	if _, ok := obj["before"]; ok {
		t.Fatalf("delete result must not expose before snapshot: %v", obj)
	}
}

// ─── Surface: insert validation ─────────────────────────────────────────────

// TestAlignment_NotebookEdit_InsertRequiresCellType asserts that
// insert without cell_type is rejected like the TS validator.
func TestAlignment_NotebookEdit_InsertRequiresCellType(t *testing.T) {
	p := alignmentWriteNotebook(t)
	_, res := alignmentRunNotebookEdit(t, map[string]any{
		"notebook_path": p,
		"cell_id":       "c1",
		"new_source":    "x = 1",
		"edit_mode":     "insert",
	})
	if !res.IsError {
		t.Fatalf("expected insert without cell_type to be rejected; got success: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "cell_type") {
		t.Errorf("expected error to mention cell_type; got %q", res.Content)
	}
}

func TestAlignment_NotebookEdit_ReplaceTypeChangeResetsFileOnly(t *testing.T) {
	p := alignmentWriteNotebook(t)
	_, res := alignmentRunNotebookEdit(t, map[string]any{
		"notebook_path": p,
		"cell_id":       "c1",
		"cell_type":     "markdown",
		"new_source":    "now markdown",
		"edit_mode":     "replace",
	})
	if res.IsError {
		t.Fatalf("replace failed: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if _, ok := obj["execution_count_reset"]; ok {
		t.Fatalf("TS result must not expose execution_count_reset: %v", obj)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), `"execution_count": null`) {
		t.Fatalf("expected edited code cell execution_count reset in file: %s", data)
	}
}
