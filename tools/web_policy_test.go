package tools

import (
	"errors"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

func TestWebSearchPolicy_AllowsUSAndEmptyRegion(t *testing.T) {
	for _, region := range []string{"", "US", "us", " us "} {
		p := NewWebSearchPolicy(region, 60)
		if err := p.Allow(); err != nil {
			t.Fatalf("region=%q: unexpected error: %v", region, err)
		}
	}
}

func TestWebSearchPolicy_BlocksNonUSRegion(t *testing.T) {
	p := NewWebSearchPolicy("DE", 60)
	err := p.Allow()
	if !errors.Is(err, ErrWebSearchRegionBlocked) {
		t.Fatalf("expected ErrWebSearchRegionBlocked, got %v", err)
	}
}

func TestWebSearchPolicy_RegionVerbatimMessage(t *testing.T) {
	want := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebPolicyRegionBlocked)
	if got := ErrWebSearchRegionBlocked.Error(); got != want {
		t.Fatalf("region error message changed: %q", got)
	}
}

func TestWebSearchPolicy_RateLimitFires(t *testing.T) {
	p := NewWebSearchPolicy("US", 3)
	for i := 0; i < 3; i++ {
		if err := p.Allow(); err != nil {
			t.Fatalf("call %d: unexpected error %v", i, err)
		}
	}
	err := p.Allow()
	if !errors.Is(err, ErrWebSearchRateLimited) {
		t.Fatalf("expected ErrWebSearchRateLimited, got %v", err)
	}
	want := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebPolicyRateLimitedWithLimit, 3)
	if got := err.Error(); got != want {
		t.Fatalf("rate limit error = %q, want %q", got, want)
	}
}

func TestWebSearchPolicy_RateLimitWindowResets(t *testing.T) {
	p := NewWebSearchPolicy("US", 2)
	current := time.Now()
	p.now = func() time.Time { return current }

	if err := p.Allow(); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := p.Allow(); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if err := p.Allow(); !errors.Is(err, ErrWebSearchRateLimited) {
		t.Fatalf("third call should be rate limited, got %v", err)
	}

	// Advance past the bucket window.
	current = current.Add(time.Minute + time.Second)
	if err := p.Allow(); err != nil {
		t.Fatalf("post-reset call: %v", err)
	}
}

func TestWebSearchPolicy_Reset(t *testing.T) {
	p := NewWebSearchPolicy("US", 1)
	if err := p.Allow(); err != nil {
		t.Fatal(err)
	}
	if err := p.Allow(); !errors.Is(err, ErrWebSearchRateLimited) {
		t.Fatalf("expected rate limit, got %v", err)
	}
	p.Reset()
	if err := p.Allow(); err != nil {
		t.Fatalf("post-Reset call: %v", err)
	}
}

func TestPermissiveWebSearchPolicy_NeverBlocks(t *testing.T) {
	p := PermissiveWebSearchPolicy()
	for i := 0; i < 1000; i++ {
		if err := p.Allow(); err != nil {
			t.Fatalf("permissive policy blocked at %d: %v", i, err)
		}
	}
}

func TestWebSearchPolicy_NilSafe(t *testing.T) {
	var p *WebSearchPolicy
	if err := p.Allow(); err != nil {
		t.Fatalf("nil receiver should be a no-op, got %v", err)
	}
	p.Reset() // Must not panic.
}

func TestWebSearchPolicy_DefaultRateLimit(t *testing.T) {
	p := NewWebSearchPolicy("US", 0)
	if p.RateLimitPerMinute != DefaultWebSearchRateLimitPerMinute {
		t.Fatalf("expected default rate limit %d, got %d", DefaultWebSearchRateLimitPerMinute, p.RateLimitPerMinute)
	}
}
