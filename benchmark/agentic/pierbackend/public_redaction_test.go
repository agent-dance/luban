package pierbackend

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestPublicRedactionAndRecursiveScanRemoveEveryPrivateAuthorityEncoding(t *testing.T) {
	fixture := redactionFixture(t)
	raw := strings.Join([]string{
		fixture.credential,
		"agentic-bench-dummy-token",
		fixture.access,
		fixture.proxyBaseURL,
		url.QueryEscape(fixture.proxyBaseURL),
		strings.ReplaceAll(fixture.proxyBaseURL, "/", `\/`),
		"HTTP_PROXY=http://user%40mail:pass%3Aword@proxy.internal:3128",
		"http%3A%2F%2Fuser%3Apass%40proxy.internal%3A3128",
		"Authorization: Bearer another-provider-secret-123456",
		"OPENAI_API_KEY=another-provider-secret-123456",
		fixture.privateTask,
		fixture.bundle.Root,
		fixture.bundle.ManifestPath,
	}, "\n")
	redacted := fixture.policy.redact([]byte(raw))
	streamRaw, err := json.Marshal(map[string]any{
		"type": "error",
		"message": strings.Join([]string{
			fixture.credential,
			fixture.proxyBaseURL,
			"Authorization: Bearer another-provider-secret-123456",
			"OPENAI_API_KEY=another-provider-secret-123456",
		}, " "),
	})
	if err != nil {
		t.Fatal(err)
	}
	redactedStream := fixture.policy.redact(append(streamRaw, '\n'))
	if !json.Valid(bytes.TrimSpace(redactedStream)) {
		t.Fatalf("redaction corrupted a JSON stream event: %q", redactedStream)
	}
	publicFiles := []string{
		filepath.Join(fixture.invocation.ArtifactDir, "pier.stdout.log"),
		filepath.Join(fixture.invocation.ArtifactDir, "pier.stderr.log"),
		filepath.Join(fixture.invocation.ArtifactDir, "pier", "agent-stream.jsonl"),
	}
	for index, path := range publicFiles {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := redacted
		if index == len(publicFiles)-1 {
			content = redactedStream
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	receiptPath := filepath.Join(fixture.invocation.ArtifactDir, "pier", "public-secret-scan.json")
	if err := fixture.policy.scanAndWriteReceipt(fixture.invocation.ArtifactDir, receiptPath); err != nil {
		t.Fatal(err)
	}
	receiptRaw, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{fixture.credential, fixture.access, fixture.proxyBaseURL, fixture.privateTask, fixture.bundle.Root, fixture.bundle.ManifestPath} {
		if strings.Contains(string(receiptRaw), secret) {
			t.Fatalf("scan receipt leaked a sentinel: %s", receiptRaw)
		}
	}
	var receipt publicSecretScanReceipt
	if err := json.Unmarshal(receiptRaw, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != publicSecretScanSchema || receipt.TotalMatches != 0 || receipt.ScannedFiles != len(publicFiles) {
		t.Fatalf("scan receipt = %#v", receipt)
	}
}

func TestPublicRedactionRemovesTaskSpecificCredentialAssignmentsWithoutKnowingTheValue(t *testing.T) {
	fixture := redactionFixture(t)
	secrets := []string{
		"dynamic-raw-secret-0123456789",
		"dynamic-json-secret-0123456789",
		"dynamic-escaped-secret-0123456789",
	}
	raw := strings.Join([]string{
		"AGENTIC_SUB_API_KEY=" + secrets[0],
		`{"AGENTIC_SUB_API_KEY":"` + secrets[1] + `"}`,
		`{\"AGENTIC_SUB_API_KEY\":\"` + secrets[2] + `\"}`,
	}, "\n")
	redacted := string(fixture.policy.redact([]byte(raw)))
	for _, secret := range secrets {
		if strings.Contains(redacted, secret) {
			t.Fatalf("task-specific provider credential leaked after redaction: %q", redacted)
		}
	}
	if strings.Count(redacted, "[redacted]") != len(secrets) {
		t.Fatalf("task-specific credential assignments were not all redacted: %q", redacted)
	}
}

func TestPublicScanFailsClosedWithoutEchoingTheLeakedSentinel(t *testing.T) {
	fixture := redactionFixture(t)
	if err := os.MkdirAll(filepath.Join(fixture.invocation.ArtifactDir, "pier"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.invocation.ArtifactDir, "leak.log"), []byte(fixture.proxyBaseURL), 0o600); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(fixture.invocation.ArtifactDir, "pier", "public-secret-scan.json")
	if err := fixture.policy.scanAndWriteReceipt(fixture.invocation.ArtifactDir, receiptPath); err == nil || strings.Contains(err.Error(), fixture.proxyBaseURL) {
		t.Fatalf("secret scan error = %v", err)
	}
	raw, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), fixture.proxyBaseURL) || strings.Contains(string(raw), fixture.access) {
		t.Fatal("failed scan receipt echoed the sentinel")
	}
	var receipt publicSecretScanReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.TotalMatches == 0 {
		t.Fatal("failed scan did not record a content-free match count")
	}
}

func TestPublicSanitizerRemovesDetectedAuthorityBeforeTheTreeIsRetained(t *testing.T) {
	fixture := redactionFixture(t)
	if err := os.MkdirAll(filepath.Join(fixture.invocation.ArtifactDir, "pier"), 0o755); err != nil {
		t.Fatal(err)
	}
	leakPath := filepath.Join(fixture.invocation.ArtifactDir, "submission.patch")
	if err := os.WriteFile(leakPath, []byte(fixture.proxyBaseURL), 0o640); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(fixture.invocation.ArtifactDir, "pier", "public-secret-scan.json")
	changed, err := fixture.policy.sanitizePublicTree(fixture.invocation.ArtifactDir, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("sanitized files = %d, want 1", changed)
	}
	if err := fixture.policy.scanAndWriteReceipt(fixture.invocation.ArtifactDir, receiptPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(leakPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), fixture.proxyBaseURL) || !strings.Contains(string(raw), "[redacted:proxy-access-path]") {
		t.Fatalf("sanitized public artifact = %q", raw)
	}
	info, err := os.Stat(leakPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("sanitized artifact mode = %o", info.Mode().Perm())
	}
}

func TestPrivateRawBundleIsOutsideLedgerAndExplicitlyNonpublished(t *testing.T) {
	fixture := redactionFixture(t)
	trial := filepath.Join(t.TempDir(), "trial")
	if err := os.MkdirAll(filepath.Join(trial, "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	for relative, raw := range map[string]string{
		"agent/stream.jsonl": `{"type":"turn.completed","secret":"` + fixture.credential + `"}` + "\n",
		"agent/stderr.log":   fixture.proxyBaseURL,
		"result.json":        `{"raw":"` + fixture.privateTask + `"}`,
	} {
		path := filepath.Join(trial, filepath.FromSlash(relative))
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runIdentity := strings.Repeat("1", 64)
	if err := archiveNonpublishedRawBundle(fixture.config, fixture.invocation, runIdentity, trial, []byte(fixture.credential), []byte(fixture.proxyBaseURL)); err != nil {
		t.Fatal(err)
	}
	bundleRoot := filepath.Join(fixture.config.PrivateWorkRoot, "nonpublished-raw", runIdentity)
	publicRoot, err := runArtifactRoot(fixture.invocation)
	if err != nil {
		t.Fatal(err)
	}
	if pathHasPrefix(bundleRoot, publicRoot) {
		t.Fatal("private raw bundle entered the public ledger root")
	}
	info, err := os.Stat(bundleRoot)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("private raw directory mode = %o", info.Mode().Perm())
	}
	manifestPath := filepath.Join(bundleRoot, "manifest.json")
	manifestInfo, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifestInfo.Mode().Perm() != 0o600 {
		t.Fatalf("private manifest mode = %o", manifestInfo.Mode().Perm())
	}
	var manifest privateRawManifest
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Published || manifest.LedgerScope != "excluded-private-work-root" || len(manifest.Files) != 5 {
		t.Fatalf("private manifest = %#v", manifest)
	}
}

func TestRedactedTerminalEvidenceHasAPubliclyVerifiableDigest(t *testing.T) {
	fixture := redactionFixture(t)
	rawStream := []byte(`{"type":"error","code":"context_length_exceeded","message":"` + fixture.proxyBaseURL + `"}` + "\n")
	publicStream := fixture.policy.redact(rawStream)
	rawEvidence, err := deriveTerminalEvidence("codex", rawStream, []byte(`{"exit_code":1}`), 1)
	if err != nil {
		t.Fatal(err)
	}
	publicEvidence, err := deriveTerminalEvidence("codex", publicStream, []byte(`{"exit_code":1}`), 1)
	if err != nil {
		t.Fatal(err)
	}
	if rawEvidence.Source != publicEvidence.Source || rawEvidence.Code != publicEvidence.Code || rawEvidence.EvidenceSHA256 == publicEvidence.EvidenceSHA256 {
		t.Fatalf("raw=%#v public=%#v", rawEvidence, publicEvidence)
	}
	line := strings.TrimSpace(string(publicStream))
	if publicEvidence.EvidenceSHA256 != sha256Hex([]byte(line)) {
		t.Fatal("public terminal digest cannot be reproduced from the redacted event")
	}
}

type publicRedactionFixture struct {
	config       Config
	invocation   harness.AgentInvocation
	bundle       codexBundleBinding
	policy       publicRedactionPolicy
	credential   string
	access       string
	proxyBaseURL string
	privateTask  string
}

func redactionFixture(t *testing.T) publicRedactionFixture {
	t.Helper()
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "public-ledger")
	privateRoot := filepath.Join(root, "private-authority")
	invocation := harness.AgentInvocation{
		PlanEntry: harness.PlanEntry{PairID: "pair-001", AgentID: "codex"},
		Agent:     harness.AgentSpec{ID: "codex"}, Task: harness.PublicTaskView{ID: "task-001"},
	}
	invocation.ArtifactDir = filepath.Join(artifactRoot, "runs", invocation.PlanEntry.PairID, invocation.Agent.ID, "attempt-001")
	if err := os.MkdirAll(invocation.ArtifactDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := Config{PrivateWorkRoot: privateRoot}
	bundle := codexBundleBinding{Root: "/private/vendor/codex", ManifestPath: "/private/source/codex.bundle.json"}
	credential := "sk-real-provider-fixture-secret-123456"
	access := "abcdef0123456789abcdef0123456789"
	proxyBaseURL := "http://host.docker.internal:43123/" + access + "/v1"
	privateTask := filepath.Join(privateRoot, "pier-trial-secret", "task")
	policy, err := newPublicRedactionPolicy(invocation, config, bundle, credential, access, proxyBaseURL, privateTask)
	if err != nil {
		t.Fatal(err)
	}
	return publicRedactionFixture{
		config: config, invocation: invocation, bundle: bundle, policy: policy,
		credential: credential, access: access, proxyBaseURL: proxyBaseURL, privateTask: privateTask,
	}
}
