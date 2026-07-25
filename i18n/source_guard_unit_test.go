package i18n

import (
	"strings"
	"testing"
)

func TestI18nSourceGuardUnitLegacyHelpers(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "aliased helper assignment",
			source: guardUnitSource(
				"package sample",
				"import loc \"github.com/agent-dance/luban/i18n\"",
				"var translate = loc.T",
			),
			want: 1,
		},
		{
			name: "aliased helper call",
			source: guardUnitSource(
				"package sample",
				"import locale \"github.com/agent-dance/luban/i18n\"",
				"func render() string { return locale.TString(locale.LangEN, \"Ready\") }",
			),
			want: 1,
		},
		{
			name: "removed command helper stays forbidden",
			source: guardUnitSource(
				"package sample",
				"import loc \"github.com/agent-dance/luban/i18n\"",
				"var commandCopy = loc.TCommand",
			),
			want: 1,
		},
		{
			name: "dot import is rejected",
			source: guardUnitSource(
				"package sample",
				"import . \"github.com/agent-dance/luban/i18n\"",
				"func render() string { return T(LangEN, \"Ready\") }",
			),
			want: 1,
		},
		{
			name: "semantic helper is allowed",
			source: guardUnitSource(
				"package sample",
				"import loc \"github.com/agent-dance/luban/i18n\"",
				"func render(lang loc.Language) string { return loc.Text(lang, loc.Key(\"sample.ready\")) }",
			),
			want: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			violations := guardUnitScan(test.source, false)
			if got := guardUnitRuleCount(violations, ruleLegacyHelper); got != test.want {
				t.Fatalf("legacy violations = %d, want %d:\n%s", got, test.want, guardUnitViolationReport(violations))
			}
		})
	}
}

func TestI18nSourceGuardUnitTestsStillRejectLegacyHelpers(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import loc \"github.com/agent-dance/luban/i18n\"",
		"func render(renderer interface{ Info(string) }) {",
		"  _ = loc.T(loc.LangEN, \"Ready\")",
		"  _ = loc.Text(loc.LangEN, loc.Key(\"sample.ready\"))",
		"  renderer.Info(\"Ready\")",
		"}",
	)
	violations := guardUnitScan(source, true)
	if got := len(violations); got != 1 || violations[0].Rule != ruleLegacyHelper {
		t.Fatalf("test files should reject only the legacy helper, got:\n%s", guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitForcedEnglish(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import loc \"github.com/agent-dance/luban/i18n\"",
		"func ReasoningEffortInfoInLanguage(lang loc.Language, effort string) string { return effort }",
		"func render(renderer interface{ Info(string) }, lang loc.Language) {",
		"  renderer.Info(loc.Text(loc.LangEN, loc.Key(\"sample.ready\")))",
		"  _ = loc.Format(loc.LangEN, loc.Key(\"sample.count\"), 1)",
		"  _ = ReasoningEffortInfoInLanguage(loc.LangEN, \"high\")",
		"  renderer.Info(loc.Text(lang, loc.Key(\"sample.ready\")))",
		"}",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleForcedEnglish); got != 3 {
		t.Fatalf("forced-English violations = %d, want 3:\n%s", got, guardUnitViolationReport(violations))
	}
	if got := guardUnitRuleCount(violations, ruleDisplayLiteral); got != 0 {
		t.Fatalf("localized calls must stop display-literal traversal, got %d:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitDisplaySinksAndFields(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import (",
		"  output \"fmt\"",
		"  \"os\"",
		"  loc \"github.com/agent-dance/luban/i18n\"",
		"  permissioncontract \"github.com/agent-dance/luban/internal/contracts/permission\"",
		"  terminal \"github.com/grindlemire/go-tui\"",
		")",
		"func render(renderer interface{ Info(string) }, lang loc.Language, raw string) {",
		"  renderer.Info(\"Ready to continue\")",
		"  terminal.WithText(\"Error details\")",
		"  output.Fprintln(os.Stderr, \"Startup failed\")",
		"  _ = permissioncontract.PermissionRequest{Impact: \"Run the requested tool\"}",
		"  request := permissioncontract.PermissionRequest{}",
		"  request.Action = \"Execute the approved plan\"",
		"  renderer.Info(loc.Text(lang, loc.Key(\"sample.ready\")))",
		"  renderer.Info(raw)",
		"  output.Fprintf(os.Stderr, \"%s\\n\", raw)",
		"}",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleDisplayLiteral); got != 5 {
		t.Fatalf("display-literal violations = %d, want 5:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitToolResultContentFields(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import (",
		"  loc \"github.com/agent-dance/luban/i18n\"",
		"  tooltypes \"github.com/agent-dance/luban/types\"",
		")",
		"func render(lang loc.Language, raw string) {",
		"  _ = tooltypes.ToolResult{Content: \"Operation completed\"}",
		"  _ = tooltypes.ToolResultBlock{Content: \"Tool execution failed\"}",
		"  result := tooltypes.ToolResult{}",
		"  result.Content = \"Ready to continue\"",
		"  block := tooltypes.ToolResultBlock{}",
		"  block.Content = \"Permission denied\"",
		"  _ = tooltypes.ToolResult{Content: loc.Text(lang, loc.Key(\"sample.ready\"))}",
		"  result.Content = raw",
		"}",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleDisplayLiteral); got != 4 {
		t.Fatalf("ToolResult content violations = %d, want 4:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitTracksTypeAssertionsAndPackageConstants(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import tooltypes \"github.com/agent-dance/luban/types\"",
		"const unchanged = \"File unchanged since last read\"",
		"func render(raw any) {",
		"  block := raw.(tooltypes.ToolResultBlock)",
		"  block.Content = \"Operation failed\"",
		"  _ = tooltypes.ToolResult{Content: unchanged}",
		"}",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleDisplayLiteral); got != 2 {
		t.Fatalf("indirect display violations = %d, want 2:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitRejectsNonEnglishDirectCopy(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"func render(renderer interface{ Info(string) }) {",
		"  renderer.Info(\"操作完成\")",
		"}",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleDisplayLiteral); got != 1 {
		t.Fatalf("non-English display violations = %d, want 1:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitTombstoneSummaryIsDisplayCopy(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import stream \"github.com/agent-dance/luban/internal/contracts/stream\"",
		"func render() {",
		"  _ = stream.TombstoneEvent{Summary: \"Assistant message replaced by retry\"}",
		"}",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleDisplayLiteral); got != 1 {
		t.Fatalf("tombstone display violations = %d, want 1:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitToolResultBoundariesRequireNarrowExceptions(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import tooltypes \"github.com/agent-dance/luban/types\"",
		"func render() {",
		"  // i18n:allow display-literal protocol -- this JSON object is the tool response wire contract",
		"  _ = tooltypes.ToolResult{Content: `{\"status\":\"ok\"}`}",
		"  // i18n:allow display-literal identifier -- HTTP is a stable protocol identifier in this result",
		"  _ = tooltypes.ToolResult{Content: \"HTTP\"}",
		"  // i18n:allow display-literal path -- this path is a stable machine-readable tool result",
		"  _ = tooltypes.ToolResult{Content: \"/tmp/tool-output\"}",
		"}",
	)
	if violations := guardUnitScan(source, false); len(violations) != 0 {
		t.Fatalf("narrow adjacent exceptions should preserve protocol and identifier boundaries:\n%s", guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitRecognizesGoTUIDefaultPackageName(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import \"github.com/grindlemire/go-tui\"",
		"func render() { tui.WithText(\"Untranslated copy\") }",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleDisplayLiteral); got != 1 {
		t.Fatalf("go-tui display violations = %d, want 1:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitLoggingSinks(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import (",
		"  standardlog \"log\"",
		"  structured \"log/slog\"",
		"  loc \"github.com/agent-dance/luban/i18n\"",
		")",
		"func render(lang loc.Language) {",
		"  standardlog.Printf(\"background task failed: %v\", \"raw\")",
		"  structured.Warn(\"retrying connection\", \"attempt\", 2)",
		"  structured.Info(loc.Text(lang, loc.Key(\"sample.connected\")))",
		"}",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleDisplayLiteral); got != 2 {
		t.Fatalf("logging display violations = %d, want 2:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitFlagSetUsage(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import \"flag\"",
		"func configure() {",
		"  fs := flag.NewFlagSet(\"sample\", flag.ContinueOnError)",
		"  var value string",
		"  fs.StringVar(&value, \"value\", \"\", \"English usage copy\")",
		"}",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleDisplayLiteral); got != 1 {
		t.Fatalf("FlagSet usage violations = %d, want 1:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitValidExceptionsAreNarrow(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"import loc \"github.com/agent-dance/luban/i18n\"",
		"func render(renderer interface{ Info(string) }) {",
		"  // i18n:allow display-literal brand -- product name is intentionally stable",
		"  renderer.Info(\"Claude Code\")",
		"  // i18n:allow forced-english wire-compat -- comparison value is persisted by older releases",
		"  _ = loc.Text(loc.LangEN, loc.Key(\"sample.legacy_value\"))",
		"}",
	)
	if violations := guardUnitScan(source, false); len(violations) != 0 {
		t.Fatalf("valid exceptions should suppress their adjacent rule only:\n%s", guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitStaleExceptionFails(t *testing.T) {
	source := guardUnitSource(
		"package sample",
		"func render(renderer interface{ Info(string) }, raw string) {",
		"  // i18n:allow display-literal raw-output -- value comes from the executed tool",
		"  renderer.Info(raw)",
		"}",
	)
	violations := guardUnitScan(source, false)
	if got := guardUnitRuleCount(violations, ruleException); got != 1 {
		t.Fatalf("stale exception violations = %d, want 1:\n%s", got, guardUnitViolationReport(violations))
	}
}

func TestI18nSourceGuardUnitSemanticKeyDeclarations(t *testing.T) {
	source := guardUnitSource(
		"package i18n",
		"const (",
		"  KeyOne Key = \"sample.one\"",
		"  KeyTwo Key = \"sample.two\"",
		")",
	)
	input := guardInput{
		Path:        "i18n/sample_keys.go",
		PackagePath: guardI18nImportPath,
		Source:      []byte(source),
	}
	declarations, violations := guardCollectKeyDeclarations([]guardInput{input})
	if len(violations) != 0 {
		t.Fatalf("collect declarations:\n%s", guardUnitViolationReport(violations))
	}
	if len(declarations) != 2 || declarations[0].Key != Key("sample.one") || declarations[1].Key != Key("sample.two") {
		t.Fatalf("unexpected declarations: %#v", declarations)
	}
	catalog := map[Key]map[Language]string{
		Key("sample.one"): {},
		Key("sample.two"): {},
	}
	if problems := guardValidateKeyCatalog(declarations, catalog); len(problems) != 0 {
		t.Fatalf("matching catalog reported problems: %v", problems)
	}
}

func TestI18nSourceGuardUnitSemanticCatalogRejectsDrift(t *testing.T) {
	declarations := []guardKeyDeclaration{
		{Name: "KeyOne", Key: Key("sample.one"), Path: "keys.go", Line: 1},
		{Name: "KeyDuplicate", Key: Key("sample.one"), Path: "keys.go", Line: 2},
		{Name: "KeySentence", Key: Key("English sentence"), Path: "keys.go", Line: 3},
	}
	catalog := map[Key]map[Language]string{
		Key("sample.extra"): {},
	}
	report := strings.Join(guardValidateKeyCatalog(declarations, catalog), "\n")
	for _, expected := range []string{
		"duplicate semantic key value",
		"non-semantic value",
		"declared but not registered",
		"without a typed Key constant declaration",
	} {
		if !strings.Contains(report, expected) {
			t.Errorf("catalog drift report missing %q:\n%s", expected, report)
		}
	}
}

func guardUnitScan(source string, isTest bool) []guardViolation {
	return guardScan([]guardInput{{
		Path:        "sample.go",
		PackagePath: guardModulePath + "/sample",
		Source:      []byte(source),
		IsTest:      isTest,
	}})
}

func guardUnitSource(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func guardUnitRuleCount(violations []guardViolation, rule guardRule) int {
	count := 0
	for _, violation := range violations {
		if violation.Rule == rule {
			count++
		}
	}
	return count
}

func guardUnitViolationReport(violations []guardViolation) string {
	if len(violations) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(violations))
	for _, violation := range violations {
		lines = append(lines, violation.String())
	}
	return strings.Join(lines, "\n")
}
