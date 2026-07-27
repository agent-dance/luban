package pierbackend

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestValidateProviderEndpointEvidenceAggregatesVerifiedPeersWithoutPinningPreflightCertificate(t *testing.T) {
	expected := providerEndpointFixtureSnapshot()
	records := []evidenceproxy.Record{
		providerEndpointFixtureRecord(0, expected),
		providerEndpointFixtureRecord(1, expected),
	}
	// A legitimate certificate rotation is a second verified observation, not
	// endpoint drift. The authority remains the exact origin, semantics, and SNI.
	records[1].TLSPeerLeafCertSHA256 = strings.Repeat("c", 64)
	records[1].TLSPeerSPKISHA256 = strings.Repeat("d", 64)
	path := filepath.Join(t.TempDir(), "provider.jsonl")
	writeJSONLines(t, path, records)
	binding, err := validateProviderEndpointEvidence(path, fixtureRunIdentity, expected)
	if err != nil {
		t.Fatal(err)
	}
	if binding.TLSBackedRoundCount != 2 || binding.TLSAbsentTransportFailureCount != 0 || binding.ApprovedOrigin != expected.ApprovedOrigin ||
		binding.SemanticsSHA256 != expected.SemanticsSHA256 || binding.TLSServerName != expected.TLSServerName ||
		!binding.TLSObservationComplete || len(binding.PeerObservations) != 2 ||
		binding.PeerObservations[0].TLSPeerLeafCertSHA256 != expected.TLSPeerLeafCertSHA256 ||
		binding.PeerObservations[1].TLSPeerLeafCertSHA256 != records[1].TLSPeerLeafCertSHA256 {
		t.Fatalf("provider endpoint run binding = %#v", binding)
	}
}

func TestValidateProviderEndpointEvidenceRejectsEndpointAndTLSDrift(t *testing.T) {
	expected := providerEndpointFixtureSnapshot()
	tests := map[string]func(*evidenceproxy.Record){
		"origin":       func(value *evidenceproxy.Record) { value.ApprovedOrigin = "https://api.openai.com" },
		"semantics":    func(value *evidenceproxy.Record) { value.SemanticsSHA256 = strings.Repeat("9", 64) },
		"server name":  func(value *evidenceproxy.Record) { value.TLSServerName = "api.openai.com" },
		"verification": func(value *evidenceproxy.Record) { value.TLSVerified = false },
		"leaf absent":  func(value *evidenceproxy.Record) { value.TLSPeerLeafCertSHA256 = "" },
		"spki invalid": func(value *evidenceproxy.Record) { value.TLSPeerSPKISHA256 = strings.Repeat("A", 64) },
		"time absent":  func(value *evidenceproxy.Record) { value.TLSObservedAt = time.Time{} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			record := providerEndpointFixtureRecord(0, expected)
			mutate(&record)
			path := filepath.Join(t.TempDir(), "provider.jsonl")
			writeJSONLines(t, path, []evidenceproxy.Record{record})
			if _, err := validateProviderEndpointEvidence(path, fixtureRunIdentity, expected); err == nil {
				t.Fatal("provider endpoint drift was accepted")
			}
		})
	}
}

func TestValidateArchivedProviderTLSRecordsFailsClosedOnRawProjectionTamper(t *testing.T) {
	expectedSnapshot := providerEndpointFixtureSnapshot()
	expectedEndpoint := harness.FormalProviderEndpoint()
	base := providerEndpointFixtureRecord(0, expectedSnapshot)
	if err := ValidateArchivedProviderTLSRecords([]evidenceproxy.Record{base}, fixtureRunIdentity, expectedEndpoint); err != nil {
		t.Fatalf("valid archived TLS projection: %v", err)
	}

	tests := map[string]func(*evidenceproxy.Record){
		"schema":       func(value *evidenceproxy.Record) { value.SchemaVersion = "agentic-bench/provider-http-v5" },
		"run identity": func(value *evidenceproxy.Record) { value.RunIdentity = strings.Repeat("9", 64) },
		"origin":       func(value *evidenceproxy.Record) { value.ApprovedOrigin = "https://api.openai.com" },
		"semantics":    func(value *evidenceproxy.Record) { value.SemanticsSHA256 = strings.Repeat("9", 64) },
		"server name":  func(value *evidenceproxy.Record) { value.TLSServerName = "api.openai.com" },
		"verification": func(value *evidenceproxy.Record) { value.TLSVerified = false },
		"observation":  func(value *evidenceproxy.Record) { value.TLSObservedAt = time.Time{} },
		"leaf":         func(value *evidenceproxy.Record) { value.TLSPeerLeafCertSHA256 = strings.Repeat("A", 64) },
		"SPKI":         func(value *evidenceproxy.Record) { value.TLSPeerSPKISHA256 = "" },
		"not started":  func(value *evidenceproxy.Record) { value.ProviderAttemptStarted = false },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if err := ValidateArchivedProviderTLSRecords([]evidenceproxy.Record{candidate}, fixtureRunIdentity, expectedEndpoint); err == nil {
				t.Fatal("tampered archived TLS projection was accepted")
			}
		})
	}

	driftedEndpoint := expectedEndpoint
	driftedEndpoint.Semantics.TLSRequired = false
	if err := ValidateArchivedProviderTLSRecords([]evidenceproxy.Record{base}, fixtureRunIdentity, driftedEndpoint); err == nil {
		t.Fatal("non-formal endpoint contract was accepted")
	}
}

func TestValidateProviderEndpointEvidencePreservesPreResponseTransportAbsence(t *testing.T) {
	expected := providerEndpointFixtureSnapshot()
	record := providerEndpointFixtureRecord(0, expected)
	record.TLSServerName = ""
	record.TLSVerified = false
	record.TLSPeerLeafCertSHA256 = ""
	record.TLSPeerSPKISHA256 = ""
	record.TLSObservedAt = time.Time{}
	record.ErrorCode = "upstream_transport"
	record.Disposition = "provider_infra_exclusion"
	path := filepath.Join(t.TempDir(), "provider.jsonl")
	writeJSONLines(t, path, []evidenceproxy.Record{record})
	binding, err := validateProviderEndpointEvidence(path, fixtureRunIdentity, expected)
	if err != nil {
		t.Fatal(err)
	}
	if binding.TLSBackedRoundCount != 0 || binding.TLSAbsentTransportFailureCount != 1 || len(binding.PeerObservations) != 0 || !binding.TLSObservationComplete {
		t.Fatalf("TLS transport-absence binding = %#v", binding)
	}

	record.HTTPStatus = 502
	path = filepath.Join(t.TempDir(), "provider.jsonl")
	writeJSONLines(t, path, []evidenceproxy.Record{record})
	if _, err := validateProviderEndpointEvidence(path, fixtureRunIdentity, expected); err == nil {
		t.Fatal("post-response TLS evidence absence was accepted")
	}

	// A lone observation timestamp is not an admissible absence receipt. It is
	// partial TLS evidence and must fail closed rather than being discarded.
	record.HTTPStatus = 0
	record.TLSObservedAt = time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	path = filepath.Join(t.TempDir(), "provider.jsonl")
	writeJSONLines(t, path, []evidenceproxy.Record{record})
	if err := ValidateArchivedProviderTLSEvidence(path, fixtureRunIdentity, expected); err == nil {
		t.Fatal("partial archived TLS observation was accepted as transport absence")
	}
}

func providerEndpointFixtureSnapshot() harness.ProviderEndpointSnapshot {
	return harness.ProviderEndpointSnapshot{
		ApprovedOrigin: harness.FormalProviderOrigin, SemanticsSHA256: harness.FormalProviderEndpointSemanticsSHA256,
		TLSServerName: harness.FormalProviderTLSServerName, TLSVerified: true,
		TLSPeerLeafCertSHA256: strings.Repeat("a", 64), TLSPeerSPKISHA256: strings.Repeat("b", 64),
	}
}

func providerEndpointFixtureRecord(round int, expected harness.ProviderEndpointSnapshot) evidenceproxy.Record {
	return evidenceproxy.Record{
		SchemaVersion: "agentic-bench/provider-http-v6", Round: round, RunIdentity: fixtureRunIdentity,
		ProviderAttemptStarted: true, Transport: "http_sse", ApprovedOrigin: expected.ApprovedOrigin,
		SemanticsSHA256: expected.SemanticsSHA256, TLSServerName: expected.TLSServerName,
		TLSVerified: true, TLSPeerLeafCertSHA256: expected.TLSPeerLeafCertSHA256,
		TLSPeerSPKISHA256: expected.TLSPeerSPKISHA256,
		TLSObservedAt:     time.Date(2026, 7, 26, 12, 0, round, 0, time.UTC),
	}
}
