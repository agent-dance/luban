package pierbackend

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestRequirePristineGitSourceRootRejectsTrackedUntrackedAndIgnoredEntries(t *testing.T) {
	repository := newSourceProvenanceRepository(t)
	ctx := context.Background()
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err != nil {
		t.Fatalf("clean source root: %v", err)
	}

	tracked := filepath.Join(repository, "src", "module.py")
	writeProvenanceFixture(t, tracked, "value = 2\n")
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err == nil {
		t.Fatal("tracked source modification was accepted")
	}
	writeProvenanceFixture(t, tracked, "value = 1\n")
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err != nil {
		t.Fatalf("restored tracked source root: %v", err)
	}
	if err := os.Remove(tracked); err != nil {
		t.Fatal(err)
	}
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err == nil {
		t.Fatal("deleted tracked source file was accepted")
	}
	writeProvenanceFixture(t, tracked, "value = 1\n")
	writeProvenanceFixture(t, tracked, "value = 3\n")
	runProvenanceGit(t, repository, "add", "src/module.py")
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err == nil {
		t.Fatal("staged source modification was accepted")
	}
	writeProvenanceFixture(t, tracked, "value = 1\n")
	runProvenanceGit(t, repository, "add", "src/module.py")
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err != nil {
		t.Fatalf("restored staged source root: %v", err)
	}

	untracked := filepath.Join(repository, "src", "untracked.txt")
	writeProvenanceFixture(t, untracked, "untracked\n")
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err == nil {
		t.Fatal("untracked source file was accepted")
	}
	if err := os.Remove(untracked); err != nil {
		t.Fatal(err)
	}

	ignoredRoot := filepath.Join(repository, "src", "__pycache__")
	writeProvenanceFixture(t, filepath.Join(ignoredRoot, "module.cpython-313.pyc"), "bytecode")
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err == nil {
		t.Fatal("ignored source runtime output was accepted")
	}
	if err := os.RemoveAll(ignoredRoot); err != nil {
		t.Fatal(err)
	}

	writeProvenanceFixture(t, filepath.Join(repository, ".runtime", "outside.pyc"), "outside")
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err != nil {
		t.Fatalf("ignored runtime output outside source root was rejected: %v", err)
	}
}

func TestNetworkAttestationPreservesPristineEvaluatorTreeAcrossRepeatedPreflight(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX Python launcher")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is unavailable")
	}
	repository := newSourceProvenanceRepository(t)
	configModule := filepath.Join(repository, "src", "pier", "models", "task", "config.py")
	writeProvenanceFixture(t, configModule, "class TaskConfig:\n    pass\n")
	for _, path := range []string{
		"src/pier/__init__.py",
		"src/pier/models/__init__.py",
		"src/pier/models/task/__init__.py",
	} {
		writeProvenanceFixture(t, filepath.Join(repository, filepath.FromSlash(path)), "")
	}
	runProvenanceGit(t, repository, "add", "src/pier")
	runProvenanceGit(t, repository, "commit", "-m", "add parser fixture")

	binRoot := t.TempDir()
	launcher := filepath.Join(binRoot, "python")
	launcherSource := "#!/bin/sh\n" +
		"test \"$PYTHONDONTWRITEBYTECODE\" = \"1\" || exit 91\n" +
		"test \"$1\" = \"-B\" || exit 92\n" +
		"exec " + strconv.Quote(python) + " \"$@\"\n"
	writeProvenanceFixture(t, launcher, launcherSource)
	if err := os.Chmod(launcher, 0o755); err != nil {
		t.Fatal(err)
	}
	config := Config{
		PierBinary:              filepath.Join(binRoot, "pier"),
		EvaluatorRepositoryRoot: repository,
	}

	ctx := context.Background()
	if err := requirePristineGitSourceRoot(ctx, repository, "src"); err != nil {
		t.Fatalf("initial source root: %v", err)
	}
	before, err := harness.HashTree(filepath.Join(repository, "src"))
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		attestation, attestErr := attestPierNetworkPolicy(ctx, config, harness.Manifest{}, nil)
		if attestErr != nil {
			t.Fatalf("attestation %d: %v", attempt+1, attestErr)
		}
		if !attestation.AgentNetworkDeny || !attestation.VerifierNetworkDeny {
			t.Fatalf("attestation %d did not preserve network denial: %#v", attempt+1, attestation)
		}
	}
	if err := requireSourceTreeUnchanged(ctx, repository, "src", before); err != nil {
		t.Fatalf("repeated preflight mutated evaluator source: %v", err)
	}
	after, err := harness.HashTree(filepath.Join(repository, "src"))
	if err != nil {
		t.Fatal(err)
	}
	if before.SHA256 != after.SHA256 || before.RawSHA256 != after.RawSHA256 || len(before.Files) != len(after.Files) {
		t.Fatalf("source tree changed across repeated preflight: before=%#v after=%#v", before, after)
	}
	for _, file := range after.Files {
		if strings.Contains(file.Path, "__pycache__/") || strings.HasSuffix(file.Path, ".pyc") {
			t.Fatalf("preflight created bytecode cache: %s", file.Path)
		}
	}
}

func TestSanitizedProcessEnvironmentForcesNoPythonBytecode(t *testing.T) {
	environment := sanitizedProcessEnvironment([]string{
		"PYTHONDONTWRITEBYTECODE=0",
		"OPENAI_API_KEY=must-not-leak",
	}, "OPENAI_API_KEY")
	if got, ok := provenanceEnvironmentValue(environment, "PYTHONDONTWRITEBYTECODE"); !ok || got != "1" {
		t.Fatalf("PYTHONDONTWRITEBYTECODE = %q, %v; want 1, true", got, ok)
	}
	if _, ok := provenanceEnvironmentValue(environment, "OPENAI_API_KEY"); ok {
		t.Fatal("provider credential leaked into sanitized process environment")
	}
}

func TestRunPierFailsClosedWhenSuccessfulProcessMutatesEvaluatorSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX launcher")
	}
	repository := newSourceProvenanceRepository(t)
	tree, err := harness.HashTree(filepath.Join(repository, "src"))
	if err != nil {
		t.Fatal(err)
	}
	binRoot := t.TempDir()
	pierBinary := filepath.Join(binRoot, "pier")
	cachePath := filepath.Join(repository, "src", "__pycache__", "injected.cpython-313.pyc")
	launcher := "#!/bin/sh\n" +
		"mkdir -p " + strconv.Quote(filepath.Dir(cachePath)) + "\n" +
		"printf bytecode > " + strconv.Quote(cachePath) + "\n" +
		"exit 0\n"
	writeProvenanceFixture(t, pierBinary, launcher)
	if err := os.Chmod(pierBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	backend := &Backend{
		config: Config{
			PierBinary:              pierBinary,
			EvaluatorRepositoryRoot: repository,
			PythonModuleRoot:        t.TempDir(),
			RegistryGatePath:        filepath.Join(t.TempDir(), "registry-gate"),
		},
		manifest:      harness.Manifest{Evaluator: harness.EvaluatorSpec{SourcePin: harness.SourcePin{Root: "src"}}},
		evaluatorTree: tree,
		ready:         true,
	}
	_, _, runErr := backend.runPier(context.Background(), nil, nil, t.TempDir())
	if runErr == nil || !isRuntimeSourceIntegrityError(runErr) {
		t.Fatalf("successful source-mutating Pier process was not a fatal integrity error: %v", runErr)
	}
}

func newSourceProvenanceRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	runProvenanceGit(t, repository, "init")
	runProvenanceGit(t, repository, "config", "user.email", "benchmark@example.invalid")
	runProvenanceGit(t, repository, "config", "user.name", "Benchmark")
	writeProvenanceFixture(t, filepath.Join(repository, ".gitignore"), "*.pyc\n__pycache__/\n.runtime/\n")
	writeProvenanceFixture(t, filepath.Join(repository, "src", "module.py"), "value = 1\n")
	runProvenanceGit(t, repository, "add", ".gitignore", "src/module.py")
	runProvenanceGit(t, repository, "commit", "-m", "source fixture")
	return repository
}

func runProvenanceGit(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repository}, arguments...)...)
	command.Env = sanitizedProcessEnvironment(nil, "")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeProvenanceFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func provenanceEnvironmentValue(environment []string, name string) (string, bool) {
	prefix := name + "="
	for _, item := range environment {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix), true
		}
	}
	return "", false
}
