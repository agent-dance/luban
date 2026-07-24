package tools

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileWriteSecurity_TeamMemoryDetection(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/repo/.claude/memory/team/policy.md", true},
		{"/repo/.deepseek-code/memory/team/notes.md", true},
		{"/repo/.claude/memory/notes.md", false},
		{"/repo/team/policy.md", false},
		{"", false},
	}
	for _, tc := range tests {
		got := isTeamMemoryFilePath(tc.path)
		if got != tc.want {
			t.Errorf("isTeamMemoryFilePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestFileWriteSecurity_TeamMemorySecretsHit(t *testing.T) {
	cases := map[string]string{
		"akia":       "AKIAIOSFODNN7EXAMPLE",
		"anthropic":  "key=sk-ant-api03-abcdefghij1234567890",
		"openai":     "OPENAI_KEY=sk-1234567890abcdefghijklmnop",
		"github_pat": "gh_token: ghp_abcdefghij1234567890ABCDEFGHIJKL",
		"github_fg":  "github_pat_11ABCDEFG0aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"private":    "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIB...",
		"jwt":        "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
		"generic":    `password: "mySuperSecretPassword123"`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if hit := scanForTeamMemorySecrets(content); hit == "" {
				t.Errorf("expected secret hit for %q content", name)
			}
		})
	}
}

func TestFileWriteSecurity_TeamMemorySecretsClean(t *testing.T) {
	clean := []string{
		"# Team policy: always test before commit\nhttps://example.com",
		"function foo() { return 42; }",
		"plain text without anything sensitive",
	}
	for _, c := range clean {
		if hit := scanForTeamMemorySecrets(c); hit != "" {
			t.Errorf("false positive on clean content %q -> %q", c, hit)
		}
	}
}

func TestFileWrite_TeamMemorySecretGuard_RejectsSecret(t *testing.T) {
	dir := t.TempDir()
	teamDir := filepath.Join(dir, ".claude", "memory", "team")
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(teamDir, "policy.md")

	tool := &FileWriteTool{
		AllowedDirs: []string{dir},
		ReadState:   NewReadFileState(),
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": target,
		"content":   "AWS_KEY=AKIAIOSFODNN7EXAMPLE\n",
	})
	if !res.IsError {
		t.Fatalf("expected team memory secret guard to reject; got success: %v", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "team memory") {
		t.Fatalf("expected error mentioning team memory; got %q", res.Content)
	}
	// Verify file was not written.
	if _, err := os.Stat(target); err == nil {
		t.Fatalf("file should not exist; secret guard should have rejected")
	}
}

func TestFileWrite_TeamMemoryNonSecretAllowed(t *testing.T) {
	dir := t.TempDir()
	teamDir := filepath.Join(dir, ".claude", "memory", "team")
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(teamDir, "notes.md")
	tool := &FileWriteTool{
		AllowedDirs: []string{dir},
		ReadState:   NewReadFileState(),
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": target,
		"content":   "# Team notes\nThis is a clean policy doc.\n",
	})
	if res.IsError {
		t.Fatalf("expected success; got error: %v", res.Content)
	}
}

func TestFileWrite_EncodingPreservation_UTF16LE(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "win.txt")
	// Create a UTF-16 LE BOM file with content "Hello"
	utf16Data := append([]byte{0xFF, 0xFE}, 0x48, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x6C, 0x00, 0x6F, 0x00)
	if err := os.WriteFile(target, utf16Data, 0o644); err != nil {
		t.Fatal(err)
	}

	state := NewReadFileState()
	rt := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	readRes, _ := rt.Execute(context.Background(), map[string]any{"file_path": target})
	if readRes.IsError {
		t.Fatalf("read failed: %v", readRes.Content)
	}
	if !strings.Contains(readRes.Content, "Hello") {
		t.Fatalf("expected decoded 'Hello' in read content, got %q", readRes.Content)
	}

	// Now overwrite with new content
	wt := &FileWriteTool{AllowedDirs: []string{dir}, ReadState: state}
	writeRes, _ := wt.Execute(context.Background(), map[string]any{
		"file_path": target,
		"content":   "World",
	})
	if writeRes.IsError {
		t.Fatalf("write failed: %v", writeRes.Content)
	}

	// Reread the raw bytes and verify UTF-16 LE BOM was preserved
	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(written, []byte{0xFF, 0xFE}) {
		t.Fatalf("expected UTF-16 LE BOM preserved; got bytes %v", written)
	}
	expected := append([]byte{0xFF, 0xFE}, 0x57, 0x00, 0x6F, 0x00, 0x72, 0x00, 0x6C, 0x00, 0x64, 0x00)
	if !bytes.Equal(written, expected) {
		t.Fatalf("expected %v, got %v", expected, written)
	}
}

func TestFileWrite_EncodingPreservation_UTF8Default(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(target, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	rt := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	if r, _ := rt.Execute(context.Background(), map[string]any{"file_path": target}); r.IsError {
		t.Fatalf("read failed: %v", r.Content)
	}
	wt := &FileWriteTool{AllowedDirs: []string{dir}, ReadState: state}
	if r, _ := wt.Execute(context.Background(), map[string]any{
		"file_path": target,
		"content":   "world\n",
	}); r.IsError {
		t.Fatalf("write failed: %v", r.Content)
	}
	written, _ := os.ReadFile(target)
	if string(written) != "world\n" {
		t.Fatalf("expected 'world\\n', got %q", string(written))
	}
}
