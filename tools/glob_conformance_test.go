// Package tools — glob_conformance_test.go pins the public matching surface
// (CompileGlob, MatchGlob, MatchGlobRelativeTo, CombinedGlobMatch) and the
// SortByMtime helper against the behaviours TS minimatch + sortByMtime.ts
// provide. These complement the integration-style search_test.go cases.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCompileGlobValidatesPattern(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		wantErr bool
	}{
		{"simple", "*.go", false},
		{"globstar", "**/*.ts", false},
		{"brace", "src/**/*.{ts,tsx}", false},
		{"posix", "[[:alpha:]].txt", false},
		{"negated", "!**/node_modules", false},
		{"empty", "", true},
		{"unbalanced", "src/**/[abc", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CompileGlob(tc.pattern)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %q", tc.pattern)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.pattern, err)
			}
		})
	}
}

func TestMatchGlobBasicPatterns(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "main.ts", false},
		{"src/**/*.go", "src/foo/bar.go", true},
		{"src/**/*.go", "src/bar.go", true},
		{"src/**/*.go", "lib/bar.go", false},
		{"src/**/*.{ts,tsx}", "src/a/b.tsx", true},
		{"src/**/*.{ts,tsx}", "src/a/b.ts", true},
		{"src/**/*.{ts,tsx}", "src/a/b.go", false},
		{"?ello", "hello", true},
		{"?ello", "heello", false},
	}
	for _, tc := range cases {
		t.Run(tc.pattern+" vs "+tc.path, func(t *testing.T) {
			got, err := MatchGlob(tc.pattern, tc.path)
			if err != nil {
				t.Fatalf("match err: %v", err)
			}
			if got != tc.want {
				t.Errorf("MatchGlob(%q,%q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}

func TestMatchGlobNegated(t *testing.T) {
	g, err := CompileGlob("!**/node_modules/**")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !g.IsNegated() {
		t.Error("expected pattern to be marked negated")
	}
	// Negated pattern returns true when path does NOT match underlying pattern
	if !g.Match("src/index.ts") {
		t.Errorf("expected src/index.ts to satisfy !node_modules")
	}
	if g.Match("node_modules/foo/index.js") {
		t.Errorf("expected node_modules path to be excluded by negated pattern")
	}
}

func TestMatchGlobRelativeBasenamePattern(t *testing.T) {
	root := "/work/proj"
	got, err := MatchGlobRelativeTo("*.go", "/work/proj/src/main.go", root)
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if !got {
		t.Error("basename pattern *.go should match nested file by basename")
	}
}

func TestMatchGlobRelativeRootedPattern(t *testing.T) {
	root := "/work/proj"
	got, err := MatchGlobRelativeTo("src/**/*.go", "/work/proj/src/foo/bar.go", root)
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if !got {
		t.Error("rooted pattern should match relative path under root")
	}
}

func TestCombinedGlobMatchPositiveOnly(t *testing.T) {
	patterns := []string{"*.go", "*.ts"}
	if !CombinedGlobMatch(patterns, "x.go", "") {
		t.Error("expected go file to match positive set")
	}
	if CombinedGlobMatch(patterns, "x.py", "") {
		t.Error("expected py file to be excluded by positive-only set")
	}
}

func TestCombinedGlobMatchNegativeWins(t *testing.T) {
	patterns := []string{"**/*.go", "!**/vendor/**"}
	if !CombinedGlobMatch(patterns, "src/main.go", "") {
		t.Error("expected non-vendor go file to match")
	}
	if CombinedGlobMatch(patterns, "vendor/x/y.go", "") {
		t.Error("expected vendor file to be excluded by negation")
	}
}

func TestCombinedGlobMatchNegativeOnlyKeepsNonMatches(t *testing.T) {
	patterns := []string{"!**/node_modules/**"}
	if !CombinedGlobMatch(patterns, "src/index.ts", "") {
		t.Error("non-matching path should be kept under negative-only patterns")
	}
	if CombinedGlobMatch(patterns, "node_modules/x.js", "") {
		t.Error("matching negative pattern should drop path")
	}
}

func TestSortByMtimeOldestFirst(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "old.txt")
	newer := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(older, []byte("o"), 0644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	// guarantee distinct mtimes regardless of FS resolution
	now := time.Now()
	pastTime := now.Add(-2 * time.Hour)
	if err := os.Chtimes(older, pastTime, pastTime); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.WriteFile(newer, []byte("n"), 0644); err != nil {
		t.Fatalf("write new: %v", err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatalf("chtimes new: %v", err)
	}

	paths := []string{older, newer}
	if err := SortByMtime(paths); err != nil {
		t.Fatalf("sort: %v", err)
	}
	if paths[0] != older {
		t.Errorf("expected oldest first, got %v", paths)
	}
}

func TestSortByMtimeMissingFilesGoToBottom(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	missing := filepath.Join(dir, "missing.txt")
	paths := []string{missing, real}
	_ = SortByMtime(paths) // error is allowed; ordering is what matters
	if paths[0] != real {
		t.Errorf("expected real file first, got %v", paths)
	}
	if paths[1] != missing {
		t.Errorf("expected missing file last, got %v", paths)
	}
}

func TestSortByMtimeStableTieBreaker(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	now := time.Now()
	for _, p := range []string{a, b} {
		if err := os.Chtimes(p, now, now); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	paths := []string{b, a}
	if err := SortByMtime(paths); err != nil {
		t.Fatalf("sort: %v", err)
	}
	// equal mtime -> alphabetical ascending
	if paths[0] != a || paths[1] != b {
		t.Errorf("expected alphabetical tiebreaker, got %v", paths)
	}
}

func TestPathScopeRejectsOutOfBoundsRoot(t *testing.T) {
	scope := t.TempDir()
	other := t.TempDir()
	SetAllowedSearchDirs([]string{scope})
	t.Cleanup(func() { SetAllowedSearchDirs(nil) })

	if IsPathWithinAllowed(other) {
		t.Errorf("path %s outside scope must be rejected", other)
	}
	if !IsPathWithinAllowed(scope) {
		t.Errorf("scope itself must be allowed")
	}
	nested := filepath.Join(scope, "nested", "deep.txt")
	if !IsPathWithinAllowed(nested) {
		t.Errorf("nested path under scope must be allowed")
	}
}

func TestPathScopeNoListAllowsAnything(t *testing.T) {
	SetAllowedSearchDirs(nil)
	if !IsPathWithinAllowed("/anywhere/at/all") {
		t.Error("empty allow-list should allow any path")
	}
}

func TestEnsureSearchRootAllowedRejectsOutside(t *testing.T) {
	scope := t.TempDir()
	other := t.TempDir()
	SetAllowedSearchDirs([]string{scope})
	t.Cleanup(func() { SetAllowedSearchDirs(nil) })

	if _, err := EnsureSearchRootAllowed(other); err == nil {
		t.Errorf("expected scope error for %s", other)
	}
	if got, err := EnsureSearchRootAllowed(scope); err != nil || got == "" {
		t.Errorf("scope itself should resolve, got %s err=%v", got, err)
	}
}

func TestFilterAllowedPathsTrimsOutOfScope(t *testing.T) {
	scope := t.TempDir()
	other := t.TempDir()
	SetAllowedSearchDirs([]string{scope})
	t.Cleanup(func() { SetAllowedSearchDirs(nil) })

	in := []string{filepath.Join(scope, "a.txt"), filepath.Join(other, "b.txt")}
	got := FilterAllowedPaths(in)
	if len(got) != 1 || !strings.HasPrefix(got[0], scope) {
		t.Errorf("expected only in-scope path, got %v", got)
	}
}

func TestGlobToolBraceExpansion(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.ts", "b.tsx", "c.go", "d.py"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.{ts,tsx}",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.ts") || !strings.Contains(res.Content, "b.tsx") {
		t.Errorf("expected ts and tsx files, got %q", res.Content)
	}
	if strings.Contains(res.Content, "c.go") || strings.Contains(res.Content, "d.py") {
		t.Errorf("brace pattern should not include go/py, got %q", res.Content)
	}
}

func TestGlobToolMtimeOrdering(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "older.txt")
	newer := filepath.Join(dir, "newer.txt")
	if err := os.WriteFile(older, []byte("o"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	now := time.Now()
	if err := os.Chtimes(older, now.Add(-3*time.Hour), now.Add(-3*time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := os.WriteFile(newer, []byte("n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	lines := strings.Split(strings.TrimSpace(res.Content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d (%q)", len(lines), res.Content)
	}
	// TS ripgrep --sort=modified returns oldest before newer.
	idxNew := -1
	idxOld := -1
	for i, line := range lines {
		if strings.Contains(line, "newer.txt") {
			idxNew = i
		}
		if strings.Contains(line, "older.txt") {
			idxOld = i
		}
	}
	if idxNew < 0 || idxOld < 0 {
		t.Fatalf("did not see both files: %q", res.Content)
	}
	if idxOld >= idxNew {
		t.Errorf("older must come before newer, got %v", lines)
	}
}

func TestGlobToolDefaultsToCwd(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "x.txt") {
		t.Errorf("expected x.txt in cwd glob result, got %q", res.Content)
	}
}

func TestGlobScopedCwdDoesNotDependOnProcessChdir(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(firstRoot, "first.txt"), []byte("first"), 0o644); err != nil {
		t.Fatalf("write first fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secondRoot, "second.txt"), []byte("second"), 0o644); err != nil {
		t.Fatalf("write second fixture: %v", err)
	}
	firstScope := NewRuntimeScope(firstRoot, true)
	firstScope.SetAllowedDirs([]string{firstRoot})
	secondScope := NewRuntimeScope(secondRoot, true)
	secondScope.SetAllowedDirs([]string{secondRoot})
	firstTool := NewGlobTool(firstScope)
	secondTool := NewGlobTool(secondScope)

	type result struct {
		name string
		res  string
		err  bool
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for name, tool := range map[string]*GlobTool{"first": firstTool, "second": secondTool} {
		wg.Add(1)
		go func(name string, tool *GlobTool) {
			defer wg.Done()
			res, _ := tool.Execute(context.Background(), map[string]any{"pattern": "*.txt"})
			results <- result{name: name, res: res.Content, err: res.IsError}
		}(name, tool)
	}
	wg.Wait()
	close(results)
	for got := range results {
		if got.err || !strings.Contains(got.res, got.name+".txt") {
			t.Fatalf("%s scoped Glob result = error:%v content:%q", got.name, got.err, got.res)
		}
		other := "second.txt"
		if got.name == "second" {
			other = "first.txt"
		}
		if strings.Contains(got.res, other) {
			t.Fatalf("%s scoped Glob leaked %s: %q", got.name, other, got.res)
		}
	}
}

func TestGlobRuntimeReadDenyRulesDoNotConsumeLimitSlots(t *testing.T) {
	root := t.TempDir()
	secretDir := filepath.Join(root, "secret")
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatalf("mkdir secret: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	for i := 0; i < defaultGlobLimit+10; i++ {
		path := filepath.Join(secretDir, fmt.Sprintf("old-%03d.go", i))
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("write denied fixture: %v", err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes denied fixture: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("keep-%03d.go", i)), nil, 0o644); err != nil {
			t.Fatalf("write kept fixture: %v", err)
		}
	}
	scope := NewRuntimeScope(root, true)
	scope.SetAllowedDirs([]string{root})
	scope.SetDeniedTools([]string{"Read(secret/**)"})

	res, _ := NewGlobTool(scope).Execute(context.Background(), map[string]any{"pattern": "**/*.go"})
	if res.IsError {
		t.Fatalf("runtime deny Glob failed: %s", res.Content)
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(res.Content, fmt.Sprintf("keep-%03d.go", i)) {
			t.Fatalf("kept result was displaced by denied files: %q", res.Content)
		}
	}
	if strings.Contains(res.Content, "secret/") {
		t.Fatalf("runtime Read deny leaked into Glob output: %q", res.Content)
	}
}

func TestGlobOrphanedPluginCacheExclusionIsSessionScoped(t *testing.T) {
	pluginsDir := t.TempDir()
	t.Setenv("CLAUDE_CODE_PLUGIN_CACHE_DIR", pluginsDir)
	cacheRoot := filepath.Join(pluginsDir, "cache")
	orphanRoot := filepath.Join(cacheRoot, "market", "sample", "1.0.0")
	liveRoot := filepath.Join(cacheRoot, "market", "sample", "2.0.0")
	for _, dir := range []string{orphanRoot, liveRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir plugin fixture: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(orphanRoot, ".orphaned_at"), []byte("now"), 0o644); err != nil {
		t.Fatalf("write orphan marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphanRoot, "stale.go"), nil, 0o644); err != nil {
		t.Fatalf("write stale plugin file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveRoot, "live.go"), nil, 0o644); err != nil {
		t.Fatalf("write live plugin file: %v", err)
	}
	scope := NewRuntimeScope(cacheRoot, true)
	scope.SetAllowedDirs([]string{cacheRoot})

	res, _ := NewGlobTool(scope).Execute(context.Background(), map[string]any{"pattern": "**/*.go"})
	if res.IsError || !strings.Contains(res.Content, "live.go") || strings.Contains(res.Content, "stale.go") {
		t.Fatalf("plugin cache exclusion mismatch: error=%v content=%q", res.IsError, res.Content)
	}
}

func TestGlobNativeBracketLimitSelectsOldestFullCandidateSet(t *testing.T) {
	assertGlobLimitSelectsOldest(t, false)
}

func TestGlobFallbackLimitSelectsOldestFullCandidateSet(t *testing.T) {
	assertGlobLimitSelectsOldest(t, true)
}

func assertGlobLimitSelectsOldest(t *testing.T, forceFallback bool) {
	t.Helper()
	if forceFallback {
		t.Setenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK", "1")
	}
	root := t.TempDir()
	now := time.Now()
	for i := 0; i <= defaultGlobLimit; i++ {
		path := filepath.Join(root, fmt.Sprintf("f%03d.txt", i))
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		mtime := now.Add(time.Duration(i) * time.Second)
		if i == defaultGlobLimit {
			mtime = now.Add(-time.Hour)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes fixture: %v", err)
		}
	}
	pattern := "*.txt"
	if !forceFallback {
		pattern = "f[0-9][0-9][0-9].txt"
	}
	res, _ := (&GlobTool{}).Execute(context.Background(), map[string]any{"pattern": pattern, "path": root})
	if res.IsError {
		t.Fatalf("Glob limit fixture failed: %s", res.Content)
	}
	if !strings.Contains(res.Content, "f100.txt") || strings.Contains(res.Content, "f099.txt") {
		t.Fatalf("Glob did not sort the full candidate set before limiting: %q", res.Content)
	}
}

func TestGlobToolSubdirectoryDescent(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deep, "deep.go"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tool := &GlobTool{}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "**/*.go",
		"path":    dir,
	})
	if res.IsError {
		t.Fatalf("err: %s", res.Content)
	}
	if !strings.Contains(res.Content, "deep.go") {
		t.Errorf("expected deep file in **/*.go, got %q", res.Content)
	}
}
