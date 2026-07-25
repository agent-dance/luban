package file

import (
	"strconv"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// notebookCellLookupError keeps the public, runtime-localized lookup message
// independent from a parser cause while preserving that cause for errors.Is
// and errors.As.
type notebookCellLookupError struct {
	display error
	cause   error
}

func (e *notebookCellLookupError) Error() string {
	if e == nil || e.display == nil {
		return ""
	}
	return e.display.Error()
}

func (e *notebookCellLookupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// ResolveCell looks up a cell by exact id or the TS `cell-N` positional
// fallback. Bare integers, negative indexes, and "last" are intentionally
// rejected: the TS NotebookEdit validator only accepts /^cell-(\d+)$/ after
// failing an exact cell id lookup.
// Returns the cell pointer and its absolute index, or an error if no match.
func ResolveCell(nb *Notebook, ref string) (*Cell, int, error) {
	if nb == nil {
		return nil, -1, i18n.NewError(i18n.KeyToolNotebookHelperNilNotebook)
	}
	if ref == "" {
		return nil, -1, nil
	}
	if len(nb.Cells) == 0 {
		return nil, -1, i18n.NewError(i18n.KeyToolNotebookHelperCellNotFound, ref)
	}

	// ID match (case-sensitive, like TS).
	for i := range nb.Cells {
		if nb.Cells[i].ID == ref {
			return &nb.Cells[i], i, nil
		}
	}

	// `cell-N` positional fallback. TS parseCellId(/^cell-(\d+)$/) treats
	// the suffix as a 0-indexed position.
	if strings.HasPrefix(ref, "cell-") {
		suffix := strings.TrimPrefix(ref, "cell-")
		if isASCIIDigits(suffix) {
			idx, err := strconv.Atoi(suffix)
			if err != nil {
				return nil, -1, &notebookCellLookupError{
					display: i18n.NewError(i18n.KeyToolNotebookHelperCellIDNotFound, ref),
					cause:   err,
				}
			}
			if idx >= 0 && idx < len(nb.Cells) {
				return &nb.Cells[idx], idx, nil
			}
			return nil, -1, i18n.NewError(i18n.KeyToolNotebookHelperCellIndexNotFound, idx)
		}
	}

	return nil, -1, i18n.NewError(i18n.KeyToolNotebookHelperCellIDNotFound, ref)
}

func isASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

// findCellIndexByRef is a lenient helper used by callers that just want an
// index and treat unknown refs as -1 instead of an error.
func findCellIndexByRef(nb *Notebook, ref string) int {
	_, idx, err := ResolveCell(nb, ref)
	if err != nil {
		return -1
	}
	return idx
}
