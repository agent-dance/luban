package lsp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestLSPServerManagerShutdownWaitsForOwnedServers(t *testing.T) {
	manager := NewLSPServerManager()
	server := &LSPServer{
		cancel:    func() {},
		closeDone: make(chan struct{}),
	}
	// Mark cleanup as already started so this test controls its completion
	// without spawning an external language-server process.
	server.closeOnce.Do(func() {})
	manager.servers["test"] = server

	result := make(chan error, 1)
	go func() {
		result <- manager.Shutdown(context.Background())
	}()

	deadline := time.After(2 * time.Second)
	for {
		manager.mu.Lock()
		remaining := len(manager.servers)
		manager.mu.Unlock()
		if remaining == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Shutdown did not take ownership of the registered server")
		case <-time.After(time.Millisecond):
		}
	}

	select {
	case err := <-result:
		t.Fatalf("Shutdown returned before server cleanup completed: %v", err)
	default:
	}

	close(server.closeDone)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not return after server cleanup completed")
	}
}

func TestLSPServerManagerShutdownHonorsContextWhileManagerBusy(t *testing.T) {
	manager := NewLSPServerManager()
	manager.mu.Lock()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := manager.Shutdown(ctx)
	elapsed := time.Since(started)
	manager.mu.Unlock()

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want context deadline exceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("Shutdown exceeded caller lifecycle boundary: %v", elapsed)
	}
}

func TestLSPServerManagerShutdownCancelsServerAtDeadline(t *testing.T) {
	manager := NewLSPServerManager()
	cancelled := make(chan struct{})
	server := &LSPServer{
		cancel: func() {
			select {
			case <-cancelled:
			default:
				close(cancelled)
			}
		},
		closeDone: make(chan struct{}),
	}
	server.closeOnce.Do(func() {})
	manager.servers["test"] = server

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := manager.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want context deadline exceeded", err)
	}

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not cancel the server process at the deadline")
	}
}

func TestLSPServerManagerShutdownAggregatesErrorsAndAcceptsNilContext(t *testing.T) {
	firstErr := errors.New("first close failure")
	secondErr := errors.New("second reap failure")
	manager := NewLSPServerManager()
	for lang, closeErr := range map[string]error{
		"first":  firstErr,
		"second": secondErr,
	} {
		closeDone := make(chan struct{})
		close(closeDone)
		server := &LSPServer{
			cancel:    func() {},
			closeDone: closeDone,
			closeErr:  closeErr,
		}
		server.closeOnce.Do(func() {})
		manager.servers[lang] = server
	}

	var unboundedContext context.Context
	err := manager.Shutdown(unboundedContext)
	if !errors.Is(err, firstErr) {
		t.Fatalf("Shutdown error = %v, want first server error", err)
	}
	if !errors.Is(err, secondErr) {
		t.Fatalf("Shutdown error = %v, want second server error", err)
	}
}

func TestLSPServerManagerShutdownJoinsCompletedErrorsWithDeadline(t *testing.T) {
	completedErr := errors.New("completed close failure")
	completedDone := make(chan struct{})
	close(completedDone)
	completed := &LSPServer{
		cancel:    func() {},
		closeDone: completedDone,
		closeErr:  completedErr,
	}
	completed.closeOnce.Do(func() {})

	pending := &LSPServer{
		cancel:    func() {},
		closeDone: make(chan struct{}),
	}
	pending.closeOnce.Do(func() {})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	err := shutdownServers(ctx, []*LSPServer{completed, pending})
	if !errors.Is(err, completedErr) {
		t.Fatalf("shutdownServers error = %v, want completed server error", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdownServers error = %v, want context deadline exceeded", err)
	}
}

// newLSPTool returns an LSPTool with fresh isolated state and manager.
func newLSPTool() *LSPTool {
	return &LSPTool{
		State: &LSPState{
			available: make(map[string]bool),
		},
		Manager: &LSPServerManager{
			servers: make(map[string]*LSPServer),
		},
	}
}

// --- Schema validation ---

func TestLSPToolSchema(t *testing.T) {
	tool := newLSPTool()
	schema := tool.Schema()

	if schema.Type != "object" {
		t.Errorf("expected schema type 'object', got %q", schema.Type)
	}

	required := map[string]bool{}
	for _, r := range schema.Required {
		required[r] = true
	}
	for _, field := range []string{"operation", "filePath", "line", "character"} {
		if !required[field] {
			t.Errorf("expected %q in required fields", field)
		}
	}

	for _, field := range []string{"operation", "filePath", "line", "character"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Errorf("expected property %q in schema", field)
		}
	}
}

func TestLSPToolName(t *testing.T) {
	tool := newLSPTool()
	if tool.Name() != "LSP" {
		t.Errorf("expected name 'LSP', got %q", tool.Name())
	}
}

func TestLSPToolMetadata(t *testing.T) {
	if got := newLSPTool().ToolMetadata(nil); got != (types.ToolMetadata{}) {
		t.Fatalf("ToolMetadata() = %#v, want empty metadata", got)
	}
}

func TestLSPWrapErrorPreservesAndRendersExternalCause(t *testing.T) {
	cause := errors.New("raw lsp cause")
	err := lspWrapError(i18n.KeyToolRuntimeLSPStartProcess, cause, "gopls")
	if !errors.Is(err, cause) {
		t.Fatal("wrapped LSP error does not preserve its cause")
	}
	if rendered := err.Error(); !strings.Contains(rendered, "gopls") || !strings.Contains(rendered, cause.Error()) {
		t.Fatalf("wrapped LSP error lost external details: %q", rendered)
	}
}

// --- Validation errors ---

func TestLSPToolMissingOperation(t *testing.T) {
	tool := newLSPTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"filePath":  "foo.go",
		"line":      1,
		"character": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing operation")
	}
	if !strings.Contains(result.Content, "operation") {
		t.Errorf("expected 'operation' in error message, got: %q", result.Content)
	}
}

func TestLSPToolMissingFilePath(t *testing.T) {
	tool := newLSPTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"operation": "hover",
		"line":      1,
		"character": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing filePath")
	}
}

func TestLSPToolInvalidLine(t *testing.T) {
	tool := newLSPTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"operation": "hover",
		"filePath":  "foo.go",
		"line":      0,
		"character": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for line=0")
	}
}

func TestLSPToolInvalidCharacter(t *testing.T) {
	tool := newLSPTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"operation": "hover",
		"filePath":  "foo.go",
		"line":      1,
		"character": 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for character=0")
	}
}

// --- Unsupported / unavailable server ---

// TestLSPToolUnknownOperationGoFile verifies that operations unsupported by the
// new implementation are handled correctly. In the new jrpc2 implementation ALL
// 9 operations are supported for Go, so this test verifies the server-not-found
// path instead (since we cannot rely on gopls being installed in CI).
func TestLSPToolUnknownOperationGoFile(t *testing.T) {
	tool := newLSPTool()
	// Force gopls to appear unavailable.
	tool.State.available["gopls"] = false

	result, err := tool.Execute(context.Background(), map[string]any{
		"operation": "prepareCallHierarchy",
		"filePath":  "foo.go",
		"line":      1,
		"character": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With gopls marked unavailable, we expect an error with install guidance.
	if !result.IsError {
		t.Errorf("expected IsError=true when gopls is unavailable, got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "gopls") {
		t.Errorf("expected 'gopls' in error message, got: %q", result.Content)
	}
}

func TestLSPToolUnsupportedLanguage(t *testing.T) {
	tool := newLSPTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"operation": "hover",
		"filePath":  "script.py",
		"line":      1,
		"character": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected graceful degradation for unsupported lang, got error: %q", result.Content)
	}
	if !strings.Contains(result.Content, "hover") {
		t.Errorf("expected operation name in degradation message, got: %q", result.Content)
	}
}

// --- Missing binary error ---

func TestLSPToolGoplsNotFound(t *testing.T) {
	tool := newLSPTool()
	// Mark gopls as unavailable.
	tool.State.available["gopls"] = false

	for _, op := range []string{
		"goToDefinition", "findReferences", "hover", "documentSymbol",
		"workspaceSymbol", "goToImplementation", "prepareCallHierarchy",
		"incomingCalls", "outgoingCalls",
	} {
		result, err := tool.Execute(context.Background(), map[string]any{
			"operation": op,
			"filePath":  "main.go",
			"line":      1,
			"character": 1,
		})
		if err != nil {
			t.Fatalf("op=%s unexpected error: %v", op, err)
		}
		if !result.IsError {
			t.Errorf("op=%s expected IsError=true when gopls unavailable", op)
		}
		if !strings.Contains(result.Content, "gopls") {
			t.Errorf("op=%s expected the missing binary name, got: %q", op, result.Content)
		}
		if !strings.Contains(result.Content, "go install") {
			t.Errorf("op=%s expected install hint, got: %q", op, result.Content)
		}
	}
}

// --- detectLanguage ---

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.ts", "typescript"},
		{"app.tsx", "typescript"},
		{"index.js", "javascript"},
		{"index.jsx", "javascript"},
		{"script.py", "python"},
		{"lib.rs", "rust"},
		{"unknown.xyz", ".xyz"},
	}
	for _, c := range cases {
		got := detectLanguage(c.path)
		if got != c.want {
			t.Errorf("detectLanguage(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// --- LSPState binary caching ---

func TestLSPStateCaching(t *testing.T) {
	s := &LSPState{available: make(map[string]bool)}

	// Pre-seed a known-unavailable binary.
	s.mu.Lock()
	s.available["__nonexistent_binary__"] = false
	s.mu.Unlock()

	if s.isAvailable("__nonexistent_binary__") {
		t.Error("expected false for nonexistent binary")
	}

	// Pre-seed as available.
	s.mu.Lock()
	s.available["__nonexistent_binary__"] = true
	s.mu.Unlock()

	if !s.isAvailable("__nonexistent_binary__") {
		t.Error("expected cached true")
	}
}

// --- Helpers ---

func TestPathToURI(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/home/user/foo.go", "file:///home/user/foo.go"},
		{"/Users/x/bar.ts", "file:///Users/x/bar.ts"},
	}
	for _, c := range cases {
		got := pathToURI(c.in)
		if got != c.want {
			t.Errorf("pathToURI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestURIToPath(t *testing.T) {
	got := uriToPath("file:///home/user/foo.go")
	if got != "/home/user/foo.go" {
		t.Errorf("uriToPath got %q", got)
	}
	got = uriToPath("/no/scheme")
	if got != "/no/scheme" {
		t.Errorf("uriToPath non-file got %q", got)
	}
}

func TestSymbolKindName(t *testing.T) {
	if symbolKindName(6) != "Method" {
		t.Errorf("expected 'Method' for kind 6, got %q", symbolKindName(6))
	}
	if symbolKindName(12) != "Function" {
		t.Errorf("expected 'Function' for kind 12, got %q", symbolKindName(12))
	}
	if symbolKindName(99) != "Kind99" {
		t.Errorf("expected 'Kind99' for unknown kind, got %q", symbolKindName(99))
	}
}

func TestExtractHoverContent(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "markup_content",
			input: `{"kind":"markdown","value":"**func** Foo()"}`,
			want:  "**func** Foo()",
		},
		{
			name:  "plain_string",
			input: `"hello world"`,
			want:  "hello world",
		},
		{
			name:  "marked_string_with_language",
			input: `{"language":"go","value":"func Foo()"}`,
			want:  "```go\nfunc Foo()\n```",
		},
		{
			name:  "null",
			input: `null`,
			want:  "",
		},
	}
	for _, c := range cases {
		got := extractHoverContent([]byte(c.input))
		if got != c.want {
			t.Errorf("extractHoverContent(%s): got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestLSPToolWithRealGopls runs a smoke-test against a real gopls server.
// It is skipped if gopls is not found on PATH.
func TestLSPToolWithRealGopls(t *testing.T) {
	t.Skip("integration test — run manually with gopls installed")
}
