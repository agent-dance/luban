# Resume Session Namespace Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent resume from decoding Claude Code project transcripts while preserving every PRC/DeepSeek/Go-owned legacy session layout.

**Architecture:** Separate complete fallback config homes from individually approved legacy session roots. Repository discovery will scan `.prc-code`, `.deepseek-code`, and `.claude-go` project layouts, but only the flat `.claude/sessions` directory from Claude's namespace.

**Tech Stack:** Go standard library, existing `session.Repository`, Go `testing`

## Global Constraints

- No new dependencies.
- Do not change PRC session JSON encoding or decoding.
- Do not scan `~/.claude/projects`.
- Preserve project and flat layouts in `.prc-code`, `.deepseek-code`, and `.claude-go`, plus flat `.claude/sessions` compatibility.

---

### Task 1: Lock namespace discovery behavior with regression tests

**Files:**
- Modify: `session/repository_test.go`
- Test: `session/repository_test.go`

**Interfaces:**
- Consumes: `DefaultRepository`, `Repository.Search`, `Repository.Resolve`, `NewFileStore`.
- Produces: regression coverage for foreign-project exclusion and legacy-layout inclusion.

- [ ] **Step 1: Write the failing foreign transcript test**

```go
func TestDefaultRepositoryDoesNotIndexClaudeCodeProjectTranscripts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	nativeProject := filepath.Join(home, ".claude", "projects", "native")
	if err := os.MkdirAll(nativeProject, 0o755); err != nil { t.Fatal(err) }
	nativeTranscript := []byte(
		`{"type":"queue-operation","content":"hello","operation":"enqueue"}` + "\n" +
			`{"type":"user","message":{"role":"user","content":"hello"}}` + "\n",
	)
	nativePath := filepath.Join(nativeProject, "foreign.jsonl")
	if err := os.WriteFile(nativePath, nativeTranscript, 0o644); err != nil { t.Fatal(err) }
	repo := DefaultRepository()
	got, err := repo.Search(SearchOptions{AllProjects: true})
	if err != nil { t.Fatal(err) }
	if len(got) != 0 { t.Fatalf("foreign Claude sessions were indexed: %+v", got) }
	if _, err := repo.Resolve("foreign", ""); !errors.Is(err, fs.ErrNotExist) { t.Fatalf("Resolve foreign error = %v, want fs.ErrNotExist", err) }
	unchanged, err := os.ReadFile(nativePath)
	if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(unchanged, nativeTranscript) { t.Fatalf("foreign transcript changed: got %q want %q", unchanged, nativeTranscript) }
	if _, err := os.Stat(filepath.Join(nativeProject, "foreign.meta.json")); !errors.Is(err, fs.ErrNotExist) { t.Fatalf("foreign transcript gained a metadata sidecar: %v", err) }
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./session -run TestDefaultRepositoryDoesNotIndexClaudeCodeProjectTranscripts -count=1`

Expected: FAIL because `foreign.jsonl` is currently indexed from `.claude/projects`.

- [ ] **Step 3: Add the legacy compatibility test**

```go
func TestDefaultRepositorySearchesAndLoadsOwnedLegacyLayouts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	want := map[string]string{
		"prc-project": "prc project",
		"prc-flat": "prc flat",
		"deepseek-project": "deepseek project",
		"deepseek-flat": "deepseek flat",
		"go-project": "go project",
		"go-flat": "go flat",
		"claude-flat": "claude flat",
	}
	stores := map[string]string{
		"prc-project": filepath.Join(home, ".prc-code", "projects", "current-project"),
		"prc-flat": filepath.Join(home, ".prc-code", "sessions"),
		"deepseek-project": filepath.Join(home, ".deepseek-code", "projects", "legacy-project"),
		"deepseek-flat": filepath.Join(home, ".deepseek-code", "sessions"),
		"go-project": filepath.Join(home, ".claude-go", "projects", "legacy-project"),
		"go-flat": filepath.Join(home, ".claude-go", "sessions"),
		"claude-flat": filepath.Join(home, ".claude", "sessions"),
	}
	for id, dir := range stores {
		if err := NewFileStore(dir).Save(id, []types.Message{types.UserMessage(want[id])}); err != nil { t.Fatal(err) }
	}
	repo := DefaultRepository()
	found, err := repo.Search(SearchOptions{AllProjects: true})
	if err != nil { t.Fatal(err) }
	if len(found) != len(want) { t.Fatalf("Search returned %d sessions, want %d: %+v", len(found), len(want), found) }
	for id, wantText := range want {
		ref, err := repo.Resolve(id, "")
		if err != nil { t.Fatalf("resolve %s: %v", id, err) }
		messages, err := repo.Load(ref)
		if err != nil { t.Fatalf("load %s: %v", id, err) }
		if len(messages) != 1 || messages[0].GetText() != wantText { t.Fatalf("load %s = %#v, want %q", id, messages, wantText) }
	}
}
```

- [ ] **Step 4: Run both tests and confirm the compatibility test also exposes the missing `.claude-go` fallback**

Run: `go test ./session -run 'TestDefaultRepository(DoesNotIndexClaudeCodeProjectTranscripts|SearchesAndLoadsOwnedLegacyLayouts)' -count=1`

Expected: FAIL before implementation; the foreign session is indexed and `.claude-go` is not indexed.

### Task 2: Isolate fallback discovery namespaces

**Files:**
- Modify: `session/repository.go`
- Test: `session/repository_test.go`

**Interfaces:**
- Consumes: `brand.LegacyDeepSeekUserConfigDir`, `brand.LegacyUserGoDir`, `brand.LegacyUserConfigDir`.
- Produces: `Repository.fallbackLegacyRoots []string` and namespace-safe `allStoreDirs()` discovery.

- [ ] **Step 1: Add explicit legacy roots and correct owned fallback homes**

```go
type Repository struct {
	projectsRoot        string
	legacyRoot          string
	fallbackConfigHomes []string
	fallbackLegacyRoots []string
}

func DefaultRepository() *Repository {
	repo := NewRepository(ConfigHomeDir())
	repo.fallbackConfigHomes = []string{
		brand.LegacyDeepSeekUserConfigDir(),
		brand.LegacyUserGoDir(),
	}
	repo.fallbackLegacyRoots = []string{
		filepath.Join(brand.LegacyUserConfigDir(), "sessions"),
	}
	return repo
}
```

- [ ] **Step 2: Include only approved flat roots during discovery**

```go
func (r *Repository) allStoreDirs() []string {
	dirs := make([]string, 0, 12)
	dirs = appendRepositoryStoreDirs(dirs, filepath.Dir(r.projectsRoot))
	for _, configHome := range r.fallbackConfigHomes {
		dirs = appendRepositoryStoreDirs(dirs, configHome)
	}
	for _, legacyRoot := range r.fallbackLegacyRoots {
		if strings.TrimSpace(legacyRoot) != "" {
			dirs = append(dirs, legacyRoot)
		}
	}
	dirs = uniqueSessionDirs(dirs)
	sort.Strings(dirs)
	return dirs
}
```

- [ ] **Step 3: Run focused tests and verify GREEN**

Run: `go test ./session -run 'TestDefaultRepository(DoesNotIndexClaudeCodeProjectTranscripts|SearchesAndLoadsOwnedLegacyLayouts|ReadsLegacyDeepSeekSessionsAndWritesPRC)' -count=1`

Expected: PASS.

- [ ] **Step 4: Run package and repository verification**

Run: `go test ./session -count=1`

Run: `go test ./... -count=1`

Run: `go vet ./...`

Expected: all commands exit 0 without new warnings.

## Self-review

Both stated compatibility requirements map to focused tests. The field names
and helper calls are consistent across tasks, and there are no placeholders or
unrelated refactors.
