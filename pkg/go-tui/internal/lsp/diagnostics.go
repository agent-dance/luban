package lsp

import (
	"github.com/grindlemire/go-tui/internal/lsp/log"
	"github.com/grindlemire/go-tui/internal/lsp/provider"
)

// Diagnostic and DiagnosticSeverity are type aliases for the canonical definitions
// in the provider package, eliminating duplicate type definitions.
type Diagnostic = provider.Diagnostic
type DiagnosticSeverity = provider.DiagnosticSeverity

// Re-export severity constants so existing lsp package code compiles unchanged.
const (
	DiagnosticSeverityError       = provider.DiagnosticSeverityError
	DiagnosticSeverityWarning     = provider.DiagnosticSeverityWarning
	DiagnosticSeverityInformation = provider.DiagnosticSeverityInformation
	DiagnosticSeverityHint        = provider.DiagnosticSeverityHint
)

// PublishDiagnosticsParams represents the parameters for publishDiagnostics.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// publishDiagnostics sends provider diagnostics for a document and merges
// them with gopls diagnostics.
func (s *Server) publishDiagnostics(doc *Document) {
	if doc == nil {
		return
	}

	diagnostics, err := s.router.registry.Diagnostics.Diagnose(doc)
	if err != nil {
		log.Server("Diagnostics provider error: %v", err)
		diagnostics = []Diagnostic{}
	}

	// Add gopls diagnostics (type errors, undefined identifiers, etc.)
	s.goplsDiagnosticsMu.RLock()
	goplsDiags := s.goplsDiagnostics[doc.URI]
	s.goplsDiagnosticsMu.RUnlock()

	for _, gd := range goplsDiags {
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: gd.Range.Start.Line, Character: gd.Range.Start.Character},
				End:   Position{Line: gd.Range.End.Line, Character: gd.Range.End.Character},
			},
			Severity: DiagnosticSeverity(gd.Severity),
			Source:   gd.Source,
			Message:  gd.Message,
		})
	}

	params := PublishDiagnosticsParams{
		URI:         doc.URI,
		Diagnostics: diagnostics,
	}

	if err := s.sendNotification("textDocument/publishDiagnostics", params); err != nil {
		log.Server("Error publishing diagnostics: %v", err)
	}
}
