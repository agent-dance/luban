package file

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// NotebookEditOp describes a single edit operation to apply to a notebook.
type NotebookEditOp struct {
	CellID    string
	NewSource string
	CellType  string // optional: "code" / "markdown" / "raw"
	EditMode  string // "replace" (default) | "insert" | "delete"
}

// NotebookEditOutcome describes the resulting state after applying an edit.
type NotebookEditOutcome struct {
	EditMode  string
	CellID    string // resulting cell id (or original for delete)
	CellIndex int
	Before    *Cell // copy of cell before the edit (nil for insert / new cells)
	After     *Cell // copy of cell after the edit (nil for delete)
}

// ApplyEdit applies an edit operation to nb and returns an outcome. nb is
// mutated in place. Mirrors the NotebookEdit mutation in the TS tool:
//   - replace: updates source and optionally cell_type; an existing code
//     cell always gets execution_count:null and outputs:[].
//   - insert: places a new cell after the targeted cell, or at index 0 if no
//     cell_id is given.
//   - delete: removes the targeted cell entirely (and its outputs).
func applyNotebookEdit(nb *Notebook, op NotebookEditOp) (*NotebookEditOutcome, error) {
	if nb == nil {
		return nil, i18n.NewError(i18n.KeyToolNotebookHelperNilNotebook)
	}
	mode := op.EditMode
	if mode == "" {
		mode = "replace"
	}

	switch mode {
	case "replace":
		return applyReplace(nb, op)
	case "insert":
		return applyInsert(nb, op)
	case "delete":
		return applyDelete(nb, op)
	default:
		return nil, i18n.NewError(i18n.KeyToolNotebookHelperUnknownEditMode, op.EditMode)
	}
}

func applyReplace(nb *Notebook, op NotebookEditOp) (*NotebookEditOutcome, error) {
	if op.CellID == "" {
		return nil, i18n.NewError(i18n.KeyToolNotebookHelperCellIDRequired, "replace")
	}
	idx := findCellIndexByRef(nb, op.CellID)
	if idx < 0 {
		return nil, i18n.NewError(i18n.KeyToolNotebookHelperCellNotFound, op.CellID)
	}
	before := cloneCell(nb.Cells[idx])

	cell := &nb.Cells[idx]
	prevType := cell.CellType
	newType := op.CellType
	if newType == "" {
		newType = prevType
	}

	cell.Source = splitSourceLines(op.NewSource)
	cell.HasSource = true
	cell.SourceAsString = true
	cell.SourceWasNull = false

	if prevType == "code" {
		cell.ExecutionCount = nil
		cell.HasExecutionCount = true
		cell.Outputs = []any{}
		cell.HasOutputs = true
		cell.OutputsWasNull = false
	}
	cell.CellType = newType

	after := cloneCell(*cell)
	return &NotebookEditOutcome{
		EditMode:  "replace",
		CellID:    cell.ID,
		CellIndex: idx,
		Before:    &before,
		After:     &after,
	}, nil
}

func applyInsert(nb *Notebook, op NotebookEditOp) (*NotebookEditOutcome, error) {
	cellType := op.CellType
	if cellType == "" {
		cellType = "code"
	}

	// Cell IDs are only emitted when the notebook format supports them
	// (4.5+). On 4.4 and earlier classic Jupyter Lab may strip or reject
	// the field, so we omit it entirely.
	id := ""
	if notebookSupportsCellID(nb.NBFormat, nb.NBFormatMinor) {
		id = generateCellID()
	}
	newCell := Cell{
		ID:             id,
		CellType:       cellType,
		Source:         splitSourceLines(op.NewSource),
		HasSource:      true,
		SourceAsString: true,
		Metadata:       map[string]any{},
	}
	if cellType == "code" {
		newCell.Outputs = []any{}
		newCell.HasOutputs = true
		newCell.OutputsWasNull = false
		newCell.ExecutionCount = nil
		newCell.HasExecutionCount = true
	}

	// Insert position
	insertAt := 0
	if op.CellID != "" {
		idx := findCellIndexByRef(nb, op.CellID)
		if idx < 0 {
			return nil, i18n.NewError(i18n.KeyToolNotebookHelperCellNotFound, op.CellID)
		}
		insertAt = idx + 1
	}
	if insertAt > len(nb.Cells) {
		insertAt = len(nb.Cells)
	}

	nb.Cells = append(nb.Cells, Cell{})
	copy(nb.Cells[insertAt+1:], nb.Cells[insertAt:])
	nb.Cells[insertAt] = newCell

	after := cloneCell(nb.Cells[insertAt])
	return &NotebookEditOutcome{
		EditMode:  "insert",
		CellID:    newCell.ID,
		CellIndex: insertAt,
		After:     &after,
	}, nil
}

func applyDelete(nb *Notebook, op NotebookEditOp) (*NotebookEditOutcome, error) {
	if op.CellID == "" {
		return nil, i18n.NewError(i18n.KeyToolNotebookHelperCellIDRequired, "delete")
	}
	idx := findCellIndexByRef(nb, op.CellID)
	if idx < 0 {
		return nil, i18n.NewError(i18n.KeyToolNotebookHelperCellNotFound, op.CellID)
	}
	before := cloneCell(nb.Cells[idx])

	nb.Cells = append(nb.Cells[:idx], nb.Cells[idx+1:]...)

	return &NotebookEditOutcome{
		EditMode:  "delete",
		CellID:    before.ID,
		CellIndex: idx,
		Before:    &before,
	}, nil
}

func cloneCell(c Cell) Cell {
	out := Cell{
		ID:                c.ID,
		CellType:          c.CellType,
		SourceAsString:    c.SourceAsString,
		HasSource:         c.HasSource,
		SourceWasNull:     c.SourceWasNull,
		HasOutputs:        c.HasOutputs,
		OutputsWasNull:    c.OutputsWasNull,
		HasExecutionCount: c.HasExecutionCount,
	}
	if c.Source != nil {
		out.Source = append([]string(nil), c.Source...)
	}
	if c.Metadata != nil {
		out.Metadata = make(map[string]any, len(c.Metadata))
		for k, v := range c.Metadata {
			out.Metadata[k] = v
		}
	}
	if c.Outputs != nil {
		out.Outputs = append([]any(nil), c.Outputs...)
	}
	if c.ExecutionCount != nil {
		v := *c.ExecutionCount
		out.ExecutionCount = &v
	}
	if c.Extra != nil {
		out.Extra = make(map[string]any, len(c.Extra))
		for k, v := range c.Extra {
			out.Extra[k] = v
		}
	}
	return out
}

// joinSource flattens cell.Source into a single string.
func joinSource(src []string) string {
	return strings.Join(src, "")
}
