// Package tools contains regression tests for WebFetch alignment invariants
// originally identified in alignment_audit.md.
package tools

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

// TestWebFetchAlignment_MaxMarkdownBytesIs100KB asserts the truncation cap
// matches the TS reference of 100KB. Audit gap P3-1.
func TestWebFetchAlignment_MaxMarkdownBytesIs100KB(t *testing.T) {
	const wantBytes = 100 * 1024
	if MaxMarkdownBytes != wantBytes {
		t.Fatalf("MaxMarkdownBytes: want %d (TS MAX_MARKDOWN_LENGTH=100KB), got %d",
			wantBytes, MaxMarkdownBytes)
	}
}

// TestWebFetchAlignment_StructuredPayloadHasTruncatedField asserts that the
// internal structured payload (webFetchStructuredPayload) exposes a Truncated
// field so consumers can render a truncation badge. Audit gap P3-1.
func TestWebFetchAlignment_StructuredPayloadHasTruncatedField(t *testing.T) {
	payloadType := reflect.TypeOf(webFetchStructuredPayload{})
	if _, ok := payloadType.FieldByName("Truncated"); !ok {
		fields := make([]string, 0, payloadType.NumField())
		for i := 0; i < payloadType.NumField(); i++ {
			fields = append(fields, payloadType.Field(i).Name)
		}
		t.Fatalf("webFetchStructuredPayload must declare a Truncated field; got fields=%v", fields)
	}
}

// TestWebFetchAlignment_BuildResultUsesTypedOutput asserts that the legacy
// helper now emits the TS output object without a Go-only content envelope.
func TestWebFetchAlignment_BuildResultUsesTypedOutput(t *testing.T) {
	res := buildWebFetchStructuredResult(
		"https://example.com/page",
		"summarise",
		strings.Repeat("x", 16),
		webExecutionModeLocalFallback,
	)
	if _, ok := res.Data.(WebFetchOutput); !ok || res.Content != strings.Repeat("x", 16) {
		t.Fatalf("expected typed TS output/result-only content, got data=%T content=%q", res.Data, res.Content)
	}
}

// TestWebFetchAlignment_CacheByteCapEviction confirms that WebFetchCache
// enforces a hard byte-cap eviction (TS uses lru-cache with a maxSize). The
// test fills the cache beyond the documented 50MB ceiling and asserts that
// the oldest entries are evicted.
func TestWebFetchAlignment_CacheByteCapEviction(t *testing.T) {
	c := NewWebFetchCacheWithTTL(time.Hour, 0)
	defer c.Stop()

	// Insert 60 entries each ~1MB so total ≈ 60MB, exceeding the documented
	// 50MB ceiling enforced by the TS lru-cache wrapper.
	const entryBytes = 1 * 1024 * 1024
	const n = 60
	body := strings.Repeat("a", entryBytes)
	for i := 0; i < n; i++ {
		key := WebFetchCacheKey("https://example.com/"+pad(i), "p")
		c.Set(key, WebFetchCacheEntry{
			Body:  body,
			Bytes: entryBytes,
		})
	}

	if got := c.Len(); got >= n {
		t.Fatalf("cache should evict oldest entries when total bytes exceed 50MB ceiling; "+
			"inserted %d entries (%dMB total), Len=%d (no eviction observed)",
			n, n*entryBytes/1024/1024, got)
	}
}

// TestWebFetchAlignment_PreapprovedHostDoesNotExpandSubdomains pins exact TS
// hostname-only allowlist semantics.
func TestWebFetchAlignment_PreapprovedHostDoesNotExpandSubdomains(t *testing.T) {
	cases := []string{
		"https://typed.docs.python.org/3/library/os.html",
		"https://stable.go.dev/doc/effective_go",
	}
	for _, u := range cases {
		if IsPreapprovedHost(u) {
			t.Fatalf("unexpected implicit subdomain preapproval for %q", u)
		}
	}
}

// TestWebFetchAlignment_HTMLToMarkdownTable verifies that the HTML→Markdown
// converter renders a simple <table> as a markdown pipe table. The current
// converter strips <table>/<tr>/<td> via the generic-tag regex, dropping the
// structure entirely.
func TestWebFetchAlignment_HTMLToMarkdownTable(t *testing.T) {
	html := `<table><thead><tr><th>A</th><th>B</th></tr></thead>` +
		`<tbody><tr><td>1</td><td>2</td></tr></tbody></table>`
	md := HTMLToMarkdown(html)
	if !strings.Contains(md, "| A | B |") {
		t.Fatalf("expected markdown pipe-table header `| A | B |` from <table>; got: %q", md)
	}
	if !strings.Contains(md, "| 1 | 2 |") {
		t.Fatalf("expected markdown pipe-table row `| 1 | 2 |`; got: %q", md)
	}
}

// TestWebFetchAlignment_CacheEntryHasContentLengthField confirms that the
// cache entry struct surfaces a ContentLength field (mirroring the
// TS lru-cache size accounting). Currently the struct only has Bytes which
// is a generic counter, not the HTTP-style Content-Length.
func TestWebFetchAlignment_CacheEntryHasContentLengthField(t *testing.T) {
	entryType := reflect.TypeOf(WebFetchCacheEntry{})
	if _, ok := entryType.FieldByName("ContentLength"); !ok {
		fields := make([]string, 0, entryType.NumField())
		for i := 0; i < entryType.NumField(); i++ {
			fields = append(fields, entryType.Field(i).Name)
		}
		t.Fatalf("WebFetchCacheEntry must expose ContentLength for byte-cap accounting; got fields=%v", fields)
	}
}

// TestWebFetchAlignment_RedirectMessageFormat asserts that the
// webFetchStructuredPayload also carries explicit RedirectURL/RedirectCode
// fields (mirroring TS) so renderers can show "Redirected to X" properly,
// rather than relying on free-text in Summary.
func TestWebFetchAlignment_RedirectMessageFormat(t *testing.T) {
	payloadType := reflect.TypeOf(webFetchStructuredPayload{})
	missing := []string{}
	for _, name := range []string{"RedirectURL", "RedirectCode"} {
		if _, ok := payloadType.FieldByName(name); !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("webFetchStructuredPayload must expose %v for redirect handling; missing", missing)
	}
}

// blockText extracts the Text payload from a content block when present.
// Used by the alignment tests above without coupling to internal types.
func blockText(blk types.ContentBlock) string {
	if tb, ok := blk.(types.TextBlock); ok {
		return tb.Text
	}
	rv := reflect.ValueOf(blk)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Struct {
		f := rv.FieldByName("Text")
		if f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
	}
	return ""
}

func describeBlocks(blocks []types.ContentBlock) string {
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, reflect.TypeOf(b).String())
	}
	return strings.Join(out, ",")
}
