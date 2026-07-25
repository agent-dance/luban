// Package tools — preapproved-host allowlist for WebFetch.
//
// Mirrors src/tools/WebFetchTool/preapproved.ts verbatim: GET-only fetches
// to these hosts skip per-domain user confirmation. Path-prefix entries
// (e.g., "github.com/anthropics") only match the prefix or a path under it,
// never an unrelated path that happens to share the prefix string.
//
// SECURITY: This list is exclusively for WebFetch GET requests. The sandbox
// system intentionally does NOT inherit this list — many entries (huggingface,
// kaggle, nuget) accept uploads, so unrestricted egress would enable exfil.
package web

import (
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

// preapprovedHostEntries mirrors PREAPPROVED_HOSTS from the TS reference,
// in declaration order, including comment grouping.
var preapprovedHostEntries = []string{
	// Anthropic
	"platform.claude.com",
	"code.claude.com",
	"modelcontextprotocol.io",
	"github.com/anthropics",
	"agentskills.io",

	// Top Programming Languages
	"docs.python.org",
	"en.cppreference.com",
	"docs.oracle.com",
	"learn.microsoft.com",
	"developer.mozilla.org",
	"go.dev",
	"pkg.go.dev",
	"www.php.net",
	"docs.swift.org",
	"kotlinlang.org",
	"ruby-doc.org",
	"doc.rust-lang.org",
	"www.typescriptlang.org",

	// Web & JavaScript Frameworks/Libraries
	"react.dev",
	"angular.io",
	"vuejs.org",
	"nextjs.org",
	"expressjs.com",
	"nodejs.org",
	"bun.sh",
	"jquery.com",
	"getbootstrap.com",
	"tailwindcss.com",
	"d3js.org",
	"threejs.org",
	"redux.js.org",
	"webpack.js.org",
	"jestjs.io",
	"reactrouter.com",

	// Python Frameworks & Libraries
	"docs.djangoproject.com",
	"flask.palletsprojects.com",
	"fastapi.tiangolo.com",
	"pandas.pydata.org",
	"numpy.org",
	"www.tensorflow.org",
	"pytorch.org",
	"scikit-learn.org",
	"matplotlib.org",
	"requests.readthedocs.io",
	"jupyter.org",

	// PHP Frameworks
	"laravel.com",
	"symfony.com",
	"wordpress.org",

	// Java Frameworks & Libraries
	"docs.spring.io",
	"hibernate.org",
	"tomcat.apache.org",
	"gradle.org",
	"maven.apache.org",

	// .NET & C# Frameworks
	"asp.net",
	"dotnet.microsoft.com",
	"nuget.org",
	"blazor.net",

	// Mobile Development
	"reactnative.dev",
	"docs.flutter.dev",
	"developer.apple.com",
	"developer.android.com",

	// Data Science & Machine Learning
	"keras.io",
	"spark.apache.org",
	"huggingface.co",
	"www.kaggle.com",

	// Databases
	"www.mongodb.com",
	"redis.io",
	"www.postgresql.org",
	"dev.mysql.com",
	"www.sqlite.org",
	"graphql.org",
	"prisma.io",

	// Cloud & DevOps
	"docs.aws.amazon.com",
	"cloud.google.com",
	// "learn.microsoft.com" already listed above (also serves Azure docs).
	"kubernetes.io",
	"www.docker.com",
	"www.terraform.io",
	"www.ansible.com",
	"vercel.com/docs",
	"docs.netlify.com",
	"devcenter.heroku.com",

	// Testing & Monitoring
	"cypress.io",
	"selenium.dev",

	// Game Development
	"docs.unity.com",
	"docs.unrealengine.com",

	// Other Essential Tools
	"git-scm.com",
	"nginx.org",
	"httpd.apache.org",
}

var (
	preapprovedHostnameOnly = make(map[string]struct{}, len(preapprovedHostEntries))
	preapprovedPathPrefixes = make(map[string][]string)
)

func init() {
	for _, entry := range preapprovedHostEntries {
		if i := strings.IndexByte(entry, '/'); i >= 0 {
			host := entry[:i]
			path := entry[i:]
			preapprovedPathPrefixes[host] = append(preapprovedPathPrefixes[host], path)
		} else {
			preapprovedHostnameOnly[entry] = struct{}{}
		}
	}
}

// IsPreapprovedHost reports whether a URL points at a preapproved host.
// Hostname matching is case-insensitive and IDN/punycode-normalised. Bare
// hostname entries are exact: the TS allowlist does not grant implicit access
// to subdomains. Path-prefix entries enforce segment boundaries:
// "/anthropics" matches itself or "/anthropics/foo", never
// "/anthropics-evil".
func IsPreapprovedHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := normalizePreapprovedHost(u.Hostname())
	if host == "" {
		return false
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	if _, ok := preapprovedHostnameOnly[host]; ok {
		return true
	}
	if prefixes, ok := preapprovedPathPrefixes[host]; ok {
		for _, p := range prefixes {
			if path == p || strings.HasPrefix(path, p+"/") {
				return true
			}
		}
	}
	return false
}

// normalizePreapprovedHost lower-cases and runs IDN/punycode through ToASCII
// so unicode hosts and their punycode encodings collapse to one canonical
// form. Empty input returns "".
func normalizePreapprovedHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if ascii, err := idna.Lookup.ToASCII(host); err == nil {
		return ascii
	}
	return host
}
