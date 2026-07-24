package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// capture records what the test server received.
// T1: mu guards concurrent writes from handler goroutines.
type capture struct {
	mu      sync.Mutex
	body    []byte
	headers http.Header
	calls   int
}

// bypassValidator replaces both the URL validator AND the HTTP client transport
// for tests that use httptest.NewServer (which binds to 127.0.0.1, a loopback
// address blocked by SSRF checks at both the pre-request and dial levels).
// It returns a restore function that the caller should defer.
func bypassValidator(t *testing.T) func() {
	t.Helper()
	origValidator := urlValidator
	origClient := hookHTTPClient

	urlValidator = func(string) error { return nil }
	hookHTTPClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		CheckRedirect: origClient.CheckRedirect,
	}

	return func() {
		urlValidator = origValidator
		hookHTTPClient = origClient
	}
}

func TestHTTPHookPostsJSON(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	var cap capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.mu.Lock()
		cap.calls++
		cap.headers = r.Header.Clone()
		cap.body, _ = io.ReadAll(r.Body)
		cap.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := Hook{
		Type: HookPreToolUse,
		Kind: HookKindHTTP,
		URL:  srv.URL,
	}
	input := HookInput{
		Type:     HookPreToolUse,
		ToolName: "Bash",
		ToolInput: map[string]any{
			"command": "echo hello",
		},
	}

	output := executeHTTPHook(context.Background(), hook, input)

	if output.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d (stderr: %s)", output.ExitCode, output.Stderr)
	}

	cap.mu.Lock()
	defer cap.mu.Unlock()
	if cap.calls != 1 {
		t.Errorf("expected 1 HTTP call, got %d", cap.calls)
	}
	if ct := cap.headers.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	// Verify the body is valid JSON matching the input
	var decoded HookInput
	if err := json.Unmarshal(cap.body, &decoded); err != nil {
		t.Fatalf("server received non-JSON body: %v\nbody: %s", err, cap.body)
	}
	if decoded.ToolName != "Bash" {
		t.Errorf("expected tool_name=Bash, got %q", decoded.ToolName)
	}
}

func TestHTTPHookCustomHeaders(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	var cap capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.mu.Lock()
		cap.calls++
		cap.headers = r.Header.Clone()
		cap.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := Hook{
		Type: HookPreToolUse,
		Kind: HookKindHTTP,
		URL:  srv.URL,
		Headers: map[string]string{
			"X-Custom-Header": "my-value",
			"Authorization":   "Bearer secret",
		},
	}

	executeHTTPHook(context.Background(), hook, HookInput{Type: HookPreToolUse})

	cap.mu.Lock()
	defer cap.mu.Unlock()
	if cap.headers.Get("X-Custom-Header") != "my-value" {
		t.Errorf("expected X-Custom-Header=my-value, got %q", cap.headers.Get("X-Custom-Header"))
	}
	if cap.headers.Get("Authorization") != "Bearer secret" {
		t.Errorf("expected Authorization=Bearer secret, got %q", cap.headers.Get("Authorization"))
	}
}

func TestHTTPHookJSONResponseParsed(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"system_reminder": "reminder from server",
			"block":           true,
		})
	}))
	defer srv.Close()

	hook := Hook{Kind: HookKindHTTP, URL: srv.URL}
	output := executeHTTPHook(context.Background(), hook, HookInput{})

	if output.SystemReminder != "reminder from server" {
		t.Errorf("expected system_reminder='reminder from server', got %q", output.SystemReminder)
	}
	if !output.Block {
		t.Error("expected Block=true from JSON response")
	}
}

func TestHTTPHookPlainTextResponse(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "plain text reminder")
	}))
	defer srv.Close()

	hook := Hook{Kind: HookKindHTTP, URL: srv.URL}
	output := executeHTTPHook(context.Background(), hook, HookInput{})

	if output.SystemReminder != "plain text reminder" {
		t.Errorf("expected plain text system reminder, got %q", output.SystemReminder)
	}
}

func TestHTTPHookNonBlockingOn4xx(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "bad request")
	}))
	defer srv.Close()

	hook := Hook{Kind: HookKindHTTP, URL: srv.URL}
	output := executeHTTPHook(context.Background(), hook, HookInput{})

	// 4xx returns a non-zero exit code but must not block
	if output.Block {
		t.Error("expected Block=false for 4xx response")
	}
	if output.ExitCode != 400 {
		t.Errorf("expected ExitCode=400, got %d", output.ExitCode)
	}
}

func TestHTTPHookRetry(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// Force a connection close to simulate network error only on first call
		if calls < 3 {
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := Hook{
		Kind:       HookKindHTTP,
		URL:        srv.URL,
		RetryCount: 3,
		Timeout:    5,
	}
	output := executeHTTPHook(context.Background(), hook, HookInput{})

	if output.ExitCode != 0 {
		t.Errorf("expected success after retries, got exit code %d: %s", output.ExitCode, output.Stderr)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", calls)
	}
}

func TestHTTPHookTimeout(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	// Use a channel so the handler returns promptly once the test is done,
	// preventing httptest.Server.Close() from blocking on open connections.
	unblock := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-unblock:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		close(unblock)
		srv.Close()
	}()

	hook := Hook{
		Kind:    HookKindHTTP,
		URL:     srv.URL,
		Timeout: 1, // 1 second timeout
	}

	start := time.Now()
	output := executeHTTPHook(context.Background(), hook, HookInput{})
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("expected fast timeout, took %v", elapsed)
	}
	if output.ExitCode == 0 {
		t.Error("expected non-zero exit code on timeout")
	}
}

func TestHTTPHookContextCancelled(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	unblock := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-unblock:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		close(unblock)
		srv.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	hook := Hook{Kind: HookKindHTTP, URL: srv.URL}
	start := time.Now()
	executeHTTPHook(ctx, hook, HookInput{})
	if time.Since(start) > 2*time.Second {
		t.Error("cancelled context should return quickly")
	}
}

func TestHTTPHookInvalidURL(t *testing.T) {
	hook := Hook{Kind: HookKindHTTP, URL: "not-a-url"}
	output := executeHTTPHook(context.Background(), hook, HookInput{})
	if output.ExitCode == 0 {
		t.Error("expected error for invalid URL")
	}
}

// ── H8: User-Agent ───────────────────────────────────────────────────────────

func TestHTTPHookUserAgent(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := Hook{Kind: HookKindHTTP, URL: srv.URL}
	executeHTTPHook(context.Background(), hook, HookInput{})

	if gotUA != "luban-code-hooks/1.0" {
		t.Errorf("expected User-Agent 'luban-code-hooks/1.0', got %q", gotUA)
	}
}

// ── H9: Content-Type cannot be overridden ────────────────────────────────────

func TestHTTPHookContentTypeNotOverridable(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := Hook{
		Kind: HookKindHTTP,
		URL:  srv.URL,
		Headers: map[string]string{
			"Content-Type": "text/plain", // attacker-supplied override
		},
	}
	executeHTTPHook(context.Background(), hook, HookInput{})

	if gotCT != "application/json" {
		t.Errorf("Content-Type should be locked to application/json, got %q", gotCT)
	}
}

// ── H1: SSRF – internal IPs blocked ─────────────────────────────────────────

func TestValidateHookURL_BlocksLoopback(t *testing.T) {
	if err := validateHookURL("http://127.0.0.1/hook"); err == nil {
		t.Error("expected loopback 127.0.0.1 to be blocked")
	}
}

func TestValidateHookURL_BlocksMetadataIP(t *testing.T) {
	if err := validateHookURL("http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Error("expected link-local metadata IP 169.254.169.254 to be blocked")
	}
}

func TestValidateHookURL_BlocksPrivateRFC1918(t *testing.T) {
	cases := []string{
		"http://10.0.0.1/hook",
		"http://172.16.0.1/hook",
		"http://192.168.1.1/hook",
	}
	for _, u := range cases {
		if err := validateHookURL(u); err == nil {
			t.Errorf("expected %q to be blocked as private RFC 1918", u)
		}
	}
}

func TestValidateHookURL_BlocksNonHTTPSchemes(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"javascript://example.com/",
		"ftp://example.com/hook",
		"ssh://example.com/hook",
	}
	for _, u := range cases {
		if err := validateHookURL(u); err == nil {
			t.Errorf("expected scheme in %q to be blocked", u)
		}
	}
}

func TestValidateHookURL_AllowsPublicHTTPS(t *testing.T) {
	// We can't rely on DNS in all CI environments, so we only test the scheme
	// check path — a clearly invalid scheme must be rejected, while http/https
	// with a syntactically valid host must not be rejected by the scheme guard.
	err := validateHookURL("file:///etc/passwd")
	if err == nil {
		t.Error("expected file:// to be rejected")
	}
	// Scheme accepted (DNS may or may not succeed in CI — don't assert nil).
	// Just confirm no scheme-level error for https.
	err2 := validateHookURL("https://example.com/hook")
	if err2 != nil && strings.Contains(err2.Error(), "scheme") {
		t.Errorf("https scheme should not be rejected, got: %v", err2)
	}
}

// ── H4: 5xx retry behavior ───────────────────────────────────────────────────

func TestHTTPHook5xxRetried(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	for _, code := range []int{502, 503, 504} {
		code := code
		t.Run(fmt.Sprintf("HTTP%d", code), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls < 3 {
					w.WriteHeader(code)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			hook := Hook{
				Kind:       HookKindHTTP,
				URL:        srv.URL,
				RetryCount: 3,
				Timeout:    5,
			}
			output := executeHTTPHook(context.Background(), hook, HookInput{})
			if output.ExitCode != 0 {
				t.Errorf("HTTP %d: expected success after retry, got exit code %d: %s", code, output.ExitCode, output.Stderr)
			}
			if calls != 3 {
				t.Errorf("HTTP %d: expected 3 calls, got %d", code, calls)
			}
		})
	}
}

func TestHTTPHook4xxNotRetried(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	for _, code := range []int{400, 401, 403, 404} {
		code := code
		t.Run(fmt.Sprintf("HTTP%d", code), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(code)
			}))
			defer srv.Close()

			hook := Hook{
				Kind:       HookKindHTTP,
				URL:        srv.URL,
				RetryCount: 3,
				Timeout:    5,
			}
			executeHTTPHook(context.Background(), hook, HookInput{})
			if calls != 1 {
				t.Errorf("HTTP %d: expected exactly 1 call (no retry), got %d", code, calls)
			}
		})
	}
}

// ── H5: Truncation detection ─────────────────────────────────────────────────

func TestHTTPHookTruncationDetected(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	// Temporarily lower the cap so we don't need to send 1 MB.
	origCap := maxHTTPBody
	maxHTTPBody = 16
	defer func() { maxHTTPBody = origCap }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write more bytes than the 16-byte cap.
		io.WriteString(w, strings.Repeat("x", 32))
	}))
	defer srv.Close()

	hook := Hook{Kind: HookKindHTTP, URL: srv.URL, RetryCount: 1, Timeout: 5}
	output := executeHTTPHook(context.Background(), hook, HookInput{})

	// A truncated response should surface as a non-zero exit code / error.
	if output.ExitCode == 0 {
		t.Error("expected non-zero exit code when response is truncated")
	}
	if !strings.Contains(output.Stderr, "truncated") {
		t.Errorf("expected 'truncated' in stderr, got: %q", output.Stderr)
	}
}

func TestHTTPHookNoFalsePositiveTruncation(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"block":false}`)
	}))
	defer srv.Close()

	hook := Hook{Kind: HookKindHTTP, URL: srv.URL}
	output := executeHTTPHook(context.Background(), hook, HookInput{})

	if output.ExitCode != 0 {
		t.Errorf("expected success for small response, got: %s", output.Stderr)
	}
}

// ── Redirect validation ───────────────────────────────────────────────────────

func TestHTTPHookRedirectToInternalIPBlocked(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	// Server that redirects to an internal address.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:9999/secret", http.StatusFound)
	}))
	defer srv.Close()

	hook := Hook{Kind: HookKindHTTP, URL: srv.URL, Timeout: 5}
	output := executeHTTPHook(context.Background(), hook, HookInput{})

	// The redirect to 127.0.0.1 must be blocked — expect a non-zero exit code.
	if output.ExitCode == 0 {
		t.Error("expected redirect to internal IP to be blocked")
	}
}

// ── H5: User-Agent cannot be overridden by hook.Headers ──────────────────────

func TestHTTPHookUserAgentNotOverridable(t *testing.T) {
	restore := bypassValidator(t)
	defer restore()

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := Hook{
		Kind: HookKindHTTP,
		URL:  srv.URL,
		Headers: map[string]string{
			"User-Agent": "evil-agent/9.9", // attacker-supplied override attempt
		},
	}
	executeHTTPHook(context.Background(), hook, HookInput{})

	if gotUA != "luban-code-hooks/1.0" {
		t.Errorf("User-Agent should be locked to 'luban-code-hooks/1.0', got %q", gotUA)
	}
}
