package web

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// MaxFetchURLLength caps the raw URL length accepted by validateURL. The
// Anthropic web_fetch_20250910 server tool rejects anything longer; matching
// the limit here keeps Go in sync (mirrors WEB_FETCH_MAX_URL_LENGTH=2000 in
// src/tools/WebFetchTool/validateUrl.ts).
const MaxFetchURLLength = 2000

// validateURL checks that a URL is safe to fetch, rejecting non-HTTP schemes,
// internal/private IPs, loopback, link-local, and cloud metadata addresses.
// This prevents Server-Side Request Forgery (SSRF) attacks.
func validateURL(rawURL string) error {
	// 0. Length cap (TS web_fetch). Reject before parse to avoid spending
	// cycles on adversarial input that the server tool would reject anyway.
	if len(rawURL) > MaxFetchURLLength {
		return i18n.NewError(i18n.KeyToolWebValidationURLTooLong, MaxFetchURLLength)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return i18n.WrapError(i18n.KeyToolWebValidationInvalidURL, err)
	}

	// 1. Reject non-http/https schemes
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return i18n.NewError(i18n.KeyToolWebValidationUnsupportedScheme, parsed.Scheme)
	}

	// 1b. Reject userinfo embedded in the URL. TS rejects user:pass@host
	// because credentials in a URL surface as transcript data and may leak
	// via referrer headers.
	if parsed.User != nil {
		return i18n.NewError(i18n.KeyToolWebValidationUserinfoForbidden)
	}

	// 2. Extract hostname (without port)
	hostname := parsed.Hostname()
	if hostname == "" {
		return i18n.NewError(i18n.KeyToolWebValidationHostnameMissing)
	}

	// 2b. Public-TLD sanity check. A literal IP, "localhost", or a
	// dot-less single-label hostname is rejected. The server tool requires
	// at least one dot in the hostname (so "intranet" or "router" do not
	// resolve via captive DNS) — the IP path is handled below.
	if net.ParseIP(hostname) == nil {
		if !strings.Contains(hostname, ".") || strings.EqualFold(hostname, "localhost") {
			return i18n.NewError(i18n.KeyToolWebValidationHostnameNotPublic, hostname)
		}
	}

	// 3. Resolve hostname to IPs
	ips, err := net.LookupHost(hostname)
	if err != nil {
		// Also check if hostname is already an IP literal
		if ip := net.ParseIP(hostname); ip != nil {
			ips = []string{hostname}
		} else {
			return i18n.WrapError(i18n.KeyToolWebValidationResolveHostname, err, hostname)
		}
	}

	// 4. Check all resolved IPs against blocked ranges using Go's built-in
	// IP classification methods for correctness.
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if err := checkBlockedIP(ip); err != nil {
			return err
		}
	}

	return nil
}

// cloudMetadataIP is the well-known cloud metadata endpoint (AWS/GCP/Azure).
var cloudMetadataIP = net.ParseIP("169.254.169.254")

func checkBlockedIP(ip net.IP) error {
	// C5+C6: Use Go's built-in IP classification for correctness instead of
	// hand-rolled CIDR lists that were previously misconfigured.
	if ip.IsLoopback() {
		return i18n.NewError(i18n.KeyToolWebValidationLoopbackAddress, ip.String())
	}
	if ip.IsPrivate() {
		return i18n.NewError(i18n.KeyToolWebValidationPrivateAddress, ip.String())
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return i18n.NewError(i18n.KeyToolWebValidationLinkLocalAddress, ip.String())
	}
	if ip.IsUnspecified() {
		return i18n.NewError(i18n.KeyToolWebValidationUnspecifiedAddress, ip.String())
	}
	// Block cloud metadata endpoints
	if ip.Equal(cloudMetadataIP) {
		return i18n.NewError(i18n.KeyToolWebValidationCloudMetadataAddress, ip.String())
	}
	return nil
}

// newSSRFSafeHTTPClient creates an HTTP client that validates resolved IPs
// at the transport (dial) level, preventing DNS rebinding attacks.
func newSSRFSafeHTTPClient(timeout time.Duration, maxRedirects int) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.LookupHost(host)
			if err != nil {
				return nil, err
			}
			for _, ipStr := range ips {
				ip := net.ParseIP(ipStr)
				if ip != nil {
					if err := checkBlockedIP(ip); err != nil {
						return nil, err
					}
				}
			}
			// Dial to the first resolved address
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if maxRedirects > 0 && len(via) > maxRedirects {
				return i18n.NewError(i18n.KeyToolWebValidationRedirectLimit, maxRedirects)
			}
			return nil
		},
	}
}
