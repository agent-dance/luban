package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/skills"
)

// -------- skill-helpers --------

func TestResolveSkillModelOverride(t *testing.T) {
	cases := []struct {
		name   string
		skill  string
		parent string
		want   string
	}{
		{"empty skill returns empty", "", "sonnet[1m]", ""},
		{"no parent suffix returns skill verbatim", "sonnet", "opus", "sonnet"},
		{"carry [1m] when parent has it", "sonnet", "sonnet[1m]", "sonnet[1m]"},
		{"carry [1m] across model families", "haiku", "sonnet[1m]", "haiku[1m]"},
		{"skill already has suffix is honored", "sonnet[2m]", "sonnet[1m]", "sonnet[2m]"},
		{"empty parent returns skill verbatim", "sonnet", "", "sonnet"},
		{"trims whitespace", "  sonnet  ", "  sonnet[1m]  ", "sonnet[1m]"},
		{"malformed parent (open bracket only)", "sonnet", "sonnet[1m", "sonnet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveSkillModelOverride(tc.skill, tc.parent)
			if got != tc.want {
				t.Errorf("resolveSkillModelOverride(%q, %q) = %q; want %q", tc.skill, tc.parent, got, tc.want)
			}
		})
	}
}

func TestMatchSkillRule(t *testing.T) {
	cases := []struct {
		rule, name string
		want       bool
	}{
		{"commit", "commit", true},
		{"commit", "review", false},
		{"/commit", "commit", true},             // slash normalized on rule
		{"commit", "/commit", true},             // slash normalized on name
		{"myplugin:*", "myplugin:commit", true}, // plugin wildcard
		{"myplugin:*", "myplugin:review", true}, // plugin wildcard
		{"myplugin:*", "other:commit", false},   // wildcard scoped
		{"review:*", "review-pr", true},         // prefix-of-prefix matches TS behavior
		{":*", "anything", false},               // empty prefix is no-op
		{"", "commit", false},                   // empty rule never matches
		{"commit", "", false},                   // empty name never matches
	}
	for _, tc := range cases {
		t.Run(tc.rule+"|"+tc.name, func(t *testing.T) {
			got := MatchSkillRule(tc.rule, tc.name)
			if got != tc.want {
				t.Errorf("MatchSkillRule(%q, %q) = %v; want %v", tc.rule, tc.name, got, tc.want)
			}
		})
	}
}

func TestFirstMatchingSkillRule(t *testing.T) {
	rules := []string{"foo:*", "commit", "review-pr"}
	if got := FirstMatchingSkillRule(rules, "commit"); got != "commit" {
		t.Errorf("expected 'commit'; got %q", got)
	}
	if got := FirstMatchingSkillRule(rules, "foo:bar"); got != "foo:*" {
		t.Errorf("expected 'foo:*'; got %q", got)
	}
	if got := FirstMatchingSkillRule(rules, "missing"); got != "" {
		t.Errorf("expected '' for no match; got %q", got)
	}
}

func TestSkillHasOnlySafeProperties(t *testing.T) {
	if !skillHasOnlySafeProperties(nil) {
		t.Error("nil skill should be considered safe")
	}
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
		t.Error("skill with model override is safe (TS treats model in SAFE_SKILL_PROPERTIES)")
	}
}

// -------- skill error codes --------

func TestSkillErrorCodes(t *testing.T) {
	if SkillErrInvalidFormat.Int() != 1 {
		t.Error("invalid format must be code 1")
	}
	if SkillErrUnknownSkill.Int() != 2 {
		t.Error("unknown skill must be code 2")
	}
	if SkillErrDisableModelInvoke.Int() != 4 {
		t.Error("disable-model-invocation must be code 4")
	}
	if SkillErrNotPromptType.Int() != 5 {
		t.Error("not prompt type must be code 5")
	}
	if SkillErrRemoteNotDiscovered.Int() != 6 {
		t.Error("remote not discovered must be code 6")
	}
	if SkillErrInvalidFormat.String() != "invalid_format" {
		t.Errorf("invalid format label mismatch; got %s", SkillErrInvalidFormat.String())
	}
}

// -------- disable-model-invocation gate --------

func TestSkillTool_DisableModelInvocationRejected(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "deploy", "---\ndisable-model-invocation: true\ndescription: deploy\n---\nrun the deploy.")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "deploy"})
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

func TestSkillTool_RuntimeDisableIsEnforcedPerSession(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "review", "review body")

	manager := newTestSkillManager(dir)
	if changed, found := manager.SetEnabled("session-a", "review", false); !found || !changed {
		t.Fatalf("disable review = changed %t found %t", changed, found)
	}
	tool := &SkillTool{Manager: manager}

	disabledCtx := WithSkillSessionID(context.Background(), "session-a")
	res, _ := tool.Execute(disabledCtx, map[string]any{"skill": "review"})
	if !res.IsError || !strings.Contains(res.Content, "disabled for this session") {
		t.Fatalf("disabled session result = %+v", res)
	}
	if got := res.Metadata["availability"]; got != "disabled" {
		t.Fatalf("availability metadata = %q, want disabled", got)
	}

	enabledCtx := WithSkillSessionID(context.Background(), "session-b")
	res, _ = tool.Execute(enabledCtx, map[string]any{"skill": "review"})
	if res.IsError {
		t.Fatalf("session-a override leaked into session-b: %+v", res)
	}
}

// -------- structured error codes --------

func TestSkillTool_UnknownSkillReturnsCode2(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "nonexistent"})
	if !res.IsError {
		t.Fatalf("expected unknown skill error")
	}
	if got := res.Metadata["errorCode"]; got != "2" {
		t.Errorf("expected errorCode=2 for unknown skill; got %q", got)
	}
}

func TestSkillTool_EmptyNameReturnsCode1(t *testing.T) {
	tool := &SkillTool{Manager: newTestSkillManager()}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": ""})
	if !res.IsError {
		t.Fatalf("expected invalid format error")
	}
	if got := res.Metadata["errorCode"]; got != "1" {
		t.Errorf("expected errorCode=1 for empty name; got %q", got)
	}
}

// -------- leading-slash strip --------

func TestSkillTool_LeadingSlashIsStripped(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "review", "review body")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "/review"})
	if res.IsError {
		t.Fatalf("expected slash strip to recover; got error: %s", res.Content)
	}
	if got := res.Metadata["leadingSlashStripped"]; got != "true" {
		t.Errorf("expected leadingSlashStripped=true; got %q", got)
	}
}

func TestNormalizeSkillName(t *testing.T) {
	if n, s := normalizeSkillName("/commit"); n != "commit" || !s {
		t.Errorf("expected (commit, true); got (%q, %v)", n, s)
	}
	if n, s := normalizeSkillName("review"); n != "review" || s {
		t.Errorf("expected (review, false); got (%q, %v)", n, s)
	}
	if n, s := normalizeSkillName("  review  "); n != "review" || s {
		t.Errorf("expected (review, false) after trim; got (%q, %v)", n, s)
	}
}

// -------- permission rules: deny + allow --------

func TestSkillTool_DenyRuleBlocks(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "danger", "danger body")

	tool := &SkillTool{
		Manager:   newTestSkillManager(dir),
		DenyRules: []string{"danger"},
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "danger"})
	if !res.IsError {
		t.Fatalf("deny rule must block execution; got success: %s", res.Content)
	}
	if got := res.Metadata["permissionDecision"]; got != "deny" {
		t.Errorf("expected permissionDecision=deny; got %q", got)
	}
	if got := res.Metadata["matchedRule"]; got != "danger" {
		t.Errorf("expected matchedRule=danger; got %q", got)
	}
}

func TestSkillTool_AllowRulePermits(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	// allowed-tools makes the skill non-safe; an explicit allow rule is the
	// only path to a non-prompt allow decision in this case.
	writeMDSkill(t, dir, "build",
		"---\ndescription: build\nallowed-tools: Bash\n---\nbuild body")

	tool := &SkillTool{
		Manager:    newTestSkillManager(dir),
		AllowRules: []string{"build"},
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "build"})
	if res.IsError {
		t.Fatalf("allow rule should permit execution; got: %s", res.Content)
	}
	if got := res.Metadata["permissionDecision"]; got != "allow" {
		t.Errorf("expected permissionDecision=allow; got %q", got)
	}
}

func TestSkillTool_AllowRulePluginWildcard(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "myplugin", "---\ndescription: m\nallowed-tools: Bash\n---\nm body")

	// The skill name "myplugin" is matched by a "myplugin:*" rule via prefix.
	// Use a closer test: the rule "myplugin:*" matches "myplugin:commit"
	// via HasPrefix. Here we use a non-wildcard exact match instead since
	// the file-system loader cannot store ":" in a directory name on Windows.
	tool := &SkillTool{
		Manager:    newTestSkillManager(dir),
		AllowRules: []string{"my*"}, // not a TS form — should NOT match
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "myplugin"})
	if res.IsError {
		t.Fatalf("safe skill should auto-allow regardless: got %s", res.Content)
	}
	// Skill has allowed-tools, so it is NOT safe — without rule match,
	// permissionDecision should be "ask" (the harness prompts).
	if got := res.Metadata["permissionDecision"]; got != "ask" {
		t.Errorf("expected permissionDecision=ask without rule match; got %q", got)
	}
}

// -------- safe-properties auto-allow --------

func TestSkillTool_SafePropertiesAutoAllow(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "trivial", "---\ndescription: trivial helper\n---\ntrivial body")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "trivial"})
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
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "build"})
	if res.IsError {
		t.Fatalf("expected skill to run; got error %s", res.Content)
	}
	if got := res.Metadata["permissionDecision"]; got != "ask" {
		t.Errorf("non-safe skill should require permission (ask); got %q", got)
	}
}

// -------- model override carries [1m] --------

func TestSkillTool_ModelOverridePreserves1mSuffix(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "ms", "---\ndescription: m\nmodel: sonnet\n---\nbody")

	tool := &SkillTool{
		Manager: newTestSkillManager(dir),
		ParentModelResolver: func(context.Context) string {
			return "opus[1m]"
		},
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "ms"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if got := res.Metadata["model"]; got != "sonnet[1m]" {
		t.Errorf("expected model carry [1m]; got %q", got)
	}
}

func TestSkillTool_ModelOverrideNoSuffixPropagation(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "ms", "---\ndescription: m\nmodel: sonnet\n---\nbody")

	tool := &SkillTool{
		Manager: newTestSkillManager(dir),
		ParentModelResolver: func(context.Context) string {
			return "opus"
		},
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "ms"})
	if got := res.Metadata["model"]; got != "sonnet" {
		t.Errorf("expected plain sonnet; got %q", got)
	}
}

// -------- effort --------

func TestSkillTool_EffortInMetadata(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "plan", "---\ndescription: p\neffort: high\n---\nbody")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "plan"})
	if got := res.Metadata["effort"]; got != "high" {
		t.Errorf("expected effort=high; got %q", got)
	}
}

// -------- allowed-tools applier hook --------

func TestSkillTool_AllowedToolsApplierInvoked(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "buildit",
		"---\ndescription: b\nallowed-tools: Bash, Read\n---\nbody")

	var seenTools []string
	var cleanupCalled bool
	tool := &SkillTool{
		Manager: newTestSkillManager(dir),
		AllowedToolsApplier: func(ctx context.Context, sessionID, name string, tools []string) func() {
			seenTools = append([]string(nil), tools...)
			return func() { cleanupCalled = true }
		},
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "buildit"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if len(seenTools) != 2 {
		t.Errorf("expected applier to receive 2 tools; got %v", seenTools)
	}
	if !cleanupCalled {
		t.Error("expected cleanup to fire on Execute return")
	}
}

// -------- frontmatter strip is applied to model context --------

func TestSkillTool_FrontmatterStrippedFromContent(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "fm", "---\ndescription: with fm\nmodel: sonnet\n---\nThe real body.")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	res, _ := tool.Execute(context.Background(), map[string]any{"skill": "fm"})
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

// -------- invoked skills compaction state --------

func TestSkillTool_InvokedSkillsRecorded(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "review", "review body")

	tool := &SkillTool{Manager: newTestSkillManager(dir)}
	ctx := WithSkillSessionID(context.Background(), "session-A")
	ctx = WithSkillToolUseID(ctx, "tool-use-123")
	if _, err := tool.Execute(ctx, map[string]any{"skill": "review"}); err != nil {
		t.Fatalf("execute err: %v", err)
	}
	recs := tool.InvokedSkills("session-A")
	if len(recs) != 1 {
		t.Fatalf("expected 1 invoked skill; got %d", len(recs))
	}
	if recs[0].SkillName != "review" {
		t.Errorf("expected skill=review; got %q", recs[0].SkillName)
	}
	if recs[0].ToolUseID != "tool-use-123" {
		t.Errorf("expected toolUseID=tool-use-123; got %q", recs[0].ToolUseID)
	}
}

// -------- usage store wiring --------

func TestSkillTool_UsageStoreRecorded(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "review", "review body")

	store := NewSkillUsageStore("") // in-memory only
	tool := &SkillTool{Manager: newTestSkillManager(dir), UsageStore: store}
	for i := 0; i < 3; i++ {
		_, _ = tool.Execute(context.Background(), map[string]any{"skill": "review"})
	}
	if got := store.Count("review"); got != 3 {
		t.Errorf("expected count=3; got %d", got)
	}
}

func TestSkillUsageStore_RankedNames(t *testing.T) {
	store := NewSkillUsageStore("")
	store.Record("zeta")
	store.Record("alpha")
	store.Record("alpha")
	store.Record("alpha")

	got := store.RankedNames([]string{"alpha", "beta", "zeta"})
	want := []string{"alpha", "zeta", "beta"} // alpha=3, zeta=1, beta=0
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rank[%d] = %q; want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}
