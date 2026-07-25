package skills

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCatalogIdentityFilesystemLocatorAndSymlink(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(realDir, "SKILL.md")
	if err := os.WriteFile(realFile, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	direct, err := canonicalFilesystemSkillLocator(filepath.Join(dir, ".", "real", "..", "real", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(realFile)
	if err != nil {
		t.Fatal(err)
	}
	want, err = filepath.Abs(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(direct) != filepath.Clean(want) {
		t.Fatalf("canonical locator = %q, want %q", direct, filepath.Clean(want))
	}

	link := filepath.Join(dir, "alias.md")
	if err := os.Symlink(realFile, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	alias, err := canonicalFilesystemSkillLocator(link)
	if err != nil {
		t.Fatal(err)
	}
	if alias != direct {
		t.Fatalf("symlink locator = %q, direct = %q", alias, direct)
	}

	missing := filepath.Join(dir, "missing", "SKILL.md")
	missingLocator, err := canonicalFilesystemSkillLocator(missing)
	if err != nil {
		t.Fatalf("deleted/missing path must remain canonicalizable: %v", err)
	}
	if !filepath.IsAbs(string(missingLocator)) || string(missingLocator) != filepath.Clean(missing) {
		t.Fatalf("missing locator = %q", missingLocator)
	}
}

func TestCatalogIdentityVirtualLocatorCanonicalization(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want SkillLocator
	}{
		{
			name: "hierarchical MCP resource",
			raw:  "SKILL://Review.EXAMPLE/catalog/./draft/../review/%53KILL.md?body=%7e",
			want: "skill://review.example/catalog/review/SKILL.md?body=~",
		},
		{name: "opaque resource", raw: "MCP:server/review%2fbody", want: "mcp:server/review%2Fbody"},
		{name: "escaped separator remains escaped", raw: "skill://srv/a%2fb", want: "skill://srv/a%2Fb"},
		{name: "trailing slash remains significant", raw: "skill://srv/a/./", want: "skill://srv/a/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := CanonicalVirtualSkillLocator(test.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("locator = %q, want %q", got, test.want)
			}
		})
	}

	for _, raw := range []string{
		"", "  ", "review", "/local/SKILL.md", "skill://srv/review#section", "skill://",
		"skill://srv/review?body=%zz",
	} {
		t.Run("reject_"+strings.ReplaceAll(raw, "/", "_"), func(t *testing.T) {
			if _, err := CanonicalVirtualSkillLocator(raw); !errors.Is(err, ErrInvalidSkillLocator) {
				t.Fatalf("error = %v, want ErrInvalidSkillLocator", err)
			}
		})
	}
}

func TestCatalogIdentitySourceSelectsLocatorModel(t *testing.T) {
	virtual, err := CanonicalSkillLocator(SourceMCP, "skill://SERVER/review/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if virtual != "skill://server/review/SKILL.md" {
		t.Fatalf("MCP locator = %q", virtual)
	}

	local, err := CanonicalSkillLocator(SourceProject, filepath.Join("skills", "review", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(string(local)) {
		t.Fatalf("project locator is not absolute: %q", local)
	}
	if _, err := canonicalFilesystemSkillLocator("skill://server/review"); !errors.Is(err, ErrInvalidSkillLocator) {
		t.Fatalf("ambiguous filesystem locator error = %v", err)
	}
	if _, err := CanonicalSkillLocator(SkillSource("unknown"), "anything"); !errors.Is(err, ErrInvalidSkillLocator) {
		t.Fatalf("unknown source error = %v", err)
	}
}

func TestCatalogIdentityStableSourceAwareSkillID(t *testing.T) {
	locator, err := canonicalFilesystemSkillLocator(filepath.Join(t.TempDir(), "review", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := ComputeSkillID(SourceProject, locator)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ComputeSkillID(SourceProject, locator)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("stable IDs differ: %q != %q", first, second)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("computed ID violates contract: %v", err)
	}
	wantPrefix := "skill:project:"
	if !strings.HasPrefix(string(first), wantPrefix) || len(first) != len(wantPrefix)+64 {
		t.Fatalf("ID wire format = %q", first)
	}

	user, err := ComputeSkillID(SourceUser, locator)
	if err != nil {
		t.Fatal(err)
	}
	if user == first {
		t.Fatal("same locator at different sources produced the same ID")
	}
	other, err := ComputeSkillID(SourceProject, SkillLocator(string(locator)+"-other"))
	if err != nil {
		t.Fatal(err)
	}
	if other == first {
		t.Fatal("different locators produced the same ID")
	}

	if _, err := ComputeSkillID(SkillSource("unknown"), locator); !errors.Is(err, ErrInvalidSkillID) {
		t.Fatalf("unknown source error = %v, want ErrInvalidSkillID", err)
	}
	if _, err := ComputeSkillID(SourceProject, ""); !errors.Is(err, ErrInvalidSkillLocator) {
		t.Fatalf("empty locator error = %v, want ErrInvalidSkillLocator", err)
	}
}

func TestSkillDigestUsesExactEffectiveContent(t *testing.T) {
	const content = "abc"
	const want SkillDigest = "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := ComputeSkillDigest(content); got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
	if err := ComputeSkillDigest(content).Validate(); err != nil {
		t.Fatalf("digest violates contract: %v", err)
	}
	variants := []string{
		"abc\n",
		"abc\r\n",
		" abc",
		"---\ndescription: x\n---\nabc",
		"abd",
	}
	for _, variant := range variants {
		if ComputeSkillDigest(variant) == want {
			t.Fatalf("content change %q did not change digest", variant)
		}
	}
}

func TestCatalogIdentitySkillRevisionInputDeterminism(t *testing.T) {
	locator, err := canonicalFilesystemSkillLocator(filepath.Join(t.TempDir(), "review", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := ComputeSkillID(SourceProject, locator)
	if err != nil {
		t.Fatal(err)
	}
	base := EffectiveSkill{
		ID:                 id,
		Name:               "review",
		Summary:            "Review changes",
		Source:             SourceProject,
		Locator:            locator,
		Digest:             ComputeSkillDigest("body"),
		Visibility:         VisibilityAuto,
		VisibilitySource:   SkillScopeProject,
		ModelVisible:       true,
		DescriptionVisible: true,
		UserInvocable:      true,
		Executable:         true,
		Mutable:            true,
	}
	first, err := skillRevisionInput(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := skillRevisionInput(base)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("same effective state produced different revision material")
	}
	first[0] ^= 0xff
	third, err := skillRevisionInput(base)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) == string(third) {
		t.Fatal("caller mutation affected future revision material")
	}

	withRevision := base
	withRevision.Revision = 99
	baseFingerprint, err := skillRevisionFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	withRevisionFingerprint, err := skillRevisionFingerprint(withRevision)
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint != withRevisionFingerprint {
		t.Fatal("current revision was included in its own revision input")
	}
	if len(baseFingerprint) != 64 || baseFingerprint != strings.ToLower(baseFingerprint) {
		t.Fatalf("revision fingerprint = %q", baseFingerprint)
	}

	changed := base
	changed.Summary = "Review all changes"
	changedFingerprint, err := skillRevisionFingerprint(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedFingerprint == baseFingerprint {
		t.Fatal("effective metadata change did not change revision fingerprint")
	}

	invalid := base
	invalid.Digest = "sha256:not-canonical"
	if _, err := skillRevisionInput(invalid); !errors.Is(err, ErrInvalidSkillDigest) {
		t.Fatalf("invalid input error = %v, want ErrInvalidSkillDigest", err)
	}
}

func TestCatalogIdentityRevisionInputFieldBoundariesDoNotCollide(t *testing.T) {
	left := make([]byte, 0)
	left = appendIdentityField(left, "a")
	left = appendIdentityField(left, "bc")
	right := make([]byte, 0)
	right = appendIdentityField(right, "ab")
	right = appendIdentityField(right, "c")
	if string(left) == string(right) {
		t.Fatal("length-prefixed fields collided")
	}
}
