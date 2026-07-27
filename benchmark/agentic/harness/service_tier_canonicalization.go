package harness

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

const (
	ServiceTierCanonicalizationEvidenceSchemaVersion = "agentic-bench/service-tier-canonicalization-evidence-v1"
	ServiceTierCanonicalizationBindingSchemaVersion  = "agentic-bench/service-tier-canonicalization-binding-v2"
)

type archivedServiceTierCanonicalizationPayload struct {
	SchemaVersion                      string `json:"schema_version"`
	Representation                     string `json:"representation"`
	ClientAgentID                      string `json:"client_agent_id"`
	ClientRuntimeVersion               string `json:"client_runtime_version"`
	RunIdentity                        string `json:"run_identity"`
	RegisteredBinarySHA256             string `json:"registered_binary_sha256"`
	FrozenBundleManifestSHA256         string `json:"frozen_bundle_manifest_sha256"`
	FrozenBundleTreeSHA256             string `json:"frozen_bundle_tree_sha256"`
	AdapterSHA256                      string `json:"adapter_sha256"`
	AdapterVersion                     string `json:"adapter_version"`
	SourceCommandArgvSHA256            string `json:"source_command_argv_sha256"`
	EffectiveArgvSHA256                string `json:"effective_argv_sha256"`
	EffectiveArgvReceiptSHA256         string `json:"effective_argv_receipt_sha256"`
	SandboxCanaryReceiptSHA256         string `json:"sandbox_canary_receipt_sha256"`
	CanonicalCanaryGeneration          string `json:"canonical_canary_generation"`
	FrozenCanonicalCanaryReceiptSHA256 string `json:"frozen_canonical_canary_receipt_sha256"`
	RawProviderEvidenceSHA256          string `json:"raw_provider_evidence_sha256"`
	TransformationEvidenceSHA256       string `json:"transformation_evidence_sha256"`
	TransformedProviderRoundCount      int    `json:"transformed_provider_round_count"`
	StaticProofSHA256                  string `json:"static_proof_sha256"`
}

type archivedServiceTierCanonicalizationReceipt struct {
	archivedServiceTierCanonicalizationPayload
	BindingSHA256 string `json:"binding_sha256"`
}

// ValidateServiceTierCanonicalizationArchive independently revalidates the
// public two-stage Codex receipt and binds every normalized round to its final
// runtime proof. Luban must carry no Codex-only canonicalization artifact.
func ValidateServiceTierCanonicalizationArchive(artifactDir, agentID string, execution AgentExecution, rounds []ProviderRoundEvidence) error {
	evidence := execution.ServiceTierCanonicalization
	if agentID != "codex" {
		if evidence != (ServiceTierCanonicalizationEvidence{}) {
			return errors.New("non-Codex execution contains Codex service-tier canonicalization evidence")
		}
		for _, round := range rounds {
			if round.ClientCanonicalizationProofSHA256 != "" {
				return fmt.Errorf("provider round %d contains a Codex-only canonicalization binding", round.Round)
			}
		}
		return nil
	}
	if evidence.SchemaVersion != ServiceTierCanonicalizationEvidenceSchemaVersion || evidence.Representation != ServiceTierEncodingClientCanonical ||
		evidence.ReceiptRelativePath != "pier/service-tier-canonicalization-receipt.json" ||
		!hex64Pattern.MatchString(evidence.ReceiptSHA256) || !hex64Pattern.MatchString(evidence.BindingSHA256) ||
		!hex64Pattern.MatchString(evidence.StaticProofSHA256) || !hex64Pattern.MatchString(evidence.TransformationEvidenceSHA256) ||
		evidence.TransformedRoundCount != uint64(len(rounds)) || len(rounds) == 0 {
		return errors.New("Codex execution lacks complete service-tier canonicalization evidence")
	}
	receiptPath, err := artifactPath(artifactDir, evidence.ReceiptRelativePath)
	if err != nil {
		return err
	}
	receiptSHA, err := HashFile(receiptPath)
	if err != nil || receiptSHA != evidence.ReceiptSHA256 {
		return errors.New("service-tier canonicalization receipt digest mismatch")
	}
	raw, err := os.ReadFile(receiptPath)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var receipt archivedServiceTierCanonicalizationReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return fmt.Errorf("decode service-tier canonicalization receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("service-tier canonicalization receipt contains trailing JSON")
	}
	computedBinding, err := HashCanonical(receipt.archivedServiceTierCanonicalizationPayload)
	if err != nil {
		return err
	}
	if receipt.SchemaVersion != ServiceTierCanonicalizationBindingSchemaVersion ||
		receipt.Representation != ServiceTierEncodingClientCanonical || receipt.ClientAgentID != "codex" ||
		receipt.ClientRuntimeVersion != "0.145.0" || receipt.AdapterVersion != "2.4.0" ||
		receipt.CanonicalCanaryGeneration != FormalExecutionCanaryGeneration ||
		receipt.RunIdentity != execution.EvidenceRunIdentity || receipt.RawProviderEvidenceSHA256 != execution.ProviderEvidence.RawEvidenceSHA256 ||
		receipt.TransformedProviderRoundCount != len(rounds) || receipt.BindingSHA256 != computedBinding ||
		receipt.BindingSHA256 != evidence.BindingSHA256 || receipt.StaticProofSHA256 != evidence.StaticProofSHA256 ||
		receipt.TransformationEvidenceSHA256 != evidence.TransformationEvidenceSHA256 {
		return errors.New("service-tier canonicalization receipt does not bind the execution")
	}
	for _, digest := range []string{
		receipt.RegisteredBinarySHA256, receipt.FrozenBundleManifestSHA256, receipt.FrozenBundleTreeSHA256,
		receipt.AdapterSHA256, receipt.SourceCommandArgvSHA256, receipt.EffectiveArgvSHA256,
		receipt.EffectiveArgvReceiptSHA256, receipt.SandboxCanaryReceiptSHA256, receipt.FrozenCanonicalCanaryReceiptSHA256,
	} {
		if !hex64Pattern.MatchString(digest) {
			return errors.New("service-tier canonicalization receipt has an invalid frozen input digest")
		}
	}
	type transformationProjection struct {
		Round            int    `json:"round"`
		OriginalBodySHA  string `json:"original_body_sha256"`
		ForwardedBodySHA string `json:"forwarded_body_sha256"`
		ProofSHA         string `json:"proof_sha256"`
	}
	transformations := make([]transformationProjection, 0, len(rounds))
	for _, round := range rounds {
		if round.ClientAgentID != "codex" || round.ClientCanonicalizationProofSHA256 != evidence.BindingSHA256 {
			return fmt.Errorf("provider round %d is not bound to the final Codex canonicalization receipt", round.Round)
		}
		transformations = append(transformations, transformationProjection{
			Round: round.Round, OriginalBodySHA: round.OriginalRequestBodySHA256,
			ForwardedBodySHA: round.ForwardedRequestBodySHA256, ProofSHA: round.ServiceTierTransformationProofSHA256,
		})
	}
	slices.SortFunc(transformations, func(left, right transformationProjection) int { return cmp.Compare(left.Round, right.Round) })
	transformationSHA, err := HashCanonical(transformations)
	if err != nil {
		return err
	}
	if transformationSHA != evidence.TransformationEvidenceSHA256 {
		return errors.New("normalized service-tier transformations do not match the final binding receipt")
	}
	return nil
}
