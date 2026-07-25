package lsp

import "github.com/grindlemire/go-tui/internal/lsp/provider"

// HoverParams represents textDocument/hover parameters.
type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// Hover and MarkupContent are type aliases for the canonical definitions
// in the provider package, eliminating duplicate type definitions.
type Hover = provider.Hover
type MarkupContent = provider.MarkupContent
