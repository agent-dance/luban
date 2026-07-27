package pierbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

const (
	historicalCodexV7CanaryReceiptRelativePath = "benchmark/agentic/pier/codex-exec-v7-multi-agent-wire.receipt.json"
	historicalCodexV7CanaryReceiptSHA256       = "1d420772136459b44666ea5744376e0dcb34b61bed84be6851dfef8458af9649"
	historicalCodexV7AdapterSHA256             = "16269c5a8d6dbe1c51b41251ae7fafa66fca7609b7acd1ae4f0530fb59fc7602"
)

func TestHistoricalCodexV7ArchiveExactBytesAcceptedForAuditOnly(t *testing.T) {
	moduleRoot := pierModuleRoot(t)
	path := filepath.Join(moduleRoot, filepath.FromSlash(historicalCodexV7CanaryReceiptRelativePath))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := auditHistoricalCodexV7Archive(raw); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if digest != historicalCodexV7CanaryReceiptSHA256 {
		t.Fatalf("historical v7 SHA-256 = %s", digest)
	}
}

func TestHistoricalCodexV7MutationRejected(t *testing.T) {
	path := filepath.Join(pierModuleRoot(t), filepath.FromSlash(historicalCodexV7CanaryReceiptRelativePath))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 1
	if err := auditHistoricalCodexV7Archive(raw); err == nil {
		t.Fatal("mutated historical v7 archive passed audit")
	}
}

func TestProductionV8AuthorityRejectsHistoricalV7AndPendingFailsTyped(t *testing.T) {
	if _, err := requireFormalCodexV8CanaryReady(); !errors.Is(err, ErrCodexV8CanonicalCanaryPending) {
		t.Fatalf("pending authority error = %v", err)
	} else if info, ok := i18n.DescribeSemanticError(err); !ok || info.Key != i18n.KeyBenchmarkCodexV8CanaryPending {
		t.Fatalf("pending authority semantic error = %#v, %v", info, ok)
	}

	v7Spec := codexCanaryAuthoritySpec{
		Generation: "v7", RelativePath: historicalCodexV7CanaryReceiptRelativePath,
		ReceiptSHA256: historicalCodexV7CanaryReceiptSHA256, State: codexCanaryVerifiedFormal,
	}
	if _, err := resolveFormalCodexV8CanaryBindingWithSpec(Config{PythonModuleRoot: pierModuleRoot(t)}, adapterBinding{}, codexBundleBinding{}, v7Spec); err == nil {
		t.Fatal("historical v7 archive was admitted as production v8 authority")
	}
}

func TestFormalV8PendingRejectsPreflightBeforeBackendWork(t *testing.T) {
	backend := &Backend{}
	if _, err := backend.Preflight(context.Background(), formalManifestFixture()); !errors.Is(err, ErrCodexV8CanonicalCanaryPending) {
		t.Fatalf("Preflight pending error = %v", err)
	}
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	if backend.ready || backend.codexCanary != (formalCodexCanonicalCanaryBinding{}) || backend.lubanCanary != (formalExecutionCanaryBinding{}) || backend.adapter != (adapterBinding{}) {
		t.Fatalf("pending Preflight mutated backend state: ready=%v codex=%#v luban=%#v adapter=%#v", backend.ready, backend.codexCanary, backend.lubanCanary, backend.adapter)
	}
}

func auditHistoricalCodexV7Archive(raw []byte) error {
	digest := sha256Hex(raw)
	if digest != historicalCodexV7CanaryReceiptSHA256 {
		return errors.New("historical Codex v7 archive differs from its audit pin")
	}
	var receipt struct {
		SchemaVersion        string `json:"schema_version"`
		AdapterSHA256        string `json:"adapter_sha256"`
		AgentKind            string `json:"agent_kind"`
		BinarySHA256         string `json:"binary_sha256"`
		EffectiveArgvReceipt struct {
			AdapterVersion string `json:"adapter_version"`
		} `json:"effective_argv_receipt"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&receipt); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("historical Codex v7 archive contains trailing JSON")
	}
	if receipt.SchemaVersion != "agentic-bench/sandbox-canary-v3" || receipt.AgentKind != "codex" ||
		receipt.BinarySHA256 != Codex0145BinarySHA256 || receipt.AdapterSHA256 != historicalCodexV7AdapterSHA256 ||
		receipt.EffectiveArgvReceipt.AdapterVersion != "2.3.0" {
		return errors.New("historical Codex v7 archive metadata differs from its audit contract")
	}
	return nil
}

func pierModuleRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
}

func fileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256Hex(raw), nil
}
