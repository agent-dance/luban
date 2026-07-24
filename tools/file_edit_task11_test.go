package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestTask11ApplyEditTable(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		oldString  string
		newString  string
		replaceAll bool
		want       string
		count      int
		wantErr    bool
	}{
		{name: "identical", content: "a", oldString: "a", newString: "a", wantErr: true},
		{name: "missing", content: "a", oldString: "b", newString: "c", wantErr: true},
		{name: "empty-old-nonempty", content: "a", oldString: "", newString: "c", wantErr: true},
		{name: "empty-file-create", content: "", oldString: "", newString: "c", want: "c", count: 1},
		{name: "single-whole", content: "a", oldString: "a", newString: "b", want: "b", count: 1},
		{name: "single-prefix", content: "abc", oldString: "a", newString: "A", want: "Abc", count: 1},
		{name: "single-middle", content: "abc", oldString: "b", newString: "B", want: "aBc", count: 1},
		{name: "single-suffix", content: "abc", oldString: "c", newString: "C", want: "abC", count: 1},
		{name: "delete-whole", content: "abc", oldString: "abc", newString: "", want: "", count: 1},
		{name: "delete-line-with-newline", content: "a\nb\nc\n", oldString: "b", newString: "", want: "a\nc\n", count: 1},
		{name: "delete-last-line-no-newline", content: "a\nb", oldString: "b", newString: "", want: "a\n", count: 1},
		{name: "ambiguous-two", content: "a a", oldString: "a", newString: "b", wantErr: true, count: 2},
		{name: "ambiguous-three", content: "x-x-x", oldString: "x", newString: "y", wantErr: true, count: 3},
		{name: "replace-all-two", content: "a a", oldString: "a", newString: "b", replaceAll: true, want: "b b", count: 2},
		{name: "replace-all-three", content: "x-x-x", oldString: "x", newString: "y", replaceAll: true, want: "y-y-y", count: 3},
		{name: "replace-all-overlap-counts-nonoverlap", content: "aaaa", oldString: "aa", newString: "b", replaceAll: true, want: "bb", count: 2},
		{name: "replace-all-delete-lines-counts-actual", content: "x x\nx\n", oldString: "x", newString: "", replaceAll: true, want: "x ", count: 2},
		{name: "multiline", content: "a\nb\nc", oldString: "a\nb", newString: "x\ny", want: "x\ny\nc", count: 1},
		{name: "multiline-delete", content: "a\nb\nc\n", oldString: "b\nc\n", newString: "", want: "a\n", count: 1},
		{name: "unicode", content: "alpha 世界 omega", oldString: "世界", newString: "宇宙", want: "alpha 宇宙 omega", count: 1},
		{name: "emoji", content: "a🙂b", oldString: "🙂", newString: "🙃", want: "a🙃b", count: 1},
		{name: "nul-byte", content: "a\x00b", oldString: "\x00", newString: "-", want: "a-b", count: 1},
		{name: "dollar-literal", content: "price=X", oldString: "X", newString: "$1", want: "price=$1", count: 1},
		{name: "backslash-literal", content: `a\b`, oldString: `\`, newString: `/`, want: "a/b", count: 1},
		{name: "tabs", content: "a\tb", oldString: "\t", newString: "  ", want: "a  b", count: 1},
		{name: "spaces", content: "a  b", oldString: "  ", newString: " ", want: "a b", count: 1},
		{name: "quotes", content: `say "hi"`, oldString: `"hi"`, newString: `"bye"`, want: `say "bye"`, count: 1},
		{name: "apostrophe", content: "don't", oldString: "'", newString: "’", want: "don’t", count: 1},
		{name: "crlf-literal", content: "a\r\nb", oldString: "\r\n", newString: "|", want: "a|b", count: 1},
		{name: "leading-newline", content: "\na", oldString: "\n", newString: "x\n", want: "x\na", count: 1},
		{name: "trailing-newline-replace", content: "a\n", oldString: "a\n", newString: "b\n", want: "b\n", count: 1},
		{name: "case-sensitive", content: "a A", oldString: "A", newString: "B", want: "a B", count: 1},
		{name: "replace-all-unicode", content: "界,界", oldString: "界", newString: "世", replaceAll: true, want: "世,世", count: 2},
		{name: "longer-replacement", content: "a-b", oldString: "-", newString: "---", want: "a---b", count: 1},
		{name: "shorter-replacement", content: "a---b", oldString: "---", newString: "-", want: "a-b", count: 1},
	}
	if len(tests) < 30 {
		t.Fatalf("task requires at least 30 ApplyEdit table cases, got %d", len(tests))
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, count, err := ApplyEdit(test.content, test.oldString, test.newString, test.replaceAll)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got output %q", got)
				}
				if test.count != 0 && count != test.count {
					t.Fatalf("error occurrence count=%d, want %d", count, test.count)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want || count != test.count {
				t.Fatalf("ApplyEdit()=(%q,%d), want (%q,%d)", got, count, test.want, test.count)
			}
		})
	}
}

func TestTask11EditStrictSemanticInput(t *testing.T) {
	path, state, _ := editFixture(t, "semantic.txt", "x x")
	tool := &FileEditTool{ReadState: state}

	unknown, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "x", "new_string": "y", "surprise": true,
	})
	if !unknown.IsError || !strings.Contains(unknown.Content, "unknown field") {
		t.Fatalf("strict input should reject unknown field, got %+v", unknown)
	}

	result, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "x", "new_string": "y", "replace_all": "true",
	})
	if result.IsError {
		t.Fatalf("semantic string boolean should succeed: %s", result.Content)
	}
	data, ok := result.Data.(EditResult)
	if !ok || !data.ReplaceAll || !data.ReplaceAllUsed || data.Occurrences != 2 {
		t.Fatalf("unexpected typed semantic result: %#v", result.Data)
	}
}

func TestTask11EditTypedContractAndMapper(t *testing.T) {
	path, state, _ := editFixture(t, "typed.txt", "alpha")
	tool := &FileEditTool{ReadState: state}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "alpha", "new_string": "beta",
	})
	if result.IsError {
		t.Fatalf("edit failed: %s", result.Content)
	}
	data, ok := result.Data.(EditResult)
	if !ok {
		t.Fatalf("ToolResult.Data should be EditResult, got %T", result.Data)
	}
	if data.FilePath != path || data.Occurrences != 1 || data.Status != "success" {
		t.Fatalf("unexpected EditResult: %+v", data)
	}
	contract := tool.ToolContract()
	if !contract.Strict || contract.OutputSchema == nil || contract.MaxResultSizeChars != 100_000 {
		t.Fatalf("unexpected Edit contract: %+v", contract)
	}
	for _, key := range []string{"filePath", "oldString", "newString", "originalFile", "structuredPatch", "userModified", "replaceAll", "gitDiff"} {
		if _, exists := contract.OutputSchema.Properties[key]; !exists {
			t.Errorf("output schema missing %q", key)
		}
	}
	block := types.MapToolResult(tool, result, "toolu_edit")
	if block.ToolUseID != "toolu_edit" || !strings.Contains(block.Content, "updated successfully") {
		t.Fatalf("unexpected mapped block: %+v", block)
	}
	if _, ok := block.Data.(EditResult); !ok {
		t.Fatalf("mapped block lost typed data: %T", block.Data)
	}
}

func TestTask11EditTeamMemorySecretGuard(t *testing.T) {
	dir := t.TempDir()
	teamDir := filepath.Join(dir, ".claude", "memory", "team")
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(teamDir, "notes.md")
	original := "token: placeholder\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	recordStrongReadEvidenceForTest(t, state, path)
	var listenerCalls atomic.Int32
	tool := &FileEditTool{
		AllowedDirs: []string{dir},
		ReadState:   state,
		ChangeListener: func(EditChangeEvent) {
			listenerCalls.Add(1)
		},
	}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "placeholder",
		"new_string": "sk-ant-api03-abcdefghijklmnopqrstuvwxyz123456",
	})
	if !result.IsError || !strings.Contains(strings.ToLower(result.Content), "team memory") {
		t.Fatalf("expected team-memory rejection, got %+v", result)
	}
	written, _ := os.ReadFile(path)
	if string(written) != original || listenerCalls.Load() != 0 {
		t.Fatalf("rejected edit had side effects: content=%q listener=%d", written, listenerCalls.Load())
	}
}

type task11SkillManager struct {
	mu        sync.Mutex
	dirs      []string
	activated []string
}

func (m *task11SkillManager) AddDirectories(dirs []string) {
	m.mu.Lock()
	m.dirs = append(m.dirs, dirs...)
	m.mu.Unlock()
}

func (m *task11SkillManager) ActivateConditionalForPath(path string) {
	m.mu.Lock()
	m.activated = append(m.activated, path)
	m.mu.Unlock()
}

func TestTask11EditSkillDiscoveryAndConditionalActivation(t *testing.T) {
	ResetDynamicSkillTriggersForTest()
	t.Cleanup(ResetDynamicSkillTriggersForTest)
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	RegisterDynamicSkillDirTrigger(targetDir, "go-edit")
	path := filepath.Join(targetDir, "main.go")
	if err := os.WriteFile(path, []byte("package old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	recordStrongReadEvidenceForTest(t, state, path)
	manager := &task11SkillManager{}
	tool := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state, SkillManager: manager}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "old", "new_string": "new",
	})
	if result.IsError {
		t.Fatalf("edit failed: %s", result.Content)
	}
	manager.mu.Lock()
	activated := append([]string(nil), manager.activated...)
	manager.mu.Unlock()
	if len(activated) != 1 || canonicalPathForComparison(activated[0]) != canonicalPathForComparison(path) {
		t.Fatalf("conditional skill activation mismatch: %v", activated)
	}

	unrelated := filepath.Join(dir, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	unrelatedState := NewReadFileState()
	recordStrongReadEvidenceForTest(t, unrelatedState, unrelated)
	unrelatedManager := &task11SkillManager{}
	unrelatedTool := &FileEditTool{AllowedDirs: []string{dir}, ReadState: unrelatedState, SkillManager: unrelatedManager}
	if result, _ = unrelatedTool.Execute(context.Background(), map[string]any{
		"file_path": unrelated, "old_string": "a", "new_string": "b",
	}); result.IsError {
		t.Fatalf("unrelated edit failed: %s", result.Content)
	}
	unrelatedManager.mu.Lock()
	defer unrelatedManager.mu.Unlock()
	if len(unrelatedManager.activated) != 0 {
		t.Fatalf("unrelated file activated conditional skills: %v", unrelatedManager.activated)
	}
}

func TestTask11EditGitDiffAnalyticsAndListener(t *testing.T) {
	path, state, absPath := editFixture(t, "diff.txt", "a\n")
	wantDiff := &EditGitDiff{
		Filename: "diff.txt", Status: "modified", Additions: 1, Deletions: 1, Changes: 2, Patch: "@@ -1 +1 @@\n-a\n+b",
	}
	var eventsMu sync.Mutex
	events := map[string]int{}
	var observed EditChangeEvent
	tool := &FileEditTool{
		ReadState: state,
		GitDiffProvider: func(_ context.Context, gotPath string) (*EditGitDiff, error) {
			if canonicalPathForComparison(gotPath) != canonicalPathForComparison(absPath) {
				t.Fatalf("git diff path=%q, want %q", gotPath, absPath)
			}
			return wantDiff, nil
		},
		AnalyticsHook: func(event string, _ map[string]any) {
			eventsMu.Lock()
			events[event]++
			eventsMu.Unlock()
		},
		ChangeListener: func(event EditChangeEvent) { observed = event },
	}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "a", "new_string": "b",
	})
	if result.IsError {
		t.Fatalf("edit failed: %s", result.Content)
	}
	data := result.Data.(EditResult)
	if data.GitDiff == nil || data.GitDiff.Changes != 2 {
		t.Fatalf("typed gitDiff missing: %+v", data.GitDiff)
	}
	var wire map[string]any
	if err := json.Unmarshal([]byte(result.Content), &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["gitDiff"].(map[string]any); !ok {
		t.Fatalf("wire gitDiff missing: %v", wire)
	}
	if observed.After != "b\n" || observed.GitDiff == nil || observed.Occurrences != 1 {
		t.Fatalf("listener payload mismatch: %+v", observed)
	}
	eventsMu.Lock()
	defer eventsMu.Unlock()
	for _, name := range []string{"tengu_edit_string_lengths", "tengu_file_operation", "tengu_tool_use_diff_computed"} {
		if events[name] != 1 {
			t.Errorf("analytics event %q count=%d, want 1", name, events[name])
		}
	}
}

func TestTask11DefaultEditGitDiffProvider(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	runGit("init", "-q")
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "a.txt")
	runGit("-c", "user.name=Task11", "-c", "user.email=task11@example.invalid", "commit", "-qm", "base")
	if err := os.WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	SetRemoteGitDiffEnabled(true)
	t.Cleanup(func() { SetRemoteGitDiffEnabled(false) })
	diff, err := defaultEditGitDiffProvider(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if diff == nil || diff.Status != "modified" || diff.Filename != "a.txt" || diff.Additions != 1 || diff.Deletions != 1 || diff.Changes != 2 {
		t.Fatalf("unexpected real git diff: %+v", diff)
	}
}

func TestTask11FileHistoryConcurrentStoresAndChronology(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".claude", "file-history")
	path := filepath.Join(t.TempDir(), "target.txt")
	stores := []*FileHistoryStore{NewFileHistoryStore(root), NewFileHistoryStore(root)}
	const entries = 24
	base := time.Now().UnixMilli()
	var wg sync.WaitGroup
	errs := make(chan error, entries)
	for i := 0; i < entries; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := strings.Repeat(string(rune('a'+i%20)), 64*1024)
			errs <- stores[i%len(stores)].TrackEdit(FileHistoryEntry{
				Path: path, Before: "before", After: payload, Tool: "Edit", Ts: base + int64(entries-i),
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent TrackEdit: %v", err)
		}
	}
	got, err := NewFileHistoryStore(root).ListEdits(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != entries {
		t.Fatalf("history entries=%d, want %d", len(got), entries)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Ts < got[i-1].Ts {
			t.Fatalf("history not chronological at %d: %d < %d", i, got[i].Ts, got[i-1].Ts)
		}
		if got[i].EditID == "" || got[i].Hash == "" {
			t.Fatalf("history entry missing identity/hash: %+v", got[i])
		}
	}
}

func TestTask11EditPreservesModeAndAtomicVisibility(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.txt")
	original := "A\n" + strings.Repeat("x\n", 128*1024)
	edited := "B" + original[1:]
	if err := os.WriteFile(path, []byte(original), 0o750); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	readResult, err := (&FileReadTool{AllowedDirs: []string{dir}, ReadState: state}).Execute(
		context.Background(), map[string]any{"file_path": path, "offset": 1, "limit": 1},
	)
	if err != nil || readResult.IsError {
		t.Fatalf("targeted production Read failed: result=%+v err=%v", readResult, err)
	}
	tool := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}

	done := make(chan types.ToolResult, 1)
	go func() {
		result, _ := tool.Execute(context.Background(), map[string]any{
			"file_path": path, "old_string": "A", "new_string": "B",
		})
		done <- result
	}()
	for {
		select {
		case result := <-done:
			if result.IsError {
				t.Fatalf("edit failed: %s", result.Content)
			}
			goto complete
		default:
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reader observed missing/unreadable target: %v", err)
			}
			if string(content) != original && string(content) != edited {
				t.Fatalf("reader observed partial content: length=%d prefix=%q", len(content), content[:min(8, len(content))])
			}
		}
	}

complete:
	content, _ := os.ReadFile(path)
	if string(content) != edited {
		t.Fatalf("final content mismatch")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o750 {
		t.Fatalf("mode=%#o, want %#o", info.Mode().Perm(), os.FileMode(0o750))
	}
}

func TestTask11ConcurrentEditsShareOneTransaction(t *testing.T) {
	for attempt := 0; attempt < 12; attempt++ {
		path, state, _ := editFixture(t, "concurrent.txt", "alpha beta")
		tool := &FileEditTool{ReadState: state}
		start := make(chan struct{})
		results := make(chan types.ToolResult, 2)
		for _, replacement := range []struct{ old, new string }{{"alpha", "ALPHA"}, {"beta", "BETA"}} {
			replacement := replacement
			go func() {
				<-start
				result, _ := tool.Execute(context.Background(), map[string]any{
					"file_path": path, "old_string": replacement.old, "new_string": replacement.new,
				})
				results <- result
			}()
		}
		close(start)
		for i := 0; i < 2; i++ {
			if result := <-results; result.IsError {
				t.Fatalf("attempt %d concurrent edit failed: %s", attempt, result.Content)
			}
		}
		content, _ := os.ReadFile(path)
		if string(content) != "ALPHA BETA" {
			t.Fatalf("attempt %d lost update: %q", attempt, content)
		}
	}
}

func TestTask11LargeSparseStructuredPatchIsBounded(t *testing.T) {
	const lines = 8_000
	oldLines := make([]string, lines)
	newLines := make([]string, lines)
	for i := 0; i < lines; i++ {
		line := "line-" + strings.Repeat("0", 6-len(string(rune(i%10)))) + string(rune('0'+i%10))
		oldLines[i] = line
		newLines[i] = line
	}
	newLines[4_000] = "changed-line"
	started := time.Now()
	hunks := generateUnifiedHunks(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"), 3)
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("large sparse diff took %s", elapsed)
	}
	if len(hunks) != 1 || len(hunks[0].Lines) > 10 {
		t.Fatalf("expected one bounded hunk, got %+v", hunks)
	}
}

func TestTask11EditNewSettingsValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")
	tool := &FileEditTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "", "new_string": "{not-json",
	})
	if !result.IsError || !strings.Contains(result.Content, "invalid JSON") {
		t.Fatalf("invalid new settings should fail, got %+v", result)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid settings edit created a file: %v", err)
	}
}

func TestTask11EditSupportedEncodingRoundTrips(t *testing.T) {
	tests := []struct {
		name string
		enc  FileEncoding
		bom  []byte
	}{
		{name: "utf8-bom", enc: EncodingUTF8BOM, bom: bomUTF8},
		{name: "utf16-le", enc: EncodingUTF16LE, bom: bomUTF16LE},
		{name: "utf16-be", enc: EncodingUTF16BE, bom: bomUTF16BE},
		{name: "latin1", enc: EncodingLatin1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "encoded.txt")
			original := "café\n"
			want := "thé\n"
			if err := os.WriteFile(path, encodeWriteBytes(original, test.enc, test.bom), 0o640); err != nil {
				t.Fatal(err)
			}
			state := NewReadFileState()
			recordStrongReadEvidenceForTest(t, state, path)
			tool := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}
			result, _ := tool.Execute(context.Background(), map[string]any{
				"file_path": path, "old_string": "café", "new_string": "thé",
			})
			if result.IsError {
				t.Fatalf("encoded edit failed: %s", result.Content)
			}
			raw, _ := os.ReadFile(path)
			if got := detectFileEncoding(raw); got.Encoding != test.enc {
				t.Fatalf("encoding=%s, want %s (raw=%x)", got.Encoding, test.enc, raw)
			} else if decoded := decodeFileBytes(raw, got); decoded != want {
				t.Fatalf("decoded=%q, want %q", decoded, want)
			}
			info, _ := os.Stat(path)
			if info.Mode().Perm() != 0o640 {
				t.Fatalf("mode=%#o, want %#o", info.Mode().Perm(), os.FileMode(0o640))
			}
		})
	}
}

func TestTask11EditCRLFSearchMatchesLFFile(t *testing.T) {
	path, state, _ := editFixture(t, "lf.txt", "alpha\nbeta\n")
	tool := &FileEditTool{ReadState: state}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "alpha\r\nbeta",
		"new_string": "one\r\ntwo",
	})
	if result.IsError {
		t.Fatalf("cross-line-ending edit failed: %s", result.Content)
	}
	written, _ := os.ReadFile(path)
	if string(written) != "one\ntwo\n" {
		t.Fatalf("LF file style not preserved: %q", written)
	}
}

func TestTask11EditNewFileNormalizesToLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "new.txt")
	tool := &FileEditTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "", "new_string": "one\r\ntwo\r\n",
	})
	if result.IsError {
		t.Fatalf("new-file edit failed: %s", result.Content)
	}
	written, _ := os.ReadFile(path)
	if string(written) != "one\ntwo\n" {
		t.Fatalf("new file did not use default LF endings: %q", written)
	}
	if data := result.Data.(EditResult); data.NewString != "one\r\ntwo\r\n" || data.OriginalFile != "" {
		t.Fatalf("result should preserve requested newString and empty original: %+v", data)
	}
}

func TestTask11StructuredPatchExpandsLeadingTabsOnly(t *testing.T) {
	path, state, _ := editFixture(t, "tabs.txt", "\told\ninside\told\n")
	tool := &FileEditTool{ReadState: state}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "old", "new_string": "new", "replace_all": true,
	})
	if result.IsError {
		t.Fatalf("tab edit failed: %s", result.Content)
	}
	patch := result.Data.(EditResult).StructuredPatch
	joined := ""
	for _, hunk := range patch {
		joined += strings.Join(hunk.Lines, "\n")
	}
	if !strings.Contains(joined, "-  old") || !strings.Contains(joined, "+  new") {
		t.Fatalf("leading tabs were not expanded in patch: %q", joined)
	}
	if !strings.Contains(joined, "-inside\told") || !strings.Contains(joined, "+inside\tnew") {
		t.Fatalf("non-leading tabs should be preserved in patch: %q", joined)
	}
}

func TestTask11ReadEditTargetRejectsDifferentInode(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	firstInfo, _ := os.Stat(first)
	if _, err := readEditTarget(second, firstInfo); err == nil || !strings.Contains(err.Error(), "replaced") {
		t.Fatalf("expected inode mismatch rejection, got %v", err)
	}
}
