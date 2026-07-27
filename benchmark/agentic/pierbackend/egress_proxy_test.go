package pierbackend

import (
	"strings"
	"testing"
)

func TestValidateEgressProxyImageReferenceRequiresRepositoryDigest(t *testing.T) {
	valid := "ubuntu/squid@sha256:" + strings.Repeat("a", 64)
	if err := validateEgressProxyImageReference(valid); err != nil {
		t.Fatalf("valid digest reference: %v", err)
	}
	for _, invalid := range []string{
		"", "ubuntu/squid:latest", "ubuntu/squid@sha256:short",
		"ubuntu/squid@sha256:" + strings.Repeat("A", 64),
	} {
		if err := validateEgressProxyImageReference(invalid); err == nil {
			t.Fatalf("mutable or malformed proxy image accepted: %q", invalid)
		}
	}
}

func TestFrozenEgressProxyImageIsDigestPinned(t *testing.T) {
	if err := validateEgressProxyImageReference(FrozenEgressProxyImage); err != nil {
		t.Fatal(err)
	}
}
