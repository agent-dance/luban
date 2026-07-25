package lsp

import (
	"encoding/json"

	"github.com/grindlemire/go-tui/internal/lsp/log"
)

// Router dispatches LSP method requests to the appropriate handler.
// All language feature methods are dispatched through providers in the Registry.
// Lifecycle and document sync methods are handled directly by the Server.
type Router struct {
	server   *Server
	registry *Registry
}

// NewRouter creates a Router with its required provider registry.
func NewRouter(server *Server, registry *Registry) *Router {
	return &Router{
		server:   server,
		registry: registry,
	}
}

// Route dispatches a request to the appropriate handler.
func (r *Router) Route(req Request) (any, *Error) {
	switch req.Method {
	// Lifecycle
	case "initialize":
		return r.server.handleInitialize(req.Params)
	case "initialized":
		return r.server.handleInitialized()
	case "shutdown":
		return r.server.handleShutdown()
	case "exit":
		r.server.handleExit()
		return nil, nil

	// Document synchronization
	case "textDocument/didOpen":
		return r.server.handleDidOpen(req.Params)
	case "textDocument/didChange":
		return r.server.handleDidChange(req.Params)
	case "textDocument/didClose":
		return r.server.handleDidClose(req.Params)
	case "textDocument/didSave":
		return r.server.handleDidSave(req.Params)

	// Language features - all dispatched to providers
	case "textDocument/hover":
		return r.handleHover(req.Params)
	case "textDocument/completion":
		return r.handleCompletion(req.Params)
	case "textDocument/definition":
		return r.handleDefinition(req.Params)
	case "textDocument/references":
		return r.handleReferences(req.Params)
	case "textDocument/documentSymbol":
		return r.handleDocumentSymbol(req.Params)
	case "workspace/symbol":
		return r.handleWorkspaceSymbol(req.Params)
	case "textDocument/formatting":
		return r.handleFormatting(req.Params)
	case "textDocument/semanticTokens/full":
		return r.handleSemanticTokensFull(req.Params)

	default:
		log.Server("Unknown method: %s", req.Method)
		return nil, &Error{Code: CodeMethodNotFound, Message: "Method not found: " + req.Method}
	}
}

// --- Provider dispatch methods ---

func (r *Router) handleHover(params json.RawMessage) (any, *Error) {
	return r.dispatchPositional(params, func(ctx *CursorContext) (any, error) {
		return r.registry.Hover.Hover(ctx)
	})
}

func (r *Router) handleCompletion(params json.RawMessage) (any, *Error) {
	return r.dispatchPositional(params, func(ctx *CursorContext) (any, error) {
		return r.registry.Completion.Complete(ctx)
	})
}

func (r *Router) handleDefinition(params json.RawMessage) (any, *Error) {
	return r.dispatchPositional(params, func(ctx *CursorContext) (any, error) {
		return r.registry.Definition.Definition(ctx)
	})
}

func (r *Router) handleReferences(params json.RawMessage) (any, *Error) {
	// References has an extra includeDeclaration parameter.
	var p struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
		Context      struct {
			IncludeDeclaration bool `json:"includeDeclaration"`
		} `json:"context"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: CodeInvalidParams, Message: err.Error()}
	}

	doc := r.server.docs.Get(p.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}

	ctx := ResolveCursorContext(doc, p.Position)
	result, err := r.registry.References.References(ctx, p.Context.IncludeDeclaration)
	if err != nil {
		return nil, &Error{Code: CodeInternalError, Message: err.Error()}
	}
	return result, nil
}

func (r *Router) handleDocumentSymbol(params json.RawMessage) (any, *Error) {
	var p DocumentSymbolParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: CodeInvalidParams, Message: err.Error()}
	}

	doc := r.server.docs.Get(p.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}

	result, err := r.registry.DocumentSymbol.DocumentSymbols(doc)
	if err != nil {
		return nil, &Error{Code: CodeInternalError, Message: err.Error()}
	}
	return result, nil
}

func (r *Router) handleWorkspaceSymbol(params json.RawMessage) (any, *Error) {
	var p WorkspaceSymbolParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: CodeInvalidParams, Message: err.Error()}
	}

	result, err := r.registry.WorkspaceSymbol.WorkspaceSymbols(p.Query)
	if err != nil {
		return nil, &Error{Code: CodeInternalError, Message: err.Error()}
	}
	return result, nil
}

func (r *Router) handleFormatting(params json.RawMessage) (any, *Error) {
	var p DocumentFormattingParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: CodeInvalidParams, Message: err.Error()}
	}

	doc := r.server.docs.Get(p.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}

	result, err := r.registry.Formatting.Format(doc, p.Options)
	if err != nil {
		return nil, &Error{Code: CodeInternalError, Message: err.Error()}
	}
	return result, nil
}

func (r *Router) handleSemanticTokensFull(params json.RawMessage) (any, *Error) {
	var p SemanticTokensParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: CodeInvalidParams, Message: err.Error()}
	}

	doc := r.server.docs.Get(p.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}

	result, err := r.registry.SemanticTokens.SemanticTokensFull(doc)
	if err != nil {
		return nil, &Error{Code: CodeInternalError, Message: err.Error()}
	}
	return result, nil
}

// dispatchPositional is a helper that parses a textDocument/position request,
// resolves a CursorContext, and dispatches to a provider function.
func (r *Router) dispatchPositional(params json.RawMessage, fn func(ctx *CursorContext) (any, error)) (any, *Error) {
	var p struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: CodeInvalidParams, Message: err.Error()}
	}

	doc := r.server.docs.Get(p.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}

	ctx := ResolveCursorContext(doc, p.Position)
	result, err := fn(ctx)
	if err != nil {
		return nil, &Error{Code: CodeInternalError, Message: err.Error()}
	}
	return result, nil
}
