package permission

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAllowAllHandler(t *testing.T) {
	decision, err := (AllowAllHandler{}).Check(context.Background(), PermissionRequest{Required: true})
	if err != nil {
		t.Fatal(err)
	}
	if decision != PermissionAllow {
		t.Fatalf("decision = %v, want %v", decision, PermissionAllow)
	}
}

func TestPermissionDecisionsAreDistinct(t *testing.T) {
	seen := map[PermissionDecision]bool{
		PermissionAllow:     true,
		PermissionDeny:      true,
		PermissionAllowOnce: true,
	}
	if len(seen) != 3 {
		t.Fatalf("permission decisions are not distinct: %#v", seen)
	}
}

func TestPermissionContractIsTheOnlyInternalAuthority(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	ownedNames := map[string]struct{}{
		"PermissionDecision":  {},
		"PermissionRequest":   {},
		"PermissionHandler":   {},
		"PermissionAllow":     {},
		"PermissionDeny":      {},
		"PermissionAllowOnce": {},
		"AllowAllHandler":     {},
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if relative == ".git" || relative == ".tmp" || relative == "benchmark-results" || strings.HasPrefix(relative, ".luban-code") || relative == "sdk" || relative == "internal/contracts/permission" {
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
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range general.Specs {
				switch typed := spec.(type) {
				case *ast.TypeSpec:
					if _, duplicate := ownedNames[typed.Name.Name]; duplicate {
						t.Errorf("%s redeclares permission contract type %s", path, typed.Name.Name)
					}
				case *ast.ValueSpec:
					for _, name := range typed.Names {
						if _, duplicate := ownedNames[name.Name]; duplicate {
							t.Errorf("%s redeclares permission contract value %s", path, name.Name)
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
