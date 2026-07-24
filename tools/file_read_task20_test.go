package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/types"
)

// Task 20 focused gate:
//
//   go test ./tools -run 'TestAlignment_FileRead_Task20' -count=1

// These tests cover parity requirements that the older Read baseline did not
// exercise. Keep them independent of process cwd and external PDF binaries
// except for the explicitly-skipped extraction lifecycle probe.

func TestAlignment_FileRead_Task20TypedContractAndMapping(t *testing.T) {
	SetActiveModelForCyberGating("claude-opus-4-6")
	t.Cleanup(func() { SetActiveModelForCyberGating("") })

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := &FileReadTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}
	result, err := tool.Execute(context.Background(), map[string]any{"file_path": path})
	if err != nil || result.IsError {
		t.Fatalf("read failed: result=%+v err=%v", result, err)
	}
	if result.Data == nil {
		t.Fatal("Read must carry its TS discriminated output in ToolResult.Data")
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("marshal typed output: %v", err)
	}
	var output struct {
		Type string `json:"type"`
		File struct {
			FilePath   string `json:"filePath"`
			Content    string `json:"content"`
			NumLines   int    `json:"numLines"`
			StartLine  int    `json:"startLine"`
			TotalLines int    `json:"totalLines"`
		} `json:"file"`
	}
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("decode typed output: %v", err)
	}
	if output.Type != "text" || output.File.FilePath != path || output.File.Content != "alpha\nbeta" || output.File.NumLines != 2 || output.File.StartLine != 1 || output.File.TotalLines != 2 {
		t.Fatalf("unexpected typed output: %+v", output)
	}
	if _, ok := any(tool).(types.ToolResultMapper); !ok {
		t.Fatal("Read must implement ToolResultMapper")
	}
	mapped := types.MapToolResult(tool, result, "toolu_read")
	if mapped.ToolUseID != "toolu_read" || !strings.Contains(mapped.TextContent(), "1\talpha") {
		t.Fatalf("unexpected mapped output: %+v", mapped)
	}

	definition := types.ToDefinition(tool)
	if !definition.Strict || !definition.Metadata.ReadOnly || !definition.Metadata.ConcurrencySafe || definition.Metadata.MaxResultSizeChars != types.UnlimitedToolResultSize {
		t.Fatalf("unexpected Read definition metadata: %+v", definition)
	}
	if got := types.ToolAutoClassifierInput(tool, map[string]any{"file_path": "  ./sample.txt  "}); got != "./sample.txt" {
		t.Fatalf("auto-classifier input = %q", got)
	}
	classification := types.ToolSearchRead(tool, nil)
	if classification.IsSearch || !classification.IsRead {
		t.Fatalf("search/read classification = %+v", classification)
	}
}

func TestAlignment_FileRead_Task20CyberUnsetAndRichScope(t *testing.T) {
	SetActiveModelForCyberGating("")
	t.Cleanup(func() { SetActiveModelForCyberGating("") })

	dir := t.TempDir()
	path := filepath.Join(dir, "benign.txt")
	if err := os.WriteFile(path, []byte("ordinary project text"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := &FileReadTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}
	result, err := tool.Execute(context.Background(), map[string]any{"file_path": path})
	if err != nil || result.IsError {
		t.Fatalf("read failed: result=%+v err=%v", result, err)
	}
	if strings.Contains(result.TextContent(), CyberRiskMitigationReminder) || len(result.NewMessages) != 1 {
		t.Fatalf("model control must be separate from visible file output: %+v", result)
	}
	if reminder := result.NewMessages[0]; reminder.InternalKind != types.InternalMessageKindFileReadSecurity || !reminder.IsMeta || !strings.Contains(reminder.GetText(), "consider whether it would be considered malware") {
		t.Fatalf("missing typed model-only security reminder: %+v", reminder)
	}
}

type task20SkillActivator struct {
	mu        sync.Mutex
	dirs      []string
	activated []string
}

func (m *task20SkillActivator) AddDirectories(dirs []string) {
	m.mu.Lock()
	m.dirs = append(m.dirs, dirs...)
	m.mu.Unlock()
}

func (m *task20SkillActivator) ActivateConditionalForPath(path string) {
	m.mu.Lock()
	m.activated = append(m.activated, path)
	m.mu.Unlock()
}

func (m *task20SkillActivator) snapshot() ([]string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.dirs...), append([]string(nil), m.activated...)
}

func TestAlignment_FileRead_Task20SkillTimingAndSimpleMode(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	skillDir := filepath.Join(nested, ".claude", "skills")
	if err := os.MkdirAll(filepath.Join(skillDir, "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "review", "SKILL.md"), []byte("---\nname: review\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(nested, "missing.go")
	manager := &task20SkillActivator{}
	tool := &FileReadTool{AllowedDirs: []string{root}, ReadState: NewReadFileState(), SkillManager: manager}
	result, err := tool.Execute(context.Background(), map[string]any{"file_path": missing})
	if err != nil || !result.IsError {
		t.Fatalf("missing read = %+v, %v", result, err)
	}
	dirs, activated := manager.snapshot()
	if len(dirs) == 0 || len(activated) == 0 {
		t.Fatalf("skill discovery must run after dedup miss but before file I/O: dirs=%v activated=%v", dirs, activated)
	}

	t.Setenv("CLAUDE_CODE_SIMPLE", "1")
	path := filepath.Join(nested, "present.go")
	if err := os.WriteFile(path, []byte("package present"), 0o600); err != nil {
		t.Fatal(err)
	}
	simpleManager := &task20SkillActivator{}
	simpleTool := &FileReadTool{AllowedDirs: []string{root}, ReadState: NewReadFileState(), SkillManager: simpleManager}
	_, _ = simpleTool.Execute(context.Background(), map[string]any{"file_path": path})
	dirs, activated = simpleManager.snapshot()
	if len(dirs) != 0 || len(activated) != 0 {
		t.Fatalf("CLAUDE_CODE_SIMPLE must suppress Read skill activation: dirs=%v activated=%v", dirs, activated)
	}
}

func TestAlignment_FileRead_Task20AnalyticsAndTextListener(t *testing.T) {
	SetActiveModelForCyberGating("claude-opus-4-6")
	t.Cleanup(func() { SetActiveModelForCyberGating("") })

	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.txt")
	content := "one\ntwo\nthree"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	tool := &FileReadTool{
		AllowedDirs: []string{dir},
		ReadState:   NewReadFileState(),
		AnalyticsHook: func(event string, payload map[string]any) {
			if event == "tengu_session_file_read" {
				events = append(events, payload)
			}
		},
	}
	registrar, ok := any(tool).(interface {
		RegisterTextListener(func(string, string)) func()
	})
	if !ok {
		t.Fatal("Read must expose the TS (resolvedPath, textualContent) listener")
	}
	var listenerPath, listenerContent string
	unsubscribe := registrar.RegisterTextListener(func(path, content string) {
		listenerPath, listenerContent = path, content
	})
	defer unsubscribe()

	result, err := tool.Execute(context.Background(), map[string]any{"file_path": path, "offset": 2, "limit": 1})
	if err != nil || result.IsError {
		t.Fatalf("read failed: result=%+v err=%v", result, err)
	}
	if listenerPath != path || listenerContent != "two" {
		t.Fatalf("listener = (%q, %q)", listenerPath, listenerContent)
	}
	if len(events) != 1 {
		t.Fatalf("session analytics count = %d, events=%v", len(events), events)
	}
	event := events[0]
	want := map[string]any{
		"totalLines":            3,
		"readLines":             1,
		"totalBytes":            int64(len(content)),
		"readBytes":             int64(len("two")),
		"offset":                2,
		"limit":                 1,
		"ext":                   "txt",
		"is_session_memory":     false,
		"is_session_transcript": false,
	}
	for key, expected := range want {
		if got := event[key]; got != expected {
			t.Fatalf("analytics[%s] = %#v, want %#v (event=%v)", key, got, expected, event)
		}
	}
}

func TestAlignment_FileRead_Task20PermissionNoIOStages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	tool := &FileReadTool{AllowedDirs: []string{dir}}

	denied, err := tool.CheckPermissions(context.Background(), map[string]any{"file_path": path}, types.ToolPermissionRequest{
		Runtime: types.ToolRuntimeContext{
			ProjectRoot: dir,
			AllowedDirs: []string{dir},
			DeniedRules: []types.PermissionRuleValue{{ToolName: "Read", RuleContent: path}},
		},
	})
	if err != nil || denied.Behavior != types.PermissionBehaviorDeny {
		t.Fatalf("explicit denied Read path = %+v, %v", denied, err)
	}

	unc := `//server/share/credential.txt`
	decision, err := tool.CheckPermissions(context.Background(), map[string]any{"file_path": unc}, types.ToolPermissionRequest{
		Runtime: types.ToolRuntimeContext{ProjectRoot: dir, AllowedDirs: []string{dir}},
	})
	if err != nil || decision.Behavior != types.PermissionBehaviorAsk || decision.BlockedPath != unc {
		t.Fatalf("UNC decision = %+v, %v", decision, err)
	}
}

func TestAlignment_FileRead_Task20ObservableNormalizer(t *testing.T) {
	root := t.TempDir()
	tool := &FileReadTool{Runtime: NewRuntimeScope(root, true)}
	original := map[string]any{"file_path": "  nested/file.go  "}
	normalizer, ok := any(tool).(interface {
		NormalizeToolInput(context.Context, map[string]any) (map[string]any, error)
	})
	if !ok {
		t.Fatal("Read must normalize observable input before main-loop hooks")
	}
	updated, err := normalizer.NormalizeToolInput(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "nested", "file.go")
	if got := updated["file_path"]; got != want {
		t.Fatalf("observable file_path = %#v, want %q", got, want)
	}
	if original["file_path"] != "  nested/file.go  " {
		t.Fatalf("normalization mutated caller input: %#v", original)
	}
}

func TestAlignment_FileRead_Task20EnvTokenLimit(t *testing.T) {
	SetActiveModelForCyberGating("claude-opus-4-6")
	t.Cleanup(func() { SetActiveModelForCyberGating("") })
	t.Setenv("CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS", "1")

	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.txt")
	if err := os.WriteFile(path, []byte("alpha beta gamma delta"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (&FileReadTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}).Execute(
		context.Background(), map[string]any{"file_path": path},
	)
	if err != nil || !result.IsError || !strings.Contains(result.Content, "maximum allowed tokens (1)") {
		t.Fatalf("env token override was not enforced: result=%+v err=%v", result, err)
	}
}

func TestAlignment_FileRead_Task20PDFPartsPersist(t *testing.T) {
	if !hasPDFToPPM() {
		t.Skip("pdftoppm unavailable; persistent extraction lifecycle requires poppler")
	}
	SetActiveModelForCyberGating("claude-opus-4-6")
	t.Cleanup(func() { SetActiveModelForCyberGating("") })

	dir := t.TempDir()
	path := filepath.Join(dir, "pages.pdf")
	writeTestPDF(t, path, 2)
	result, err := (&FileReadTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}).Execute(
		context.Background(), map[string]any{"file_path": path, "pages": "1"},
	)
	if err != nil || result.IsError {
		t.Fatalf("PDF page read failed: result=%+v err=%v", result, err)
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("marshal parts output: %v", err)
	}
	var output struct {
		Type string `json:"type"`
		File struct {
			FilePath     string `json:"filePath"`
			OriginalSize int64  `json:"originalSize"`
			Count        int    `json:"count"`
			OutputDir    string `json:"outputDir"`
		} `json:"file"`
	}
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("decode parts output: %v", err)
	}
	if output.Type != "parts" || output.File.FilePath != path || output.File.OriginalSize <= 0 || output.File.Count != 1 || output.File.OutputDir == "" {
		t.Fatalf("unexpected parts output: %+v", output)
	}
	entries, err := os.ReadDir(output.File.OutputDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("extracted pages did not survive return: dir=%q entries=%v err=%v", output.File.OutputDir, entries, err)
	}
}

func resetTask20PDFToPPMCache() {
	pdftoppmAvailableOnce = sync.Once{}
	pdftoppmAvailableCache = false
}

func TestAlignment_FileRead_Task20PDFStructuredErrorPaths(t *testing.T) {
	SetActiveModelForCyberGating("claude-opus-4-6")
	t.Cleanup(func() { SetActiveModelForCyberGating("") })

	t.Run("empty", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.pdf")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := (&FileReadTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}).Execute(
			context.Background(), map[string]any{"file_path": path},
		)
		if err != nil || !result.IsError || !strings.Contains(result.Content, "PDF file is empty") {
			t.Fatalf("empty PDF result=%+v err=%v", result, err)
		}
		if result.Metadata["pdfErrorReason"] != string(PDFErrorEmpty) || result.Data != nil {
			t.Fatalf("empty PDF typed error metadata=%v data=%#v", result.Metadata, result.Data)
		}
		mapped := types.MapToolResult(&FileReadTool{}, result, "toolu_pdf_empty")
		if !mapped.IsError || !strings.Contains(mapped.Content, "PDF file is empty") || strings.Contains(mapped.Content, "invalid typed result") {
			t.Fatalf("empty PDF error mapping=%+v", mapped)
		}
	})

	t.Run("corrupt", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "corrupt.pdf")
		if err := os.WriteFile(path, []byte("not a pdf"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, _, typedErr := readPDFFile(path)
		var pdfErr *PDFError
		if !errors.As(typedErr, &pdfErr) || pdfErr.Reason != PDFErrorCorrupted {
			t.Fatalf("readPDFFile error=%T %v, want corrupted PDFError", typedErr, typedErr)
		}
		result, err := (&FileReadTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}).Execute(
			context.Background(), map[string]any{"file_path": path},
		)
		if err != nil || !result.IsError || result.Metadata["pdfErrorReason"] != string(PDFErrorCorrupted) {
			t.Fatalf("corrupt PDF result=%+v err=%v", result, err)
		}
	})

	t.Run("unsupported_model", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "unsupported.pdf")
		writeTestPDF(t, path, 1)
		tool := &FileReadTool{
			AllowedDirs: []string{dir}, ReadState: NewReadFileState(),
			ModelProvider:          func() string { return "claude-3-haiku-20240307" },
			ToolResultsDirProvider: func() string { return filepath.Join(dir, "tool-results") },
		}
		result, err := tool.Execute(context.Background(), map[string]any{"file_path": path})
		if err != nil || !result.IsError || !strings.Contains(result.Content, "Reading full PDFs is not supported with this model") {
			t.Fatalf("unsupported PDF result=%+v err=%v", result, err)
		}
		if result.Data != nil || len(result.ContentBlocks) != 0 || len(result.NewMessages) != 0 || strings.Contains(result.TextContent(), CyberRiskMitigationReminder) {
			t.Fatalf("unsupported PDF leaked typed/media/reminder data: %+v", result)
		}
	})

	t.Run("renderer_unavailable", func(t *testing.T) {
		resetTask20PDFToPPMCache()
		t.Cleanup(resetTask20PDFToPPMCache)
		t.Setenv("PATH", t.TempDir())
		dir := t.TempDir()
		path := filepath.Join(dir, "pages.pdf")
		writeTestPDF(t, path, 1)
		result, err := (&FileReadTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}).Execute(
			context.Background(), map[string]any{"file_path": path, "pages": "1"},
		)
		if err != nil || !result.IsError || result.Metadata["pdfErrorReason"] != string(PDFErrorUnavailable) || !strings.Contains(result.Content, "pdftoppm is not installed") {
			t.Fatalf("unavailable renderer result=%+v err=%v", result, err)
		}
	})

	t.Run("password_protected", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("fake pdftoppm fixture uses a POSIX script")
		}
		resetTask20PDFToPPMCache()
		t.Cleanup(resetTask20PDFToPPMCache)
		bin := t.TempDir()
		fake := filepath.Join(bin, "pdftoppm")
		if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'Incorrect password' >&2\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
		dir := t.TempDir()
		path := filepath.Join(dir, "protected.pdf")
		writeTestPDF(t, path, 1)
		result, err := (&FileReadTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}).Execute(
			context.Background(), map[string]any{"file_path": path, "pages": "1"},
		)
		if err != nil || !result.IsError || result.Metadata["pdfErrorReason"] != string(PDFErrorPasswordProtected) || !strings.Contains(result.Content, "password-protected") {
			t.Fatalf("password PDF result=%+v err=%v", result, err)
		}
	})
}
