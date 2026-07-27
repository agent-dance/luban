package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHashTreeHashesSymlinkTextWithoutDereferencing(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside-secret")
	if err := os.WriteFile(outside, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	first, err := HashTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("changed external bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := HashTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 || len(first.Files) != 1 || first.Files[0].Kind != "symlink" {
		t.Fatalf("symlink target bytes affected source hash: %#v %#v", first, second)
	}
	if _, err := HashTreeExcluding(root, nil); err == nil {
		t.Fatal("artifact inventory accepted a symlink")
	}
}

func TestHashTreeCanonicalIdentityIgnoresUmaskBitsButPreservesExecutability(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "script.sh")
	writeTestFile(t, path, []byte("#!/bin/sh\nexit 0\n"))
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := HashTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatal(err)
	}
	shared, err := HashTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if base.SHA256 != shared.SHA256 || base.RawSHA256 == shared.RawSHA256 || shared.Files[0].Mode != "0644" || shared.Files[0].RawMode != "0664" {
		t.Fatalf("non-semantic permission drift changed canonical identity: %#v %#v", base, shared)
	}
	if err := os.Chmod(path, 0o775); err != nil {
		t.Fatal(err)
	}
	executableShared, err := HashTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if executableShared.SHA256 == shared.SHA256 || executableShared.Files[0].Mode != "0755" || executableShared.Files[0].RawMode != "0775" {
		t.Fatalf("executable semantic bit was not preserved: %#v", executableShared)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	executable, err := HashTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if executable.SHA256 != executableShared.SHA256 || executable.RawSHA256 == executableShared.RawSHA256 {
		t.Fatalf("executable umask drift changed canonical identity: %#v %#v", executableShared, executable)
	}
}

func TestWriteBytesAtomicCreatesDurableNestedPathAndLeavesNoTemporaryFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "one", "two", "state.json")
	if err := WriteBytesAtomic(path, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteBytesAtomic(path, []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "second\n" {
		t.Fatalf("atomic artifact = %q", raw)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".agentic-bench-") {
			t.Fatalf("atomic writer left temporary file %s", entry.Name())
		}
	}
}

func TestSnapshotAgentArchivesDirtySourceAndBindsBuildReceiptWithoutTouchingIndex(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repository, "init")
	runTestGit(t, repository, "config", "user.email", "benchmark@example.invalid")
	runTestGit(t, repository, "config", "user.name", "Benchmark")
	writeTestFile(t, filepath.Join(repository, "tracked.go"), []byte("package fixture\n"))
	runTestGit(t, repository, "add", ".")
	runTestGit(t, repository, "commit", "-m", "base")
	base := strings.TrimSpace(runTestGit(t, repository, "rev-parse", "HEAD"))
	writeTestFile(t, filepath.Join(repository, "tracked.go"), []byte("package fixture\n// dirty source\n"))
	writeTestFile(t, filepath.Join(repository, "untracked.go"), []byte("package fixture\n"))
	excludedDir := filepath.Join(repository, "benchmark-results")
	if err := os.MkdirAll(excludedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const excludedSecret = "DO-NOT-READ-EXCLUDED-SOURCE-SENTINEL"
	excludedPath := filepath.Join(excludedDir, "private-events.jsonl")
	writeTestFile(t, excludedPath, []byte(excludedSecret))
	if err := os.Chmod(excludedPath, 0); err != nil {
		t.Fatal(err)
	}

	temporaryRoot := t.TempDir()
	objects := filepath.Join(temporaryRoot, "objects")
	if err := os.MkdirAll(objects, 0o700); err != nil {
		t.Fatal(err)
	}
	baseObjects, err := runGit(context.Background(), repository, cleanGitEnvironment(), "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		t.Fatal(err)
	}
	environment := append(cleanGitEnvironment(),
		"GIT_INDEX_FILE="+filepath.Join(temporaryRoot, "index"),
		"GIT_OBJECT_DIRECTORY="+objects,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES="+strings.TrimSpace(string(baseObjects)),
	)
	if _, err := runGit(context.Background(), repository, environment, "read-tree", base); err != nil {
		t.Fatal(err)
	}
	policy := FormalSourcePathPolicy()
	policySHA, err := HashCanonical(policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(context.Background(), repository, environment, sourceSnapshotAddArgs(policy)...); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range policy.ExcludedPrefixes {
		if _, err := runGit(context.Background(), repository, environment, "rm", "-r", "-f", "--cached", "--ignore-unmatch", "--", strings.TrimSuffix(prefix, "/")); err != nil {
			t.Fatal(err)
		}
	}
	_, _, exclusionSHA, err := captureSourceExclusionReceipt(policy, policySHA)
	if err != nil {
		t.Fatal(err)
	}
	treeRaw, err := runGit(context.Background(), repository, environment, "write-tree")
	if err != nil {
		t.Fatal(err)
	}
	treeOID := strings.TrimSpace(string(treeRaw))
	patch, err := runGit(context.Background(), repository, environment, "diff", "--cached", "--binary", "--full-index", "--no-ext-diff", base, "--")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := runGit(context.Background(), repository, environment, "archive", "--format=tar", "--mtime=1970-01-01T00:00:00Z", treeOID)
	if err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(t.TempDir(), "luban")
	writeTestFile(t, binaryPath, []byte("frozen binary"))
	binarySHA, err := HashFile(binaryPath)
	if err != nil {
		t.Fatal(err)
	}
	receipt := AgentBuildReceipt{
		SchemaVersion: "agentic-bench/agent-build-receipt-v2", AgentID: "luban", BaseCommit: base, TreeOID: treeOID,
		PatchSHA256: hashBytes(patch), ArchiveSHA256: hashBytes(archive), PathPolicy: policy, PathPolicySHA256: policySHA, ExclusionReceiptSHA256: exclusionSHA, BinarySHA256: binarySHA,
		BuildArgv: []string{"go", "build", "./cmd/luban"}, Toolchain: "go fixture", BuiltAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
	}
	receiptRaw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(t.TempDir(), "build-receipt.json")
	writeTestFile(t, receiptPath, receiptRaw)
	agent := AgentSpec{ID: "luban", Binary: binaryPath, BinarySHA256: binarySHA, SourceSnapshot: &AgentSourceSpec{
		Worktree: repository, BaseCommit: base, TreeOID: treeOID, PatchSHA256: hashBytes(patch), ArchiveSHA256: hashBytes(archive),
		PathPolicy: policy, PathPolicySHA256: policySHA, ExclusionReceiptSHA256: exclusionSHA,
		BuildReceipt: receiptPath, BuildReceiptSHA256: hashBytes(receiptRaw),
	}}
	artifactDir := filepath.Join(t.TempDir(), "sources", "luban")
	snapshot, err := SnapshotAgentAt(context.Background(), agent, time.Now(), artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Source == nil || snapshot.Source.TreeOID != treeOID || snapshot.Source.PatchSHA256 != hashBytes(patch) {
		t.Fatalf("source snapshot = %#v", snapshot.Source)
	}
	for _, name := range []string{"source.patch", "source.tar", "source-exclusions.json"} {
		raw, err := os.ReadFile(filepath.Join(artifactDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte(excludedSecret)) || bytes.Contains(raw, []byte("private-events.jsonl")) {
			t.Fatalf("%s leaked an excluded artifact path or content", name)
		}
	}
	if err := os.Chmod(excludedPath, 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, excludedPath, []byte("changed excluded bytes"))
	writeTestFile(t, filepath.Join(excludedDir, "another-private-file"), []byte("another secret"))
	if _, err := SnapshotAgent(context.Background(), agent, time.Now()); err != nil {
		t.Fatalf("excluded artifact history changed formal source identity: %v", err)
	}
	for _, name := range []string{"source.patch", "source.tar", "source-exclusions.json", "build-receipt.json"} {
		if _, err := os.Stat(filepath.Join(artifactDir, name)); err != nil {
			t.Fatalf("source evidence %s was not archived: %v", name, err)
		}
	}
	if staged := strings.TrimSpace(runTestGit(t, repository, "diff", "--cached", "--name-only")); staged != "" {
		t.Fatalf("source snapshot modified the real index: %s", staged)
	}
	writeTestFile(t, filepath.Join(repository, "tracked.go"), []byte("package fixture\n// drift\n"))
	if _, err := SnapshotAgent(context.Background(), agent, time.Now()); err == nil {
		t.Fatal("source drift was accepted after the immutable snapshot")
	}
}

func TestFormalSourcePolicyRejectsTrackedPrivateArtifactPathsWithoutReadingContent(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(repository, "benchmark-results"), 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repository, "init")
	runTestGit(t, repository, "config", "user.email", "benchmark@example.invalid")
	runTestGit(t, repository, "config", "user.name", "Benchmark")
	path := filepath.Join(repository, "benchmark-results", "tracked-private.jsonl")
	writeTestFile(t, path, []byte("private sentinel that must never enter an archive"))
	runTestGit(t, repository, "add", ".")
	runTestGit(t, repository, "commit", "-m", "base")
	base := strings.TrimSpace(runTestGit(t, repository, "rev-parse", "HEAD"))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if err := validateBaseTreeExclusions(context.Background(), repository, base, FormalSourcePathPolicy()); err == nil {
		t.Fatal("tracked private artifact path was admitted to formal source identity")
	}
}
