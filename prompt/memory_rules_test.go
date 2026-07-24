package prompt

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverMemoryFilesLoadsUnconditionalRulesAndStripsFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	rules := filepath.Join(project, ".claude", "rules")

	writeFile(t, filepath.Join(rules, "01-base.md"), "base rule\n@./shared.txt\n`@./inline.txt`\n```md\n@./fenced.txt\n```\n")
	writeFile(t, filepath.Join(rules, "02-conditional.md"), "---\npaths: src/**/*.go\n---\nconditional rule\n@./conditional-include.txt\n")
	writeFile(t, filepath.Join(rules, "shared.txt"), "shared include")
	writeFile(t, filepath.Join(rules, "inline.txt"), "inline include")
	writeFile(t, filepath.Join(rules, "fenced.txt"), "fenced include")
	writeFile(t, filepath.Join(rules, "conditional-include.txt"), "conditional include")
	writeFile(t, filepath.Join(rules, "nested", "03-nested.md"), "---\npaths: **\n---\nnested match-all rule")
	writeFile(t, filepath.Join(rules, "ignored.png"), "not text")

	files := discoverMemoryFiles(project, memoryPaths{})
	got := memoryContents(files)
	joined := strings.Join(got, "|")

	assertInOrder(t, joined, []string{"base rule", "shared include", "nested match-all rule"})
	if strings.Contains(joined, "---") {
		t.Fatalf("frontmatter should be stripped from rule content: %q", joined)
	}
	for _, forbidden := range []string{"conditional rule", "conditional include", "inline include", "fenced include", "not text"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("eager rules unexpectedly included %q in %q", forbidden, joined)
		}
	}
}

func TestDiscoverMemoryFilesForTargetLoadsMatchingConditionalRulesWithIncludes(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	rules := filepath.Join(project, ".claude", "rules")
	writeFile(t, filepath.Join(rules, "go.md"), "---\npaths: src/**/*.go\n---\ngo rule\n@./go-extra.txt\n")
	writeFile(t, filepath.Join(rules, "go-extra.txt"), "go include")
	writeFile(t, filepath.Join(rules, "docs.md"), "---\npaths: docs/**\n---\ndocs rule")

	matching := DiscoverMemoryFilesForTarget(project, filepath.Join(project, "src", "main.go"))
	got := strings.Join(memoryContents(matching), "|")
	assertInOrder(t, got, []string{"go rule", "go include"})
	if strings.Contains(got, "docs rule") {
		t.Fatalf("non-matching conditional rule injected: %q", got)
	}

	nonMatching := DiscoverMemoryFilesForTarget(project, filepath.Join(project, "README.md"))
	if got := strings.Join(memoryContents(nonMatching), "|"); strings.Contains(got, "go rule") || strings.Contains(got, "docs rule") {
		t.Fatalf("conditional rules injected for non-matching target: %q", got)
	}
}

func TestIncludeExpansionSupportsPathFormsAndSkipsExternalUnlessAllowed(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	home := filepath.Join(tmp, "home")
	external := filepath.Join(tmp, "external.txt")
	homeFile := filepath.Join(home, "home.txt")
	absolute := filepath.Join(project, "absolute.txt")
	main := filepath.Join(project, "CLAUDE.md")

	writeFile(t, external, "external include")
	writeFile(t, homeFile, "home include")
	writeFile(t, absolute, "absolute include")
	writeFile(t, filepath.Join(project, "relative.txt"), "relative include")
	writeFile(t, main, strings.Join([]string{
		"main",
		"@relative.txt",
		"@./relative.txt",
		"@~/" + filepath.Base(homeFile),
		"@" + absolute,
		"@" + external,
	}, "\n"))

	oldHome := osUserHomeDir
	osUserHomeDir = func() (string, error) { return home, nil }
	defer func() { osUserHomeDir = oldHome }()

	skipped := processMemoryFile(main, MemoryTypeProject, nil, false, 0, "", project)
	gotSkipped := strings.Join(memoryContents(skipped), "|")
	for _, want := range []string{"main", "relative include", "absolute include"} {
		if !strings.Contains(gotSkipped, want) {
			t.Fatalf("include result missing %q: %q", want, gotSkipped)
		}
	}
	for _, forbidden := range []string{"home include", "external include"} {
		if strings.Contains(gotSkipped, forbidden) {
			t.Fatalf("external include %q should be skipped without approval: %q", forbidden, gotSkipped)
		}
	}

	allowed := processMemoryFile(main, MemoryTypeProject, nil, true, 0, "", project)
	gotAllowed := strings.Join(memoryContents(allowed), "|")
	for _, want := range []string{"home include", "external include"} {
		if !strings.Contains(gotAllowed, want) {
			t.Fatalf("approved external include missing %q: %q", want, gotAllowed)
		}
	}
	externals := GetExternalClaudeMdIncludes(allowed, project)
	if len(externals) != 2 {
		t.Fatalf("external include metadata length = %d, want 2: %#v", len(externals), externals)
	}
}

func TestCircularIncludesTerminateWithoutDuplicateContent(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	a := filepath.Join(project, "a.md")
	b := filepath.Join(project, "b.md")
	writeFile(t, a, "a\n@./b.md")
	writeFile(t, b, "b\n@./a.md")

	files := processMemoryFile(a, MemoryTypeProject, nil, true, 0, "", project)
	got := strings.Join(memoryContents(files), "|")
	if strings.Count(got, "a\n@./b.md") != 1 || strings.Count(got, "b\n@./a.md") != 1 {
		t.Fatalf("cycle should include each file once, got %q", got)
	}
}
