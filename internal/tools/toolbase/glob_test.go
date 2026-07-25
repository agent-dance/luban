package toolbase

import "testing"

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{name: "basename", pattern: "*.go", path: "main.go", want: true},
		{name: "different extension", pattern: "*.go", path: "main.ts", want: false},
		{name: "globstar", pattern: "src/**/*.go", path: "src/foo/bar.go", want: true},
		{name: "globstar direct child", pattern: "src/**/*.go", path: "src/bar.go", want: true},
		{name: "root mismatch", pattern: "src/**/*.go", path: "lib/bar.go", want: false},
		{name: "brace first alternative", pattern: "src/**/*.{ts,tsx}", path: "src/a/b.ts", want: true},
		{name: "brace second alternative", pattern: "src/**/*.{ts,tsx}", path: "src/a/b.tsx", want: true},
		{name: "native bracket negation", pattern: "[^abc].txt", path: "d.txt", want: true},
		{name: "question mark", pattern: "?ello", path: "hello", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchGlob(tt.pattern, tt.path)
			if err != nil {
				t.Fatalf("MatchGlob() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("MatchGlob(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchGlobRejectsInvalidPattern(t *testing.T) {
	for _, pattern := range []string{"", "src/**/[abc"} {
		if _, err := MatchGlob(pattern, "src/main.go"); err == nil {
			t.Errorf("MatchGlob(%q) expected an error", pattern)
		}
	}
}

func TestMatchGlobRelativeTo(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		root    string
	}{
		{
			name:    "basename pattern",
			pattern: "*.go",
			path:    "/work/proj/src/main.go",
			root:    "/work/proj",
		},
		{
			name:    "rooted pattern",
			pattern: "src/**/*.go",
			path:    "/work/proj/src/foo/bar.go",
			root:    "/work/proj",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchGlobRelativeTo(tt.pattern, tt.path, tt.root)
			if err != nil {
				t.Fatalf("MatchGlobRelativeTo() error = %v", err)
			}
			if !got {
				t.Fatalf("MatchGlobRelativeTo(%q, %q, %q) = false", tt.pattern, tt.path, tt.root)
			}
		})
	}
}

func TestMatchGlobDoesNotTranslateHistoricalSyntax(t *testing.T) {
	got, err := MatchGlob("+(foo|bar).go", "foo.go")
	if err != nil {
		t.Fatalf("MatchGlob() error = %v", err)
	}
	if got {
		t.Fatal("extglob syntax must not be translated to brace expansion")
	}

	got, err = MatchGlob("!**/node_modules/**", "src/index.ts")
	if err != nil {
		t.Fatalf("MatchGlob() error = %v", err)
	}
	if got {
		t.Fatal("leading ! must not be interpreted as whole-pattern negation")
	}
}
