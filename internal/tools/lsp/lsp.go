package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

const lspTimeout = 10 * time.Second
const lspInitTimeout = 15 * time.Second

// ---------------------------------------------------------------------------
// Binary availability cache
// ---------------------------------------------------------------------------

// LSPState caches availability of LSP server binaries.
type LSPState struct {
	mu        sync.Mutex
	available map[string]bool
}

// isAvailable checks (and caches) whether a binary is on PATH.
func (s *LSPState) isAvailable(binary string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.available[binary]; ok {
		return v
	}
	_, err := exec.LookPath(binary)
	s.available[binary] = err == nil
	return err == nil
}

// NewLSPState creates an LSPState with an initialised availability cache.
func NewLSPState() *LSPState {
	return &LSPState{available: make(map[string]bool)}
}

// NewLSPServerManager creates an LSPServerManager ready for use.
func NewLSPServerManager() *LSPServerManager {
	return &LSPServerManager{servers: make(map[string]*LSPServer)}
}

// ---------------------------------------------------------------------------
// LSP wire types (minimal subset used by this implementation)
// ---------------------------------------------------------------------------

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type lspLocationLink struct {
	TargetURI            string   `json:"targetUri"`
	TargetRange          lspRange `json:"targetRange"`
	TargetSelectionRange lspRange `json:"targetSelectionRange"`
}

type lspTextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type lspTextDocumentPositionParams struct {
	TextDocument lspTextDocumentIdentifier `json:"textDocument"`
	Position     lspPosition               `json:"position"`
}

type lspTextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type lspDidOpenParams struct {
	TextDocument lspTextDocumentItem `json:"textDocument"`
}

type lspInitializeParams struct {
	ProcessID    int                    `json:"processId"`
	RootURI      string                 `json:"rootUri"`
	Capabilities map[string]interface{} `json:"capabilities"`
	ClientInfo   map[string]interface{} `json:"clientInfo,omitempty"`
}

type lspDocumentSymbol struct {
	Name           string              `json:"name"`
	Detail         string              `json:"detail,omitempty"`
	Kind           int                 `json:"kind"`
	Range          lspRange            `json:"range"`
	SelectionRange lspRange            `json:"selectionRange"`
	Children       []lspDocumentSymbol `json:"children,omitempty"`
}

type lspSymbolInformation struct {
	Name          string      `json:"name"`
	Kind          int         `json:"kind"`
	Location      lspLocation `json:"location"`
	ContainerName string      `json:"containerName,omitempty"`
}

type lspReferenceParams struct {
	TextDocument lspTextDocumentIdentifier `json:"textDocument"`
	Position     lspPosition               `json:"position"`
	Context      struct {
		IncludeDeclaration bool `json:"includeDeclaration"`
	} `json:"context"`
}

type lspCallHierarchyItem struct {
	Name           string   `json:"name"`
	Kind           int      `json:"kind"`
	URI            string   `json:"uri"`
	Range          lspRange `json:"range"`
	SelectionRange lspRange `json:"selectionRange"`
	Detail         string   `json:"detail,omitempty"`
}

type lspCallHierarchyIncomingCall struct {
	From       lspCallHierarchyItem `json:"from"`
	FromRanges []lspRange           `json:"fromRanges"`
}

type lspCallHierarchyOutgoingCall struct {
	To         lspCallHierarchyItem `json:"to"`
	FromRanges []lspRange           `json:"fromRanges"`
}

// ---------------------------------------------------------------------------
// LSP Server Manager — manages one persistent LSP server process per language
// ---------------------------------------------------------------------------

// LSPServerManager manages persistent LSP server processes, one per language.
type LSPServerManager struct {
	mu      sync.Mutex
	servers map[string]*LSPServer // lang -> server
}

// LSPServer represents a single running LSP server process.
type LSPServer struct {
	cmd      *exec.Cmd
	client   *jrpc2.Client
	cancel   context.CancelFunc // cancels the server-lifetime context
	mu       sync.Mutex
	openDocs map[string]bool // URI -> already-sent didOpen

	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error
}

// getOrStart returns the cached LSP server for lang, starting one if needed.
func (m *LSPServerManager) getOrStart(ctx context.Context, lang, workspaceRoot string) (*LSPServer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if srv, ok := m.servers[lang]; ok && !srv.client.IsStopped() {
		return srv, nil
	}

	srv, err := newLSPServer(lang)
	if err != nil {
		return nil, err
	}

	initCtx, cancel := context.WithTimeout(ctx, lspInitTimeout)
	defer cancel()

	if err := srv.initialize(initCtx, workspaceRoot); err != nil {
		_ = srv.close(initCtx)
		return nil, lspWrapError(i18n.KeyToolRuntimeLSPInitializeForLanguage, err, lang)
	}

	m.servers[lang] = srv
	return srv, nil
}

// Shutdown sends exit to all running servers and waits for them to stop.
// When ctx expires, the remaining server processes are cancelled and Shutdown
// returns ctx.Err without waiting beyond the caller-owned lifecycle boundary.
func (m *LSPServerManager) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	serversReady := make(chan []*LSPServer, 1)
	go func() {
		m.mu.Lock()
		servers := make([]*LSPServer, 0, len(m.servers))
		for lang, srv := range m.servers {
			delete(m.servers, lang)
			servers = append(servers, srv)
		}
		m.mu.Unlock()
		serversReady <- servers
	}()

	select {
	case servers := <-serversReady:
		return shutdownServers(ctx, servers)
	case <-ctx.Done():
		// The snapshot goroutine may still be waiting for an in-flight startup
		// to release the manager lock. Ensure it eventually consumes and stops
		// those servers without extending this caller's expired boundary.
		go func() {
			servers := <-serversReady
			_ = shutdownServers(ctx, servers)
		}()
		return ctx.Err()
	}
}

func shutdownServers(ctx context.Context, servers []*LSPServer) error {
	results := make(chan error, len(servers))
	for _, srv := range servers {
		go func() {
			results <- srv.close(ctx)
		}()
	}

	errs := make([]error, 0, len(servers)+1)
	remaining := len(servers)
	for remaining > 0 {
		select {
		case err := <-results:
			remaining--
			if err != nil {
				errs = append(errs, err)
			}
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
			for {
				select {
				case err := <-results:
					remaining--
					if err != nil {
						errs = append(errs, err)
					}
				default:
					return errors.Join(errs...)
				}
			}
		}
	}
	return errors.Join(errs...)
}

// newLSPServer spawns the LSP server process and connects a jrpc2 client.
func newLSPServer(lang string) (*LSPServer, error) {
	args := lspServerArgs(lang)
	if len(args) == 0 {
		return nil, i18n.NewError(i18n.KeyToolRuntimeLSPNoServerConfigured, lang)
	}

	srvCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(srvCtx, args[0], args[1:]...)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, lspWrapError(i18n.KeyToolRuntimeLSPStdinPipe, err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, lspWrapError(i18n.KeyToolRuntimeLSPStdoutPipe, err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, lspWrapError(i18n.KeyToolRuntimeLSPStartProcess, err, args[0])
	}

	ch := channel.LSP(stdoutPipe, stdinPipe)
	client := jrpc2.NewClient(ch, nil)

	return &LSPServer{
		cmd:       cmd,
		client:    client,
		cancel:    cancel,
		openDocs:  make(map[string]bool),
		closeDone: make(chan struct{}),
	}, nil
}

// initialize sends the LSP initialize + initialized handshake.
func (s *LSPServer) initialize(ctx context.Context, workspaceRoot string) error {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		absRoot = workspaceRoot
	}
	rootURI := pathToURI(absRoot)

	params := lspInitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   rootURI,
		ClientInfo: map[string]interface{}{
			"name":    brand.CommandName,
			"version": "0.1",
		},
		Capabilities: map[string]interface{}{
			"textDocument": map[string]interface{}{
				"synchronization": map[string]interface{}{
					"dynamicRegistration": false,
					"didOpen":             true,
				},
				"definition": map[string]interface{}{
					"dynamicRegistration": false,
					"linkSupport":         false,
				},
				"references": map[string]interface{}{
					"dynamicRegistration": false,
				},
				"hover": map[string]interface{}{
					"dynamicRegistration": false,
					"contentFormat":       []string{"markdown", "plaintext"},
				},
				"documentSymbol": map[string]interface{}{
					"dynamicRegistration":               false,
					"hierarchicalDocumentSymbolSupport": true,
				},
				"implementation": map[string]interface{}{
					"dynamicRegistration": false,
					"linkSupport":         false,
				},
				"callHierarchy": map[string]interface{}{
					"dynamicRegistration": false,
				},
			},
			"workspace": map[string]interface{}{
				"symbol": map[string]interface{}{
					"dynamicRegistration": false,
				},
			},
		},
	}

	var initializeResult json.RawMessage
	if err := s.client.CallResult(ctx, "initialize", params, &initializeResult); err != nil {
		return lspWrapError(i18n.KeyToolRuntimeLSPInitializeRequest, err)
	}
	// Acknowledge initialization (notification, no response).
	if err := s.client.Notify(ctx, "initialized", map[string]interface{}{}); err != nil {
		return lspWrapError(i18n.KeyToolRuntimeLSPInitializedNotification, err)
	}
	return nil
}

const maxOpenDocs = 200

// ensureOpen sends textDocument/didOpen for absPath if not already sent.
func (s *LSPServer) ensureOpen(ctx context.Context, absPath, langID string) error {
	uri := pathToURI(absPath)

	// H23: Hold the lock for the entire check-read-send-set sequence to prevent
	// TOCTOU race where two concurrent calls both send didOpen.
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.openDocs[uri] {
		return nil
	}

	text, _ := os.ReadFile(absPath) // ignore error; empty string is valid

	params := lspDidOpenParams{
		TextDocument: lspTextDocumentItem{
			URI:        uri,
			LanguageID: langID,
			Version:    1,
			Text:       string(text),
		},
	}
	if err := s.client.Notify(ctx, "textDocument/didOpen", params); err != nil {
		return err
	}

	s.openDocs[uri] = true

	// H22: Evict oldest document when exceeding maxOpenDocs to prevent unbounded growth.
	if len(s.openDocs) > maxOpenDocs {
		// Pick an arbitrary entry to evict (map iteration is random).
		for evictURI := range s.openDocs {
			if evictURI == uri {
				continue // don't evict the one we just added
			}
			closeParams := lspTextDocumentIdentifier{URI: evictURI}
			_ = s.client.Notify(ctx, "textDocument/didClose", map[string]interface{}{
				"textDocument": closeParams,
			})
			delete(s.openDocs, evictURI)
			break
		}
	}

	return nil
}

// close gracefully shuts down the server and waits for process reaping. It is
// safe to call concurrently; every caller observes the same cleanup.
func (s *LSPServer) close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.closeOnce.Do(func() {
		go func() {
			notifyCtx, cancelNotify := context.WithTimeout(ctx, 3*time.Second)
			notifyErr := s.client.Notify(notifyCtx, "exit", nil)
			cancelNotify()

			// A caller whose lifecycle context expires cancels the process below,
			// which also unblocks Close when a peer ignores the exit notification.
			clientErr := s.client.Close()
			s.cancel()
			waitErr := s.cmd.Wait()
			s.closeErr = errors.Join(notifyErr, clientErr, waitErr)
			close(s.closeDone)
		}()
	})

	select {
	case <-s.closeDone:
		return s.closeErr
	case <-ctx.Done():
		s.cancel()
		return ctx.Err()
	}
}

// ---------------------------------------------------------------------------
// LSPTool — implements the tool.Tool interface
// ---------------------------------------------------------------------------

// LSPTool provides Language Server Protocol operations via a persistent server.
type LSPTool struct {
	State   *LSPState         // binary availability cache; must not be nil (inject via SetupRegistry)
	Manager *LSPServerManager // persistent server manager; must not be nil (inject via SetupRegistry)
}

func (t *LSPTool) Name() string { return "LSP" }

func (t *LSPTool) Description() string {
	return lspText(i18n.KeyToolLSPDescription)
}

func (t *LSPTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{}
}

func (t *LSPTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": lspText(i18n.KeyToolLSPInputOperationDescription),
				"enum": []string{
					"goToDefinition",
					"findReferences",
					"hover",
					"documentSymbol",
					"workspaceSymbol",
					"goToImplementation",
					"prepareCallHierarchy",
					"incomingCalls",
					"outgoingCalls",
				},
			},
			"filePath": map[string]any{
				"type":        "string",
				"description": lspText(i18n.KeyToolLSPInputFilePathDescription),
			},
			"line": map[string]any{
				"type":        "integer",
				"description": lspText(i18n.KeyToolLSPInputLineDescription),
			},
			"character": map[string]any{
				"type":        "integer",
				"description": lspText(i18n.KeyToolLSPInputCharacterDescription),
			},
		},
		Required: []string{"operation", "filePath", "line", "character"},
	}
}

func (t *LSPTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := toolbase.ParseInputOrError[LSPInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	// --- Input validation ---
	if in.Operation == "" {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPMissingOperation), IsError: true}, nil
	}
	if in.FilePath == "" {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPMissingFilePath), IsError: true}, nil
	}
	if in.Line <= 0 {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPInvalidLine), IsError: true}, nil
	}
	if in.Character <= 0 {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPInvalidCharacter), IsError: true}, nil
	}

	lang := detectLanguage(in.FilePath)

	// --- Check whether a server binary exists for this language ---
	binary := lspBinaryForLang(lang)
	if binary == "" {
		return types.ToolResult{
			Content: lspFormat(i18n.KeyToolRuntimeLSPUnsupportedOperation, in.Operation, lang),
		}, nil
	}

	state := t.State
	if state == nil {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPStateUnavailable), IsError: true}, nil
	}
	if !state.isAvailable(binary) {
		return types.ToolResult{
			Content: lspFormat(i18n.KeyToolRuntimeLSPBinaryNotFound, binary, lspInstallHint(binary)),
			IsError: true,
		}, nil
	}

	// --- Resolve absolute path and workspace root ---
	absPath, err := filepath.Abs(in.FilePath)
	if err != nil {
		absPath = in.FilePath
	}
	workspaceRoot := filepath.Dir(absPath)

	// --- Get or start the persistent server ---
	manager := t.Manager
	if manager == nil {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPManagerUnavailable), IsError: true}, nil
	}

	srv, err := manager.getOrStart(ctx, lang, workspaceRoot)
	if err != nil {
		return types.ToolResult{
			Content: lspFormat(i18n.KeyToolRuntimeLSPStartServerError, err),
			IsError: true,
		}, nil
	}

	// --- Ensure the file is open on the server ---
	if err := srv.ensureOpen(ctx, absPath, lang); err != nil {
		return types.ToolResult{
			Content: lspFormat(i18n.KeyToolRuntimeLSPOpenFileError, err),
			IsError: true,
		}, nil
	}

	// --- Dispatch ---
	uri := pathToURI(absPath)
	// Convert 1-based (tool input) → 0-based (LSP protocol)
	pos := lspPosition{Line: in.Line - 1, Character: in.Character - 1}
	tdPos := lspTextDocumentPositionParams{
		TextDocument: lspTextDocumentIdentifier{URI: uri},
		Position:     pos,
	}

	tctx, cancel := context.WithTimeout(ctx, lspTimeout)
	defer cancel()

	switch in.Operation {
	case "goToDefinition":
		return t.doDefinition(tctx, srv, tdPos)
	case "findReferences":
		return t.doReferences(tctx, srv, tdPos)
	case "hover":
		return t.doHover(tctx, srv, tdPos)
	case "documentSymbol":
		return t.doDocumentSymbol(tctx, srv, uri)
	case "workspaceSymbol":
		return t.doWorkspaceSymbol(tctx, srv, in.FilePath)
	case "goToImplementation":
		return t.doImplementation(tctx, srv, tdPos)
	case "prepareCallHierarchy":
		return t.doPrepareCallHierarchy(tctx, srv, tdPos)
	case "incomingCalls":
		return t.doIncomingCalls(tctx, srv, tdPos)
	case "outgoingCalls":
		return t.doOutgoingCalls(tctx, srv, tdPos)
	default:
		return types.ToolResult{
			Content: lspFormat(i18n.KeyToolRuntimeLSPUnknownOperation, in.Operation),
			IsError: true,
		}, nil
	}
}

// ---------------------------------------------------------------------------
// Operation implementations
// ---------------------------------------------------------------------------

func (t *LSPTool) doDefinition(ctx context.Context, srv *LSPServer, params lspTextDocumentPositionParams) (types.ToolResult, error) {
	var raw json.RawMessage
	if err := srv.client.CallResult(ctx, "textDocument/definition", params, &raw); err != nil {
		return lspErrorResult("definition", err), nil
	}
	return formatLocationsResult(raw, "definition"), nil
}

func (t *LSPTool) doImplementation(ctx context.Context, srv *LSPServer, params lspTextDocumentPositionParams) (types.ToolResult, error) {
	var raw json.RawMessage
	if err := srv.client.CallResult(ctx, "textDocument/implementation", params, &raw); err != nil {
		return lspErrorResult("implementation", err), nil
	}
	return formatLocationsResult(raw, "implementation"), nil
}

func (t *LSPTool) doReferences(ctx context.Context, srv *LSPServer, params lspTextDocumentPositionParams) (types.ToolResult, error) {
	refParams := lspReferenceParams{
		TextDocument: params.TextDocument,
		Position:     params.Position,
	}
	refParams.Context.IncludeDeclaration = true

	var raw json.RawMessage
	if err := srv.client.CallResult(ctx, "textDocument/references", refParams, &raw); err != nil {
		return lspErrorResult("references", err), nil
	}
	return formatLocationsResult(raw, "references"), nil
}

func (t *LSPTool) doHover(ctx context.Context, srv *LSPServer, params lspTextDocumentPositionParams) (types.ToolResult, error) {
	var raw json.RawMessage
	if err := srv.client.CallResult(ctx, "textDocument/hover", params, &raw); err != nil {
		return lspErrorResult("hover", err), nil
	}
	if lspIsNull(raw) {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoHover)}, nil
	}

	var hover struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(raw, &hover); err != nil {
		return types.ToolResult{Content: string(raw)}, nil
	}
	content := extractHoverContent(hover.Contents)
	if content == "" {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoHover)}, nil
	}
	return types.ToolResult{Content: content}, nil
}

func (t *LSPTool) doDocumentSymbol(ctx context.Context, srv *LSPServer, uri string) (types.ToolResult, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	}
	var raw json.RawMessage
	if err := srv.client.CallResult(ctx, "textDocument/documentSymbol", params, &raw); err != nil {
		return lspErrorResult("documentSymbol", err), nil
	}
	if lspIsNull(raw) {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoSymbols)}, nil
	}

	// Detect whether the server returned DocumentSymbol[] or SymbolInformation[].
	// DocumentSymbol has selectionRange but no location; SymbolInformation has location.
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoSymbols)}, nil
	}

	var probe struct {
		Location *lspLocation `json:"location,omitempty"`
	}
	_ = json.Unmarshal(items[0], &probe)

	var sb strings.Builder
	if probe.Location != nil {
		// SymbolInformation (flat list)
		var syms []lspSymbolInformation
		if err := json.Unmarshal(raw, &syms); err == nil {
			for _, s := range syms {
				fmt.Fprintf(&sb, "%s %s %s\n", symbolKindName(s.Kind), s.Name, formatLocation(s.Location))
			}
		}
	} else {
		// DocumentSymbol (hierarchical)
		var syms []lspDocumentSymbol
		if err := json.Unmarshal(raw, &syms); err == nil {
			writeDocumentSymbols(&sb, syms, 0)
		}
	}
	result := strings.TrimSpace(sb.String())
	if result == "" {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoSymbols)}, nil
	}
	return types.ToolResult{Content: result}, nil
}

func (t *LSPTool) doWorkspaceSymbol(ctx context.Context, srv *LSPServer, query string) (types.ToolResult, error) {
	params := map[string]interface{}{"query": query}
	var raw json.RawMessage
	if err := srv.client.CallResult(ctx, "workspace/symbol", params, &raw); err != nil {
		return lspErrorResult("workspaceSymbol", err), nil
	}
	if lspIsNull(raw) {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoWorkspaceSymbols)}, nil
	}

	var syms []lspSymbolInformation
	if err := json.Unmarshal(raw, &syms); err != nil {
		return types.ToolResult{Content: string(raw)}, nil
	}
	if len(syms) == 0 {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoWorkspaceSymbols)}, nil
	}

	var sb strings.Builder
	for _, s := range syms {
		container := ""
		if s.ContainerName != "" {
			container = s.ContainerName + "."
		}
		fmt.Fprintf(&sb, "%s %s%s %s\n", symbolKindName(s.Kind), container, s.Name, formatLocation(s.Location))
	}
	return types.ToolResult{Content: strings.TrimSpace(sb.String())}, nil
}

func (t *LSPTool) doPrepareCallHierarchy(ctx context.Context, srv *LSPServer, params lspTextDocumentPositionParams) (types.ToolResult, error) {
	var items []lspCallHierarchyItem
	if err := srv.client.CallResult(ctx, "textDocument/prepareCallHierarchy", params, &items); err != nil {
		return lspErrorResult("prepareCallHierarchy", err), nil
	}
	if len(items) == 0 {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoCallHierarchyItem)}, nil
	}

	var sb strings.Builder
	for _, item := range items {
		detail := ""
		if item.Detail != "" {
			detail = " — " + item.Detail
		}
		fmt.Fprintf(&sb, "%s %s%s\n  %s:%d:%d\n",
			symbolKindName(item.Kind), item.Name, detail,
			uriToPath(item.URI), item.Range.Start.Line+1, item.Range.Start.Character+1)
	}
	return types.ToolResult{Content: strings.TrimSpace(sb.String())}, nil
}

func (t *LSPTool) doIncomingCalls(ctx context.Context, srv *LSPServer, params lspTextDocumentPositionParams) (types.ToolResult, error) {
	// First prepare the call hierarchy item at this position.
	var prepItems []lspCallHierarchyItem
	if err := srv.client.CallResult(ctx, "textDocument/prepareCallHierarchy", params, &prepItems); err != nil {
		return lspErrorResult("prepareCallHierarchy (for incomingCalls)", err), nil
	}
	if len(prepItems) == 0 {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoCallHierarchyItem)}, nil
	}

	var calls []lspCallHierarchyIncomingCall
	callParams := map[string]interface{}{"item": prepItems[0]}
	if err := srv.client.CallResult(ctx, "callHierarchy/incomingCalls", callParams, &calls); err != nil {
		return lspErrorResult("callHierarchy/incomingCalls", err), nil
	}
	if len(calls) == 0 {
		return types.ToolResult{Content: lspFormat(i18n.KeyToolRuntimeLSPNoIncomingCalls, prepItems[0].Name)}, nil
	}

	var sb strings.Builder
	fmt.Fprintln(&sb, lspFormat(i18n.KeyToolRuntimeLSPIncomingCallsHeader, prepItems[0].Name))
	for _, c := range calls {
		fmt.Fprintf(&sb, "  %s %s\n    %s:%d:%d\n",
			symbolKindName(c.From.Kind), c.From.Name,
			uriToPath(c.From.URI), c.From.Range.Start.Line+1, c.From.Range.Start.Character+1)
	}
	return types.ToolResult{Content: strings.TrimSpace(sb.String())}, nil
}

func (t *LSPTool) doOutgoingCalls(ctx context.Context, srv *LSPServer, params lspTextDocumentPositionParams) (types.ToolResult, error) {
	// First prepare the call hierarchy item at this position.
	var prepItems []lspCallHierarchyItem
	if err := srv.client.CallResult(ctx, "textDocument/prepareCallHierarchy", params, &prepItems); err != nil {
		return lspErrorResult("prepareCallHierarchy (for outgoingCalls)", err), nil
	}
	if len(prepItems) == 0 {
		return types.ToolResult{Content: lspText(i18n.KeyToolRuntimeLSPNoCallHierarchyItem)}, nil
	}

	var calls []lspCallHierarchyOutgoingCall
	callParams := map[string]interface{}{"item": prepItems[0]}
	if err := srv.client.CallResult(ctx, "callHierarchy/outgoingCalls", callParams, &calls); err != nil {
		return lspErrorResult("callHierarchy/outgoingCalls", err), nil
	}
	if len(calls) == 0 {
		return types.ToolResult{Content: lspFormat(i18n.KeyToolRuntimeLSPNoOutgoingCalls, prepItems[0].Name)}, nil
	}

	var sb strings.Builder
	fmt.Fprintln(&sb, lspFormat(i18n.KeyToolRuntimeLSPOutgoingCallsHeader, prepItems[0].Name))
	for _, c := range calls {
		fmt.Fprintf(&sb, "  %s %s\n    %s:%d:%d\n",
			symbolKindName(c.To.Kind), c.To.Name,
			uriToPath(c.To.URI), c.To.Range.Start.Line+1, c.To.Range.Start.Character+1)
	}
	return types.ToolResult{Content: strings.TrimSpace(sb.String())}, nil
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// detectLanguage returns a simple language identifier from a file path.
func detectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	default:
		return ext
	}
}

// lspBinaryForLang returns the binary name for an LSP server, or "" if unsupported.
func lspBinaryForLang(lang string) string {
	switch lang {
	case "go":
		return "gopls"
	case "typescript", "javascript":
		return "typescript-language-server"
	default:
		return ""
	}
}

// lspServerArgs returns the command + arguments to launch the LSP server.
func lspServerArgs(lang string) []string {
	switch lang {
	case "go":
		return []string{"gopls", "serve"}
	case "typescript", "javascript":
		return []string{"typescript-language-server", "--stdio"}
	default:
		return nil
	}
}

// lspInstallHint returns a human-readable install instruction for the binary.
func lspInstallHint(binary string) string {
	switch binary {
	case "gopls":
		return "go install golang.org/x/tools/gopls@latest"
	case "typescript-language-server":
		return "npm install -g typescript-language-server typescript"
	default:
		return lspFormat(i18n.KeyToolRuntimeLSPInstallBinaryFallback, binary)
	}
}

// pathToURI converts an absolute filesystem path to a file:// URI.
func pathToURI(absPath string) string {
	// Ensure the path is slash-separated and starts with /
	p := strings.ReplaceAll(absPath, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

// uriToPath converts a file:// URI back to a filesystem path.
func uriToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}
	p := strings.TrimPrefix(uri, "file://")
	if len(p) >= 4 && p[0] == '/' && p[2] == ':' && (p[3] == '/' || p[3] == '\\') {
		p = p[1:]
	}
	if len(p) >= 3 && p[0] == '/' && p[2] != ':' {
		return p
	}
	return filepath.FromSlash(p)
}

// lspIsNull reports whether raw JSON is null or empty.
func lspIsNull(raw json.RawMessage) bool {
	return len(raw) == 0 || strings.TrimSpace(string(raw)) == "null"
}

// lspErrorResult wraps an LSP call error into a ToolResult.
func lspErrorResult(op string, err error) types.ToolResult {
	return types.ToolResult{
		Content: lspFormat(i18n.KeyToolRuntimeLSPOperationError, op, err),
		IsError: true,
	}
}

// formatLocationsResult formats a raw locations/locationLinks JSON value as text.
func formatLocationsResult(raw json.RawMessage, op string) types.ToolResult {
	if lspIsNull(raw) {
		return types.ToolResult{Content: lspFormat(i18n.KeyToolRuntimeLSPNoOperationResults, op)}
	}

	trimmed := strings.TrimSpace(string(raw))

	if strings.HasPrefix(trimmed, "[") {
		// Could be Location[], LocationLink[], or empty array.
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
			return types.ToolResult{Content: lspFormat(i18n.KeyToolRuntimeLSPNoOperationResults, op)}
		}

		// Peek at first element to distinguish Location from LocationLink.
		var probe struct {
			TargetURI string `json:"targetUri"`
			URI       string `json:"uri"`
		}
		_ = json.Unmarshal(items[0], &probe)

		var sb strings.Builder
		if probe.TargetURI != "" {
			var links []lspLocationLink
			if err := json.Unmarshal(raw, &links); err == nil {
				for _, l := range links {
					fmt.Fprintf(&sb, "%s:%d:%d\n", uriToPath(l.TargetURI),
						l.TargetRange.Start.Line+1, l.TargetRange.Start.Character+1)
				}
			}
		} else {
			var locs []lspLocation
			if err := json.Unmarshal(raw, &locs); err == nil {
				for _, l := range locs {
					fmt.Fprintf(&sb, "%s\n", formatLocation(l))
				}
			}
		}
		result := strings.TrimSpace(sb.String())
		if result == "" {
			return types.ToolResult{Content: lspFormat(i18n.KeyToolRuntimeLSPNoOperationResults, op)}
		}
		return types.ToolResult{Content: result}
	}

	// Single Location object.
	var loc lspLocation
	if err := json.Unmarshal(raw, &loc); err == nil && loc.URI != "" {
		return types.ToolResult{Content: formatLocation(loc)}
	}

	return types.ToolResult{Content: string(raw)}
}

// formatLocation formats an LSP Location as "path:line:col" (1-based).
func formatLocation(loc lspLocation) string {
	return fmt.Sprintf("%s:%d:%d",
		uriToPath(loc.URI),
		loc.Range.Start.Line+1,
		loc.Range.Start.Character+1)
}

// extractHoverContent converts the polymorphic hover contents to a plain string.
func extractHoverContent(raw json.RawMessage) string {
	if lspIsNull(raw) {
		return ""
	}

	// Plain string from older LSP peers; retained for protocol interoperability.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return s
	}

	// MarkedString: { language: string, value: string }
	// Must check before MarkupContent because both have a "value" field.
	var ms struct {
		Language *string `json:"language"` // pointer so we can distinguish presence
		Value    string  `json:"value"`
	}
	if err := json.Unmarshal(raw, &ms); err == nil && ms.Language != nil {
		if *ms.Language != "" {
			return fmt.Sprintf("```%s\n%s\n```", *ms.Language, ms.Value)
		}
		return ms.Value
	}

	// MarkupContent: { kind: "markdown"|"plaintext", value: string }
	var markup struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &markup); err == nil && markup.Value != "" {
		return markup.Value
	}

	// MarkedString array from older LSP peers; retained for protocol interoperability.
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err == nil {
			var parts []string
			for _, item := range items {
				if c := extractHoverContent(item); c != "" {
					parts = append(parts, c)
				}
			}
			return strings.Join(parts, "\n\n")
		}
	}

	return string(raw)
}

// writeDocumentSymbols recursively writes hierarchical document symbols.
func writeDocumentSymbols(sb *strings.Builder, syms []lspDocumentSymbol, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, s := range syms {
		detail := ""
		if s.Detail != "" {
			detail = " " + s.Detail
		}
		fmt.Fprintf(sb, "%s%s %s%s %s\n",
			indent, symbolKindName(s.Kind), s.Name, detail,
			lspFormat(i18n.KeyToolRuntimeLSPSymbolLine, s.Range.Start.Line+1))
		if len(s.Children) > 0 {
			writeDocumentSymbols(sb, s.Children, depth+1)
		}
	}
}

func lspText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func lspFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func lspWrapError(key i18n.Key, err error, args ...any) error {
	return i18n.WrapError(key, err, args...)
}

// symbolKindName maps an LSP SymbolKind integer to a human-readable name.
func symbolKindName(kind int) string {
	names := [...]string{
		"", "File", "Module", "Namespace", "Package",
		"Class", "Method", "Property", "Field", "Constructor",
		"Enum", "Interface", "Function", "Variable", "Constant",
		"String", "Number", "Boolean", "Array", "Object",
		"Key", "Null", "EnumMember", "Struct", "Event",
		"Operator", "TypeParameter",
	}
	if kind >= 1 && kind < len(names) {
		return names[kind]
	}
	return fmt.Sprintf("Kind%d", kind)
}
