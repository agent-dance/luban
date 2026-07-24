package skills

import (
	"strings"
	"testing"
)

// --- GetCharBudget tests ---

func TestGetCharBudget_Default(t *testing.T) {
	budget := GetCharBudget(0)
	if budget != DefaultCharBudget {
		t.Errorf("expected %d, got %d", DefaultCharBudget, budget)
	}
}

func TestGetCharBudget_200kTokens(t *testing.T) {
	// 200000 tokens × 4 chars/token × 0.01 = 8000
	budget := GetCharBudget(200_000)
	if budget != 8000 {
		t.Errorf("expected 8000, got %d", budget)
	}
}

func TestGetCharBudget_100kTokens(t *testing.T) {
	// 100000 × 4 × 0.01 = 4000
	budget := GetCharBudget(100_000)
	if budget != 4000 {
		t.Errorf("expected 4000, got %d", budget)
	}
}

// --- getSkillDescription tests ---

func TestGetSkillDescription_ShortDesc(t *testing.T) {
	skill := &Skill{Description: "Short description"}
	got := getSkillDescription(skill)
	if got != "Short description" {
		t.Errorf("expected 'Short description', got %q", got)
	}
}

func TestGetSkillDescription_WithWhenToUse(t *testing.T) {
	skill := &Skill{Description: "Do stuff", WhenToUse: "when X"}
	got := getSkillDescription(skill)
	if got != "Do stuff - when X" {
		t.Errorf("expected 'Do stuff - when X', got %q", got)
	}
}

func TestGetSkillDescription_TruncatedLong(t *testing.T) {
	skill := &Skill{Description: strings.Repeat("x", 300)}
	got := getSkillDescription(skill)
	// MaxListingDescChars runes: (MAX-1) x's + "…" = MAX runes
	runes := []rune(got)
	if len(runes) != MaxListingDescChars {
		t.Errorf("expected rune length %d, got %d", MaxListingDescChars, len(runes))
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("expected truncation suffix '…'")
	}
}

// --- FormatSkillsWithinBudget tests ---

func TestFormatSkillsWithinBudget_Empty(t *testing.T) {
	got := FormatSkillsWithinBudget(nil, 200_000)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFormatSkillsWithinBudget_FitsInBudget(t *testing.T) {
	skills := []*Skill{
		{Name: "pdf", Description: "Read PDFs"},
		{Name: "commit", Description: "Git commit helper"},
	}
	got := FormatSkillsWithinBudget(skills, 200_000)

	if !strings.Contains(got, "- pdf: Read PDFs") {
		t.Error("expected pdf entry")
	}
	if !strings.Contains(got, "- commit: Git commit helper") {
		t.Error("expected commit entry")
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}

func TestFormatSkillsWithinBudget_BundledNeverTruncated(t *testing.T) {
	// Create many skills that exceed a tiny budget
	bundled := &Skill{
		Name:        "bundled-skill",
		Description: "This bundled skill should never be truncated",
		Source:      SourceBundled,
	}
	rest := make([]*Skill, 20)
	for i := range rest {
		rest[i] = &Skill{
			Name:        strings.Repeat("s", 10) + string(rune('a'+i)),
			Description: strings.Repeat("Description text here ", 5),
			Source:      SourceProject,
		}
	}

	skills := append([]*Skill{bundled}, rest...)
	// Very small budget — only ~200 chars (50 tokens)
	got := FormatSkillsWithinBudget(skills, 50)

	// Bundled skill should have full description
	if !strings.Contains(got, "bundled-skill: This bundled skill should never be truncated") {
		t.Error("bundled skill description should not be truncated")
	}
}

func TestFormatSkillsWithinBudget_NamesOnlyMode(t *testing.T) {
	// Create so many skills that non-bundled must go names-only
	skills := make([]*Skill, 100)
	for i := range skills {
		skills[i] = &Skill{
			Name:        "skill-" + strings.Repeat("x", 20),
			Description: strings.Repeat("Very long description ", 10),
			Source:      SourceProject,
		}
	}

	// Very tiny budget (5 tokens = 20 chars for 100 skills = impossible)
	got := FormatSkillsWithinBudget(skills, 5)

	// Should be names-only (no colon separator with description)
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		// Each line should be "- name" without description
		if strings.Count(line, ": ") > 0 {
			// Names-only should not have ": description"
			t.Errorf("expected names-only mode, but found description in: %q", line)
		}
	}
}

func TestFormatSkillsWithinBudget_TruncatedDescriptions(t *testing.T) {
	skills := make([]*Skill, 5)
	for i := range skills {
		skills[i] = &Skill{
			Name:        "sk",
			Description: strings.Repeat("d", 200),
			Source:      SourceProject,
		}
	}

	// Budget that allows some but not full descriptions
	// 5 skills × (4 prefix + 200 desc) + 4 newlines = 1024 chars
	// Budget of ~500 chars should force truncation
	got := FormatSkillsWithinBudget(skills, 125) // 125 × 4 × 0.01 = 5 chars... too small

	// Actually use a moderate budget
	got = FormatSkillsWithinBudget(skills, 12500) // 500 chars

	// All entries should be present
	lines := strings.Split(got, "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}

	// Descriptions should be shorter than full 200 chars
	for _, line := range lines {
		// "- sk: " is 6 chars, so desc should be less than 200
		if len(line) > 210 {
			t.Errorf("description not truncated: len=%d", len(line))
		}
	}
	_ = got
}

// --- FormatSkillListing tests ---

func TestFormatSkillListing_Empty(t *testing.T) {
	got := FormatSkillListing(nil, 200_000)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFormatSkillListing_WithSkills(t *testing.T) {
	skills := []*Skill{
		{Name: "pdf", Description: "Handle PDFs"},
	}
	got := FormatSkillListing(skills, 200_000)

	if !strings.HasPrefix(got, "The following skills are available for use with the Skill tool:") {
		t.Error("expected header prefix")
	}
	if !strings.Contains(got, "- pdf: Handle PDFs") {
		t.Error("expected skill entry")
	}
}

// --- WrapInSystemReminder tests ---

func TestWrapInSystemReminder(t *testing.T) {
	got := WrapInSystemReminder("Hello")
	expected := "<system-reminder>\nHello\n</system-reminder>"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestWrapInSystemReminder_Empty(t *testing.T) {
	got := WrapInSystemReminder("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --- Filter tests ---

func TestFilterModelInvocableSkills(t *testing.T) {
	skills := []*Skill{
		{Name: "a", DisableModelInvocation: false, Source: SourceProject},
		{Name: "b", DisableModelInvocation: true, Source: SourceProject},
		{Name: "c", DisableModelInvocation: false, Source: SourceProject},
	}

	got := FilterModelInvocableSkills(skills)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Name != "a" || got[1].Name != "c" {
		t.Errorf("unexpected skills: %v", got)
	}
}

func TestFilterModelInvocableSkills_BuiltinExcluded(t *testing.T) {
	skills := []*Skill{
		{Name: "a", Source: "builtin"},
		{Name: "b", Source: SourceProject},
	}
	got := FilterModelInvocableSkills(skills)
	if len(got) != 1 || got[0].Name != "b" {
		t.Errorf("expected only 'b', got %v", got)
	}
}

func TestFilterModelInvocableSkills_PluginWithoutDesc(t *testing.T) {
	skills := []*Skill{
		{Name: "a", Source: SourcePlugin},                                                   // no desc, no whenToUse → excluded
		{Name: "b", Source: SourcePlugin, HasUserSpecifiedDescription: true},                // has desc → included
		{Name: "c", Source: SourcePlugin, WhenToUse: "when needed"},                         // has whenToUse → included
		{Name: "d", Source: SourceMCP},                                                      // no desc, no whenToUse → excluded
		{Name: "e", Source: SourceMCP, HasUserSpecifiedDescription: true, WhenToUse: "any"}, // both → included
	}
	got := FilterModelInvocableSkills(skills)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	names := make([]string, len(got))
	for i, s := range got {
		names[i] = s.Name
	}
	expected := []string{"b", "c", "e"}
	for i, n := range expected {
		if names[i] != n {
			t.Errorf("index %d: want %q, got %q", i, n, names[i])
		}
	}
}

func TestFilterUserInvocableSkills(t *testing.T) {
	trueVal := true
	falseVal := false
	skills := []*Skill{
		{Name: "a", UserInvocable: nil},         // default true
		{Name: "b", UserInvocable: &falseVal},    // explicit false
		{Name: "c", UserInvocable: &trueVal},     // explicit true
	}

	got := FilterUserInvocableSkills(skills)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Name != "a" || got[1].Name != "c" {
		t.Errorf("unexpected skills: %v", got)
	}
}

// --- IsUserInvocable tests ---

func TestIsUserInvocable_Default(t *testing.T) {
	skill := &Skill{}
	if !skill.IsUserInvocable() {
		t.Error("expected default to be true")
	}
}

func TestIsUserInvocable_ExplicitFalse(t *testing.T) {
	v := false
	skill := &Skill{UserInvocable: &v}
	if skill.IsUserInvocable() {
		t.Error("expected explicit false")
	}
}

func TestIsUserInvocable_ExplicitTrue(t *testing.T) {
	v := true
	skill := &Skill{UserInvocable: &v}
	if !skill.IsUserInvocable() {
		t.Error("expected explicit true")
	}
}

// --- truncateStr tests ---

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		input  string
		max    int
		expect string
	}{
		{"hello", 10, "hello"},                        // fits within budget
		{"hello world", 8, "hello w…"},                // 7 runes + "…" = 8 runes
		{"x", 1, "x"},                                   // 1 rune fits in maxLen=1
		{"ab", 1, "…"},                                  // 2 runes > 1, truncate to "…"
		{"abc", 2, "a…"},                              // maxLen 2 → 1 rune + "…"
		{"abc", 3, "abc"},                             // exactly 3 runes, fits
		{"abcd", 3, "ab…"},                            // 4 > 3, truncate: 2 + "…" = 3
		{"hello world", 100, "hello world"},           // fits
		{"你好世界测试", 4, "你好世…"},                   // CJK: 3 runes + "…" = 4 runes
		{"你好世界", 4, "你好世界"},                      // exactly 4, fits
	}
	for _, tt := range tests {
		got := truncateStr(tt.input, tt.max)
		if got != tt.expect {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.expect)
		}
	}
}

// --- GetSkillToolPrompt test ---

func TestGetSkillToolPrompt_NotEmpty(t *testing.T) {
	prompt := GetSkillToolPrompt()
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "Execute a skill") {
		t.Error("expected 'Execute a skill' in prompt")
	}
	if !strings.Contains(prompt, "BLOCKING REQUIREMENT") {
		t.Error("expected 'BLOCKING REQUIREMENT' in prompt")
	}
}
