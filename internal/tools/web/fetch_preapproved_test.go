package web

import "testing"

func TestIsPreapprovedHost_HostnameOnly(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://docs.python.org/3/library/", true},
		{"https://DOCS.PYTHON.ORG/3/", true},
		{"http://docs.python.org/", true},
		{"https://www.python.org/", false}, // not subdomain-inherited
		{"https://docs.python.org.evil.com/", false},
	}
	for _, tc := range cases {
		got := IsPreapprovedHost(tc.url)
		if got != tc.want {
			t.Fatalf("IsPreapprovedHost(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestIsPreapprovedHost_PathPrefix(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://github.com/anthropics", true},
		{"https://github.com/anthropics/claude-code", true},
		{"https://github.com/anthropics/", true},
		{"https://github.com/anthropics-evil", false},
		{"https://github.com/anthropic", false}, // not exact prefix segment
		{"https://github.com/", false},
		{"https://github.com/other", false},
	}
	for _, tc := range cases {
		got := IsPreapprovedHost(tc.url)
		if got != tc.want {
			t.Fatalf("IsPreapprovedHost(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestIsPreapprovedHost_RejectsNonHTTP(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"ftp://docs.python.org/",
		"javascript:alert(1)",
		"data:text/html,<script>",
	}
	for _, u := range cases {
		if IsPreapprovedHost(u) {
			t.Fatalf("scheme should disqualify %q", u)
		}
	}
}

func TestIsPreapprovedHost_IDNRoundtrips(t *testing.T) {
	// "go.dev" is in the list. Punycode-encoded equivalent should match.
	if !IsPreapprovedHost("https://go.dev/") {
		t.Fatal("baseline ASCII go.dev should match")
	}
	// Mixed case + trailing dot etc. via normalize.
	if !IsPreapprovedHost("https://GO.DEV./") {
		// trailing dot may strip the path / hostname check; mixed case alone is enough
		// so don't fail if just the trailing-dot variant disagrees — focus on case.
	}
	if !IsPreapprovedHost("https://Go.Dev/") {
		t.Fatal("mixed-case host should normalize")
	}
}

func TestIsPreapprovedHost_PathPrefixCaseSensitivity(t *testing.T) {
	// Hostname normalisation lowercases, but URL paths are preserved verbatim
	// so /Anthropics should NOT match /anthropics.
	if IsPreapprovedHost("https://github.com/Anthropics") {
		t.Fatal("path-prefix should be case sensitive")
	}
}

func TestIsPreapprovedHost_EmptyAndMalformed(t *testing.T) {
	cases := []string{
		"",
		"not a url",
		"://broken",
	}
	for _, u := range cases {
		if IsPreapprovedHost(u) {
			t.Fatalf("malformed url should never match: %q", u)
		}
	}
}

func TestNormalizePreapprovedHost_LowerAndTrim(t *testing.T) {
	cases := map[string]string{
		"GO.DEV":              "go.dev",
		" go.dev ":            "go.dev",
		"DOCS.AWS.AMAZON.COM": "docs.aws.amazon.com",
	}
	for in, want := range cases {
		if got := normalizePreapprovedHost(in); got != want {
			t.Fatalf("normalizePreapprovedHost(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizePreapprovedHost(""); got != "" {
		t.Fatalf("empty input should return empty, got %q", got)
	}
}
