package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/skills"
)

// -----------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------

// makeTempSkillDir creates a temp directory, adds a skills/ sub-directory and
// returns (tempRoot, skillsDir).
func makeTempSkillDir(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	return root, skillsDir
}

// writeMDSkill writes a SKILL.md inside skillsDir/<name>/.
func writeMDSkill(t *testing.T, skillsDir, name, content string) {
	t.Helper()
	dir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// newTestSkillManager creates a skills.Manager pointing at the given directories.
func newTestSkillManager(dirs ...string) *skills.Manager {
	sources := make([]skills.DirSource, len(dirs))
	for i, d := range dirs {
		sources[i] = skills.DirSource{Dir: d, Source: skills.SourceProject}
	}
	return skills.NewManager(sources...)
}

// -----------------------------------------------------------------------
// SkillTool.Execute – prompt-type skill
// -----------------------------------------------------------------------

func TestSkillTool_ExecutePrompt(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "review", "Please review the following code carefully.")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, err := tool.Execute(context.Background(), map[string]any{
		"skill": "review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "Please review the following code carefully.") {
		t.Errorf("expected SKILL.md content in result, got: %s", res.Content)
	}
}

func TestSkillTool_ExecutePromptWithArgs(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "prompt-skill", "Base prompt with $ARGUMENTS")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, err := tool.Execute(context.Background(), map[string]any{
		"skill": "prompt-skill",
		"args":  "extra instructions",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "extra instructions") {
		t.Errorf("expected args substituted, got: %s", res.Content)
	}
}

func TestSkillTool_ExecutePromptFallbackAppend(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "plain", "No placeholder content here.")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, err := tool.Execute(context.Background(), map[string]any{
		"skill": "plain",
		"args":  "extra info",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// When no $ARGUMENTS placeholder, args are appended as "ARGUMENTS: ..."
	if !strings.Contains(res.Content, "ARGUMENTS: extra info") {
		t.Errorf("expected fallback append, got: %s", res.Content)
	}
}

func TestSkillTool_ExecutePromptWithBaseDir(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "dirskill", "Use ${CLAUDE_SKILL_DIR}/scripts/run.sh")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, err := tool.Execute(context.Background(), map[string]any{
		"skill": "dirskill",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// SkillDir should be set and ${CLAUDE_SKILL_DIR} replaced
	if strings.Contains(res.Content, "${CLAUDE_SKILL_DIR}") {
		t.Errorf("expected ${CLAUDE_SKILL_DIR} to be substituted, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "Base directory for this skill:") {
		t.Errorf("expected base dir header, got: %s", res.Content)
	}
}

// -----------------------------------------------------------------------
// SkillTool.Execute – error paths
// -----------------------------------------------------------------------

func TestSkillTool_UnknownSkillListsAvailable(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "foo", "Foo skill content")
	writeMDSkill(t, skillsDir, "bar", "Bar skill content")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, err := tool.Execute(context.Background(), map[string]any{
		"skill": "nonexistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for unknown skill")
	}
	if !strings.Contains(res.Content, "nonexistent") {
		t.Errorf("error should mention requested skill name, got: %s", res.Content)
	}
	// Should list available skills
	if !strings.Contains(res.Content, "foo") || !strings.Contains(res.Content, "bar") {
		t.Errorf("error should list available skills, got: %s", res.Content)
	}
}

func TestSkillTool_MissingSkillParam(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	res, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for missing 'skill' param")
	}
}

func TestSkillTool_EmptySkillParam(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	res, err := tool.Execute(context.Background(), map[string]any{
		"skill": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for empty 'skill' param")
	}
}

func TestSkillTool_NoSkillsInstalled(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"skill": "whatever",
	})
	if !res.IsError {
		t.Error("expected error when no skills installed")
	}
	if !strings.Contains(res.Content, "No skills are currently installed") {
		t.Errorf("expected 'No skills' message, got: %s", res.Content)
	}
}

// -----------------------------------------------------------------------
// Schema & name
// -----------------------------------------------------------------------

func TestSkillTool_NameAndDescription(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	if tool.Name() != "Skill" {
		t.Errorf("Name: got %q, want %q", tool.Name(), "Skill")
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
}

func TestSkillTool_Schema(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	schema := tool.Schema()
	if schema.Type != "object" {
		t.Errorf("Schema.Type: got %q", schema.Type)
	}
	if _, ok := schema.Properties["skill"]; !ok {
		t.Error("schema must have 'skill' property")
	}
	if _, ok := schema.Properties["args"]; !ok {
		t.Error("schema must have 'args' property")
	}
	if len(schema.Required) == 0 || schema.Required[0] != "skill" {
		t.Error("'skill' must be in Required")
	}
}

// -----------------------------------------------------------------------
// validateSkillName
// -----------------------------------------------------------------------

func TestValidateSkillName_Empty(t *testing.T) {
	if err := validateSkillName(""); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestValidateSkillName_LeadingSlashRejected(t *testing.T) {
	err := validateSkillName("/commit")
	if err == nil {
		t.Fatal("expected error for leading slash")
	}
	if !strings.Contains(err.Error(), "leading slash") {
		t.Errorf("expected 'leading slash' in error, got: %v", err)
	}
}

func TestValidateSkillName_PluginNamespaceAccepted(t *testing.T) {
	if err := validateSkillName("git:commit"); err != nil {
		t.Errorf("expected git:commit to be accepted, got: %v", err)
	}
}

func TestValidateSkillName_BadPluginNamespaceRejected(t *testing.T) {
	cases := []string{
		":commit", // empty plugin
		"git:",    // empty skill
		"a:b:c",   // too many colons
		":",       // both empty
	}
	for _, name := range cases {
		if err := validateSkillName(name); err == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}

func TestValidateSkillName_PlainNameAccepted(t *testing.T) {
	if err := validateSkillName("commit"); err != nil {
		t.Errorf("expected 'commit' to be accepted, got: %v", err)
	}
	if err := validateSkillName("review-pr"); err != nil {
		t.Errorf("expected 'review-pr' to be accepted, got: %v", err)
	}
}

// -----------------------------------------------------------------------
// SkillTool.Execute – name validation
// -----------------------------------------------------------------------

func TestSkillTool_LeadingSlashRejected(t *testing.T) {
	// TS strips leading slash and proceeds (with a tengu_skill_leading_slash
	// analytics event). Go now does the same: a "/commit" call resolves to
	// "commit" and runs successfully, with a metadata flag the harness can
	// observe.
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "commit", "commit content")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"skill": "/commit",
	})
	if res.IsError {
		t.Fatalf("expected leading slash to be stripped, got error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "commit content") {
		t.Errorf("expected skill body in result after slash strip, got: %s", res.Content)
	}
	if got := res.Metadata["leadingSlashStripped"]; got != "true" {
		t.Errorf("expected metadata leadingSlashStripped=true, got %q", got)
	}
	if got := res.Metadata["commandName"]; got != "commit" {
		t.Errorf("expected commandName=commit, got %q", got)
	}
}

func TestSkillTool_BadPluginNamespaceRejected(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"skill": ":bad",
	})
	if !res.IsError {
		t.Errorf("expected IsError for malformed plugin namespace, got: %s", res.Content)
	}
}

// -----------------------------------------------------------------------
// SkillTool inflight guard
// -----------------------------------------------------------------------

func TestSkillTool_RecursiveInvocationRejected(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "loopy", "loopy content")

	tool := &SkillTool{
		Manager:  newTestSkillManager(skillsDir),
		inflight: map[string]struct{}{"loopy": {}},
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"skill": "loopy",
	})
	if !res.IsError {
		t.Errorf("expected IsError for recursive invocation, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "recursive") {
		t.Errorf("expected 'recursive' in error, got: %s", res.Content)
	}
}

func TestSkillTool_InflightClearedAfterCompletion(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "once", "once content")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	if _, err := tool.Execute(context.Background(), map[string]any{
		"skill": "once",
	}); err != nil {
		t.Fatal(err)
	}

	// Inflight should be empty for a successful run.
	tool.inflightMu.Lock()
	defer tool.inflightMu.Unlock()
	if _, busy := tool.inflight["once"]; busy {
		t.Error("expected inflight set to be cleared after execution")
	}
}
