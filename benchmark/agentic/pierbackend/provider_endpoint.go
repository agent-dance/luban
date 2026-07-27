package pierbackend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

// preflightProviderEndpoint performs a credential-free, direct TLS handshake
// with the exact endpoint authority frozen in the formal manifest. net.Dialer
// is used deliberately: HTTP(S)_PROXY and other application proxy settings do
// not participate in this attestation.
func preflightProviderEndpoint(ctx context.Context, endpoint harness.ProviderEndpointSpec, configuredOrigin string) (harness.ProviderEndpointSnapshot, error) {
	if endpoint != harness.FormalProviderEndpoint() || configuredOrigin != endpoint.ApprovedOrigin {
		return harness.ProviderEndpointSnapshot{}, errors.New("provider TLS preflight is not bound to the preregistered endpoint")
	}
	return dialProviderEndpointTLS(ctx, endpoint, nil)
}

// dialProviderEndpointTLS accepts an explicit root pool only so focused tests
// can use a local certificate authority. Production always passes nil and
// therefore uses the platform WebPKI roots.
func dialProviderEndpointTLS(ctx context.Context, endpoint harness.ProviderEndpointSpec, roots *x509.CertPool) (harness.ProviderEndpointSnapshot, error) {
	origin, err := parseProviderEndpointOrigin(endpoint)
	if err != nil {
		return harness.ProviderEndpointSnapshot{}, err
	}
	hostname := origin.Hostname()
	port := origin.Port()
	if port == "" {
		port = "443"
	}
	return dialProviderEndpointTLSAddress(ctx, endpoint, roots, net.JoinHostPort(hostname, port))
}

func dialProviderEndpointTLSAddress(ctx context.Context, endpoint harness.ProviderEndpointSpec, roots *x509.CertPool, address string) (harness.ProviderEndpointSnapshot, error) {
	origin, err := parseProviderEndpointOrigin(endpoint)
	if err != nil {
		return harness.ProviderEndpointSnapshot{}, err
	}
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return harness.ProviderEndpointSnapshot{}, fmt.Errorf("connect provider TLS endpoint: %w", err)
	}
	tlsConnection := tls.Client(raw, &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: origin.Hostname(),
	})
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return harness.ProviderEndpointSnapshot{}, fmt.Errorf("verify provider TLS endpoint: %w", err)
	}
	defer tlsConnection.Close()
	return projectProviderEndpointTLS(endpoint, tlsConnection.ConnectionState())
}

func parseProviderEndpointOrigin(endpoint harness.ProviderEndpointSpec) (*url.URL, error) {
	digest, err := harness.HashCanonical(endpoint.Semantics)
	if err != nil || digest != endpoint.SemanticsSHA256 {
		return nil, errors.New("provider endpoint semantics hash is invalid")
	}
	origin, err := url.Parse(endpoint.ApprovedOrigin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.Hostname() == "" ||
		origin.User != nil || origin.Opaque != "" || origin.Path != "" || origin.RawPath != "" ||
		origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.String() != endpoint.ApprovedOrigin {
		return nil, errors.New("provider endpoint is not a canonical HTTPS origin")
	}
	return origin, nil
}

func projectProviderEndpointTLS(endpoint harness.ProviderEndpointSpec, state tls.ConnectionState) (harness.ProviderEndpointSnapshot, error) {
	origin, err := parseProviderEndpointOrigin(endpoint)
	if err != nil {
		return harness.ProviderEndpointSnapshot{}, err
	}
	hostname := origin.Hostname()
	if !state.HandshakeComplete || state.ServerName != hostname || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return harness.ProviderEndpointSnapshot{}, errors.New("provider TLS handshake lacks verified peer evidence")
	}
	leaf := state.PeerCertificates[0]
	if len(leaf.Raw) == 0 || len(leaf.RawSubjectPublicKeyInfo) == 0 {
		return harness.ProviderEndpointSnapshot{}, errors.New("provider TLS leaf certificate lacks DER or SPKI evidence")
	}
	if err := leaf.VerifyHostname(hostname); err != nil {
		return harness.ProviderEndpointSnapshot{}, fmt.Errorf("verify provider TLS hostname: %w", err)
	}
	for _, chain := range state.VerifiedChains {
		if len(chain) == 0 || !bytes.Equal(chain[0].Raw, leaf.Raw) {
			return harness.ProviderEndpointSnapshot{}, errors.New("provider TLS verified chain is not bound to the peer leaf")
		}
	}
	leafDigest := sha256.Sum256(leaf.Raw)
	spkiDigest := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	return harness.ProviderEndpointSnapshot{
		ApprovedOrigin:        endpoint.ApprovedOrigin,
		SemanticsSHA256:       endpoint.SemanticsSHA256,
		TLSServerName:         hostname,
		TLSVerified:           true,
		TLSPeerLeafCertSHA256: hex.EncodeToString(leafDigest[:]),
		TLSPeerSPKISHA256:     hex.EncodeToString(spkiDigest[:]),
	}, nil
}

func validateProviderEndpointSnapshot(endpoint harness.ProviderEndpointSpec, snapshot harness.ProviderEndpointSnapshot) error {
	if endpoint != harness.FormalProviderEndpoint() || snapshot.ApprovedOrigin != endpoint.ApprovedOrigin ||
		snapshot.SemanticsSHA256 != endpoint.SemanticsSHA256 || snapshot.TLSServerName != harness.FormalProviderTLSServerName ||
		!snapshot.TLSVerified || !lowerHexSHA256(snapshot.TLSPeerLeafCertSHA256) || !lowerHexSHA256(snapshot.TLSPeerSPKISHA256) {
		return errors.New("provider endpoint snapshot is incomplete or differs from the formal TLS contract")
	}
	return nil
}

// providerEndpointSnapshot exposes only the process-local preflight
// observation stored with the ready backend. Per-round transport observations
// are validated independently by the evidence path.
func (backend *Backend) providerEndpointSnapshot() (harness.ProviderEndpointSnapshot, error) {
	backend.mu.RLock()
	snapshot, endpoint, ready := backend.endpoint, backend.manifest.ProviderEndpoint, backend.ready
	backend.mu.RUnlock()
	if !ready || backend.config.ProviderUpstream != endpoint.ApprovedOrigin {
		return harness.ProviderEndpointSnapshot{}, errors.New("provider endpoint requested before verified preflight")
	}
	if err := validateProviderEndpointSnapshot(endpoint, snapshot); err != nil {
		return harness.ProviderEndpointSnapshot{}, err
	}
	return snapshot, nil
}
