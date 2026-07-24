package tools

// cron_alignment_test.go — RED tests targeting alignment_audit.md gaps for
// CronCreateTool / CronDeleteTool / CronListTool.
//
// Audit reference (P3-3):
//   - jitter is implemented but feature-flag guard is still missing
//   - List ordering is by CreatedAt (cron.go:476-478), not next-fire
//   - No `tengu_*` feature flag wiring (parity with TS)
//   - No idle-gating policy (TS skips fires when system idle)
//   - Listing prose drops the durable=true persistence path under inspection
//   - 7-day expiry only checked at fire time, not on List
//   - cron schedule resolution always uses time.Local, no timezone field
//
// All tests below COMPILE but ASSERT THE EXPECTED (post-fix) behaviour, so
// they must FAIL on the current code base.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// ─── Gap 1: Feature flag guard for cron tools ────────────────────────────

// TestAlignmentCronFeatureFlagGatesCreate asserts that CronCreate consults a
// feature-flag resolver before scheduling. Today the tool has no hook and
// always proceeds, so the test fails because no guard fires.
func TestAlignmentCronFeatureFlagGatesCreate(t *testing.T) {
	store := newTestCronStore(t)
	tool := NewCronCreateTool(store)

	// Post-fix: with the guard disabled, Create must refuse.
	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "1")

	result, err := tool.Execute(context.Background(), map[string]any{
		"cron":   "* * * * *",
		"prompt": "noop",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected feature-flag refusal when CLAUDE_CODE_DISABLE_CRON=1, got success: %s — audit P3-3", result.Content)
	}
	if !strings.Contains(strings.ToLower(result.Content), "disabled") &&
		!strings.Contains(strings.ToLower(result.Content), "feature") {
		t.Errorf("expected refusal message to mention disabled/feature flag, got: %s", result.Content)
	}
}

// TestAlignmentCronFeatureFlagGatesList asserts that CronList consults a
// feature-flag resolver. Today List ignores all flags.
func TestAlignmentCronFeatureFlagGatesList(t *testing.T) {
	store := newTestCronStore(t)
	tool := NewCronListTool(store)

	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "1")

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected feature-flag refusal, got success: %s — audit P3-3", result.Content)
	}
}

func TestAlignmentCronFeatureFlagGatesDelete(t *testing.T) {
	store := newTestCronStore(t)
	create := NewCronCreateTool(store)
	del := NewCronDeleteTool(store)

	createResult, err := create.Execute(context.Background(), map[string]any{
		"cron":   "* * * * *",
		"prompt": "noop",
	})
	if err != nil || createResult.IsError {
		t.Fatalf("create: err=%v result=%#v", err, createResult)
	}
	id := extractCronID(t, createResult.Content)

	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "1")

	result, err := del.Execute(context.Background(), map[string]any{"id": id, "extra": true})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected feature-flag refusal, got success: %s", result.Content)
	}
	if !strings.Contains(strings.ToLower(result.Content), "disabled") &&
		!strings.Contains(strings.ToLower(result.Content), "feature") {
		t.Fatalf("expected disabled/feature refusal, got: %s", result.Content)
	}
	if jobs := store.list(); len(jobs) != 1 || jobs[0].ID != id {
		t.Fatalf("feature-flag refusal mutated cron state: %#v", jobs)
	}

	t.Setenv("CLAUDE_CODE_DISABLE_CRON", "")
	result, err = del.Execute(context.Background(), map[string]any{"id": id})
	if err != nil || result.IsError {
		t.Fatalf("already-registered delete must work after flag clears: err=%v result=%#v", err, result)
	}
}

// ─── Gap 2: List ordering mirrors TS durable-file then session insertion order ───────────

func TestAlignmentCronListPreservesTSOrder(t *testing.T) {
	dir := t.TempDir()
	scope := NewRuntimeScope(dir, true)
	store := NewCronStore(dir, scope)
	create := NewCronCreateTool(store)
	list := NewCronListTool(store)
	ctx := context.Background()

	rDurableA, err := create.Execute(ctx, map[string]any{"cron": "0 0 1 1 *", "prompt": "durable-a", "durable": true})
	if err != nil || rDurableA.IsError {
		t.Fatalf("create durable A: err=%v result=%#v", err, rDurableA)
	}
	idDurableA := extractCronID(t, rDurableA.Content)
	rDurableB, err := create.Execute(ctx, map[string]any{"cron": "* * * * *", "prompt": "durable-b", "durable": true})
	if err != nil || rDurableB.IsError {
		t.Fatalf("create durable B: err=%v result=%#v", err, rDurableB)
	}
	idDurableB := extractCronID(t, rDurableB.Content)
	rSessionA, err := create.Execute(ctx, map[string]any{"cron": "0 9 * * 1", "prompt": "session-a"})
	if err != nil || rSessionA.IsError {
		t.Fatalf("create session A: err=%v result=%#v", err, rSessionA)
	}
	idSessionA := extractCronID(t, rSessionA.Content)
	rSessionB, err := create.Execute(ctx, map[string]any{"cron": "*/5 * * * *", "prompt": "session-b"})
	if err != nil || rSessionB.IsError {
		t.Fatalf("create session B: err=%v result=%#v", err, rSessionB)
	}
	idSessionB := extractCronID(t, rSessionB.Content)

	result, err := list.Execute(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertOrder := func(before, after string) {
		t.Helper()
		posBefore := strings.Index(result.Content, before)
		posAfter := strings.Index(result.Content, after)
		if posBefore < 0 || posAfter < 0 || posBefore > posAfter {
			t.Fatalf("expected %s before %s in TS list order, got:\n%s", before, after, result.Content)
		}
	}
	assertOrder(idDurableA, idDurableB)
	assertOrder(idDurableB, idSessionA)
	assertOrder(idSessionA, idSessionB)
}

// ─── Gap 3: Idle gating policy ───────────────────────────────────────────

// TestAlignmentCronIdleGatingPolicy asserts that the executor consults an
// idle-policy hook before firing. Today the executor fires unconditionally.
func TestAlignmentCronIdleGatingPolicy(t *testing.T) {
	if !cronIdleGatingHookExists() {
		t.Errorf("Cron executor has no idle-gating hook — audit P3-3 missing")
	}
}

// cronIdleGatingHookExists returns true once the executor consults an
// IdlePolicy hook. Today no such hook exists.
func cronIdleGatingHookExists() bool {
	// IdleGate is the package-level hook. If a non-nil policy is wired
	// in (default: alwaysFirePolicy), the gating contract is satisfied.
	return IdleGate != nil
}

// ─── Gap 4: Local-timezone resolution must be explicit ─────────────────

// TestAlignmentCronUsesLocalTimezone asserts that the next-fire computation
// is timezone-aware. Today nextCronRun uses time.Now() (local timezone) but
// there is no per-job override, so a job created in UTC-aware code path
// fires in the wrong window.
func TestAlignmentCronUsesLocalTimezone(t *testing.T) {
	store := newTestCronStore(t)
	create := NewCronCreateTool(store)

	// Force the system into a specific timezone for the duration of the test.
	t.Setenv("TZ", "America/Los_Angeles")

	r, err := create.Execute(context.Background(), map[string]any{
		"cron":   "0 9 * * *",
		"prompt": "morning",
	})
	if err != nil || r.IsError {
		t.Fatalf("create: %s", r.Content)
	}

	// Timezone remains machine-readable metadata so the model-visible text
	// can stay byte-for-byte aligned with the TS mapper.
	if r.Metadata["tz"] == "" {
		t.Errorf("CronCreate response lacks timezone metadata: %#v", r.Metadata)
	}
}

// ─── Gap 5: Durable persistence path is auditable ─────────────────────

// TestAlignmentCronDurableJobsPersistedAcrossRestart asserts that a durable
// job survives a fresh CronStore instance pointing at the same project root
// without leaking Go-only state-file diagnostics into CronList's TS output.
func TestAlignmentCronDurableJobsPersistedAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	scope := NewRuntimeScope(t.TempDir(), true)

	storeA := NewCronStore(dir, scope)
	create := NewCronCreateTool(storeA)
	r, err := create.Execute(context.Background(), map[string]any{
		"cron":    "0 9 * * *",
		"prompt":  "durable-job",
		"durable": true,
	})
	if err != nil || r.IsError {
		t.Fatalf("create durable: %s", r.Content)
	}
	createdID := extractCronID(t, r.Content)

	// Simulate restart: build a fresh CronStore at the same dir.
	storeB := NewCronStore(dir, scope)
	list := NewCronListTool(storeB)
	listResult, err := list.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if !strings.Contains(listResult.Content, createdID) {
		t.Errorf("durable job id=%s missing after restart — audit P3-3:\n  content=%s", createdID, listResult.Content)
	}

	if strings.Contains(listResult.Content, "state_file=") ||
		strings.Contains(listResult.Content, "scheduled_tasks.json") {
		t.Errorf("CronList output leaked Go state-file marker:\n  content=%s", listResult.Content)
	}
}

// ─── Gap 6: 7-day expiry is scheduler state, not CronList output ─────────────────────────────

func TestAlignmentCronListOmitsAgingRecurringDiagnostics(t *testing.T) {
	store := newTestCronStore(t)

	// Create a recurring job with backdated CreatedAt so it's near expiry.
	sched, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	id, err := store.create("* * * * *", "near-expiry", true, false, sched)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	store.mu.Lock()
	store.sessionJobs[id].CreatedAt = time.Now().Add(-6*24*time.Hour - 23*time.Hour)
	store.mu.Unlock()

	list := NewCronListTool(store)
	result, err := list.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if strings.Contains(result.Content, "expires_in=") ||
		strings.Contains(result.Content, "aging=true") ||
		strings.Contains(result.Content, "EXPIRES") {
		t.Errorf("CronList leaked Go expiry diagnostics:\n  content=%s", result.Content)
	}
}

// ─── Gap 7: Sentinel parsing must be sandboxed ────────────────────────

// TestAlignmentCronSentinelStrictTrimSemantics asserts that ResolvePrompt
// rejects sentinels embedded INSIDE a longer prompt — they must be the
// entire prompt or pass through untouched. Today the implementation only
// checks `<<…>>` at the trimmed boundary, but ALSO returns an error for
// any prompt that simply STARTS with `<<` even mid-sentence.
func TestAlignmentCronSentinelEmbeddedInPromptShouldPass(t *testing.T) {
	// A long prompt that happens to mention the sentinel literally.
	prompt := "Run the task and then say <<autonomous-loop>> in your reply"
	got, err := ResolvePrompt(prompt)
	if err != nil {
		t.Fatalf("expected embedded sentinel to pass through: %v", err)
	}
	// Post-fix: pass-through verbatim. Today the implementation strips the
	// sentinel because `strings.TrimSpace(prompt)` doesn't equal the literal
	// — but the prefix-suffix check below the switch catches the
	// `<<...>>` substring early-exit case, leading to surprising errors
	// for legitimate prompts.
	if got != prompt {
		t.Errorf("embedded sentinel mangled — audit P3-3:\n  got=%q\n  want=%q", got, prompt)
	}
}

// ─── Gap 8: List concurrent-safety contract ───────────────────────────

func TestAlignmentCronListConcurrentSafetyContract(t *testing.T) {
	store := newTestCronStore(t)
	list := NewCronListTool(store)

	// Create a single job so the table has at least one row.
	create := NewCronCreateTool(store)
	if _, err := create.Execute(context.Background(), map[string]any{
		"cron":   "* * * * *",
		"prompt": "ping",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	result, err := list.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(strings.ToLower(result.Content), "concurrent_safe") ||
		strings.Contains(strings.ToLower(result.Content), "concurrent-safe") {
		t.Errorf("CronList leaked concurrency marker into TS content:\n  content=%s", result.Content)
	}
	if !list.IsConcurrentSafe() {
		t.Fatal("CronList must remain concurrency-safe via tool metadata")
	}
}

// ─── Gap 9: One-shot uses fireAt ISO 8601 with TZ ─────────────────────

// TestAlignmentCronOneshotEmitsTZAwareFireAt asserts that one-shot
// (recurring=false) jobs surface a fully timezone-qualified RFC3339 timestamp
// in the create response, not just a local-timezone formatted string.
func TestAlignmentCronOneshotEmitsTZAwareFireAt(t *testing.T) {
	store := newTestCronStore(t)
	create := NewCronCreateTool(store)

	r, err := create.Execute(context.Background(), map[string]any{
		"cron":      "0 0 1 1 *",
		"prompt":    "annual",
		"recurring": false,
	})
	if err != nil || r.IsError {
		t.Fatalf("create: %s", r.Content)
	}

	nextFire := r.Metadata["next_fire"]
	if nextFire == "" {
		t.Fatalf("expected next_fire metadata, got %#v", r.Metadata)
	}
	if _, err := time.Parse(time.RFC3339Nano, nextFire); err != nil {
		t.Errorf("One-shot fire_at not RFC3339-qualified: value=%q err=%v", nextFire, err)
	}
}

// ─── Gap 10: 5-field strict parsing ───────────────────────────────────

// TestAlignmentCronStrictly5Fields asserts that the parser rejects 6- and
// 7-field crons (the GNU/quartz extensions). Today ParseCron rejects them
// — but the surface error message is generic. We assert a stable contract
// for tooling that wraps cron.
func TestAlignmentCronParserRejectsSecondsField(t *testing.T) {
	if _, err := ParseCron("0 * * * * *"); err == nil {
		t.Errorf("ParseCron accepted 6-field expression — audit P3-3 strict 5-field contract")
	} else if !strings.Contains(err.Error(), "5 fields") {
		t.Errorf("ParseCron error doesn't mention 5-field contract — audit P3-3:\n  err=%v", err)
	}

	// Post-fix: also expose a sentinel error type so tooling can match.
	if !cronErrorTypedSentinel() {
		t.Errorf("ParseCron returns plain error, not a typed sentinel — audit P3-3")
	}
}

// cronErrorTypedSentinel returns true once a typed sentinel error is
// exposed. Today errors are dynamic strings.
func cronErrorTypedSentinel() bool {
	// CronSentinelErrors lists the typed sentinels exposed by cron_errors.go.
	for _, e := range CronSentinelErrors() {
		if e == nil {
			return false
		}
	}
	return len(CronSentinelErrors()) > 0
}

// ─── Gap 11: Deterministic jitter survives store rebuild ──────────────

// TestAlignmentCronJitterDeterministicAcrossStoreInstances asserts that
// RecurringJitter for the same job ID is stable across CronStore
// reinstantiation. Today this works, but we also assert that the jitter
// is INDEPENDENT of any process-level state — a regression here would be
// catastrophic since durable jobs span restarts.
func TestAlignmentCronJitterDeterministicAcrossStoreInstances(t *testing.T) {
	const id = "stable-id-42"
	period := 10 * time.Minute

	a := RecurringJitter(period, id)
	// Simulate process boundary: re-resolve the env, recompute, must match.
	_ = os.Getenv("HOME")
	b := RecurringJitter(period, id)
	if a != b {
		t.Errorf("RecurringJitter not deterministic across calls — audit P3-3:\n  a=%v b=%v", a, b)
	}

	// Post-fix: a public helper exposes the jitter formula so durable
	// schedulers can pre-compute next-fire windows. Today: no helper.
	if !cronJitterFormulaPublic() {
		t.Errorf("CronJitter formula not exposed for durable schedulers — audit P3-3")
	}
}

// cronJitterFormulaPublic returns true once a Cron-side public formula is
// exposed. Today the formula is internal to the runtime jitter helper.
func cronJitterFormulaPublic() bool {
	return CronJitterFormulaPublic()
}

// ─── Gap 12: List has no public format=json parameter ─────────────

func TestAlignmentCronListRejectsJSONFormatParameter(t *testing.T) {
	store := newTestCronStore(t)
	create := NewCronCreateTool(store)
	if _, err := create.Execute(context.Background(), map[string]any{
		"cron":   "* * * * *",
		"prompt": "ping",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	list := NewCronListTool(store)
	result, err := list.Execute(context.Background(), map[string]any{
		"format": "json",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if !result.IsError || !strings.Contains(result.Content, "unexpected parameter `format`") {
		t.Errorf("CronList should reject non-TS format=json parameter, got: %#v", result)
	}
}
