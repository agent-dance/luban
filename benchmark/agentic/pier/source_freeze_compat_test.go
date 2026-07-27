package pier_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

type freezeManifestValues struct {
	SchemaVersion        string            `json:"schema_version"`
	ManifestReplacements map[string]string `json:"manifest_replacements"`
}

func TestFreezeLubanSourceIsAcceptedByGoVerifier(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("atomic no-replace publication is supported on Darwin and Linux")
	}
	repository := filepath.Join(t.TempDir(), "source")
	mustMkdirAll(t, filepath.Join(repository, "cmd", "luban-code"))
	mustWrite(t, filepath.Join(repository, "go.mod"), []byte("module example.invalid/luban\n\ngo 1.23\n"))
	mustWrite(t, filepath.Join(repository, "cmd", "luban-code", "main.go"), []byte("package main\nfunc main() {}\n"))
	run(t, repository, "git", "init")
	run(t, repository, "git", "config", "user.email", "benchmark@example.invalid")
	run(t, repository, "git", "config", "user.name", "Benchmark")
	run(t, repository, "git", "add", ".")
	run(t, repository, "git", "commit", "-m", "base")
	base := strings.TrimSpace(run(t, repository, "git", "rev-parse", "HEAD^{commit}"))

	// Preserve a deliberately different real index while the formal temporary
	// index captures the final tracked and untracked worktree bytes.
	mustWrite(t, filepath.Join(repository, "cmd", "luban-code", "main.go"), []byte("package main\n// staged\nfunc main() {}\n"))
	run(t, repository, "git", "add", "cmd/luban-code/main.go")
	mustWrite(t, filepath.Join(repository, "cmd", "luban-code", "main.go"), []byte("package main\n// final\nfunc main() {}\n"))
	mustWrite(t, filepath.Join(repository, "untracked.go"), []byte("package fixture\n"))
	excluded := filepath.Join(repository, "benchmark-results", "private")
	mustMkdirAll(t, filepath.Dir(excluded))
	mustWrite(t, excluded, []byte("excluded sentinel"))
	if err := os.Chmod(excluded, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(excluded, 0o600) })
	indexBefore := run(t, repository, "git", "diff", "--cached", "--binary", "--full-index")

	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	goBinary, err := filepath.EvalSymlinks(goBinary)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "frozen")
	script := filepath.Join(mustGetwd(t), "freeze_luban_source.py")
	arguments := []string{
		script,
		"--worktree", repository,
		"--output-dir", output,
		"--base-commit", base,
		"--go-binary", goBinary,
		"--built-at", "2026-07-26T00:00:00Z",
	}
	t.Setenv("GOAMD64", "v4")
	t.Setenv("GOCACHEPROG", "ambient-cache-program-must-not-run")
	t.Setenv("GOEXPERIMENT", "ambient-experiment-must-not-apply")
	run(t, repository, "python3", arguments...)
	secondOutput := filepath.Join(t.TempDir(), "frozen")
	secondArguments := slices.Clone(arguments)
	secondArguments[4] = secondOutput
	run(t, repository, "python3", secondArguments...)
	for _, name := range []string{"luban", "source.patch", "source.tar", "source-exclusions.json", "build-receipt.json"} {
		if !bytes.Equal(mustRead(t, filepath.Join(output, name)), mustRead(t, filepath.Join(secondOutput, name))) {
			t.Fatalf("repeated freeze produced different %s bytes", name)
		}
	}
	indexAfter := run(t, repository, "git", "diff", "--cached", "--binary", "--full-index")
	if indexAfter != indexBefore {
		t.Fatal("source freeze modified the real Git index")
	}

	manifestRaw, err := os.ReadFile(filepath.Join(output, "manifest-values.json"))
	if err != nil {
		t.Fatal(err)
	}
	var values freezeManifestValues
	if err := json.Unmarshal(manifestRaw, &values); err != nil {
		t.Fatal(err)
	}
	if values.SchemaVersion != "agentic-bench/luban-source-freeze-v1" {
		t.Fatalf("schema = %q", values.SchemaVersion)
	}
	replacements := values.ManifestReplacements
	agent := harness.AgentSpec{
		ID:           "luban",
		Binary:       replacements["ABSOLUTE_LUBAN_BINARY"],
		BinarySHA256: replacements["LUBAN_BINARY_SHA256"],
		SourceSnapshot: &harness.AgentSourceSpec{
			Worktree:               replacements["ABSOLUTE_LUBAN_WORKTREE"],
			BaseCommit:             replacements["LUBAN_SOURCE_BASE_COMMIT"],
			TreeOID:                replacements["LUBAN_SOURCE_TREE_OID"],
			PatchSHA256:            replacements["LUBAN_SOURCE_PATCH_SHA256"],
			ArchiveSHA256:          replacements["LUBAN_SOURCE_ARCHIVE_SHA256"],
			PathPolicy:             harness.FormalSourcePathPolicy(),
			PathPolicySHA256:       "86059e44a68eb7d36f7d4953d53f90945ca4f2a94a83c98c560d670afbf980b5",
			ExclusionReceiptSHA256: "6ea05139a2686d237ec093866ad5b2223d967977fd9590b742fb79b0c0960020",
			BuildReceipt:           replacements["ABSOLUTE_LUBAN_BUILD_RECEIPT"],
			BuildReceiptSHA256:     replacements["LUBAN_BUILD_RECEIPT_SHA256"],
		},
	}
	archive := filepath.Join(t.TempDir(), "sources", "luban")
	if _, err := harness.SnapshotAgentAt(context.Background(), agent, time.Now(), archive); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"source.patch", "source.tar", "source-exclusions.json", "build-receipt.json"} {
		if !bytes.Equal(mustRead(t, filepath.Join(output, name)), mustRead(t, filepath.Join(archive, name))) {
			t.Fatalf("archived %s differs from generator output", name)
		}
	}
	binary := mustRead(t, replacements["ABSOLUTE_LUBAN_BINARY"])
	if len(binary) < 20 || !bytes.Equal(binary[:4], []byte{'\x7f', 'E', 'L', 'F'}) || binary[18] != 62 || binary[19] != 0 {
		t.Fatal("frozen binary is not Linux amd64 ELF")
	}

	manifestHashBefore := fileSHA256(t, filepath.Join(output, "manifest-values.json"))
	command := exec.Command("python3", arguments...)
	command.Dir = repository
	if err := command.Run(); err == nil {
		t.Fatal("existing output directory was overwritten")
	}
	if manifestHashAfter := fileSHA256(t, filepath.Join(output, "manifest-values.json")); manifestHashAfter != manifestHashBefore {
		t.Fatal("failed no-clobber invocation modified the frozen output")
	}

	mustWrite(t, filepath.Join(repository, "untracked.go"), []byte("package fixture\n// drift\n"))
	if _, err := harness.SnapshotAgent(context.Background(), agent, time.Now()); err == nil {
		t.Fatal("Go verifier accepted source drift after freeze")
	}
}

func TestFreezeLubanSourceRejectsExternalLocalModuleReplace(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("atomic no-replace publication is supported on Darwin and Linux")
	}
	root := t.TempDir()
	external := filepath.Join(root, "external")
	mustMkdirAll(t, external)
	mustWrite(t, filepath.Join(external, "go.mod"), []byte("module example.invalid/external\n\ngo 1.23\n"))
	mustWrite(t, filepath.Join(external, "external.go"), []byte("package external\n"))
	repository := filepath.Join(root, "source")
	mustMkdirAll(t, filepath.Join(repository, "cmd", "luban-code"))
	goMod := "module example.invalid/luban\n\ngo 1.23\n\nrequire example.invalid/external v0.0.0\nreplace example.invalid/external => " + filepath.ToSlash(external) + "\n"
	mustWrite(t, filepath.Join(repository, "go.mod"), []byte(goMod))
	mustWrite(t, filepath.Join(repository, "cmd", "luban-code", "main.go"), []byte("package main\nimport _ \"example.invalid/external\"\nfunc main() {}\n"))
	run(t, repository, "git", "init")
	run(t, repository, "git", "config", "user.email", "benchmark@example.invalid")
	run(t, repository, "git", "config", "user.name", "Benchmark")
	run(t, repository, "git", "add", ".")
	run(t, repository, "git", "commit", "-m", "base")
	base := strings.TrimSpace(run(t, repository, "git", "rev-parse", "HEAD^{commit}"))
	goBinary, err := filepath.EvalSymlinks(filepath.Join(runtime.GOROOT(), "bin", "go"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		"python3", filepath.Join(mustGetwd(t), "freeze_luban_source.py"),
		"--worktree", repository,
		"--output-dir", filepath.Join(t.TempDir(), "frozen"),
		"--base-commit", base,
		"--go-binary", goBinary,
		"--built-at", "2026-07-26T00:00:00Z",
	)
	output, err := command.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte(`"error_code":"local_module_replace_outside_frozen_source"`)) {
		t.Fatalf("external replace result: err=%v output=%s", err, output)
	}
}

func TestFreezeLubanSourceRejectsOutputInsideWorktree(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "source")
	mustMkdirAll(t, repository)
	run(t, repository, "git", "init")
	run(t, repository, "git", "config", "user.email", "benchmark@example.invalid")
	run(t, repository, "git", "config", "user.name", "Benchmark")
	mustWrite(t, filepath.Join(repository, "tracked"), []byte("fixture"))
	run(t, repository, "git", "add", ".")
	run(t, repository, "git", "commit", "-m", "base")
	base := strings.TrimSpace(run(t, repository, "git", "rev-parse", "HEAD^{commit}"))
	goBinary, err := filepath.EvalSymlinks(filepath.Join(runtime.GOROOT(), "bin", "go"))
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		"python3", filepath.Join(mustGetwd(t), "freeze_luban_source.py"),
		"--worktree", repository,
		"--output-dir", filepath.Join(repository, "frozen"),
		"--base-commit", base,
		"--go-binary", goBinary,
		"--built-at", "2026-07-26T00:00:00Z",
	)
	output, err := command.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte(`"error_code":"output_inside_worktree"`)) {
		t.Fatalf("inside-worktree result: err=%v output=%s", err, output)
	}
}

func run(t *testing.T, directory, name string, arguments ...string) string {
	t.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, arguments, err, output)
	}
	return string(output)
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, value []byte) {
	t.Helper()
	if err := os.WriteFile(path, value, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	sum := sha256.Sum256(mustRead(t, path))
	return hex.EncodeToString(sum[:])
}
