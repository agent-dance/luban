package pierbackend

import (
	"errors"
	"fmt"
	"slices"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const providerEndpointObservationAuthority = "configured-gateway"

// providerEndpointRunBinding is the content-free per-run projection of every
// independently WebPKI-verified TLS peer observed by provider attempts. Leaf or
// SPKI rotation after preflight is legitimate; the receipt therefore aggregates
// distinct peers and their observation windows without pinning them to the
// preflight peer.
type providerEndpointRunBinding struct {
	ApprovedOrigin                 string
	SemanticsSHA256                string
	TLSServerName                  string
	TLSObservationComplete         bool
	TLSBackedRoundCount            int
	TLSAbsentTransportFailureCount int
	PeerObservations               []providerTLSPeerObservation
}

// ValidateArchivedProviderTLSEvidence verifies the content-free endpoint and
// per-attempt TLS projection retained in a raw provider-http-v6 archive. It is
// a path-oriented compatibility wrapper; archive validators that already hold
// the strictly decoded records should use ValidateArchivedProviderTLSRecords
// so the raw ledger is read exactly once.
func ValidateArchivedProviderTLSEvidence(rawPath, expectedRunIdentity string, expected harness.ProviderEndpointSnapshot) error {
	_, err := validateProviderEndpointEvidence(rawPath, expectedRunIdentity, expected)
	return err
}

// ValidateArchivedProviderTLSRecords validates already decoded content-free
// raw records against the preregistered endpoint contract. The contract pins
// origin, semantics, and SNI, while legitimately allowing WebPKI leaf/SPKI
// rotation across requests.
func ValidateArchivedProviderTLSRecords(records []evidenceproxy.Record, expectedRunIdentity string, expected harness.ProviderEndpointSpec) error {
	if expected != harness.FormalProviderEndpoint() {
		return errors.New("archived provider TLS evidence is not bound to the formal endpoint contract")
	}
	_, err := validateProviderEndpointRecords(
		records, expectedRunIdentity, expected.ApprovedOrigin, expected.SemanticsSHA256, harness.FormalProviderTLSServerName,
	)
	return err
}

func validateProviderEndpointEvidence(rawPath, expectedRunIdentity string, expected harness.ProviderEndpointSnapshot) (providerEndpointRunBinding, error) {
	if !lowerHexSHA256(expectedRunIdentity) {
		return providerEndpointRunBinding{}, errors.New("provider endpoint evidence run identity is invalid")
	}
	if expected.ApprovedOrigin != harness.FormalProviderOrigin ||
		expected.SemanticsSHA256 != harness.FormalProviderEndpointSemanticsSHA256 ||
		expected.TLSServerName != harness.FormalProviderTLSServerName ||
		!expected.TLSVerified ||
		!lowerHexSHA256(expected.TLSPeerLeafCertSHA256) ||
		!lowerHexSHA256(expected.TLSPeerSPKISHA256) {
		return providerEndpointRunBinding{}, errors.New("preflight provider endpoint snapshot is not formal or complete")
	}
	records, err := harness.ReadJSONLines[evidenceproxy.Record](rawPath)
	if err != nil {
		return providerEndpointRunBinding{}, err
	}
	return validateProviderEndpointRecords(records, expectedRunIdentity, expected.ApprovedOrigin, expected.SemanticsSHA256, expected.TLSServerName)
}

func validateProviderEndpointRecords(records []evidenceproxy.Record, expectedRunIdentity, approvedOrigin, semanticsSHA256, tlsServerName string) (providerEndpointRunBinding, error) {
	if !lowerHexSHA256(expectedRunIdentity) {
		return providerEndpointRunBinding{}, errors.New("provider endpoint evidence run identity is invalid")
	}
	if len(records) == 0 {
		return providerEndpointRunBinding{}, errors.New("provider endpoint ledger captured no rounds")
	}
	binding := providerEndpointRunBinding{
		ApprovedOrigin: approvedOrigin, SemanticsSHA256: semanticsSHA256,
		TLSServerName: tlsServerName, TLSObservationComplete: true,
	}
	type peerKey struct{ leaf, spki string }
	peerIndexes := make(map[peerKey]int)
	for _, record := range records {
		if record.SchemaVersion != "agentic-bench/provider-http-v6" {
			return providerEndpointRunBinding{}, fmt.Errorf("provider endpoint round %d has unsupported evidence schema", record.Round)
		}
		if record.RunIdentity != expectedRunIdentity {
			return providerEndpointRunBinding{}, fmt.Errorf("provider endpoint round %d belongs to another run", record.Round)
		}
		if record.ApprovedOrigin != approvedOrigin || record.SemanticsSHA256 != semanticsSHA256 {
			return providerEndpointRunBinding{}, fmt.Errorf("provider endpoint round %d changed the approved gateway semantics", record.Round)
		}
		if !record.ProviderAttemptStarted {
			continue
		}
		tlsPresent := record.TLSServerName != "" || record.TLSVerified || !record.TLSObservedAt.IsZero() ||
			record.TLSPeerLeafCertSHA256 != "" || record.TLSPeerSPKISHA256 != ""
		if !tlsPresent {
			if record.Transport != "http_sse" || record.Disposition != "provider_infra_exclusion" ||
				record.ErrorCode != "upstream_transport" || !record.UpstreamHeadersAt.IsZero() || record.HTTPStatus != 0 || record.ResponseBytes != 0 {
				return providerEndpointRunBinding{}, fmt.Errorf("provider endpoint round %d lacks TLS evidence outside a pre-response transport failure", record.Round)
			}
			binding.TLSAbsentTransportFailureCount++
			continue
		}
		if record.TLSServerName != tlsServerName || !record.TLSVerified ||
			!lowerHexSHA256(record.TLSPeerLeafCertSHA256) || !lowerHexSHA256(record.TLSPeerSPKISHA256) {
			return providerEndpointRunBinding{}, fmt.Errorf("provider endpoint round %d has incomplete or unverified TLS peer evidence", record.Round)
		}
		observedAt := record.TLSObservedAt
		if observedAt.IsZero() {
			return providerEndpointRunBinding{}, fmt.Errorf("provider endpoint round %d lacks TLS observation time", record.Round)
		}
		binding.TLSBackedRoundCount++
		key := peerKey{leaf: record.TLSPeerLeafCertSHA256, spki: record.TLSPeerSPKISHA256}
		if index, exists := peerIndexes[key]; exists {
			observation := &binding.PeerObservations[index]
			observation.RoundCount++
			if observedAt.Before(observation.FirstObservedAt) {
				observation.FirstObservedAt = observedAt
			}
			if observedAt.After(observation.LastObservedAt) {
				observation.LastObservedAt = observedAt
			}
			continue
		}
		peerIndexes[key] = len(binding.PeerObservations)
		binding.PeerObservations = append(binding.PeerObservations, providerTLSPeerObservation{
			TLSPeerLeafCertSHA256: key.leaf, TLSPeerSPKISHA256: key.spki,
			FirstObservedAt: observedAt, LastObservedAt: observedAt, RoundCount: 1,
		})
	}
	if binding.TLSBackedRoundCount == 0 && binding.TLSAbsentTransportFailureCount == 0 {
		return providerEndpointRunBinding{}, errors.New("provider endpoint ledger has no started provider attempt")
	}
	slices.SortFunc(binding.PeerObservations, func(left, right providerTLSPeerObservation) int {
		if left.TLSPeerLeafCertSHA256 < right.TLSPeerLeafCertSHA256 {
			return -1
		}
		if left.TLSPeerLeafCertSHA256 > right.TLSPeerLeafCertSHA256 {
			return 1
		}
		if left.TLSPeerSPKISHA256 < right.TLSPeerSPKISHA256 {
			return -1
		}
		if left.TLSPeerSPKISHA256 > right.TLSPeerSPKISHA256 {
			return 1
		}
		return 0
	})
	return binding, nil
}

func validProviderTLSPeerObservations(observations []providerTLSPeerObservation, backedRounds int) bool {
	total := 0
	previousLeaf, previousSPKI := "", ""
	for _, observation := range observations {
		if !lowerHexSHA256(observation.TLSPeerLeafCertSHA256) || !lowerHexSHA256(observation.TLSPeerSPKISHA256) ||
			observation.FirstObservedAt.IsZero() || observation.LastObservedAt.Before(observation.FirstObservedAt) || observation.RoundCount <= 0 {
			return false
		}
		if previousLeaf > observation.TLSPeerLeafCertSHA256 ||
			(previousLeaf == observation.TLSPeerLeafCertSHA256 && previousSPKI >= observation.TLSPeerSPKISHA256) {
			return false
		}
		previousLeaf, previousSPKI = observation.TLSPeerLeafCertSHA256, observation.TLSPeerSPKISHA256
		total += observation.RoundCount
	}
	return total == backedRounds && (backedRounds == 0) == (len(observations) == 0)
}
