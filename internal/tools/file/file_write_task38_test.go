package file

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	value, ok := result.Data.(FileWriteResult)
	if !ok {
		t.Fatalf("missing typed FileWriteResult: %#v", result.Data)
	}
	return value
}

func TestFileWrite_Task38ContractAndModelText(t *testing.T) {
	root := t.TempDir()
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: NewReadFileState()}
	metadata := tool.ToolMetadata(nil)
	if !metadata.Write || metadata.MaxResultSizeChars != 100_000 || !tool.Schema().RejectsUnknownFields() {
		t.Fatalf("Write must publish strict input metadata: %#v", metadata)
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
	for name, data := range map[string]any{
		"pointer": &created,
		"map":     map[string]any{"type": "create", "filePath": path},
	} {
		if got := tool.MapToolResultToToolResultBlock(data, "toolu_invalid"); !got.IsError {
			t.Fatalf("%s compatibility payload must be rejected: %#v", name, got)
		}
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
	allowed := map[string]bool{"type": true, "filePath": true, "content": true, "structuredPatch": true, "originalFile": true}
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
	rule := types.PermissionRuleValue{ToolName: "Write", RuleContent: filepath.Join(root, "**")}

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
	seedCanonicalFileReadState(t, state, path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime().Add(-time.Second), info.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: state}
	if result, _ := tool.Execute(context.Background(), map[string]any{"file_path": path, "content": "next\n"}); result.IsError {
		t.Fatalf("mtime-only change should fall back to content: %s", result.Content)
	}

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
	seedCanonicalFileReadState(t, state, path)
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
	seedCanonicalFileReadState(t, state, link)
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

func TestFileWrite_Task38ParityPolicies(t *testing.T) {
	root := t.TempDir()
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: NewReadFileState()}
	for _, name := range []string{"notebook.ipynb", "README.md", "AGENTS.md", "LUBAN.md"} {
		result, _ := tool.Execute(context.Background(), map[string]any{"file_path": filepath.Join(root, name), "content": "{}"})
		if result.IsError {
			t.Fatalf("parity Write rejected %s: %s", name, result.Content)
		}
		assertWriteWireKeys(t, mustJSONMap(t, result.Content))
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
	seedCanonicalFileReadState(t, state, path)
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
