package execution

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestAuthorityIssuanceIsOwnedByRuntimeLoop(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	restricted := map[string]struct{}{"NewOwner": {}, "BeginRun": {}, "EndRun": {}, "Bind": {}}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative == ".git" || relative == ".tmp" || relative == "benchmark-results" || strings.HasPrefix(relative, ".luban-code") || relative == "internal/contracts/execution" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || relative == "internal/runtime/loop" || strings.HasPrefix(relative, "internal/runtime/loop"+string(filepath.Separator)) {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		aliases := map[string]struct{}{}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil || importPath != "github.com/agent-dance/luban/internal/contracts/execution" {
				continue
			}
			alias := "execution"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			if alias == "." {
				t.Errorf("%s dot-imports the execution authority contract", path)
				continue
			}
			aliases[alias] = struct{}{}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, imported := aliases[identifier.Name]; !imported {
				return true
			}
			if _, issuance := restricted[selector.Sel.Name]; issuance {
				t.Errorf("%s calls runtime-owned execution authority API %s", path, selector.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecutionContextHasNoRuntimeWrappersOrAliases(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative == ".git" || relative == ".tmp" || relative == "benchmark-results" || strings.HasPrefix(relative, ".luban-code") || relative == "internal/contracts/execution" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.GenDecl:
				for _, spec := range typed.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Name.Name == "ToolExecutionContext" {
						t.Errorf("%s redeclares ToolExecutionContext", path)
					}
				}
			case *ast.FuncDecl:
				if typed.Name.Name == "WithToolExecutionContext" || typed.Name.Name == "ToolExecutionContextFromContext" {
					t.Errorf("%s wraps execution context API %s", path, typed.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
