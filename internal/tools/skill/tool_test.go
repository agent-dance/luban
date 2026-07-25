package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
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
	manager := skills.NewManager(sources...)
	manager.SetOverrideStore(skillTestOverrideStore{})
	return manager
}

func newTestSkillManagerFromSources(sources ...skills.DirSource) *skills.Manager {
	manager := skills.NewManager(sources...)
	manager.SetOverrideStore(skillTestOverrideStore{})
	return manager
}

type skillTestOverrideStore struct{}

func (skillTestOverrideStore) Snapshot(string) (skills.OverrideSnapshot, error) {
	return skills.OverrideSnapshot{
		User: map[skills.SkillID]skills.VisibilityOverride{}, Project: map[skills.SkillID]skills.VisibilityOverride{},
		Managed: map[skills.SkillID]skills.VisibilityOverride{}, Session: map[skills.SkillID]skills.VisibilityOverride{},
	}, nil
}

func (skillTestOverrideStore) Set(string, skills.VisibilityOverride) error { return nil }
func (skillTestOverrideStore) Reset(string, skills.SkillScope, skills.SkillID) error {
	return nil
}
func (skillTestOverrideStore) CompareAndSetProject(skills.OverrideStoreRevision, skills.SkillID, *skills.VisibilityOverride) (skills.ProjectOverrideRestore, error) {
	return skills.ProjectOverrideRestore{}, skills.ErrOverrideRevisionConflict
}
func (skillTestOverrideStore) RestoreProject(skills.ProjectOverrideRestore) error { return nil }

// executeSkillModelTest exercises the authoritative model-origin invocation
// path with a generation pinned from the same catalog snapshot. Production
// model calls obtain both values from their owning QueryLoop; unit tests must
// not rely on the removed context-free Execute compatibility path.
func executeSkillModelTest(t testing.TB, tool *SkillTool, ctx context.Context, input map[string]any) (types.ToolResult, error) {
	t.Helper()
	if ctx == nil {
		ctx = context.Background()
	}
	if tool != nil && tool.LanguageResolver == nil {
		tool.LanguageResolver = func(context.Context) i18n.Language { return i18n.LangEN }
	}
	in, err := toolbase.ParseInput[skillToolInput](input)
	if err != nil {
		return tool.Execute(ctx, input)
	}
	if _, provided := input["revision"]; provided && in.Revision == 0 {
		return tool.Execute(ctx, input)
	}
	if tool == nil || tool.Manager == nil {
		return tool.Execute(ctx, input)
	}
	const sessionID = "session"
	binding, err := tool.Manager.SnapshotBinding(sessionID)
	if err != nil {
		return types.ToolResult{}, err
	}
	var arguments *string
	if _, provided := input["args"]; provided {
		value := in.Args
		arguments = &value
	}
	return tool.invoke(ctx, SkillInvocationRequest{
		SessionID: sessionID, Selector: in.Skill,
		ExpectedRevision:          skills.SkillRevision(in.Revision),
		ExpectedProjectGeneration: binding.ProjectGeneration,
		Origin:                    skills.InvocationOriginModel, Arguments: arguments,
	})
}

// -----------------------------------------------------------------------
// SkillTool.Execute – prompt-type skill
// -----------------------------------------------------------------------

func TestSkillTool_ExecutePrompt(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "review", "Please review the following code carefully.")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{
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

func TestSkillToolExecuteRejectsContextFreeModelInvocation(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "review", "review body")
	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}

	result, err := tool.Execute(context.Background(), map[string]any{"skill": "review"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("context-free model invocation succeeded: %#v", result)
	}
}

func TestSkillTool_ExecutePromptWithArgs(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "prompt-skill", "Base prompt with $ARGUMENTS")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{
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
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{
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
	writeMDSkill(t, skillsDir, "dirskill", "Use ${LUBAN_SKILL_DIR}/scripts/run.sh")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{
		"skill": "dirskill",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// SkillDir should be set and ${LUBAN_SKILL_DIR} replaced
	if strings.Contains(res.Content, "${LUBAN_SKILL_DIR}") {
		t.Errorf("expected ${LUBAN_SKILL_DIR} to be substituted, got: %s", res.Content)
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
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{
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
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for missing 'skill' param")
	}
}

func TestSkillTool_EmptySkillParam(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{
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
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{
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
	tool := &SkillTool{
		Manager:          newTestSkillManager(),
		LanguageResolver: func(context.Context) i18n.Language { return i18n.LangEN },
	}
	if tool.Name() != "Skill" {
		t.Errorf("Name: got %q, want %q", tool.Name(), "Skill")
	}
	description := tool.Description()
	if !strings.Contains(description, "blocking requirement") || !strings.Contains(description, "never mention") {
		t.Fatalf("Description lost required invocation ordering: %q", description)
	}
}

func TestSkillTool_Metadata(t *testing.T) {
	if got := (&SkillTool{}).ToolMetadata(nil); got != (types.ToolMetadata{}) {
		t.Fatalf("ToolMetadata() = %#v, want zero metadata", got)
	}
}

func TestSkillTool_Schema(t *testing.T) {
	tool := &SkillTool{
		Manager:          newTestSkillManager(),
		LanguageResolver: func(context.Context) i18n.Language { return i18n.LangEN },
	}
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
	if err := validateSkillName("/commit"); err == nil {
		t.Fatal("expected error for leading slash")
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
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "commit", "commit content")

	tool := &SkillTool{Manager: newTestSkillManager(skillsDir)}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{
		"skill": "/commit",
	})
	if !res.IsError || res.Metadata["errorCode"] != "1" {
		t.Fatalf("leading slash selector was not rejected: %#v", res)
	}
}

func TestSkillTool_BadPluginNamespaceRejected(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{
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
		Manager:        newTestSkillManager(skillsDir),
		inflightOwners: map[string]string{"loopy": "active-session"},
	}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{
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
	if _, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{
		"skill": "once",
	}); err != nil {
		t.Fatal(err)
	}

	// Inflight should be empty for a successful run.
	tool.inflightMu.Lock()
	defer tool.inflightMu.Unlock()
	if _, busy := tool.inflightOwners["once"]; busy {
		t.Error("expected inflight set to be cleared after execution")
	}
}
