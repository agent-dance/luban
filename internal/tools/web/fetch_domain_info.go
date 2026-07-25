// Package tools — webfetch-domain-blocklist-preflight.
//
// Mirrors src/tools/WebFetchTool/utils.ts:171-203,386-398. Before any
// outbound fetch we ask Anthropic's domain_info endpoint whether the
// hostname is on the brand/security blocklist. The result is cached
// per-hostname for 5 minutes so the preflight cost is paid at most
// once per allowed hostname per window. Blocked and failed verdicts are never
// cached, matching the TS fail-closed security boundary.
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const (
	domainInfoDefaultEndpoint = "https://api.anthropic.com/api/web/domain_info"
	domainInfoTimeout         = 10 * time.Second
	domainInfoCacheTTL        = 5 * time.Minute
)

// domainInfoCacheEntry represents an allowed verdict. No other verdict is
// cached, so a policy change or transient check failure is retried next call.
type domainInfoCacheEntry struct {
	expiry time.Time
}

var (
	domainInfoCacheMu sync.Mutex
	domainInfoCache   = make(map[string]domainInfoCacheEntry)
)

// ResetDomainInfoCache clears the per-hostname verdict cache. Tests use
// this to ensure each invocation talks to the (fake) endpoint.
func ResetDomainInfoCache() {
	domainInfoCacheMu.Lock()
	domainInfoCache = make(map[string]domainInfoCacheEntry)
	domainInfoCacheMu.Unlock()
}

// domainInfoResponse is the exact TS domain_info response field. A pointer is
// required so a missing/null can_fetch cannot be mistaken for false/blocked.
type domainInfoResponse struct {
	CanFetch *bool `json:"can_fetch"`
}

type domainInfoCheckError struct {
	Hostname string
	Cause    error
}

func (e *domainInfoCheckError) Error() string {
	if e == nil {
		return ""
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebDomainSafetyCheckFailed, e.Hostname)
}

func (e *domainInfoCheckError) Unwrap() error { return e.Cause }

// domainInfoLookup performs the preflight with caching. endpoint is the
// base URL (no query string); the hostname is appended as ?domain=...
// The supplied http.Client must already enforce SSRF protection if the
// caller cares — by convention this is the Anthropic API client.
func domainInfoLookup(ctx context.Context, client *http.Client, endpoint, hostname string) (blocked bool, err error) {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		return false, &domainInfoCheckError{Hostname: hostname, Cause: i18n.NewError(i18n.KeyToolWebDomainEmptyHostname)}
	}

	// Cache hit?
	domainInfoCacheMu.Lock()
	entry, ok := domainInfoCache[hostname]
	if ok && time.Now().Before(entry.expiry) {
		domainInfoCacheMu.Unlock()
		return false, nil
	}
	domainInfoCacheMu.Unlock()

	if endpoint == "" {
		endpoint = domainInfoDefaultEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: domainInfoTimeout}
	}

	q := url.Values{}
	q.Set("domain", hostname)
	requestURL := endpoint + "?" + q.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, domainInfoTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return false, i18n.WrapError(i18n.KeyToolWebDomainBuildRequest, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return false, &domainInfoCheckError{Hostname: hostname, Cause: i18n.WrapError(i18n.KeyToolWebDomainRequest, err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, &domainInfoCheckError{Hostname: hostname, Cause: i18n.NewError(i18n.KeyToolWebDomainStatus, resp.StatusCode)}
	}

	var out domainInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, &domainInfoCheckError{Hostname: hostname, Cause: i18n.WrapError(i18n.KeyToolWebDomainDecodeResponse, err)}
	}
	if out.CanFetch == nil {
		return false, &domainInfoCheckError{Hostname: hostname, Cause: i18n.NewError(i18n.KeyToolWebDomainMissingCanFetch)}
	}
	if !*out.CanFetch {
		return true, nil
	}

	domainInfoCacheMu.Lock()
	domainInfoCache[hostname] = domainInfoCacheEntry{expiry: time.Now().Add(domainInfoCacheTTL)}
	domainInfoCacheMu.Unlock()
	return false, nil
}

// PreflightDomainInfoEnabled reports whether a WebFetchTool instance has
// the preflight wired up. Used so callers (and tests) can skip the
// preflight when no endpoint is configured.
func PreflightDomainInfoEnabled(w *WebFetchTool) bool {
	return w != nil && !w.SkipWebFetchPreflight && (w.DomainInfoEndpoint != "" || w.DomainInfoClient != nil)
}
