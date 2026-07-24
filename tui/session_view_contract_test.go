package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"
)

func TestRootAppStateAccessesAreExplicitlyClassified(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "root.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	used := make(map[string]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		stateSelector, ok := selector.X.(*ast.SelectorExpr)
		if !ok || stateSelector.Sel.Name != "state" {
			return true
		}
		component, ok := stateSelector.X.(*ast.Ident)
		if !ok || component.Name != "c" {
			return true
		}
		used[selector.Sel.Name] = struct{}{}
		return true
	})

	var unknown, stale []string
	for name := range used {
		if _, ok := rootSessionViewAccessContract[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	for name := range rootSessionViewAccessContract {
		if _, ok := used[name]; !ok {
			stale = append(stale, name)
		}
	}
	sort.Strings(unknown)
	sort.Strings(stale)
	if len(unknown) > 0 || len(stale) > 0 {
		t.Fatalf("Root session-view ownership registry drift: unknown=%v stale=%v", unknown, stale)
	}
}
