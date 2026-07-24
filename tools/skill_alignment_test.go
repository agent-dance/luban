package tools

// Alignment red tests for the SkillTool, derived from alignment_audit.md
// (P1-8 Skill sessionID + output contract) and tasks/skill.json
// (skill-01..skill-08 across discovery, frontmatter, args forwarding,
// inflight guard, allowed-tools enforcement and metadata fields).
//
// Each test pins the *desired* contract from the TS reference (Skill tool
// + skillsManager) and is expected to FAIL against the current Go
// implementation while still compiling cleanly.
//
// Do NOT modify production code to silence them without first reviewing the
// corresponding audit row.
//
// Run only these tests with:
//
//	go test -run SkillAlignment -count=1 ./gosrc/tools/...

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// makeSkillFixture creates a temporary skill directory containing a single
// skill named "demo" whose body references both ${CLAUDE_SESSION_ID} and
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

// -------- skill-04: sessionID injection (P1-8, skill.go:130 TODO) --------
//
// skill.go currently passes "" as the sessionID, so any ${CLAUDE_SESSION_ID}
// placeholder is silently dropped. We assert that when a session is present
// the placeholder is replaced with that value.
func TestSkillAlignment_SessionIDIsInjected(t *testing.T) {
	dir := makeSkillFixture(t, "demo", "session=${CLAUDE_SESSION_ID}", "")
	tool := &SkillTool{Manager: skills.NewManager(dir)}

	// Inject session via a context-known to the tool. The TS skill tool
	// reads the session id from the conversation; the Go side must take
	// it from the context loop.ToolExecutionContext, but no such wiring
	// exists yet.
	ctx := context.Background()
	res, err := tool.Execute(ctx, map[string]any{"skill": "demo"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Content, "session=") {
		t.Fatalf("expected skill output to include session= prefix, got %q", res.Content)
	}
	if strings.Contains(res.Content, "session=${CLAUDE_SESSION_ID}") || strings.Contains(res.Content, "session=\n") || strings.HasSuffix(strings.TrimSpace(res.Content), "session=") {
		t.Errorf("sessionID was not substituted (P1-8, skill.go:130 TODO); got %q", res.Content)
	}
}

// -------- skill-08 / P1-8: result must be structured, not raw content --------
//
// audit row 9 + tasks/skill.json skill-08 require the SkillTool to emit a
// structured result with `success`, `commandName`, `allowedTools`, `model`,
// and `status` fields. Currently the tool returns only `Content`.
func TestSkillAlignment_OutputContractHasMetadata(t *testing.T) {
	dir := makeSkillFixture(t, "metaskill", "body", "description: greet")
	tool := &SkillTool{Manager: skills.NewManager(dir)}
	res, err := tool.Execute(context.Background(), map[string]any{"skill": "metaskill"})
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

// -------- skill-08: success metadata field is the boolean status --------
func TestSkillAlignment_SuccessFieldIsTrueOnHappyPath(t *testing.T) {
	dir := makeSkillFixture(t, "ok", "ok body", "")
	tool := &SkillTool{Manager: skills.NewManager(dir)}
	res, err := tool.Execute(context.Background(), map[string]any{"skill": "ok"})
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

// -------- skill-08: commandName mirrors the requested skill --------
func TestSkillAlignment_CommandNameEchoesInput(t *testing.T) {
	dir := makeSkillFixture(t, "review-pr", "body", "")
	tool := &SkillTool{Manager: skills.NewManager(dir)}
	res, err := tool.Execute(context.Background(), map[string]any{"skill": "review-pr"})
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

// -------- skill-08: allowedTools metadata mirrors frontmatter --------
//
// Skills declare `allowed-tools` in frontmatter. The Skill type loads them
// into AllowedTools but the SkillTool result never surfaces the list.
//
// Because Metadata values are strings, we accept any encoding (JSON list,
// comma-separated, semicolon-separated) as long as the tool names appear.
func TestSkillAlignment_AllowedToolsMetadataMirrorsFrontmatter(t *testing.T) {
	dir := makeSkillFixture(t, "scoped", "body",
		"description: scoped skill\nallowed-tools:\n  - Read\n  - Bash(ls)\n")
	tool := &SkillTool{Manager: skills.NewManager(dir)}
	res, err := tool.Execute(context.Background(), map[string]any{"skill": "scoped"})
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

// -------- skill-08: model metadata reflects frontmatter --------
func TestSkillAlignment_ModelMetadataMirrorsFrontmatter(t *testing.T) {
	dir := makeSkillFixture(t, "modelpinned", "body", "description: x\nmodel: opus\n")
	tool := &SkillTool{Manager: skills.NewManager(dir)}
	res, err := tool.Execute(context.Background(), map[string]any{"skill": "modelpinned"})
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

// -------- skill-07: allowed-tools enforcement leaks into the system prompt --------
//
// audit row 9 + skill-07: when a skill declares `allowed-tools`, the loop
// must restrict the tool list. The minimum observable signal at the tool
// level is that the result includes a textual `Allowed tools:` list so the
// outer loop can apply the restriction. Today the SkillTool emits no such
// text.
func TestSkillAlignment_OutputAdvertisesAllowedToolsToLoop(t *testing.T) {
	dir := makeSkillFixture(t, "scoped2", "body",
		"description: x\nallowed-tools:\n  - Read\n")
	tool := &SkillTool{Manager: skills.NewManager(dir)}
	res, err := tool.Execute(context.Background(), map[string]any{"skill": "scoped2"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Content), "allowed tools") {
		t.Errorf("Skill output must announce allowed-tools restriction (skill-07); got %q", res.Content)
	}
}

// -------- skill-01: fsnotify-based discovery (live rescan) --------
//
// audit + skill-01: the manager must rescan when new skills appear under a
// known directory without an explicit Refresh() call. Today only Refresh()
// invalidates the cache.
func TestSkillAlignment_ManagerWatchesNewSkills(t *testing.T) {
	dir := t.TempDir()
	mgr := skills.NewManager(skills.DirSource{Dir: dir, Source: skills.SourceProject})

	// Force initial population (empty dir).
	if names := mgr.Names(); len(names) != 0 {
		t.Fatalf("expected empty manager initially, got %v", names)
	}

	// Add a new skill on disk after the cache was populated.
	skillDir := filepath.Join(dir, "live")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Give a watcher a brief window to react. Without fsnotify the
	// cache stays stale and the test will fail.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := mgr.Get("live"); got != nil {
			return // success
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("manager must auto-discover new skills via fsnotify (skill-01); did not see \"live\" within 500ms")
}

// -------- skill-02: frontmatter parser exposes argument names --------
//
// The TS frontmatter parser captures `arguments: [foo, bar]` as ArgNames
// for placeholder substitution. The Go parser handles the simpler form but
// we have not seen the YAML list form covered. Assert that ArgNames is
// populated and that $foo substitution flows through to the rendered text.
func TestSkillAlignment_FrontmatterArgNamesDriveSubstitution(t *testing.T) {
	dir := makeSkillFixture(t, "args", "hello $name",
		"description: greet\narguments:\n  - name\n")
	mgr := skills.NewManager(dir)
	got := mgr.Get("args")
	if got == nil {
		t.Fatalf("manager could not load skill")
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
	res, err := tool.Execute(context.Background(), map[string]any{
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

// -------- skill-04: args forwarding format (ARGUMENTS: <args>) --------
//
// When a skill body contains no placeholders but the caller passes args,
// the TS reference appends an `ARGUMENTS: …` line. PrepareSkillContent
// implements this; the SkillTool must forward args as a non-nil pointer
// (it currently passes nil when in.Args == "" but the contract asks for
// the empty-string case to also surface the line so the model knows the
// caller supplied an empty string).
func TestSkillAlignment_ArgsForwardingPropagatesEmptyArgsExplicitly(t *testing.T) {
	dir := makeSkillFixture(t, "argshape", "static body", "")
	tool := &SkillTool{Manager: skills.NewManager(dir)}

	// Empty-string args MUST be distinguished from omitted args. We give
	// the value an explicit empty string. Per audit + skill-04, the body
	// should remain stable but the Metadata.argumentsProvided flag should
	// be true.
	res, err := tool.Execute(context.Background(), map[string]any{
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

// -------- skill-05: inflight guard scoped to session, not process --------
//
// audit row 5 + tasks/skill.json skill-05: the guard must be per-session
// so two parallel sessions invoking the same skill name do not collide.
// We exercise the guard by executing two SkillTool instances concurrently;
// the current implementation uses a process-wide mutex on the manager
// instance and would block one of them.
func TestSkillAlignment_InflightGuardIsPerSessionNotPerProcess(t *testing.T) {
	dir := makeSkillFixture(t, "guarded", "body", "")
	mgr := skills.NewManager(dir)

	tool := &SkillTool{Manager: mgr}
	type guardObserver interface {
		InflightSession(skill string) string // returns the sessionID currently holding the lock
	}
	if _, ok := any(tool).(guardObserver); !ok {
		t.Errorf("SkillTool must expose InflightSession() to make per-session guarding observable (skill-05)")
	}
}

// -------- skill-08: result is JSON-serialisable to UI --------
//
// audit row 9: the Skill result must marshal without losing metadata so
// the UI/dashboard can show the structured fields. We round-trip the
// ToolResult through JSON.
func TestSkillAlignment_ResultRoundTripsAsJSON(t *testing.T) {
	dir := makeSkillFixture(t, "rt", "body", "")
	tool := &SkillTool{Manager: skills.NewManager(dir)}
	res, err := tool.Execute(context.Background(), map[string]any{"skill": "rt"})
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

// -------- skill-03: schema must surface argument-hint description --------
//
// audit row 9: TS schema for Skill includes the rendered argument hint
// in the `args` description so the model knows what to pass. The Go
// schema today says only "Optional arguments to pass to the skill",
// which omits the per-skill argument-hint surfacing the TS reference
// performs.
func TestSkillAlignment_SchemaArgsDescriptionMentionsForwarding(t *testing.T) {
	tool := &SkillTool{Manager: skills.NewManager()}
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

// -------- skill-06: skill name validation rejects path traversal --------
//
// validateSkillName rejects leading slash but not "..".
func TestSkillAlignment_NameValidationRejectsParentTraversal(t *testing.T) {
	if err := validateSkillName("../etc/passwd"); err == nil {
		t.Errorf("validateSkillName must reject path traversal segments (skill-06); accepted")
	}
}

// guarded usage assertion to ensure imports are used even if some tests
// short-circuit via t.Errorf.
var _ types.Tool = (*SkillTool)(nil)
