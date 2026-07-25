package i18n

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

const (
	guardModulePath      = "github.com/agent-dance/luban"
	guardI18nImportPath  = guardModulePath + "/i18n"
	guardTypesImportPath = guardModulePath + "/types"
	guardGoTUIPath       = "github.com/grindlemire/go-tui"
)

type guardRule string

const (
	ruleLegacyHelper   guardRule = "legacy-helper"
	ruleForcedEnglish  guardRule = "forced-english"
	ruleDisplayLiteral guardRule = "display-literal"
	ruleException      guardRule = "invalid-exception"
)

var (
	legacyI18nNames = map[string]struct{}{
		"T": {}, "TString": {}, "TCommand": {},
	}
	semanticKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)
	displayMethodArgs  = map[string][]int{
		"Text": {0}, "Thinking": {0}, "Info": {0}, "Warning": {0},
		"Error": {0}, "Success": {0}, "Bold": {0},
		"TextAtEpoch": {1}, "ThinkingAtEpoch": {1}, "InfoAtEpoch": {1},
		"WarningAtEpoch": {1}, "ErrorAtEpoch": {1}, "SuccessAtEpoch": {1},
	}
	displayStructFields = map[guardTypeID]map[string]struct{}{
		{Package: guardTypesImportPath, Name: "ToolResult"}: {
			"Content": {},
		},
		{Package: guardTypesImportPath, Name: "ToolResultBlock"}: {
			"Content": {},
		},
		{Package: guardModulePath + "/tui", Name: "Message"}: {
			"Text": {},
		},
		{Package: guardModulePath + "/commands", Name: "CommandPresentation"}: {
			"Summary": {}, "Result": {}, "NextAction": {},
		},
		{Package: guardModulePath + "/commands", Name: "CommandDomainResult"}: {
			"Result": {}, "NextAction": {},
		},
		{Package: guardModulePath + "/commands", Name: "CommandPresentationSection"}: {
			"Label": {}, "Text": {},
		},
		{Package: guardModulePath + "/commands", Name: "CommandPresentationContract"}: {
			"CompletedNextAction": {}, "FailedNextAction": {},
		},
		{Package: guardModulePath + "/internal/contracts/permission", Name: "PermissionRequest"}: {
			"Action": {}, "Impact": {}, "RiskReason": {}, "RuleSource": {},
			"ApprovalScope": {}, "Body": {}, "Description": {}, "Message": {},
			"ReviewDetails": {},
		},
		{Package: guardModulePath + "/types", Name: "ToolPermissionResult"}: {
			"Message": {},
		},
		{Package: guardModulePath + "/internal/contracts/stream", Name: "Event"}: {
			"Text": {},
		},
		{Package: guardModulePath + "/internal/contracts/stream", Name: "ProgressEvent"}: {
			"Message": {},
		},
		{Package: guardModulePath + "/internal/contracts/stream", Name: "CompactBoundaryEvent"}: {
			"UserDisplayMessage": {},
		},
		{Package: guardModulePath + "/internal/contracts/stream", Name: "TombstoneEvent"}: {
			"Summary": {},
		},
	}
)

type guardTypeID struct {
	Package string
	Name    string
}

type guardInput struct {
	Path        string
	PackagePath string
	Source      []byte
	IsTest      bool
}

type guardParsedFile struct {
	guardInput
	AST          *ast.File
	FileSet      *token.FileSet
	Imports      map[string]string
	StringValues map[string]ast.Expr
	Exceptions   []*guardException
}

type guardViolation struct {
	Rule    guardRule
	Path    string
	Line    int
	Column  int
	Message string
	Anchor  int
}

func (v guardViolation) String() string {
	return fmt.Sprintf("%s:%d:%d: i18n/%s: %s", v.Path, v.Line, v.Column, v.Rule, v.Message)
}

type guardException struct {
	Line     int
	Rule     guardRule
	Category string
	Reason   string
	Used     bool
	Raw      string
}

type guardKeyDeclaration struct {
	Name string
	Key  Key
	Path string
	Line int
}

func TestI18nSourceGuard(t *testing.T) {
	root := guardRepositoryRoot(t)
	inputs, err := guardLoadRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	violations := guardScan(inputs)
	if len(violations) == 0 {
		return
	}
	var report strings.Builder
	report.WriteString("i18n source guard found violations:\n")
	for _, violation := range violations {
		report.WriteString("  - ")
		report.WriteString(violation.String())
		report.WriteByte('\n')
	}
	t.Fatal(report.String())
}

func TestI18nSemanticKeyDeclarationsMatchCatalog(t *testing.T) {
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
	root := guardRepositoryRoot(t)
	inputs, err := guardLoadRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	declarations, violations := guardCollectKeyDeclarations(inputs)
	if len(violations) > 0 {
		for _, violation := range violations {
			t.Errorf("%s", violation.String())
		}
	}

	for _, problem := range guardValidateKeyCatalog(declarations, semanticTranslations) {
		t.Error(problem)
	}
}

func guardRepositoryRoot(t testing.TB) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate i18n source guard")
	}
	return filepath.Dir(filepath.Dir(filename))
}

func guardLoadRepository(root string) ([]guardInput, error) {
	var inputs []guardInput
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			name := entry.Name()
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			} else if !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		directory := filepath.ToSlash(filepath.Dir(relative))
		packagePath := guardModulePath
		if directory != "." {
			packagePath += "/" + directory
		}
		inputs = append(inputs, guardInput{
			Path: filepath.ToSlash(relative), PackagePath: packagePath, Source: source,
			IsTest: strings.HasSuffix(path, "_test.go"),
		})
		return nil
	})
	return inputs, err
}

func guardScan(inputs []guardInput) []guardViolation {
	files, parseViolations := guardParse(inputs)
	localizers := guardCollectLocalizerNames(files)
	violations := append([]guardViolation(nil), parseViolations...)
	for _, file := range files {
		violations = append(violations, guardScanFile(file, localizers)...)
	}
	guardSortViolations(violations)
	return violations
}

func guardParse(inputs []guardInput) ([]*guardParsedFile, []guardViolation) {
	files := make([]*guardParsedFile, 0, len(inputs))
	var violations []guardViolation
	for _, input := range inputs {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, input.Path, input.Source, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			violations = append(violations, guardViolation{Rule: ruleException, Path: input.Path, Line: 1, Column: 1, Message: "parse source: " + err.Error()})
			continue
		}
		file := &guardParsedFile{
			guardInput: input, AST: parsed, FileSet: fileSet,
			Imports: guardImports(parsed), StringValues: guardTopLevelStringValues(parsed),
		}
		if !input.IsTest {
			file.Exceptions, violations = guardParseExceptions(file, violations)
		}
		files = append(files, file)
	}
	return files, violations
}

func guardTopLevelStringValues(file *ast.File) map[string]ast.Expr {
	values := make(map[string]ast.Expr)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || (general.Tok != token.CONST && general.Tok != token.VAR) {
			continue
		}
		for _, item := range general.Specs {
			specification, ok := item.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range specification.Names {
				if index < len(specification.Values) {
					values[name.Name] = specification.Values[index]
				}
			}
		}
	}
	return values
}

func guardImports(file *ast.File) map[string]string {
	imports := make(map[string]string, len(file.Imports))
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		// The import path ends in go-tui, but the package clause is "tui".
		// Keep this explicit rather than guessing package names from paths.
		if path == guardGoTUIPath {
			name = "tui"
		}
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = path
	}
	return imports
}

func guardCollectLocalizerNames(files []*guardParsedFile) map[string]struct{} {
	names := map[string]struct{}{"Text": {}, "Format": {}}
	for _, file := range files {
		if file.PackagePath != guardI18nImportPath || file.IsTest {
			continue
		}
		for _, declaration := range file.AST.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Type.Params == nil || len(function.Type.Params.List) == 0 {
				continue
			}
			if !guardTypeNamed(function.Type.Params.List[0].Type, "Language") || !guardReturnsString(function.Type.Results) {
				continue
			}
			names[function.Name.Name] = struct{}{}
		}
	}
	return names
}

func guardTypeNamed(expression ast.Expr, name string) bool {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name == name
	case *ast.SelectorExpr:
		return value.Sel.Name == name
	default:
		return false
	}
}

func guardReturnsString(results *ast.FieldList) bool {
	if results == nil || len(results.List) != 1 {
		return false
	}
	field := results.List[0]
	identifier, ok := field.Type.(*ast.Ident)
	return ok && identifier.Name == "string"
}

func guardScanFile(file *guardParsedFile, localizers map[string]struct{}) []guardViolation {
	var violations []guardViolation

	for _, spec := range file.AST.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err == nil && path == guardI18nImportPath && spec.Name != nil && spec.Name.Name == "." {
			violations = append(violations, file.violation(ruleLegacyHelper, spec, spec, "dot-importing the i18n package can bypass the legacy-helper guard"))
		}
	}

	ast.Inspect(file.AST, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.SelectorExpr:
			if identifier, ok := value.X.(*ast.Ident); ok && file.Imports[identifier.Name] == guardI18nImportPath {
				if _, legacy := legacyI18nNames[value.Sel.Name]; legacy {
					violations = append(violations, file.violation(ruleLegacyHelper, value, value, "legacy i18n."+value.Sel.Name+" is forbidden; use a semantic Key with Text or Format"))
				}
			}
		case *ast.CallExpr:
			if file.PackagePath == guardI18nImportPath {
				if identifier, ok := value.Fun.(*ast.Ident); ok {
					if _, legacy := legacyI18nNames[identifier.Name]; legacy {
						violations = append(violations, file.violation(ruleLegacyHelper, value, value, "legacy "+identifier.Name+" is forbidden; use a semantic Key with Text or Format"))
					}
				}
			}
			if !file.IsTest && guardCallForcesEnglish(file, value, localizers) {
				violation := file.violation(ruleForcedEnglish, value, value, "user-visible copy is forced to LangEN; pass the active runtime language")
				if !file.suppress(&violation) {
					violations = append(violations, violation)
				}
			}
			if !file.IsTest {
				violations = append(violations, guardDisplayCallViolations(file, value)...)
			}
		case *ast.CompositeLit:
			if !file.IsTest {
				violations = append(violations, guardDisplayCompositeViolations(file, value)...)
			}
		}
		return true
	})

	if !file.IsTest {
		violations = append(violations, guardDisplayAssignmentViolations(file)...)
		for _, exception := range file.Exceptions {
			if exception.Used {
				continue
			}
			position := token.Position{Filename: file.Path, Line: exception.Line, Column: 1}
			violations = append(violations, guardViolation{
				Rule: ruleException, Path: file.Path, Line: position.Line, Column: position.Column,
				Message: "unused i18n exception: " + exception.Raw,
			})
		}
	}
	return violations
}

func guardCallForcesEnglish(file *guardParsedFile, call *ast.CallExpr, localizers map[string]struct{}) bool {
	if len(call.Args) == 0 || !guardIsLangEN(file, call.Args[0]) {
		return false
	}
	name, importPath := guardCallName(file, call)
	if importPath == guardI18nImportPath {
		_, localized := localizers[name]
		return localized
	}
	return strings.HasSuffix(name, "InLanguage") || name == "Localized"
}

func guardIsLangEN(file *guardParsedFile, expression ast.Expr) bool {
	if parenthesized, ok := expression.(*ast.ParenExpr); ok {
		return guardIsLangEN(file, parenthesized.X)
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		return file.PackagePath == guardI18nImportPath && identifier.Name == "LangEN"
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "LangEN" {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && file.Imports[identifier.Name] == guardI18nImportPath
}

func guardCallName(file *guardParsedFile, call *ast.CallExpr) (string, string) {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		return function.Name, file.PackagePath
	case *ast.SelectorExpr:
		if identifier, ok := function.X.(*ast.Ident); ok {
			return function.Sel.Name, file.Imports[identifier.Name]
		}
		return function.Sel.Name, ""
	default:
		return "", ""
	}
}

func guardDisplayCallViolations(file *guardParsedFile, call *ast.CallExpr) []guardViolation {
	name, importPath := guardCallName(file, call)
	var indexes []int

	switch {
	case importPath == guardGoTUIPath && (name == "WithText" || name == "WithTextPrefix" || name == "WithTextAreaPlaceholder"):
		indexes = []int{0}
	case importPath == "fmt" && (name == "Print" || name == "Printf" || name == "Println"):
		indexes = guardIndexesFrom(0, len(call.Args))
	case importPath == "fmt" && (name == "Fprint" || name == "Fprintf" || name == "Fprintln") && len(call.Args) > 1 && guardIsStdStream(file, call.Args[0]):
		indexes = guardIndexesFrom(1, len(call.Args))
	case importPath == "log" && (name == "Print" || name == "Printf" || name == "Println"):
		indexes = []int{0}
	case importPath == "log/slog" && (name == "Debug" || name == "Info" || name == "Warn" || name == "Error"):
		indexes = []int{0}
	case importPath == "log/slog" && (name == "DebugContext" || name == "InfoContext" || name == "WarnContext" || name == "ErrorContext"):
		indexes = []int{1}
	case importPath == "net/http" && name == "Error":
		indexes = []int{1}
	case importPath == "flag" && guardIsFlagDefinition(name):
		indexes = []int{len(call.Args) - 1}
	case importPath == "" && guardIsFlagDefinition(name) && guardIsInstanceMethod(file, call.Fun):
		indexes = []int{len(call.Args) - 1}
	case importPath == "":
		if methodIndexes, ok := displayMethodArgs[name]; ok && guardIsInstanceMethod(file, call.Fun) {
			indexes = methodIndexes
		}
	}

	var violations []guardViolation
	for _, index := range indexes {
		if index < 0 || index >= len(call.Args) {
			continue
		}
		for _, literal := range guardEnglishLiterals(file, call.Args[index]) {
			message := fmt.Sprintf("direct English literal %q reaches display sink %s; use Text/Format or an auditable exception", literal.Text, name)
			violation := file.violation(ruleDisplayLiteral, literal.Node, call, message)
			if !file.suppress(&violation) {
				violations = append(violations, violation)
			}
		}
	}
	return violations
}

func guardIsInstanceMethod(file *guardParsedFile, expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if identifier, ok := selector.X.(*ast.Ident); ok {
		_, importedPackage := file.Imports[identifier.Name]
		return !importedPackage
	}
	return true
}

func guardIsStdStream(file *guardParsedFile, expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || (selector.Sel.Name != "Stdout" && selector.Sel.Name != "Stderr") {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && file.Imports[identifier.Name] == "os"
}

func guardIsFlagDefinition(name string) bool {
	switch name {
	case "String", "StringVar", "Bool", "BoolVar", "Int", "IntVar", "Int64", "Int64Var",
		"Uint", "UintVar", "Uint64", "Uint64Var", "Float64", "Float64Var",
		"Duration", "DurationVar", "TextVar", "Func", "BoolFunc":
		return true
	default:
		return false
	}
}

func guardIndexesFrom(start, end int) []int {
	if start >= end {
		return nil
	}
	indexes := make([]int, 0, end-start)
	for index := start; index < end; index++ {
		indexes = append(indexes, index)
	}
	return indexes
}

func guardDisplayCompositeViolations(file *guardParsedFile, literal *ast.CompositeLit) []guardViolation {
	typeID, ok := guardResolveType(file, literal.Type)
	if !ok {
		return nil
	}
	fields := displayStructFields[typeID]
	if len(fields) == 0 {
		return nil
	}
	var violations []guardViolation
	for _, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := field.Key.(*ast.Ident)
		if !ok {
			continue
		}
		if _, display := fields[name.Name]; !display {
			continue
		}
		violations = append(violations, guardDisplayExpressionViolations(file, field.Value, literal, typeID.Name+"."+name.Name)...)
	}
	return violations
}

func guardDisplayAssignmentViolations(file *guardParsedFile) []guardViolation {
	var violations []guardViolation
	for _, declaration := range file.AST.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		bindings := guardCollectTypeBindings(file, function)
		ast.Inspect(function.Body, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for index, left := range assignment.Lhs {
				if index >= len(assignment.Rhs) {
					break
				}
				selector, ok := left.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				receiver, ok := selector.X.(*ast.Ident)
				if !ok {
					continue
				}
				typeID, ok := bindings[receiver.Name]
				if !ok {
					continue
				}
				if _, display := displayStructFields[typeID][selector.Sel.Name]; !display {
					continue
				}
				violations = append(violations, guardDisplayExpressionViolations(file, assignment.Rhs[index], assignment, typeID.Name+"."+selector.Sel.Name)...)
			}
			return true
		})
	}
	return violations
}

func guardCollectTypeBindings(file *guardParsedFile, function *ast.FuncDecl) map[string]guardTypeID {
	bindings := make(map[string]guardTypeID)
	guardBindFieldList(file, bindings, function.Type.Params)
	if function.Recv != nil {
		guardBindFieldList(file, bindings, function.Recv)
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.DeclStmt:
			declaration, ok := value.Decl.(*ast.GenDecl)
			if !ok || declaration.Tok != token.VAR {
				return true
			}
			for _, item := range declaration.Specs {
				specification, ok := item.(*ast.ValueSpec)
				if !ok || specification.Type == nil {
					continue
				}
				typeID, ok := guardResolveType(file, specification.Type)
				if !ok {
					continue
				}
				for _, name := range specification.Names {
					bindings[name.Name] = typeID
				}
			}
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				if index >= len(value.Rhs) {
					break
				}
				name, ok := left.(*ast.Ident)
				if !ok {
					continue
				}
				if typeID, ok := guardExpressionType(file, value.Rhs[index]); ok {
					bindings[name.Name] = typeID
				}
			}
		}
		return true
	})
	return bindings
}

func guardBindFieldList(file *guardParsedFile, bindings map[string]guardTypeID, fields *ast.FieldList) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		typeID, ok := guardResolveType(file, field.Type)
		if !ok {
			continue
		}
		for _, name := range field.Names {
			bindings[name.Name] = typeID
		}
	}
}

func guardExpressionType(file *guardParsedFile, expression ast.Expr) (guardTypeID, bool) {
	switch value := expression.(type) {
	case *ast.CompositeLit:
		return guardResolveType(file, value.Type)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return guardExpressionType(file, value.X)
		}
	case *ast.TypeAssertExpr:
		if value.Type != nil {
			return guardResolveType(file, value.Type)
		}
	}
	return guardTypeID{}, false
}

func guardResolveType(file *guardParsedFile, expression ast.Expr) (guardTypeID, bool) {
	switch value := expression.(type) {
	case *ast.Ident:
		return guardTypeID{Package: file.PackagePath, Name: value.Name}, true
	case *ast.SelectorExpr:
		identifier, ok := value.X.(*ast.Ident)
		if !ok {
			return guardTypeID{}, false
		}
		packagePath, ok := file.Imports[identifier.Name]
		return guardTypeID{Package: packagePath, Name: value.Sel.Name}, ok
	case *ast.StarExpr:
		return guardResolveType(file, value.X)
	default:
		return guardTypeID{}, false
	}
}

func guardDisplayExpressionViolations(file *guardParsedFile, expression ast.Expr, anchor ast.Node, sink string) []guardViolation {
	var violations []guardViolation
	for _, literal := range guardEnglishLiterals(file, expression) {
		message := fmt.Sprintf("direct English literal %q reaches display field %s; use Text/Format or an auditable exception", literal.Text, sink)
		violation := file.violation(ruleDisplayLiteral, literal.Node, anchor, message)
		if !file.suppress(&violation) {
			violations = append(violations, violation)
		}
	}
	return violations
}

type guardLiteral struct {
	Node *ast.BasicLit
	Text string
}

func guardEnglishLiterals(file *guardParsedFile, expression ast.Expr) []guardLiteral {
	var literals []guardLiteral
	visited := make(map[string]bool)
	var inspectExpression func(ast.Expr)
	inspectExpression = func(current ast.Expr) {
		ast.Inspect(current, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok && guardCallIsLocalized(file, call) {
				return false
			}
			if identifier, ok := node.(*ast.Ident); ok {
				if replacement, exists := file.StringValues[identifier.Name]; exists && !visited[identifier.Name] {
					visited[identifier.Name] = true
					inspectExpression(replacement)
					return false
				}
			}
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(literal.Value)
			if err == nil && guardContainsASCIICopy(text) {
				literals = append(literals, guardLiteral{Node: literal, Text: text})
			}
			return false
		})
	}
	inspectExpression(expression)
	return literals
}

func guardCallIsLocalized(file *guardParsedFile, call *ast.CallExpr) bool {
	name, importPath := guardCallName(file, call)
	if importPath == guardI18nImportPath && (name == "Text" || name == "Format") {
		return true
	}
	return strings.HasSuffix(name, "InLanguage") || name == "Localized"
}

func guardContainsASCIICopy(text string) bool {
	text = guardStripPrintfDirectives(text)
	for _, character := range text {
		if unicode.IsLetter(character) {
			return true
		}
	}
	return false
}

func guardStripPrintfDirectives(text string) string {
	var result strings.Builder
	for index := 0; index < len(text); {
		if text[index] != '%' {
			result.WriteByte(text[index])
			index++
			continue
		}
		if index+1 < len(text) && text[index+1] == '%' {
			result.WriteByte('%')
			index += 2
			continue
		}
		index++
		for index < len(text) {
			character := text[index]
			index++
			if unicode.IsLetter(rune(character)) || character == '%' {
				break
			}
		}
	}
	return result.String()
}

func (file *guardParsedFile) violation(rule guardRule, node, anchor ast.Node, message string) guardViolation {
	position := file.FileSet.Position(node.Pos())
	anchorPosition := file.FileSet.Position(anchor.Pos())
	return guardViolation{Rule: rule, Path: file.Path, Line: position.Line, Column: position.Column, Message: message, Anchor: anchorPosition.Line}
}

func (file *guardParsedFile) suppress(violation *guardViolation) bool {
	for _, exception := range file.Exceptions {
		if exception.Rule != violation.Rule || (exception.Line != violation.Anchor && exception.Line != violation.Anchor-1) {
			continue
		}
		exception.Used = true
		return true
	}
	return false
}

func guardParseExceptions(file *guardParsedFile, violations []guardViolation) ([]*guardException, []guardViolation) {
	var exceptions []*guardException
	for _, group := range file.AST.Comments {
		for _, comment := range group.List {
			content := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(comment.Text), "//"), "*/"))
			content = strings.TrimSpace(strings.TrimPrefix(content, "/*"))
			if !strings.HasPrefix(content, "i18n:allow") {
				continue
			}
			position := file.FileSet.Position(comment.Pos())
			exception, err := guardParseException(content, position.Line)
			if err != nil {
				violations = append(violations, guardViolation{Rule: ruleException, Path: file.Path, Line: position.Line, Column: position.Column, Message: err.Error()})
				continue
			}
			exceptions = append(exceptions, exception)
		}
	}
	return exceptions, violations
}

func guardParseException(content string, line int) (*guardException, error) {
	const prefix = "i18n:allow "
	if !strings.HasPrefix(content, prefix) {
		return nil, fmt.Errorf("malformed i18n exception %q", content)
	}
	parts := strings.SplitN(strings.TrimPrefix(content, prefix), " -- ", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return nil, fmt.Errorf("i18n exception must include a reason after ' -- ': %q", content)
	}
	header := strings.Fields(parts[0])
	if len(header) != 2 {
		return nil, fmt.Errorf("i18n exception must name one rule and category: %q", content)
	}
	rule := guardRule(header[0])
	category := header[1]
	allowed := false
	switch rule {
	case ruleDisplayLiteral:
		switch category {
		case "protocol", "raw-output", "brand", "identifier", "ascii-logo", "path":
			allowed = true
		}
	case ruleForcedEnglish:
		allowed = category == "wire-compat"
	}
	if !allowed {
		return nil, fmt.Errorf("unsupported i18n exception rule/category %q %q", rule, category)
	}
	return &guardException{Line: line, Rule: rule, Category: category, Reason: strings.TrimSpace(parts[1]), Raw: content}, nil
}

func guardCollectKeyDeclarations(inputs []guardInput) ([]guardKeyDeclaration, []guardViolation) {
	files, violations := guardParse(inputs)
	var declarations []guardKeyDeclaration
	for _, file := range files {
		if file.PackagePath != guardI18nImportPath || file.IsTest {
			continue
		}
		for _, declaration := range file.AST.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, item := range general.Specs {
				specification, ok := item.(*ast.ValueSpec)
				if !ok || !guardTypeNamed(specification.Type, "Key") {
					continue
				}
				for index, name := range specification.Names {
					position := file.FileSet.Position(name.Pos())
					if index >= len(specification.Values) {
						violations = append(violations, guardViolation{Rule: ruleException, Path: file.Path, Line: position.Line, Column: position.Column, Message: "semantic Key constant requires an explicit string value"})
						continue
					}
					literal, ok := specification.Values[index].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						violations = append(violations, guardViolation{Rule: ruleException, Path: file.Path, Line: position.Line, Column: position.Column, Message: "semantic Key constant must use a string literal"})
						continue
					}
					value, err := strconv.Unquote(literal.Value)
					if err != nil {
						violations = append(violations, guardViolation{Rule: ruleException, Path: file.Path, Line: position.Line, Column: position.Column, Message: "decode semantic Key: " + err.Error()})
						continue
					}
					declarations = append(declarations, guardKeyDeclaration{Name: name.Name, Key: Key(value), Path: file.Path, Line: position.Line})
				}
			}
		}
	}
	return declarations, violations
}

func guardValidateKeyCatalog(declarations []guardKeyDeclaration, catalog map[Key]map[Language]string) []string {
	var problems []string
	declared := make(map[Key]guardKeyDeclaration, len(declarations))
	for _, declaration := range declarations {
		if previous, exists := declared[declaration.Key]; exists {
			problems = append(problems, fmt.Sprintf(
				"duplicate semantic key value %q: %s at %s:%d and %s at %s:%d",
				declaration.Key, previous.Name, previous.Path, previous.Line,
				declaration.Name, declaration.Path, declaration.Line,
			))
			continue
		}
		declared[declaration.Key] = declaration
		if !semanticKeyPattern.MatchString(string(declaration.Key)) {
			problems = append(problems, fmt.Sprintf(
				"%s:%d: semantic key %s has non-semantic value %q",
				declaration.Path, declaration.Line, declaration.Name, declaration.Key,
			))
		}
	}
	for key, declaration := range declared {
		if _, registered := catalog[key]; !registered {
			problems = append(problems, fmt.Sprintf(
				"%s:%d: semantic key %s (%q) is declared but not registered",
				declaration.Path, declaration.Line, declaration.Name, key,
			))
		}
	}
	for key := range catalog {
		if _, exists := declared[key]; !exists {
			problems = append(problems, fmt.Sprintf(
				"semantic catalog contains %q without a typed Key constant declaration", key,
			))
		}
	}
	sort.Strings(problems)
	return problems
}

func guardSortViolations(violations []guardViolation) {
	sort.Slice(violations, func(left, right int) bool {
		if violations[left].Path != violations[right].Path {
			return violations[left].Path < violations[right].Path
		}
		if violations[left].Line != violations[right].Line {
			return violations[left].Line < violations[right].Line
		}
		if violations[left].Column != violations[right].Column {
			return violations[left].Column < violations[right].Column
		}
		return violations[left].Rule < violations[right].Rule
	})
}
