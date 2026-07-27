package harness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type FileRecord struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	RawMode string `json:"raw_mode,omitempty"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Kind    string `json:"kind,omitempty"`
}

type TreeInventory struct {
	SchemaVersion string       `json:"schema_version"`
	Files         []FileRecord `json:"files"`
	SHA256        string       `json:"sha256"`
	RawSHA256     string       `json:"raw_sha256"`
}

// HashTree computes a cross-process deterministic source inventory. A symlink
// is hashed as its link text with domain separation and is never dereferenced,
// so a repository-owned link cannot smuggle bytes from outside root. Other
// special files fail closed. Artifact ledgers remain stricter and reject every
// non-regular file in HashTreeExcluding.
func HashTree(root string) (TreeInventory, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return TreeInventory{}, err
	}
	var files []FileRecord
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rawMode := fmt.Sprintf("%04o", info.Mode().Perm())
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(path)
			if readErr != nil {
				return readErr
			}
			sum := sha256.Sum256(append([]byte("symlink\x00"), []byte(target)...))
			files = append(files, FileRecord{
				Path: filepath.ToSlash(relative), Mode: "120000", RawMode: rawMode,
				Size: int64(len([]byte(target))), SHA256: hex.EncodeToString(sum[:]), Kind: "symlink",
			})
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("tree contains unsupported file type: %s", path)
		}
		digest, err := HashFile(path)
		if err != nil {
			return err
		}
		files = append(files, FileRecord{
			Path: filepath.ToSlash(relative), Mode: canonicalSourceMode(info.Mode()), RawMode: rawMode,
			Size: info.Size(), SHA256: digest,
		})
		return nil
	})
	if err != nil {
		return TreeInventory{}, err
	}
	slices.SortFunc(files, func(left, right FileRecord) int { return strings.Compare(left.Path, right.Path) })
	hasher := sha256.New()
	rawHasher := sha256.New()
	for _, file := range files {
		writeTreeRecord(hasher, file, file.Mode)
		writeTreeRecord(rawHasher, file, file.RawMode)
	}
	return TreeInventory{
		SchemaVersion: "agentic-bench/tree-v2",
		Files:         files,
		SHA256:        hex.EncodeToString(hasher.Sum(nil)),
		RawSHA256:     hex.EncodeToString(rawHasher.Sum(nil)),
	}, nil
}

// canonicalSourceMode retains the only source-mode bit with execution
// semantics. Group/other permission bits commonly drift under umask, archive
// extraction, and shared worktrees and therefore cannot define task identity.
func canonicalSourceMode(mode fs.FileMode) string {
	if mode.Perm()&0o111 != 0 {
		return "0755"
	}
	return "0644"
}

func writeTreeRecord(writer io.Writer, file FileRecord, mode string) {
	_, _ = io.WriteString(writer, file.Path)
	_, _ = writer.Write([]byte{0})
	_, _ = io.WriteString(writer, mode)
	_, _ = writer.Write([]byte{0})
	_, _ = io.WriteString(writer, strconv.FormatInt(file.Size, 10))
	_, _ = writer.Write([]byte{0})
	_, _ = io.WriteString(writer, file.SHA256)
	_, _ = writer.Write([]byte{'\n'})
}

func HashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

type ArtifactLedger struct {
	SchemaVersion  string       `json:"schema_version"`
	ManifestSHA256 string       `json:"manifest_sha256"`
	Files          []FileRecord `json:"files"`
	LedgerSHA256   string       `json:"ledger_sha256"`
}

// BuildArtifactLedger hashes every regular artifact except the ledger itself.
// The ledger digest covers canonical JSON with LedgerSHA256 empty.
func BuildArtifactLedger(root, ledgerRelativePath, manifestSHA256 string) (ArtifactLedger, error) {
	inventory, err := HashTreeExcluding(root, map[string]struct{}{filepath.ToSlash(ledgerRelativePath): {}})
	if err != nil {
		return ArtifactLedger{}, err
	}
	for index := range inventory.Files {
		inventory.Files[index].Kind = artifactKind(inventory.Files[index].Path)
	}
	ledger := ArtifactLedger{
		SchemaVersion:  "agentic-bench/artifact-ledger-v1",
		ManifestSHA256: manifestSHA256,
		Files:          inventory.Files,
	}
	canonical, err := json.Marshal(ledger)
	if err != nil {
		return ArtifactLedger{}, err
	}
	sum := sha256.Sum256(canonical)
	ledger.LedgerSHA256 = hex.EncodeToString(sum[:])
	return ledger, nil
}

func HashTreeExcluding(root string, excluded map[string]struct{}) (TreeInventory, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return TreeInventory{}, err
	}
	var files []FileRecord
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, skip := excluded[relative]; skip {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact tree contains unsupported file type: %s", relative)
		}
		digest, err := HashFile(path)
		if err != nil {
			return err
		}
		files = append(files, FileRecord{Path: relative, Mode: fmt.Sprintf("%04o", info.Mode().Perm()), Size: info.Size(), SHA256: digest})
		return nil
	})
	if err != nil {
		return TreeInventory{}, err
	}
	slices.SortFunc(files, func(left, right FileRecord) int { return strings.Compare(left.Path, right.Path) })
	return TreeInventory{SchemaVersion: "agentic-bench/tree-v1", Files: files}, nil
}

func artifactKind(path string) string {
	switch {
	case strings.HasSuffix(path, ".patch"):
		return "submission"
	case strings.Contains(path, "/verifier/") || strings.HasPrefix(path, "verifier/"):
		return "verifier"
	case strings.Contains(path, "/metrics/") || strings.HasPrefix(path, "metrics/"):
		return "metrics"
	case strings.Contains(path, "/logs/") || strings.HasPrefix(path, "logs/"):
		return "log"
	default:
		return "metadata"
	}
}

func WriteJSONAtomic(path string, value any, mode fs.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return WriteBytesAtomic(path, data, mode)
}

func WriteBytesAtomic(path string, data []byte, mode fs.FileMode) error {
	directory := filepath.Dir(path)
	if err := mkdirAllDurable(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".agentic-bench-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if written, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	} else if written != len(data) {
		temporary.Close()
		return io.ErrShortWrite
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func mkdirAllDurable(path string, mode fs.FileMode) error {
	path = filepath.Clean(path)
	missing := make([]string, 0)
	current := path
	for {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("artifact parent is not a directory: %s", current)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("no existing ancestor for artifact directory")
		}
		current = parent
	}
	for index := len(missing) - 1; index >= 0; index-- {
		if err := os.Mkdir(missing[index], mode); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := syncDirectory(filepath.Dir(missing[index])); err != nil {
			return err
		}
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}

// CaptureGitWorkspace creates one binary patch covering committed, staged,
// unstaged, deleted, and untracked files without modifying the real Git index.
func CaptureGitWorkspace(ctx context.Context, repoDir, baseCommit, patchPath string) error {
	if !hex40Pattern.MatchString(baseCommit) {
		return errors.New("base commit must be a full Git SHA")
	}
	repoDir, err := filepath.Abs(repoDir)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp("", "agentic-bench-index-*")
	if err != nil {
		return err
	}
	indexPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(indexPath); err != nil {
		return err
	}
	defer os.Remove(indexPath)
	environment := append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
	if _, err := runGit(ctx, repoDir, environment, "rev-parse", "--verify", baseCommit+"^{commit}"); err != nil {
		return fmt.Errorf("verify base commit: %w", err)
	}
	if _, err := runGit(ctx, repoDir, environment, "read-tree", baseCommit); err != nil {
		return fmt.Errorf("initialize temporary index: %w", err)
	}
	if _, err := runGit(ctx, repoDir, environment, "add", "-A", "--", "."); err != nil {
		return fmt.Errorf("capture workspace in temporary index: %w", err)
	}
	patch, err := runGit(ctx, repoDir, environment, "diff", "--cached", "--binary", "--full-index", "--no-ext-diff", baseCommit, "--")
	if err != nil {
		return fmt.Errorf("generate binary workspace patch: %w", err)
	}
	return WriteBytesAtomic(patchPath, patch, 0o644)
}

// CaptureGitWorkspaceEvidence runs CaptureGitWorkspace and returns the
// normalized evidence that a Backend must attach to AgentExecution.
func CaptureGitWorkspaceEvidence(ctx context.Context, repoDir, baseCommit, patchPath string) (SubmissionCaptureEvidence, error) {
	if err := CaptureGitWorkspace(ctx, repoDir, baseCommit, patchPath); err != nil {
		return SubmissionCaptureEvidence{}, err
	}
	digest, err := HashFile(patchPath)
	if err != nil {
		return SubmissionCaptureEvidence{}, err
	}
	return SubmissionCaptureEvidence{
		Method: "temporary-git-index-v1", BaseCommit: baseCommit, PatchSHA256: digest,
		IncludesTracked: true, IncludesUntracked: true, IncludesBinary: true,
	}, nil
}

func runGit(ctx context.Context, directory string, environment []string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = directory
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func SnapshotAgent(ctx context.Context, agent AgentSpec, now time.Time) (AgentSnapshot, error) {
	return SnapshotAgentAt(ctx, agent, now, "")
}

var formalSourceExcludedPrefixes = []string{
	".agentic-bench/",
	".codex/",
	".luban/",
	".tmp/",
	"benchmark-artifacts/",
	"benchmark-results/",
}

func FormalSourcePathPolicy() SourcePathPolicy {
	return SourcePathPolicy{
		SchemaVersion:    "agentic-bench/source-path-policy-v1",
		ExcludedPrefixes: slices.Clone(formalSourceExcludedPrefixes),
	}
}

func validateFormalSourcePathPolicy(policy SourcePathPolicy, claimedSHA string) error {
	want := FormalSourcePathPolicy()
	if policy.SchemaVersion != want.SchemaVersion || !slices.Equal(policy.ExcludedPrefixes, want.ExcludedPrefixes) {
		return errors.New("source path policy does not match the formal content-blind exclusion policy")
	}
	digest, err := HashCanonical(policy)
	if err != nil {
		return err
	}
	if digest != claimedSHA {
		return errors.New("source path policy digest mismatch")
	}
	return nil
}

func sourceSnapshotAddArgs(policy SourcePathPolicy) []string {
	args := []string{"add", "-A", "--", "."}
	for _, prefix := range policy.ExcludedPrefixes {
		path := strings.TrimSuffix(prefix, "/")
		args = append(args, ":(exclude)"+path, ":(exclude,glob)"+path+"/**")
	}
	return args
}

func validateBaseTreeExclusions(ctx context.Context, worktree, baseCommit string, policy SourcePathPolicy) error {
	args := []string{"ls-tree", "-r", "--name-only", "-z", baseCommit, "--"}
	for _, prefix := range policy.ExcludedPrefixes {
		args = append(args, strings.TrimSuffix(prefix, "/"))
	}
	paths, err := runGit(ctx, worktree, cleanGitEnvironment(), args...)
	if err != nil {
		return err
	}
	if len(bytesTrimSpace(bytes.ReplaceAll(paths, []byte{0}, []byte{' '}))) != 0 {
		return errors.New("base source tree contains a path reserved for private benchmark artifacts")
	}
	return nil
}

func captureSourceExclusionReceipt(policy SourcePathPolicy, policySHA string) (SourceExclusionReceipt, []byte, string, error) {
	receipt := SourceExclusionReceipt{
		SchemaVersion: "agentic-bench/source-exclusion-receipt-v1",
		PathPolicy:    policy, PathPolicySHA256: policySHA, Applied: true,
		Implementation: "git-negative-pathspec-before-content-scan-v1",
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return SourceExclusionReceipt{}, nil, "", err
	}
	raw = append(raw, '\n')
	return receipt, raw, hashBytes(raw), nil
}

// SnapshotAgentAt verifies an agent binary and, when configured, reconstructs
// its complete source tree in an isolated temporary Git index/object store.
// A non-empty artifactDir persists the immutable patch, deterministic archive,
// and build receipt; existing files are verified byte-for-byte, never replaced.
func SnapshotAgentAt(ctx context.Context, agent AgentSpec, now time.Time, artifactDir string) (AgentSnapshot, error) {
	digest, err := HashFile(agent.Binary)
	if err != nil {
		return AgentSnapshot{}, err
	}
	if digest != agent.BinarySHA256 {
		return AgentSnapshot{}, fmt.Errorf("agent %s binary SHA-256 mismatch", agent.ID)
	}
	snapshot := AgentSnapshot{AgentID: agent.ID, BinarySHA256: digest, CapturedAt: now.UTC()}
	if agent.SourceSnapshot == nil {
		return snapshot, nil
	}
	source := *agent.SourceSnapshot
	if err := validateFormalSourcePathPolicy(source.PathPolicy, source.PathPolicySHA256); err != nil {
		return AgentSnapshot{}, fmt.Errorf("agent %s: %w", agent.ID, err)
	}
	worktree, err := filepath.Abs(source.Worktree)
	if err != nil {
		return AgentSnapshot{}, err
	}
	receiptPath, err := filepath.Abs(source.BuildReceipt)
	if err != nil {
		return AgentSnapshot{}, err
	}
	if receiptPath == worktree || stringsHasPathPrefix(receiptPath, worktree) {
		return AgentSnapshot{}, fmt.Errorf("agent %s build receipt must be outside its source worktree", agent.ID)
	}
	head, err := runGit(ctx, worktree, cleanGitEnvironment(), "rev-parse", "HEAD^{commit}")
	if err != nil {
		return AgentSnapshot{}, err
	}
	if strings.TrimSpace(string(head)) != source.BaseCommit {
		return AgentSnapshot{}, fmt.Errorf("agent %s source base commit mismatch", agent.ID)
	}
	if err := validateBaseTreeExclusions(ctx, worktree, source.BaseCommit, source.PathPolicy); err != nil {
		return AgentSnapshot{}, fmt.Errorf("agent %s: %w", agent.ID, err)
	}

	temporaryRoot, err := os.MkdirTemp("", "agentic-bench-source-*")
	if err != nil {
		return AgentSnapshot{}, err
	}
	defer os.RemoveAll(temporaryRoot)
	objectDirectory := filepath.Join(temporaryRoot, "objects")
	if err := os.MkdirAll(objectDirectory, 0o700); err != nil {
		return AgentSnapshot{}, err
	}
	baseObjects, err := runGit(ctx, worktree, cleanGitEnvironment(), "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return AgentSnapshot{}, err
	}
	gitEnvironment := append(cleanGitEnvironment(),
		"GIT_INDEX_FILE="+filepath.Join(temporaryRoot, "index"),
		"GIT_OBJECT_DIRECTORY="+objectDirectory,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES="+strings.TrimSpace(string(baseObjects)),
	)
	if _, err := runGit(ctx, worktree, gitEnvironment, "read-tree", source.BaseCommit); err != nil {
		return AgentSnapshot{}, fmt.Errorf("initialize agent %s source snapshot: %w", agent.ID, err)
	}
	if _, err := runGit(ctx, worktree, gitEnvironment, sourceSnapshotAddArgs(source.PathPolicy)...); err != nil {
		return AgentSnapshot{}, fmt.Errorf("capture agent %s source snapshot: %w", agent.ID, err)
	}
	for _, prefix := range source.PathPolicy.ExcludedPrefixes {
		if _, err := runGit(ctx, worktree, gitEnvironment, "rm", "-r", "-f", "--cached", "--ignore-unmatch", "--", strings.TrimSuffix(prefix, "/")); err != nil {
			return AgentSnapshot{}, fmt.Errorf("exclude private source path for agent %s: %w", agent.ID, err)
		}
	}
	_, exclusionRaw, exclusionSHA, err := captureSourceExclusionReceipt(source.PathPolicy, source.PathPolicySHA256)
	if err != nil {
		return AgentSnapshot{}, fmt.Errorf("capture content-blind source exclusions for agent %s: %w", agent.ID, err)
	}
	if exclusionSHA != source.ExclusionReceiptSHA256 {
		return AgentSnapshot{}, fmt.Errorf("agent %s source exclusion receipt SHA-256 mismatch", agent.ID)
	}
	treeBytes, err := runGit(ctx, worktree, gitEnvironment, "write-tree")
	if err != nil {
		return AgentSnapshot{}, fmt.Errorf("write agent %s source tree: %w", agent.ID, err)
	}
	treeOID := strings.TrimSpace(string(treeBytes))
	if treeOID != source.TreeOID {
		return AgentSnapshot{}, fmt.Errorf("agent %s source tree OID mismatch", agent.ID)
	}
	patch, err := runGit(ctx, worktree, gitEnvironment, "diff", "--cached", "--binary", "--full-index", "--no-ext-diff", source.BaseCommit, "--")
	if err != nil {
		return AgentSnapshot{}, fmt.Errorf("generate agent %s source patch: %w", agent.ID, err)
	}
	patchSHA := hashBytes(patch)
	if patchSHA != source.PatchSHA256 {
		return AgentSnapshot{}, fmt.Errorf("agent %s source patch SHA-256 mismatch", agent.ID)
	}
	archive, err := runGit(ctx, worktree, gitEnvironment, "archive", "--format=tar", "--mtime=1970-01-01T00:00:00Z", treeOID)
	if err != nil {
		return AgentSnapshot{}, fmt.Errorf("archive agent %s source tree: %w", agent.ID, err)
	}
	archiveSHA := hashBytes(archive)
	if archiveSHA != source.ArchiveSHA256 {
		return AgentSnapshot{}, fmt.Errorf("agent %s source archive SHA-256 mismatch", agent.ID)
	}
	receiptRaw, err := os.ReadFile(receiptPath)
	if err != nil {
		return AgentSnapshot{}, err
	}
	if hashBytes(receiptRaw) != source.BuildReceiptSHA256 {
		return AgentSnapshot{}, fmt.Errorf("agent %s build receipt SHA-256 mismatch", agent.ID)
	}
	if err := validateAgentBuildReceipt(receiptRaw, agent, source); err != nil {
		return AgentSnapshot{}, err
	}
	if artifactDir != "" {
		for name, content := range map[string][]byte{
			"source.patch": patch, "source.tar": archive, "source-exclusions.json": exclusionRaw, "build-receipt.json": receiptRaw,
		} {
			if err := writeOrVerifyBytes(filepath.Join(artifactDir, name), content, 0o644); err != nil {
				return AgentSnapshot{}, fmt.Errorf("archive agent %s source evidence: %w", agent.ID, err)
			}
		}
	}
	snapshot.Source = &AgentSourceSnapshot{
		BaseCommit: source.BaseCommit, TreeOID: treeOID, PatchSHA256: patchSHA,
		ArchiveSHA256: archiveSHA, PathPolicy: source.PathPolicy, PathPolicySHA256: source.PathPolicySHA256,
		ExclusionReceiptSHA256: exclusionSHA, BuildReceiptSHA256: source.BuildReceiptSHA256,
	}
	return snapshot, nil
}

func validateAgentBuildReceipt(raw []byte, agent AgentSpec, source AgentSourceSpec) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var receipt AgentBuildReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return fmt.Errorf("decode agent %s build receipt: %w", agent.ID, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode agent %s build receipt: trailing JSON value", agent.ID)
		}
		return fmt.Errorf("decode agent %s build receipt trailer: %w", agent.ID, err)
	}
	if receipt.SchemaVersion != "agentic-bench/agent-build-receipt-v2" || receipt.AgentID != agent.ID ||
		receipt.BaseCommit != source.BaseCommit || receipt.TreeOID != source.TreeOID ||
		receipt.PatchSHA256 != source.PatchSHA256 || receipt.ArchiveSHA256 != source.ArchiveSHA256 ||
		receipt.PathPolicy.SchemaVersion != source.PathPolicy.SchemaVersion || !slices.Equal(receipt.PathPolicy.ExcludedPrefixes, source.PathPolicy.ExcludedPrefixes) ||
		receipt.PathPolicySHA256 != source.PathPolicySHA256 || receipt.ExclusionReceiptSHA256 != source.ExclusionReceiptSHA256 ||
		receipt.BinarySHA256 != agent.BinarySHA256 || len(receipt.BuildArgv) == 0 ||
		receipt.Toolchain == "" || receipt.BuiltAt.IsZero() {
		return fmt.Errorf("agent %s build receipt does not bind the frozen source and binary", agent.ID)
	}
	return nil
}

func cleanGitEnvironment() []string {
	result := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES":
			continue
		default:
			result = append(result, value)
		}
	}
	return result
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func writeOrVerifyBytes(path string, value []byte, mode fs.FileMode) error {
	existing, err := os.ReadFile(path)
	if err == nil {
		if !slices.Equal(existing, value) {
			return errors.New("immutable artifact content differs")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return WriteBytesAtomic(path, value, mode)
}

func bytesTrimSpace(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}

// ReadJSONLines streams strict normalized evidence without accepting trailing
// garbage or unknown fields.
func ReadJSONLines[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var values []T
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		if len(bytesTrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		var value T
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode JSONL line %d: %w", line, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			if err == nil {
				return nil, fmt.Errorf("decode JSONL line %d: trailing JSON value", line)
			}
			return nil, fmt.Errorf("decode JSONL line %d trailer: %w", line, err)
		}
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
