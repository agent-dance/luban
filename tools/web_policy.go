// Package tools — region/rate-limit policy gate for WebSearch.
//
// Mirrors the policy block in src/tools/WebSearchTool/WebSearchTool.ts:
// the hosted server tool only ships results for callers in the US, and
// per-session rate-limits keep abusive callers from monopolising the
// quota. The default WebSearchTool runs in permissive mode (no region
// check, no rate limit) so unit tests that don't care about policy just
// work; production setup wires a real WebSearchPolicy via Configure.
package tools

import (
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// ErrWebSearchRegionBlocked is returned when the caller's region is not
// in the allow-list.
var ErrWebSearchRegionBlocked = i18n.NewError(i18n.KeyToolWebPolicyRegionBlocked)

// ErrWebSearchRateLimited is returned when the per-session rate limit has
// been exceeded.
var ErrWebSearchRateLimited = i18n.NewError(i18n.KeyToolWebPolicyRateLimited)

// DefaultWebSearchRateLimitPerMinute is the default per-session ceiling.
const DefaultWebSearchRateLimitPerMinute = 60

// WebSearchPolicy controls runtime gating for the WebSearch tool. Region
// is the ISO-3166 alpha-2 country code of the caller (case-insensitive);
// "US" or "" passes the region check, anything else is rejected.
// RateLimitPerMinute caps the per-session call count.
type WebSearchPolicy struct {
	Region              string
	RateLimitPerMinute  int
	now                 func() time.Time
	mu                  sync.Mutex
	bucketStart         time.Time
	bucketCount         int
	skipRegionEnforce   bool
	skipRateLimitEnforc bool
}

// NewWebSearchPolicy creates a policy with explicit region/limit values.
func NewWebSearchPolicy(region string, rateLimit int) *WebSearchPolicy {
	if rateLimit <= 0 {
		rateLimit = DefaultWebSearchRateLimitPerMinute
	}
	return &WebSearchPolicy{
		Region:             strings.ToUpper(strings.TrimSpace(region)),
		RateLimitPerMinute: rateLimit,
		now:                time.Now,
	}
}

// PermissiveWebSearchPolicy is a no-op policy that allows every call.
// Used by default in WebSearchTool so unit tests exercising the search
// pipeline don't need to opt out of region checks.
func PermissiveWebSearchPolicy() *WebSearchPolicy {
	p := NewWebSearchPolicy("US", DefaultWebSearchRateLimitPerMinute)
	p.skipRegionEnforce = true
	p.skipRateLimitEnforc = true
	return p
}

// Allow reports whether a fresh search call may proceed. The first
// non-nil error short-circuits before the rate-limit counter is bumped
// so blocked callers don't waste their quota.
func (p *WebSearchPolicy) Allow() error {
	if p == nil {
		return nil
	}
	if !p.skipRegionEnforce {
		if region := strings.ToUpper(strings.TrimSpace(p.Region)); region != "" && region != "US" {
			return ErrWebSearchRegionBlocked
		}
	}
	if p.skipRateLimitEnforc {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.nowOrDefault()
	if now.Sub(p.bucketStart) >= time.Minute {
		p.bucketStart = now
		p.bucketCount = 0
	}
	p.bucketCount++
	if p.RateLimitPerMinute > 0 && p.bucketCount > p.RateLimitPerMinute {
		return i18n.WrapInternalError(i18n.KeyToolWebPolicyRateLimitedWithLimit, ErrWebSearchRateLimited, p.RateLimitPerMinute)
	}
	return nil
}

func (p *WebSearchPolicy) nowOrDefault() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// Reset clears the rate-limit bucket. Intended for tests.
func (p *WebSearchPolicy) Reset() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bucketStart = time.Time{}
	p.bucketCount = 0
}
