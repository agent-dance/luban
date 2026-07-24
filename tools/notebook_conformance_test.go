package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// fixtureNotebookJSON is the canonical .ipynb-shape JSON used by these tests.
// It exercises both string-source and array-source cell forms, kernelspec
// metadata, and a code cell with stored outputs.
const fixtureNotebookJSON = `{
 "cells": [
  {
   "cell_type": "markdown",
   "id": "abc12345",
   "metadata": {"author":"alice"},
   "source": "# Title\n\nIntro paragraph"
  },
  {
   "cell_type": "code",
   "id": "def67890",
   "metadata": {},
   "execution_count": 1,
   "source": ["print(", "\"hello\"", ")"],
   "outputs": [
    {"output_type":"stream","name":"stdout","text":["hello\n"]}
   ]
  }
 ],
 "metadata": {
  "kernelspec": {"name":"python3","display_name":"Python 3"},
  "language_info": {"name":"python"}
 },
 "nbformat": 4,
 "nbformat_minor": 5
}
`

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.ipynb")
	if err := os.WriteFile(path, []byte(fixtureNotebookJSON), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func seedNotebookReadState(t *testing.T, state *ReadFileState, path string) {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read state content: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		t.Fatalf("read state stat: %v", err)
	}
	state.Set(filepath.Clean(abs), ReadFileEntry{
		TimestampMs:   info.ModTime().UnixMilli(),
		Content:       string(data),
		IsPartialView: false,
		LastTool:      "Read",
	})
}

func newSeededNotebookEditTool(t *testing.T, path string) *NotebookEditTool {
	t.Helper()
	state := NewReadFileState()
	seedNotebookReadState(t, state, path)
	return &NotebookEditTool{ReadState: state}
}

func TestNotebookParseRoundtrip(t *testing.T) {
	nb, err := ParseNotebook([]byte(fixtureNotebookJSON))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if nb.NBFormat != 4 || nb.NBFormatMinor != 5 {
		t.Errorf("nbformat lost: got %d.%d", nb.NBFormat, nb.NBFormatMinor)
	}
	if _, ok := nb.Metadata["kernelspec"]; !ok {
		t.Errorf("kernelspec metadata not preserved")
	}
	if _, ok := nb.Metadata["language_info"]; !ok {
		t.Errorf("language_info metadata not preserved")
	}
	if len(nb.Cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(nb.Cells))
	}
	if joinSource(nb.Cells[0].Source) != "# Title\n\nIntro paragraph" {
		t.Errorf("string source lost roundtrip: %q", joinSource(nb.Cells[0].Source))
	}
	// Array source preserved as joined back
	if joinSource(nb.Cells[1].Source) != `print("hello")` {
		t.Errorf("array source lost: %q", joinSource(nb.Cells[1].Source))
	}
	if nb.Cells[1].ExecutionCount == nil || *nb.Cells[1].ExecutionCount != 1 {
		t.Errorf("execution_count lost")
	}
	if len(nb.Cells[1].Outputs) != 1 {
		t.Errorf("outputs lost on parse: %#v", nb.Cells[1].Outputs)
	}

	out, err := SerializeNotebook(nb)
	if err != nil {
		t.Fatalf("serialise: %v", err)
	}
	// Round-trip again.
	nb2, err := ParseNotebook(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(nb2.Cells) != 2 {
		t.Fatalf("roundtrip cell count")
	}
	if nb2.Cells[1].ExecutionCount == nil || *nb2.Cells[1].ExecutionCount != 1 {
		t.Errorf("execution_count lost on serialise")
	}
}

func TestNotebookParsePreservesMissingCellIDs(t *testing.T) {
	src := `{"cells":[{"cell_type":"code","metadata":{},"source":""}],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	nb, err := ParseNotebook([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if nb.Cells[0].ID != "" {
		t.Errorf("existing id-less cells should remain id-less, got %q", nb.Cells[0].ID)
	}
}

func TestResolveCellByID(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	c, idx, err := ResolveCell(nb, "def67890")
	if err != nil || idx != 1 || c == nil {
		t.Fatalf("resolve by id failed: idx=%d err=%v", idx, err)
	}
}

func TestResolveCellRejectsBareNumericIndex(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	if _, _, err := ResolveCell(nb, "0"); err == nil {
		t.Fatalf("bare numeric indexes are not accepted by TS NotebookEdit")
	}
}

func TestResolveCellRejectsLast(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	if _, _, err := ResolveCell(nb, "last"); err == nil {
		t.Fatalf("last alias is not accepted by TS NotebookEdit")
	}
}

// TSref: parseCellId accepts only /^cell-(\d+)$/ and exact IDs are compared
// without trimming or normalization.
func TestNotebookCellIDStrictFallbackAndExactMatch(t *testing.T) {
	nb := &Notebook{Cells: []Cell{{ID: " spaced "}, {ID: "plain"}}}
	if _, idx, err := ResolveCell(nb, " spaced "); err != nil || idx != 0 {
		t.Fatalf("exact spaced ID lookup = idx %d, err %v", idx, err)
	}
	for _, ref := range []string{" plain ", "cell-+1", "cell--1", "cell- 1", "cell-1x"} {
		if _, _, err := ResolveCell(nb, ref); err == nil {
			t.Fatalf("ResolveCell(%q) accepted a non-TS target", ref)
		}
	}
}

func TestResolveCellUnknown(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	if _, _, err := ResolveCell(nb, "nope"); err == nil {
		t.Error("expected error for unknown cell")
	}
}

func TestApplyEditReplaceClearsCodeOutputs(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	outcome, err := applyNotebookEdit(nb, NotebookEditOp{
		CellID:    "def67890",
		NewSource: "print(\"hi\")",
		EditMode:  "replace",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(nb.Cells[1].Outputs) != 0 {
		t.Error("expected outputs to be cleared when code source changes")
	}
	if !nb.Cells[1].HasExecutionCount || nb.Cells[1].ExecutionCount != nil {
		t.Error("expected execution_count:null when code source changes")
	}
	if outcome.Before == nil || outcome.After == nil {
		t.Error("expected before/after snapshots on replace")
	}
}

func TestApplyEditTypeChangeClearsOutputs(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	_, err := applyNotebookEdit(nb, NotebookEditOp{
		CellID:    "def67890",
		NewSource: "now markdown",
		CellType:  "markdown",
		EditMode:  "replace",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(nb.Cells[1].Outputs) != 0 {
		t.Error("expected outputs cleared on cell_type change")
	}
	if nb.Cells[1].ExecutionCount != nil {
		t.Error("expected execution_count cleared on type change")
	}
}

func TestApplyEditInsertAtTop(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	outcome, err := applyNotebookEdit(nb, NotebookEditOp{
		NewSource: "# new",
		CellType:  "markdown",
		EditMode:  "insert",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if outcome.CellIndex != 0 {
		t.Errorf("expected insert at index 0, got %d", outcome.CellIndex)
	}
	if len(nb.Cells) != 3 {
		t.Errorf("expected 3 cells after insert, got %d", len(nb.Cells))
	}
}

func TestApplyEditInsertAfterCell(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	_, err := applyNotebookEdit(nb, NotebookEditOp{
		CellID:    "abc12345",
		NewSource: "added",
		CellType:  "code",
		EditMode:  "insert",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(nb.Cells) != 3 || joinSource(nb.Cells[1].Source) != "added" {
		t.Errorf("insert after id failed; cells=%d source=%q", len(nb.Cells), joinSource(nb.Cells[1].Source))
	}
}

func TestApplyEditDelete(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	outcome, err := applyNotebookEdit(nb, NotebookEditOp{
		CellID:   "abc12345",
		EditMode: "delete",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(nb.Cells) != 1 {
		t.Errorf("expected 1 cell after delete, got %d", len(nb.Cells))
	}
	if outcome.CellID != "abc12345" {
		t.Errorf("expected outcome.CellID=abc12345, got %s", outcome.CellID)
	}
}

func TestApplyEditDeleteRequiresCellID(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	if _, err := applyNotebookEdit(nb, NotebookEditOp{EditMode: "delete"}); err == nil {
		t.Error("expected error when delete missing cell_id")
	}
}

func TestApplyEditUnknownMode(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	if _, err := applyNotebookEdit(nb, NotebookEditOp{EditMode: "explode"}); err == nil {
		t.Error("expected error for unknown edit_mode")
	}
}

func TestCapOutputsTruncatesOversize(t *testing.T) {
	big := strings.Repeat("x", maxCellOutputBytes+1)
	outs := []any{
		map[string]any{"output_type": "stream", "text": []string{big}},
	}
	capped := CapOutputs(outs)
	if len(capped) == 0 {
		t.Fatal("expected truncation marker preserved")
	}
	last := capped[len(capped)-1].(map[string]any)
	text := last["text"].([]string)
	if !strings.Contains(text[0], truncatedOutputMarker) {
		t.Errorf("expected truncation marker in last output, got %v", last)
	}
}

func TestCapOutputsKeepsSmallOutputs(t *testing.T) {
	outs := []any{map[string]any{"output_type": "stream", "text": []string{"ok"}}}
	if got := CapOutputs(outs); len(got) != 1 {
		t.Errorf("expected pass-through, got %d outputs", len(got))
	}
}

func TestNotebookEditToolReplaceUpdatesFile(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print(\"updated\")",
	})
	if res.IsError {
		t.Fatalf("tool error: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "updated") {
		t.Errorf("file not updated: %s", string(data))
	}
	// kernelspec preserved
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid json after edit: %v", err)
	}
	meta, _ := raw["metadata"].(map[string]any)
	if _, ok := meta["kernelspec"]; !ok {
		t.Errorf("kernelspec lost on edit")
	}
}

func TestNotebookEditToolRejectsNonNotebookPath(t *testing.T) {
	tool := &NotebookEditTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": "not-a-notebook.txt",
		"new_source":    "x",
	})
	if !res.IsError || !strings.Contains(res.Content, ".ipynb") {
		t.Errorf("expected .ipynb validation, got %q", res.Content)
	}
}

func TestNotebookEditToolRejectsBadCellType(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"new_source":    "x",
		"cell_type":     "spreadsheet",
	})
	if !res.IsError || !strings.Contains(res.Content, "cell_type") {
		t.Errorf("expected cell_type validation, got %q", res.Content)
	}
}

func TestNotebookEditToolInsertAndDelete(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "abc12345",
		"new_source":    "print('mid')",
		"cell_type":     "code",
		"edit_mode":     "insert",
	})
	if res.IsError {
		t.Fatalf("insert err: %s", res.Content)
	}
	// Delete the original first cell
	res, _ = tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "abc12345",
		"new_source":    "",
		"edit_mode":     "delete",
	})
	if res.IsError {
		t.Fatalf("delete err: %s", res.Content)
	}
}

func TestNotebookEditToolStructuredResult(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print('done')",
	})
	if res.IsError {
		t.Fatalf("tool error: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result JSON: %v", err)
	}
	if obj["cell_id"] != "def67890" || obj["notebook_path"] == "" || obj["original_file"] == "" || obj["updated_file"] == "" {
		t.Errorf("unexpected TS result payload: %v", obj)
	}
	wantKeys := []string{"cell_id", "cell_type", "edit_mode", "error", "language", "new_source", "notebook_path", "original_file", "updated_file"}
	gotKeys := make([]string, 0, len(obj))
	for key := range obj {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("model-visible result keys = %v, want TS keys %v", gotKeys, wantKeys)
	}
	if obj["error"] != "" {
		t.Fatalf("successful TS result must expose error as an empty string: %v", obj)
	}
}

func TestNotebookSplitSourceLines(t *testing.T) {
	got := splitSourceLines("a\nb\nc")
	want := []string{"a\n", "b\n", "c"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("idx %d: got %q want %q", i, got[i], want[i])
		}
	}
	if got := splitSourceLines(""); len(got) != 0 {
		t.Errorf("expected empty source to produce empty slice, got %v", got)
	}
}

func TestNotebookEditNoAttributionStamp(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print('z')",
	})
	if res.IsError {
		t.Fatalf("tool error: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "\"editedBy\"") || strings.Contains(string(data), "\"claude\"") {
		t.Errorf("TS parity path must not add attribution metadata: %s", string(data))
	}
}

func TestNotebookFixtureKernelspecPreservedAcrossDelete(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "abc12345",
		"new_source":    "",
		"edit_mode":     "delete",
	})
	if res.IsError {
		t.Fatalf("tool error: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "kernelspec") {
		t.Errorf("kernelspec must survive delete: %s", string(data))
	}
}

func TestApplyEditInsertWithoutAnchorAtTop(t *testing.T) {
	src := `{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	nb, _ := ParseNotebook([]byte(src))
	out, err := applyNotebookEdit(nb, NotebookEditOp{NewSource: "first", CellType: "code", EditMode: "insert"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(nb.Cells) != 1 || out.CellIndex != 0 {
		t.Errorf("expected single inserted cell at top")
	}
}

func TestNotebookEditToolReplaceMissingCell(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "missing",
		"new_source":    "x",
	})
	if !res.IsError || !strings.Contains(res.Content, "not found") {
		t.Errorf("expected Cell not found, got %q", res.Content)
	}
	var out NotebookEditResult
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("missing-cell error must use structured recovery payload: %v", err)
	}
	if out.Error == "" || out.OriginalFile != "" || out.UpdatedFile != "" {
		t.Fatalf("missing-cell recovery payload = %#v", out)
	}
}

func TestMarshalNotebookEditResult(t *testing.T) {
	r := NotebookEditResult{NewSource: "x", CellID: "abc", CellType: "code", Language: "python", EditMode: "replace", NotebookPath: "/n.ipynb", OriginalFile: "old", UpdatedFile: "new"}
	data, err := MarshalNotebookEditResult(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "\"edit_mode\":\"replace\"") || strings.Contains(string(data), "editMode") {
		t.Errorf("unexpected marshal output: %s", data)
	}
}

func TestNotebookEditToolUnknownMode(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"new_source":    "x",
		"edit_mode":     "explode",
	})
	if !res.IsError || !strings.Contains(res.Content, "edit_mode") {
		t.Errorf("expected edit_mode validation, got %q", res.Content)
	}
}

func TestApplyEditInsertWithUnknownAnchorErrors(t *testing.T) {
	nb, _ := ParseNotebook([]byte(fixtureNotebookJSON))
	if _, err := applyNotebookEdit(nb, NotebookEditOp{
		CellID:    "missing",
		NewSource: "x",
		EditMode:  "insert",
	}); err == nil {
		t.Error("expected error for unknown anchor")
	}
}

func TestNotebookEditToolStrictUnknownInput(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "x",
		"surprise":      true,
	})
	if !res.IsError || !strings.Contains(res.Content, "surprise") {
		t.Fatalf("expected strict unknown-field error, got isErr=%v content=%s", res.IsError, res.Content)
	}
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatalf("NotebookEdit schema should advertise additionalProperties=false")
	}
}

// TSref: z.strictObject requires both string-valued required properties and
// rejects null for every z.string field while still allowing new_source="".
func TestNotebookEditToolRequiredAndNullInputs(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	cases := []map[string]any{
		{"notebook_path": path, "cell_id": "def67890"},
		{"notebook_path": nil, "cell_id": "def67890", "new_source": "x"},
		{"notebook_path": path, "cell_id": nil, "new_source": "x"},
		{"notebook_path": path, "cell_id": "def67890", "new_source": nil},
		{"notebook_path": path, "cell_id": "def67890", "new_source": "x", "cell_type": nil},
		{"notebook_path": path, "cell_id": "def67890", "new_source": "x", "edit_mode": nil},
	}
	for _, input := range cases {
		res, _ := tool.Execute(context.Background(), input)
		if !res.IsError {
			t.Fatalf("expected strict string validation for %#v", input)
		}
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "",
	})
	if res.IsError {
		t.Fatalf("empty new_source is a valid z.string: %s", res.Content)
	}
}

// TSref: NotebookEdit delegates to the same write permission helper as the
// file mutation tools, including plan-mode denial and outside-dir asks.
func TestNotebookEditToolAllowedPathWritePermissionLifecycle(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.ipynb")
	out := filepath.Join(t.TempDir(), "outside.ipynb")
	tool := &NotebookEditTool{AllowedDirs: []string{root}}
	checker, ok := any(tool).(types.ToolPermissionChecker)
	if !ok {
		t.Fatal("NotebookEdit must participate in the write permission lifecycle")
	}
	req := types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{AllowedDirs: []string{root}}}
	decision, err := checker.CheckPermissions(context.Background(), map[string]any{"notebook_path": inside}, req)
	if err != nil || decision.Behavior != types.PermissionBehaviorPassthrough || len(decision.Suggestions) == 0 {
		t.Fatalf("inside write decision = %+v, err=%v", decision, err)
	}
	decision, err = checker.CheckPermissions(context.Background(), map[string]any{"notebook_path": out}, req)
	if err != nil || decision.Behavior != types.PermissionBehaviorAsk || decision.BlockedPath == "" {
		t.Fatalf("outside write decision = %+v, err=%v", decision, err)
	}

	plan := NewPlanState()
	plan.Enter("")
	tool.PlanState = plan
	decision, err = checker.CheckPermissions(context.Background(), map[string]any{"notebook_path": inside}, req)
	if err != nil || decision.Behavior != types.PermissionBehaviorDeny || decision.Message != toolPermissionText(i18n.KeyToolPermissionNotebookPlanMode) {
		t.Fatalf("plan-mode decision = %+v, err=%v", decision, err)
	}
}

func TestNotebookEditToolStaleRead(t *testing.T) {
	path := writeFixture(t)
	state := NewReadFileState()
	abs, _ := filepath.Abs(path)
	info, _ := os.Stat(path)
	state.Set(abs, ReadFileEntry{TimestampMs: info.ModTime().UnixMilli() - 1, Content: fixtureNotebookJSON})
	tool := &NotebookEditTool{ReadState: state}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "x",
	})
	if !res.IsError || !strings.Contains(res.Content, "modified since read") {
		t.Fatalf("expected stale read rejection, got isErr=%v content=%s", res.IsError, res.Content)
	}
}

func TestNotebookEditToolPostWriteReadStateRefresh(t *testing.T) {
	path := writeFixture(t)
	state := NewReadFileState()
	seedNotebookReadState(t, state, path)
	tool := &NotebookEditTool{ReadState: state}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print('one')",
	})
	if res.IsError {
		t.Fatalf("first edit failed: %s", res.Content)
	}
	abs, _ := filepath.Abs(path)
	entry, ok := state.Get(abs)
	if !ok || !strings.Contains(entry.Content, "print('one')") || entry.LastTool != "NotebookEdit" {
		t.Fatalf("read state not refreshed: ok=%v entry=%#v", ok, entry)
	}
	res, _ = tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print('two')",
	})
	if res.IsError {
		t.Fatalf("second edit should use refreshed read state: %s", res.Content)
	}
}

func TestNotebookEditToolAllowedPathSymlinkAndRelative(t *testing.T) {
	dir := t.TempDir()
	allowed := filepath.Join(dir, "allowed")
	outside := filepath.Join(dir, "outside")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	insidePath := filepath.Join(allowed, "fixture.ipynb")
	if err := os.WriteFile(insidePath, []byte(fixtureNotebookJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(allowed)
	state := NewReadFileState()
	seedNotebookReadState(t, state, "fixture.ipynb")
	tool := &NotebookEditTool{AllowedDirs: []string{allowed}, ReadState: state}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": "fixture.ipynb",
		"cell_id":       "def67890",
		"new_source":    "print('relative')",
	})
	if res.IsError {
		t.Fatalf("relative path inside allowed dir should pass: %s", res.Content)
	}

	outsidePath := filepath.Join(outside, "outside.ipynb")
	if err := os.WriteFile(outsidePath, []byte(fixtureNotebookJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _ = tool.Execute(context.Background(), map[string]any{
		"notebook_path": outsidePath,
		"cell_id":       "def67890",
		"new_source":    "x",
	})
	if !res.IsError || !strings.Contains(res.Content, "outside allowed directories") {
		t.Fatalf("expected outside allowed dirs rejection, got %s", res.Content)
	}

	linkPath := filepath.Join(allowed, "escape.ipynb")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	res, _ = tool.Execute(context.Background(), map[string]any{
		"notebook_path": linkPath,
		"cell_id":       "def67890",
		"new_source":    "x",
	})
	if !res.IsError || !strings.Contains(res.Content, "outside allowed directories") {
		t.Fatalf("expected symlink escape rejection, got %s", res.Content)
	}
}

func TestNotebookRoundtripUnknownSourceAndCellID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.ipynb")
	body := `{"nbformat":4,"nbformat_minor":5,"metadata":{"language_info":{"name":"python"},"keep":true},"custom_top":{"x":1},"cells":[{"cell_type":"markdown","metadata":{"m":1},"attachments":{"a":{}},"source":["old"]}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "cell-0",
		"new_source":    "new markdown",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	var raw map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["custom_top"]; !ok {
		t.Fatalf("unknown top-level field lost: %s", data)
	}
	cell := raw["cells"].([]any)[0].(map[string]any)
	if _, ok := cell["attachments"]; !ok {
		t.Fatalf("unknown cell field lost: %v", cell)
	}
	if _, ok := cell["id"]; ok {
		t.Fatalf("existing id-less cell should remain id-less: %v", cell)
	}
	if _, ok := cell["source"].(string); !ok {
		t.Fatalf("edited source should serialize as string, got %T %v", cell["source"], cell["source"])
	}
}

// TSref: NotebookEdit parses and mutates the whole JSON object in place;
// untouched null-valued source/output fields therefore remain null.
func TestNotebookRoundtripPreservesUntouchedNullFields(t *testing.T) {
	src := `{"cells":[{"cell_type":"code","metadata":{},"source":null,"outputs":null,"execution_count":null},{"cell_type":"markdown","metadata":{},"source":"old"}],"metadata":{},"nbformat":4,"nbformat_minor":4}`
	nb, err := ParseNotebook([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applyNotebookEdit(nb, NotebookEditOp{CellID: "cell-1", NewSource: "new", EditMode: "replace"}); err != nil {
		t.Fatal(err)
	}
	out, err := SerializeNotebook(nb)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Cells []map[string]any `json:"cells"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"source", "outputs", "execution_count"} {
		value, exists := decoded.Cells[0][key]
		if !exists || value != nil {
			t.Fatalf("untouched %s = %#v (exists=%v), want explicit null; JSON=%s", key, value, exists, out)
		}
	}
}

func TestNotebookRoundtripPreservesZeroFormatMinor(t *testing.T) {
	src := `{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":0}`
	nb, err := ParseNotebook([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if nb.NBFormatMinor != 0 {
		t.Fatalf("parse changed nbformat_minor=0 to %d", nb.NBFormatMinor)
	}
	out, err := SerializeNotebook(nb)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["nbformat_minor"] != float64(0) {
		t.Fatalf("serialized nbformat_minor = %v, want 0", decoded["nbformat_minor"])
	}
}

func TestNotebookExecutionCountOutputsSerialize(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print('cleared')",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	var raw map[string]any
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	cell := raw["cells"].([]any)[1].(map[string]any)
	if _, ok := cell["source"].(string); !ok {
		t.Fatalf("source should be string after edit: %v", cell)
	}
	if v, ok := cell["execution_count"]; !ok || v != nil {
		t.Fatalf("execution_count should serialize as null: %v", cell)
	}
	outputs, ok := cell["outputs"].([]any)
	if !ok || len(outputs) != 0 {
		t.Fatalf("outputs should serialize as []: %v", cell)
	}
}

func TestNotebookExecutionCountOutputsInsertedCodeAndMarkdownFieldShapes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "insert.ipynb")
	body := `{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"new_source":    "print('new')",
		"cell_type":     "code",
		"edit_mode":     "insert",
	})
	if res.IsError {
		t.Fatalf("code insert failed: %s", res.Content)
	}
	res, _ = tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"new_source":    "# heading",
		"cell_type":     "markdown",
		"edit_mode":     "insert",
	})
	if res.IsError {
		t.Fatalf("markdown insert failed: %s", res.Content)
	}
	var decoded struct {
		Cells []map[string]any `json:"cells"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	markdown, code := decoded.Cells[0], decoded.Cells[1]
	if _, ok := markdown["execution_count"]; ok {
		t.Fatalf("markdown insert gained execution_count: %v", markdown)
	}
	if _, ok := markdown["outputs"]; ok {
		t.Fatalf("markdown insert gained outputs: %v", markdown)
	}
	if value, ok := code["execution_count"]; !ok || value != nil {
		t.Fatalf("code execution_count = %#v (present=%v), want null", value, ok)
	}
	if outputs, ok := code["outputs"].([]any); !ok || len(outputs) != 0 {
		t.Fatalf("code outputs = %#v, want []", code["outputs"])
	}
}

func TestNotebookLineEndingEncodingAndFileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "encoded.ipynb")
	body := strings.ReplaceAll(fixtureNotebookJSON, "\n", "\r\n")
	raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte(body)...)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print('mode')",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	after, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(after), "\xEF\xBB\xBF") {
		t.Fatalf("UTF-8 BOM not preserved: % x", after[:3])
	}
	if !strings.Contains(string(after), "\r\n") {
		t.Fatalf("CRLF line endings not preserved: %q", string(after[:80]))
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode not preserved: %v", info.Mode().Perm())
	}
}

// TSref: detectLineEndingsForString selects CRLF only when CRLF strictly
// outnumbers bare LF. A tie must therefore serialize with LF.
func TestNotebookLineEndingTieUsesLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.ipynb")
	body := "{\r\n\"cells\":[{\"cell_type\":\"code\",\"id\":\"c\",\"metadata\":{},\"source\":\"old\",\"outputs\":[],\"execution_count\":null}],\n\"metadata\":{},\"nbformat\":4,\"nbformat_minor\":5}"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{"notebook_path": path, "cell_id": "c", "new_source": "new"})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "\r\n") {
		t.Fatalf("mixed-ending tie must use LF like TS: %q", out[:min(80, len(out))])
	}
}

func TestNotebookEncodingUTF16LEPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "utf16.ipynb")
	raw := encodeWriteBytes(fixtureNotebookJSON, EncodingUTF16LE, bomUTF16LE)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print('utf16')",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), string(bomUTF16LE)) {
		t.Fatalf("UTF-16LE BOM not preserved: % x", after[:min(4, len(after))])
	}
	decoded := decodeFileBytes(after, detectFileEncoding(after))
	if !strings.Contains(decoded, "print('utf16')") {
		t.Fatalf("UTF-16LE content not preserved: %q", decoded)
	}
}

func TestNotebookHistoryOriginalUpdatedPayload(t *testing.T) {
	path := writeFixture(t)
	state := NewReadFileState()
	seedNotebookReadState(t, state, path)
	store := NewFileHistoryStore(filepath.Join(t.TempDir(), "history"))
	tool := &NotebookEditTool{ReadState: state, HistoryStore: store}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print('history')",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	var out NotebookEditResult
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatal(err)
	}
	if out.OriginalFile == "" || out.UpdatedFile == "" || !strings.Contains(out.UpdatedFile, "history") {
		t.Fatalf("original/updated payload missing: %#v", out)
	}
	entries, err := store.ListEdits(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Tool != "NotebookEdit" || !strings.Contains(entries[0].After, "history") {
		t.Fatalf("history entry mismatch: %#v", entries)
	}
}

func TestNotebookEditResultErrorShapeInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.ipynb")
	if err := os.WriteFile(path, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "cell-0",
		"new_source":    "x",
	})
	if !res.IsError {
		t.Fatalf("expected invalid JSON error")
	}
	var out NotebookEditResult
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatal(err)
	}
	if out.Error == "" || out.NotebookPath == "" {
		t.Fatalf("error payload missing TS fields: %#v", out)
	}
}

func TestNotebookEditResultOutputMapping(t *testing.T) {
	tool := &NotebookEditTool{}
	for _, tc := range []struct {
		mode string
		want string
	}{
		{mode: "replace", want: "Updated cell c with x"},
		{mode: "insert", want: "Inserted cell c with x"},
		{mode: "delete", want: "Deleted cell c"},
	} {
		block := tool.MapToolResultToToolResultBlock(NotebookEditResult{CellID: "c", NewSource: "x", EditMode: tc.mode}, "toolu_1")
		if block.Type != types.ContentTypeToolResult || block.ToolUseID != "toolu_1" || block.IsError || block.Content != tc.want {
			t.Fatalf("%s mapping = %+v, want %q tool_result", tc.mode, block, tc.want)
		}
	}
	block := tool.MapToolResultToToolResultBlock(NotebookEditResult{Error: "failed"}, "toolu_2")
	if block.Type != types.ContentTypeToolResult || !block.IsError || block.Content != "failed" {
		t.Fatalf("error mapping = %+v", block)
	}
}

func TestNotebookEditToolWriteErrorPayload(t *testing.T) {
	path := writeFixture(t)
	state := NewReadFileState()
	seedNotebookReadState(t, state, path)
	tool := &NotebookEditTool{ReadState: state}
	if runtime.GOOS == "windows" {
		t.Skip("directory write permissions are not deterministic on Windows")
	}
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"new_source":    "x",
		"cell_type":     "code",
		"edit_mode":     "insert",
	})
	if !res.IsError {
		t.Fatal("expected deterministic write failure")
	}
	var out NotebookEditResult
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("write error should be JSON payload: %v; content=%s", err, res.Content)
	}
	if out.Error == "" || out.NotebookPath == "" || out.EditMode != "replace" || out.OriginalFile != "" || out.UpdatedFile != "" {
		t.Fatalf("write recovery payload mismatch: %#v", out)
	}
}

// TSref: new_cell_id is the requested reference for replace/delete only when
// the notebook format supports cell IDs; positional references stay visible.
func TestNotebookEditResultCellIDFollowsTSFormatGate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		minor      int
		cellID     string
		target     string
		wantCellID string
	}{
		{name: "idless positional 4.5", minor: 5, target: "cell-0", wantCellID: "cell-0"},
		{name: "actual id 4.4 omitted", minor: 4, cellID: "legacy", target: "legacy", wantCellID: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "fixture.ipynb")
			body := `{"cells":[{"cell_type":"code","metadata":{},"source":"old","outputs":[],"execution_count":null` + func() string {
				if tc.cellID == "" {
					return ""
				}
				return `,"id":"` + tc.cellID + `"`
			}() + `}],"metadata":{},"nbformat":4,"nbformat_minor":` + fmt.Sprint(tc.minor) + `}`
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			tool := newSeededNotebookEditTool(t, path)
			res, _ := tool.Execute(context.Background(), map[string]any{"notebook_path": path, "cell_id": tc.target, "new_source": "new"})
			if res.IsError {
				t.Fatalf("edit failed: %s", res.Content)
			}
			var out NotebookEditResult
			if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
				t.Fatal(err)
			}
			if out.CellID != tc.wantCellID {
				t.Fatalf("cell_id = %q, want %q; payload=%s", out.CellID, tc.wantCellID, res.Content)
			}
		})
	}
}

func TestNotebookEditToolMissingCellIDAndCellTarget(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	for _, input := range []map[string]any{
		{"notebook_path": path, "new_source": "x", "edit_mode": "replace"},
		{"notebook_path": path, "new_source": "x", "cell_id": "last", "edit_mode": "replace"},
		{"notebook_path": path, "new_source": "x", "cell_id": "0", "edit_mode": "replace"},
		{"notebook_path": path, "new_source": "x", "cell_id": "cell-+1", "edit_mode": "replace"},
		{"notebook_path": path, "new_source": "x", "cell_id": "cell-99", "edit_mode": "replace"},
	} {
		res, _ := tool.Execute(context.Background(), input)
		if !res.IsError {
			t.Fatalf("expected target validation error for %#v", input)
		}
	}
}

func TestNotebookEditToolInsertAtBeginningAndAfterTarget(t *testing.T) {
	path := writeFixture(t)
	tool := newSeededNotebookEditTool(t, path)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"new_source":    "# first",
		"cell_type":     "markdown",
		"edit_mode":     "insert",
	})
	if res.IsError {
		t.Fatalf("insert at beginning failed: %s", res.Content)
	}
	res, _ = tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "print('after')",
		"cell_type":     "code",
		"edit_mode":     "insert",
	})
	if res.IsError {
		t.Fatalf("insert after target failed: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# first") || !strings.Contains(string(data), "after") {
		t.Fatalf("inserted sources missing: %s", data)
	}
}

func TestNotebookEditToolReadBeforeEditUnread(t *testing.T) {
	path := writeFixture(t)
	tool := &NotebookEditTool{ReadState: NewReadFileState()}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "x",
	})
	if !res.IsError || !strings.Contains(res.Content, "not been read") {
		t.Fatalf("expected unread rejection, got %s", res.Content)
	}
}

func TestNotebookEditToolStaleReadByFutureMtime(t *testing.T) {
	path := writeFixture(t)
	state := NewReadFileState()
	seedNotebookReadState(t, state, path)
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	tool := &NotebookEditTool{ReadState: state}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"notebook_path": path,
		"cell_id":       "def67890",
		"new_source":    "x",
	})
	if !res.IsError || !strings.Contains(res.Content, "modified since read") {
		t.Fatalf("expected stale rejection, got %s", res.Content)
	}
}
