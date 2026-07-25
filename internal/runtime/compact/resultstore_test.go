package compact

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

type unmarshalableResultStoreTextBlock struct {
	Type        types.ContentType `json:"type"`
	Unsupported chan int          `json:"unsupported"`
}

func (unmarshalableResultStoreTextBlock) GetType() types.ContentType {
	return types.ContentTypeText
}

func TestResultStoreErrorsUseRuntimeLanguageAndPreserveFilesystemCause(t *testing.T) {
	previousLanguage := i18n.DetectOrLoadLanguage()
	if err := i18n.SaveLanguage(i18n.LangEN); err != nil {
		t.Fatalf("set English test language: %v", err)
	}
	t.Cleanup(func() {
		if err := i18n.SaveLanguage(previousLanguage); err != nil {
			t.Errorf("restore test language: %v", err)
		}
	})

	parentFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed parent file: %v", err)
	}
	rs := &ResultStore{dir: filepath.Join(parentFile, "tool-results")}
	_, _, err := rs.PersistRawOutput("bash", []byte("raw-output"), 0)
	if err == nil {
		t.Fatal("expected persistence error")
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("error did not preserve *os.PathError: %T %v", err, err)
	}
	english := err.Error()
	if !strings.HasPrefix(english, "persist raw tool output: ") || !strings.Contains(english, parentFile) {
		t.Fatalf("English output or raw path changed: %q", english)
	}

	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatalf("set Chinese test language: %v", err)
	}
	chinese := err.Error()
	if chinese == english || !strings.Contains(chinese, parentFile) || !strings.Contains(chinese, pathErr.Error()) {
		t.Fatalf("localized error lost raw filesystem diagnostics: en=%q zh=%q", english, chinese)
	}
}

func TestResultStoreNilErrorUsesSemanticCopy(t *testing.T) {
	var rs *ResultStore
	_, originalSize, err := rs.PersistRawOutput("bash", []byte("raw"), 0)
	if err == nil {
		t.Fatal("expected nil-store error")
	}
	if originalSize != 3 {
		t.Fatalf("original size = %d, want 3", originalSize)
	}
	if got, want := err.Error(), i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCompactResultStoreUnavailable); got != want {
		t.Fatalf("nil-store error = %q, want %q", got, want)
	}
}

func TestResultStoreStructuredSerializationErrorIsSemanticAndTyped(t *testing.T) {
	rs := NewResultStore(t.TempDir())
	original := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_unserializable",
		ContentBlocks: []types.ContentBlock{
			unmarshalableResultStoreTextBlock{Type: types.ContentTypeText, Unsupported: make(chan int)},
		},
	}

	got, err := rs.ProcessResultForTool(original, "RawToolName")
	if err == nil {
		t.Fatal("expected serialization error")
	}
	var typeErr *json.UnsupportedTypeError
	if !errors.As(err, &typeErr) {
		t.Fatalf("error did not preserve *json.UnsupportedTypeError: %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "toolu_unserializable") || !strings.Contains(err.Error(), typeErr.Error()) {
		t.Fatalf("semantic error lost tool-use ID or raw JSON error: %v", err)
	}
	if len(got.ContentBlocks) != 1 {
		t.Fatalf("serialization failure changed original result: %#v", got)
	}
}

func TestResultStorePersistsLargeText(t *testing.T) {
	rs := NewResultStore(t.TempDir())
	content := strings.Repeat("line\n", 20)

	got, err := rs.ProcessResultForTool(types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_text",
		Content:   content,
		Metadata:  map[string]string{"maxResultSizeChars": "10"},
	}, "Bash")
	if err != nil {
		t.Fatalf("ProcessResultForTool: %v", err)
	}

	path := filepath.Join(rs.dir, "toolu_text.txt")
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected persisted text file: %v", err)
	}
	if string(saved) != content {
		t.Fatalf("persisted content mismatch")
	}
	assertPersistedWrapper(t, got.Content, path)
}

func TestResultStorePersistsStructuredTextAsJSON(t *testing.T) {
	rs := NewResultStore(t.TempDir())
	got, err := rs.ProcessResultForTool(types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_json",
		ContentBlocks: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: strings.Repeat("structured\n", 10)},
		},
		Metadata: map[string]string{"maxResultSizeChars": "10"},
	}, "Read")
	if err != nil {
		t.Fatalf("ProcessResultForTool: %v", err)
	}

	path := filepath.Join(rs.dir, "toolu_json.json")
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected persisted json file: %v", err)
	}
	if !strings.Contains(string(saved), `"type": "text"`) || !strings.Contains(string(saved), `"text": "structured`) {
		t.Fatalf("expected structured text JSON, got: %s", saved)
	}
	if got.HasStructuredContent() {
		t.Fatalf("persisted result should replace structured content with wrapper text")
	}
	assertPersistedWrapper(t, got.Content, path)
}

func TestResultStoreSkipsImageContent(t *testing.T) {
	rs := NewResultStore(t.TempDir())
	original := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_image",
		ContentBlocks: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: strings.Repeat("text", 20)},
			types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "abc"}},
		},
		Metadata: map[string]string{"maxResultSizeChars": "10"},
	}

	got, err := rs.ProcessResultForTool(original, "Bash")
	if err != nil {
		t.Fatalf("ProcessResultForTool: %v", err)
	}
	if got.Content != "" || len(got.ContentBlocks) != len(original.ContentBlocks) {
		t.Fatalf("image content should be left unchanged, got %#v", got)
	}
	if _, err := os.Stat(filepath.Join(rs.dir, "toolu_image.json")); !os.IsNotExist(err) {
		t.Fatalf("image result should not be persisted, stat err=%v", err)
	}
}

func TestResultStoreEmptyOutputMarker(t *testing.T) {
	rs := NewResultStore(t.TempDir())
	got, err := rs.ProcessResultForTool(types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_empty",
		Content:   " \n\t ",
	}, "Bash")
	if err != nil {
		t.Fatalf("ProcessResultForTool: %v", err)
	}
	if got.Content != "(Bash completed with no output)" {
		t.Fatalf("empty marker mismatch: %q", got.Content)
	}

	got, err = rs.ProcessResultForTool(types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_struct_empty",
		ContentBlocks: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "  "},
		},
	}, "MCP")
	if err != nil {
		t.Fatalf("ProcessResultForTool structured: %v", err)
	}
	if got.Content != "(MCP completed with no output)" || got.HasStructuredContent() {
		t.Fatalf("structured empty marker mismatch: %#v", got)
	}
}

func TestResultStoreExistingFileReplayIsStable(t *testing.T) {
	rs := NewResultStore(t.TempDir())
	existing := strings.Repeat("already persisted\n", 20)
	path := filepath.Join(rs.dir, "toolu_existing.txt")
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat existing file: %v", err)
	}

	block := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_existing",
		Content:   existing,
		Metadata:  map[string]string{"maxResultSizeChars": "10"},
	}
	first, err := rs.ProcessResultForTool(block, "Bash")
	if err != nil {
		t.Fatalf("first ProcessResultForTool: %v", err)
	}
	second, err := rs.ProcessResultForTool(block, "Bash")
	if err != nil {
		t.Fatalf("second ProcessResultForTool: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after replay: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		t.Fatalf("existing file should not be rewritten")
	}
	if runtime.GOOS != "windows" && after.Mode().Perm() != 0o600 {
		t.Fatalf("result mode = %#o, want 0600", after.Mode().Perm())
	}
	if first.Content != second.Content {
		t.Fatalf("replay wrapper should be byte-identical")
	}
	if !strings.Contains(first.Content, "already persisted") {
		t.Fatalf("wrapper preview should reflect create-once file content, got: %s", first.Content)
	}
}

func TestResultStorePreservesCallerDirectoryAndUsesPrivateOwnedModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	sessionDir := filepath.Join(t.TempDir(), "session-artifacts")
	if err := os.Mkdir(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rs := NewResultStore(sessionDir)
	// The caller may pass a workspace CWD (Bash does). ResultStore owns only
	// tool-results and must never tighten the caller-owned parent directory.
	assertResultStoreMode(t, sessionDir, 0o755)
	assertResultStoreMode(t, rs.dir, 0o700)

	content := strings.Repeat("private\n", 20)
	if _, err := rs.ProcessResultForTool(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "toolu_private", Content: content,
		Metadata: map[string]string{"maxResultSizeChars": "10"},
	}, "Bash"); err != nil {
		t.Fatal(err)
	}
	assertResultStoreMode(t, filepath.Join(rs.dir, "toolu_private.txt"), 0o600)
	rawPath, _, err := rs.PersistRawOutput("bash", []byte("private raw output"), 0)
	if err != nil {
		t.Fatal(err)
	}
	assertResultStoreMode(t, rawPath, 0o600)
}

func TestResultStoreRejectsTraversalToolUseIDs(t *testing.T) {
	sessionDir := t.TempDir()
	rs := NewResultStore(sessionDir)
	content := strings.Repeat("large\n", 20)
	for _, id := range []string{"../escape", "nested/result", ".", " tool "} {
		original := types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: id, Content: content,
			Metadata: map[string]string{"maxResultSizeChars": "10"},
		}
		got, err := rs.ProcessResultForTool(original, "Bash")
		if !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("ProcessResultForTool(%q) error = %v, want fs.ErrInvalid", id, err)
		}
		if got.Content != original.Content {
			t.Errorf("invalid ID %q changed original result", id)
		}
		if _, err := rs.PersistReplacement(id, content); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("PersistReplacement(%q) error = %v, want fs.ErrInvalid", id, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(sessionDir, "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("tool-use traversal created an outside file: %v", err)
	}
}

func TestResultStoreRejectsUntrustedEEXISTTargets(t *testing.T) {
	content := strings.Repeat("expected\n", 20)
	newBlock := func(id string) types.ToolResultBlock {
		return types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: id, Content: content,
			Metadata: map[string]string{"maxResultSizeChars": "10"},
		}
	}

	t.Run("partial file", func(t *testing.T) {
		rs := NewResultStore(t.TempDir())
		path := filepath.Join(rs.dir, "toolu_partial.txt")
		if err := os.WriteFile(path, []byte(content[:len(content)/2]), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := rs.ProcessResultForTool(newBlock("toolu_partial"), "Bash")
		if err == nil || got.Content != content {
			t.Fatalf("partial EEXIST result = %#v, error = %v", got, err)
		}
		saved, readErr := os.ReadFile(path)
		if readErr != nil || string(saved) != content[:len(content)/2] {
			t.Fatalf("partial file was trusted or overwritten: %q, %v", saved, readErr)
		}
	})

	t.Run("same size different content", func(t *testing.T) {
		rs := NewResultStore(t.TempDir())
		path := filepath.Join(rs.dir, "toolu_mismatch.txt")
		if err := os.WriteFile(path, bytesOf('x', len(content)), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := rs.ProcessResultForTool(newBlock("toolu_mismatch"), "Bash"); err == nil {
			t.Fatal("same-size mismatched EEXIST file was trusted")
		}
	})

	t.Run("non-regular file", func(t *testing.T) {
		rs := NewResultStore(t.TempDir())
		path := filepath.Join(rs.dir, "toolu_directory.txt")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := rs.ProcessResultForTool(newBlock("toolu_directory"), "Bash"); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("directory EEXIST error = %v, want fs.ErrInvalid", err)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("symlink", func(t *testing.T) {
			rs := NewResultStore(t.TempDir())
			outside := filepath.Join(t.TempDir(), "outside.txt")
			if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(rs.dir, "toolu_link.txt")); err != nil {
				t.Fatal(err)
			}
			if _, err := rs.ProcessResultForTool(newBlock("toolu_link"), "Bash"); !errors.Is(err, fs.ErrInvalid) {
				t.Fatalf("symlink EEXIST error = %v, want fs.ErrInvalid", err)
			}
			got, err := os.ReadFile(outside)
			if err != nil || string(got) != "outside" {
				t.Fatalf("symlink target changed: %q, %v", got, err)
			}
		})
	}
}

func TestResultStoreRejectsSymlinkRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires additional privileges on Windows")
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "session-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	rs := NewResultStore(link)
	_, err := rs.PersistReplacement("toolu_blocked", strings.Repeat("x", 100))
	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("symlink root error = %v, want fs.ErrInvalid", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "tool-results")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("result directory created through symlink root: %v", err)
	}
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for i := range result {
		result[i] = value
	}
	return result
}

func assertResultStoreMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %#o, want %#o", path, got, want)
	}
}

func TestResultStoreWriteFailureFallsBackToOriginalWithError(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("x"), 0644); err != nil {
		t.Fatalf("seed parent file: %v", err)
	}
	rs := &ResultStore{dir: filepath.Join(parentFile, "tool-results")}
	original := strings.Repeat("large\n", 20)

	got, err := rs.ProcessResultForTool(types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_fail",
		Content:   original,
		Metadata:  map[string]string{"maxResultSizeChars": "10"},
	}, "Bash")
	if err == nil {
		t.Fatal("expected persistence error")
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("persistence error did not preserve *os.PathError: %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "persist tool result") || !strings.Contains(err.Error(), parentFile) {
		t.Fatalf("expected useful filesystem error, got: %v", err)
	}
	if got.Content != original {
		t.Fatalf("failure must preserve original content")
	}
}

func TestResultStoreThresholdMetadataClampAndOptOut(t *testing.T) {
	rs := NewResultStore(t.TempDir())
	content := strings.Repeat("x", maxResultSizeChars+1)
	got, err := rs.ProcessResultForTool(types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_clamped_threshold",
		Content:   content,
		Metadata:  map[string]string{"maxResultSizeChars": "999999"},
	}, "Bash")
	if err != nil {
		t.Fatalf("clamped threshold ProcessResultForTool: %v", err)
	}
	if !strings.Contains(got.Content, persistedOutputTag) {
		t.Fatalf("declared threshold should be clamped to global default and persist")
	}

	got, err = rs.ProcessResultForTool(types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_opt_out",
		Content:   content,
		Metadata:  map[string]string{"maxResultSizeChars": "Infinity"},
	}, "Read")
	if err != nil {
		t.Fatalf("opt-out ProcessResultForTool: %v", err)
	}
	if got.Content != content {
		t.Fatalf("infinite threshold should hard opt out of persistence")
	}
}

func assertPersistedWrapper(t *testing.T, content, path string) {
	t.Helper()
	for _, want := range []string{
		persistedOutputTag,
		"Output too large (",
		"Full output saved to: " + path,
		"Preview (first 2KB):",
		persistedOutputClosingTag,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("wrapper missing %q in:\n%s", want, content)
		}
	}
}
