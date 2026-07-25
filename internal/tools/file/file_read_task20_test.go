package file

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

	metadata := tool.ToolMetadata(nil)
	if !tool.Schema().RejectsUnknownFields() || !metadata.ReadOnly || !metadata.ConcurrencySafe || metadata.MaxResultSizeChars != types.UnlimitedToolResultSize {
		t.Fatalf("unexpected Read metadata: %+v", metadata)
	}
}

func TestAlignment_FileRead_Task20TextReadHasNoInternalMessages(t *testing.T) {
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
	if len(result.NewMessages) != 0 {
		t.Fatalf("plain text Read must not inject internal messages: %+v", result.NewMessages)
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
	tool := &FileReadTool{Runtime: testRuntimeProvider{snapshot: types.ToolRuntimeContext{ProjectRoot: root}}}
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
	t.Setenv("LUBAN_CODE_FILE_READ_MAX_OUTPUT_TOKENS", "1")

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
