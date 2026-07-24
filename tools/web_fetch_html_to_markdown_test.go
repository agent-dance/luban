package tools

import (
	"strings"
	"testing"
)

func TestHTMLToMarkdown_StripsScriptStyleNavFooter(t *testing.T) {
	in := `<html><head><title>x</title></head><body>
<nav>menu</nav>
<script>alert(1)</script>
<style>.a{color:red}</style>
<noscript>fallback</noscript>
<p>Hello world</p>
<footer>copyright</footer>
</body></html>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "Hello world") {
		t.Fatalf("paragraph dropped: %q", got)
	}
	for _, banned := range []string{"alert(1)", "color:red", "menu", "copyright", "fallback"} {
		if strings.Contains(got, banned) {
			t.Fatalf("expected to strip %q, got %q", banned, got)
		}
	}
}

func TestHTMLToMarkdown_PreservesPreCodeWithLanguageHint(t *testing.T) {
	in := `<pre><code class="language-go">package main</code></pre>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "```go") {
		t.Fatalf("missing language hint, got %q", got)
	}
	if !strings.Contains(got, "package main") {
		t.Fatalf("code body lost, got %q", got)
	}
}

func TestHTMLToMarkdown_PreserveCodeOnPreClass(t *testing.T) {
	in := `<pre class="language-python"><code>print("hi")</code></pre>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "```python") {
		t.Fatalf("expected python fence, got %q", got)
	}
	if !strings.Contains(got, `print("hi")`) {
		t.Fatalf("code body lost: %q", got)
	}
}

func TestHTMLToMarkdown_InlineCodeBacktick(t *testing.T) {
	in := `<p>Use <code>foo()</code> here.</p>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "`foo()`") {
		t.Fatalf("inline code not preserved as backticks: %q", got)
	}
}

func TestHTMLToMarkdown_HeadingsHaveCorrectDepth(t *testing.T) {
	in := `<h1>Top</h1><h2>Sub</h2><h6>Tiny</h6>`
	got := HTMLToMarkdown(in)
	for _, want := range []string{"# Top", "## Sub", "###### Tiny"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestHTMLToMarkdown_OrderedAndUnorderedLists(t *testing.T) {
	in := `<ul><li>one</li><li>two</li></ul><ol><li>first</li><li>second</li></ol>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "- one") || !strings.Contains(got, "- two") {
		t.Fatalf("ul not rendered: %q", got)
	}
	if !strings.Contains(got, "1. first") || !strings.Contains(got, "2. second") {
		t.Fatalf("ol not numbered: %q", got)
	}
}

func TestHTMLToMarkdown_AnchorsBecomeMarkdownLinks(t *testing.T) {
	in := `<a href="https://example.com">click</a>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "[click](https://example.com)") {
		t.Fatalf("anchor not converted: %q", got)
	}
}

func TestHTMLToMarkdown_AnchorWithoutHrefSurvivesAsText(t *testing.T) {
	in := `<a href="#section">jump</a>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "jump") || strings.Contains(got, "[jump](#section)") {
		t.Fatalf("hash-only link should drop the URL: %q", got)
	}
}

func TestHTMLToMarkdown_BoldAndItalic(t *testing.T) {
	in := `<b>bold</b> <strong>also</strong> <i>italic</i> <em>too</em>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "**bold**") || !strings.Contains(got, "**also**") {
		t.Fatalf("bold not rendered: %q", got)
	}
	if !strings.Contains(got, "*italic*") || !strings.Contains(got, "*too*") {
		t.Fatalf("italic not rendered: %q", got)
	}
}

func TestHTMLToMarkdown_ImagesWithAlt(t *testing.T) {
	in := `<img alt="logo" src="https://example.com/x.png">`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "![logo](https://example.com/x.png)") {
		t.Fatalf("image alt+src not rendered: %q", got)
	}
}

func TestHTMLToMarkdown_HTMLEntitiesDecoded(t *testing.T) {
	in := `<p>foo &amp; bar &lt;tag&gt; &quot;ok&quot;</p>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "foo & bar <tag> \"ok\"") {
		t.Fatalf("entities not decoded: %q", got)
	}
}

func TestHTMLToMarkdown_NumericEntity(t *testing.T) {
	in := `<p>&#65;&#x42;</p>` // A B
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "AB") {
		t.Fatalf("numeric entities not decoded: %q", got)
	}
}

func TestHTMLToMarkdown_TruncationMarker(t *testing.T) {
	body := strings.Repeat("a", MaxMarkdownBytes+200)
	in := `<p>` + body + `</p>`
	got := HTMLToMarkdown(in)
	if !strings.HasSuffix(got, "[Content truncated due to length...]") {
		t.Fatalf("expected truncation marker, got %q", got[len(got)-60:])
	}
}

func TestHTMLToMarkdown_CustomMaxBytes(t *testing.T) {
	body := strings.Repeat("b", 200)
	in := `<p>` + body + `</p>`
	got := HTMLToMarkdownWithOptions(in, HTMLToMarkdownOptions{MaxBytes: 50})
	if !strings.HasSuffix(got, "[Content truncated due to length...]") {
		t.Fatalf("expected truncation, got %q", got)
	}
	if len(got) > 50+len("[Content truncated due to length...]")+10 {
		t.Fatalf("truncation respected? len=%d", len(got))
	}
}

func TestHTMLToMarkdown_CustomStripList(t *testing.T) {
	in := `<aside>side</aside><div class="ad">remove me</div><p>keep</p>`
	got := HTMLToMarkdownWithOptions(in, HTMLToMarkdownOptions{Strip: []string{"aside"}})
	if strings.Contains(got, "side") {
		t.Fatalf("custom strip ignored: %q", got)
	}
	if !strings.Contains(got, "keep") {
		t.Fatalf("good content removed: %q", got)
	}
}

func TestIsHTMLContentType(t *testing.T) {
	cases := map[string]bool{
		"text/html":                true,
		"text/html; charset=utf-8": true,
		"application/xhtml+xml":    true,
		"text/plain":               false,
		"application/json":         false,
		"":                         false,
	}
	for ct, want := range cases {
		if got := IsHTMLContentType(ct); got != want {
			t.Fatalf("IsHTMLContentType(%q) = %v, want %v", ct, got, want)
		}
	}
}

func TestHTMLToMarkdown_EmptyInput(t *testing.T) {
	if got := HTMLToMarkdown(""); got != "" {
		t.Fatalf("empty input should yield empty output, got %q", got)
	}
}

func TestHTMLToMarkdown_BlockquoteRendering(t *testing.T) {
	in := `<blockquote>foo bar</blockquote>`
	got := HTMLToMarkdown(in)
	if !strings.Contains(got, "> foo bar") {
		t.Fatalf("blockquote prefix missing: %q", got)
	}
}
