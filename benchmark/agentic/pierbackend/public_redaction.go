package pierbackend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const (
	publicSecretScanSchema = "agentic-bench/public-secret-scan-v1"
	privateRawBundleSchema = "agentic-bench/nonpublished-raw-bundle-v1"
)

type exactRedactionRule struct {
	id       string
	variants [][]byte
}

type regexRedactionRule struct {
	id          string
	pattern     *regexp.Regexp
	replacement []byte
}

type publicRedactionPolicy struct {
	exact []exactRedactionRule
	regex []regexRedactionRule
}

type publicSecretScanRule struct {
	ID         string `json:"id"`
	MatchCount int    `json:"match_count"`
	RuleSHA256 string `json:"rule_sha256"`
}

type publicSecretScanReceipt struct {
	SchemaVersion     string                 `json:"schema_version"`
	ScannedFiles      int                    `json:"scanned_files"`
	ScannedBytes      int64                  `json:"scanned_bytes"`
	ScannedTreeSHA256 string                 `json:"scanned_tree_sha256"`
	TotalMatches      int                    `json:"total_matches"`
	Rules             []publicSecretScanRule `json:"rules"`
}

type privateRawFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type privateRawManifest struct {
	SchemaVersion string           `json:"schema_version"`
	Published     bool             `json:"published"`
	LedgerScope   string           `json:"ledger_scope"`
	RunIdentity   string           `json:"run_identity"`
	AgentID       string           `json:"agent_id"`
	TaskID        string           `json:"task_id"`
	Files         []privateRawFile `json:"files"`
}

func newPublicRedactionPolicy(invocation harness.AgentInvocation, config Config, bundle codexBundleBinding, credential, access, proxyBaseURL, privateTask string) (publicRedactionPolicy, error) {
	if credential == "" || access == "" || proxyBaseURL == "" || privateTask == "" {
		return publicRedactionPolicy{}, errors.New("public redaction policy lacks per-run private authority")
	}
	policy := publicRedactionPolicy{}
	for _, value := range []struct {
		id             string
		secret         string
		filesystemPath bool
	}{
		{id: "provider-credential", secret: credential},
		{id: "agent-dummy-token", secret: "agentic-bench-dummy-token"},
		{id: "proxy-access-path", secret: access},
		{id: "proxy-base-url", secret: proxyBaseURL},
		{id: "private-task-path", secret: privateTask, filesystemPath: true},
		{id: "private-trial-root", secret: filepath.Dir(privateTask), filesystemPath: true},
		{id: "private-work-root", secret: config.PrivateWorkRoot, filesystemPath: true},
		{id: "bundle-root-path", secret: bundle.Root, filesystemPath: true},
		{id: "bundle-manifest-path", secret: bundle.ManifestPath, filesystemPath: true},
		{id: "agent-binary-path", secret: invocation.Agent.Binary, filesystemPath: true},
		{id: "task-instruction-path", secret: invocation.Task.InstructionPath, filesystemPath: true},
		{id: "pier-binary-path", secret: config.PierBinary, filesystemPath: true},
		{id: "python-module-root", secret: config.PythonModuleRoot, filesystemPath: true},
		{id: "dataset-repository-root", secret: config.DatasetRepositoryRoot, filesystemPath: true},
		{id: "evaluator-repository-root", secret: config.EvaluatorRepositoryRoot, filesystemPath: true},
		{id: "evaluator-manifest-path", secret: config.EvaluatorManifestPath, filesystemPath: true},
		{id: "inventory-lock-path", secret: config.InventoryLockPath, filesystemPath: true},
		{id: "registry-gate-path", secret: config.RegistryGatePath, filesystemPath: true},
	} {
		if value.secret == "" {
			continue
		}
		if value.filesystemPath {
			clean := filepath.Clean(value.secret)
			if !filepath.IsAbs(clean) || clean == string(os.PathSeparator) {
				return publicRedactionPolicy{}, errors.New("public redaction policy contains an unsafe private path sentinel")
			}
		}
		policy.exact = append(policy.exact, exactRedactionRule{id: value.id, variants: secretEncodingVariants(value.secret)})
	}
	policy.regex = []regexRedactionRule{
		{
			id:      "proxy-url-userinfo",
			pattern: regexp.MustCompile(`(?i)\b((?:https?|socks5h?)://)[^\s/@]+@`),
			// Remove the whole authority prefix. Keeping the scheme and @ would
			// leave an idempotently detectable credential-shaped URL in the
			// public artifact even after its value had been replaced.
			replacement: []byte(`[redacted:proxy-url-userinfo]`),
		},
		{
			id:          "percent-encoded-proxy-userinfo",
			pattern:     regexp.MustCompile(`(?i)\b((?:https?|socks5h?)%3a%2f%2f)[^\s]+?%40`),
			replacement: []byte(`[redacted:percent-encoded-proxy-userinfo]`),
		},
		{
			id:          "authorization-header",
			pattern:     regexp.MustCompile(`(?i)((?:authorization)(?:\\?["']?)\s*[:=]\s*(?:\\?["']?)?(?:bearer|basic)\s+)[A-Za-z0-9._~+/=%:-]{12,}`),
			replacement: []byte(`${1}[redacted]`),
		},
		{
			id:          "provider-key-assignment",
			pattern:     regexp.MustCompile(`(?i)((?:AGENTIC_SUB_API_KEY|OPENAI_API_KEY|CODEX_API_KEY)(?:\\?["']?)\s*[:=]\s*(?:\\?["']?))[A-Za-z0-9._~+/=%:-]{16,}`),
			replacement: []byte(`${1}[redacted]`),
		},
	}
	return policy, nil
}

func secretEncodingVariants(value string) [][]byte {
	seen := map[string]struct{}{}
	add := func(candidate string) {
		if candidate != "" {
			seen[candidate] = struct{}{}
		}
	}
	add(value)
	add(url.QueryEscape(value))
	add(url.PathEscape(value))
	add(strings.ReplaceAll(value, "/", `\/`))
	if quoted := strconv.Quote(value); len(quoted) >= 2 {
		add(quoted[1 : len(quoted)-1])
	}
	var upper, lower strings.Builder
	for _, octet := range []byte(value) {
		upper.WriteString(fmt.Sprintf("%%%02X", octet))
		lower.WriteString(fmt.Sprintf("%%%02x", octet))
	}
	add(upper.String())
	add(lower.String())
	result := make([][]byte, 0, len(seen))
	for candidate := range seen {
		result = append(result, []byte(candidate))
	}
	sort.Slice(result, func(left, right int) bool { return len(result[left]) > len(result[right]) })
	return result
}

func (policy publicRedactionPolicy) redact(raw []byte) []byte {
	result := bytes.Clone(raw)
	for _, rule := range policy.exact {
		replacement := []byte("[redacted:" + rule.id + "]")
		for _, variant := range rule.variants {
			result = bytes.ReplaceAll(result, variant, replacement)
		}
	}
	for _, rule := range policy.regex {
		result = rule.pattern.ReplaceAll(result, rule.replacement)
	}
	return result
}

// sanitizePublicTree removes every recognized private authority from an
// already-materialized public run tree. It returns the number of files that
// changed so the caller can invalidate the attempt even though the resulting
// tree is safe to retain for diagnosis.
func (policy publicRedactionPolicy) sanitizePublicTree(root, receiptPath string) (int, error) {
	root = filepath.Clean(root)
	receiptPath = filepath.Clean(receiptPath)
	if receiptPath == root || !pathHasPrefix(receiptPath, root) {
		return 0, errors.New("public secret scan receipt escapes the run artifact root")
	}
	changedFiles := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("public artifact tree contains a symlink")
		}
		if path == receiptPath {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("public artifact tree contains a non-regular file")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		redacted := policy.redact(raw)
		if bytes.Equal(raw, redacted) {
			return nil
		}
		if err := harness.WriteBytesAtomic(path, redacted, info.Mode().Perm()); err != nil {
			return err
		}
		changedFiles++
		return nil
	})
	return changedFiles, err
}

func (policy publicRedactionPolicy) scanAndWriteReceipt(root, receiptPath string) error {
	root = filepath.Clean(root)
	receiptPath = filepath.Clean(receiptPath)
	if receiptPath == root || !pathHasPrefix(receiptPath, root) {
		return errors.New("public secret scan receipt escapes the run artifact root")
	}
	counts := make(map[string]int, len(policy.exact)+len(policy.regex))
	type scannedFile struct {
		relative string
		raw      []byte
	}
	files := make([]scannedFile, 0)
	var scannedBytes int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("public artifact tree contains a symlink")
		}
		if path == receiptPath {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("public artifact tree contains a non-regular file")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		files = append(files, scannedFile{relative: relative, raw: raw})
		scannedBytes += int64(len(raw))
		for _, rule := range policy.exact {
			for _, variant := range rule.variants {
				counts[rule.id] += bytes.Count(raw, variant)
			}
		}
		for _, rule := range policy.regex {
			counts[rule.id] += len(rule.pattern.FindAllIndex(raw, -1))
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(files, func(left, right int) bool { return files[left].relative < files[right].relative })
	tree := bytes.Buffer{}
	for _, file := range files {
		digest := sha256Hex(file.raw)
		tree.WriteString(file.relative)
		tree.WriteByte(0)
		tree.WriteString(digest)
		tree.WriteByte('\n')
	}
	ruleIDs := make([]string, 0, len(counts))
	for _, rule := range policy.exact {
		ruleIDs = append(ruleIDs, rule.id)
	}
	for _, rule := range policy.regex {
		ruleIDs = append(ruleIDs, rule.id)
	}
	slices.Sort(ruleIDs)
	ruleIDs = slices.Compact(ruleIDs)
	receipt := publicSecretScanReceipt{
		SchemaVersion: publicSecretScanSchema, ScannedFiles: len(files), ScannedBytes: scannedBytes,
		ScannedTreeSHA256: sha256Hex(tree.Bytes()),
	}
	for _, id := range ruleIDs {
		count := counts[id]
		receipt.TotalMatches += count
		receipt.Rules = append(receipt.Rules, publicSecretScanRule{
			ID: id, MatchCount: count, RuleSHA256: sha256Hex([]byte(id + ":v1")),
		})
	}
	if err := harness.WriteJSONAtomic(receiptPath, receipt, 0o600); err != nil {
		return err
	}
	if receipt.TotalMatches != 0 {
		return errors.New("public artifact secret scan found prohibited private authority")
	}
	return nil
}

func archiveNonpublishedRawBundle(config Config, invocation harness.AgentInvocation, runIdentity, trialDir string, pierStdout, pierStderr []byte) error {
	artifactRoot, err := runArtifactRoot(invocation)
	if err != nil {
		return err
	}
	privateRoot := filepath.Clean(config.PrivateWorkRoot)
	if pathsOverlap(privateRoot, artifactRoot) {
		return errors.New("private raw root overlaps the public artifact ledger scope")
	}
	nonpublishedRoot := filepath.Join(privateRoot, "nonpublished-raw")
	if err := os.MkdirAll(nonpublishedRoot, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(nonpublishedRoot, 0o700); err != nil {
		return err
	}
	bundleRoot := filepath.Join(nonpublishedRoot, runIdentity)
	if err := os.Mkdir(bundleRoot, 0o700); err != nil {
		return err
	}
	sources := []struct {
		name string
		raw  []byte
		path string
	}{
		{name: "pier.stdout.log", raw: pierStdout},
		{name: "pier.stderr.log", raw: pierStderr},
		{name: "agent-stream.jsonl", path: filepath.Join(trialDir, "agent", "stream.jsonl")},
		{name: "agent-stderr.log", path: filepath.Join(trialDir, "agent", "stderr.log")},
		{name: "pier-result.json", path: filepath.Join(trialDir, "result.json")},
	}
	manifest := privateRawManifest{
		SchemaVersion: privateRawBundleSchema, Published: false, LedgerScope: "excluded-private-work-root",
		RunIdentity: runIdentity, AgentID: invocation.Agent.ID, TaskID: invocation.Task.ID,
	}
	for _, source := range sources {
		raw := source.raw
		if source.path != "" {
			raw, err = os.ReadFile(source.path)
			if err != nil {
				return err
			}
		}
		path := filepath.Join(bundleRoot, source.name)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, privateRawFile{Name: source.name, SHA256: sha256Hex(raw), Size: int64(len(raw))})
	}
	if raw, readErr := os.ReadFile(filepath.Join(trialDir, "agent", "terminal-evidence.json")); readErr == nil {
		const name = "agent-terminal-evidence.json"
		if err := os.WriteFile(filepath.Join(bundleRoot, name), raw, 0o600); err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, privateRawFile{Name: name, SHA256: sha256Hex(raw), Size: int64(len(raw))})
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(bundleRoot, "manifest.json"), append(manifestRaw, '\n'), 0o600)
}

func runArtifactRoot(invocation harness.AgentInvocation) (string, error) {
	attempt := filepath.Clean(invocation.ArtifactDir)
	agentDir := filepath.Dir(attempt)
	pairDir := filepath.Dir(agentDir)
	runsDir := filepath.Dir(pairDir)
	root := filepath.Dir(runsDir)
	if filepath.Base(attempt) != "attempt-001" || filepath.Base(agentDir) != invocation.Agent.ID || filepath.Base(pairDir) != invocation.PlanEntry.PairID || filepath.Base(runsDir) != "runs" {
		return "", errors.New("agent artifact path does not have the frozen run layout")
	}
	return root, nil
}

func pathsOverlap(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	return left == right || pathHasPrefix(left, right) || pathHasPrefix(right, left)
}

func pathHasPrefix(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
