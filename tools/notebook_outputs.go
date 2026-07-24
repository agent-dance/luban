package tools

import (
	"encoding/json"
)

// MaxNotebookOutputBytes caps the on-disk size of stored outputs per cell to
// prevent runaway notebook growth. Mirrors TS NOTEBOOK_OUTPUT_MAX_BYTES.
// Exposed for downstream tooling that needs to mirror the cap (the TS
// reference exports the same constant).
const MaxNotebookOutputBytes = 100 * 1024

// MAX_NOTEBOOK_OUTPUT_BYTES is the SCREAMING_SNAKE alias for
// MaxNotebookOutputBytes, matching the TS export name verbatim. Either form
// can be referenced; both refer to the same byte cap.
const MAX_NOTEBOOK_OUTPUT_BYTES = MaxNotebookOutputBytes

// maxCellOutputBytes is retained as the package-internal name used by the
// CapOutputs helper. Kept identical to MaxNotebookOutputBytes to avoid two
// sources of truth.
const maxCellOutputBytes = MaxNotebookOutputBytes

const truncatedOutputMarker = "[outputs truncated by notebook editor]"

// CapOutputs returns a possibly-truncated copy of outputs whose serialised
// size does not exceed maxCellOutputBytes. If truncation occurs, a single
// stream output containing truncatedOutputMarker is appended so users can
// see why the data is gone.
func CapOutputs(outputs []any) []any {
	if len(outputs) == 0 {
		return outputs
	}
	data, err := json.Marshal(outputs)
	if err != nil || len(data) <= maxCellOutputBytes {
		return outputs
	}
	// Walk outputs in order and keep adding until we exceed the cap, then
	// append the truncation marker.
	var (
		kept    []any
		running int
	)
	for _, out := range outputs {
		raw, err := json.Marshal(out)
		if err != nil {
			continue
		}
		if running+len(raw) > maxCellOutputBytes {
			break
		}
		running += len(raw)
		kept = append(kept, out)
	}
	kept = append(kept, map[string]any{
		"output_type": "stream",
		"name":        "stderr",
		"text":        []string{truncatedOutputMarker},
	})
	return kept
}
