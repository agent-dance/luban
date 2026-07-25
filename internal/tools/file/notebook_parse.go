package file

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"strconv"

	"github.com/agent-dance/luban/i18n"
)

// ─── Notebook parser ────────────────────────────────────────────────────────
//
// Source can be a string or an array of strings on disk. The editable view is
// []string, while representation flags preserve untouched JSON values during
// the NotebookEdit round trip.

// Notebook is the in-memory representation of a Jupyter notebook.
type Notebook struct {
	NBFormat      int            `json:"nbformat"`
	NBFormatMinor int            `json:"nbformat_minor"`
	Metadata      map[string]any `json:"metadata"`
	Cells         []Cell         `json:"cells"`
	Extra         map[string]any `json:"-"`
}

// Cell is the in-memory representation of a single notebook cell.
type Cell struct {
	ID                string         `json:"id,omitempty"`
	CellType          string         `json:"cell_type"`
	Source            []string       `json:"source"`
	Metadata          map[string]any `json:"metadata"`
	Outputs           []any          `json:"outputs,omitempty"`
	ExecutionCount    *int           `json:"execution_count,omitempty"`
	HasSource         bool           `json:"-"`
	SourceAsString    bool           `json:"-"`
	SourceWasNull     bool           `json:"-"`
	HasOutputs        bool           `json:"-"`
	OutputsWasNull    bool           `json:"-"`
	HasExecutionCount bool           `json:"-"`
	Extra             map[string]any `json:"-"`
}

// rawNotebook is used during JSON unmarshalling so we can normalise `source`
// without losing fields we don't model explicitly.
type rawNotebook struct {
	NBFormat      int            `json:"nbformat"`
	NBFormatMinor int            `json:"nbformat_minor"`
	Metadata      map[string]any `json:"metadata"`
	Cells         []rawCell      `json:"cells"`
	Extra         map[string]any `json:"-"`
}

type rawCell struct {
	ID                string          `json:"id,omitempty"`
	CellType          string          `json:"cell_type"`
	Source            json.RawMessage `json:"source"`
	Metadata          map[string]any  `json:"metadata"`
	Outputs           []any           `json:"outputs,omitempty"`
	ExecutionCount    *int            `json:"execution_count,omitempty"`
	HasSource         bool            `json:"-"`
	SourceWasNull     bool            `json:"-"`
	HasOutputs        bool            `json:"-"`
	OutputsWasNull    bool            `json:"-"`
	HasExecutionCount bool            `json:"-"`
	Extra             map[string]any  `json:"-"`
}

func (r *rawNotebook) UnmarshalJSON(data []byte) error {
	type alias rawNotebook
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	var extra map[string]any
	if err := json.Unmarshal(data, &extra); err != nil {
		return err
	}
	delete(extra, "nbformat")
	delete(extra, "nbformat_minor")
	delete(extra, "metadata")
	delete(extra, "cells")
	*r = rawNotebook(a)
	r.Extra = extra
	return nil
}

func (r *rawCell) UnmarshalJSON(data []byte) error {
	type alias rawCell
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	var extra map[string]any
	if err := json.Unmarshal(data, &extra); err != nil {
		return err
	}
	delete(extra, "id")
	delete(extra, "cell_type")
	sourceValue, hasSource := extra["source"]
	delete(extra, "source")
	delete(extra, "metadata")
	outputsValue, hasOutputs := extra["outputs"]
	delete(extra, "outputs")
	_, hasExecutionCount := extra["execution_count"]
	delete(extra, "execution_count")
	*r = rawCell(a)
	r.HasSource = hasSource
	r.SourceWasNull = hasSource && sourceValue == nil
	r.HasOutputs = hasOutputs
	r.OutputsWasNull = hasOutputs && outputsValue == nil
	r.HasExecutionCount = hasExecutionCount
	r.Extra = extra
	return nil
}

// ParseNotebook parses raw .ipynb bytes into a Notebook.
func ParseNotebook(data []byte) (*Notebook, error) {
	var raw rawNotebook
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, i18n.WrapError(i18n.KeyToolNotebookHelperInvalidJSON, err)
	}

	nb := &Notebook{
		NBFormat:      raw.NBFormat,
		NBFormatMinor: raw.NBFormatMinor,
		Metadata:      raw.Metadata,
		Extra:         raw.Extra,
	}
	if nb.Metadata == nil {
		nb.Metadata = map[string]any{}
	}
	nb.Cells = make([]Cell, 0, len(raw.Cells))
	for _, rc := range raw.Cells {
		cell := Cell{
			ID:                rc.ID,
			CellType:          rc.CellType,
			Source:            coerceSourceFromJSON(rc.Source),
			Metadata:          rc.Metadata,
			Outputs:           rc.Outputs,
			ExecutionCount:    rc.ExecutionCount,
			HasSource:         rc.HasSource,
			SourceAsString:    sourceWasString(rc.Source),
			SourceWasNull:     rc.SourceWasNull,
			HasOutputs:        rc.HasOutputs,
			OutputsWasNull:    rc.OutputsWasNull,
			HasExecutionCount: rc.HasExecutionCount,
			Extra:             rc.Extra,
		}
		if cell.Metadata == nil {
			cell.Metadata = map[string]any{}
		}
		if cell.CellType == "" {
			cell.CellType = "code"
		}
		nb.Cells = append(nb.Cells, cell)
	}
	return nb, nil
}

// SerializeNotebook turns a Notebook into pretty-printed .ipynb bytes.
func SerializeNotebook(nb *Notebook) ([]byte, error) {
	if nb == nil {
		return nil, i18n.NewError(i18n.KeyToolNotebookHelperNilNotebook)
	}
	if nb.Metadata == nil {
		nb.Metadata = map[string]any{}
	}
	if nb.Cells == nil {
		nb.Cells = []Cell{}
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", " ")
	if err := encoder.Encode(nb); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func (nb Notebook) MarshalJSON() ([]byte, error) {
	out := cloneMap(nb.Extra)
	out["cells"] = nb.Cells
	if nb.Metadata == nil {
		out["metadata"] = map[string]any{}
	} else {
		out["metadata"] = nb.Metadata
	}
	out["nbformat"] = nb.NBFormat
	out["nbformat_minor"] = nb.NBFormatMinor
	return json.Marshal(out)
}

func (c Cell) MarshalJSON() ([]byte, error) {
	out := cloneMap(c.Extra)
	if c.ID != "" {
		out["id"] = c.ID
	}
	out["cell_type"] = c.CellType
	if c.HasSource || c.Source != nil || c.SourceAsString || c.SourceWasNull {
		switch {
		case c.SourceWasNull:
			out["source"] = nil
		case c.SourceAsString:
			out["source"] = joinSource(c.Source)
		default:
			out["source"] = c.Source
		}
	}
	if c.Metadata == nil {
		out["metadata"] = map[string]any{}
	} else {
		out["metadata"] = c.Metadata
	}
	if c.HasExecutionCount || c.ExecutionCount != nil {
		if c.ExecutionCount == nil {
			out["execution_count"] = nil
		} else {
			out["execution_count"] = *c.ExecutionCount
		}
	}
	if c.HasOutputs || c.Outputs != nil {
		if c.OutputsWasNull {
			out["outputs"] = nil
		} else if c.Outputs == nil {
			out["outputs"] = []any{}
		} else {
			out["outputs"] = c.Outputs
		}
	}
	return json.Marshal(out)
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+4)
	for k, v := range in {
		out[k] = v
	}
	return out
}

// coerceSourceFromJSON normalises the `source` field which may be either a
// string or an array of strings on disk.
func coerceSourceFromJSON(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}
	}
	// Try array first.
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	// Then string.
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return splitSourceLines(str)
	}
	return []string{}
}

func sourceWasString(raw json.RawMessage) bool {
	var str string
	return len(raw) > 0 && json.Unmarshal(raw, &str) == nil
}

// notebookSupportsCellID returns true when the notebook format includes
// per-cell `id` fields. Cell IDs were introduced in nbformat 4.5; on 4.4 and
// earlier they may be rejected or stripped by classic Jupyter consumers.
func notebookSupportsCellID(nbformat, nbformatMinor int) bool {
	if nbformat > 4 {
		return true
	}
	if nbformat == 4 && nbformatMinor >= 5 {
		return true
	}
	return false
}

// generateCellID produces a 13-character base36 identifier matching the TS
// reference (Math.random().toString(36).substring(2,15)). The TS form is what
// downstream snapshot-based tests expect; emitting the same shape avoids
// determinism / conformance drift across runtimes.
func generateCellID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// On failure the value is "0" — non-empty so the cell is still
		// addressable; the caller is exempt from null-id handling.
		return "0000000000000"
	}
	// Take 64 random bits, render as base36, then take 13 chars (TS slice).
	n := binary.BigEndian.Uint64(buf[:])
	s := strconv.FormatUint(n, 36)
	if len(s) >= 13 {
		return s[:13]
	}
	// Pad with leading zeros so the length matches TS exactly.
	pad := make([]byte, 13-len(s))
	for i := range pad {
		pad[i] = '0'
	}
	return string(pad) + s
}
