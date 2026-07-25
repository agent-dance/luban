// Package search exercises Grep output modes, pagination, multiline and
// fallback behavior across platforms.
package search

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestGrepToolFallbackOutputModeFiles(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "match\n")
	mustWrite(t, filepath.Join(dir, "b.txt"), "skip\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "match",
		"path":        dir,
		"output_mode": "files_with_matches",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.txt") {
		t.Errorf("expected a.txt in result, got %q", res.Content)
	}
	if strings.Contains(res.Content, "b.txt") {
		t.Errorf("did not expect b.txt: %q", res.Content)
	}
}

func TestGrepLeadingDashGlobIsPassedAsData(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), "needle\n")

	result, err := NewGrep(nil).Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
		"glob":    "--include=*.go",
	})
	if err != nil || result.IsError {
		t.Fatalf("leading-dash glob was interpreted as an option: err=%v result=%#v", err, result)
	}
	output, ok := result.Data.(grepOutput)
	if !ok || output.NumFiles != 0 {
		t.Fatalf("leading-dash glob should be a literal non-matching glob: %#v", result.Data)
	}
}

func TestGrepToolFallbackCountMode(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	fp := filepath.Join(dir, "data.txt")
	mustWrite(t, fp, "match\nother\nmatch\nmatch\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "match",
		"path":        fp,
		"output_mode": "count",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "3") {
		t.Errorf("expected count 3, got %q", res.Content)
	}
}

func TestGrepToolFallbackContentMode(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	fp := filepath.Join(dir, "x.txt")
	mustWrite(t, fp, "alpha\nbeta\ngamma\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "beta",
		"path":        fp,
		"output_mode": "content",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "beta") {
		t.Errorf("expected beta in content, got %q", res.Content)
	}
}

func TestGrepToolFallbackOffset(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		mustWrite(t, filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), "needle\n")
	}

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":    "needle",
		"path":       dir,
		"head_limit": float64(2),
		"offset":     float64(1),
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	lines := grepResultPayloadLines(res)
	if len(lines) != 2 {
		t.Errorf("expected 2 lines after offset+limit, got %d (%q)", len(lines), res.Content)
	}
}

func TestGrepToolFallbackHeadLimitDefault(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	for i := 0; i < defaultGrepHeadLimit+10; i++ {
		mustWrite(t, filepath.Join(dir, fmt.Sprintf("f%04d.txt", i)), "needle\n")
	}

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	lines := grepResultPayloadLines(res)
	if len(lines) != defaultGrepHeadLimit {
		t.Errorf("expected default cap %d, got %d", defaultGrepHeadLimit, len(lines))
	}
}

func TestGrepToolFallbackUnlimited(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		mustWrite(t, filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), "needle\n")
	}

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":    "needle",
		"path":       dir,
		"head_limit": float64(0),
		"offset":     float64(0),
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	lines := grepResultPayloadLines(res)
	if len(lines) != 5 {
		t.Errorf("expected all 5 results, got %d (%q)", len(lines), res.Content)
	}
	output, ok := res.Data.(grepOutput)
	if !ok {
		t.Fatalf("result data has type %T, want grepOutput", res.Data)
	}
	if output.AppliedLimit != 0 || output.AppliedOffset != 0 {
		t.Fatalf("zero pagination values must mean unlimited from the first result, got limit=%d offset=%d", output.AppliedLimit, output.AppliedOffset)
	}
}

func TestGrepToolRejectsInvalidPaginationNumbers(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value float64
	}{
		{name: "negative head limit", field: "head_limit", value: -1},
		{name: "fractional head limit", field: "head_limit", value: 1.5},
		{name: "NaN head limit", field: "head_limit", value: math.NaN()},
		{name: "infinite head limit", field: "head_limit", value: math.Inf(1)},
		{name: "negative offset", field: "offset", value: -1},
		{name: "fractional offset", field: "offset", value: 0.5},
		{name: "NaN offset", field: "offset", value: math.NaN()},
		{name: "infinite offset", field: "offset", value: math.Inf(-1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := (&grepTool{}).Execute(context.Background(), map[string]any{
				"pattern":  test.name,
				test.field: test.value,
			})
			if err != nil {
				t.Fatalf("Execute returned infrastructure error: %v", err)
			}
			if !res.IsError {
				t.Fatalf("Execute accepted %s=%v", test.field, test.value)
			}
			if !strings.Contains(res.Content, test.field) {
				t.Fatalf("validation error %q does not identify %s", res.Content, test.field)
			}
		})
	}
}

func TestGrepToolFallbackInvalidOutputMode(t *testing.T) {
	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "x",
		"output_mode": "html",
	})
	if !res.IsError {
		t.Fatalf("expected error for invalid output_mode")
	}
	if !strings.Contains(res.Content, "output_mode") {
		t.Errorf("expected output_mode in error, got %q", res.Content)
	}
}

func TestGrepToolFallbackTypeFilter(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "main.go"), "needle\n")
	mustWrite(t, filepath.Join(dir, "main.py"), "needle\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
		"type":    "go",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "main.go") {
		t.Errorf("expected main.go: %q", res.Content)
	}
	if strings.Contains(res.Content, "main.py") {
		t.Errorf("type filter should exclude py: %q", res.Content)
	}
}

func TestGrepToolFallbackGlobFilter(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), "needle\n")
	mustWrite(t, filepath.Join(dir, "b.txt"), "needle\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
		"glob":    "*.go",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.go") {
		t.Errorf("expected a.go: %q", res.Content)
	}
	if strings.Contains(res.Content, "b.txt") {
		t.Errorf("glob should exclude txt: %q", res.Content)
	}
}

func TestGrepToolFallbackContextLines(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	fp := filepath.Join(dir, "log.txt")
	mustWrite(t, fp, "line1\nline2\ntarget\nline4\nline5\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "target",
		"path":        fp,
		"output_mode": "content",
		"-B":          float64(1),
		"-A":          float64(1),
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "line2") || !strings.Contains(res.Content, "line4") {
		t.Errorf("expected -B/-A context lines, got %q", res.Content)
	}
}

func TestGrepToolFallbackCaseInsensitive(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	fp := filepath.Join(dir, "log.txt")
	mustWrite(t, fp, "HELLO\nhello\nHeLLo\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "hello",
		"path":        fp,
		"output_mode": "count",
		"-i":          true,
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "3") {
		t.Errorf("expected count 3, got %q", res.Content)
	}
}

func TestGrepToolFallbackShowsLineNumbers(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	fp := filepath.Join(dir, "log.txt")
	mustWrite(t, fp, "first\nneedle\nthird\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        fp,
		"output_mode": "content",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "2:needle") {
		t.Errorf("expected '2:needle' (default line numbers), got %q", res.Content)
	}
}

func TestGrepToolNoMatchSimple(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "alpha\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    dir,
	})
	if res.IsError {
		t.Errorf("no match should not be an error: %q", res.Content)
	}
	if res.Content != toolRuntimeText(i18n.KeyToolSearchNoFiles) {
		t.Errorf("expected no-match message, got %q", res.Content)
	}
}

func TestGrepToolPathOutsideAllowed(t *testing.T) {
	scope := t.TempDir()
	other := t.TempDir()
	mustWrite(t, filepath.Join(other, "x.txt"), "needle\n")
	runtime := NewRuntimeScope(scope, true)
	runtime.SetAllowedDirs([]string{scope})

	tool := NewGrep(runtime)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle",
		"path":    other,
	})
	if !res.IsError {
		t.Fatalf("expected error for out-of-scope path, got %q", res.Content)
	}
	if !strings.Contains(res.Content, other) {
		t.Errorf("expected scope error, got %q", res.Content)
	}
}

func TestGrepToolMultilineFallbackSingleLine(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	fp := filepath.Join(dir, "x.txt")
	mustWrite(t, fp, "alpha\nbeta\ngamma\n")

	tool := &grepTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "alpha",
		"path":        fp,
		"output_mode": "content",
		"multiline":   true,
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "alpha") {
		t.Errorf("expected alpha match in multiline mode, got %q", res.Content)
	}
}

func TestGrepToolRejectsUnknownSchemaKeys(t *testing.T) {
	res, _ := (&grepTool{}).Execute(context.Background(), map[string]any{
		"pattern":          "needle",
		"case_insensitive": true,
	})
	if !res.IsError || !strings.Contains(res.Content, "case_insensitive") {
		t.Fatalf("expected strict schema unknown-field error, got %#v", res)
	}
}

func TestGrepToolPatternWhitespacePreserved(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "needle\n needle \n")

	res, _ := (&grepTool{}).Execute(context.Background(), map[string]any{
		"pattern":     " needle ",
		"path":        dir,
		"output_mode": "content",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if strings.Contains(res.Content, "1:needle") || !strings.Contains(res.Content, "2: needle ") {
		t.Fatalf("pattern whitespace should be preserved, got %q", res.Content)
	}
}

func TestGrepToolEmptyPatternIsForwarded(t *testing.T) {
	withUnavailableRipgrep(t)
	file := filepath.Join(t.TempDir(), "a.txt")
	mustWrite(t, file, "first\nsecond\n")
	res, _ := (&grepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "", "path": file, "output_mode": "content",
	})
	if res.IsError || !strings.Contains(res.Content, "first") || !strings.Contains(res.Content, "second") {
		t.Fatalf("an explicitly supplied empty regex pattern must remain valid: error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGrepToolContextZeroOverridesBeforeAfter(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "before\nneedle\nafter\n")

	res, _ := (&grepTool{}).Execute(context.Background(), map[string]any{
		"pattern":     "needle",
		"path":        dir,
		"output_mode": "content",
		"context":     float64(0),
		"-A":          float64(1),
		"-B":          float64(1),
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if strings.Contains(res.Content, "before") || strings.Contains(res.Content, "after") {
		t.Fatalf("context:0 should override -A/-B and suppress context lines, got %q", res.Content)
	}
}

func TestGrepRunRipgrepUsesLubanRGPath(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "rg")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'ripgrep 99.0.0'; exit 0; fi\necho \"$LUBAN_FAKE_RG_OUTPUT\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake rg: %v", err)
	}
	t.Setenv("LUBAN_RG_PATH", fake)
	t.Setenv("LUBAN_FAKE_RG_OUTPUT", "from-override.txt")
	resetRipgrepLocationForTest()
	t.Cleanup(resetRipgrepLocationForTest)

	run, err := runRipgrepDetailed(context.Background(), []string{"-l", "needle"}, dir)
	if err != nil {
		t.Fatalf("runRipgrep with LUBAN_RG_PATH: %v", err)
	}
	out := run.Lines
	if len(out) != 1 || out[0] != "from-override.txt" {
		t.Fatalf("expected fake rg output, got %#v", out)
	}
}

func TestGrepSchemaSeparatesContextAndPaginationNumberSemantics(t *testing.T) {
	properties := (&grepTool{}).Schema().Properties
	for _, key := range []string{"-B", "-A", "-C", "context"} {
		property, ok := properties[key].(map[string]any)
		if !ok {
			t.Fatalf("schema property %s has type %T", key, properties[key])
		}
		if minimum, exists := property["minimum"]; exists {
			t.Fatalf("%s must not advertise a minimum; rg owns edge validation (got %v)", key, minimum)
		}
	}
	for _, key := range []string{"head_limit", "offset"} {
		property, ok := properties[key].(map[string]any)
		if !ok {
			t.Fatalf("schema property %s has type %T", key, properties[key])
		}
		if property["minimum"] != 0 {
			t.Fatalf("%s minimum = %v, want 0", key, property["minimum"])
		}
		if property["integer"] != true {
			t.Fatalf("%s integer = %v, want true", key, property["integer"])
		}
	}
}

func TestGrepContextUsesSingleCFlagAndPreservesDecimal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell ripgrep fixture requires a POSIX shell")
	}
	argsFile := filepath.Join(t.TempDir(), "args")
	fake := writeFakeRipgrep(t, "printf '%s\\n' \"$@\" > \"$LUBAN_FAKE_RG_ARGS\"; exit 2")
	withFakeRipgrep(t, fake)
	t.Setenv("LUBAN_FAKE_RG_ARGS", argsFile)
	_, _ = (&grepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": t.TempDir(), "output_mode": "content", "context": 1.5,
	})
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read rg args: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-C 1.5") || strings.Contains(joined, "-B") || strings.Contains(joined, "-A") {
		t.Fatalf("context must be passed as exact rg -C value, args=%q", joined)
	}
}

func TestGrepTypeUsageErrorComesFromSelectedRipgrep(t *testing.T) {
	fake := writeFakeRipgrep(t, "if [ \"$1\" = \"--type-list\" ]; then echo 'go: *.go'; exit 0; fi; echo 'custom rg type usage error' >&2; exit 2")
	withFakeRipgrep(t, fake)
	res, _ := (&grepTool{}).Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": t.TempDir(), "type": "not-a-real-type",
	})
	if !res.IsError || !strings.Contains(res.Content, "custom rg type usage error") {
		t.Fatalf("type must be forwarded to rg without Go prevalidation: error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGrepMissingRipgrepUsesScannerFallback(t *testing.T) {
	t.Setenv("LUBAN_RG_PATH", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	resetRipgrepLocationForTest()
	t.Cleanup(resetRipgrepLocationForTest)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "would-match.txt"), "needle\n")
	res, _ := (&grepTool{}).Execute(context.Background(), map[string]any{"pattern": "needle", "path": dir})
	if res.IsError || !strings.Contains(res.Content, "would-match.txt") {
		t.Fatalf("missing rg must use the scanner fallback: error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGrepUNCPathRequiresPermissionAskWithoutProbe(t *testing.T) {
	tool := &grepTool{}
	input := map[string]any{"pattern": "needle", "path": `\\\\server\\share`}
	decision, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("permission check: %v", err)
	}
	if decision.Behavior != types.PermissionBehaviorAsk || !decision.Required || !strings.Contains(decision.Message, "UNC") {
		t.Fatalf("UNC path must take required ask path without filesystem probing: %+v", decision)
	}
}

func TestRipgrepLocatorRejectsNonRipgrepOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell ripgrep fixture requires a POSIX shell")
	}
	fake := filepath.Join(t.TempDir(), "rg")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'not ripgrep'\n"), 0o755); err != nil {
		t.Fatalf("write fake rg: %v", err)
	}
	t.Setenv("LUBAN_RG_PATH", fake)
	resetRipgrepLocationForTest()
	t.Cleanup(resetRipgrepLocationForTest)
	if _, err := locateRipgrep(); err == nil || !strings.Contains(err.Error(), "failed --version sanity check") {
		t.Fatalf("non-ripgrep override must fail verification, got %v", err)
	}
}

func TestRipgrepLocatorFindsBundledHomeCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell ripgrep fixture requires a POSIX shell")
	}
	home := t.TempDir()
	embedded := filepath.Join(home, ".luban-code", "bin", "rg")
	if err := os.MkdirAll(filepath.Dir(embedded), 0o755); err != nil {
		t.Fatalf("mkdir embedded rg: %v", err)
	}
	if err := os.WriteFile(embedded, []byte("#!/bin/sh\necho 'ripgrep 99.0.0'\n"), 0o755); err != nil {
		t.Fatalf("write embedded rg: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("LUBAN_RG_PATH", "")
	t.Setenv("USE_BUILTIN_RIPGREP", "1")
	resetRipgrepLocationForTest()
	t.Cleanup(resetRipgrepLocationForTest)
	got, err := locateRipgrep()
	if err != nil || filepath.Clean(got) != filepath.Clean(embedded) {
		t.Fatalf("embedded locator mismatch: path=%q err=%v", got, err)
	}
}

func TestRipgrepLocatorCacheResetSelectsNewOverride(t *testing.T) {
	first := writeFakeRipgrep(t, "exit 0")
	second := writeFakeRipgrep(t, "exit 0")
	t.Setenv("LUBAN_RG_PATH", first)
	resetRipgrepLocationForTest()
	t.Cleanup(resetRipgrepLocationForTest)
	got, err := locateRipgrep()
	if err != nil || got != first {
		t.Fatalf("first locator: path=%q err=%v", got, err)
	}
	t.Setenv("LUBAN_RG_PATH", second)
	if cached, err := locateRipgrep(); err != nil || cached != first {
		t.Fatalf("location must remain cached before reset: path=%q err=%v", cached, err)
	}
	resetRipgrepLocationForTest()
	if refreshed, err := locateRipgrep(); err != nil || refreshed != second {
		t.Fatalf("location must refresh after reset: path=%q err=%v", refreshed, err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
