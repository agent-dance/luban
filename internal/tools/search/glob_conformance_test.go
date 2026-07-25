package search

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

func TestEnsureSearchRootAllowedRejectsOutOfBoundsRoot(t *testing.T) {
	scope := t.TempDir()
	other := t.TempDir()

	if _, err := ensureSearchRootAllowed(other, scope, []string{scope}); err == nil {
		t.Errorf("expected scope error for %s", other)
	}
	if got, err := ensureSearchRootAllowed(scope, scope, []string{scope}); err != nil || got == "" {
		t.Errorf("scope itself should resolve, got %s err=%v", got, err)
	}
	nested := filepath.Join(scope, "nested", "deep.txt")
	if got, err := ensureSearchRootAllowed(nested, scope, []string{scope}); err != nil || got == "" {
		t.Errorf("nested root should resolve, got %s err=%v", got, err)
	}
}

func TestGlobToolBraceExpansion(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.ts", "b.tsx", "c.go", "d.py"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	tool := &globTool{}
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

	tool := &globTool{}
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
	// Glob's modified-time ordering returns older files before newer files.
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
	tool := &globTool{}
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
	firstTool := NewGlob(firstScope)
	secondTool := NewGlob(secondScope)

	type result struct {
		name string
		res  string
		err  bool
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for name, tool := range map[string]*globTool{"first": firstTool, "second": secondTool} {
		wg.Add(1)
		go func(name string, tool *globTool) {
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

	res, _ := NewGlob(scope).Execute(context.Background(), map[string]any{"pattern": "**/*.go"})
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
	t.Setenv("LUBAN_CODE_PLUGIN_CACHE_DIR", pluginsDir)
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

	res, _ := NewGlob(scope).Execute(context.Background(), map[string]any{"pattern": "**/*.go"})
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
		withUnavailableRipgrep(t)
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
	res, _ := (&globTool{}).Execute(context.Background(), map[string]any{"pattern": pattern, "path": root})
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
	tool := &globTool{}
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
