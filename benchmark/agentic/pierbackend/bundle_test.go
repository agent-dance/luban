package pierbackend

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestFrozenCodexBundleManifestIdentity(t *testing.T) {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate bundle test source")
	}
	manifestPath := filepath.Join(filepath.Dir(sourceFile), "..", "pier", filepath.Base(CodexBundleRelativePath))
	manifest, manifestSHA, err := loadCodexBundleManifest(manifestPath)
	if err != nil {
		t.Fatalf("load frozen Codex manifest: %v", err)
	}
	if manifest.Package != frozenCodexPackage || manifest.RegistrySnapshot != frozenCodexRegistrySnapshot {
		t.Fatal("frozen Codex provenance does not match backend constants")
	}
	if manifestSHA != CodexBundleManifestSHA256 {
		t.Fatalf("frozen Codex manifest SHA-256 = %q", manifestSHA)
	}
	if manifest.TreeSHA256 != CodexBundleTreeSHA256 || canonicalBundleTreeSHA256(manifest.Files) != CodexBundleTreeSHA256 {
		t.Fatalf("frozen Codex tree = %q", manifest.TreeSHA256)
	}
	if len(manifest.Files) != 6 {
		t.Fatalf("frozen Codex vendor file count = %d", len(manifest.Files))
	}
	foundBinary := false
	foundCodeModeHost := false
	for _, entry := range manifest.Files {
		switch entry.Path {
		case CodexBinaryRelativePath:
			foundBinary = entry.SHA256 == CodexBinarySHA256
		case "x86_64-unknown-linux-musl/bin/codex-code-mode-host":
			foundCodeModeHost = true
		}
	}
	if !foundBinary || !foundCodeModeHost {
		t.Fatal("frozen Codex manifest lacks the pinned runtime binaries")
	}
}

func TestValidateCodexBundleTreeRejectsAnythingOutsideManifest(t *testing.T) {
	root, manifest := syntheticBundle(t)
	if err := validateCodexBundleTree(root, manifest); err != nil {
		t.Fatalf("valid tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "unexpected"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateCodexBundleTree(root, manifest); err == nil {
		t.Fatal("unexpected file was accepted")
	}
}

func TestValidateCodexBundleTreeRejectsSymlinkSubstitution(t *testing.T) {
	root, manifest := syntheticBundle(t)
	binary := filepath.Join(root, filepath.FromSlash(manifest.Files[0].Path))
	target := filepath.Join(t.TempDir(), "replacement")
	raw, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, raw, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, binary); err != nil {
		t.Fatal(err)
	}
	if err := validateCodexBundleTree(root, manifest); err == nil {
		t.Fatal("symlink substitution was accepted")
	}
}

func TestCanonicalBundleTreeSHA256IsOrderIndependent(t *testing.T) {
	files := []codexBundleFile{
		{Path: "z", Mode: "0644", Size: 1, SHA256: string(make([]byte, 64))},
		{Path: "a", Mode: "0755", Size: 2, SHA256: string(make([]byte, 64))},
	}
	forward := canonicalBundleTreeSHA256(files)
	files[0], files[1] = files[1], files[0]
	if reverse := canonicalBundleTreeSHA256(files); reverse != forward {
		t.Fatalf("tree hash depends on order: %s != %s", forward, reverse)
	}
}

func syntheticBundle(t *testing.T) (string, codexBundleManifest) {
	t.Helper()
	root := t.TempDir()
	relative := "x86_64-unknown-linux-musl/bin/codex"
	binary := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("synthetic-codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	digest, err := harness.HashFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	manifest := codexBundleManifest{Files: []codexBundleFile{{
		Path: relative, Mode: "0755", Size: info.Size(), SHA256: digest,
	}}}
	manifest.TreeSHA256 = canonicalBundleTreeSHA256(manifest.Files)
	return root, manifest
}
