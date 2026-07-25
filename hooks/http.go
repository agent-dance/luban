package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

const defaultHTTPTimeout = 30 * time.Second

// maxHTTPBody is the response body cap. It is a var (not const) so tests can
// temporarily lower it to exercise truncation detection without sending 1 MB.
var maxHTTPBody int64 = 1 << 20 // 1 MB

// privateRanges holds the CIDR blocks that are blocked for SSRF prevention.
var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		// RFC 1918 private ranges
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		// Loopback
		"127.0.0.0/8",
		// Link-local (includes AWS/GCP metadata at 169.254.169.254)
		"169.254.0.0/16",
		// IPv6: loopback, ULA, link-local
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("hooks: invalid private CIDR %q: %v", cidr, err))
		}
		privateRanges = append(privateRanges, network)
	}
}

// validateHookURL returns an error if rawURL should not be contacted by a hook.
// It blocks non-http(s) schemes and any hostname that resolves to a private,
// loopback, or link-local address (SSRF prevention).
func validateHookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookURLInvalid), err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookSchemeNotAllowed, u.Scheme))
	}

	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookHostnameMissing))
	}

	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookDNSLookupFailed, hostname), err)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		for _, network := range privateRanges {
			if network.Contains(ip) {
				return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookBlockedIP, ip))
			}
		}
	}

	return nil
}

// urlValidator is the SSRF-validation function used by executeHTTPHook.
// Tests may replace this with a no-op to permit httptest.NewServer addresses.
var urlValidator = validateHookURL

// ssrfDialContext is a DialContext that resolves DNS, validates every resolved
// IP against privateRanges, and then connects directly to the first validated
// IP address.  Pinning the connection to the checked IP prevents DNS-rebinding
// TOCTOU attacks where a second lookup at dial time could return a different
// (private) address.
func ssrfDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookDNSLookupFailed, host), err)
	}

	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil {
			continue
		}
		for _, ipNet := range privateRanges {
			if ipNet.Contains(ip) {
				return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookSSRFBlocked, host, a))
			}
		}
	}

	// Connect directly to the first validated IP, bypassing a second DNS lookup.
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(addrs[0], port))
}

// hookHTTPClient is a hardened client used for all hook requests.
// It enforces connection limits, TLS timeouts, a redirect cap, and
// re-validates every redirect target against the SSRF block list.
// The custom DialContext pins DNS resolution to prevent rebinding attacks.
var hookHTTPClient = &http.Client{
	Transport: &http.Transport{
		DialContext:         ssrfDialContext,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookRedirectLimit))
		}
		// Re-validate redirect target using the real validator (not the
		// injectable variable) so redirect checks cannot be bypassed.
		if err := validateHookURL(req.URL.String()); err != nil {
			return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookRedirectBlocked), err)
		}
		return nil
	},
}

// executeHTTPHook POSTs the hook input as JSON to the configured URL and
// interprets the response body the same way executeCommandHook interprets
// stdout: JSON with optional system_reminder/block/modified_input keys, or
// plain text that becomes the system reminder.
//
// Retry semantics: the hook is attempted up to max(1, hook.RetryCount) times.
// Network-level errors and HTTP 502/503/504 trigger a retry with exponential
// backoff.  Other 4xx/5xx responses are returned directly (no retry).
func executeHTTPHook(parentCtx context.Context, hook Hook, input HookInput) HookOutput {
	// H1: validate URL before touching the network.
	if err := urlValidator(hook.URL); err != nil {
		return hookErrorOutput(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookHTTPURLValidationFailed, err), true)
	}

	timeout := time.Duration(hook.Timeout) * time.Second
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}

	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	body, err := json.Marshal(input)
	if err != nil {
		return hookErrorOutput(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookHTTPInputMarshalFailed), true)
	}

	attempts := hook.RetryCount
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	var lastOutput HookOutput
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			break
		}
		output, err := doHTTPRequest(ctx, hook, body)
		lastOutput = output
		if err == nil {
			return output
		}
		lastErr = err

		// H3: exponential backoff between retries; bail early if context done.
		if i < attempts-1 {
			delay := time.Duration(1<<uint(i)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
			case <-time.After(delay):
			}
		}
	}

	lastOutput.ExitCode = -1
	lastOutput.ExecutionError = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookAttemptsFailed, attempts, lastErr)
	lastOutput.Block = false // network errors are non-blocking by default
	return lastOutput
}

func doHTTPRequest(ctx context.Context, hook Hook, body []byte) (HookOutput, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		return HookOutput{}, fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookRequestBuildFailed), err)
	}

	// Apply user-supplied headers first, then enforce protected headers so
	// they cannot be overridden by a misconfigured or malicious hook definition.
	for k, v := range hook.Headers {
		req.Header.Set(k, v)
	}
	// H9: Content-Type is always application/json — set after user headers.
	req.Header.Set("Content-Type", "application/json")
	// H5: User-Agent is always our sentinel — set after user headers so it
	// cannot be spoofed via hook.Headers.
	req.Header.Set("User-Agent", brand.CommandName+"-hooks/1.0")

	// H2: use the hardened client instead of http.DefaultClient.
	resp, err := hookHTTPClient.Do(req)
	if err != nil {
		return HookOutput{}, err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxHTTPBody)
	respBody, err := io.ReadAll(limited)
	evidence := HookOutput{
		Stdout:      string(respBody),
		StdoutBytes: int64(len(respBody)),
	}
	if err != nil {
		return evidence, fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookResponseReadFailed), err)
	}

	// H5: detect truncation by attempting to read one more byte beyond the cap.
	var extra [1]byte
	if n, _ := resp.Body.Read(extra[:]); n > 0 {
		evidence.StdoutBytes += int64(n)
		evidence.StdoutTruncated = true
		return evidence, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookResponseTruncated, maxHTTPBody))
	}

	// H4: 502/503/504 are likely transient — return an error so the retry
	// loop can attempt the request again.
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return evidence, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode >= 400 {
		evidence.ExitCode = resp.StatusCode
		evidence.Stderr = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))
		evidence.StderrBytes = int64(len(evidence.Stderr))
		return evidence, nil
	}

	// Parse response body — same contract as command hook stdout.
	output := evidence
	var structured struct {
		SystemReminder      string         `json:"system_reminder"`
		Block               bool           `json:"block"`
		ModifiedInput       map[string]any `json:"modified_input"`
		UpdatedInput        map[string]any `json:"updated_input"`
		UpdatedInputCamel   map[string]any `json:"updatedInput"`
		PreventContinuation bool           `json:"prevent_continuation"`
		PreventContCamel    bool           `json:"preventContinuation"`
		StopReason          string         `json:"stop_reason"`
		StopReasonCamel     string         `json:"stopReason"`
		AdditionalContext   string         `json:"additional_context"`
		AdditionalContexts  []string       `json:"additional_contexts"`
		AdditionalCtxCamel  []string       `json:"additionalContexts"`
		PermissionBehavior  string         `json:"permission_behavior"`
		PermissionCamel     string         `json:"permissionBehavior"`
		NewCustomInstr      string         `json:"new_custom_instructions"`
		NewCustomInstrCamel string         `json:"newCustomInstructions"`
		UserDisplayMessage  string         `json:"user_display_message"`
		UserDisplayCamel    string         `json:"userDisplayMessage"`
	}
	if len(respBody) > 0 {
		if json.Unmarshal(respBody, &structured) == nil {
			output.SystemReminder = structured.SystemReminder
			output.Block = structured.Block
			output.ModifiedInput = structured.ModifiedInput
			if output.ModifiedInput == nil {
				output.ModifiedInput = structured.UpdatedInput
			}
			if output.ModifiedInput == nil {
				output.ModifiedInput = structured.UpdatedInputCamel
			}
			output.PreventContinuation = structured.PreventContinuation
			if structured.PreventContCamel {
				output.PreventContinuation = true
			}
			output.StopReason = structured.StopReason
			if output.StopReason == "" {
				output.StopReason = structured.StopReasonCamel
			}
			output.AdditionalContext = structured.AdditionalContext
			output.AdditionalContexts = structured.AdditionalContexts
			if len(output.AdditionalContexts) == 0 {
				output.AdditionalContexts = structured.AdditionalCtxCamel
			}
			output.PermissionBehavior = structured.PermissionBehavior
			if output.PermissionBehavior == "" {
				output.PermissionBehavior = structured.PermissionCamel
			}
			output.NewCustomInstructions = structured.NewCustomInstr
			if output.NewCustomInstructions == "" {
				output.NewCustomInstructions = structured.NewCustomInstrCamel
			}
			output.UserDisplayMessage = structured.UserDisplayMessage
			if output.UserDisplayMessage == "" {
				output.UserDisplayMessage = structured.UserDisplayCamel
			}
		} else {
			output.SystemReminder = string(respBody)
		}
	}

	return output, nil
}
