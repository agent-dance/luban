package lsp

import (
	"github.com/grindlemire/go-tui/internal/lsp/provider"
)

// CompletionParams represents textDocument/completion parameters.
type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      *CompletionContext     `json:"context,omitempty"`
}

// CompletionContext contains additional information about the context.
type CompletionContext struct {
	TriggerKind      int    `json:"triggerKind"`
	TriggerCharacter string `json:"triggerCharacter,omitempty"`
}

// CompletionList, CompletionItem, and CompletionItemKind are type aliases for
// the canonical definitions in the provider package, eliminating duplicate type
// definitions.
type CompletionList = provider.CompletionList
type CompletionItem = provider.CompletionItem
type CompletionItemKind = provider.CompletionItemKind

// Re-export CompletionItemKind constants so existing lsp package code compiles unchanged.
const (
	CompletionItemKindText          = provider.CompletionItemKindText
	CompletionItemKindMethod        = provider.CompletionItemKindMethod
	CompletionItemKindFunction      = provider.CompletionItemKindFunction
	CompletionItemKindConstructor   = provider.CompletionItemKindConstructor
	CompletionItemKindField         = provider.CompletionItemKindField
	CompletionItemKindVariable      = provider.CompletionItemKindVariable
	CompletionItemKindClass         = provider.CompletionItemKindClass
	CompletionItemKindInterface     = provider.CompletionItemKindInterface
	CompletionItemKindModule        = provider.CompletionItemKindModule
	CompletionItemKindProperty      = provider.CompletionItemKindProperty
	CompletionItemKindUnit          = provider.CompletionItemKindUnit
	CompletionItemKindValue         = provider.CompletionItemKindValue
	CompletionItemKindEnum          = provider.CompletionItemKindEnum
	CompletionItemKindKeyword       = provider.CompletionItemKindKeyword
	CompletionItemKindSnippet       = provider.CompletionItemKindSnippet
	CompletionItemKindColor         = provider.CompletionItemKindColor
	CompletionItemKindFile          = provider.CompletionItemKindFile
	CompletionItemKindReference     = provider.CompletionItemKindReference
	CompletionItemKindFolder        = provider.CompletionItemKindFolder
	CompletionItemKindEnumMember    = provider.CompletionItemKindEnumMember
	CompletionItemKindConstant      = provider.CompletionItemKindConstant
	CompletionItemKindStruct        = provider.CompletionItemKindStruct
	CompletionItemKindEvent         = provider.CompletionItemKindEvent
	CompletionItemKindOperator      = provider.CompletionItemKindOperator
	CompletionItemKindTypeParameter = provider.CompletionItemKindTypeParameter
)
