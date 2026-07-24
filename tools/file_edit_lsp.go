// Package tools — file_edit_lsp.go defines the narrow LSP interface that
// FileEditTool consumes after a successful write. The Go runtime currently
// has no first-party LSP implementation; the existing LSPTool surfaces
// diagnostics through MCP plumbing and doesn't expose a synchronous
// Diagnose method. To keep FileEditTool independent of that wiring we
// declare a minimal interface here so callers can supply any
// implementation (real LSP, in-memory mock for tests, no-op).
//
// Mirrors the JSON shape used by src/services/lsp/* so transcripts replay
// cleanly across runtimes.
package tools

import "context"

// LSPDiagnostic mirrors the protocol-level diagnostic shape: a severity
// string, a one-line message, the originating source (e.g. "tsserver",
// "rustc"), an optional code, and a 0-based (line, character) range pair.
type LSPDiagnostic struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
	Code     string `json:"code,omitempty"`

	StartLine      int `json:"startLine"`
	StartCharacter int `json:"startCharacter"`
	EndLine        int `json:"endLine"`
	EndCharacter   int `json:"endCharacter"`
}

// LSPDiagnoser is the interface FileEditTool consumes. Implementations may
// fan out to a long-lived LSP server, run a stateless lint pass, or return
// nil when no provider is configured.
//
// Failures are non-fatal: FileEditTool ignores any returned error and
// surfaces an empty diagnostics list in the result payload.
type LSPDiagnoser interface {
	// Diagnose returns the diagnostics produced by analysing `content` as
	// if it were the on-disk content of `absPath`. Implementations should
	// honour `ctx` cancellation since FileEditTool may abort on a slow
	// LSP server.
	Diagnose(ctx context.Context, absPath, content string) ([]LSPDiagnostic, error)
}

// LSPDocumentSync is an optional capability that FileEditTool will invoke
// (when implemented) so the language server can keep a coherent view of the
// file across an edit. Mirrors the TS pair lspManager.changeFile +
// lspManager.saveFile, used to drive textDocument/didChange &
// textDocument/didSave so subsequent rename / refactor / hover requests on
// the same file inside the session see the post-edit text.
//
// FileEditTool calls these best-effort: errors do not block the edit. A
// Diagnoser that does not implement this interface continues to work via
// the one-shot Diagnose path.
type LSPDocumentSync interface {
	// DidChange is invoked after the new content has been written and the
	// in-memory view should be updated. `content` is the post-edit text.
	DidChange(ctx context.Context, absPath, content string) error
	// DidSave signals that the file has been persisted to disk. Some
	// servers run heavyweight checks only on save.
	DidSave(ctx context.Context, absPath, content string) error
}

// DiagnosticsTracker is an optional component that FileEditTool consults so
// repeated edits which fix and re-introduce the same diagnostic still report
// the second-round issue as "new". Mirrors the TS clearDeliveredDiagnosticsForFile
// behaviour: the tracker is cleared before each edit and consulted afterwards
// to filter previously-delivered duplicates.
//
// Implementations should be cheap (in-memory keyed by absolute path).
// FileEditTool calls all three methods unconditionally; nil-safe wrapper is
// supplied via newNoopDiagnosticsTracker.
type DiagnosticsTracker interface {
	// ClearForFile drops the per-file delivered set, called before the
	// edit so post-edit diagnostics are reported regardless of prior
	// delivery.
	ClearForFile(absPath string)
	// FilterUndelivered returns the subset of `diagnostics` that have not
	// been recorded for `absPath`.
	FilterUndelivered(absPath string, diagnostics []LSPDiagnostic) []LSPDiagnostic
	// MarkDelivered records the diagnostics so subsequent calls would
	// filter them out.
	MarkDelivered(absPath string, diagnostics []LSPDiagnostic)
}

// noopLSPDiagnoser is the default when no provider is supplied. It returns
// no diagnostics and is exposed via NewNoopLSPDiagnoser for callers that
// want an explicit "diagnostics disabled" sentinel.
type noopLSPDiagnoser struct{}

// NewNoopLSPDiagnoser returns a Diagnoser that always returns no
// diagnostics. Useful in tests and as a default in CLI bootstrap.
func NewNoopLSPDiagnoser() LSPDiagnoser { return noopLSPDiagnoser{} }

// Diagnose implements LSPDiagnoser by returning no diagnostics.
func (noopLSPDiagnoser) Diagnose(ctx context.Context, absPath, content string) ([]LSPDiagnostic, error) {
	return nil, nil
}
