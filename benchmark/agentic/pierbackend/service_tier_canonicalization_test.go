package pierbackend

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
)

func TestValidateServiceTierCanonicalizationStaticEvidenceRequiresExactControllerDiff(t *testing.T) {
	record := func(round int) evidenceproxy.Record {
		value := evidenceproxy.Record{
			SchemaVersion: "agentic-bench/provider-http-v6", Round: round, RunIdentity: fixtureRunIdentity,
			ClientAgentID: "codex", RequestedServiceTierCanonical: "default",
			RequestedServiceTierRepresentation:      "client_canonicalized_default",
			ClientCanonicalizationStaticProofSHA256: strings.Repeat("a", 64),
			OriginalRequestBodySHA256:               strings.Repeat("1", 64), ForwardedRequestBodySHA256: strings.Repeat("2", 64),
			OriginalRequestCanonicalSHA256: strings.Repeat("3", 64), ForwardedRequestCanonicalSHA256: strings.Repeat("4", 64),
			OriginalRequestWithoutServiceTierSHA256: strings.Repeat("5", 64), ForwardedRequestWithoutServiceTierSHA256: strings.Repeat("5", 64),
			ForwardedServiceTierPresent: true, ForwardedServiceTier: "default", ForwardedRequestBytes: 100,
			ServiceTierTransformation: "inject_explicit_default", ServiceTierTransformationExactDiff: true,
		}
		proof, err := evidenceproxy.ServiceTierTransformationProofSHA256(value)
		if err != nil {
			t.Fatal(err)
		}
		value.ServiceTierTransformationProofSHA256 = proof
		return value
	}
	records := []evidenceproxy.Record{record(1), record(0)}
	path := filepath.Join(t.TempDir(), "provider.jsonl")
	writeJSONLines(t, path, records)
	evidence, err := validateServiceTierCanonicalizationStaticEvidence(path, fixtureRunIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.StaticProofSHA256 != strings.Repeat("a", 64) || !lowerHexSHA256(evidence.TransformationEvidenceSHA256) || evidence.TransformedProviderRoundCount != 2 {
		t.Fatalf("canonicalization evidence = %#v", evidence)
	}

	for name, mutate := range map[string]func(*evidenceproxy.Record){
		"body unchanged": func(value *evidenceproxy.Record) { value.ForwardedRequestBodySHA256 = value.OriginalRequestBodySHA256 },
		"canonical unchanged": func(value *evidenceproxy.Record) {
			value.ForwardedRequestCanonicalSHA256 = value.OriginalRequestCanonicalSHA256
		},
		"non-tier drift": func(value *evidenceproxy.Record) {
			value.ForwardedRequestWithoutServiceTierSHA256 = strings.Repeat("6", 64)
		},
		"forwarded tier omitted":  func(value *evidenceproxy.Record) { value.ForwardedServiceTierPresent = false },
		"unproven transformation": func(value *evidenceproxy.Record) { value.ServiceTierTransformationExactDiff = false },
		"unbound proof digest": func(value *evidenceproxy.Record) {
			value.ServiceTierTransformationProofSHA256 = strings.Repeat("f", 64)
		},
		"old evidence schema": func(value *evidenceproxy.Record) { value.SchemaVersion = "agentic-bench/provider-http-v5" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := append([]evidenceproxy.Record(nil), records...)
			mutate(&invalid[0])
			invalidPath := filepath.Join(t.TempDir(), "provider.jsonl")
			writeJSONLines(t, invalidPath, invalid)
			if _, err := validateServiceTierCanonicalizationStaticEvidence(invalidPath, fixtureRunIdentity); err == nil {
				t.Fatal("invalid controller transformation evidence was accepted")
			}
		})
	}
}
