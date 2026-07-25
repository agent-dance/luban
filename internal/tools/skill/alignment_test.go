package skill

// Contract tests for SkillTool discovery, frontmatter, argument forwarding,
// inflight guards, allowed-tools enforcement, and metadata fields.
//
// Run only these tests with:
//
//	go test -run SkillAlignment -count=1 ./internal/tools/skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// makeSkillFixture creates a temporary skill directory containing a single
// skill named "demo" whose body references both ${LUBAN_SESSION_ID} and
// $ARGUMENTS so we can detect substitutions in tests.
func makeSkillFixture(t *testing.T, name, body, frontmatter string) skills.DirSource {
	t.Helper()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	full := body
	if frontmatter != "" {
		full = "---\n" + frontmatter + "\n---\n" + body
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(full), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	return skills.DirSource{Dir: dir, Source: skills.SourceProject}
}

// -------- Session ID injection --------
//
// The model invocation helper pins a session and generation from the same
// catalog snapshot, matching the authority supplied by a production QueryLoop.
func TestSkillAlignment_SessionIDIsInjected(t *testing.T) {
	dir := makeSkillFixture(t, "demo", "session=${LUBAN_SESSION_ID}", "")
	tool := &SkillTool{Manager: newTestSkillManagerFromSources(dir)}

	ctx := context.Background()
	res, err := executeSkillModelTest(t, tool, ctx, map[string]any{"skill": "demo"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Content, "session=") {
		t.Fatalf("expected skill output to include session= prefix, got %q", res.Content)
	}
	if strings.Contains(res.Content, "session=${LUBAN_SESSION_ID}") || strings.Contains(res.Content, "session=\n") || strings.HasSuffix(strings.TrimSpace(res.Content), "session=") {
		t.Errorf("sessionID was not substituted (P1-8, skill.go:130 TODO); got %q", res.Content)
	}
}

// -------- Structured result contract --------
//
// Skill results expose structured execution metadata alongside content.
func TestSkillAlignment_OutputContractHasMetadata(t *testing.T) {
	dir := makeSkillFixture(t, "metaskill", "body", "description: greet")
	tool := &SkillTool{Manager: newTestSkillManagerFromSources(dir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "metaskill"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Metadata == nil {
		t.Fatalf("Skill result must populate Metadata (P1-8); got nil")
	}
	for _, key := range []string{"success", "commandName", "allowedTools", "status"} {
		if _, ok := res.Metadata[key]; !ok {
			t.Errorf("Skill result Metadata is missing %q (audit row 9, P1-8); got %v", key, res.Metadata)
		}
	}
	if v, ok := res.Metadata["status"]; ok {
		if v != "inline" && v != "forked" {
			t.Errorf("status must be inline|forked, got %q", v)
		}
	}
}

// -------- Boolean success metadata --------
func TestSkillAlignment_SuccessFieldIsTrueOnHappyPath(t *testing.T) {
	dir := makeSkillFixture(t, "ok", "ok body", "")
	tool := &SkillTool{Manager: newTestSkillManagerFromSources(dir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "ok"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Metadata == nil {
		t.Fatalf("metadata missing")
	}
	v, ok := res.Metadata["success"]
	if !ok {
		t.Fatalf("success missing from metadata; got %#v", res.Metadata)
	}
	if v != "true" {
		t.Errorf("success must be \"true\" on happy path; got %q", v)
	}
}

// -------- commandName mirrors the requested skill --------
func TestSkillAlignment_CommandNameEchoesInput(t *testing.T) {
	dir := makeSkillFixture(t, "review-pr", "body", "")
	tool := &SkillTool{Manager: newTestSkillManagerFromSources(dir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "review-pr"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Metadata == nil {
		t.Fatalf("metadata missing")
	}
	got := res.Metadata["commandName"]
	if got != "review-pr" {
		t.Errorf("commandName must echo input %q; got %q", "review-pr", got)
	}
}

// -------- allowedTools metadata mirrors frontmatter --------
//
// Skills declare `allowed-tools` in frontmatter. The Skill type loads them
// into AllowedTools but the SkillTool result never surfaces the list.
//
// Because Metadata values are strings, we accept any encoding (JSON list,
// comma-separated, semicolon-separated) as long as the tool names appear.
func TestSkillAlignment_AllowedToolsMetadataMirrorsFrontmatter(t *testing.T) {
	dir := makeSkillFixture(t, "scoped", "body",
		"description: scoped skill\nallowed-tools:\n  - Read\n  - Bash(ls)\n")
	tool := &SkillTool{Manager: newTestSkillManagerFromSources(dir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "scoped"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Metadata == nil {
		t.Fatalf("metadata missing")
	}
	allowed, present := res.Metadata["allowedTools"]
	if !present {
		t.Fatalf("allowedTools missing from metadata; got %v", res.Metadata)
	}
	if !strings.Contains(allowed, "Read") {
		t.Errorf("expected allowedTools metadata to surface \"Read\"; got %q", allowed)
	}
	if !strings.Contains(allowed, "Bash") {
		t.Errorf("expected allowedTools metadata to surface \"Bash\"; got %q", allowed)
	}
}

// -------- Model metadata reflects frontmatter --------
func TestSkillAlignment_ModelMetadataMirrorsFrontmatter(t *testing.T) {
	dir := makeSkillFixture(t, "modelpinned", "body", "description: x\nmodel: opus\n")
	tool := &SkillTool{Manager: newTestSkillManagerFromSources(dir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "modelpinned"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Metadata == nil {
		t.Fatalf("metadata missing")
	}
	got := res.Metadata["model"]
	if got != "opus" {
		t.Errorf("model must mirror frontmatter; got %q", got)
	}
}

// -------- Allowed-tools enforcement reaches the system prompt --------
//
// When a skill declares `allowed-tools`, the loop
// must restrict the tool list. The minimum observable signal at the tool
// level is that the result includes a textual `Allowed tools:` list so the
// outer loop can apply the restriction.
func TestSkillAlignment_OutputAdvertisesAllowedToolsToLoop(t *testing.T) {
	dir := makeSkillFixture(t, "scoped2", "body",
		"description: x\nallowed-tools:\n  - Read\n")
	tool := &SkillTool{Manager: newTestSkillManagerFromSources(dir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "scoped2"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Content), "allowed tools") {
		t.Errorf("Skill output must announce allowed-tools restriction (skill-07); got %q", res.Content)
	}
}

// -------- Frontmatter parser exposes argument names --------
//
// The frontmatter parser captures `arguments: [foo, bar]` as ArgNames for
// placeholder substitution. This verifies that the YAML list form reaches the
// rendered text.
func TestSkillAlignment_FrontmatterArgNamesDriveSubstitution(t *testing.T) {
	dir := makeSkillFixture(t, "args", "hello $name",
		"description: greet\narguments:\n  - name\n")
	mgr := newTestSkillManagerFromSources(dir)
	binding, err := mgr.SnapshotBinding("session")
	if err != nil {
		t.Fatal(err)
	}
	row := task16OnlySkill(t, mgr, "session")
	var got *skills.Skill
	result, err := mgr.ResolveLatest(skills.SkillResolveRequest{
		SessionID: "session", Selector: string(row.ID), ExpectedRevision: row.Revision,
		ExpectedProjectGeneration: binding.ProjectGeneration, Origin: skills.InvocationOriginModel,
	}, func(resolved skills.ResolvedSkill) error {
		got = resolved.Skill
		return nil
	})
	if err != nil || result.Outcome != skills.SkillResolveResolved || got == nil {
		t.Fatalf("manager could not resolve skill: result=%#v err=%v", result, err)
	}
	found := false
	for _, n := range got.ArgNames {
		if n == "name" {
			found = true
		}
	}
	if !found {
		t.Errorf("frontmatter parser must expose `arguments` as ArgNames (skill-02); got %v", got.ArgNames)
	}

	tool := &SkillTool{Manager: mgr}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{
		"skill": "args",
		"args":  "world",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Content, "hello world") {
		t.Errorf("named arg substitution must replace $name with arg value; got %q", res.Content)
	}
}

// -------- Args forwarding format (ARGUMENTS: <args>) --------
//
// When a skill body contains no placeholders but the caller passes args,
// the invocation renderer appends an `ARGUMENTS: …` line. The Skill tool must
// forward args as a non-nil pointer
// while preserving the distinction between omitted and explicitly empty
// arguments.
func TestSkillAlignment_ArgsForwardingPropagatesEmptyArgsExplicitly(t *testing.T) {
	dir := makeSkillFixture(t, "argshape", "static body", "")
	tool := &SkillTool{Manager: newTestSkillManagerFromSources(dir)}

	// Empty-string args MUST be distinguished from omitted args. We give
	// the value an explicit empty string. Under the forwarding contract, the body
	// should remain stable but the Metadata.argumentsProvided flag should
	// be true.
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{
		"skill": "argshape",
		"args":  "", // explicit empty string
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Metadata == nil {
		t.Fatalf("metadata missing")
	}
	provided := res.Metadata["argumentsProvided"] == "true"
	if !provided {
		t.Errorf("argumentsProvided must be true when caller passed args (even \"\"); got false")
	}
}

// -------- Result is JSON-serialisable to UI --------
//
// The Skill result must marshal without losing metadata so
// the UI/dashboard can show the structured fields. We round-trip the
// ToolResult through JSON.
func TestSkillAlignment_ResultRoundTripsAsJSON(t *testing.T) {
	dir := makeSkillFixture(t, "rt", "body", "")
	tool := &SkillTool{Manager: newTestSkillManagerFromSources(dir)}
	res, err := executeSkillModelTest(t, tool, context.Background(), map[string]any{"skill": "rt"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wrapper := struct {
		Content  any               `json:"content"`
		Metadata map[string]string `json:"metadata"`
	}{Content: res.Content, Metadata: res.Metadata}
	data, err := json.Marshal(wrapper)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"metadata":{`) {
		t.Errorf("Skill ToolResult must carry non-empty metadata in JSON (audit row 9); got %s", string(data))
	}
}

// -------- Schema surfaces argument-hint description --------
//
// The Skill schema includes the rendered argument hint in the `args`
// description so the model knows what to pass.
func TestSkillAlignment_SchemaArgsDescriptionMentionsForwarding(t *testing.T) {
	tool := &SkillTool{
		Manager:          newTestSkillManagerFromSources(),
		LanguageResolver: func(context.Context) i18n.Language { return i18n.LangEN },
	}
	schema := tool.Schema()
	args, ok := schema.Properties["args"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing args property")
	}
	desc, _ := args["description"].(string)
	lower := strings.ToLower(desc)
	if !strings.Contains(lower, "argument-hint") && !strings.Contains(lower, "forwarded") {
		t.Errorf("args schema description should mention argument-hint or forwarded semantics (skill-03); got %q", desc)
	}
}

// -------- Skill name validation rejects path traversal --------
//
// validateSkillName rejects path separators and parent-traversal selectors.
func TestSkillAlignment_NameValidationRejectsParentTraversal(t *testing.T) {
	if err := validateSkillName("../etc/passwd"); err == nil {
		t.Errorf("validateSkillName must reject path traversal segments (skill-06); accepted")
	}
}

// guarded usage assertion to ensure imports are used even if some tests
// short-circuit via t.Errorf.
var _ types.Tool = (*SkillTool)(nil)
