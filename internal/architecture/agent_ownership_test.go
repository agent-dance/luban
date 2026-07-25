package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var agentImplementationTypes = map[string]struct{}{
	"AgentTool":                         {},
	"AgentSourceGroup":                  {},
	"AgentDefinition":                   {},
	"AgentSessionRuntime":               {},
	"AgentProgressEmitter":              {},
	"AgentResultKind":                   {},
	"AgentResult":                       {},
	"AgentResultBase":                   {},
	"AgentCompleted":                    {},
	"AgentError":                        {},
	"AgentAborted":                      {},
	"AgentPartial":                      {},
	"AgentIncomplete":                   {},
	"MCPReadinessProbe":                 {},
	"MCPReadinessReport":                {},
	"BackgroundTaskOutput":              {},
	"BackgroundTask":                    {},
	"BackgroundTaskManager":             {},
	"RuntimeNotificationSink":           {},
	"RuntimeNotificationSinkFunc":       {},
	"RuntimeNotificationFollowUpTarget": {},
	"AgentBackgroundRunner":             {},
	"ScheduledAgentRunner":              {},
}

func TestAgentImplementationOwnership(t *testing.T) {
	root := architectureModuleRoot(t)
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && shouldSkipSourceDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		imports := sourceImports(file)
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if typeSpec.Name.Name == "AgentTool" && !strings.HasPrefix(rel, "internal/agent/") {
					violations = append(violations, rel+" declares agent implementation type "+typeSpec.Name.Name)
				}
				if _, owned := agentImplementationTypes[typeSpec.Name.Name]; owned &&
					typeSpec.Name.Name != "AgentTool" && strings.HasPrefix(rel, "tools/") {
					violations = append(violations, rel+" restores agent-owned type "+typeSpec.Name.Name)
				}
				if strings.HasPrefix(rel, "tools/") && typeSpec.Assign.IsValid() && aliasesAgentLayer(typeSpec.Type, imports) {
					violations = append(violations, rel+" aliases an agent-owned type as "+typeSpec.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("invalid agent implementation ownership:\n%s", strings.Join(violations, "\n"))
	}
}

func architectureModuleRoot(t *testing.T) string {
	t.Helper()
	goMod, err := os.ReadFile("../../go.mod")
	if err == nil && strings.Contains(string(goMod), "module "+modulePath) {
		root, err := filepath.Abs("../..")
		if err != nil {
			t.Fatal(err)
		}
		return root
	}
	t.Fatalf("cannot locate %s module root", modulePath)
	return ""
}

func shouldSkipSourceDirectory(name string) bool {
	return name == "vendor" || strings.HasPrefix(name, ".")
}

func sourceImports(file *ast.File) map[string]string {
	imports := make(map[string]string, len(file.Imports))
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = path
	}
	return imports
}

func aliasesAgentLayer(expr ast.Expr, imports map[string]string) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		path := imports[ident.Name]
		if path == modulePath+"/internal/agent" ||
			path == modulePath+"/internal/contracts/agent" ||
			path == modulePath+"/internal/store/runtime" {
			found = true
			return false
		}
		return true
	})
	return found
}
