package hooks

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPHookPreservesRawStructuredResponseEvidence(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	raw := `{"system_reminder":"accepted","audit_token":"exact-token"}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, raw)
	}))
	defer server.Close()

	output := executeHTTPHook(context.Background(), Hook{Kind: HookKindHTTP, URL: server.URL, Timeout: 5}, HookInput{})
	if output.Stdout != raw || output.StdoutBytes != int64(len(raw)) || output.StdoutTruncated {
		t.Fatalf("raw HTTP evidence = stdout %q bytes %d truncated %t", output.Stdout, output.StdoutBytes, output.StdoutTruncated)
	}
	if output.SystemReminder != "accepted" {
		t.Fatalf("structured response was not interpreted: %+v", output)
	}
}

func TestHTTPHookTruncationKeepsCapturedPrefixAndMetadata(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()
	originalLimit := maxHTTPBody
	maxHTTPBody = 16
	defer func() { maxHTTPBody = originalLimit }()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, strings.Repeat("z", 32))
	}))
	defer server.Close()

	output := executeHTTPHook(context.Background(), Hook{Kind: HookKindHTTP, URL: server.URL, Timeout: 5}, HookInput{})
	if output.Stdout != strings.Repeat("z", 16) || !output.StdoutTruncated || output.StdoutBytes != 17 {
		t.Fatalf("truncated HTTP evidence = stdout %q bytes %d truncated %t", output.Stdout, output.StdoutBytes, output.StdoutTruncated)
	}
	if !strings.Contains(output.Stderr, "truncated") {
		t.Fatalf("truncation error missing from evidence: %+v", output)
	}
}

func TestHTTPHookValidationFailureCountsStderrEvidence(t *testing.T) {
	output := executeHTTPHook(context.Background(), Hook{Kind: HookKindHTTP, URL: "file:///tmp/hook"}, HookInput{})
	if output.Stderr == "" {
		t.Fatal("validation failure did not preserve stderr evidence")
	}
	if output.StderrBytes != int64(len(output.Stderr)) {
		t.Fatalf("stderr byte count = %d, want %d", output.StderrBytes, len(output.Stderr))
	}
}

func TestUnknownHookKindCountsStderrEvidence(t *testing.T) {
	output := NewRunner([]Hook{{Type: HookPreToolUse, Kind: HookKind("future")}}).
		Run(context.Background(), HookPreToolUse, HookInput{})[0]
	if output.Stderr == "" {
		t.Fatal("unknown kind did not preserve stderr evidence")
	}
	if output.StderrBytes != int64(len(output.Stderr)) {
		t.Fatalf("stderr byte count = %d, want %d", output.StderrBytes, len(output.Stderr))
	}
}
