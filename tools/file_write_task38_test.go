package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

type task38Runtime struct{ snapshot types.ToolRuntimeContext }

func (r task38Runtime) ToolRuntimeContext() types.ToolRuntimeContext { return r.snapshot }

func task38Result(t *testing.T, result types.ToolResult) FileWriteResult {
	t.Helper()
	if result.IsError {
		t.Fatalf("Write failed: %s", result.Content)
	}
	value, ok := coerceFileWriteResult(result.Data)
	if !ok {
		t.Fatalf("missing typed FileWriteResult: %#v", result.Data)
	}
	return value
}

func task38ReadEntry(t *testing.T, path, content string) ReadFileEntry {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return ReadFileEntry{TimestampMs: info.ModTime().UnixMilli(), Content: content, LastTool: "Read", DedupEligible: true}
}

func TestFileWrite_Task38ContractAndModelText(t *testing.T) {
	root := t.TempDir()
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: NewReadFileState()}
	contract := tool.ToolContract()
	if !contract.Strict || contract.OutputSchema == nil || !tool.Schema().RejectsUnknownFields() {
		t.Fatalf("Write must publish strict input/output contracts: %#v", contract)
	}
	for _, key := range []string{"type", "filePath", "content", "structuredPatch", "originalFile", "gitDiff"} {
		if _, ok := contract.OutputSchema.Properties[key]; !ok {
			t.Fatalf("output schema missing %q", key)
		}
	}

	path := filepath.Join(root, "created.txt")
	created := task38Result(t, mustToolResult(tool.Execute(context.Background(), map[string]any{
		"file_path": path, "content": "alpha\n",
	})))
	if created.Type != "create" || created.FilePath != path || created.Content != "alpha\n" || created.OriginalFile != nil || len(created.StructuredPatch) != 0 {
		t.Fatalf("unexpected create result: %#v", created)
	}
	assertWriteWireKeys(t, mustJSONMap(t, mustToolResult(tool.Execute(context.Background(), map[string]any{
		"file_path": path, "content": "beta\n",
	})).Content))
	updatedRaw := mustToolResult(tool.Execute(context.Background(), map[string]any{"file_path": path, "content": "gamma\n"}))
	updated := task38Result(t, updatedRaw)
	if updated.Type != "update" || updated.OriginalFile == nil || *updated.OriginalFile != "beta\n" || len(updated.StructuredPatch) == 0 {
		t.Fatalf("unexpected update result: %#v", updated)
	}
	assertWriteWireKeys(t, mustJSONMap(t, updatedRaw.Content))
	if got := tool.MapToolResultToToolResultBlock(created, "toolu_create"); got.Content != "File created successfully at: "+path || got.ToolUseID != "toolu_create" {
		t.Fatalf("create mapper mismatch: %#v", got)
	}
	if got := tool.MapToolResultToToolResultBlock(updated, "toolu_update"); got.Content != "The file "+path+" has been updated successfully." {
		t.Fatalf("update mapper mismatch: %#v", got)
	}
}

func mustToolResult(result types.ToolResult, err error) types.ToolResult {
	if err != nil {
		panic(err)
	}
	return result
}

func mustJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return out
}

func assertWriteWireKeys(t *testing.T, out map[string]any) {
	t.Helper()
	allowed := map[string]bool{"type": true, "filePath": true, "content": true, "structuredPatch": true, "originalFile": true, "gitDiff": true}
	for key := range out {
		if !allowed[key] {
			t.Fatalf("Go-only Write field %q leaked into %#v", key, out)
		}
	}
	for _, key := range []string{"type", "filePath", "content", "structuredPatch", "originalFile"} {
		if _, ok := out[key]; !ok {
			t.Fatalf("required Write field %q missing from %#v", key, out)
		}
	}
}

func TestFileWrite_Task38StrictValidationAndPathExpansion(t *testing.T) {
	root := t.TempDir()
	tool := &FileWriteTool{
		Runtime:   task38Runtime{types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}}},
		ReadState: NewReadFileState(),
	}
	for _, input := range []map[string]any{
		{"file_path": "x", "content": "x", "extra": true},
		{"file_path": 7, "content": "x"},
		{"file_path": "x", "content": 7},
		{"file_path": "x"},
	} {
		result, _ := tool.Execute(context.Background(), input)
		if !result.IsError {
			t.Fatalf("invalid input accepted: %#v", input)
		}
	}
	result := task38Result(t, mustToolResult(tool.Execute(context.Background(), map[string]any{
		"file_path": "  nested/file.txt  ", "content": "ok",
	})))
	want := filepath.Join(root, "nested", "file.txt")
	if result.FilePath != want {
		t.Fatalf("relative/whitespace expansion = %q, want %q", result.FilePath, want)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	backfilled, err := tool.BackfillObservableInput(map[string]any{"file_path": "~/note.txt", "content": "x"})
	if err != nil || backfilled["file_path"] != filepath.Join(home, "note.txt") {
		t.Fatalf("tilde backfill = %#v, %v", backfilled, err)
	}
}

func TestFileWrite_Task38PermissionOrdering(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "main.go")
	tool := &FileWriteTool{AllowedDirs: []string{root}}
	input := map[string]any{"file_path": path, "content": "package main"}
	rule := types.PermissionRuleValue{ToolName: "Edit", RuleContent: filepath.Join(root, "**")}

	decision, _ := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "acceptEdits",
		DeniedRules: []types.PermissionRuleValue{rule},
	}})
	if decision.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("deny must precede acceptEdits: %#v", decision)
	}
	decision, _ = tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "acceptEdits",
		AskRules: []types.PermissionRuleValue{rule},
	}})
	if decision.Behavior != types.PermissionBehaviorAsk || !decision.Required {
		t.Fatalf("ask must precede acceptEdits: %#v", decision)
	}
	decision, _ = tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "acceptEdits",
	}})
	if decision.Behavior != types.PermissionBehaviorAllow || decision.UpdatedInput["file_path"] != path {
		t.Fatalf("acceptEdits working-dir decision: %#v", decision)
	}
	decision, _ = tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, AllowedRules: []types.PermissionRuleValue{rule},
	}})
	if decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("allow rule not honored: %#v", decision)
	}

	outside := filepath.Join(t.TempDir(), "outside.txt")
	decision, _ = tool.CheckPermissions(context.Background(), map[string]any{"file_path": outside, "content": "x"}, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root},
	}})
	if decision.Behavior != types.PermissionBehaviorAsk || len(decision.Suggestions) < 2 || decision.Suggestions[0].Type != types.PermissionUpdateAddDirectories {
		t.Fatalf("outside-cwd ask/suggestions mismatch: %#v", decision)
	}
}

func TestFileWrite_Task38StaleContentFallback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sync.txt")
	if err := os.WriteFile(path, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	state.Set(path, ReadFileEntry{TimestampMs: 1, Content: "same\n", LastTool: "Read"})
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: state}
	if result, _ := tool.Execute(context.Background(), map[string]any{"file_path": path, "content": "next\n"}); result.IsError {
		t.Fatalf("mtime-only change should fall back to content: %s", result.Content)
	}

	state.Set(path, ReadFileEntry{TimestampMs: 1, Content: "next\n", LastTool: "Read"})
	if err := os.WriteFile(path, []byte("external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, _ := tool.Execute(context.Background(), map[string]any{"file_path": path, "content": "model\n"})
	if !result.IsError || result.Content != "File has been modified since read, either by the user or by a linter. Read it again before attempting to write it." {
		t.Fatalf("true stale result mismatch: %#v", result)
	}
}

func TestFileWrite_Task38PostWriteReadDoesNotDedup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.txt")
	state := NewReadFileState()
	write := &FileWriteTool{AllowedDirs: []string{root}, ReadState: state}
	if result, _ := write.Execute(context.Background(), map[string]any{"file_path": path, "content": "visible\n"}); result.IsError {
		t.Fatal(result.Content)
	}
	read := &FileReadTool{AllowedDirs: []string{root}, ReadState: state}
	result, _ := read.Execute(context.Background(), map[string]any{"file_path": path})
	if result.IsError || strings.Contains(result.Content, fileUnchangedStubText()) || !strings.Contains(result.Content, "visible") {
		t.Fatalf("Read after Write must return content: %#v", result)
	}
	if second, _ := write.Execute(context.Background(), map[string]any{"file_path": path, "content": "again\n"}); second.IsError {
		t.Fatalf("Write after Write should use refreshed state: %s", second.Content)
	}
}

func TestFileWrite_Task38PreservesModelLineEndingsAndTSEncoding(t *testing.T) {
	root := t.TempDir()
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: NewReadFileState()}
	for name, content := range map[string]string{"crlf.txt": "a\r\nb\r\n", "cr.txt": "a\rb\r"} {
		path := filepath.Join(root, name)
		if result, _ := tool.Execute(context.Background(), map[string]any{"file_path": path, "content": content}); result.IsError {
			t.Fatal(result.Content)
		}
		raw, _ := os.ReadFile(path)
		if string(raw) != content {
			t.Fatalf("%s bytes rewritten: %q", name, raw)
		}
	}

	path := filepath.Join(root, "be.txt")
	be := append(append([]byte(nil), bomUTF16BE...), encodeUTF16([]rune("old"), true)...)
	if err := os.WriteFile(path, be, 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	state.Set(path, task38ReadEntry(t, path, "old"))
	beTool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: state}
	if result, _ := beTool.Execute(context.Background(), map[string]any{"file_path": path, "content": "utf8"}); result.IsError {
		t.Fatal(result.Content)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "utf8" {
		t.Fatalf("TS defaults UTF-16BE to UTF-8, got %v", raw)
	}
}

func TestFileWrite_Task38SymlinkAndMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics require developer mode on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target.sh")
	if err := os.WriteFile(target, []byte("old\n"), 0o751); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.sh")
	if err := os.Symlink("target.sh", link); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	state.Set(link, task38ReadEntry(t, target, "old\n"))
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: state}
	if result, _ := tool.Execute(context.Background(), map[string]any{"file_path": link, "content": "new\n"}); result.IsError {
		t.Fatal(result.Content)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: %v, %v", info, err)
	}
	raw, _ := os.ReadFile(target)
	info, _ := os.Stat(target)
	if string(raw) != "new\n" || info.Mode().Perm() != 0o751 {
		t.Fatalf("target content/mode = %q %o", raw, info.Mode().Perm())
	}

	outside := t.TempDir()
	escapeTarget := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(escapeTarget, []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(root, "escape.txt")
	if err := os.Symlink(escapeTarget, escape); err != nil {
		t.Fatal(err)
	}
	if result, _ := tool.Execute(context.Background(), map[string]any{"file_path": escape, "content": "bad"}); !result.IsError {
		t.Fatal("AllowedDirs symlink escape was accepted")
	}
}

type task38SkillActivator struct {
	mu        sync.Mutex
	dirs      []string
	activated []string
}

func (s *task38SkillActivator) AddDirectories(dirs []string) {
	s.mu.Lock()
	s.dirs = append(s.dirs, dirs...)
	s.mu.Unlock()
}
func (s *task38SkillActivator) ActivateConditionalForPath(path string) {
	s.mu.Lock()
	s.activated = append(s.activated, path)
	s.mu.Unlock()
}

func TestFileWrite_Task38SkillLifecycleBestEffort(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".claude", "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: review\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := &task38SkillActivator{}
	path := filepath.Join(root, "src", "main.go")
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: NewReadFileState(), SkillManager: manager}
	if result, _ := tool.Execute(context.Background(), map[string]any{"file_path": path, "content": "package main\n"}); result.IsError {
		t.Fatal(result.Content)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.dirs) == 0 || len(manager.activated) != 1 || manager.activated[0] != path {
		t.Fatalf("skill lifecycle not invoked: dirs=%v paths=%v", manager.dirs, manager.activated)
	}
}

type task38PreparationTracker struct{ called bool }

func (p *task38PreparationTracker) BeforeFileEdited(_ context.Context, _ string) error {
	p.called = true
	return nil
}

func TestFileWrite_Task38HistoryLSPVSCodeAndRemoteLifecycle(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lifecycle.go")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	state.Set(path, task38ReadEntry(t, path, "old\n"))
	history := NewFileHistoryStore(filepath.Join(root, ".claude", "file-history"))
	lsp := &fakeLSPSync{diagnostics: []LSPDiagnostic{{Message: "x"}}}
	prep := &task38PreparationTracker{}
	var notifiedPath, notifiedOld, notifiedNew string
	var notified atomic.Int32
	analytics := map[string]map[string]any{}
	tool := &FileWriteTool{
		AllowedDirs: []string{root}, ReadState: state, HistoryStore: history,
		HistoryEnabled: func() bool { return true }, HistoryCorrelationID: func(context.Context) string { return "parent-message-uuid" },
		LSP: lsp, DiagnosticsTracker: &fakeDiagTracker{}, PreparationTracker: prep,
		VSCodeNotifier:       fakeWriteVSCodeNotifier{count: &notified, path: &notifiedPath, oldContent: &notifiedOld, newContent: &notifiedNew},
		RemoteGitDiffEnabled: func() bool { return true },
		GitDiffProvider: func(context.Context, string) (*EditGitDiff, error) {
			return &EditGitDiff{Filename: "lifecycle.go", Status: "modified", Additions: 1, Deletions: 1, Changes: 2, Patch: "@@"}, nil
		},
		AnalyticsHook: func(event string, payload map[string]any) { analytics[event] = payload },
	}
	result := task38Result(t, mustToolResult(tool.Execute(context.Background(), map[string]any{"file_path": path, "content": "new\n"})))
	if !prep.called || lsp.didChangeCount != 1 || lsp.didSaveCount != 1 || notified.Load() != 1 {
		t.Fatalf("lifecycle calls prep=%v change=%d save=%d vscode=%d", prep.called, lsp.didChangeCount, lsp.didSaveCount, notified.Load())
	}
	if notifiedPath != path || notifiedOld != "old\n" || notifiedNew != "new\n" {
		t.Fatalf("VSCode payload = %q %q -> %q", notifiedPath, notifiedOld, notifiedNew)
	}
	if result.GitDiff == nil || result.GitDiff.Filename != "lifecycle.go" || analytics["tengu_tool_use_diff_computed"]["hasDiff"] != true {
		t.Fatalf("remote diff/analytics mismatch: %#v %#v", result.GitDiff, analytics)
	}
	entries, err := history.ListEdits(path)
	if err != nil || len(entries) != 1 || entries[0].EditID != "parent-message-uuid" || entries[0].Before != "old\n" {
		t.Fatalf("history correlation mismatch: %#v, %v", entries, err)
	}
}

func TestFileWrite_Task38HistoryRunsBeforeStaleAndCanBeDisabled(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "stale.txt")
	if err := os.WriteFile(path, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	state.Set(path, ReadFileEntry{TimestampMs: 1, Content: "old"})
	history := NewFileHistoryStore(filepath.Join(root, "history"))
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: state, HistoryStore: history, HistoryEnabled: func() bool { return true }}
	result, _ := tool.Execute(context.Background(), map[string]any{"file_path": path, "content": "model"})
	entries, _ := history.ListEdits(path)
	if !result.IsError || len(entries) != 1 || entries[0].Before != "external" {
		t.Fatalf("pre-stale history missing: result=%#v entries=%#v", result, entries)
	}

	disabledPath := filepath.Join(root, "disabled.txt")
	disabledStore := NewFileHistoryStore(filepath.Join(root, "disabled-history"))
	disabled := &FileWriteTool{AllowedDirs: []string{root}, ReadState: NewReadFileState(), HistoryStore: disabledStore, HistoryEnabled: func() bool { return false }}
	if result, _ := disabled.Execute(context.Background(), map[string]any{"file_path": disabledPath, "content": "x"}); result.IsError {
		t.Fatal(result.Content)
	}
	if entries, _ := disabledStore.ListEdits(disabledPath); len(entries) != 0 {
		t.Fatalf("disabled history wrote entries: %#v", entries)
	}
}

func TestFileWrite_Task38ParityPolicies(t *testing.T) {
	root := t.TempDir()
	events := []string{}
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: NewReadFileState(), AnalyticsHook: func(event string, _ map[string]any) { events = append(events, event) }}
	for _, name := range []string{"notebook.ipynb", "README.md", "AGENTS.md", "CLAUDE.md"} {
		result, _ := tool.Execute(context.Background(), map[string]any{"file_path": filepath.Join(root, name), "content": "{}"})
		if result.IsError {
			t.Fatalf("parity Write rejected %s: %s", name, result.Content)
		}
		assertWriteWireKeys(t, mustJSONMap(t, result.Content))
	}
	if strings.Join(events, ",") != "tengu_write_claudemd" {
		t.Fatalf("memory analytics should fire only for exact CLAUDE.md: %v", events)
	}
}

func TestFileWrite_Task38UTF16LEAndBOMPolicy(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "utf16.txt")
	original := encodeWriteBytes("old\r\n", EncodingUTF16LE, bomUTF16LE)
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	state.Set(path, task38ReadEntry(t, path, "old\n"))
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: state}
	content := "new\r\n"
	if result, _ := tool.Execute(context.Background(), map[string]any{"file_path": path, "content": content}); result.IsError {
		t.Fatal(result.Content)
	}
	raw, _ := os.ReadFile(path)
	if !bytes.Equal(raw, encodeWriteBytes(content, EncodingUTF16LE, bomUTF16LE)) {
		t.Fatalf("UTF-16LE output mismatch: %v", raw)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("UTF-16 mode not preserved: %o", info.Mode().Perm())
	}
}

// Keep time imported in this file so stale tests remain explicit about the
// intended mtime comparison granularity.
var _ = time.Millisecond
