package pierbackend

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestDialProviderEndpointTLSProjectsLocalVerifiedPeerWithoutHTTPProxy(t *testing.T) {
	origin, roots, leaf, observedServerName := startLocalProviderTLSServer(t)
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	endpoint := harness.FormalProviderEndpoint()
	endpoint.ApprovedOrigin = origin
	snapshot, err := dialProviderEndpointTLS(context.Background(), endpoint, roots)
	if err != nil {
		t.Fatal(err)
	}
	parsedOrigin, err := url.Parse(origin)
	if err != nil {
		t.Fatal(err)
	}
	leafDigest := sha256.Sum256(leaf.Raw)
	spkiDigest := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	if snapshot.ApprovedOrigin != endpoint.ApprovedOrigin || snapshot.SemanticsSHA256 != endpoint.SemanticsSHA256 ||
		snapshot.TLSServerName != parsedOrigin.Hostname() || !snapshot.TLSVerified ||
		snapshot.TLSPeerLeafCertSHA256 != hex.EncodeToString(leafDigest[:]) ||
		snapshot.TLSPeerSPKISHA256 != hex.EncodeToString(spkiDigest[:]) {
		t.Fatalf("provider TLS snapshot = %#v", snapshot)
	}
	select {
	case serverName := <-observedServerName:
		if serverName != parsedOrigin.Hostname() {
			t.Fatalf("TLS SNI = %q, want %q", serverName, parsedOrigin.Hostname())
		}
	case <-time.After(time.Second):
		t.Fatal("local TLS server did not observe the preflight handshake")
	}
}

func startLocalProviderTLSServer(t *testing.T) (string, *x509.CertPool, *x509.Certificate, <-chan string) {
	t.Helper()
	now := time.Now()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "provider endpoint test root"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, root, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER, rootDER}, PrivateKey: leafKey}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	observedServerName := make(chan string, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		tlsConnection, ok := connection.(*tls.Conn)
		if !ok || tlsConnection.Handshake() != nil {
			return
		}
		observedServerName <- tlsConnection.ConnectionState().ServerName
	}()
	roots := x509.NewCertPool()
	roots.AddCert(root)
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return "https://localhost:" + port, roots, leaf, observedServerName
}

func TestProjectProviderEndpointTLSRejectsIncompleteOrUnboundEvidence(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	origin, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := harness.FormalProviderEndpoint()
	endpoint.ApprovedOrigin = server.URL
	leaf := server.Certificate()
	validState := func() tls.ConnectionState {
		return tls.ConnectionState{
			HandshakeComplete: true,
			ServerName:        origin.Hostname(),
			PeerCertificates:  []*x509.Certificate{leaf},
			VerifiedChains:    [][]*x509.Certificate{{leaf}},
		}
	}

	tests := map[string]func(*harness.ProviderEndpointSpec, *tls.ConnectionState){
		"handshake incomplete":  func(_ *harness.ProviderEndpointSpec, state *tls.ConnectionState) { state.HandshakeComplete = false },
		"server name drift":     func(_ *harness.ProviderEndpointSpec, state *tls.ConnectionState) { state.ServerName = "other.invalid" },
		"peer absent":           func(_ *harness.ProviderEndpointSpec, state *tls.ConnectionState) { state.PeerCertificates = nil },
		"verified chain absent": func(_ *harness.ProviderEndpointSpec, state *tls.ConnectionState) { state.VerifiedChains = nil },
		"verified chain leaf drift": func(_ *harness.ProviderEndpointSpec, state *tls.ConnectionState) {
			other := *leaf
			other.Raw = append([]byte(nil), leaf.Raw...)
			other.Raw[0] ^= 0xff
			state.VerifiedChains = [][]*x509.Certificate{{&other}}
		},
		"hostname mismatch": func(endpoint *harness.ProviderEndpointSpec, state *tls.ConnectionState) {
			endpoint.ApprovedOrigin = "https://unmatched.invalid"
			state.ServerName = "unmatched.invalid"
		},
		"semantics hash drift": func(endpoint *harness.ProviderEndpointSpec, _ *tls.ConnectionState) {
			endpoint.SemanticsSHA256 = strings.Repeat("0", 64)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidateEndpoint := endpoint
			candidateState := validState()
			mutate(&candidateEndpoint, &candidateState)
			if _, err := projectProviderEndpointTLS(candidateEndpoint, candidateState); err == nil {
				t.Fatal("invalid provider TLS evidence was accepted")
			}
		})
	}
}

func TestProviderEndpointSnapshotAccessorRequiresReadyFormalBinding(t *testing.T) {
	endpoint := harness.FormalProviderEndpoint()
	valid := harness.ProviderEndpointSnapshot{
		ApprovedOrigin: endpoint.ApprovedOrigin, SemanticsSHA256: endpoint.SemanticsSHA256,
		TLSServerName: harness.FormalProviderTLSServerName, TLSVerified: true,
		TLSPeerLeafCertSHA256: strings.Repeat("a", 64), TLSPeerSPKISHA256: strings.Repeat("b", 64),
	}
	makeBackend := func() *Backend {
		return &Backend{
			config:   Config{ProviderUpstream: endpoint.ApprovedOrigin},
			manifest: harness.Manifest{ProviderEndpoint: endpoint}, endpoint: valid, ready: true,
		}
	}
	backend := makeBackend()
	if got, err := backend.providerEndpointSnapshot(); err != nil || got != valid {
		t.Fatalf("valid provider endpoint accessor = %#v, %v", got, err)
	}

	tests := map[string]func(*Backend){
		"not ready":                func(value *Backend) { value.ready = false },
		"configured origin drift":  func(value *Backend) { value.config.ProviderUpstream = "https://other.invalid" },
		"manifest semantics drift": func(value *Backend) { value.manifest.ProviderEndpoint.Semantics.TLSRequired = false },
		"server name drift":        func(value *Backend) { value.endpoint.TLSServerName = "other.invalid" },
		"unverified":               func(value *Backend) { value.endpoint.TLSVerified = false },
		"leaf hash absent":         func(value *Backend) { value.endpoint.TLSPeerLeafCertSHA256 = "" },
		"SPKI hash invalid":        func(value *Backend) { value.endpoint.TLSPeerSPKISHA256 = strings.Repeat("A", 64) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := makeBackend()
			mutate(candidate)
			if _, err := candidate.providerEndpointSnapshot(); err == nil {
				t.Fatal("invalid ready provider endpoint snapshot was accepted")
			}
		})
	}
}

func TestPreflightProviderEndpointRejectsNonformalBindingBeforeDial(t *testing.T) {
	endpoint := harness.FormalProviderEndpoint()
	if _, err := preflightProviderEndpoint(context.Background(), endpoint, "https://other.invalid"); err == nil {
		t.Fatal("configured origin drift was accepted")
	}
	endpoint.Semantics.WebSocketAllowed = false
	if _, err := preflightProviderEndpoint(context.Background(), endpoint, endpoint.ApprovedOrigin); err == nil {
		t.Fatal("nonformal endpoint semantics were accepted")
	}
}
