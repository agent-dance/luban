package tui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestProductionRuntimeHasNoUnownedTerminalWrites prevents a regression where
// a background runtime path prints an error directly into a fullscreen input
// row. Tests are excluded. The remaining exceptions are output owners with a
// deliberately different lifecycle: standalone CLI binaries, print/screen-
// reader/protocol bootstrap, go-tui's owner constructor and one-shot print
// surface, and external editors run while fullscreen ownership is released.
func TestProductionRuntimeHasNoUnownedTerminalWrites(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate terminal writer guard")
	}
	repositoryRoot, rootErr := terminalGuardRepositoryRoot(filename)
	if rootErr != nil {
		t.Fatal(rootErr)
	}
	interactiveLoggingProtected, protectionErr := terminalGuardInteractiveDiagnosticLifecycle(repositoryRoot)
	if protectionErr != nil {
		t.Fatal(protectionErr)
	}
	fileSet := token.NewFileSet()
	var violations []string
	walkErr := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if terminalGuardExcludedDirectory(relative) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(relative, ".go") || strings.HasSuffix(relative, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return err
		}
		imports := terminalGuardImports(parsed)
		inspectScope := func(root ast.Node, scope string) {
			ast.Inspect(root, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.CallExpr:
					if terminalGuardImplicitPrint(typed.Fun, imports) && !terminalGuardAllowed(relative, scope) {
						position := fileSet.Position(typed.Pos())
						violations = append(violations, fmt.Sprintf("%s:%d: implicit stdout/stderr print in %s", relative, position.Line, scope))
					}
					if terminalGuardDefaultLoggerPrint(typed.Fun, imports) && !interactiveLoggingProtected && !terminalGuardAllowed(relative, scope) {
						position := fileSet.Position(typed.Pos())
						violations = append(violations, fmt.Sprintf("%s:%d: default log/slog call lacks interactive sink ownership in %s", relative, position.Line, scope))
					}
					if terminalGuardLoggerTerminalSink(typed, imports) && !terminalGuardAllowed(relative, scope) {
						position := fileSet.Position(typed.Pos())
						violations = append(violations, fmt.Sprintf("%s:%d: logger is routed directly to process stdout/stderr in %s", relative, position.Line, scope))
					}
				case *ast.SelectorExpr:
					identifier, ok := typed.X.(*ast.Ident)
					if ok && terminalGuardImportedPackage(identifier, imports) == "os" && (typed.Sel.Name == "Stdout" || typed.Sel.Name == "Stderr") && !terminalGuardAllowed(relative, scope) {
						position := fileSet.Position(typed.Pos())
						violations = append(violations, fmt.Sprintf("%s:%d: direct os.%s access in %s", relative, position.Line, typed.Sel.Name, scope))
					}
				}
				return true
			})
		}
		for _, declaration := range parsed.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				if typed.Body != nil {
					inspectScope(typed.Body, typed.Name.Name)
				}
			case *ast.GenDecl:
				inspectScope(typed, "<package>")
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		t.Fatalf("production runtime bypasses the terminal owner:\n%s", strings.Join(violations, "\n"))
	}
}

// terminalGuardInteractiveDiagnosticLifecycle proves that default log and
// slog calls are redirected before any interactive background service starts,
// remain redirected through both interactive transports, and are restored at
// lifecycle exit. A default logger call is safe only while this proof holds.
func terminalGuardInteractiveDiagnosticLifecycle(repositoryRoot string) (bool, error) {
	diagnosticSource, err := os.ReadFile(filepath.Join(repositoryRoot, "internal", "app", "interactive_diagnostics.go"))
	if err != nil {
		return false, err
	}
	diagnostic := string(diagnosticSource)
	for _, required := range []string{
		"log.SetOutput(sink)",
		"slog.SetDefault(slog.New(slog.NewTextHandler(sink, nil)))",
		"log.SetOutput(previousLogWriter)",
		"slog.SetDefault(previousSlog)",
	} {
		if !strings.Contains(diagnostic, required) {
			return false, fmt.Errorf("interactive diagnostic lifecycle is missing %q", required)
		}
	}

	mainSource, err := os.ReadFile(filepath.Join(repositoryRoot, "internal", "app", "main.go"))
	if err != nil {
		return false, err
	}
	mainText := string(mainSource)
	requiredOrder := []string{
		"interactive := !opts.Print && !opts.SDK",
		"installInteractiveDiagnosticLogger()",
		"defer restoreDiagnosticLogger()",
		"SetupRegistry(",
		"RunScreenReaderREPL(",
		"RunTUIREPL(",
	}
	previous := -1
	for _, marker := range requiredOrder {
		position := strings.Index(mainText, marker)
		if position < 0 || position <= previous {
			return false, fmt.Errorf("interactive diagnostic lifecycle ordering is missing %q", marker)
		}
		previous = position
	}
	return true, nil
}

func terminalGuardRepositoryRoot(filename string) (string, error) {
	for directory := filepath.Dir(filename); ; directory = filepath.Dir(directory) {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("repository root not found from %s", filename)
		}
	}
}

func terminalGuardExcludedDirectory(relative string) bool {
	switch {
	case relative == ".git", relative == ".tmp", relative == "benchmark-results", relative == "vendor", relative == ".luban-code":
		return true
	case relative == "cmd/cache-bench" || strings.HasPrefix(relative, "cmd/cache-bench/"):
		// The standalone benchmark owns its stdout/stderr lifecycle.
		return true
	case relative == "pkg/go-tui/cmd" || strings.HasPrefix(relative, "pkg/go-tui/cmd/"):
		// Standalone formatter/LSP/terminal-fixture protocol surfaces.
		return true
	case relative == "pkg/go-tui/examples" || strings.HasPrefix(relative, "pkg/go-tui/examples/"):
		return true
	default:
		return false
	}
}

func terminalGuardImports(file *ast.File) map[string]string {
	imports := make(map[string]string)
	if file == nil {
		return imports
	}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath == "" {
			continue
		}
		name := filepath.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name != "" && name != "_" && name != "." {
			imports[name] = importPath
		}
	}
	return imports
}

func terminalGuardImportedPackage(identifier *ast.Ident, imports map[string]string) string {
	if identifier == nil {
		return ""
	}
	if importPath := imports[identifier.Name]; importPath != "" {
		return filepath.Base(importPath)
	}
	// Expression-only unit fixtures do not carry import declarations.
	return identifier.Name
}

func terminalGuardImplicitPrint(expression ast.Expr, imports map[string]string) bool {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name == "print" || typed.Name == "println"
	case *ast.SelectorExpr:
		identifier, ok := typed.X.(*ast.Ident)
		if !ok {
			return false
		}
		if terminalGuardImportedPackage(identifier, imports) == "fmt" {
			return typed.Sel.Name == "Print" || typed.Sel.Name == "Printf" || typed.Sel.Name == "Println"
		}
	}
	return false
}

func terminalGuardDefaultLoggerPrint(expression ast.Expr, imports map[string]string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if call, ok := selector.X.(*ast.CallExpr); ok {
		factory, ok := call.Fun.(*ast.SelectorExpr)
		if ok && factory.Sel.Name == "Default" {
			if identifier, ok := factory.X.(*ast.Ident); ok {
				switch terminalGuardImportedPackage(identifier, imports) {
				case "log":
					return terminalGuardLogMethod(selector.Sel.Name)
				case "slog":
					return terminalGuardSlogMethod(selector.Sel.Name)
				}
			}
		}
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	switch terminalGuardImportedPackage(identifier, imports) {
	case "log":
		return terminalGuardLogMethod(selector.Sel.Name)
	case "slog":
		return terminalGuardSlogMethod(selector.Sel.Name)
	}
	return false
}

func terminalGuardLogMethod(name string) bool {
	switch name {
	case "Print", "Printf", "Println", "Fatal", "Fatalf", "Fatalln", "Panic", "Panicf", "Panicln", "Output":
		return true
	default:
		return false
	}
}

func terminalGuardSlogMethod(name string) bool {
	switch name {
	case "Debug", "DebugContext", "Info", "InfoContext", "Warn", "WarnContext", "Error", "ErrorContext", "Log", "LogAttrs":
		return true
	default:
		return false
	}
}

func terminalGuardLoggerTerminalSink(call *ast.CallExpr, imports map[string]string) bool {
	if call == nil || len(call.Args) == 0 || !terminalGuardOSTerminal(call.Args[0], imports) {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if selector.Sel.Name == "SetOutput" {
		return true
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	packageName := terminalGuardImportedPackage(identifier, imports)
	if packageName == "log" && selector.Sel.Name == "New" {
		return true
	}
	return packageName == "slog" && (selector.Sel.Name == "NewTextHandler" || selector.Sel.Name == "NewJSONHandler")
}

func terminalGuardOSTerminal(expression ast.Expr, imports map[string]string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && terminalGuardImportedPackage(identifier, imports) == "os" && (selector.Sel.Name == "Stdout" || selector.Sel.Name == "Stderr")
}

func TestTerminalGuardRecognizesDefaultAndIndirectLoggerWrites(t *testing.T) {
	for _, source := range []string{"log.Print(\"x\")", "log.Printf(\"x\")", "log.Println(\"x\")", "log.Default().Print(\"x\")", "slog.Error(\"x\")", "slog.Default().Warn(\"x\")"} {
		expression, err := parser.ParseExpr(source)
		if err != nil {
			t.Fatal(err)
		}
		call := expression.(*ast.CallExpr)
		if !terminalGuardDefaultLoggerPrint(call.Fun, nil) {
			t.Fatalf("default logger call was not recognized: %s", source)
		}
	}
	for _, source := range []string{
		"log.New(os.Stderr, \"\", 0)",
		"logger.SetOutput(os.Stdout)",
		"slog.NewTextHandler(os.Stderr, nil)",
		"slog.NewJSONHandler(os.Stdout, nil)",
	} {
		expression, err := parser.ParseExpr(source)
		if err != nil {
			t.Fatal(err)
		}
		if !terminalGuardLoggerTerminalSink(expression.(*ast.CallExpr), nil) {
			t.Fatalf("indirect logger terminal sink was not recognized: %s", source)
		}
	}
	aliases := map[string]string{"l": "log", "sl": "log/slog", "sys": "os"}
	for _, source := range []string{"l.Print(\"x\")", "sl.Warn(\"x\")"} {
		expression, err := parser.ParseExpr(source)
		if err != nil {
			t.Fatal(err)
		}
		if !terminalGuardDefaultLoggerPrint(expression.(*ast.CallExpr).Fun, aliases) {
			t.Fatalf("aliased default logger call was not recognized: %s", source)
		}
	}
	expression, err := parser.ParseExpr("l.New(sys.Stderr, \"\", 0)")
	if err != nil {
		t.Fatal(err)
	}
	if !terminalGuardLoggerTerminalSink(expression.(*ast.CallExpr), aliases) {
		t.Fatal("aliased log.New(os.Stderr) was not recognized")
	}
}

func terminalGuardAllowed(path, function string) bool {
	// app.Main and cli are the lifecycle router for print, screen-reader, SDK raw
	// protocol and fullscreen modes. Their writes occur before owner acquisition
	// or after owner restoration; fullscreen work is delegated to repl_tui.go.
	if path == "internal/app/main.go" || strings.HasPrefix(path, "cli/") {
		return true
	}
	if path == "internal/app/printmode.go" {
		return true // explicit non-fullscreen print surface
	}
	switch path + ":" + function {
	case "internal/ui/tui/app.go:OpenFileInEditor":
		return true // go-tui RunExternal releases and reacquires terminal ownership
	case "commands/memory.go:Execute":
		return true // non-TUI fallback; fullscreen injects OpenFileInEditor above
	case "commands/diff.go:isTTY":
		return true // capability probe only; it never writes
	case "pkg/go-tui/app.go:NewApp", "pkg/go-tui/app.go:NewAppWithReader":
		return true // terminal-owner construction
	case "pkg/go-tui/print.go:Print", "pkg/go-tui/print.go:Sprint":
		return true // explicit one-shot print surface
	default:
		return false
	}
}
