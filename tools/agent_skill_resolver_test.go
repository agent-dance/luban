package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/skills"
)

func TestResolveSkillName_NilManagerReturnsFalse(t *testing.T) {
	if _, ok := ResolveSkillName(nil, "foo"); ok {
		t.Fatalf("nil manager should not resolve")
	}
}

func TestResolveSkillName_DirectMatch(t *testing.T) {
	mgr := newAgentSkillTestManager(t, map[string]string{
		"foo": "name: foo\ndescription: a foo skill\n",
	})
	if got, ok := ResolveSkillName(mgr, "foo"); !ok || got == nil || got.Name != "foo" {
		t.Fatalf("expected direct match for foo, got=%v ok=%v", got, ok)
	}
}

func TestResolveSkillName_PluginPrefixMatch(t *testing.T) {
	mgr := newAgentSkillTestManager(t, map[string]string{
		"plugin-a:foo": "name: plugin-a:foo\ndescription: namespaced foo\n",
	})
	if got, ok := ResolveSkillName(mgr, "foo"); !ok || got == nil || got.Name != "plugin-a:foo" {
		t.Fatalf("expected plugin-prefix match, got=%v ok=%v", got, ok)
	}
}

func TestResolveSkillName_SuffixMatchSlash(t *testing.T) {
	mgr := newAgentSkillTestManager(t, map[string]string{
		"namespace/bar": "name: namespace/bar\ndescription: slash namespaced\n",
	})
	if got, ok := ResolveSkillName(mgr, "bar"); !ok || got == nil || got.Name != "namespace/bar" {
		t.Fatalf("expected slash-suffix match, got=%v ok=%v", got, ok)
	}
}

func TestResolveSkillName_NoMatchReturnsFalse(t *testing.T) {
	mgr := newAgentSkillTestManager(t, map[string]string{
		"baz": "name: baz\ndescription: baz\n",
	})
	if got, ok := ResolveSkillName(mgr, "qux"); ok || got != nil {
		t.Fatalf("expected no match for qux, got=%v ok=%v", got, ok)
	}
}

// newAgentSkillTestManager seeds a skills.Manager with synthetic skill
// directories. Each entry's key becomes the skill folder name + the
// SKILL.md contents. The frontmatter must include name+description so
// the loader keeps it.
func newAgentSkillTestManager(t *testing.T, files map[string]string) *skills.Manager {
	t.Helper()
	dir := t.TempDir()
	for skillName, body := range files {
		// Sanitize for filesystem.
		safe := skillName
		for _, c := range []string{":", "/", "\\"} {
			safe = sanitizeReplaceAll(safe, c, "__")
		}
		skillDir := filepath.Join(dir, safe)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		full := "---\n" + body + "---\nbody\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(full), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return skills.NewManager(skills.DirSource{Dir: dir, Source: skills.SourceProject})
}

func sanitizeReplaceAll(s, old, new string) string {
	out := ""
	for i := 0; i < len(s); i++ {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			out += new
			i += len(old) - 1
			continue
		}
		out += string(s[i])
	}
	return out
}
