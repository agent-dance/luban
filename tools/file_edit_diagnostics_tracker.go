// Package tools — file_edit_diagnostics_tracker.go: an in-memory
// DiagnosticsTracker that mirrors the TS clearDeliveredDiagnosticsForFile
// behaviour. Used by FileEditTool / FileWriteTool to ensure a regression
// that re-introduces a previously-fixed diagnostic still surfaces as a
// "new" delivery instead of being silently suppressed as
// "already-delivered".
package tools

import (
	"strconv"
	"sync"
)

// InMemoryDiagnosticsTracker is a simple per-file delivered-diagnostics
// store. The key is an absolute path; values are the canonical
// representations of the diagnostics that have already been surfaced. The
// tracker is cleared before each edit so post-edit diagnostics are reported
// regardless of prior delivery.
type InMemoryDiagnosticsTracker struct {
	mu        sync.Mutex
	delivered map[string]map[string]struct{}
}

// NewInMemoryDiagnosticsTracker constructs a fresh tracker. Returned as a
// concrete pointer so callers can pin it (e.g. share across requests) while
// still satisfying the DiagnosticsTracker interface.
func NewInMemoryDiagnosticsTracker() *InMemoryDiagnosticsTracker {
	return &InMemoryDiagnosticsTracker{
		delivered: map[string]map[string]struct{}{},
	}
}

// ClearForFile drops the delivered set for absPath. Called before each
// edit.
func (t *InMemoryDiagnosticsTracker) ClearForFile(absPath string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.delivered, absPath)
}

// FilterUndelivered returns the subset of `diagnostics` that have not been
// recorded for absPath. The tracker is mutated only by MarkDelivered; this
// method is purely a read-side filter.
func (t *InMemoryDiagnosticsTracker) FilterUndelivered(absPath string, diagnostics []LSPDiagnostic) []LSPDiagnostic {
	if t == nil || len(diagnostics) == 0 {
		return diagnostics
	}
	t.mu.Lock()
	delivered := t.delivered[absPath]
	t.mu.Unlock()
	if len(delivered) == 0 {
		return diagnostics
	}
	out := make([]LSPDiagnostic, 0, len(diagnostics))
	for _, d := range diagnostics {
		if _, ok := delivered[diagnosticKey(d)]; ok {
			continue
		}
		out = append(out, d)
	}
	return out
}

// MarkDelivered records the diagnostics so subsequent FilterUndelivered
// calls will filter them out.
func (t *InMemoryDiagnosticsTracker) MarkDelivered(absPath string, diagnostics []LSPDiagnostic) {
	if t == nil || len(diagnostics) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	set, ok := t.delivered[absPath]
	if !ok {
		set = map[string]struct{}{}
		t.delivered[absPath] = set
	}
	for _, d := range diagnostics {
		set[diagnosticKey(d)] = struct{}{}
	}
}

// diagnosticKey produces a deterministic canonical form for a diagnostic so
// it can be used as a map key. Exact-match semantics — any byte difference
// makes the diagnostic "new". Mirrors the TS dedup which compares
// {severity, message, source, code, range} verbatim.
func diagnosticKey(d LSPDiagnostic) string {
	// Format: severity|source|code|message|sl:sc-el:ec
	// Using a delimiter that cannot appear inside any of the components is
	// not strictly required (the boundaries are unambiguous when read in
	// order), but `\x1f` (US, ASCII 31) is the closest thing to a safe
	// universal delimiter.
	const sep = "\x1f"
	return d.Severity + sep + d.Source + sep + d.Code + sep + d.Message + sep +
		fmtIntPair(d.StartLine, d.StartCharacter) + "-" + fmtIntPair(d.EndLine, d.EndCharacter)
}

func fmtIntPair(a, b int) string {
	return strconv.Itoa(a) + ":" + strconv.Itoa(b)
}
