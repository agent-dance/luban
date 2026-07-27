package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateServiceTierCanonicalizationArchiveAcceptsExactV2Binding(t *testing.T) {
	artifactDir := t.TempDir()
	pierDir := filepath.Join(artifactDir, "pier")
	metricsDir := filepath.Join(artifactDir, "metrics")
	if err := os.MkdirAll(pierDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(metricsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rawEvidencePath := filepath.Join(metricsDir, "provider-http.raw.jsonl")
	if err := os.WriteFile(rawEvidencePath, []byte("sealed raw evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rawEvidenceSHA, err := HashFile(rawEvidencePath)
	if err != nil {
		t.Fatal(err)
	}
	round := ProviderRoundEvidence{
		Round: 0, ClientAgentID: "codex",
		OriginalRequestBodySHA256: strings.Repeat("1", 64), ForwardedRequestBodySHA256: strings.Repeat("2", 64),
		ServiceTierTransformationProofSHA256: strings.Repeat("3", 64),
	}
	type transformationProjection struct {
		Round            int    `json:"round"`
		OriginalBodySHA  string `json:"original_body_sha256"`
		ForwardedBodySHA string `json:"forwarded_body_sha256"`
		ProofSHA         string `json:"proof_sha256"`
	}
	transformationSHA, err := HashCanonical([]transformationProjection{{
		Round: round.Round, OriginalBodySHA: round.OriginalRequestBodySHA256,
		ForwardedBodySHA: round.ForwardedRequestBodySHA256, ProofSHA: round.ServiceTierTransformationProofSHA256,
	}})
	if err != nil {
		t.Fatal(err)
	}
	runIdentity := strings.Repeat("f", 64)
	payload := archivedServiceTierCanonicalizationPayload{
		SchemaVersion:  ServiceTierCanonicalizationBindingSchemaVersion,
		Representation: ServiceTierEncodingClientCanonical, ClientAgentID: "codex", ClientRuntimeVersion: "0.145.0",
		RunIdentity: runIdentity, RegisteredBinarySHA256: strings.Repeat("4", 64),
		FrozenBundleManifestSHA256: strings.Repeat("5", 64), FrozenBundleTreeSHA256: strings.Repeat("6", 64),
		AdapterSHA256: strings.Repeat("7", 64), AdapterVersion: "2.4.0",
		SourceCommandArgvSHA256: strings.Repeat("8", 64), EffectiveArgvSHA256: strings.Repeat("9", 64),
		EffectiveArgvReceiptSHA256: strings.Repeat("a", 64), SandboxCanaryReceiptSHA256: strings.Repeat("b", 64),
		CanonicalCanaryGeneration: FormalExecutionCanaryGeneration, FrozenCanonicalCanaryReceiptSHA256: strings.Repeat("c", 64),
		RawProviderEvidenceSHA256: rawEvidenceSHA, TransformationEvidenceSHA256: transformationSHA,
		TransformedProviderRoundCount: 1, StaticProofSHA256: strings.Repeat("d", 64),
	}
	bindingSHA, err := HashCanonical(payload)
	if err != nil {
		t.Fatal(err)
	}
	round.ClientCanonicalizationProofSHA256 = bindingSHA
	receipt := archivedServiceTierCanonicalizationReceipt{archivedServiceTierCanonicalizationPayload: payload, BindingSHA256: bindingSHA}
	receiptPath := filepath.Join(pierDir, "service-tier-canonicalization-receipt.json")
	if err := WriteJSONAtomic(receiptPath, receipt, 0o644); err != nil {
		t.Fatal(err)
	}
	receiptSHA, err := HashFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	execution := AgentExecution{
		EvidenceRunIdentity: runIdentity,
		ProviderEvidence:    ProviderEvidenceSeal{RawEvidenceSHA256: rawEvidenceSHA},
		ServiceTierCanonicalization: ServiceTierCanonicalizationEvidence{
			SchemaVersion: ServiceTierCanonicalizationEvidenceSchemaVersion, Representation: ServiceTierEncodingClientCanonical,
			ReceiptRelativePath: "pier/service-tier-canonicalization-receipt.json", ReceiptSHA256: receiptSHA,
			BindingSHA256: bindingSHA, StaticProofSHA256: payload.StaticProofSHA256,
			TransformationEvidenceSHA256: transformationSHA, TransformedRoundCount: 1,
		},
	}
	if err := ValidateServiceTierCanonicalizationArchive(artifactDir, "codex", execution, []ProviderRoundEvidence{round}); err != nil {
		t.Fatalf("exact v2 canonicalization binding was rejected: %v", err)
	}

	invalidRound := round
	invalidRound.ClientCanonicalizationProofSHA256 = payload.StaticProofSHA256
	if err := ValidateServiceTierCanonicalizationArchive(artifactDir, "codex", execution, []ProviderRoundEvidence{invalidRound}); err == nil {
		t.Fatal("raw static proof was accepted in place of the final runtime binding")
	}
	if err := ValidateServiceTierCanonicalizationArchive(artifactDir, "luban", execution, []ProviderRoundEvidence{round}); err == nil {
		t.Fatal("Codex-only canonicalization receipt was accepted for Luban")
	}
}
