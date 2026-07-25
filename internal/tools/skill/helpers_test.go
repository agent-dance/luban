package skill

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/skills"
)

func TestSkillHasOnlySafeProperties(t *testing.T) {
	bare := &skills.Skill{Name: "foo", Description: "bar"}
	if !skillHasOnlySafeProperties(bare) {
		t.Error("bare skill (description only) should be safe")
	}
	withTools := &skills.Skill{Name: "foo", AllowedTools: []string{"Bash"}}
	if skillHasOnlySafeProperties(withTools) {
		t.Error("skill with allowed-tools should require permission")
	}
	withShell := &skills.Skill{Name: "foo", Shell: "bash"}
	if skillHasOnlySafeProperties(withShell) {
		t.Error("skill with shell override should require permission")
	}
	withModel := &skills.Skill{Name: "foo", Model: "sonnet"}
	if !skillHasOnlySafeProperties(withModel) {
		t.Error("skill with model override should remain safe")
	}
}

// -------- skill error codes --------

func TestSkillErrorCodes(t *testing.T) {
	if int(skillErrInvalidFormat) != 1 {
		t.Error("invalid format must be code 1")
	}
	if int(skillErrUnknownSkill) != 2 {
		t.Error("unknown skill must be code 2")
	}
	if int(skillErrDisableModelInvoke) != 4 {
		t.Error("disable-model-invocation must be code 4")
	}
	if skillErrInvalidFormat.String() != "invalid_format" {
		t.Errorf("invalid format label mismatch; got %s", skillErrInvalidFormat.String())
	}
}

// -------- disable-model-invocation gate --------

func TestSkillTool_DisableModelInvocationRejected(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "deploy", "---\ndisable-model-invocation: true\ndescription: deploy\n---\nrun the deploy.")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "deploy"})
	if !res.IsError {
		t.Fatalf("expected error for disable-model-invocation skill; got: %s", res.Content)
	}
	if got := res.Metadata["errorCode"]; got != "4" {
		t.Errorf("expected errorCode 4; got %q (full meta=%v)", got, res.Metadata)
	}
	if got := res.Metadata["errorReason"]; got != "disable_model_invocation" {
		t.Errorf("expected errorReason=disable_model_invocation; got %q", got)
	}
}

// -------- structured error codes --------

func TestSkillTool_UnknownSkillReturnsCode2(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "nonexistent"})
	if !res.IsError {
		t.Fatalf("expected unknown skill error")
	}
	if got := res.Metadata["errorCode"]; got != "2" {
		t.Errorf("expected errorCode=2 for unknown skill; got %q", got)
	}
}

func TestSkillTool_EmptyNameReturnsCode1(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": ""})
	if !res.IsError {
		t.Fatalf("expected invalid format error")
	}
	if got := res.Metadata["errorCode"]; got != "1" {
		t.Errorf("expected errorCode=1 for empty name; got %q", got)
	}
}

// -------- safe-properties auto-allow --------

func TestSkillTool_SafePropertiesAutoAllow(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "trivial", "---\ndescription: trivial helper\n---\ntrivial body")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "trivial"})
	if res.IsError {
		t.Fatalf("safe skill should run without error: %s", res.Content)
	}
	if got := res.Metadata["permissionDecision"]; got != "allow" {
		t.Errorf("expected auto-allow for safe skill; got permissionDecision=%q", got)
	}
}

func TestSkillTool_NonSafePropertiesPromptsAsk(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "build",
		"---\ndescription: build\nallowed-tools: Bash\n---\nbuild body")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "build"})
	if res.IsError {
		t.Fatalf("expected skill to run; got error %s", res.Content)
	}
	if got := res.Metadata["permissionDecision"]; got != "ask" {
		t.Errorf("non-safe skill should require permission (ask); got %q", got)
	}
}

// -------- effort --------

func TestSkillTool_EffortInMetadata(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "plan", "---\ndescription: p\neffort: high\n---\nbody")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "plan"})
	if got := res.Metadata["effort"]; got != "high" {
		t.Errorf("expected effort=high; got %q", got)
	}
}

// -------- frontmatter strip is applied to model context --------

func TestSkillTool_FrontmatterStrippedFromContent(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "fm", "---\ndescription: with fm\nmodel: sonnet\n---\nThe real body.")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "fm"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if strings.Contains(res.Content, "model: sonnet") || strings.Contains(res.Content, "---") {
		t.Errorf("frontmatter must be stripped before injection; got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "The real body.") {
		t.Errorf("expected body in content; got: %s", res.Content)
	}
}
