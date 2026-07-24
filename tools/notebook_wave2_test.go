// Package tools — notebook_wave2_test.go: regression tests for the wave-2
// notebook alignment fixes. Each test corresponds to one task in
// tasks/waves2/I_edit_notebook.json.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNotebookWave2_NBFormatCellIDGate — ne-nbformat-cellid-gate.
// Parsing existing cells must not synthesize persistent id fields. TS only
// generates ids for newly inserted cells when the notebook format supports it.
func TestNotebookWave2_NBFormatCellIDGate(t *testing.T) {
	older := []byte(`{"nbformat":4,"nbformat_minor":4,"metadata":{},"cells":[{"cell_type":"code","source":["x=1"]}]}`)
	nb, err := ParseNotebook(older)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(nb.Cells) != 1 {
		t.Fatalf("expected 1 cell, got %d", len(nb.Cells))
	}
	if nb.Cells[0].ID != "" {
		t.Fatalf("expected no id on nbformat 4.4 cell, got %q", nb.Cells[0].ID)
	}

	newer := []byte(`{"nbformat":4,"nbformat_minor":5,"metadata":{},"cells":[{"cell_type":"code","source":["x=1"]}]}`)
	nb2, err := ParseNotebook(newer)
	if err != nil {
		t.Fatalf("parse newer: %v", err)
	}
	if nb2.Cells[0].ID != "" {
		t.Fatalf("expected existing nbformat 4.5 id-less cell to remain id-less, got %q", nb2.Cells[0].ID)
	}
}

// TestNotebookWave2_CellIDFormat — ne-cellid-format-divergence.
// generateCellID must return a 13-char base36-charset id, matching the TS
// Math.random().toString(36).substring(2,15) shape.
func TestNotebookWave2_CellIDFormat(t *testing.T) {
	for i := 0; i < 32; i++ {
		id := generateCellID()
		if len(id) != 13 {
			t.Fatalf("expected 13-char id, got %d (%q)", len(id), id)
		}
		for _, r := range id {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')) {
				t.Fatalf("non-base36 char %q in id %q", r, id)
			}
		}
	}
}

// TestNotebookWave2_OutputsClearedOnSameTypeReplace verifies TS behavior:
// editing a code cell source resets execution_count and clears outputs.
func TestNotebookWave2_OutputsClearedOnSameTypeReplace(t *testing.T) {
	count := 5
	nb := &Notebook{
		NBFormat:      4,
		NBFormatMinor: 5,
		Metadata:      map[string]any{},
		Cells: []Cell{{
			ID:             "c1",
			CellType:       "code",
			Source:         []string{"old"},
			Metadata:       map[string]any{},
			Outputs:        []any{map[string]any{"output_type": "stream", "text": []string{"hi"}}},
			ExecutionCount: &count,
		}},
	}
	out, err := applyNotebookEdit(nb, NotebookEditOp{CellID: "c1", NewSource: "new", EditMode: "replace"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if out.EditMode != "replace" {
		t.Fatalf("expected replace, got %q", out.EditMode)
	}
	cell := nb.Cells[0]
	if len(cell.Outputs) != 0 {
		t.Fatalf("expected outputs cleared on same-type source replace, got %d", len(cell.Outputs))
	}
	if !cell.HasExecutionCount || cell.ExecutionCount != nil {
		t.Fatalf("expected execution_count:null, got has=%v value=%v", cell.HasExecutionCount, cell.ExecutionCount)
	}
}

// TestNotebookWave2_ParseCellIDNumericFallback — ne-parsecellid-numeric-fallback.
// Resolving `cell-N` should map to the 0-indexed Nth cell.
func TestNotebookWave2_ParseCellIDNumericFallback(t *testing.T) {
	nb := &Notebook{
		NBFormat: 4, NBFormatMinor: 5,
		Cells: []Cell{
			{ID: "first", CellType: "code", Source: []string{"a"}},
			{ID: "second", CellType: "code", Source: []string{"b"}},
			{ID: "third", CellType: "code", Source: []string{"c"}},
		},
	}
	_, idx, err := ResolveCell(nb, "cell-1")
	if err != nil {
		t.Fatalf("resolve cell-1: %v", err)
	}
	if idx != 1 {
		t.Fatalf("expected idx 1 for cell-1, got %d", idx)
	}
	_, idx, err = ResolveCell(nb, "cell-0")
	if err != nil {
		t.Fatalf("resolve cell-0: %v", err)
	}
	if idx != 0 {
		t.Fatalf("expected idx 0 for cell-0, got %d", idx)
	}
}

// TestNotebookWave2_ReplacePastEndRejected rejects out-of-range `cell-N`
// targets during validation instead of silently promoting replace to insert.
func TestNotebookWave2_ReplacePastEndRejected(t *testing.T) {
	nb := &Notebook{
		NBFormat: 4, NBFormatMinor: 5,
		Cells: []Cell{
			{ID: "a", CellType: "code", Source: []string{"a"}},
			{ID: "b", CellType: "code", Source: []string{"b"}},
		},
	}
	if _, err := applyNotebookEdit(nb, NotebookEditOp{CellID: "cell-2", NewSource: "appended", EditMode: "replace"}); err == nil {
		t.Fatalf("expected out-of-range cell-N replace to be rejected")
	}
}

// TestNotebookWave2_ReadBeforeEditAlwaysOn — ne-read-before-edit-gate.
// A bare &NotebookEditTool{} must reject a notebook that has not been read.
func TestNotebookWave2_ReadBeforeEditAlwaysOn(t *testing.T) {
	dir := t.TempDir()
	nbPath := filepath.Join(dir, "x.ipynb")
	body := `{"nbformat":4,"nbformat_minor":5,"metadata":{},"cells":[{"id":"a","cell_type":"code","source":["x=1"]}]}`
	if err := os.WriteFile(nbPath, []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Use an isolated read state so the global default doesn't leak between
	// test runs that may have read the same path.
	tool := &NotebookEditTool{ReadState: NewReadFileState()}
	res, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": nbPath,
		"cell_id":       "a",
		"new_source":    "x=2",
		"edit_mode":     "replace",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error for unread notebook, got %s", res.Content)
	}
	if !strings.Contains(res.Content, "has not been read") {
		t.Fatalf("expected 'has not been read' in error, got %s", res.Content)
	}
}

// TestNotebookWave2_RawCellTypeRejected — ne-schema-raw-celltype.
// Execute must reject cell_type='raw' to remove the schema/runtime
// contradiction.
func TestNotebookWave2_RawCellTypeRejected(t *testing.T) {
	dir := t.TempDir()
	nbPath := filepath.Join(dir, "x.ipynb")
	body := `{"nbformat":4,"nbformat_minor":5,"metadata":{},"cells":[{"id":"a","cell_type":"code","source":["x"]}]}`
	if err := os.WriteFile(nbPath, []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	state := NewReadFileState()
	abs, _ := filepath.Abs(nbPath)
	info, err := os.Stat(nbPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	state.Set(filepath.Clean(abs), ReadFileEntry{TimestampMs: info.ModTime().UnixMilli(), Content: body})
	tool := &NotebookEditTool{ReadState: state}
	res, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": nbPath,
		"cell_id":       "a",
		"cell_type":     "raw",
		"new_source":    "x=2",
		"edit_mode":     "replace",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "invalid cell_type") {
		t.Fatalf("expected invalid cell_type error, got %s", res.Content)
	}
}

// TestNotebookWave2_LargeNotebookNoJqHint verifies NotebookEdit does not add
// a Go-only size refusal before the TS parse/edit/write path.
func TestNotebookWave2_LargeNotebookNoJqHint(t *testing.T) {
	const removedNotebookGuardSize = 32 * 1024 * 1024
	dir := t.TempDir()
	nbPath := filepath.Join(dir, "big.ipynb")
	// Compose a notebook bigger than the cap by padding a single cell's
	// source with junk.
	cells := []map[string]any{{
		"id":        "a",
		"cell_type": "code",
		"source":    []string{strings.Repeat("x", removedNotebookGuardSize+1024)},
	}}
	body, _ := json.Marshal(map[string]any{
		"nbformat": 4, "nbformat_minor": 5,
		"metadata": map[string]any{}, "cells": cells,
	})
	if err := os.WriteFile(nbPath, body, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	state := NewReadFileState()
	abs, _ := filepath.Abs(nbPath)
	info, err := os.Stat(nbPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	state.Set(filepath.Clean(abs), ReadFileEntry{TimestampMs: info.ModTime().UnixMilli(), Content: string(body)})
	tool := &NotebookEditTool{ReadState: state}
	res, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": nbPath,
		"cell_id":       "a",
		"new_source":    "x=2",
		"edit_mode":     "replace",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("large notebook should not be rejected by a jq hint path: %s", res.Content)
	}
	if strings.Contains(res.Content, "jq") {
		t.Fatalf("unexpected jq hint in result: %s", res.Content)
	}
}
