package pierbackend

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const (
	serviceTierCanonicalizationRepresentation = "client_canonicalized_default"
	serviceTierCanonicalizationReceiptName    = "service-tier-canonicalization-receipt.json"
)

type serviceTierCanonicalizationBindingPayload struct {
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

type serviceTierCanonicalizationBindingReceipt struct {
	serviceTierCanonicalizationBindingPayload
	BindingSHA256 string `json:"binding_sha256"`
}

type serviceTierCanonicalizationRunBinding struct {
	Receipt       serviceTierCanonicalizationBindingReceipt
	ReceiptSHA256 string
}

func writeServiceTierCanonicalizationReceipt(
	pierDir, rawEvidence, runIdentity, effectiveReceiptSHA, sandboxCanarySHA string,
	invocation harness.AgentInvocation, adapter adapterBinding, bundle codexBundleBinding, canonicalCanary formalCodexCanonicalCanaryBinding, effective effectiveArgvReceipt,
) (serviceTierCanonicalizationBindingReceipt, string, error) {
	if invocation.Agent.ID != "codex" {
		return serviceTierCanonicalizationBindingReceipt{}, "", errors.New("service-tier canonicalization receipt is only valid for frozen Codex wire omission")
	}
	staticEvidence, err := validateServiceTierCanonicalizationStaticEvidence(rawEvidence, runIdentity)
	if err != nil {
		return serviceTierCanonicalizationBindingReceipt{}, "", err
	}
	rawEvidenceSHA, err := harness.HashFile(rawEvidence)
	if err != nil {
		return serviceTierCanonicalizationBindingReceipt{}, "", err
	}
	if invocation.Agent.BinarySHA256 != Codex0145BinarySHA256 ||
		bundle.ManifestSHA256 != CodexBundleManifestSHA256 || bundle.TreeSHA256 != CodexBundleTreeSHA256 ||
		adapter.SHA256 == "" || !lowerHexSHA256(adapter.SHA256) ||
		canonicalCanary.Generation != formalCodexV8CanaryGeneration || !lowerHexSHA256(canonicalCanary.SHA256) ||
		!lowerHexSHA256(effective.SourceCommandArgvSHA256) || !lowerHexSHA256(effective.EffectiveArgvSHA256) ||
		!lowerHexSHA256(effectiveReceiptSHA) || !lowerHexSHA256(sandboxCanarySHA) ||
		effective.SemanticProjection.ServiceTier != harness.FormalServiceTier {
		return serviceTierCanonicalizationBindingReceipt{}, "", errors.New("service-tier canonicalization binding inputs are incomplete")
	}
	expectedStaticProof, err := evidenceproxy.ServiceTierCanonicalizationStaticProof(evidenceproxy.Config{
		AgentID: "codex", RegisteredBinarySHA256: invocation.Agent.BinarySHA256,
		FrozenBundleManifestSHA256: bundle.ManifestSHA256, FrozenBundleTreeSHA256: bundle.TreeSHA256,
		FrozenCanonicalCanaryReceiptSHA256: canonicalCanary.SHA256,
		AdapterSHA256:                      adapter.SHA256, AdapterVersion: PinnedAdapterVersion,
		SourceCommandArgvSHA256: effective.SourceCommandArgvSHA256,
	})
	if err != nil || expectedStaticProof != staticEvidence.StaticProofSHA256 {
		return serviceTierCanonicalizationBindingReceipt{}, "", errors.New("service-tier canonicalization static proof differs from its frozen sources")
	}
	payload := serviceTierCanonicalizationBindingPayload{
		SchemaVersion:  harness.ServiceTierCanonicalizationBindingSchemaVersion,
		Representation: serviceTierCanonicalizationRepresentation, ClientAgentID: "codex", ClientRuntimeVersion: "0.145.0",
		RunIdentity: runIdentity, RegisteredBinarySHA256: invocation.Agent.BinarySHA256,
		FrozenBundleManifestSHA256: bundle.ManifestSHA256, FrozenBundleTreeSHA256: bundle.TreeSHA256,
		AdapterSHA256: adapter.SHA256, AdapterVersion: PinnedAdapterVersion,
		SourceCommandArgvSHA256: effective.SourceCommandArgvSHA256, EffectiveArgvSHA256: effective.EffectiveArgvSHA256,
		EffectiveArgvReceiptSHA256: effectiveReceiptSHA, SandboxCanaryReceiptSHA256: sandboxCanarySHA,
		CanonicalCanaryGeneration: canonicalCanary.Generation, FrozenCanonicalCanaryReceiptSHA256: canonicalCanary.SHA256, RawProviderEvidenceSHA256: rawEvidenceSHA,
		TransformationEvidenceSHA256:  staticEvidence.TransformationEvidenceSHA256,
		TransformedProviderRoundCount: staticEvidence.TransformedProviderRoundCount,
		StaticProofSHA256:             staticEvidence.StaticProofSHA256,
	}
	bindingSHA, err := harness.HashCanonical(payload)
	if err != nil {
		return serviceTierCanonicalizationBindingReceipt{}, "", err
	}
	receipt := serviceTierCanonicalizationBindingReceipt{serviceTierCanonicalizationBindingPayload: payload, BindingSHA256: bindingSHA}
	path := filepath.Join(pierDir, serviceTierCanonicalizationReceiptName)
	if err := harness.WriteJSONAtomic(path, receipt, 0o644); err != nil {
		return serviceTierCanonicalizationBindingReceipt{}, "", err
	}
	fileSHA, err := harness.HashFile(path)
	if err != nil {
		return serviceTierCanonicalizationBindingReceipt{}, "", err
	}
	return receipt, fileSHA, nil
}

type serviceTierCanonicalizationStaticEvidence struct {
	StaticProofSHA256             string
	TransformationEvidenceSHA256  string
	TransformedProviderRoundCount int
}

func validateServiceTierCanonicalizationStaticEvidence(rawPath, expectedRunIdentity string) (serviceTierCanonicalizationStaticEvidence, error) {
	records, err := harness.ReadJSONLines[evidenceproxy.Record](rawPath)
	if err != nil {
		return serviceTierCanonicalizationStaticEvidence{}, err
	}
	if len(records) == 0 {
		return serviceTierCanonicalizationStaticEvidence{}, errors.New("Codex service-tier canonicalization proof has no provider rounds")
	}
	proof := ""
	type transformationProjection struct {
		Round            int    `json:"round"`
		OriginalBodySHA  string `json:"original_body_sha256"`
		ForwardedBodySHA string `json:"forwarded_body_sha256"`
		ProofSHA         string `json:"proof_sha256"`
	}
	transformations := make([]transformationProjection, 0, len(records))
	for _, record := range records {
		if record.SchemaVersion != "agentic-bench/provider-http-v6" ||
			record.RunIdentity != expectedRunIdentity || record.ClientAgentID != "codex" ||
			record.RequestedServiceTierPresent || record.RequestedServiceTier != "" ||
			record.RequestedServiceTierCanonical != harness.FormalServiceTier ||
			record.RequestedServiceTierRepresentation != serviceTierCanonicalizationRepresentation ||
			!lowerHexSHA256(record.ClientCanonicalizationStaticProofSHA256) {
			return serviceTierCanonicalizationStaticEvidence{}, fmt.Errorf("provider round %d lacks the frozen Codex service-tier omission proof", record.Round)
		}
		if proof == "" {
			proof = record.ClientCanonicalizationStaticProofSHA256
		} else if proof != record.ClientCanonicalizationStaticProofSHA256 {
			return serviceTierCanonicalizationStaticEvidence{}, fmt.Errorf("provider round %d changed the Codex service-tier static proof", record.Round)
		}
		if !lowerHexSHA256(record.OriginalRequestBodySHA256) || !lowerHexSHA256(record.ForwardedRequestBodySHA256) ||
			!lowerHexSHA256(record.OriginalRequestCanonicalSHA256) || !lowerHexSHA256(record.ForwardedRequestCanonicalSHA256) ||
			!lowerHexSHA256(record.OriginalRequestWithoutServiceTierSHA256) || !lowerHexSHA256(record.ForwardedRequestWithoutServiceTierSHA256) ||
			record.OriginalRequestWithoutServiceTierSHA256 != record.ForwardedRequestWithoutServiceTierSHA256 ||
			record.OriginalRequestBodySHA256 == record.ForwardedRequestBodySHA256 || record.OriginalRequestCanonicalSHA256 == record.ForwardedRequestCanonicalSHA256 || record.ForwardedRequestBytes <= 0 ||
			record.OriginalServiceTierPresent || record.OriginalServiceTier != "" ||
			!record.ForwardedServiceTierPresent || record.ForwardedServiceTier != harness.FormalServiceTier ||
			record.ServiceTierTransformation != "inject_explicit_default" || !record.ServiceTierTransformationExactDiff ||
			!lowerHexSHA256(record.ServiceTierTransformationProofSHA256) {
			return serviceTierCanonicalizationStaticEvidence{}, fmt.Errorf("provider round %d lacks exact controller service-tier normalization evidence", record.Round)
		}
		if err := evidenceproxy.ValidateServiceTierTransformationProof(record); err != nil {
			return serviceTierCanonicalizationStaticEvidence{}, fmt.Errorf("provider round %d has an unbound controller service-tier transformation proof: %w", record.Round, err)
		}
		transformations = append(transformations, transformationProjection{
			Round: record.Round, OriginalBodySHA: record.OriginalRequestBodySHA256,
			ForwardedBodySHA: record.ForwardedRequestBodySHA256, ProofSHA: record.ServiceTierTransformationProofSHA256,
		})
	}
	slices.SortFunc(transformations, func(left, right transformationProjection) int { return left.Round - right.Round })
	transformationSHA, err := harness.HashCanonical(transformations)
	if err != nil {
		return serviceTierCanonicalizationStaticEvidence{}, err
	}
	return serviceTierCanonicalizationStaticEvidence{
		StaticProofSHA256: proof, TransformationEvidenceSHA256: transformationSHA,
		TransformedProviderRoundCount: len(transformations),
	}, nil
}
