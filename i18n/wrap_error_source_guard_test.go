package i18n

import (
	"fmt"
	"go/ast"
	"regexp"
	"sort"
	"strconv"
	"testing"
)

var wrapErrorFormatVerb = regexp.MustCompile(`%(?:\[([0-9]+)\])?[-+# 0]*(?:[0-9]+|\*)?(?:\.(?:[0-9]+|\*))?[A-Za-z]`)

// TestWrapErrorCatalogContracts prevents a semantic wrapper from appending a
// visible cause to a key that has no matching format slot. Such calls compile,
// but fmt renders `%!(EXTRA ...)` or silently mixes internal English into a
// localized message. Hidden causes must use WrapInternalError instead.
func TestWrapErrorCatalogContracts(t *testing.T) {
	root := guardRepositoryRoot(t)
	inputs, err := guardLoadRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	declarations, declarationViolations := guardCollectKeyDeclarations(inputs)
	if len(declarationViolations) != 0 {
		t.Fatalf("cannot inspect WrapError contracts with invalid key declarations: %v", declarationViolations)
	}
	keysByName := make(map[string]Key, len(declarations))
	for _, declaration := range declarations {
		keysByName[declaration.Name] = declaration.Key
	}

	files, parseViolations := guardParse(inputs)
	if len(parseViolations) != 0 {
		t.Fatalf("cannot inspect WrapError contracts with parse failures: %v", parseViolations)
	}
	var problems []string
	for _, file := range files {
		if file.IsTest {
			continue
		}
		ast.Inspect(file.AST, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || call.Ellipsis.IsValid() || len(call.Args) < 2 {
				return true
			}
			name, importPath := guardCallName(file, call)
			if name != "WrapError" || importPath != guardI18nImportPath {
				return true
			}
			keyName := wrapErrorKeyName(file, call.Args[0])
			key, known := keysByName[keyName]
			if !known {
				return true // a dynamic key is checked by its focused constructor test
			}
			format := semanticTranslations[key][LangEN]
			want := len(call.Args) - 1 // explicit args plus the appended cause
			got := wrapErrorFormatArgumentCount(format)
			if got != want {
				position := file.FileSet.Position(call.Pos())
				problems = append(problems, fmt.Sprintf(
					"%s:%d: WrapError(%s) renders %d arguments but key %q consumes %d; use WrapInternalError for a hidden cause or fix the semantic format",
					file.Path, position.Line, keyName, want, key, got,
				))
			}
			return true
		})
	}
	if len(problems) != 0 {
		sort.Strings(problems)
		for _, problem := range problems {
			t.Error(problem)
		}
	}
}

func wrapErrorKeyName(file *guardParsedFile, expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		identifier, ok := value.X.(*ast.Ident)
		if ok && file.Imports[identifier.Name] == guardI18nImportPath {
			return value.Sel.Name
		}
	case *ast.Ident:
		if file.PackagePath == guardI18nImportPath {
			return value.Name
		}
	}
	return ""
}

func wrapErrorFormatArgumentCount(format string) int {
	sequential, maximum := 0, 0
	for _, match := range wrapErrorFormatVerb.FindAllStringSubmatch(format, -1) {
		if match[0] == "%%" {
			continue
		}
		if match[1] != "" {
			index, err := strconv.Atoi(match[1])
			if err == nil {
				sequential = index
				if index > maximum {
					maximum = index
				}
			}
			continue
		}
		sequential++
		if sequential > maximum {
			maximum = sequential
		}
	}
	return maximum
}
