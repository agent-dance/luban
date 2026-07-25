package remote

import "testing"

// TestRemoteTriggerLoadCustomCABundleEmpty verifies RT-06: when no env
// vars are set, no custom pool is constructed.
func TestRemoteTriggerLoadCustomCABundleEmpty(t *testing.T) {
	t.Setenv("SSL_CERT_FILE", "")
	t.Setenv("NODE_EXTRA_CA_CERTS", "")
	if pool := loadCustomCABundle(); pool != nil {
		t.Fatalf("expected nil pool when no env vars set")
	}
}
