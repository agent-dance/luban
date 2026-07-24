// Package tools — additional URL validation tests for the
// max-length, userinfo, and public-TLD guards added to validateURL.
package tools

import (
	"strings"
	"testing"
)

func TestValidateURL_MaxLength(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", 3000)
	if err := validateURL(long); err == nil {
		t.Fatalf("expected long URL rejection")
	} else if !strings.Contains(err.Error(), "max length") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateURL_RejectsUserinfo(t *testing.T) {
	if err := validateURL("https://user:pass@example.com/x"); err == nil {
		t.Fatalf("expected userinfo rejection")
	} else if !strings.Contains(err.Error(), "userinfo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateURL_RejectsBareHostname(t *testing.T) {
	for _, host := range []string{
		"https://intranet/",
		"https://router/",
		"https://localhost/x",
	} {
		if err := validateURL(host); err == nil {
			t.Errorf("expected non-public hostname rejection for %q", host)
		}
	}
}
